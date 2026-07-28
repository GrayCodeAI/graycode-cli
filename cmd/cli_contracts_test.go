package cmd

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	hawkconfig "github.com/GrayCodeAI/hawk/internal/config"
	"github.com/GrayCodeAI/hawk/internal/engine"
	"github.com/GrayCodeAI/hawk/internal/session"
)

func TestResumeRecoveredSession_StartsChatFlow(t *testing.T) {
	oldResumeID := resumeID
	oldContinueFlag := continueFlag
	oldEnsure := recoverEnsureCatalogBeforeAgent
	oldRunChat := recoverRunChat
	t.Cleanup(func() {
		resumeID = oldResumeID
		continueFlag = oldContinueFlag
		recoverEnsureCatalogBeforeAgent = oldEnsure
		recoverRunChat = oldRunChat
	})

	var (
		catalogCalled bool
		chatCalled    bool
	)
	recoverEnsureCatalogBeforeAgent = func(ctx context.Context, strict bool) error {
		catalogCalled = true
		if strict {
			t.Fatal("resumeRecoveredSession should use non-strict catalog startup")
		}
		return nil
	}
	recoverRunChat = func() error {
		chatCalled = true
		return nil
	}

	continueFlag = true
	if err := resumeRecoveredSession(context.Background(), "session-123"); err != nil {
		t.Fatalf("resumeRecoveredSession returned error: %v", err)
	}
	if !catalogCalled {
		t.Fatal("expected catalog startup before entering chat")
	}
	if !chatCalled {
		t.Fatal("expected chat flow to start")
	}
	if resumeID != "session-123" {
		t.Fatalf("resumeID = %q, want session-123", resumeID)
	}
	if continueFlag {
		t.Fatal("continueFlag should be cleared when resuming a specific recovered session")
	}
}

func TestPrepareSession_ResumeUsesRecoveryPath(t *testing.T) {
	t.Setenv("HAWK_STATE_DIR", t.TempDir())

	oldResumeID := resumeID
	oldContinueFlag := continueFlag
	oldSessionIDFlag := sessionIDFlag
	oldForkSessionFlag := forkSessionFlag
	t.Cleanup(func() {
		resumeID = oldResumeID
		continueFlag = oldContinueFlag
		sessionIDFlag = oldSessionIDFlag
		forkSessionFlag = oldForkSessionFlag
	})

	saved := &session.Session{
		ID:       "resume-me",
		Model:    "test-model",
		Provider: "test-provider",
		Messages: []session.Message{
			{Role: "user", Content: "hello"},
			{Role: "assistant", Content: "hi"},
		},
		CreatedAt: time.Now().Add(-time.Minute),
		UpdatedAt: time.Now().Add(-time.Second),
	}
	if err := session.Save(saved); err != nil {
		t.Fatalf("Save() error = %v", err)
	}

	wal, err := session.NewWAL(saved.ID)
	if err != nil {
		t.Fatalf("NewWAL() error = %v", err)
	}
	if appendErr := wal.AppendMeta(saved.Model, saved.Provider, saved.CWD); appendErr != nil {
		t.Fatalf("AppendMeta() error = %v", appendErr)
	}
	if closeErr := wal.Close(); closeErr != nil {
		t.Fatalf("Close() error = %v", closeErr)
	}

	resumeID = saved.ID
	continueFlag = false
	sessionIDFlag = ""
	forkSessionFlag = false

	sess := engine.NewSession("test-provider", "test-model", "system", nil)
	id, loaded, err := prepareSession(sess)
	if err != nil {
		t.Fatalf("prepareSession() error = %v", err)
	}
	if id != saved.ID {
		t.Fatalf("session id = %q, want %q", id, saved.ID)
	}
	if loaded == nil {
		t.Fatal("expected saved session to be returned")
	}
	if sess.MessageCount() != len(saved.Messages) {
		t.Fatalf("loaded persisted messages = %d, want %d", sess.MessageCount(), len(saved.Messages))
	}

	walPath := filepath.Join(os.Getenv("HAWK_STATE_DIR"), "sessions", saved.ID+".wal")
	if _, err := os.Stat(walPath); !os.IsNotExist(err) {
		t.Fatalf("expected stale WAL to be removed, stat err = %v", err)
	}
}

func TestPrepareSessionRejectsInvalidSessionID(t *testing.T) {
	oldResumeID := resumeID
	oldContinueFlag := continueFlag
	oldSessionIDFlag := sessionIDFlag
	oldForkSessionFlag := forkSessionFlag
	t.Cleanup(func() {
		resumeID = oldResumeID
		continueFlag = oldContinueFlag
		sessionIDFlag = oldSessionIDFlag
		forkSessionFlag = oldForkSessionFlag
	})

	resumeID = ""
	continueFlag = false
	sessionIDFlag = "../../escape"
	forkSessionFlag = false

	sess := engine.NewSession("test-provider", "test-model", "system", nil)
	if _, _, err := prepareSession(sess); err == nil {
		t.Fatal("prepareSession accepted an unsafe --session-id")
	}
}

func TestReplBuiltinResponse_ToolsAndSession(t *testing.T) {
	sess := engine.NewSession("demo-provider", "demo-model", "system", nil)
	sess.AddUser("hello")

	toolsOut, handled, err := replBuiltinResponse("/tools", sess, hawkconfig.Settings{}, "session-1")
	if err != nil {
		t.Fatalf("/tools error = %v", err)
	}
	if !handled {
		t.Fatal("/tools should be handled as a REPL builtin")
	}
	if !strings.Contains(toolsOut, "Built-in tools") {
		t.Fatalf("/tools output missing tool summary: %q", toolsOut)
	}

	sessionOut, handled, err := replBuiltinResponse("/session", sess, hawkconfig.Settings{}, "session-1")
	if err != nil {
		t.Fatalf("/session error = %v", err)
	}
	if !handled {
		t.Fatal("/session should be handled as a REPL builtin")
	}
	for _, want := range []string{"Session info:", "ID: session-1", "Provider: demo-provider", "Model: demo-model"} {
		if !strings.Contains(sessionOut, want) {
			t.Fatalf("/session output missing %q in %q", want, sessionOut)
		}
	}
}

func TestReplBuiltinResponse_Models(t *testing.T) {
	sess := engine.NewSession("openrouter", "openrouter/auto", "system", nil)
	out, handled, err := replBuiltinResponse("/models", sess, hawkconfig.Settings{}, "session-1")
	if err != nil {
		t.Fatalf("/models error = %v", err)
	}
	if !handled {
		t.Fatal("/models should be handled as a REPL builtin")
	}
	if strings.TrimSpace(out) == "" {
		t.Fatal("/models output should not be empty")
	}
}

func TestPromptInputReadLine_WithoutInteractiveReader(t *testing.T) {
	_, err := (promptInput{}).readLine("prompt")
	if err == nil {
		t.Fatal("expected an error when no interactive prompt input is available")
	}
	if err != errNoInteractivePromptInput {
		t.Fatalf("error = %v, want %v", err, errNoInteractivePromptInput)
	}
}

func TestPluginListSubcommandUsesCobraTree(t *testing.T) {
	buf := new(bytes.Buffer)
	rootCmd.SetOut(buf)
	rootCmd.SetErr(buf)
	rootCmd.SetArgs([]string{"plugin", "list"})

	if err := rootCmd.Execute(); err != nil {
		t.Fatalf("rootCmd.Execute() error = %v", err)
	}
	if strings.TrimSpace(buf.String()) == "" {
		t.Fatal("expected plugin list output")
	}
}

func TestRecoverCommand_ExecutesResumeFlow(t *testing.T) {
	t.Setenv("HAWK_STATE_DIR", t.TempDir())

	oldResumeID := resumeID
	oldContinueFlag := continueFlag
	oldEnsure := recoverEnsureCatalogBeforeAgent
	oldRunChat := recoverRunChat
	t.Cleanup(func() {
		resumeID = oldResumeID
		continueFlag = oldContinueFlag
		recoverEnsureCatalogBeforeAgent = oldEnsure
		recoverRunChat = oldRunChat
	})

	saved := &session.Session{
		ID:        "recover-cobra",
		Model:     "demo-model",
		Provider:  "demo-provider",
		Messages:  []session.Message{{Role: "user", Content: "hello"}},
		CreatedAt: time.Now().Add(-time.Minute),
		UpdatedAt: time.Now(),
	}
	if err := session.Save(saved); err != nil {
		t.Fatalf("Save() error = %v", err)
	}

	var (
		catalogCalled bool
		chatCalled    bool
	)
	recoverEnsureCatalogBeforeAgent = func(ctx context.Context, strict bool) error {
		catalogCalled = true
		if strict {
			t.Fatal("recover command should use non-strict catalog startup")
		}
		return nil
	}
	recoverRunChat = func() error {
		chatCalled = true
		return nil
	}

	buf := new(bytes.Buffer)
	rootCmd.SetOut(buf)
	rootCmd.SetErr(buf)
	rootCmd.SetArgs([]string{"recover", saved.ID})

	if err := rootCmd.Execute(); err != nil {
		t.Fatalf("rootCmd.Execute() error = %v", err)
	}
	if !catalogCalled {
		t.Fatal("expected recover command to run catalog startup before chat")
	}
	if !chatCalled {
		t.Fatal("expected recover command to enter chat flow")
	}
	if resumeID != saved.ID {
		t.Fatalf("resumeID = %q, want %q", resumeID, saved.ID)
	}
	out := buf.String()
	if !strings.Contains(out, "Resuming session "+saved.ID) {
		t.Fatalf("recover output missing resume message: %q", out)
	}
}
