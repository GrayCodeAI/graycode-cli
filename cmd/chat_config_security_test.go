package cmd

import (
	"strings"
	"testing"
)

func TestSanitizeConfigNotice_RedactsAPIKey(t *testing.T) {
	got := sanitizeConfigNotice("invalid key sk-test123456789012345678901234567890")
	if strings.Contains(got, "sk-test") {
		t.Fatalf("expected redacted notice, got %q", got)
	}
	if !strings.Contains(got, "REDACTED") {
		t.Fatalf("expected REDACTED placeholder, got %q", got)
	}
}

func TestRenderConfigNotice_ErrorTone(t *testing.T) {
	got := renderConfigNotice("Request failed: rate limit")
	if got == "" {
		t.Fatal("expected styled notice")
	}
}
