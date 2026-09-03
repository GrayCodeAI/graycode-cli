package eventlog

import "testing"

func TestProjectMessagesOnlyUserAndAssistant(t *testing.T) {
	l := New(nil)
	l.Append(SessionMeta, Meta{ID: "s1", FormatVersion: 1})
	l.Append(UserMessage, Message{Content: "hi"})
	l.Append(ToolCall, ToolCallPayload{Name: "Read"})
	l.Append(AssistantMsg, Message{Content: "hello"})
	l.Append(SessionCompacted, Message{Content: "summary"})

	got := ProjectMessages(l.Snapshot())
	if len(got) != 2 {
		t.Fatalf("projected %d messages, want 2", len(got))
	}
	if got[0].Content != "hi" || got[1].Content != "hello" {
		t.Fatalf("unexpected projection order: %+v", got)
	}
}

// Compaction events passing a Message payload may also be projected when their
// payload is a summarised user turn. Because the summarised message is still
// model-visible and carries enough context to stand alone, the projection can be
// a full turns-history transcript that looms compaction facts.
func TestProjectMessagesIgnoresNonMessagePayloads(t *testing.T) {
	l := New(nil)
	// SessionCompacted with a non-Message payload must not corrupt the projection.
	l.Append(SessionCompacted, nil)
	l.Append(UserMessage, Message{Content: "only"})
	got := ProjectMessages(l.Snapshot())
	if len(got) != 1 {
		t.Fatalf("projected %d messages, want 1", len(got))
	}
}

// TestProjectMessagesEmptyAssistantSkipped verifies that empty-content
// assistant messages (used only to host usage) are dropped, matching DSH's
// deriveEventMessage which returns null for content-less assistant/message.
func TestProjectMessagesEmptyAssistantSkipped(t *testing.T) {
	l := New(nil)
	l.Append(UserMessage, Message{Content: "hi"})
	// Empty assistant message — exists only to host usage data.
	l.Append(AssistantMsg, Message{})
	l.Append(AssistantMsg, Message{Content: "hello"})
	got := ProjectMessages(l.Snapshot())
	if len(got) != 2 {
		t.Fatalf("projected %d messages, want 2 (empty assistant skipped)", len(got))
	}
	if got[0].Content != "hi" || got[1].Content != "hello" {
		t.Fatalf("unexpected projection: %+v", got)
	}
}

// TestTurnEndReasonAndIgnorableAndFormatVersion covers the DSH parity additions:
// turn/end carries a reason (not just a boundary), ignorable flag round-trips,
// and Validate refuses foreign format versions.
func TestTurnEndReasonAndIgnorableAndFormatVersion(t *testing.T) {
	log := New(nil)
	// SessionMeta with current format version
	log.Append(SessionMeta, Meta{ID: "s1", FormatVersion: SessionFormatVersion})
	log.AppendTurnStart(1)
	log.AppendTurnEnd(1, "max-tokens")
	log.AppendTurnEndWithError(1, "bad gateway", "502")
	log.AppendTurnEndAborted(1, "user cancellation")
	// Ignorable event (informational, safe to skip)
	log.AppendIgnorable(LLMRetry, LLMRetryFact{Attempt: 1, Reason: "transient"})

	snap := log.Snapshot()
	if len(snap) != 6 {
		t.Fatalf("expected 6 events, got %d", len(snap))
	}

	// Verify turn/end facts carry reasons
	endEvents := log.OfType(TurnEnd)
	if len(endEvents) != 3 {
		t.Fatalf("expected 3 turn/end events, got %d", len(endEvents))
	}
	if endEvents[0].Data.(TurnEndFact).Reason != "max-tokens" {
		t.Errorf("turn/end 0 reason: got %q, want max-tokens", endEvents[0].Data.(TurnEndFact).Reason)
	}
	if endEvents[1].Data.(TurnEndFact).Reason != "error" {
		t.Errorf("turn/end 1 reason: got %q, want error", endEvents[1].Data.(TurnEndFact).Reason)
	}
	if endEvents[2].Data.(TurnEndFact).Reason != "aborted" {
		t.Errorf("turn/end 2 reason: got %q, want aborted", endEvents[2].Data.(TurnEndFact).Reason)
	}

	// Verify ignorable flag round-trips through MarshalWire/DecodeWire
	wire, err := MarshalWire(snap)
	if err != nil {
		t.Fatal(err)
	}
	events, err := DecodeWire(wire)
	if err != nil {
		t.Fatal(err)
	}
	retryEv := events[5]
	if !retryEv.Ignorable {
		t.Error("ignorable flag should round-trip as true")
	}

	// Verify format version refusal: a meta with version 999 must fail Validate
	badLog := New(nil)
	badLog.Append(SessionMeta, Meta{ID: "bad", FormatVersion: 999})
	badLog.Append(UserMessage, Message{Content: "hi"})
	if err := Validate(badLog.Snapshot()); err == nil {
		t.Fatal("expected format version refusal, got nil")
	}
}

// TestProjectMessagesFullSurface verifies the enhanced projection that
// matches DSH's deriveMessages(): request headers become system messages,
// context injection adds system messages, and compaction summary/prune
// folds replace pruned content.
func TestProjectMessagesFullSurface(t *testing.T) {
	l := New(nil)
	l.Append(RequestHeader, RequestHeaderFact{
		System: "You are a helpful assistant.",
		Reason: RequestHeaderInitial,
	})
	l.Append(UserMessage, Message{Content: "hello"})
	l.Append(AssistantMsg, Message{Content: "hi there"})
	l.Append(ContextInjected, ContextInjectedFact{Content: "Project: graycode"})
	l.Append(AssistantMsg, Message{Content: "what can I help with?"})
	// Compaction: prune 2 messages, then summary
	l.Append(CompactionStart, CompactionStartFact{Strategy: "auto"})
	l.Append(CompactionPrune, CompactionPruneFact{Strategy: "auto", Messages: 2})
	l.Append(CompactionSummary, CompactionSummaryFact{Summary: "User asked for help, assistant replied."})
	l.Append(CompactionEnd, CompactionEndFact{Strategy: "auto"})
	// One more message after compaction
	l.Append(UserMessage, Message{Content: "more context"})

	got := ProjectMessages(l.Snapshot())

	// Expected: system(prompt) + user(hello) + assistant(hi there)
	// After prune(2): system(prompt) + user(hello) + assistant(hi there)
	// Then: + system(summary) + user(more)
	if len(got) != 5 {
		t.Fatalf("projected %d messages, want 5: %+v", len(got), got)
	}
	if got[0].Role != "system" || got[0].Content != "You are a helpful assistant." {
		t.Errorf("msg 0: got %+v, want system/You are a helpful assistant.", got[0])
	}
	if got[1].Content != "hello" {
		t.Errorf("msg 1: got %+v, want Content=hello", got[1])
	}
	if got[2].Content != "hi there" {
		t.Errorf("msg 2: got %+v, want Content=hi there", got[2])
	}
	// system(context) and assistant(what) were pruned by the compaction pass
	if got[3].Role != "system" || got[3].Content != "User asked for help, assistant replied." {
		t.Errorf("msg 3: got %+v, want system summary", got[3])
	}
	if got[4].Content != "more context" {
		t.Errorf("msg 4: got %+v, want Content=more context", got[4])
	}
}

// TestProjectMessagesSurfaceReplaceShadowing verifies that a surface `replace`
// op splices its replacement in at the replaced position and shadows the
// replaced nodes out of the projected history — DSH deriveMessages parity.
func TestProjectMessagesSurfaceReplaceShadowing(t *testing.T) {
	l := New(nil)
	// A compact prior history: user turn + assistant reply.
	oldUser := userMsg(t, l, "old turn")
	asstSeq := assistantMsg(t, l, "old reply", "call-1")

	// Replace the whole prior surface (oldUser+asst) with a fresh user recap.
	l.AppendSurface(UserMessage, Message{Role: "user", Content: "[recap]"}, "replace", oldUser, asstSeq, []uint64{oldUser, asstSeq})

	// A new turn after the replacement.
	userMsg(t, l, "follow-up")

	got := ProjectMessages(l.Snapshot())
	if len(got) != 2 {
		t.Fatalf("projected %d messages, want 2 (replacement + follow-up): %+v", len(got), got)
	}
	// The replacement splices in at the replaced position (head), not the tail;
	// the shadowed old turn/reply are absent.
	if got[0].Content != "[recap]" {
		t.Errorf("msg 0: got %+v, want replacement spliced at head", got[0])
	}
	if got[1].Content != "follow-up" {
		t.Errorf("msg 1: got %+v, want follow-up at tail", got[1])
	}
}

// TestProjectMessagesSurfaceReplaceMidSurface verifies a replacement of a
// single interior node keeps its surface neighbors in the correct order.
func TestProjectMessagesSurfaceReplaceMidSurface(t *testing.T) {
	l := New(nil)
	userMsg(t, l, "a")
	b := userMsg(t, l, "b")
	userMsg(t, l, "c")

	// Replace interior node b with a rewritten reword.
	l.AppendSurface(UserMessage, Message{Role: "user", Content: "B2"}, "replace", b, b, []uint64{b})

	got := ProjectMessages(l.Snapshot())
	if len(got) != 3 {
		t.Fatalf("projected %d messages, want 3: %+v", len(got), got)
	}
	if got[0].Content != "a" || got[1].Content != "B2" || got[2].Content != "c" {
		t.Fatalf("interior replacement out of order: %+v", got)
	}
}
