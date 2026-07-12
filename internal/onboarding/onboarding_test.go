package onboarding

import (
	"testing"
)

func TestNeedsSetup_AlwaysFalseForTUI(t *testing.T) {
	if NeedsSetup() {
		t.Error("NeedsSetup() should be false; use /config or hawk setup instead")
	}
}

func TestSelectProvider(t *testing.T) {
	providers := []string{"anthropic", "openai", "ollama"}
	tests := []struct {
		name    string
		input   string
		want    string
		wantErr bool
	}{
		{name: "first", input: "1\n", want: "anthropic"},
		{name: "last", input: "3", want: "ollama"},
		{name: "not a number", input: "openai", wantErr: true},
		{name: "zero", input: "0", wantErr: true},
		{name: "too large", input: "4", wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := selectProvider(providers, tt.input)
			if (err != nil) != tt.wantErr {
				t.Fatalf("selectProvider() error = %v, wantErr %v", err, tt.wantErr)
			}
			if got != tt.want {
				t.Fatalf("selectProvider() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestSetupProviderOptionsComeFromEyrie(t *testing.T) {
	providers := setupProviderOptions()
	if len(providers) == 0 {
		t.Fatal("expected Eyrie setup providers")
	}
	for _, provider := range providers {
		if provider == "" {
			t.Fatal("provider ID must not be empty")
		}
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
