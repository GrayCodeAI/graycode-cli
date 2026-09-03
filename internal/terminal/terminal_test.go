package terminal

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/GrayCodeAI/graycode-cli/internal/sandbox"
)

func TestTerminal_LifecycleAndRead(t *testing.T) {
	store := NewStore()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	// Spawn echo / interactive shell
	term, err := store.Create(ctx, "session-1", "", "echo hello_graycode", 24, 80, sandbox.SandboxConfig{})
	if err != nil {
		t.Fatalf("Create terminal failed: %v", err)
	}
	defer func() { _ = term.Kill() }()

	if !strings.HasPrefix(term.ID, "terminal-") {
		t.Errorf("expected branded terminal ID (terminal-<N>), got %s", term.ID)
	}

	// Read output with polling
	var out string
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		chunk, _, rErr := term.Read(1024, 200*time.Millisecond)
		if rErr != nil {
			t.Fatalf("Read failed: %v", rErr)
		}
		out += chunk
		if strings.Contains(out, "hello_graycode") {
			break
		}
	}
	if !strings.Contains(out, "hello_graycode") {
		t.Errorf("expected output to contain hello_graycode, got %q", out)
	}
}

func TestTerminal_OwnershipEnforcement(t *testing.T) {
	store := NewStore()
	ctx := context.Background()

	term, err := store.Create(ctx, "session-alpha", "", "echo isolation", 24, 80, sandbox.SandboxConfig{})
	if err != nil {
		t.Fatalf("Create failed: %v", err)
	}
	defer func() { _ = term.Kill() }()

	// Access with authorized session
	got, err := store.Get("session-alpha", term.ID)
	if err != nil || got == nil {
		t.Fatalf("expected session-alpha to access terminal, got err: %v", err)
	}

	// Access with unauthorized session must fail
	_, err = store.Get("session-beta", term.ID)
	if !errors.Is(err, ErrUnauthorizedSession) {
		t.Fatalf("expected ErrUnauthorizedSession for session-beta, got %v", err)
	}

	// Delete from unauthorized session must fail
	err = store.Delete("session-beta", term.ID)
	if !errors.Is(err, ErrUnauthorizedSession) {
		t.Fatalf("expected ErrUnauthorizedSession on Delete, got %v", err)
	}

	// Delete from authorized session succeeds
	err = store.Delete("session-alpha", term.ID)
	if err != nil {
		t.Fatalf("expected Delete to succeed for session-alpha, got %v", err)
	}
}

func TestTerminal_ListAndCloseSession(t *testing.T) {
	store := NewStore()
	ctx := context.Background()

	t1, err := store.Create(ctx, "sess-x", "", "cat", 24, 80, sandbox.SandboxConfig{})
	if err != nil {
		t.Fatalf("Create t1 failed: %v", err)
	}
	defer func() { _ = t1.Kill() }()

	t2, err := store.Create(ctx, "sess-x", "", "cat", 24, 80, sandbox.SandboxConfig{})
	if err != nil {
		t.Fatalf("Create t2 failed: %v", err)
	}
	defer func() { _ = t2.Kill() }()

	t3, err := store.Create(ctx, "sess-y", "", "cat", 24, 80, sandbox.SandboxConfig{})
	if err != nil {
		t.Fatalf("Create t3 failed: %v", err)
	}
	defer func() { _ = t3.Kill() }()

	listX := store.List("sess-x")
	if len(listX) != 2 {
		t.Errorf("expected 2 terminals for sess-x, got %d", len(listX))
	}

	listY := store.List("sess-y")
	if len(listY) != 1 {
		t.Errorf("expected 1 terminal for sess-y, got %d", len(listY))
	}

	// Close session x disposes both t1 and t2
	store.CloseSession("sess-x")

	if len(store.List("sess-x")) != 0 {
		t.Errorf("expected 0 terminals for sess-x after CloseSession, got %d", len(store.List("sess-x")))
	}

	// sess-y is untouched
	if len(store.List("sess-y")) != 1 {
		t.Errorf("expected 1 terminal for sess-y to remain, got %d", len(store.List("sess-y")))
	}
}

func TestTerminal_ResizeAndSend(t *testing.T) {
	store := NewStore()
	ctx := context.Background()

	term, err := store.Create(ctx, "session-cmd", "", "cat", 24, 80, sandbox.SandboxConfig{})
	if err != nil {
		t.Fatalf("Create failed: %v", err)
	}
	defer func() { _ = term.Kill() }()

	if err := term.Resize(40, 120); err != nil {
		t.Fatalf("Resize failed: %v", err)
	}

	if err := term.Send("ping_term", true); err != nil {
		t.Fatalf("Send failed: %v", err)
	}

	out, _, err := term.Read(1024, 2*time.Second)
	if err != nil {
		t.Fatalf("Read failed: %v", err)
	}
	if !strings.Contains(out, "ping_term") {
		t.Errorf("expected echoed input ping_term, got %q", out)
	}
}
