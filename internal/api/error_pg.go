package api

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"golang.org/x/text/cases"
	"golang.org/x/text/language"
)

const (
	// Class 23 — Integrity Constraint Violation
	PgUniqueViolation     = "23505"
	PgForeignKeyViolation = "23503"
	PgNotNullViolation    = "23502"
	PgCheckViolation      = "23514"
	PgExclusionViolation  = "23P01"

	// Class 22 — Data Exception
	PgStringTruncation  = "22001"
	PgNumericOutOfRange = "22003"
	PgInvalidTextRep    = "22P02"
	PgDivisionByZero    = "22012"
	PgInvalidDatetime   = "22007"

	// Class 40 — Transaction Rollback
	PgDeadlockDetected     = "40P01"
	PgSerializationFailure = "40001"

	// Class 08 — Connection Exception
	PgConnectionDone    = "08003"
	PgConnectionRefused = "08006"
	PgConnectionFailure = "08001"

	// Class 42 — Syntax Error or Access Rule Violation
	PgSyntaxError           = "42601"
	PgUndefinedColumn       = "42703"
	PgUndefinedTable        = "42P01"
	PgInsufficientPrivilege = "42501"

	// Class 53 — Insufficient Resources
	PgDiskFull           = "53100"
	PgOutOfMemory        = "53200"
	PgTooManyConnections = "53300"

	// Class 57 — Operator Intervention
	PgQueryCanceled = "57014"
	PgAdminShutdown = "57P01"
)

var constraintFieldMap = map[string]string{
	// Users table
	"users_email_key":           "email",
	"users_tenant_id_email_key": "email",

	// Tenants table
	"tenants_slug_key": "slug",

	// Environments table
	"environments_tenant_id_slug_key": "slug",
	"environments_tenant_id_name_key": "name",

	// API Keys table
	"api_keys_key_hash_key": "key",

	// Alert Rules table
	"alert_rules_tenant_id_name_key": "name",

	// Alert Channels table
	"alert_channels_tenant_id_name_key": "name",

	// Error Groups table
	"error_groups_env_id_signature_key": "signature",

	// Anonymization Profiles table
	"anon_profiles_tenant_id_name_key": "name",

	// Performance Budgets table
	"perf_budgets_environment_id_name_key": "name",
}
var foreignKeyMap = map[string]string{
	"environments_tenant_id_fkey":      "tenant",
	"users_tenant_id_fkey":             "tenant",
	"api_keys_tenant_id_fkey":          "tenant",
	"api_keys_environment_id_fkey":     "environment",
	"error_groups_env_id_fkey":         "environment",
	"profiler_recordings_env_id_fkey":  "environment",
	"schema_snapshots_env_id_fkey":     "environment",
	"migration_scans_env_id_fkey":      "environment",
	"orm_stats_env_id_fkey":            "environment",
	"alert_deliveries_rule_id_fkey":    "alert_rule",
	"alert_deliveries_channel_id_fkey": "alert_channel",
	"anon_jobs_profile_id_fkey":        "anonymization_profile",
	"anon_jobs_environment_id_fkey":    "environment",
}

// =============================================================================
// PostgreSQL Error Handler
// =============================================================================

var pgErrorCodeMap = map[string]func(*pgconn.PgError) *APIError{
	// --- Integrity Constraint Violations ---
	PgUniqueViolation: func(pgErr *pgconn.PgError) *APIError {
		field := extractFieldFromConstraint(pgErr.ConstraintName)
		return ErrDuplicateResource(extractTableFromConstraint(pgErr.ConstraintName), field).
			WithInternal(pgErr)
	},
	PgForeignKeyViolation: func(pgErr *pgconn.PgError) *APIError {
		resource := extractResourceFromForeignKey(pgErr.ConstraintName)
		return ErrInvalidReference(pgErr.ColumnName, resource).
			WithInternal(pgErr)
	},
	PgNotNullViolation: func(pgErr *pgconn.PgError) *APIError {
		return ErrMissingRequired(pgErr.ColumnName).
			WithInternal(pgErr)
	},
	PgCheckViolation: func(pgErr *pgconn.PgError) *APIError {
		return ErrValidation(fmt.Sprintf("Value violates constraint: %s", pgErr.ConstraintName)).
			WithDetail(pgErr.ColumnName, "violates constraint").
			WithInternal(pgErr)
	},
	PgExclusionViolation: func(pgErr *pgconn.PgError) *APIError {
		return ErrConflict("Operation conflicts with an existing record").
			WithInternal(pgErr)
	},
	// --- Data Exceptions ---
	PgStringTruncation: func(pgErr *pgconn.PgError) *APIError {
		return ErrValidation(fmt.Sprintf("Value too long for field '%s'", pgErr.ColumnName)).
			WithDetail(pgErr.ColumnName, "exceeds maximum length").
			WithInternal(pgErr)
	},
	PgNumericOutOfRange: func(pgErr *pgconn.PgError) *APIError {
		return ErrValidation(fmt.Sprintf("Numeric value out of range for '%s'", pgErr.ColumnName)).
			WithDetail(pgErr.ColumnName, "value out of range").
			WithInternal(pgErr)
	},
	PgInvalidTextRep: func(pgErr *pgconn.PgError) *APIError {
		return ErrValidation("Invalid value format").
			WithInternal(pgErr)
	},
	PgInvalidDatetime: func(pgErr *pgconn.PgError) *APIError {
		return ErrValidation(fmt.Sprintf("Invalid datetime value for '%s'", pgErr.ColumnName)).
			WithDetail(pgErr.ColumnName, "invalid datetime").
			WithInternal(pgErr)
	},
	// --- Transaction Rollback ---
	PgDeadlockDetected: func(pgErr *pgconn.PgError) *APIError {
		return ErrConflict("Request could not be completed due to a conflict. Please retry.").
			WithInternal(pgErr)
	},
	PgSerializationFailure: func(pgErr *pgconn.PgError) *APIError {
		return ErrConflict("Concurrent modification detected. Please retry.").
			WithInternal(pgErr)
	},
	// --- Connection Exceptions ---
	PgConnectionDone: func(pgErr *pgconn.PgError) *APIError {
		return ErrDatabaseUnavailable().WithInternal(pgErr)
	},
	PgConnectionRefused: func(pgErr *pgconn.PgError) *APIError {
		return ErrDatabaseUnavailable().WithInternal(pgErr)
	},
	PgConnectionFailure: func(pgErr *pgconn.PgError) *APIError {
		return ErrDatabaseUnavailable().WithInternal(pgErr)
	},
	// --- Insufficient Resources ---
	PgDiskFull: func(pgErr *pgconn.PgError) *APIError {
		return ErrUnavailable("Database storage full").WithInternal(pgErr)
	},
	PgOutOfMemory: func(pgErr *pgconn.PgError) *APIError {
		return ErrUnavailable("Database out of memory").WithInternal(pgErr)
	},
	PgTooManyConnections: func(pgErr *pgconn.PgError) *APIError {
		return ErrUnavailable("Too many database connections").WithInternal(pgErr)
	},
	// --- Access Errors ---
	PgInsufficientPrivilege: func(pgErr *pgconn.PgError) *APIError {
		return ErrInternal(fmt.Errorf("database permission denied: %w", pgErr))
	},
	PgSyntaxError: func(pgErr *pgconn.PgError) *APIError {
		return ErrInternal(pgErr)
	},
	PgUndefinedColumn: func(pgErr *pgconn.PgError) *APIError {
		return ErrInternal(pgErr)
	},
	PgUndefinedTable: func(pgErr *pgconn.PgError) *APIError {
		return ErrInternal(pgErr)
	},
	// --- Operator Intervention ---
	PgQueryCanceled: func(pgErr *pgconn.PgError) *APIError {
		return ErrTimeout("database query was canceled")
	},
	PgAdminShutdown: func(pgErr *pgconn.PgError) *APIError {
		return ErrDatabaseUnavailable().WithInternal(pgErr)
	},
}

func handlePgError(pgErr *pgconn.PgError) *APIError {
	if handler, ok := pgErrorCodeMap[pgErr.Code]; ok {
		return handler(pgErr)
	}
	return ErrDatabase(pgErr)
}

// FromPgError converts a PostgreSQL error to an API error
func FromPgError(err error) *APIError {
	if err == nil {
		return nil
	}

	// Handle pgx.ErrNoRows
	if errors.Is(err, pgx.ErrNoRows) {
		return ErrNotFound("Record")
	}
	// Handle context errors
	if errors.Is(err, context.DeadlineExceeded) {
		return ErrTimeout("database query")
	}
	if errors.Is(err, context.Canceled) {
		return NewAPIError(ErrCodeInternal, "Request was canceled", 499).WithInternal(err)
	}

	// Handle pgconn.PgError
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) {
		return handlePgError(pgErr)
	}

	// Handle connection errors
	if isConnectionError(err) {
		return ErrDatabaseUnavailable().WithInternal(err)
	}

	// Unknown error
	return ErrDatabase(err)
}

// =============================================================================
// Helper Functions
// =============================================================================

func extractFieldFromConstraint(constraint string) string {
	if field, ok := constraintFieldMap[constraint]; ok {
		return field
	}

	parts := strings.Split(constraint, "_")
	if len(parts) >= 2 {
		if parts[len(parts)-1] == "key" || parts[len(parts)-1] == "unique" {
			return parts[len(parts)-2]
		}
	}

	return "value"
}

func extractTableFromConstraint(constraint string) string {
	parts := strings.Split(constraint, "_")
	if len(parts) > 0 {
		return cases.Title(language.English).String(strings.ReplaceAll(parts[0], "_", " "))
	}
	return "Record"
}

func extractResourceFromForeignKey(constraint string) string {
	if resource, ok := foreignKeyMap[constraint]; ok {
		return resource
	}

	parts := strings.Split(constraint, "_")
	if len(parts) >= 3 && parts[len(parts)-1] == "fkey" {
		column := parts[len(parts)-2]
		return strings.TrimSuffix(column, "_id")
	}

	return "referenced record"
}
func isConnectionError(err error) bool {
	if err == nil {
		return false
	}

	errStr := strings.ToLower(err.Error())
	connectionErrors := []string{
		"connection refused",
		"connection reset",
		"connection closed",
		"no connection",
		"broken pipe",
		"i/o timeout",
		"network is unreachable",
	}

	for _, connErr := range connectionErrors {
		if strings.Contains(errStr, connErr) {
			return true
		}
	}

	return false
}

// =============================================================================
// PostgreSQL Error Checking Utilities
// =============================================================================

func IsPgError(err error, code string) bool {
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) {
		return pgErr.Code == code
	}
	return false
}
func IsUniqueViolation(err error) bool {
	return IsPgError(err, PgUniqueViolation)
}

func IsForeignKeyViolation(err error) bool {
	return IsPgError(err, PgForeignKeyViolation)
}

func IsNotNullViolation(err error) bool {
	return IsPgError(err, PgNotNullViolation)
}

func IsDeadlock(err error) bool {
	return IsPgError(err, PgDeadlockDetected)
}

func IsSerializationFailure(err error) bool {
	return IsPgError(err, PgSerializationFailure)
}

func IsConnectionError(err error) bool {
	return IsPgError(err, PgConnectionDone) ||
		IsPgError(err, PgConnectionRefused) ||
		IsPgError(err, PgConnectionFailure) ||
		isConnectionError(err)
}

func IsRecordNotFound(err error) bool {
	return errors.Is(err, pgx.ErrNoRows)
}

func IsRetryable(err error) bool {
	return IsDeadlock(err) ||
		IsSerializationFailure(err) ||
		IsConnectionError(err)
}

// =============================================================================
// Transaction Helper with Auto-Retry
// =============================================================================

func WithRetry(maxAttempts int, fn func() error) error {
	var lastErr error

	for attempt := 1; attempt <= maxAttempts; attempt++ {
		err := fn()
		if err == nil {
			return nil
		}

		lastErr = err

		if !IsRetryable(err) {
			return FromPgError(err)
		}

		if attempt < maxAttempts {
			continue
		}
	}

	return FromPgError(lastErr)
}
