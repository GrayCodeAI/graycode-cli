package cmd

import (
	"errors"
	"strings"
	"testing"
)

func TestFriendlyErrorMessageAppendsProviderHint(t *testing.T) {
	// A provider-origin error should get an errhint next-step appended.
	msg := friendlyErrorMessage(errors.New("provider error: 401 unauthorized"))
	if !strings.Contains(msg, "provider") && !strings.Contains(strings.ToLower(msg), "api key") {
		t.Fatalf("expected provider hint in message, got: %q", msg)
	}
}

func TestFriendlyErrorMessageNoHintForLocalError(t *testing.T) {
	// A local error must not draw an errhint provider hint (hawk's own
	// ExitNotFound enrichment is separate and fine).
	msg := friendlyErrorMessage(errors.New("file not found: x"))
	for _, marker := range []string{"API key rejected", "Rate limited", "Can't reach the provider", "Context window full", "Model unavailable"} {
		if strings.Contains(msg, marker) {
			t.Fatalf("local error drew errhint hint %q: %q", marker, msg)
		}
	}
}
