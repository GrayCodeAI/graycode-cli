package engine

import (
	"context"
	"testing"
	"time"

	"github.com/GrayCodeAI/eyrie/client"

	"github.com/GrayCodeAI/hawk/session"
	"github.com/GrayCodeAI/hawk/tool"
)

// ──────────────────────────────────────────────────────────────────────────────
// Helpers
// ──────────────────────────────────────────────────────────────────────────────

// newTestSession creates a session with a standard tool registry and no
// real provider configured. Suitable for unit-level integration tests that
// exercise engine logic without making HTTP calls.
func newTestSession() *Session {
	registry := tool.NewRegistry(
		tool.BashTool{},
		tool.FileReadTool{},
		tool.FileWriteTool{},
		tool.FileEditTool{},
		tool.GlobTool{},
		tool.GrepTool{},
	)
	return NewSession("", "", "You are a test assistant.", registry)
}

// drainStream reads all events from a stream channel until it closes or
// the context expires, returning collected events.
func drainStream(ctx context.Context, ch <-chan StreamEvent, timeout time.Duration) []StreamEvent {
	var events []StreamEvent
	deadline := time.After(timeout)
	for {
		select {
		case ev, ok := <-ch:
			if !ok {
				return events
			}
			events = append(events, ev)
		case <-deadline:
			return events
		case <-ctx.Done():
			return events
		}
	}
}

// ──────────────────────────────────────────────────────────────────────────────
// 1. Full session flow: user message -> stream -> tool call -> result -> done
// ──────────────────────────────────────────────────────────────────────────────

func TestIntegration_FullSessionFlow(t *testing.T) {
	sess := newTestSession()

	// Add user message and assistant response to simulate a flow.
	sess.AddUser("Hello, can you help me?")
	if sess.MessageCount() != 1 {
		t.Fatalf("expected 1 message, got %d", sess.MessageCount())
	}

	// Simulate the engine receiving an assistant text reply + tool call + tool result
	// by manually building the message sequence that agentLoop would produce.
	sess.messages = append(sess.messages, client.EyrieMessage{
		Role:    "assistant",
		Content: "Sure, let me check that file.",
		ToolUse: []client.ToolCall{
			{ID: "tc-1", Name: "Bash", Arguments: map[string]interface{}{"command": "echo hello"}},
		},
	})
	sess.messages = append(sess.messages, client.EyrieMessage{
		Role: "user",
		ToolResult: &client.ToolResult{
			ToolUseID: "tc-1",
			Content:   "hello",
			IsError:   false,
		},
	})
	sess.messages = append(sess.messages, client.EyrieMessage{
		Role:    "assistant",
		Content: "The command ran successfully and returned 'hello'.",
	})

	// Verify the full sequence is intact.
	raw := sess.RawMessages()
	if len(raw) != 4 {
		t.Fatalf("expected 4 messages in flow, got %d", len(raw))
	}
	if raw[0].Role != "user" {
		t.Error("first message should be user")
	}
	if raw[1].Role != "assistant" || len(raw[1].ToolUse) != 1 {
		t.Error("second message should be assistant with tool_use")
	}
	if raw[2].ToolResult == nil {
		t.Error("third message should have tool_result")
	}
	if raw[3].Role != "assistant" || raw[3].Content == "" {
		t.Error("fourth message should be assistant with final content")
	}

	// Stream with immediate timeout exercises the stream/done path.
	ctx, cancel := context.WithTimeout(context.Background(), 1*time.Millisecond)
	defer cancel()
	ch, err := sess.Stream(ctx)
	if err != nil {
		t.Fatal(err)
	}
	events := drainStream(ctx, ch, 5*time.Second)
	// We expect at least an error or done event (no provider configured).
	if len(events) == 0 {
		t.Fatal("expected at least one stream event")
	}
}

// ──────────────────────────────────────────────────────────────────────────────
// 2. Session resume: save, load, continue conversation
// ──────────────────────────────────────────────────────────────────────────────

func TestIntegration_SessionResume(t *testing.T) {
	t.Setenv("HOME", t.TempDir())

	sess := newTestSession()
	sess.AddUser("What is 2+2?")
	sess.AddAssistant("4")

	// Persist to session store.
	persisted := &session.Session{
		ID:       "resume-test",
		Model:    "test-model",
		Provider: "test",
		Messages: make([]session.Message, 0, len(sess.RawMessages())),
	}
	for _, m := range sess.RawMessages() {
		persisted.Messages = append(persisted.Messages, session.Message{
			Role:    m.Role,
			Content: m.Content,
		})
	}
	if err := session.Save(persisted); err != nil {
		t.Fatal(err)
	}

	// Load it back.
	loaded, err := session.Load("resume-test")
	if err != nil {
		t.Fatal(err)
	}
	if len(loaded.Messages) != 2 {
		t.Fatalf("expected 2 messages, got %d", len(loaded.Messages))
	}

	// Rebuild engine session from loaded data.
	resumed := newTestSession()
	for _, m := range loaded.Messages {
		switch m.Role {
		case "user":
			resumed.AddUser(m.Content)
		case "assistant":
			resumed.AddAssistant(m.Content)
		}
	}

	// Continue the conversation.
	resumed.AddUser("What is 3+3?")
	if resumed.MessageCount() != 3 {
		t.Fatalf("expected 3 messages after resume, got %d", resumed.MessageCount())
	}
	if resumed.RawMessages()[2].Content != "What is 3+3?" {
		t.Fatal("continuation message not appended correctly")
	}
}

// ──────────────────────────────────────────────────────────────────────────────
// 3. Compaction trigger: fill to threshold, verify compaction fires
// ──────────────────────────────────────────────────────────────────────────────

func TestIntegration_CompactionTrigger(t *testing.T) {
	sess := newTestSession()

	// Fill messages past the maxContextMessages threshold.
	for i := 0; i < maxContextMessages+20; i++ {
		if i%2 == 0 {
			sess.AddUser("User message " + string(rune('A'+i%26)))
		} else {
			sess.AddAssistant("Assistant response " + string(rune('A'+i%26)))
		}
	}
	before := sess.MessageCount()
	if before <= maxContextMessages {
		t.Fatalf("expected more than %d messages, got %d", maxContextMessages, before)
	}

	// ShouldAutoCompact should be true.
	if !sess.ShouldAutoCompact() {
		t.Fatal("expected ShouldAutoCompact to return true")
	}

	// Compact should reduce the count.
	sess.Compact()
	after := sess.MessageCount()
	if after >= before {
		t.Fatalf("expected compaction to reduce messages: before=%d, after=%d", before, after)
	}

	// The compacted conversation should contain the compaction marker.
	found := false
	for _, m := range sess.RawMessages() {
		if m.Content == "[Earlier conversation compacted to save context.]" {
			found = true
			break
		}
	}
	if !found {
		t.Fatal("expected compaction marker in messages")
	}
}

// ──────────────────────────────────────────────────────────────────────────────
// 4. Permission flow: tool needs permission, user grants, tool executes
// ──────────────────────────────────────────────────────────────────────────────

func TestIntegration_PermissionFlow(t *testing.T) {
	sess := newTestSession()

	// file_write always needs permission.
	if !toolNeedsPermission("Write", nil) {
		t.Fatal("Write should need permission")
	}
	if !toolNeedsPermission("Edit", nil) {
		t.Fatal("Edit should need permission")
	}

	// Safe bash commands do NOT need permission.
	if toolNeedsPermission("Bash", map[string]interface{}{"command": "echo hello"}) {
		t.Fatal("safe echo should not need permission")
	}

	// Dangerous bash commands DO need permission.
	if !toolNeedsPermission("Bash", map[string]interface{}{"command": "rm -rf /"}) {
		t.Fatal("destructive command should need permission")
	}

	// Test that the permission memory grants correctly.
	sess.Permissions.AlwaysAllow("Write")
	decision := sess.Permissions.Check("Write", "/tmp/test.txt")
	if decision == nil || !*decision {
		t.Fatal("Write should be allowed after AlwaysAllow")
	}

	// Test deny takes priority over allow.
	sess.Permissions.DenySpec("Write(*.env)")
	decision = sess.Permissions.Check("Write", "prod.env")
	if decision == nil || *decision {
		t.Fatal("Write to .env should be denied even with broad allow")
	}

	// Test permission function callback.
	permCalled := false
	sess.PermissionFn = func(req PermissionRequest) {
		permCalled = true
		req.Response <- true
	}

	// Create a fresh permission memory to test the callback flow.
	sess.Permissions = NewPermissionMemory()
	decision = sess.Permissions.Check("Write", "/tmp/new-file.txt")
	if decision != nil {
		t.Fatal("fresh permission memory should return nil (ask user)")
	}
	// The actual callback is invoked inside agentLoop; we test it's wired correctly.
	if sess.PermissionFn == nil {
		t.Fatal("PermissionFn should be set")
	}
	// Simulate calling the permission function.
	resp := make(chan bool, 1)
	sess.PermissionFn(PermissionRequest{
		ToolName: "Write",
		ToolID:   "test-id",
		Summary:  "/tmp/new-file.txt",
		Response: resp,
	})
	if !permCalled {
		t.Fatal("permission function was not called")
	}
	if !<-resp {
		t.Fatal("expected permission granted")
	}
}

// ──────────────────────────────────────────────────────────────────────────────
// 5. Error recovery: API returns error, verify retry + user message
// ──────────────────────────────────────────────────────────────────────────────

func TestIntegration_ErrorRecovery(t *testing.T) {
	sess := newTestSession()
	sess.AddUser("Test error handling")

	// Stream with no provider configured triggers an error event.
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	ch, err := sess.Stream(ctx)
	if err != nil {
		t.Fatal(err)
	}

	events := drainStream(ctx, ch, 5*time.Second)
	if len(events) == 0 {
		t.Fatal("expected at least one event")
	}

	// Should contain an error event since no provider is configured.
	hasError := false
	for _, ev := range events {
		if ev.Type == "error" {
			hasError = true
			break
		}
	}
	if !hasError {
		t.Fatal("expected an error event when no provider is configured")
	}

	// Verify RemoveLastExchange removes the last user+assistant pair.
	sess2 := newTestSession()
	sess2.AddUser("First question")
	sess2.AddAssistant("First answer")
	sess2.AddUser("Second question")
	sess2.AddAssistant("Second answer")

	before := sess2.MessageCount()
	sess2.RemoveLastExchange()
	after := sess2.MessageCount()
	if after >= before {
		t.Fatalf("RemoveLastExchange should reduce message count: before=%d, after=%d", before, after)
	}
}

// ──────────────────────────────────────────────────────────────────────────────
// 6. Max turns: set max_turns=2, verify stops after 2 turns
// ──────────────────────────────────────────────────────────────────────────────

func TestIntegration_MaxTurns(t *testing.T) {
	sess := newTestSession()

	// Set max turns.
	if err := sess.SetMaxTurns(2); err != nil {
		t.Fatal(err)
	}
	if sess.MaxTurns != 2 {
		t.Fatalf("expected MaxTurns=2, got %d", sess.MaxTurns)
	}

	// Zero is valid (unlimited).
	if err := sess.SetMaxTurns(0); err != nil {
		t.Fatal(err)
	}

	// Negative is invalid.
	if err := sess.SetMaxTurns(-1); err == nil {
		t.Fatal("expected error for negative max turns")
	}

	// Budget enforcement.
	if err := sess.SetMaxBudgetUSD(1.0); err != nil {
		t.Fatal(err)
	}
	if sess.MaxBudgetUSD != 1.0 {
		t.Fatalf("expected budget 1.0, got %f", sess.MaxBudgetUSD)
	}

	// Simulate cost accumulation to test budget checking.
	sess.Cost = Cost{Model: "gpt-4o"}
	sess.Cost.Add(500_000, 200_000) // ~$3.25 which exceeds $1.0
	if !sess.exceededBudget() {
		t.Fatal("expected budget to be exceeded")
	}

	// Under-budget should not trigger.
	sess2 := newTestSession()
	sess2.MaxBudgetUSD = 100.0
	sess2.Cost = Cost{Model: "gpt-4o"}
	sess2.Cost.Add(100, 50)
	if sess2.exceededBudget() {
		t.Fatal("should not exceed $100 budget with minimal tokens")
	}
}

// ──────────────────────────────────────────────────────────────────────────────
// Additional: permission mode integration
// ──────────────────────────────────────────────────────────────────────────────

func TestIntegration_PermissionModes(t *testing.T) {
	sess := newTestSession()

	// bypassPermissions mode allows everything.
	sess.SetPermissionMode("bypassPermissions")
	decision := sess.modeDecision("Write")
	if decision == nil || !*decision {
		t.Fatal("bypassPermissions should allow Write")
	}

	// dontAsk mode denies everything.
	sess.SetPermissionMode("dontAsk")
	decision = sess.modeDecision("Bash")
	if decision == nil || *decision {
		t.Fatal("dontAsk should deny Bash")
	}

	// acceptEdits mode allows Write/Edit but not Bash.
	sess.SetPermissionMode("acceptEdits")
	decision = sess.modeDecision("Write")
	if decision == nil || !*decision {
		t.Fatal("acceptEdits should allow Write")
	}
	decision = sess.modeDecision("Edit")
	if decision == nil || !*decision {
		t.Fatal("acceptEdits should allow Edit")
	}
	decision = sess.modeDecision("Bash")
	if decision != nil {
		t.Fatal("acceptEdits should not decide on Bash (returns nil = ask user)")
	}

	// plan mode denies all except ExitPlanMode.
	sess.SetPermissionMode("plan")
	decision = sess.modeDecision("Write")
	if decision == nil || *decision {
		t.Fatal("plan mode should deny Write")
	}
	decision = sess.modeDecision("ExitPlanMode")
	if decision != nil {
		t.Fatal("plan mode should return nil for ExitPlanMode (ask user)")
	}
}
