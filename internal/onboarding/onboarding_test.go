package onboarding

import (
	"testing"
)

func TestNeedsSetup_AlwaysFalseForTUI(t *testing.T) {
	if NeedsSetup() {
		t.Error("NeedsSetup() should be false; use /config or graycode setup instead")
	}
}

func TestValidateAPIKey(t *testing.T) {
	tests := []struct {
		name     string
		provider string
		key      string
		valid    bool
	}{
		{"valid anthropic", "anthropic", "sk-ant-api01-abcdefghijklmnopqrstuvwxyz", true},
		{"valid openai", "openai", "sk-abcdefghijklmnopqrstuvwxyz123456", true},
		{"too short", "anthropic", "sk-ant", false},
		{"wrong prefix anthropic", "anthropic", "wrong-prefix-long-enough-key", false},
		{"wrong prefix openai", "openai", "not-sk-prefix-long-enough-key", false},
		{"unknown provider accepts any", "gemini", "any-key-long-enough-to-be-valid", true},
		{"empty key", "anthropic", "", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, valid := validateAPIKey(tt.provider, tt.key)
			if valid != tt.valid {
				t.Errorf("validateAPIKey(%q, %q) valid = %v, want %v", tt.provider, tt.key, valid, tt.valid)
			}
		})
	}
}

func TestWelcome(t *testing.T) {
	Welcome("1.0.0")
}

func TestColorConstants(t *testing.T) {
	if teal == "" {
		t.Error("teal color should not be empty")
	}
	if reset == "" {
		t.Error("reset should not be empty")
	}
	if bold == "" {
		t.Error("bold should not be empty")
	}
}
