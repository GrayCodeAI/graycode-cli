package engine

import (
	"os"
	"strings"
	"testing"

	"github.com/GrayCodeAI/graycode-cli/internal/types"
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
		// TODO: set a deterministic HOME in CI instead of skipping.
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

// TestCompleteResultRedactsDisplayEvent verifies the display-path wiring: the
// tool_result stream event carries redacted output when a redactor is wired,
// and unchanged output when it is not.
func TestCompleteResultRedactsDisplayEvent(t *testing.T) {
	secret := "sk-test12345678901234567890"
	output := "the key is " + secret

	t.Run("redactor wired", func(t *testing.T) {
		s := &Session{life: NewLifecycleService(nil)}
		svc := NewToolService(nil)
		svc.WithExecutionDeps(toolExecutionDeps{
			redactOutput: s.redactToolResult,
		})

		ch := make(chan StreamEvent, 1)
		_ = svc.CompleteResult(t.Context(), toolExecResult{tc: types.ToolCall{Name: "Read", ID: "t1"}, output: output}, ch)

		ev := <-ch
		if strings.Contains(ev.Content, secret) {
			t.Fatalf("display event carried raw secret: %q", ev.Content)
		}
		if !strings.Contains(ev.Content, "[REDACTED") {
			t.Fatalf("expected redaction placeholder in display event, got: %q", ev.Content)
		}
	})

	t.Run("no redactor wired", func(t *testing.T) {
		svc := NewToolService(nil)
		ch := make(chan StreamEvent, 1)
		_ = svc.CompleteResult(t.Context(), toolExecResult{tc: types.ToolCall{Name: "Read", ID: "t2"}, output: output}, ch)
		ev := <-ch
		if ev.Content != output {
			t.Fatalf("expected unchanged output without redactor, got: %q", ev.Content)
		}
	})
}
