package eventlog

import (
	"strings"
	"testing"
)

// mkSurfaceEvent builds a surface-carrying event at seq with an explicit
// SurfaceOp and optional source-event provenance.
func mkSurfaceEvent(t Type, seq uint64, op string, start, end uint64, sources []uint64, data any) Event {
	return Event{Type: t, Seq: seq, Data: data, SurfaceOp: &SurfaceOp{Op: op, Start: start, End: end}, SourceEventSeqs: sources}
}

func userEv(seq uint64, content string) Event {
	return mkSurfaceEvent(UserMessage, seq, SurfaceOpAppend, 0, 0, nil, Message{Role: "user", Content: content})
}

func asstEv(seq uint64, content string, calls ...string) Event {
	tus := make([]ToolCallPayload, 0, len(calls))
	for _, id := range calls {
		tus = append(tus, ToolCallPayload{ID: id, Name: "test_tool"})
	}
	return mkSurfaceEvent(AssistantMsg, seq, SurfaceOpAppend, 0, 0, nil, Message{Role: "assistant", Content: content, ToolUse: tus})
}

func toolResultEv(seq uint64, callID string, content string, isErr bool) Event {
	return mkSurfaceEvent(ToolResult, seq, SurfaceOpAppend, 0, 0, nil, ToolResultPayload{ToolUseID: callID, Content: content, IsError: isErr})
}

func replaceUserEv(seq uint64, start, end uint64, sources []uint64, content string) Event {
	return mkSurfaceEvent(UserMessage, seq, SurfaceOpReplace, start, end, sources, Message{Role: "user", Content: content})
}

func replaceAsstEv(seq uint64, start, end uint64, sources []uint64, content string) Event {
	return mkSurfaceEvent(AssistantMsg, seq, SurfaceOpReplace, start, end, sources, Message{Role: "assistant", Content: content})
}

func errContains(t *testing.T, err error, substr string) {
	t.Helper()
	if err == nil {
		t.Fatalf("expected error containing %q, got nil", substr)
	}
	if !strings.Contains(err.Error(), substr) {
		t.Fatalf("expected error containing %q, got %q", substr, err.Error())
	}
}

func TestFoldSurfaceProvenanceAccept(t *testing.T) {
	// A surface with absent provenance and a replace with complete coverage folds.
	events := []Event{
		userEv(1, "first"),
		asstEv(2, "summary", "call-1"),
		// Replace the assistant summary (its tool call is shadowed); sources
		// must cover both shadowed nodes.
		replaceAsstEv(3, 2, 2, []uint64{2}, "cleaned"),
	}
	res, err := FoldSurface(events)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if want := []uint64{1, 3}; !equalUint64(res.Nodes, want) {
		t.Fatalf("nodes = %v, want %v", res.Nodes, want)
	}
	if len(res.Replacements) != 1 {
		t.Fatalf("want 1 replacement, got %d", len(res.Replacements))
	}
	rep := res.Replacements[0]
	if rep.Seq != 3 || rep.Start != 2 || rep.End != 2 {
		t.Fatalf("replacement = %+v, want seq=3 start=2 end=2", rep)
	}
	if want := []uint64{2}; !equalUint64(rep.ShadowedSeqs, want) {
		t.Fatalf("shadowed = %v, want %v", rep.ShadowedSeqs, want)
	}
}

func TestFoldSurfaceSourceRefOnNonSurface(t *testing.T) {
	// A non-surface event carrying source references is rejected.
	events := []Event{
		mkEvent(SessionMeta, 1, Meta{}),
		{Type: TurnStart, Seq: 2, Data: BoundaryFact{Turn: 1}, SourceEventSeqs: []uint64{1}},
	}
	_, err := FoldSurface(events)
	errContains(t, err, "not surface-eligible")
}

func TestFoldSurfaceEmptySourcesNonAssistant(t *testing.T) {
	// Empty (non-nil) source list on a non-assistant surface event is rejected;
	// an explicit-empty assistant append is permitted.
	userWithEmpty := mkSurfaceEvent(UserMessage, 1, SurfaceOpAppend, 0, 0, []uint64{}, Message{Role: "user", Content: "x"})
	if _, err := FoldSurface([]Event{userWithEmpty}); err == nil {
		t.Fatal("expected error for empty sources on user/message")
	}

	asstWithEmpty := asstEv(1, "ok")
	asstWithEmpty.SourceEventSeqs = []uint64{}
	if _, err := FoldSurface([]Event{asstWithEmpty}); err != nil {
		t.Fatalf("explicit-empty assistant append must be allowed: %v", err)
	}
}

func TestFoldSurfaceProvenanceRejections(t *testing.T) {
	base := []Event{userEv(1, "a")}

	t.Run("duplicate sources", func(t *testing.T) {
		ev := replaceUserEv(2, 1, 1, []uint64{1, 1}, "b")
		_, err := FoldSurface(append(base, ev))
		errContains(t, err, "must not contain duplicates")
	})

	t.Run("self reference", func(t *testing.T) {
		ev := replaceUserEv(2, 1, 1, []uint64{2}, "b") // source == self seq
		_, err := FoldSurface(append(base, ev))
		errContains(t, err, "must reference earlier events")
	})

	t.Run("future reference", func(t *testing.T) {
		ev := replaceUserEv(2, 1, 1, []uint64{9}, "b")
		_, err := FoldSurface(append(base, ev))
		errContains(t, err, "must reference earlier events")
	})

	t.Run("incomplete replacement coverage", func(t *testing.T) {
		// Replace range [1..2] but only cite 1, missing 2.
		events := []Event{
			userEv(1, "a"),
			userEv(2, "b"),
			replaceUserEv(3, 1, 2, []uint64{1}, "c"),
		}
		_, err := FoldSurface(events)
		errContains(t, err, "must include every shadowed surface node; missing 2")
	})
}

func TestFoldSurfaceNonContiguousSeq(t *testing.T) {
	events := []Event{userEv(1, "a"), userEv(3, "gap")} // 2 skipped
	_, err := FoldSurface(events)
	errContains(t, err, "is not contiguous")
}

func TestFoldSurfaceReplaceRangeNotFound(t *testing.T) {
	t.Run("start not found", func(t *testing.T) {
		events := []Event{userEv(1, "a"), replaceUserEv(2, 42, 42, []uint64{}, "b")}
		_, err := FoldSurface(events)
		errContains(t, err, "start seq 42 not found")
	})
	t.Run("end not found", func(t *testing.T) {
		events := []Event{userEv(1, "a"), replaceUserEv(2, 1, 42, []uint64{}, "b")}
		_, err := FoldSurface(events)
		errContains(t, err, "end seq 42 not found")
	})
	t.Run("start after end", func(t *testing.T) {
		events := []Event{userEv(1, "a"), userEv(2, "b"), replaceUserEv(3, 2, 1, []uint64{}, "c")}
		_, err := FoldSurface(events)
		errContains(t, err, "is after")
	})
}

// --- tool/result rewrite restriction (dsh assertToolResultRewrite) ---

func TestFoldSurfaceToolResultReplace(t *testing.T) {
	mk := func() []Event {
		return []Event{
			userEv(1, "use tool"),
			asstEv(2, "call", "call-1"),
			toolResultEv(3, "call-1", "original", false),
		}
	}

	t.Run("content-only rewrite accepted", func(t *testing.T) {
		base := mk()
		ev := mkSurfaceEvent(ToolResult, 4, SurfaceOpReplace, 3, 3, []uint64{3}, ToolResultPayload{ToolUseID: "call-1", Content: "rewritten", IsError: false})
		res, err := FoldSurface(append(base, ev))
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if want := []uint64{1, 2, 4}; !equalUint64(res.Nodes, want) {
			t.Fatalf("nodes = %v, want %v", res.Nodes, want)
		}
	})

	t.Run("rewrites exactly one node", func(t *testing.T) {
		base := mk()
		ev := mkSurfaceEvent(ToolResult, 4, SurfaceOpReplace, 2, 3, []uint64{2, 3}, ToolResultPayload{ToolUseID: "call-1", Content: "x", IsError: false})
		_, err := FoldSurface(append(base, ev))
		errContains(t, err, "must rewrite exactly one current node")
	})

	t.Run("must target a tool/result", func(t *testing.T) {
		base := mk()
		ev := mkSurfaceEvent(ToolResult, 4, SurfaceOpReplace, 2, 2, []uint64{2}, ToolResultPayload{ToolUseID: "call-1", Content: "x", IsError: false})
		_, err := FoldSurface(append(base, ev))
		errContains(t, err, "must target a current tool/result")
	})

	t.Run("toolUseID change rejected", func(t *testing.T) {
		base := mk()
		ev := mkSurfaceEvent(ToolResult, 4, SurfaceOpReplace, 3, 3, []uint64{3}, ToolResultPayload{ToolUseID: "other-call", Content: "x", IsError: false})
		_, err := FoldSurface(append(base, ev))
		errContains(t, err, "may change only content")
	})

	t.Run("isError change rejected", func(t *testing.T) {
		base := mk()
		ev := mkSurfaceEvent(ToolResult, 4, SurfaceOpReplace, 3, 3, []uint64{3}, ToolResultPayload{ToolUseID: "call-1", Content: "x", IsError: true})
		_, err := FoldSurface(append(base, ev))
		errContains(t, err, "may change only content")
	})
}

// --- surface order / replacement splice semantics ---

func TestFoldSurfaceOrderExamples(t *testing.T) {
	t.Run("nodes for a turn", func(t *testing.T) {
		// turn/start(0 non-surface), user(1), assistant(2) -> nodes [1,2]
		events := []Event{
			mkEvent(TurnStart, 1, BoundaryFact{Turn: 1}),
			userEv(2, "hello"),
			asstEv(3, "hi"),
		}
		res, err := FoldSurface(events)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if want := []uint64{2, 3}; !equalUint64(res.Nodes, want) {
			t.Fatalf("nodes = %v, want %v", res.Nodes, want)
		}
	})

	t.Run("rebuild with replace splices range at head", func(t *testing.T) {
		events := []Event{
			userEv(1, "u1"),
			asstEv(2, "a2"),
			replaceAsstEv(3, 1, 2, []uint64{1, 2}, "summary"),
		}
		res, err := FoldSurface(events)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if want := []uint64{3}; !equalUint64(res.Nodes, want) {
			t.Fatalf("nodes = %v, want %v", res.Nodes, want)
		}
	})

	t.Run("replace both ends splices only that range", func(t *testing.T) {
		// dsh: 3 user msgs (seqs 1,2,3), replace 1..2 -> nodes [4,3]
		events := []Event{
			userEv(1, "a"),
			userEv(2, "b"),
			userEv(3, "c"),
			replaceUserEv(4, 1, 2, []uint64{1, 2}, "z"),
		}
		res, err := FoldSurface(events)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if want := []uint64{4, 3}; !equalUint64(res.Nodes, want) {
			t.Fatalf("nodes = %v, want %v", res.Nodes, want)
		}
	})

	t.Run("single-node replace keeps surrounding", func(t *testing.T) {
		events := []Event{
			userEv(1, "a"),
			userEv(2, "b"),
			userEv(3, "c"),
			replaceUserEv(4, 2, 2, []uint64{2}, "z"),
		}
		res, err := FoldSurface(events)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if want := []uint64{1, 4, 3}; !equalUint64(res.Nodes, want) {
			t.Fatalf("nodes = %v, want %v", res.Nodes, want)
		}
	})

	t.Run("mid-replace preserves position", func(t *testing.T) {
		events := []Event{
			userEv(1, "a"),
			userEv(2, "b"),
			userEv(3, "c"),
			replaceUserEv(4, 2, 2, []uint64{2}, "z"),
		}
		res, err := FoldSurface(events)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if want := []uint64{1, 4, 3}; !equalUint64(res.Nodes, want) {
			t.Fatalf("nodes = %v, want %v", res.Nodes, want)
		}
	})
}

func equalUint64(a, b []uint64) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

// --- SurfaceManager incremental behavior ---

func TestSurfaceManagerMatchesFoldSurface(t *testing.T) {
	l := New(nil)
	userMsg(t, l, "hello")
	assistantMsg(t, l, "hi")
	mgr := NewSurfaceManager(l)
	nodes, err := mgr.Nodes()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	res, _ := FoldSurface(l.Snapshot())
	if !equalUint64(nodes, res.Nodes) {
		t.Fatalf("manager nodes %v != fold nodes %v", nodes, res.Nodes)
	}
	if gen, _ := mgr.ReplaceGeneration(); gen != 0 {
		t.Fatalf("generation = %d, want 0", gen)
	}
}

func TestSurfaceManagerIncrementalPicksUpNewEvents(t *testing.T) {
	l := New(nil)
	userMsg(t, l, "hello")
	mgr := NewSurfaceManager(l)
	nodes, _ := mgr.Nodes()
	if want := []uint64{1}; !equalUint64(nodes, want) {
		t.Fatalf("nodes = %v, want %v", nodes, want)
	}
	// New events appended after construction must be folded on next access.
	assistantMsg(t, l, "hi")
	nodes, _ = mgr.Nodes()
	if want := []uint64{1, 2}; !equalUint64(nodes, want) {
		t.Fatalf("nodes after delta = %v, want %v", nodes, want)
	}
	if gen, _ := mgr.ReplaceGeneration(); gen != 0 {
		t.Fatalf("generation = %d, want 0", gen)
	}
}

func TestSurfaceManagerReplaceGenerationAndAtomic(t *testing.T) {
	l := New(nil)
	seqU := userMsg(t, l, "u1")
	seqA := assistantMsg(t, l, "a2")
	_ = seqU
	mgr := NewSurfaceManager(l)
	if gen, _ := mgr.ReplaceGeneration(); gen != 0 {
		t.Fatalf("initial generation = %d, want 0", gen)
	}
	// Replace [1..2] with a summary; generation must bump to 1.
	replEv := mkSurfaceEvent(AssistantMsg, seqA+1, SurfaceOpReplace, 1, seqA, []uint64{1, seqA}, Message{Role: "assistant", Content: "summary"})
	if err := mgr.ValidateNext(replEv); err != nil {
		t.Fatalf("unexpected validation error: %v", err)
	}
	// So far validation must NOT have mutated the surface.
	nodes, _ := mgr.Nodes()
	if want := []uint64{1, seqA}; !equalUint64(nodes, want) {
		t.Fatalf("nodes before commit = %v, want %v (no mutation on validate)", nodes, want)
	}
	// Rejected candidate leaves state unchanged.
	bad := mkSurfaceEvent(ToolResult, seqA+2, SurfaceOpReplace, seqA, seqA, []uint64{seqA}, ToolResultPayload{ToolUseID: "other", Content: "x", IsError: false})
	if err := mgr.ValidateNext(bad); err == nil {
		t.Fatal("expected validation error for bad tool/result rewrite")
	}
	nodes, _ = mgr.Nodes()
	if want := []uint64{1, seqA}; !equalUint64(nodes, want) {
		t.Fatalf("nodes after rejected candidate = %v, want %v", nodes, want)
	}
	if gen, _ := mgr.ReplaceGeneration(); gen != 0 {
		t.Fatalf("generation after rejection = %d, want 0", gen)
	}
}

func TestSurfaceManagerValidateNextNoMutationOnError(t *testing.T) {
	l := New(nil)
	u := userMsg(t, l, "a")
	mgr := NewSurfaceManager(l)
	// Non-contiguous candidate seq rejected.
	cand := userEv(u+5, "gap")
	if err := mgr.ValidateNext(cand); err == nil {
		t.Fatal("expected contiguity error")
	}
	nodes, _ := mgr.Nodes()
	if want := []uint64{u}; !equalUint64(nodes, want) {
		t.Fatalf("nodes after rejected candidate = %v, want %v", nodes, want)
	}
}

func TestFoldSurfaceEmptyIsEmpty(t *testing.T) {
	res, err := FoldSurface([]Event{})
	if err != nil {
		t.Fatalf("empty fold must succeed, got %v", err)
	}
	if len(res.Nodes) != 0 || len(res.Replacements) != 0 {
		t.Fatalf("empty fold produced %+v", res)
	}
}

// TestSurfaceManagerFullReplaceCommit exercises the realistic path: a caller
// approves a candidate (ValidateNext passes), appends it to the log, and the
// manager folds it into the surface with an incremented generation.
func TestSurfaceManagerFullReplaceCommit(t *testing.T) {
	l := New(nil)
	userMsg(t, l, "u1")
	seqA := assistantMsg(t, l, "a2")
	mgr := NewSurfaceManager(l)
	nodes, _ := mgr.Nodes()
	if want := []uint64{1, seqA}; !equalUint64(nodes, want) {
		t.Fatalf("initial nodes = %v, want %v", nodes, want)
	}
	// Pre-validate the candidate.
	replEv := mkSurfaceEvent(AssistantMsg, seqA+1, SurfaceOpReplace, 1, seqA, []uint64{1, seqA}, Message{Role: "assistant", Content: "summary"})
	if err := mgr.ValidateNext(replEv); err != nil {
		t.Fatalf("candidate rejected: %v", err)
	}
	// Commit it to the log (simulating AppendSurface assigning the same seq).
	l.AppendSurface(AssistantMsg, replEv.Data, SurfaceOpReplace, 1, seqA, []uint64{1, seqA})
	nodes, _ = mgr.Nodes()
	if want := []uint64{seqA + 1}; !equalUint64(nodes, want) {
		t.Fatalf("nodes after commit = %v, want %v", nodes, want)
	}
	if gen, _ := mgr.ReplaceGeneration(); gen != 1 {
		t.Fatalf("generation after commit = %d, want 1", gen)
	}
}

// Ensure a valid replace consumes the full sources and witnesses were recorded.
func TestFoldSurfaceMultipleReplacements(t *testing.T) {
	events := []Event{
		userEv(1, "a"),
		userEv(2, "b"),
		userEv(3, "c"),
		replaceUserEv(4, 1, 2, []uint64{1, 2}, "z"),
		userEv(5, "d"),
		// Replace the node that still holds seq 3 (position index 1 after the
		// previous splice), not seq 2 which was already shadowed.
		replaceUserEv(6, 3, 3, []uint64{3}, "y"),
	}
	res, err := FoldSurface(events)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// Build expected fold: start [1,2,3]; r4 splices [1,2]->4 -> [4,3];
	// append 5 -> [4,3,5]; r6 replaces node at index1 (seq3)->6 -> [4,6,5].
	if want := []uint64{4, 6, 5}; !equalUint64(res.Nodes, want) {
		t.Fatalf("nodes = %v, want %v", res.Nodes, want)
	}
	if len(res.Replacements) != 2 {
		t.Fatalf("want 2 replacements, got %d", len(res.Replacements))
	}
	if res.Replacements[0].Seq != 4 || res.Replacements[1].Seq != 6 {
		t.Fatalf("replacement seqs = %d,%d; want 4,6", res.Replacements[0].Seq, res.Replacements[1].Seq)
	}
}
