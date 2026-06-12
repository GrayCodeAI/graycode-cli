// Package validator provides config validation utilities.
package config

import (
	"context"
	"fmt"
	"strings"

	"github.com/GrayCodeAI/eyrie/credentials"
)

// ValidationError represents a config validation error.
type ValidationError struct {
	Field   string `json:"field"`
	Message string `json:"message"`
	Value   string `json:"value,omitempty"`
}

func (e ValidationError) Error() string {
	if e.Value != "" {
		return fmt.Sprintf("%s: %s (got: %s)", e.Field, e.Message, e.Value)
	}
	return fmt.Sprintf("%s: %s", e.Field, e.Message)
}

// ValidationResult contains all validation errors.
type ValidationResult struct {
	Errors []ValidationError `json:"errors"`
	Valid  bool              `json:"valid"`
}

// Error returns a formatted error string.
func (r ValidationResult) Error() string {
	if r.Valid {
		return ""
	}
	var parts []string
	for _, e := range r.Errors {
		parts = append(parts, e.Error())
	}
	return strings.Join(parts, "; ")
}

// ValidateSettings validates a Settings object.
func ValidateSettings(s Settings) ValidationResult {
	var errors []ValidationError

	// Provider names are delegated to Eyrie. Do not hardcode/validate here.

	// Validate model selection (stored in eyrie provider.json)
	activeModel := strings.TrimSpace(s.Model)
	if activeModel == "" {
		activeModel = ActiveModel(context.Background())
	}
	if activeModel != "" && strings.Contains(activeModel, " ") {
		errors = append(errors, ValidationError{
			Field:   "model",
			Message: "model name cannot contain spaces",
			Value:   activeModel,
		})
	}

	activeProvider := strings.TrimSpace(s.Provider)
	if activeProvider == "" {
		activeProvider = ActiveProvider(context.Background())
	}
	// Hawk: validate API key is in the OS secret store (not in settings)
	if activeProvider != "" {
		envKey := ProviderAPIKeyEnv(activeProvider)
		if envKey != "" && APIKeyForProvider(activeProvider) == "" {
			errors = append(errors, ValidationError{
				Field:   "apiKey",
				Message: fmt.Sprintf("save your %s API key with /config (%s)", activeProvider, credentials.PlatformSecretStoreName()),
			})
		}
	}

	// Validate max budget
	if s.MaxBudgetUSD < 0 {
		errors = append(errors, ValidationError{
			Field:   "maxBudgetUSD",
			Message: "cannot be negative",
			Value:   fmt.Sprintf("%f", s.MaxBudgetUSD),
		})
	}

	return ValidationResult{
		Errors: errors,
		Valid:  len(errors) == 0,
	}
}
