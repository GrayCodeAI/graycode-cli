package onboarding

import (
	"os"
	"testing"
)

func TestNeedsSetup(t *testing.T) {
	// When no provider is set and no API key env vars exist
	os.Unsetenv("ANTHROPIC_API_KEY")
	os.Unsetenv("OPENAI_API_KEY")
	os.Unsetenv("GOOGLE_API_KEY")

	// This may return true or false depending on settings file state
	// Just make sure it doesn't panic
	_ = NeedsSetup()
}

func TestTealColor(t *testing.T) {
	if teal == "" {
		t.Fatal("expected teal color code")
	}
}

func TestResetColor(t *testing.T) {
	if reset == "" {
		t.Fatal("expected reset color code")
	}
}

func TestBoldColor(t *testing.T) {
	if bold == "" {
		t.Fatal("expected bold color code")
	}
}
