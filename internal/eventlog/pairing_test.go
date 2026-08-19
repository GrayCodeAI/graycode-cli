package eventlog

import "testing"

// testSurface builds a surface node with the given type, op and payload.
func testSurface(t *testing.T, l *Log, typ Type, op string, data any) uint64 {
	t.Helper()
	l.AppendSurface(typ, data, op, 0, 0, nil)
	evs := l.Snapshot()
	return evs[len(evs)-1].Seq
}

func userMsg(t *testing.T, l *Log, content string) uint64 {
	return testSurface(t, l, UserMessage, "append", Message{Role: "user", Content: content})
}

func assistantMsg(t *testing.T, l *Log, content string, calls ...string) uint64 {
	tus := make([]ToolCallPayload, 0, len(calls))
	for _, id := range calls {
		tus = append(tus, ToolCallPayload{ID: id, Name: "test_tool"})
	}
	return testSurface(t, l, AssistantMsg, "append", Message{Role: "assistant", Content: content, ToolUse: tus})
}

func toolResult(t *testing.T, l *Log, callID string, isErr bool) uint64 {
	return testSurface(t, l, ToolResult, "append", ToolResultPayload{ToolUseID: callID, Content: "ok", IsError: isErr})
}

func replaceResult(t *testing.T, l *Log, callID string) uint64 {
	return testSurface(t, l, ToolResult, "replace", ToolResultPayload{ToolUseID: callID, Content: "rewritten"})
}

func TestToolPairingBalancedAt(t *testing.T) {
	t.Run("empty log", func(t *testing.T) {
		if ToolPairingBalancedAt(New(nil), 1) {
			t.Fatal("empty log must report unbalanced")
		}
		if ToolPairingBalancedAt(nil, 1) {
			t.Fatal("nil log must report unbalanced")
		}
	})

	t.Run("plain messages are balanced", func(t *testing.T) {
		l := New(nil)
		u1 := userMsg(t, l, "hello")
		a1 := assistantMsg(t, l, "hi")
		u2 := userMsg(t, l, "again")
		if !ToolPairingBalancedAt(l, u1) {
			t.Fatal("cut before first user must be balanced")
		}
		if !ToolPairingBalancedAt(l, a1) || !ToolPairingBalancedAfter(l, a1) {
			t.Fatal("no tool calls: both edges of assistant balanced")
		}
		if !ToolPairingBalancedAt(l, a1) || !ToolPairingBalancedAt(l, u2) {
			t.Fatal("interior cuts of a paired conversation stay balanced")
		}
	})

	t.Run("unanswered call blocks the trailing cut", func(t *testing.T) {
		l := New(nil)
		u1 := userMsg(t, l, "do it")
		a1 := assistantMsg(t, l, "calling", "call-1")
		if !ToolPairingBalancedAt(l, u1) {
			t.Fatal("cut before user must be balanced")
		}
		if !ToolPairingBalancedAt(l, a1) {
			t.Fatal("cut before assistant must be balanced (call not yet open)")
		}
		if ToolPairingBalancedAfter(l, a1) {
			t.Fatal("cut after assistant with an open call must be unbalanced")
		}
	})

	t.Run("result closes the pair", func(t *testing.T) {
		l := New(nil)
		a1 := assistantMsg(t, l, "calling", "call-1")
		r1 := toolResult(t, l, "call-1", false)
		if ToolPairingBalancedAfter(l, a1) {
			t.Fatal("cut after assistant must still be open")
		}
		// The cut BEFORE the result sits after the assistant's open call.
		if ToolPairingBalancedAt(l, r1) {
			t.Fatal("cut before result sits inside the open pair")
		}
		if !ToolPairingBalancedAfter(l, r1) {
			t.Fatal("cut after the matching result must be balanced")
		}
	})

	t.Run("multiple calls pair in order", func(t *testing.T) {
		l := New(nil)
		a1 := assistantMsg(t, l, "two calls", "call-1", "call-2")
		r1 := toolResult(t, l, "call-1", false)
		r2 := toolResult(t, l, "call-2", false)
		if ToolPairingBalancedAfter(l, a1) || ToolPairingBalancedAfter(l, r1) {
			t.Fatal("open until both results arrive")
		}
		if !ToolPairingBalancedAfter(l, r2) {
			t.Fatal("balanced after the last result")
		}
	})

	t.Run("non-surface events are neutral", func(t *testing.T) {
		l := New(nil)
		a1 := assistantMsg(t, l, "calling", "call-1")
		l.Append(ToolCall, ToolCallPayload{Turn: 1, Step: 1, ID: "call-1", Name: "test_tool"})
		l.Append(CompactionSummary, CompactionSummaryFact{Summary: "s"})
		turnStart := l.Snapshot()[len(l.Snapshot())-1].Seq + 1
		l.AppendTurnStart(1)
		r1 := toolResult(t, l, "call-1", false)
		// The turn/start seq (non-surface) must never report balanced-at.
		if ToolPairingBalancedAt(l, turnStart) {
			t.Fatal("non-surface seq must not be a cut candidate")
		}
		if ToolPairingBalancedAfter(l, a1) {
			t.Fatal("tool/call + compaction/summary must not close the pair")
		}
		if !ToolPairingBalancedAfter(l, r1) {
			t.Fatal("tool/call + compaction/summary must not affect balance")
		}
	})

	t.Run("session.compacted is log-only", func(t *testing.T) {
		l := New(nil)
		a1 := assistantMsg(t, l, "calling", "call-1")
		l.AppendCompaction(CompactionFact{Strategy: "summarize", TokensBefore: 100, TokensAfter: 10})
		r1 := toolResult(t, l, "call-1", false)
		if ToolPairingBalancedAfter(l, a1) {
			t.Fatal("session.compacted must not close the pair")
		}
		if !ToolPairingBalancedAfter(l, r1) {
			t.Fatal("session.compacted must not re-open it either")
		}
	})

	t.Run("unknown seq reports false", func(t *testing.T) {
		l := New(nil)
		userMsg(t, l, "hello")
		if ToolPairingBalancedAt(l, 999) {
			t.Fatal("absent seq must report false")
		}
	})
}

func TestToolPairingExemptions(t *testing.T) {
	t.Run("replace-surface result does not unbalance", func(t *testing.T) {
		l := New(nil)
		u1 := userMsg(t, l, "do it")
		a1 := assistantMsg(t, l, "calling", "call-1")
		r1 := toolResult(t, l, "call-1", false)
		replaced := replaceResult(t, l, "call-1")
		if !ToolPairingBalancedAt(l, u1) || !ToolPairingBalancedAt(l, a1) || !ToolPairingBalancedAfter(l, r1) {
			t.Fatal("leading cuts stay balanced")
		}
		if !ToolPairingBalancedAt(l, replaced) {
			t.Fatal("replace node is a surface member")
		}
		if !ToolPairingBalancedAfter(l, replaced) {
			t.Fatal("replace result must not reopen the pair (delta 0)")
		}
	})

	t.Run("synthetic TOOL_NOT_STARTED result is exempt", func(t *testing.T) {
		l := New(nil)
		a1 := assistantMsg(t, l, "denied")
		synthetic := toolResult(t, l, "", true)
		if !ToolPairingBalancedAt(l, a1) || !ToolPairingBalancedAfter(l, synthetic) {
			t.Fatal("synthetic fail-closed result must not unbalance the surface")
		}
	})

	t.Run("real result without a call marks the surface corrupt", func(t *testing.T) {
		l := New(nil)
		u1 := userMsg(t, l, "hi")
		orphan := toolResult(t, l, "ghost-call", false)
		u2 := userMsg(t, l, "after")
		if !ToolPairingBalancedAt(l, u1) {
			t.Fatal("before corruption all cuts stay balanced")
		}
		// The cut immediately before the ghost result has no open call across
		// it; the corruption is only visible once the result is consumed.
		if !ToolPairingBalancedAt(l, orphan) {
			t.Fatal("cut before the corrupt result is still balanced")
		}
		if ToolPairingBalancedAfter(l, orphan) {
			t.Fatal("cut after the corrupt result must be unbalanced")
		}
		if ToolPairingBalancedAt(l, u2) || ToolPairingBalancedAfter(l, u2) {
			t.Fatal("cuts after corruption must never authorize")
		}
	})
}

func TestSafeCompactionRange(t *testing.T) {
	t.Run("nil log rejected", func(t *testing.T) {
		if ok, _ := SafeCompactionRange(nil, 1, 2); ok {
			t.Fatal("nil log must reject")
		}
	})

	t.Run("closed conversation range is legal", func(t *testing.T) {
		l := New(nil)
		u1 := userMsg(t, l, "do it")
		assistantMsg(t, l, "calling", "call-1")
		r1 := toolResult(t, l, "call-1", false)
		u2 := userMsg(t, l, "then this")
		a2 := assistantMsg(t, l, "done")
		if ok, reason := SafeCompactionRange(l, u1, a2); !ok {
			t.Fatalf("fully paired range must be legal: %s", reason)
		}
		if !ToolPairingBalancedAt(l, u2) {
			t.Fatal("cut before second user must be balanced")
		}
		if ok, _ := SafeCompactionRange(l, u1, r1); !ok {
			t.Fatal("range ending at the result must be legal")
		}
	})

	t.Run("unbalanced trailing edge rejected", func(t *testing.T) {
		l := New(nil)
		u1 := userMsg(t, l, "do it")
		a1 := assistantMsg(t, l, "calling", "call-1")
		if ok, reason := SafeCompactionRange(l, u1, a1); ok {
			t.Fatalf("range with an open call at the trailing edge must be rejected: %s", reason)
		}
	})

	t.Run("unbalanced leading edge rejected", func(t *testing.T) {
		l := New(nil)
		u1 := userMsg(t, l, "do it")
		a1 := assistantMsg(t, l, "calling", "call-1")
		r1 := toolResult(t, l, "call-1", false)
		u2 := userMsg(t, l, "then")
		if ok, reason := SafeCompactionRange(l, r1, u2); ok {
			t.Fatalf("range starting inside an open pair must be rejected: %s", reason)
		}
		if !ToolPairingBalancedAt(l, u1) {
			t.Fatal("u1 is a balanced cut candidate")
		}
		if ToolPairingBalancedAfter(l, a1) {
			t.Fatal("a1 leaves an open pair")
		}
	})

	t.Run("non-surface endpoints rejected", func(t *testing.T) {
		l := New(nil)
		u1 := userMsg(t, l, "hi")
		a1 := assistantMsg(t, l, "yo")
		l.AppendTurnStart(1)
		ts := l.Snapshot()[len(l.Snapshot())-1].Seq
		if ok, reason := SafeCompactionRange(l, ts, a1); ok {
			t.Fatalf("turn/start is not a surface node: %s", reason)
		}
		if ok, _ := SafeCompactionRange(l, u1, 999); ok {
			t.Fatal("absent end seq must be rejected")
		}
	})

	t.Run("start after end rejected", func(t *testing.T) {
		l := New(nil)
		u1 := userMsg(t, l, "hi")
		a1 := assistantMsg(t, l, "yo")
		if ok, _ := SafeCompactionRange(l, a1, u1); ok {
			t.Fatal("inverted range must be rejected")
		}
	})

	t.Run("completed turn history shadowed whole is legal", func(t *testing.T) {
		l := New(nil)
		l.AppendTurnStart(1)
		u1 := userMsg(t, l, "turn one")
		a1 := assistantMsg(t, l, "answer")
		l.AppendTurnEnd(1, "completed")
		l.AppendTurnStart(2)
		u2 := userMsg(t, l, "turn two")
		a2 := assistantMsg(t, l, "answer")
		l.AppendTurnEnd(2, "completed")
		// Shadow turn 1 fully (its start precedes the range, its end sits
		// inside); the range ends at turn 1's last surface, before turn 2.
		if ok, reason := SafeCompactionRange(l, u1, a1); !ok {
			t.Fatalf("fully completed turn must be legal to shadow: %s", reason)
		}
		if ok, reason := SafeCompactionRange(l, u1, a2); !ok {
			t.Fatalf("both completed turns legal: %s", reason)
		}
		if !ToolPairingBalancedAt(l, u2) {
			t.Fatal("turn two interior cut stays balanced")
		}
	})

	t.Run("live turn start shadowed rejected", func(t *testing.T) {
		l := New(nil)
		l.AppendTurnStart(1)
		u1 := userMsg(t, l, "turn one")
		a1 := assistantMsg(t, l, "answer")
		l.AppendTurnEnd(1, "completed")
		l.AppendTurnStart(2)
		u2 := userMsg(t, l, "turn two")
		a2 := assistantMsg(t, l, "answer")
		// Range reaches into the open turn 2 and shadows its turn/start marker.
		if ok, reason := SafeCompactionRange(l, u1, a2); ok {
			t.Fatalf("range shadowing a live turn's start must be rejected: %s", reason)
		}
		if !ToolPairingBalancedAt(l, u2) {
			t.Fatal("turn two interior cut stays balanced")
		}
		// Range ending before turn 2's start marker is legal.
		if ok, reason := SafeCompactionRange(l, u1, a1); !ok {
			t.Fatalf("range ending in completed history must be legal: %s", reason)
		}
	})
}

func TestSurfaceSeqAtAndCount(t *testing.T) {
	l := New(nil)
	u1 := userMsg(t, l, "hi")
	a1 := assistantMsg(t, l, "yo")
	r1 := toolResult(t, l, "call-1", false)
	l.Append(ToolCall, ToolCallPayload{ID: "call-1"})
	l.AppendTurnStart(1)

	if n := SurfaceCount(l); n != 3 {
		t.Fatalf("surface count = %d, want 3", n)
	}
	for idx, want := range []uint64{u1, a1, r1} {
		got, ok := SurfaceSeqAt(l, idx)
		if !ok || got != want {
			t.Fatalf("SurfaceSeqAt(%d) = (%d,%v), want (%d,true)", idx, got, ok, want)
		}
	}
	if _, ok := SurfaceSeqAt(l, 3); ok {
		t.Fatal("index past the surface must report not-found")
	}
	if _, ok := SurfaceSeqAt(l, -1); ok {
		t.Fatal("negative index must report not-found")
	}
	if n := SurfaceCount(nil); n != 0 {
		t.Fatal("nil log has no surface")
	}
}

func TestHasOrphanedCompaction(t *testing.T) {
	t.Run("balanced bracket", func(t *testing.T) {
		l := New(nil)
		l.AppendCompactionStart("summarize")
		l.AppendCompactionEnd(CompactionEndFact{Strategy: "summarize"})
		if HasOrphanedCompaction(l) {
			t.Fatal("balanced bracket must not be orphaned")
		}
	})

	t.Run("unmatched start is orphaned", func(t *testing.T) {
		l := New(nil)
		l.AppendCompactionStart("summarize")
		if !HasOrphanedCompaction(l) {
			t.Fatal("start without end must be orphaned")
		}
	})

	t.Run("end without start is not orphaned", func(t *testing.T) {
		l := New(nil)
		l.AppendCompactionEnd(CompactionEndFact{Strategy: "summarize"})
		if HasOrphanedCompaction(l) {
			t.Fatal("end without start cannot orphan a start")
		}
	})

	t.Run("id-correlated brackets", func(t *testing.T) {
		l := New(nil)
		l.AppendCompactionStartFull(CompactionStartFact{Strategy: "summarize", CompactionID: "c1"})
		l.AppendCompactionStartFull(CompactionStartFact{Strategy: "summarize", CompactionID: "c2"})
		l.AppendCompactionEnd(CompactionEndFact{Strategy: "summarize", CompactionID: "c1"})
		if !HasOrphanedCompaction(l) {
			t.Fatal("c2 remains orphaned after c1 closes")
		}
		l.AppendCompactionEnd(CompactionEndFact{Strategy: "summarize", CompactionID: "c2"})
		if HasOrphanedCompaction(l) {
			t.Fatal("all ids closed")
		}
	})

	t.Run("nil log", func(t *testing.T) {
		if HasOrphanedCompaction(nil) {
			t.Fatal("nil log has no orphan")
		}
	})
}

func TestReplaceGenerationRebuildsMembership(t *testing.T) {
	// After a replace, the summary node is a fresh high-seq surface member
	// while the shadowed events stay in the raw log. The pairing fold must see
	// the new node and keep the surface balanced (delta 0 for the replace).
	l := New(nil)
	u1 := userMsg(t, l, "old turn")
	a1 := assistantMsg(t, l, "calling", "call-1")
	r1 := toolResult(t, l, "call-1", false)

	// Replace the whole old surface with a summary user node.
	summarySeq := l.Snapshot()[len(l.Snapshot())-1].Seq + 1
	l.AppendSurface(UserMessage, Message{Role: "user", Content: "[summary]"}, "replace", u1, r1, []uint64{u1, a1, r1})
	if summarySeq != l.Snapshot()[len(l.Snapshot())-1].Seq {
		t.Fatal("summary node must land at a fresh high seq")
	}
	if !ToolPairingBalancedAt(l, summarySeq) {
		t.Fatal("summary node must be a balanced cut candidate")
	}
	if ToolPairingBalancedAt(l, u1) || ToolPairingBalancedAt(l, r1) {
		t.Fatal("shadowed members must not authorize cuts after the replace")
	}
	// The raw log still holds the shadowed events for deterministic replay.
	if got := SurfaceCount(l); got != 4 {
		t.Fatalf("surface count after replace = %d, want 4 (3 shadowed + 1 summary)", got)
	}
}
