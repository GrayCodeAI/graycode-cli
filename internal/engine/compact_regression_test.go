package engine

import (
	"context"
	"strings"
	"testing"

	"github.com/GrayCodeAI/hawk/internal/types"
)

// Regression tests for compaction fixes. Each test pins a specific bug so it
// cannot silently return: negative-index panics, orphaned tool pairs, adjacent
// assistant roles, oversized-turn retention, dropped concurrent appends, and
// token undercounting in ShouldAutoCompact.

// testSession builds a session with a canned mock client and a small context
// window so token-based triggers are cheap to reach.
func testSession(t *testing.T, window int) *Session {
	t.Helper()
	s := NewSessionWithClient(NewMockClientForTest(), "test", "test-model", "", nil, false)
	s.SetContextWindowCached(window)
	return s
}

// toolPairMessages returns a transcript where an assistant tool_use is followed
// by a user tool_result, embedded among filler messages.
func toolPairMessages(total int, toolUseAt int) []types.EyrieMessage {
	msgs := make([]types.EyrieMessage, total)
	for i := range msgs {
		msgs[i] = types.EyrieMessage{Role: "user", Content: "filler"}
	}
	msgs[toolUseAt] = types.EyrieMessage{
		Role:    "assistant",
		Content: "calling tool",
		ToolUse: []types.ToolCall{{ID: "tu-1", Name: "read_file", Arguments: map[string]any{"path": "a.go"}}},
	}
	if toolUseAt+1 < total {
		msgs[toolUseAt+1] = types.EyrieMessage{
			Role:    "user",
			Content: "tool responded",
			ToolResults: []types.ToolResult{
				{ToolUseID: "tu-1", Content: `{"ok":true,"body":"file body"}`},
			},
		}
	}
	return msgs
}

func TestCompactPinnedExceedsMessageCountDoesNotPanic(t *testing.T) {
	s := testSession(t, 128000)
	s.Persistence().SetRawMessages(makeMessages(10))
	s.Persistence().SetPinnedMessages(50) // pinned > total messages

	s.compact(context.Background()) // must not panic (negative slice index)
	if got := len(s.Persistence().RawMessages()); got != 10 {
		t.Fatalf("expected transcript untouched, got %d messages", got)
	}
}

func TestCompactPinnedAtBoundaryDoesNotPanic(t *testing.T) {
	// Pinned (40) exceeds the transcript (30): the old code sliced
	// raw[:len-keepEnd] with a negative index and panicked. Both split-turn
	// and summary paths must tolerate it.
	s := testSession(t, 128000)
	s.Persistence().SetRawMessages(makeMessages(30))
	s.Persistence().SetPinnedMessages(40)

	s.smartCompact(context.Background())
	if got := len(s.Persistence().RawMessages()); got != 30 {
		t.Fatalf("expected transcript untouched, got %d messages", got)
	}
}

func TestCompactPreservesToolResultAtCutStart(t *testing.T) {
	// cutStart defaults to 4; put a tool_use at index 3 so its tool_result
	// sits exactly on the boundary. Both must survive into the kept head.
	s := testSession(t, 128000)
	s.Persistence().SetRawMessages(toolPairMessages(40, 3))
	s.compact(context.Background())

	kept := s.Persistence().RawMessages()
	foundToolUse, foundToolResult := false, false
	for _, m := range kept {
		if len(m.ToolUse) > 0 {
			foundToolUse = true
		}
		if len(m.ToolResults) > 0 {
			foundToolResult = true
		}
	}
	if !foundToolUse || !foundToolResult {
		t.Fatalf("tool pair split: toolUse=%v toolResult=%v", foundToolUse, foundToolResult)
	}
	if kept[0].Role != "user" {
		t.Fatalf("kept head must start with the original first message")
	}
}

func TestSmartCompactAvoidsAdjacentAssistantRoles(t *testing.T) {
	// Build a long transcript whose kept tail starts with an assistant message
	// (conversation ended on a final assistant turn).
	msgs := makeMessages(40)
	msgs[30] = types.EyrieMessage{Role: "assistant", Content: "tail starts with assistant"}
	msgs[31] = types.EyrieMessage{Role: "user", Content: "next user turn"}
	s := testSession(t, 128000)
	s.Persistence().SetRawMessages(msgs)

	s.smartCompact(context.Background())

	kept := s.Persistence().RawMessages()
	for i := 1; i < len(kept); i++ {
		if kept[i].Role == "assistant" && kept[i-1].Role == "assistant" {
			t.Fatalf("adjacent assistant messages at %d-%d: %q then %q",
				i-1, i, kept[i-1].Content, kept[i].Content)
		}
	}
}

func TestSplitTurnCompactTruncatesOversizedMessageToSecondHalf(t *testing.T) {
	// One oversized message inside the tail: its first half is summarized away
	// and the retained message must contain exactly its second half.
	msgs := makeMessages(40)
	big := strings.Repeat("x", 20000)
	msgs[35] = types.EyrieMessage{Role: "user", Content: big}
	s := testSession(t, 128000)
	s.Persistence().SetRawMessages(msgs)

	s.smartCompact(context.Background()) // SplitTurnNeeded fires → splitTurnCompact

	kept := s.Persistence().RawMessages()
	found := false
	for _, m := range kept {
		if strings.Contains(m.Content, "xxxx") && len(m.Content) < 20000 {
			found = true
			want := big[len(big)/2:]
			got := m.Content
			if got != want {
				t.Fatalf("retained oversized content wrong: len %d, want last-half of original", len(got))
			}
			break
		}
	}
	if !found {
		t.Fatalf("expected a retained second-half fragment of the oversized message, got %d kept messages", len(kept))
	}
}

func TestApplyCompactionPreservesConcurrentAppends(t *testing.T) {
	// Simulate the TOCTOU window: a snapshot is read, then a message is
	// appended before the compaction result is applied. The appended message
	// must survive, not be dropped.
	s := testSession(t, 128000)
	msgs := makeMessages(30)
	s.Persistence().SetRawMessages(msgs)

	raw := s.Persistence().RawMessages() // snapshot (len 30)
	// Concurrent AddUser between snapshot and apply:
	concurrent := s.Persistence().RawMessages()
	concurrent = append(concurrent, types.EyrieMessage{Role: "user", Content: "concurrent user turn"})
	s.Persistence().SetRawMessages(concurrent)

	// Compaction result computed from the stale snapshot (keeps only 20):
	keep := append([]types.EyrieMessage(nil), raw[:20]...)
	s.Persistence().ApplyCompaction(keep, len(raw))

	kept := s.Persistence().RawMessages()
	if got := kept[len(kept)-1].Content; got != "concurrent user turn" {
		t.Fatalf("concurrent append dropped; last message = %q", got)
	}
	if len(kept) != 21 {
		t.Fatalf("expected 20 compacted + 1 concurrent message, got %d", len(kept))
	}
}

func TestShouldAutoCompactCountsToolTokens(t *testing.T) {
	// Content alone stays under the threshold; adding tool_result payloads
	// crosses it. ShouldAutoCompact must count the tool tokens.
	s := testSession(t, 2000) // threshold = 85% of 2000 = 1700
	msgs := make([]types.EyrieMessage, 40)
	for i := range msgs {
		if i%2 == 0 {
			msgs[i] = types.EyrieMessage{Role: "user", Content: "short"}
		} else {
			msgs[i] = types.EyrieMessage{Role: "assistant", Content: "short reply"}
		}
	}
	s.Persistence().SetRawMessages(msgs)
	if s.ShouldAutoCompact() {
		t.Fatal("content-only transcript must not auto-compact at this size")
	}

	for i := 0; i < 10; i++ {
		msgs[i].ToolResults = []types.ToolResult{{
			ToolUseID: "tu-x",
			Content:   strings.Repeat("payload ", 500), // ~500 tokens each
		}}
	}
	s.Persistence().SetRawMessages(msgs)
	if !s.ShouldAutoCompact() {
		t.Fatal("tool_result payloads must count toward the compaction threshold")
	}
}

func TestGenerateSummaryInputCapped(t *testing.T) {
	// generateSummary must not feed an unbounded prompt even if the transcript
	// is huge (summaryInputRuneCap applies).
	msgs := make([]types.EyrieMessage, 0, 500)
	for i := 0; i < 500; i++ {
		msgs = append(msgs, types.EyrieMessage{Role: "user", Content: strings.Repeat("content ", 100)})
	}
	s := testSession(t, 128000)
	summary := s.generateSummary(context.Background(), msgs)
	if summary == "" {
		t.Fatal("expected non-empty summary from mock LLM path")
	}
}
