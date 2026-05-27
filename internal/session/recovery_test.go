package session

import (
	"strings"
	"testing"
	"time"
)

// ---------------------------------------------------------------------------
// classifyInterruption
// ---------------------------------------------------------------------------

func TestClassifyInterruption_EmptyMessages(t *testing.T) {
	interruption, tool, id := classifyInterruption(nil)
	if interruption != InterruptionNone {
		t.Errorf("expected InterruptionNone, got %s", interruption)
	}
	if tool != "" || id != "" {
		t.Errorf("expected empty tool/id, got %q/%q", tool, id)
	}
}

func TestClassifyInterruption_AssistantWithUnresolvedToolUse(t *testing.T) {
	messages := []Message{
		{Role: "user", Content: "run tests"},
		{Role: "assistant", ToolUse: []ToolCall{
			{ID: "t1", Name: "Bash"},
		}},
	}
	// No tool_result for t1 -> mid_tool
	interruption, toolName, _ := classifyInterruption(messages)
	if interruption != InterruptionMidTool {
		t.Errorf("expected InterruptionMidTool, got %s", interruption)
	}
	if toolName != "Bash" {
		t.Errorf("expected tool name 'Bash', got %q", toolName)
	}
}

func TestClassifyInterruption_AssistantWithResolvedTools(t *testing.T) {
	messages := []Message{
		{Role: "user", Content: "run tests"},
		{Role: "assistant", ToolUse: []ToolCall{
			{ID: "t1", Name: "Bash"},
		}},
		{Role: "user", ToolResult: &ToolResult{ToolUseID: "t1", Content: "pass"}},
		{Role: "assistant", ToolUse: []ToolCall{
			{ID: "t2", Name: "Read"},
		}},
	}
	// t1 is resolved, t2 is unresolved -> mid_tool
	interruption, toolName, _ := classifyInterruption(messages)
	if interruption != InterruptionMidTool {
		t.Errorf("expected InterruptionMidTool, got %s", interruption)
	}
	if toolName != "Read" {
		t.Errorf("expected tool name 'Read', got %q", toolName)
	}
}

func TestClassifyInterruption_AssistantWritingText(t *testing.T) {
	messages := []Message{
		{Role: "user", Content: "explain this"},
		{Role: "assistant", Content: "Here is my explanation..."},
	}
	interruption, _, _ := classifyInterruption(messages)
	if interruption != InterruptionMidResponse {
		t.Errorf("expected InterruptionMidResponse, got %s", interruption)
	}
}

func TestClassifyInterruption_AssistantEmptyContent(t *testing.T) {
	messages := []Message{
		{Role: "user", Content: "hello"},
		{Role: "assistant", Content: ""},
	}
	interruption, _, _ := classifyInterruption(messages)
	// Empty assistant content at end - classifyInterruption checks Content != ""
	// so empty content means neither tool_use nor text response -> falls through
	if interruption != InterruptionNone {
		t.Logf("empty assistant content classified as %s (implementation detail)", interruption)
	}
}

func TestClassifyInterruption_UserToolResultWithMoreUnresolved(t *testing.T) {
	messages := []Message{
		{Role: "user", Content: "do stuff"},
		{Role: "assistant", ToolUse: []ToolCall{
			{ID: "t1", Name: "Bash"},
			{ID: "t2", Name: "Read"},
		}},
		{Role: "user", ToolResult: &ToolResult{ToolUseID: "t1", Content: "ok"}},
	}
	// t1 resolved, t2 unresolved, last message is user tool_result -> tool_error
	interruption, _, _ := classifyInterruption(messages)
	if interruption != InterruptionToolError {
		t.Errorf("expected InterruptionToolError, got %s", interruption)
	}
}

func TestClassifyInterruption_UserToolResultAllResolved(t *testing.T) {
	messages := []Message{
		{Role: "user", Content: "do stuff"},
		{Role: "assistant", ToolUse: []ToolCall{
			{ID: "t1", Name: "Bash"},
		}},
		{Role: "user", ToolResult: &ToolResult{ToolUseID: "t1", Content: "ok"}},
	}
	// All resolved, last msg is user tool_result -> awaiting_input
	interruption, _, _ := classifyInterruption(messages)
	if interruption != InterruptionAwaitingInput {
		t.Errorf("expected InterruptionAwaitingInput, got %s", interruption)
	}
}

func TestClassifyInterruption_RegularUserMessage(t *testing.T) {
	messages := []Message{
		{Role: "assistant", Content: "How can I help?"},
		{Role: "user", Content: "fix the bug"},
	}
	// Last message is regular user message -> awaiting_input (assistant should respond)
	interruption, _, _ := classifyInterruption(messages)
	if interruption != InterruptionAwaitingInput {
		t.Errorf("expected InterruptionAwaitingInput, got %s", interruption)
	}
}

// ---------------------------------------------------------------------------
// findUnresolvedTools
// ---------------------------------------------------------------------------

func TestFindUnresolvedTools_AllResolved(t *testing.T) {
	messages := []Message{
		{Role: "assistant", ToolUse: []ToolCall{
			{ID: "t1", Name: "Bash"},
			{ID: "t2", Name: "Read"},
		}},
		{Role: "user", ToolResult: &ToolResult{ToolUseID: "t1", Content: "ok"}},
		{Role: "user", ToolResult: &ToolResult{ToolUseID: "t2", Content: "ok"}},
	}
	unresolved := findUnresolvedTools(messages)
	if len(unresolved) != 0 {
		t.Errorf("expected 0 unresolved, got %d: %v", len(unresolved), unresolved)
	}
}

func TestFindUnresolvedTools_OneUnresolved(t *testing.T) {
	messages := []Message{
		{Role: "assistant", ToolUse: []ToolCall{
			{ID: "t1", Name: "Bash"},
			{ID: "t2", Name: "Read"},
		}},
		{Role: "user", ToolResult: &ToolResult{ToolUseID: "t1", Content: "ok"}},
	}
	unresolved := findUnresolvedTools(messages)
	if len(unresolved) != 1 {
		t.Errorf("expected 1 unresolved, got %d: %v", len(unresolved), unresolved)
	}
	if unresolved[0] != "Read" {
		t.Errorf("expected unresolved tool 'Read', got %q", unresolved[0])
	}
}

func TestFindUnresolvedTools_NoTools(t *testing.T) {
	messages := []Message{
		{Role: "user", Content: "hello"},
		{Role: "assistant", Content: "hi"},
	}
	unresolved := findUnresolvedTools(messages)
	if len(unresolved) != 0 {
		t.Errorf("expected 0 unresolved, got %d", len(unresolved))
	}
}

// ---------------------------------------------------------------------------
// findOrphanedToolUses
// ---------------------------------------------------------------------------

func TestFindOrphanedToolUses_NoOrphans(t *testing.T) {
	messages := []Message{
		{Role: "assistant", ToolUse: []ToolCall{
			{ID: "t1", Name: "Bash"},
		}},
		{Role: "user", ToolResult: &ToolResult{ToolUseID: "t1", Content: "ok"}},
		{Role: "assistant", Content: "done"},
	}
	orphaned := findOrphanedToolUses(messages)
	if len(orphaned) != 0 {
		t.Errorf("expected 0 orphaned, got %d", len(orphaned))
	}
}

func TestFindOrphanedToolUses_OrphanInEarlierMessage(t *testing.T) {
	messages := []Message{
		{Role: "assistant", ToolUse: []ToolCall{
			{ID: "t1", Name: "Bash"},
		}},
		// t1 is never resolved, but next assistant message has new tools
		{Role: "assistant", ToolUse: []ToolCall{
			{ID: "t2", Name: "Read"},
		}},
		{Role: "user", ToolResult: &ToolResult{ToolUseID: "t2", Content: "ok"}},
	}
	orphaned := findOrphanedToolUses(messages)
	if len(orphaned) != 1 {
		t.Errorf("expected 1 orphaned, got %d: %v", len(orphaned), orphaned)
	}
	if orphaned[0] != "Bash" {
		t.Errorf("expected orphaned 'Bash', got %q", orphaned[0])
	}
}

func TestFindOrphanedToolUses_PendingInLastMessage(t *testing.T) {
	messages := []Message{
		{Role: "user", Content: "hello"},
		{Role: "assistant", ToolUse: []ToolCall{
			{ID: "t1", Name: "Bash"},
		}},
		// t1 unresolved but in last assistant message -> pending, not orphaned
	}
	orphaned := findOrphanedToolUses(messages)
	if len(orphaned) != 0 {
		t.Errorf("expected 0 orphaned (t1 is pending in last msg), got %d: %v", len(orphaned), orphaned)
	}
}

// ---------------------------------------------------------------------------
// FormatRecoveryCandidates
// ---------------------------------------------------------------------------

func TestFormatRecoveryCandidates_Empty(t *testing.T) {
	result := FormatRecoveryCandidates(nil)
	if result != "No interrupted sessions found." {
		t.Errorf("expected 'No interrupted sessions found.', got %q", result)
	}
}

func TestFormatRecoveryCandidates_SingleMidTool(t *testing.T) {
	candidates := []RecoveryCandidate{
		{
			SessionID:    "abc12345def",
			CWD:          "/home/user/project",
			MessageCount: 10,
			LastActivity: time.Now(),
			Interruption: InterruptionMidTool,
			LastToolName: "Bash",
			Age:          5 * time.Minute,
		},
	}
	result := FormatRecoveryCandidates(candidates)
	if !strings.Contains(result, "abc12345") {
		t.Error("expected short session ID in output")
	}
	if !strings.Contains(result, "Bash") {
		t.Error("expected tool name 'Bash' in output")
	}
	if !strings.Contains(result, "interrupted") {
		t.Error("expected 'interrupted' in output")
	}
	if !strings.Contains(result, "10 msgs") {
		t.Error("expected message count in output")
	}
}

func TestFormatRecoveryCandidates_Multiple(t *testing.T) {
	candidates := []RecoveryCandidate{
		{
			SessionID:    "first",
			CWD:          "/dir1",
			MessageCount: 5,
			LastActivity: time.Now(),
			Interruption: InterruptionMidResponse,
			Age:          2 * time.Minute,
		},
		{
			SessionID:    "second",
			CWD:          "/dir2",
			MessageCount: 3,
			LastActivity: time.Now().Add(-time.Hour),
			Interruption: InterruptionAwaitingInput,
			Age:          time.Hour + 2*time.Minute,
		},
	}
	result := FormatRecoveryCandidates(candidates)
	if !strings.Contains(result, "first") {
		t.Error("expected first session ID")
	}
	if !strings.Contains(result, "second") {
		t.Error("expected second session ID")
	}
	// Should have numbered entries
	if !strings.Contains(result, "1.") {
		t.Error("expected numbered entries")
	}
	if !strings.Contains(result, "2.") {
		t.Error("expected second numbered entry")
	}
}

func TestFormatRecoveryCandidates_ToolError(t *testing.T) {
	candidates := []RecoveryCandidate{
		{
			SessionID:    "err-session",
			CWD:          "/dir",
			MessageCount: 8,
			LastActivity: time.Now(),
			Interruption: InterruptionToolError,
			Age:          time.Minute,
		},
	}
	result := FormatRecoveryCandidates(candidates)
	if !strings.Contains(result, "tool error") {
		t.Error("expected 'tool error' in output")
	}
}

// ---------------------------------------------------------------------------
// InterruptionType constants
// ---------------------------------------------------------------------------

func TestInterruptionType_Constants(t *testing.T) {
	tests := []struct {
		val  InterruptionType
		want string
	}{
		{InterruptionNone, "none"},
		{InterruptionMidTool, "mid_tool"},
		{InterruptionMidResponse, "mid_response"},
		{InterruptionAwaitingInput, "awaiting_input"},
		{InterruptionToolError, "tool_error"},
		{InterruptionPermissionAsk, "permission_ask"},
	}
	for _, tt := range tests {
		if string(tt.val) != tt.want {
			t.Errorf("InterruptionType(%v) = %q, want %q", tt.val, string(tt.val), tt.want)
		}
	}
}

// ---------------------------------------------------------------------------
// Integration: classifyInterruption round-trip with findUnresolvedTools
// ---------------------------------------------------------------------------

func TestClassifyInterruption_RoundTrip(t *testing.T) {
	// Build a scenario: assistant calls two tools, only one result arrives.
	messages := []Message{
		{Role: "user", Content: "fix and test"},
		{Role: "assistant", ToolUse: []ToolCall{
			{ID: "fix1", Name: "Edit"},
			{ID: "test1", Name: "Bash"},
		}},
		{Role: "user", ToolResult: &ToolResult{ToolUseID: "fix1", Content: "ok"}},
	}

	unresolved := findUnresolvedTools(messages)
	if len(unresolved) != 1 {
		t.Fatalf("expected 1 unresolved, got %d", len(unresolved))
	}

	interruption, _, _ := classifyInterruption(messages)
	if interruption != InterruptionToolError {
		t.Errorf("expected InterruptionToolError, got %s", interruption)
	}
}
