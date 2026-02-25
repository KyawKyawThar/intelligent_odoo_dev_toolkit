package db

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/google/uuid"
)

type RunAnonymizationTxParams struct {
	ProfileID uuid.UUID
	TenantID  uuid.UUID
	UserID    uuid.UUID
	AuditRef  string // S3 key to audit log
	Status    string // "running" | "completed" | "failed"
}

// RunAnonymizationTx updates the anonymization profile status and creates
// an audit log entry atomically. Called at the start and end of an anon run.
func (store *SQLStore) RunAnonymizationTx(ctx context.Context, arg RunAnonymizationTxParams) error {

	return store.execTx(ctx, func(q *Queries) error {
		// /1. Update profile status
		_, err := q.UpdateAnonProfileStatus(ctx, UpdateAnonProfileStatusParams{
			ID:        arg.ProfileID,
			Status:    arg.Status,
			LastRunBy: &arg.UserID,
			AuditRef:  &arg.AuditRef,
			TenantID:  arg.TenantID,
		})

		if err != nil {
			return fmt.Errorf("update anon profile status: %w", err)
		}
		// 2. Audit log
		metadata, err := json.Marshal(map[string]any{
			"profile_id": arg.ProfileID,
			"status":     arg.Status,
			"audit_ref":  arg.AuditRef,
		})
		if err != nil {
			return fmt.Errorf("marshal audit metadata: %w", err)
		}
		_, err = q.CreateAuditLog(ctx, CreateAuditLogParams{
			TenantID:   arg.TenantID,
			UserID:     &arg.UserID,
			Action:     "anon." + arg.Status,
			Resource:   stringPtr("anon_profiles"),
			ResourceID: stringPtr(arg.ProfileID.String()),
			Metadata:   json.RawMessage(metadata),
		})
		if err != nil {
			return fmt.Errorf("create audit log: %w", err)
		}
		return nil
	})
}
