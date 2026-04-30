package config

import (
	"strings"
	"testing"
)

func TestValidateSettingsValid(t *testing.T) {
	s := Settings{
		Provider:     "anthropic",
		Model:        "claude-sonnet-4-20250514",
		APIKey:       "sk-ant-test123456789",
		MaxBudgetUSD: 10.0,
	}
	result := ValidateSettings(s)
	if !result.Valid {
		t.Fatalf("expected valid, got errors: %v", result.Errors)
	}
}

func TestValidateSettingsInvalidProvider(t *testing.T) {
	s := Settings{Provider: "invalid"}
	result := ValidateSettings(s)
	if result.Valid {
		t.Fatal("expected invalid")
	}
	if len(result.Errors) != 1 || result.Errors[0].Field != "provider" {
		t.Fatalf("unexpected errors: %v", result.Errors)
	}
}

func TestValidateSettingsNegativeBudget(t *testing.T) {
	s := Settings{MaxBudgetUSD: -1}
	result := ValidateSettings(s)
	if result.Valid {
		t.Fatal("expected invalid")
	}
	found := false
	for _, e := range result.Errors {
		if e.Field == "maxBudgetUSD" {
			found = true
		}
	}
	if !found {
		t.Fatalf("expected maxBudgetUSD error, got: %v", result.Errors)
	}
}

func TestValidateSettingsModelWithSpaces(t *testing.T) {
	s := Settings{Model: "gpt 4"}
	result := ValidateSettings(s)
	if result.Valid {
		t.Fatal("expected invalid")
	}
}

func TestValidationErrorString(t *testing.T) {
	e := ValidationError{Field: "test", Message: "error", Value: "bad"}
	if !strings.Contains(e.Error(), "test") || !strings.Contains(e.Error(), "bad") {
		t.Fatalf("unexpected error string: %q", e.Error())
	}

	e2 := ValidationError{Field: "test", Message: "error"}
	if strings.Contains(e2.Error(), "got:") {
		t.Fatal("expected no value in error string")
	}
}

func TestValidationResultError(t *testing.T) {
	r := ValidationResult{Valid: true}
	if r.Error() != "" {
		t.Fatal("expected empty error for valid result")
	}

	r = ValidationResult{
		Valid:  false,
		Errors: []ValidationError{{Field: "a", Message: "err1"}, {Field: "b", Message: "err2"}},
	}
	if !strings.Contains(r.Error(), "err1") || !strings.Contains(r.Error(), "err2") {
		t.Fatalf("unexpected error string: %q", r.Error())
	}
}
