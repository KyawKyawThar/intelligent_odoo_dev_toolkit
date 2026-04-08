package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	db "Intelligent_Dev_ToolKit_Odoo/db/sqlc"
	"Intelligent_Dev_ToolKit_Odoo/internal/api"
	"Intelligent_Dev_ToolKit_Odoo/internal/dto"
	"Intelligent_Dev_ToolKit_Odoo/internal/migration"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

// MigrationService implements MigrationServicer.
type MigrationService struct {
	store db.Store
}

// NewMigrationService creates a MigrationService.
func NewMigrationService(store db.Store) *MigrationService {
	return &MigrationService{store: store}
}

// RunScan performs a deprecation scan against the environment's latest (or
// pinned) schema snapshot and persists the result in migration_scans.
func (s *MigrationService) RunScan(
	ctx context.Context,
	tenantID, envID uuid.UUID,
	userID *uuid.UUID,
	req *dto.RunMigrationScanRequest,
) (*dto.MigrationScanResponse, error) {
	// Verify environment belongs to tenant.
	if _, err := s.store.GetEnvironmentByID(ctx, db.GetEnvironmentByIDParams{
		ID:       envID,
		TenantID: tenantID,
	}); err != nil {
		return nil, api.FromPgError(err)
	}

	// Load the schema snapshot.
	var modelsJSON json.RawMessage
	if req.SnapshotID != nil {
		snap, err := s.store.GetSchemaByID(ctx, db.GetSchemaByIDParams{
			ID:    *req.SnapshotID,
			EnvID: envID,
		})
		if err != nil {
			return nil, api.FromPgError(err)
		}
		modelsJSON = snap.Models
	} else {
		snap, err := s.store.GetLatestSchema(ctx, envID)
		if err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				return nil, api.ErrNotFound("no schema snapshot found for this environment — push a schema first")
			}
			return nil, api.FromPgError(err)
		}
		modelsJSON = snap.Models
	}

	// Run the scanner.
	result, err := migration.Scan(modelsJSON, req.FromVersion, req.ToVersion)
	if err != nil {
		return nil, api.ErrBadRequest(err.Error())
	}

	// Serialize issues to JSONB.
	issuesJSON, err := json.Marshal(result.Issues)
	if err != nil {
		return nil, api.ErrInternal(fmt.Errorf("marshal issues: %w", err))
	}

	// Persist.
	scan, err := s.store.CreateMigrationScan(ctx, db.CreateMigrationScanParams{
		EnvID:         envID,
		TriggeredBy:   userID,
		FromVersion:   req.FromVersion,
		ToVersion:     req.ToVersion,
		Issues:        issuesJSON,
		BreakingCount: int32(result.BreakingCount), //nolint:gosec // counts will not exceed int32 max
		WarningCount:  int32(result.WarningCount),  //nolint:gosec // counts will not exceed int32 max
		MinorCount:    int32(result.MinorCount),    //nolint:gosec // counts will not exceed int32 max
		Status:        "completed",
	})
	if err != nil {
		return nil, api.FromPgError(err)
	}

	return toMigrationScanResponse(scan, result.Issues), nil
}

// GetScan returns one migration scan by ID.
func (s *MigrationService) GetScan(
	ctx context.Context,
	tenantID, envID, scanID uuid.UUID,
) (*dto.MigrationScanResponse, error) {
	if _, err := s.store.GetEnvironmentByID(ctx, db.GetEnvironmentByIDParams{
		ID:       envID,
		TenantID: tenantID,
	}); err != nil {
		return nil, api.FromPgError(err)
	}

	scan, err := s.store.GetMigrationScan(ctx, db.GetMigrationScanParams{
		ID:    scanID,
		EnvID: envID,
	})
	if err != nil {
		return nil, api.FromPgError(err)
	}

	var issues []migration.Issue
	if err := json.Unmarshal(scan.Issues, &issues); err != nil {
		issues = []migration.Issue{}
	}

	return toMigrationScanResponse(scan, issues), nil
}

// GetLatestScan returns the most recent scan for an environment.
func (s *MigrationService) GetLatestScan(
	ctx context.Context,
	tenantID, envID uuid.UUID,
) (*dto.MigrationScanResponse, error) {
	if _, err := s.store.GetEnvironmentByID(ctx, db.GetEnvironmentByIDParams{
		ID:       envID,
		TenantID: tenantID,
	}); err != nil {
		return nil, api.FromPgError(err)
	}

	scan, err := s.store.GetLatestMigrationScan(ctx, envID)
	if err != nil {
		return nil, api.FromPgError(err)
	}

	var issues []migration.Issue
	if err := json.Unmarshal(scan.Issues, &issues); err != nil {
		issues = []migration.Issue{}
	}

	return toMigrationScanResponse(scan, issues), nil
}

// ListScans returns a paginated list of migration scans for an environment.
func (s *MigrationService) ListScans(
	ctx context.Context,
	tenantID, envID uuid.UUID,
	req *dto.ListMigrationScansRequest,
) (*dto.MigrationScanListResponse, error) {
	if _, err := s.store.GetEnvironmentByID(ctx, db.GetEnvironmentByIDParams{
		ID:       envID,
		TenantID: tenantID,
	}); err != nil {
		return nil, api.FromPgError(err)
	}

	limit := req.Limit
	if limit <= 0 {
		limit = 20
	}
	offset := req.Offset
	if offset < 0 {
		offset = 0
	}

	rows, err := s.store.ListMigrationScans(ctx, db.ListMigrationScansParams{
		EnvID:  envID,
		Limit:  limit,
		Offset: offset,
	})
	if err != nil {
		return nil, api.FromPgError(err)
	}

	items := make([]dto.MigrationScanListItem, 0, len(rows))
	for _, r := range rows {
		items = append(items, dto.MigrationScanListItem{
			ID:            r.ID,
			EnvID:         r.EnvID,
			FromVersion:   r.FromVersion,
			ToVersion:     r.ToVersion,
			Status:        r.Status,
			BreakingCount: int(r.BreakingCount),
			WarningCount:  int(r.WarningCount),
			MinorCount:    int(r.MinorCount),
			ScannedAt:     r.ScannedAt,
		})
	}

	return &dto.MigrationScanListResponse{
		Scans:  items,
		Total:  int64(len(items)),
		Limit:  limit,
		Offset: offset,
	}, nil
}

// DeleteScan removes a migration scan record.
func (s *MigrationService) DeleteScan(
	ctx context.Context,
	tenantID, envID, scanID uuid.UUID,
) error {
	if _, err := s.store.GetEnvironmentByID(ctx, db.GetEnvironmentByIDParams{
		ID:       envID,
		TenantID: tenantID,
	}); err != nil {
		return api.FromPgError(err)
	}

	return s.store.DeleteMigrationScan(ctx, db.DeleteMigrationScanParams{
		ID:    scanID,
		EnvID: envID,
	})
}

// ScanSourceCode scans uploaded Odoo module source files for deprecated
// Python/XML patterns in the given version transition path.
// The operation is stateless — no database record is created.
func (s *MigrationService) ScanSourceCode(
	_ context.Context,
	_ uuid.UUID,
	req *dto.ScanSourceRequest,
) (*dto.ScanSourceResponse, error) {
	const maxFiles = 200
	const maxFileBytes = 512 * 1024 // 512 KB

	if len(req.Files) > maxFiles {
		return nil, api.ErrBadRequest(fmt.Sprintf("too many files: %d (max %d)", len(req.Files), maxFiles))
	}
	for name, content := range req.Files {
		if len(content) > maxFileBytes {
			return nil, api.ErrBadRequest(fmt.Sprintf("file %q exceeds 512 KB limit", name))
		}
	}

	result, err := migration.ScanSource(req.Files, req.FromVersion, req.ToVersion)
	if err != nil {
		return nil, api.ErrBadRequest(err.Error())
	}

	findings := make([]dto.CodeFinding, 0, len(result.Findings))
	for _, f := range result.Findings {
		findings = append(findings, dto.CodeFinding{
			File:        f.File,
			Line:        f.Line,
			Snippet:     f.Snippet,
			Severity:    f.Severity,
			Kind:        f.Kind,
			Model:       f.Model,
			Field:       f.Field,
			OldName:     f.OldName,
			NewName:     f.NewName,
			Message:     f.Message,
			Fix:         f.Fix,
			FromVersion: f.FromVersion,
			ToVersion:   f.ToVersion,
		})
	}

	total := result.BreakingCount + result.WarningCount + result.MinorCount

	return &dto.ScanSourceResponse{
		FromVersion: req.FromVersion,
		ToVersion:   req.ToVersion,
		Findings:    findings,
		Summary: dto.CodeScanSummary{
			BreakingCount: result.BreakingCount,
			WarningCount:  result.WarningCount,
			MinorCount:    result.MinorCount,
			TotalFindings: total,
			FilesScanned:  result.FilesScanned,
		},
	}, nil
}

// SupportedTransitions returns the version pairs covered by the deprecation DB.
func (s *MigrationService) SupportedTransitions() *dto.SupportedTransitionsResponse {
	pairs := migration.Transitions()
	transitions := make([]dto.VersionTransition, 0, len(pairs))
	for _, p := range pairs {
		rules := migration.ByTransition(p[0], p[1])
		transitions = append(transitions, dto.VersionTransition{
			FromVersion: p[0],
			ToVersion:   p[1],
			RuleCount:   len(rules),
		})
	}
	return &dto.SupportedTransitionsResponse{Transitions: transitions}
}

// ─── Helpers ─────────────────────────────────────────────────────────────────

func toMigrationScanResponse(scan db.MigrationScan, issues []migration.Issue) *dto.MigrationScanResponse {
	dtoIssues := make([]dto.MigrationIssue, 0, len(issues))
	for _, iss := range issues {
		dtoIssues = append(dtoIssues, dto.MigrationIssue{
			Severity:    iss.Severity,
			Kind:        iss.Kind,
			Model:       iss.Model,
			Field:       iss.Field,
			OldName:     iss.OldName,
			NewName:     iss.NewName,
			Message:     iss.Message,
			Fix:         iss.Fix,
			FromVersion: iss.FromVersion,
			ToVersion:   iss.ToVersion,
		})
	}

	total := int(scan.BreakingCount) + int(scan.WarningCount) + int(scan.MinorCount)

	return &dto.MigrationScanResponse{
		ID:          scan.ID,
		EnvID:       scan.EnvID,
		FromVersion: scan.FromVersion,
		ToVersion:   scan.ToVersion,
		Status:      scan.Status,
		Issues:      dtoIssues,
		Summary: dto.MigrationScanSummary{
			BreakingCount: int(scan.BreakingCount),
			WarningCount:  int(scan.WarningCount),
			MinorCount:    int(scan.MinorCount),
			TotalIssues:   total,
			FromVersion:   scan.FromVersion,
			ToVersion:     scan.ToVersion,
		},
		ScannedAt: scan.ScannedAt,
	}
}
