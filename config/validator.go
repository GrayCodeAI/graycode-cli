// Package validator provides config validation utilities.
package config

import (
	"fmt"
	"strings"
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

	// Validate provider
	validProviders := map[string]bool{
		"anthropic": true, "openai": true, "google": true,
		"ollama": true, "groq": true, "openrouter": true, "grok": true,
	}
	if s.Provider != "" && !validProviders[s.Provider] {
		errors = append(errors, ValidationError{
			Field:   "provider",
			Message: "invalid provider",
			Value:   s.Provider,
		})
	}

	// Validate model
	if s.Model != "" && strings.Contains(s.Model, " ") {
		errors = append(errors, ValidationError{
			Field:   "model",
			Message: "model name cannot contain spaces",
			Value:   s.Model,
		})
	}

	// Validate API key format (basic check)
	if s.APIKey != "" && len(s.APIKey) < 10 {
		errors = append(errors, ValidationError{
			Field:   "apiKey",
			Message: "API key seems too short",
		})
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
