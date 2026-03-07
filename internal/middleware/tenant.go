package middleware

import (
	db "Intelligent_Dev_ToolKit_Odoo/db/sqlc"
	"Intelligent_Dev_ToolKit_Odoo/internal/api"
	"context"
	"errors"
	"net/http"

	"github.com/google/uuid"
)

// =============================================================================
// Tenant Resolver
// =============================================================================
type TenantInfo struct {
	TenantID   string
	TenantSlug string
	Plan       string
	IsActive   bool
}

// TenantLookupFunc is a function that looks up tenant info from user ID
type TenantLookupFunc func(ctx context.Context, userID string) (*TenantInfo, error)

// Additional context keys for tenant info
const (
	contextKeyTenantPlan contextKey = "tenant_plan"
	contextKeyTenantSlug contextKey = "tenant_slug"
)

// GetTenantPlan retrieves the tenant plan from context
func GetTenantPlan(ctx context.Context) string {
	if plan, ok := ctx.Value(contextKeyTenantPlan).(string); ok {
		return plan
	}
	return ""
}

// GetTenantSlug retrieves the tenant slug from context
func GetTenantSlug(ctx context.Context) string {
	if slug, ok := ctx.Value(contextKeyTenantSlug).(string); ok {
		return slug
	}
	return ""
}

// TenantResolver resolves tenant from the authenticated user
func TenantResolver(lookupFunc TenantLookupFunc) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			// Get user ID from context (set by auth middleware)
			userID := GetUserID(r.Context())
			if userID == "" {
				api.HandleError(w, r, api.ErrUnauthorized("User not authenticated"))
				return
			}

			// Look up tenant
			tenantInfo, err := lookupFunc(r.Context(), userID)
			if err != nil {
				var apiErr *api.APIError
				if errors.As(err, &apiErr) {
					api.HandleError(w, r, apiErr)
					return
				}
				api.HandleError(w, r, api.ErrTenantNotFound())
				return
			}

			// Check if tenant is active
			if !tenantInfo.IsActive {
				api.HandleError(w, r, api.ErrForbidden("Tenant account is suspended"))
				return
			}

			// Set tenant context
			ctx := SetTenantID(r.Context(), tenantInfo.TenantID)
			ctx = context.WithValue(ctx, contextKeyTenantPlan, tenantInfo.Plan)
			ctx = context.WithValue(ctx, contextKeyTenantSlug, tenantInfo.TenantSlug)

			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

// =============================================================================
// Tenant Header Resolver (Alternative)
// =============================================================================

// TenantFromHeader resolves tenant from X-Tenant-ID header
// Use this for internal services or admin endpoints
func TenantFromHeader(validateFunc func(ctx context.Context, tenantID string) (*TenantInfo, error)) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			tenantID := r.Header.Get("X-Tenant-ID")
			if tenantID == "" {
				api.HandleError(w, r, api.ErrBadRequest("Missing X-Tenant-ID header"))
				return
			}

			// Validate tenant exists
			tenantInfo, err := validateFunc(r.Context(), tenantID)
			if err != nil {
				var apiErr *api.APIError
				if errors.As(err, &apiErr) {
					api.HandleError(w, r, apiErr)
					return
				}
				api.HandleError(w, r, api.ErrTenantNotFound())
				return
			}

			if !tenantInfo.IsActive {
				api.HandleError(w, r, api.ErrForbidden("Tenant account is suspended"))
				return
			}

			ctx := SetTenantID(r.Context(), tenantInfo.TenantID)
			ctx = context.WithValue(ctx, contextKeyTenantPlan, tenantInfo.Plan)

			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

// =============================================================================
// Tenant Ownership Verification
// =============================================================================

// ResourceOwnershipFunc checks if a resource belongs to the tenant
type ResourceOwnershipFunc func(ctx context.Context, resourceID, tenantID string) (bool, error)

// RequireTenantOwnership ensures the requested resource belongs to the tenant
func RequireTenantOwnership(getResourceTenantID ResourceOwnershipFunc, resourceParam string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			tenantID := GetTenantID(r.Context())
			if tenantID == "" {
				api.HandleError(w, r, api.ErrUnauthorized("Tenant not resolved"))
				return
			}

			// Get resource ID from URL (implementation depends on your router)
			// This is a placeholder - you'd use chi.URLParam or similar
			resourceID := r.PathValue(resourceParam)
			if resourceID == "" {
				// Try query param
				resourceID = r.URL.Query().Get(resourceParam)
			}

			if resourceID != "" {
				belongs, err := getResourceTenantID(r.Context(), resourceID, tenantID)
				if err != nil {
					api.HandleError(w, r, api.ErrInternal(err))
					return
				}

				if !belongs {
					api.HandleError(w, r, api.ErrTenantMismatch())
					return
				}
			}

			next.ServeHTTP(w, r)
		})
	}
}

// DatabaseTenantLookup returns a TenantLookupFunc that fetches tenant info from the database.
func DatabaseTenantLookup(store db.Store) TenantLookupFunc {
	return func(ctx context.Context, userID string) (*TenantInfo, error) {
		// parse the UUID coming from the auth middleware
		uid, err := uuid.Parse(userID)
		if err != nil {
			return nil, api.NewAPIError(api.ErrCodeValidation, "invalid user id", http.StatusBadRequest)
		}

		// get the user without a tenant filter so we can determine which tenant to
		// load.
		user, err := store.GetUserByIDGlobal(ctx, uid)
		if err != nil {
			return nil, api.FromPgError(err)
		}

		tenant, err := store.GetTenantByID(ctx, user.TenantID)
		if err != nil {
			return nil, api.FromPgError(err)
		}

		isActive := tenant.PlanStatus == "active"

		return &TenantInfo{
			TenantID:   tenant.ID.String(),
			TenantSlug: tenant.Slug,
			Plan:       tenant.Plan,
			IsActive:   isActive,
		}, nil
	}
}
