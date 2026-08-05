package engine

import (
	"os"
	"strings"
	"testing"
)

func TestRedactToolResultRedactsKnownSecrets(t *testing.T) {
	s := &Session{life: NewLifecycleService(nil)}
	output := "the api key is sk-test12345678901234567890 and all is well"
	got := s.redactToolResult(output)
	if strings.Contains(got, "sk-test12345678901234567890") {
		t.Fatalf("tool result was not redacted: %q", got)
	}
	if !strings.Contains(got, "[REDACTED:api_key]") {
		t.Fatalf("expected redaction placeholder, got: %q", got)
	}
}

func TestRedactToolResultRedactsEnvSecrets(t *testing.T) {
	t.Setenv("GITHUB_TOKEN", "ghp_redactenv123456789012345678901234567")
	s := &Session{life: NewLifecycleService(nil)}
	got := s.redactToolResult("token=ghp_redactenv123456789012345678901234567")
	if strings.Contains(got, "ghp_redactenv123456789012345678901234567") {
		t.Fatalf("env secret was not redacted: %q", got)
	}
}

func TestRedactToolResultRedactsHomePaths(t *testing.T) {
	home, err := os.UserHomeDir()
	if err != nil || home == "" {
		t.Skip("no home directory available")
	}
	s := &Session{life: NewLifecycleService(nil)}
	got := s.redactToolResult(home + "/secret notes")
	if strings.Contains(got, home) {
		t.Fatalf("home path not collapsed: %q", got)
	}
	if !strings.Contains(got, "~/secret notes") {
		t.Fatalf("expected ~/ collapse, got: %q", got)
	}
}

func TestRedactToolResultNilSafe(t *testing.T) {
	var s *Session
	if got := s.redactToolResult("anything"); got != "anything" {
		t.Fatalf("nil session should pass through, got %q", got)
	}
	zero := &Session{}
	if got := zero.redactToolResult("anything"); got != "anything" {
		t.Fatalf("zero session should pass through, got %q", got)
	}
}
