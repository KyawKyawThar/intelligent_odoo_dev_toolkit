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
		// 1. Upsert error group
		eg, err := q.UpsertErrorGroup(ctx, UpsertErrorGroupParams{
			EnvID:         arg.EnvID,
			Signature:     arg.Signature,
			ErrorType:     arg.ErrorType,
			Message:       arg.Message,
			Module:        arg.Model,
			Model:         arg.Module,
			FirstSeen:     arg.Timestamp,
			AffectedUsers: arg.AffectedUIDs,
			RawTraceRef:   arg.RawTraceRef,
		})

		if err != nil {
			return fmt.Errorf("upsert error group: %w", err)
		}
		// 2. Merge affected user IDs
		if len(arg.AffectedUIDs) > 0 {
			if err := q.AppendAffectedUsers(ctx, AppendAffectedUsersParams{
				ID:      eg.EnvID,
				UserIds: arg.AffectedUIDs,
			}); err != nil {
				return fmt.Errorf("append affected users: %w", err)
			}
		}
		// 3. Check if we crossed the spike threshold → create alert

		if arg.SpikeThreshold > 0 && eg.OccurrenceCount >= int32(arg.SpikeThreshold) {
			// Only alert once per threshold crossing (check previous count)

			preCount := eg.OccurrenceCount - 1

			if preCount < int32(arg.SpikeThreshold) {

				metadata, _ := json.Marshal(map[string]any{
					"signature":        arg.Signature,
					"error_type":       arg.ErrorType,
					"occurrence_count": eg.OccurrenceCount,
					"model":            arg.Model,
					"module":           arg.Module})

				_, err := q.CreateAlert(ctx, CreateAlertParams{
					EnvID:    arg.EnvID,
					Type:     "error_spike",
					Severity: "warning",
					Message:  fmt.Sprintf("%s: %d occurrences of %s", arg.ErrorType, eg.OccurrenceCount, arg.Signature[:8]),
					Metadata: json.RawMessage(metadata),
				})

				if err != nil {
					return fmt.Errorf("create spike alert: %w", err)

				}
			}

		}

		return nil
	})
}
