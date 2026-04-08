package db

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/google/uuid"
)

type IngestErrorBatchParams struct {
	EnvID        uuid.UUID
	TenantID     uuid.UUID
	Signature    string
	ErrorType    string
	Message      string
	Module       *string
	Model        *string
	Timestamp    time.Time // timestamptz
	AffectedUIDs []int32
	RawTraceRef  *string

	// Alert thresholds (checked after upsert)
	SpikeThreshold int // if occurrence_count crosses this, create alert
}

func (store *SQLStore) IngestErrorBatchTx(ctx context.Context, arg IngestErrorBatchParams) error {
	return store.execTx(ctx, func(q *Queries) error {
		affectedUsers := arg.AffectedUIDs
		if affectedUsers == nil {
			affectedUsers = []int32{}
		}

		// 0. Snapshot current status BEFORE the upsert so we can detect transitions.
		// If the group doesn't exist yet, wasClosedStatus is false.
		var wasClosedStatus bool
		if existing, err := q.GetErrorGroupBySignature(ctx, GetErrorGroupBySignatureParams{
			EnvID:     arg.EnvID,
			Signature: arg.Signature,
		}); err == nil {
			wasClosedStatus = existing.Status == "resolved" || existing.Status == "acknowledged"
		}

		// 1. Upsert error group
		eg, err := q.UpsertErrorGroup(ctx, UpsertErrorGroupParams{
			EnvID:         arg.EnvID,
			Signature:     arg.Signature,
			ErrorType:     arg.ErrorType,
			Message:       arg.Message,
			Module:        arg.Module,
			Model:         arg.Model,
			FirstSeen:     arg.Timestamp,
			AffectedUsers: affectedUsers,
			RawTraceRef:   arg.RawTraceRef,
		})
		if err != nil {
			return fmt.Errorf("upsert error group: %w", err)
		}

		// 2. Merge affected user IDs
		if len(arg.AffectedUIDs) > 0 {
			if err := q.AppendAffectedUsers(ctx, AppendAffectedUsersParams{
				ID:      eg.ID,
				UserIds: arg.AffectedUIDs,
			}); err != nil {
				return fmt.Errorf("append affected users: %w", err)
			}
		}

		// 3. Alert if a previously resolved/acknowledged error just re-fired
		if err := store.checkForReopenAlert(ctx, q, eg, arg, wasClosedStatus); err != nil {
			return err
		}

		// 4. Check for spike alert
		if err := store.checkForSpikeAlert(ctx, q, eg, arg); err != nil {
			return err
		}

		return nil
	})
}

// checkForReopenAlert fires a critical alert when the upsert flipped an
// already-resolved or acknowledged error back to open — meaning the fix
// didn't actually work.
func (store *SQLStore) checkForReopenAlert(ctx context.Context, q *Queries, eg ErrorGroup, arg IngestErrorBatchParams, wasClosedStatus bool) error {
	// Only fire when the error was previously resolved/acknowledged and the
	// upsert just flipped it back to open. wasClosedStatus is captured before
	// the upsert so we know the actual transition happened.
	if !wasClosedStatus || eg.Status != "open" {
		return nil
	}

	metadata, err := json.Marshal(map[string]any{
		"signature":        arg.Signature,
		"error_type":       arg.ErrorType,
		"occurrence_count": eg.OccurrenceCount,
		"model":            arg.Model,
		"module":           arg.Module,
	})
	if err != nil {
		return fmt.Errorf("marshal reopen alert metadata: %w", err)
	}

	sigShort := arg.Signature
	if len(sigShort) > 40 {
		sigShort = sigShort[:40]
	}

	_, err = q.CreateAlert(ctx, CreateAlertParams{
		EnvID:    arg.EnvID,
		Type:     "error_reopen",
		Severity: "critical",
		Message:  fmt.Sprintf("Resolved error re-fired: %s (%d total occurrences)", sigShort, eg.OccurrenceCount),
		Metadata: json.RawMessage(metadata),
	})
	if err != nil {
		return fmt.Errorf("create reopen alert: %w", err)
	}

	return nil
}

func (store *SQLStore) checkForSpikeAlert(ctx context.Context, q *Queries, eg ErrorGroup, arg IngestErrorBatchParams) error {
	if arg.SpikeThreshold <= 0 || eg.OccurrenceCount < int32(arg.SpikeThreshold) { //nolint:gosec
		return nil
	}

	// Only alert once per threshold crossing
	preCount := eg.OccurrenceCount - 1
	if preCount >= int32(arg.SpikeThreshold) { //nolint:gosec
		return nil
	}

	metadata, err := json.Marshal(map[string]any{
		"signature":        arg.Signature,
		"error_type":       arg.ErrorType,
		"occurrence_count": eg.OccurrenceCount,
		"model":            arg.Model,
		"module":           arg.Module,
	})
	if err != nil {
		return fmt.Errorf("marshal alert metadata: %w", err)
	}

	sigShort := arg.Signature
	if len(sigShort) > 8 {
		sigShort = sigShort[:8]
	}

	_, err = q.CreateAlert(ctx, CreateAlertParams{
		EnvID:    arg.EnvID,
		Type:     "error_spike",
		Severity: "warning",
		Message:  fmt.Sprintf("%s: %d occurrences of %s", arg.ErrorType, eg.OccurrenceCount, sigShort),
		Metadata: json.RawMessage(metadata),
	})
	if err != nil {
		return fmt.Errorf("create spike alert: %w", err)
	}

	return nil
}
