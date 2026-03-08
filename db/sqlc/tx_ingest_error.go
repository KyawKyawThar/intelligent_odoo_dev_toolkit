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

		// 3. Check for spike alert
		if err := store.checkForSpikeAlert(ctx, q, eg, arg); err != nil {
			return err
		}

		return nil
	})
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
