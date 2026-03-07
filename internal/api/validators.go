package api

import (
	"Intelligent_Dev_ToolKit_Odoo/internal/config"
	"fmt"
	"net/url"
	"reflect"
	"slices"
	"strings"

	"github.com/go-playground/validator/v10"
)

// =============================================================================
// Validation Registration
// =============================================================================

// RegisterCustomValidations registers all custom validators.
func RegisterCustomValidations(v *validator.Validate) error {
	// Use JSON tag names for field names in errors
	v.RegisterTagNameFunc(func(fld reflect.StructField) string {
		name := strings.SplitN(fld.Tag.Get("json"), ",", 2)[0]
		if name == "-" {
			return fld.Name
		}
		if name == "" {
			return toSnakeCase(fld.Name)
		}
		return name
	})

	registrations := []func(*validator.Validate) error{
		registerOdooValidators,
		registerGeneralValidators,
		registerAppSpecificValidators,
	}

	for _, register := range registrations {
		if err := register(v); err != nil {
			return err
		}
	}
	return nil
}

// registerOdooValidators registers validators specific to Odoo.
func registerOdooValidators(v *validator.Validate) error {
	if err := v.RegisterValidation("odoo_version", validateOdooVersion); err != nil {
		return err
	}
	// Add other Odoo-specific validators here if any
	return nil
}

// registerGeneralValidators registers generic-purpose validators.
func registerGeneralValidators(v *validator.Validate) error {
	if err := v.RegisterValidation("slug", validateSlug); err != nil {
		return err
	}
	if err := v.RegisterValidation("url", isURL); err != nil {
		return err
	}
	return nil
}

// registerAppSpecificValidators registers validators for application-specific types.
func registerAppSpecificValidators(v *validator.Validate) error {
	validators := map[string]func(fl validator.FieldLevel) bool{
		"env_type":       validateEnvType,
		"error_level":    validateErrorLevel,
		"error_status":   validateErrorStatus,
		"channel_type":   validateChannelType,
		"user_role":      validateUserRole,
		"anon_strategy":  validateAnonStrategy,
		"alert_operator": validateAlertOperator,
	}

	for name, fn := range validators {
		if err := v.RegisterValidation(name, fn); err != nil {
			return fmt.Errorf("failed to register '%s' validator: %w", name, err)
		}
	}
	return nil
}

// =============================================================================
// Validation Functions
// =============================================================================

func validateOdooVersion(fl validator.FieldLevel) bool {
	version := fl.Field().String()
	validVersions := []string{"14.0", "15.0", "16.0", "17.0", "18.0"}
	return slices.Contains(validVersions, version)
}

func validateSlug(fl validator.FieldLevel) bool {
	slug := fl.Field().String()
	if slug == "" {
		return true // Allow empty slugs, use 'required' tag if needed
	}
	if strings.HasPrefix(slug, "-") || strings.HasSuffix(slug, "-") {
		return false
	}
	if strings.Contains(slug, "--") {
		return false
	}
	for _, r := range slug {
		if (r < 'a' || r > 'z') && (r < '0' || r > '9') && r != '-' {
			return false
		}
	}
	return true
}

func isURL(fl validator.FieldLevel) bool {
	u, err := url.Parse(fl.Field().String())
	return err == nil && u.Scheme != "" && u.Host != ""
}

func validateEnvType(fl validator.FieldLevel) bool {
	envType := fl.Field().String()
	validTypes := []string{config.EnvironmentDevelopment, config.EnvironmentStaging, config.EnvironmentProduction}
	return slices.Contains(validTypes, envType)
}

func validateErrorLevel(fl validator.FieldLevel) bool {
	level := fl.Field().String()
	validLevels := []string{"debug", "info", "warning", "error", "critical"}
	return slices.Contains(validLevels, level)
}

func validateErrorStatus(fl validator.FieldLevel) bool {
	status := fl.Field().String()
	validStatuses := []string{"unresolved", "resolved", "ignored"}
	return slices.Contains(validStatuses, status)
}

func validateChannelType(fl validator.FieldLevel) bool {
	channelType := fl.Field().String()
	validTypes := []string{"slack", "email", "webhook", "pagerduty", "discord", "teams"}
	return slices.Contains(validTypes, channelType)
}

func validateUserRole(fl validator.FieldLevel) bool {
	role := fl.Field().String()
	validRoles := []string{"owner", "admin", "member"}
	return slices.Contains(validRoles, role)
}

func validateAnonStrategy(fl validator.FieldLevel) bool {
	strategy := fl.Field().String()
	validStrategies := []string{"fake_name", "fake_email", "mask", "nullify", "randomize", "hash", "keep"}
	return slices.Contains(validStrategies, strategy)
}

func validateAlertOperator(fl validator.FieldLevel) bool {
	op := fl.Field().String()
	validOps := []string{"greater_than", "less_than", "equal_to", "not_equal_to", "greater_than_or_equal", "less_than_or_equal"}
	return slices.Contains(validOps, op)
}
