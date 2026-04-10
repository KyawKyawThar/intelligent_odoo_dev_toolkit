package worker

import (
	"context"
	"encoding/json"
	"time"

	db "Intelligent_Dev_ToolKit_Odoo/db/sqlc"
	"Intelligent_Dev_ToolKit_Odoo/internal/storage"

	"github.com/google/uuid"
	"github.com/rs/zerolog"
)

// RetentionConfig holds configuration for the retention worker.
type RetentionConfig struct {
	// RunInterval controls how often the cleanup cycle runs (default: 1h).
	RunInterval time.Duration
}

// DefaultRetentionConfig returns sensible defaults.
func DefaultRetentionConfig() RetentionConfig {
	return RetentionConfig{
		RunInterval: time.Hour,
	}
}

// tenantRetentionPolicy is parsed from a tenant's retention_config JSONB column.
// Zero values fall back to the package-level defaults below.
type tenantRetentionPolicy struct {
	ErrorTracesDays        int `json:"error_traces_days"`
	ProfilerRecordingsDays int `json:"profiler_recordings_days"`
	BudgetSamplesDays      int `json:"budget_samples_days"`
	SchemaSnapshotsKeep    int `json:"schema_snapshots_keep"`
	RawLogsDays            int `json:"raw_logs_days"`
	AuditLogsDays          int `json:"audit_log_days"`
}

var defaultPolicy = tenantRetentionPolicy{
	ErrorTracesDays:        7,
	ProfilerRecordingsDays: 7,
	BudgetSamplesDays:      30,
	SchemaSnapshotsKeep:    10,
	RawLogsDays:            3,
	AuditLogsDays:          90,
}

// RetentionWorker runs periodic cleanup of expired data across all tenants.
// It reads each tenant's retention_config JSONB to determine cutoff windows,
// deletes orphaned S3 traces before removing DB records, and logs row counts.
type RetentionWorker struct {
	store  db.Store
	s3     *storage.S3Client // nil → S3 cleanup skipped, relying on lifecycle policies
	config RetentionConfig
	logger zerolog.Logger
}

// NewRetentionWorker creates a RetentionWorker.
func NewRetentionWorker(
	store db.Store,
	s3 *storage.S3Client,
	cfg RetentionConfig,
	logger zerolog.Logger,
) *RetentionWorker {
	if cfg.RunInterval <= 0 {
		cfg.RunInterval = time.Hour
	}
	return &RetentionWorker{
		store:  store,
		s3:     s3,
		config: cfg,
		logger: logger.With().Str("component", "retention-worker").Logger(),
	}
}

// Run starts the ticker loop. Runs cleanup once immediately on startup,
// then repeats on each interval tick. Blocks until ctx is canceled.
func (w *RetentionWorker) Run(ctx context.Context) error {
	w.logger.Info().
		Dur("interval", w.config.RunInterval).
		Msg("retention worker starting")

	// First run immediately so the operator sees results without waiting.
	w.runAll(ctx)

	ticker := time.NewTicker(w.config.RunInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			w.runAll(ctx)
		case <-ctx.Done():
			w.logger.Info().Msg("retention worker stopped")
			return nil
		}
	}
}

// runAll lists every tenant and runs cleanup for each one sequentially.
func (w *RetentionWorker) runAll(ctx context.Context) {
	tenants, err := w.store.ListTenants(ctx)
	if err != nil {
		w.logger.Error().Err(err).Msg("retention: failed to list tenants")
		return
	}

	w.logger.Info().Int("tenants", len(tenants)).Msg("retention run started")

	for _, t := range tenants {
		if ctx.Err() != nil {
			return // context canceled — stop early
		}
		w.runForTenant(ctx, t)
	}

	w.logger.Info().Msg("retention run complete")
}

// runForTenant executes all cleanup tasks for a single tenant.
func (w *RetentionWorker) runForTenant(ctx context.Context, tenant db.Tenant) { //nolint:gocognit,gocyclo // sequential per-table cleanup blocks; complexity is inherent to the retention policy shape
	log := w.logger.With().
		Str("tenant_id", tenant.ID.String()).
		Str("slug", tenant.Slug).
		Logger()

	policy := w.parsePolicy(tenant.RetentionConfig, log)
	now := time.Now().UTC()

	log.Debug().
		Int("error_traces_days", policy.ErrorTracesDays).
		Int("profiler_recordings_days", policy.ProfilerRecordingsDays).
		Int("budget_samples_days", policy.BudgetSamplesDays).
		Int("schema_snapshots_keep", policy.SchemaSnapshotsKeep).
		Int("raw_logs_days", policy.RawLogsDays).
		Int("audit_logs_days", policy.AuditLogsDays).
		Msg("retention policy applied")

	// ── Error groups (S3 trace cleanup first, then DB rows) ─────────────────
	if policy.ErrorTracesDays > 0 {
		cutoff := now.AddDate(0, 0, -policy.ErrorTracesDays)
		w.deleteErrorGroups(ctx, tenant.ID, cutoff, log)
	}

	// ── Profiler recordings ──────────────────────────────────────────────────
	if policy.ProfilerRecordingsDays > 0 {
		cutoff := now.AddDate(0, 0, -policy.ProfilerRecordingsDays)
		tag, err := w.store.DeleteOldProfilerRecordings(ctx, db.DeleteOldProfilerRecordingsParams{
			TenantID:   tenant.ID,
			RecordedAt: cutoff,
		})
		if err != nil {
			log.Error().Err(err).Msg("retention: failed to delete profiler recordings")
		} else {
			log.Info().Int64("deleted", tag.RowsAffected()).Msg("deleted old profiler recordings")
		}
	}

	// ── Budget samples ───────────────────────────────────────────────────────
	if policy.BudgetSamplesDays > 0 {
		cutoff := now.AddDate(0, 0, -policy.BudgetSamplesDays)
		tag, err := w.store.DeleteOldBudgetSamples(ctx, db.DeleteOldBudgetSamplesParams{
			TenantID:  tenant.ID,
			SampledAt: cutoff,
		})
		if err != nil {
			log.Error().Err(err).Msg("retention: failed to delete budget samples")
		} else {
			log.Info().Int64("deleted", tag.RowsAffected()).Msg("deleted old budget samples")
		}
	}

	// ── ORM stats (raw_logs_days window) ─────────────────────────────────────
	if policy.RawLogsDays > 0 {
		cutoff := now.AddDate(0, 0, -policy.RawLogsDays)
		tag, err := w.store.DeleteOldORMStats(ctx, db.DeleteOldORMStatsParams{
			TenantID: tenant.ID,
			Period:   cutoff,
		})
		if err != nil {
			log.Error().Err(err).Msg("retention: failed to delete ORM stats")
		} else {
			log.Info().Int64("deleted", tag.RowsAffected()).Msg("deleted old ORM stats")
		}
	}

	// ── Alerts (same window as error traces) ─────────────────────────────────
	if policy.ErrorTracesDays > 0 {
		cutoff := now.AddDate(0, 0, -policy.ErrorTracesDays)
		tag, err := w.store.DeleteOldAlerts(ctx, db.DeleteOldAlertsParams{
			TenantID:  tenant.ID,
			CreatedAt: cutoff,
		})
		if err != nil {
			log.Error().Err(err).Msg("retention: failed to delete alerts")
		} else {
			log.Info().Int64("deleted", tag.RowsAffected()).Msg("deleted old alerts")
		}
	}

	// ── Schema snapshots (keep N most recent per env) ────────────────────────
	if policy.SchemaSnapshotsKeep > 0 {
		tag, err := w.store.DeleteOldSchemaSnapshots(ctx, db.DeleteOldSchemaSnapshotsParams{
			TenantID: tenant.ID,
			Limit:    int32(policy.SchemaSnapshotsKeep), //nolint:gosec // bounded config value
		})
		if err != nil {
			log.Error().Err(err).Msg("retention: failed to delete schema snapshots")
		} else {
			log.Info().Int64("deleted", tag.RowsAffected()).Msg("deleted old schema snapshots")
		}
	}

	// ── Audit logs ───────────────────────────────────────────────────────────
	if policy.AuditLogsDays > 0 {
		cutoff := now.AddDate(0, 0, -policy.AuditLogsDays)
		tag, err := w.store.DeleteOldAuditLogs(ctx, db.DeleteOldAuditLogsParams{
			TenantID:  tenant.ID,
			CreatedAt: cutoff,
		})
		if err != nil {
			log.Error().Err(err).Msg("retention: failed to delete audit logs")
		} else {
			log.Info().Int64("deleted", tag.RowsAffected()).Msg("deleted old audit logs")
		}
	}

	// ── Heartbeats (fixed 7-day window, not configurable) ────────────────────
	tag, err := w.store.DeleteOldHeartbeats(ctx, db.DeleteOldHeartbeatsParams{
		TenantID:   tenant.ID,
		ReceivedAt: now.AddDate(0, 0, -7),
	})
	if err != nil {
		log.Error().Err(err).Msg("retention: failed to delete heartbeats")
	} else if tag.RowsAffected() > 0 {
		log.Info().Int64("deleted", tag.RowsAffected()).Msg("deleted old heartbeats")
	}
}

// deleteErrorGroups fetches S3 trace refs for expiring error groups, removes
// them from S3, then deletes the DB records.
func (w *RetentionWorker) deleteErrorGroups(ctx context.Context, tenantID uuid.UUID, cutoff time.Time, log zerolog.Logger) {
	// Collect S3 refs before the DB rows are gone.
	if w.s3 != nil {
		refs, err := w.store.ListExpiringErrorGroupRefs(ctx, db.ListExpiringErrorGroupRefsParams{
			TenantID: tenantID,
			LastSeen: cutoff,
		})
		if err != nil {
			log.Error().Err(err).Msg("retention: failed to list expiring error group S3 refs")
			// Proceed with DB deletion — S3 lifecycle policies will clean up eventually.
		} else {
			deleted, failed := 0, 0
			for _, refPtr := range refs {
				if refPtr == nil {
					continue
				}
				if err := w.s3.Delete(ctx, *refPtr); err != nil {
					log.Warn().Err(err).Str("key", *refPtr).Msg("retention: failed to delete S3 trace")
					failed++
				} else {
					deleted++
				}
			}
			if len(refs) > 0 {
				log.Info().
					Int("deleted", deleted).
					Int("failed", failed).
					Msg("deleted S3 error traces")
			}
		}
	}

	// Delete DB rows.
	tag, err := w.store.DeleteOldErrorGroups(ctx, db.DeleteOldErrorGroupsParams{
		TenantID: tenantID,
		LastSeen: cutoff,
	})
	if err != nil {
		log.Error().Err(err).Msg("retention: failed to delete error groups")
	} else {
		log.Info().Int64("deleted", tag.RowsAffected()).Msg("deleted old error groups")
	}
}

// parsePolicy unmarshals retention_config JSONB and fills in default values
// for any field that is missing or zero.
func (w *RetentionWorker) parsePolicy(raw json.RawMessage, log zerolog.Logger) tenantRetentionPolicy {
	p := defaultPolicy
	if len(raw) == 0 {
		return p
	}
	if err := json.Unmarshal(raw, &p); err != nil {
		log.Warn().Err(err).Msg("retention: failed to parse retention_config, using defaults")
		return defaultPolicy
	}
	// Back-fill any zero fields with defaults so a partial config still works.
	if p.ErrorTracesDays == 0 {
		p.ErrorTracesDays = defaultPolicy.ErrorTracesDays
	}
	if p.ProfilerRecordingsDays == 0 {
		p.ProfilerRecordingsDays = defaultPolicy.ProfilerRecordingsDays
	}
	if p.BudgetSamplesDays == 0 {
		p.BudgetSamplesDays = defaultPolicy.BudgetSamplesDays
	}
	if p.SchemaSnapshotsKeep == 0 {
		p.SchemaSnapshotsKeep = defaultPolicy.SchemaSnapshotsKeep
	}
	if p.RawLogsDays == 0 {
		p.RawLogsDays = defaultPolicy.RawLogsDays
	}
	if p.AuditLogsDays == 0 {
		p.AuditLogsDays = defaultPolicy.AuditLogsDays
	}
	return p
}
