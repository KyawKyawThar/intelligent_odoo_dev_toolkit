package db

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/google/uuid"
)

func stringPtr(s string) *string {
	return &s
}

// optionalStringPtr returns nil if s is empty, otherwise &s.
// Use for optional text fields like FullName.
func optionalStringPtr(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}

type RegisterTenantParams struct {
	TenantName   string
	Slug         string
	Plan         string
	OwnerEmail   string
	PasswordHash string
	FullName     string // optional — empty string → NULL in DB
}

type RegisterTenantResult struct {
	Tenant       Tenant
	User         User
	Subscription Subscription
}

// defaultRetentionConfig returns the default retention policy for a given plan.
func defaultRetentionConfig(plan string) json.RawMessage {
	configs := map[string]string{
		"cloud": `{
			"error_traces_days": 7,
			"profiler_recordings_days": 7,
			"budget_samples_days": 30,
			"schema_snapshots_keep": 10,
			"raw_logs_days": 3
		}`,
		"onprem": `{
			"error_traces_days": 30,
			"profiler_recordings_days": 30,
			"budget_samples_days": 90,
			"schema_snapshots_keep": 30,
			"raw_logs_days": 14
		}`,
		"enterprise": `{
			"error_traces_days": -1,
			"profiler_recordings_days": -1,
			"budget_samples_days": -1,
			"schema_snapshots_keep": -1,
			"raw_logs_days": 90
		}`,
	}
	if cfg, ok := configs[plan]; ok {
		return json.RawMessage(cfg)
	}
	return json.RawMessage(configs["cloud"]) // safe default
}

// RegisterTenantTx creates a tenant, owner user, and subscription atomically.
// Used in POST /auth/register.
func (store *SQLStore) RegisterTenantTx(ctx context.Context, arg RegisterTenantParams) (RegisterTenantResult, error) {
	var result RegisterTenantResult

	err := store.execTx(ctx, func(q *Queries) error {
		var err error

		// 1. Create tenant
		result.Tenant, err = q.CreateTenant(ctx, CreateTenantParams{
			Name:            arg.TenantName,
			Slug:            arg.Slug,
			Plan:            arg.Plan,
			PlanStatus:      "trialing",
			Settings:        json.RawMessage(`{}`),
			RetentionConfig: defaultRetentionConfig(arg.Plan),
		})
		if err != nil {
			return err
		}

		// 2. Create owner user
		result.User, err = q.CreateUser(ctx, CreateUserParams{
			TenantID:     result.Tenant.ID,
			Email:        arg.OwnerEmail,
			PasswordHash: arg.PasswordHash,
			FullName:     optionalStringPtr(arg.FullName), // nil if empty
			Role:         "owner",
			IsActive:     true,
		})
		if err != nil {
			return err
		}

		// 3. Create subscription (starts as trialing)
		result.Subscription, err = q.CreateSubscription(ctx, CreateSubscriptionParams{
			TenantID: result.Tenant.ID,
			Plan:     arg.Plan,
			Status:   "trialing",
		})
		if err != nil {
			return err
		}

		return nil
	})

	return result, err
}

// DeleteEnvironmentTxParams holds params for atomic environment deletion.
type DeleteEnvironmentTxParams struct {
	EnvID    uuid.UUID
	TenantID uuid.UUID
	UserID   uuid.UUID // for audit log
}

// DeleteEnvironmentTx deletes an environment and logs it to audit atomically.
func (store *SQLStore) DeleteEnvironmentTx(ctx context.Context, arg DeleteEnvironmentTxParams) error {
	return store.execTx(ctx, func(q *Queries) error {
		// 1. Delete environment
		result, err := q.DeleteEnvironment(ctx, DeleteEnvironmentParams{
			ID:       arg.EnvID,
			TenantID: arg.TenantID,
		})
		if err != nil {
			return err
		}
		if result.RowsAffected() == 0 {
			return fmt.Errorf("environment not found or not owned by tenant")
		}

		// 2. Write audit log
		_, err = q.CreateAuditLog(ctx, CreateAuditLogParams{
			TenantID:   arg.TenantID,
			UserID:     &arg.UserID,
			Action:     "env.delete",
			Resource:   stringPtr("environments"),
			ResourceID: stringPtr(arg.EnvID.String()),
			Metadata:   json.RawMessage(`{}`),
		})
		return err
	})
}
