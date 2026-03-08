package api

import (
	"Intelligent_Dev_ToolKit_Odoo/internal/config"
	"errors"
	"fmt"

	"strings"

	"github.com/go-playground/validator/v10"
)

var validationTagMessages = map[string]string{
	// Required
	"required":         "%s is required",
	"required_if":      "%s is required",
	"required_unless":  "%s is required",
	"required_with":    "%s is required when %s is present",
	"required_without": "%s is required when %s is not present",

	// Type/Format
	"email":       "%s must be a valid email address",
	"url":         "%s must be a valid URL",
	"uri":         "%s must be a valid URI",
	"uuid":        "%s must be a valid UUID",
	"uuid4":       "%s must be a valid UUID v4",
	"json":        "%s must be valid JSON",
	"jwt":         "%s must be a valid JWT",
	"base64":      "%s must be valid base64",
	"datetime":    "%s must be a valid datetime",
	"timezone":    "%s must be a valid timezone",
	"e164":        "%s must be a valid E.164 phone number",
	"hexadecimal": "%s must be hexadecimal",

	// String
	"alpha":      "%s must contain only letters",
	"alphanum":   "%s must contain only letters and numbers",
	"ascii":      "%s must contain only ASCII characters",
	"lowercase":  "%s must be lowercase",
	"uppercase":  "%s must be uppercase",
	"contains":   "%s must contain '%s'",
	"excludes":   "%s must not contain '%s'",
	"startswith": "%s must start with '%s'",
	"endswith":   "%s must end with '%s'",

	// Numeric/Length
	"min":     "%s must be at least %s",
	"max":     "%s must not exceed %s",
	"len":     "%s must be exactly %s characters",
	"eq":      "%s must be equal to %s",
	"ne":      "%s must not be equal to %s",
	"gt":      "%s must be greater than %s",
	"gte":     "%s must be greater than or equal to %s",
	"lt":      "%s must be less than %s",
	"lte":     "%s must be less than or equal to %s",
	"number":  "%s must be a valid number",
	"numeric": "%s must be numeric",
	"boolean": "%s must be a boolean",

	// Comparison
	"eqfield":  "%s must be equal to %s",
	"nefield":  "%s must not be equal to %s",
	"gtfield":  "%s must be greater than %s",
	"gtefield": "%s must be greater than or equal to %s",
	"ltfield":  "%s must be less than %s",
	"ltefield": "%s must be less than or equal to %s",

	// Collections
	"oneof":  "%s must be one of: %s",
	"unique": "%s must contain unique values",

	// Network
	"ip":               "%s must be a valid IP address",
	"ipv4":             "%s must be a valid IPv4 address",
	"ipv6":             "%s must be a valid IPv6 address",
	"cidr":             "%s must be a valid CIDR notation",
	"hostname":         "%s must be a valid hostname",
	"hostname_rfc1123": "%s must be a valid RFC1123 hostname",
	"fqdn":             "%s must be a fully qualified domain name",
}

// =============================================================================
// String Utilities
// =============================================================================

func toSnakeCase(s string) string {
	var result strings.Builder
	result.Grow(len(s) + 5)

	for i, r := range s {
		if i > 0 && r >= 'A' && r <= 'Z' {
			result.WriteByte('_')
		}
		result.WriteRune(r)
	}
	return strings.ToLower(result.String())
}

// =============================================================================
// OdooDevTools Custom Validation Tags
// =============================================================================

var customTagMessages = map[string]string{
	"odoo_version":   "%s must be a valid Odoo version (e.g., 14.0, 15.0, 16.0, 17.0, 18.0)",
	"odoo_domain":    "%s must be a valid Odoo domain expression",
	"env_type":       fmt.Sprintf("%%s must be one of: %s, %s, %s", config.EnvironmentDevelopment, config.EnvironmentStaging, config.EnvironmentProduction),
	"alert_operator": "%s must be one of: greater_than, less_than, equal_to, not_equal_to",
	"error_level":    "%s must be one of: debug, info, warning, error, critical",
	"error_status":   "%s must be one of: unresolved, resolved, ignored",
	"anon_strategy":  "%s must be one of: fake_name, fake_email, mask, nullify, randomize, hash",
	"channel_type":   "%s must be one of: slack, email, webhook, pagerduty, discord, teams",
	"user_role":      "%s must be one of: owner, admin, member",
	"plan_type":      "%s must be one of: free, starter, professional, enterprise",
	"slug":           "%s must be a valid slug (lowercase letters, numbers, hyphens only)",
	"api_key_scope":  "%s must be a valid API key scope",
}

// =============================================================================
// Validation Error Conversion
// =============================================================================

// FromValidationError converts validator.ValidationErrors to Error
func FromValidationError(err error) *Error {
	var validationErrors validator.ValidationErrors
	if !errors.As(err, &validationErrors) {
		return ErrValidation("Invalid input").WithInternal(err)
	}

	details := make([]ErrorDetail, 0, len(validationErrors))
	for _, fieldErr := range validationErrors {
		detail := ErrorDetail{
			Field:   toSnakeCase(fieldErr.Field()),
			Message: formatFieldError(fieldErr),
			Code:    fieldErr.Tag(),
		}
		details = append(details, detail)
	}

	return NewError(ErrCodeValidation, "Validation failed", 400).
		WithDetails(details...).
		WithInternal(err)
}
func formatFieldError(fe validator.FieldError) string {
	field := toSnakeCase(fe.Field())
	tag := fe.Tag()
	param := fe.Param()

	// Check custom tags first
	if msg, ok := customTagMessages[tag]; ok {
		if strings.Contains(msg, "%s") && strings.Count(msg, "%s") == 2 {
			return fmt.Sprintf(msg, field, param)
		}
		return fmt.Sprintf(msg, field)
	}

	// Check standard tags
	if msg, ok := validationTagMessages[tag]; ok {
		switch tag {
		case "min", "max", "len", "gt", "gte", "lt", "lte", "eq", "ne":
			return fmt.Sprintf(msg, field, param)
		case "oneof":
			values := strings.ReplaceAll(param, " ", ", ")
			return fmt.Sprintf(msg, field, values)
		case "eqfield", "nefield", "gtfield", "gtefield", "ltfield", "ltefield":
			return fmt.Sprintf(msg, field, toSnakeCase(param))
		case "required_with", "required_without":
			return fmt.Sprintf(msg, field, toSnakeCase(param))
		case "contains", "excludes", "startswith", "endswith":
			return fmt.Sprintf(msg, field, param)
		default:
			if strings.Contains(msg, "%s") {
				return fmt.Sprintf(msg, field)
			}
			return msg
		}
	}

	// Fallback for unknown tags
	if param != "" {
		return fmt.Sprintf("%s failed validation: %s=%s", field, tag, param)
	}
	return fmt.Sprintf("%s failed validation: %s", field, tag)
}

// =============================================================================
// Validation Helper Functions
// =============================================================================

// ValidateStruct validates a struct and returns an Error if validation fails
func ValidateStruct(v *validator.Validate, s any) *Error {
	if err := v.Struct(s); err != nil {
		return FromValidationError(err)
	}
	return nil
}

func formatValidationTag(tag string) string {
	if msg, ok := validationTagMessages[tag]; ok {
		msg = strings.ReplaceAll(msg, "%s", "value")
		return msg
	}
	return fmt.Sprintf("must satisfy: %s", tag)
}

// ValidateVar validates a single variable and returns an Error if validation fails
func ValidateVar(v *validator.Validate, field any, tag, fieldName string) *Error {
	if err := v.Var(field, tag); err != nil {
		return ErrValidation(fmt.Sprintf("Invalid %s", fieldName)).
			WithDetail(fieldName, formatValidationTag(tag)).
			WithInternal(err)
	}
	return nil
}
