package engine

import (
	"strings"
	"testing"
	"time"
)

func eqPrompt(text string) EnqueuedPrompt {
	return EnqueuedPrompt{ID: "p" + text, Text: text, Priority: PriorityNormal, Source: "user", EnqueuedAt: time.Now()}
}

func TestCombinePrefixLen(t *testing.T) {
	gates := []CombineGate{
		{PlainPrompt: true, Text: "a"},
		{PlainPrompt: true, Text: "b"},
		{PlainPrompt: true, HasImages: true, Text: "img"}, // follower with images stops the run
		{PlainPrompt: true, Text: "c"},
	}
	if got := CombinePrefixLen(gates); got != 2 {
		t.Fatalf("CombinePrefixLen = %d, want 2", got)
	}

	syntheticFirst := []CombineGate{{Synthetic: true}, {PlainPrompt: true, Text: "a"}}
	if got := CombinePrefixLen(syntheticFirst); got != 1 {
		t.Fatalf("synthetic front prefix = %d, want 1", got)
	}
	if got := CombinePrefixLen(nil); got != 0 {
		t.Fatalf("empty prefix = %d, want 0", got)
	}
}

func TestJoinTextsAndIsCombined(t *testing.T) {
	if got := JoinTexts([]string{"a", "", "b"}); got != "a\n\nb" {
		t.Fatalf("JoinTexts = %q", got)
	}
	if !IsCombined([]string{"a", "b"}) || IsCombined([]string{"a"}) {
		t.Fatal("IsCombined wrong")
	}
}

func TestDequeueCombinedMergesRun(t *testing.T) {
	pq := NewPromptQueue()
	pq.Enqueue(eqPrompt("first"))
	pq.Enqueue(eqPrompt("second"))

	prompt, ids, merged := pq.DequeueCombined()
	if !merged {
		t.Fatal("expected merge")
	}
	if prompt.Text != "first\n\nsecond" {
		t.Fatalf("merged text = %q", prompt.Text)
	}
	if len(ids) != 2 || ids[0] != "pfirst" || ids[1] != "psecond" {
		t.Fatalf("ids = %v", ids)
	}
	if !pq.IsEmpty() {
		t.Fatalf("queue should be drained, len=%d", pq.Len())
	}
}

func TestDequeueCombinedSteeringSortsFirstAndNeverMerges(t *testing.T) {
	pq := NewPromptQueue()
	pq.Enqueue(eqPrompt("plain1"))
	pq.Enqueue(eqPrompt("plain2"))
	// Higher priority sorts ahead of the plain prompts.
	pq.Enqueue(EnqueuedPrompt{ID: "steer", Text: "urgent", Priority: PrioritySteering, Source: "cron"})

	// First turn: steering alone (front ineligible -> no merge).
	prompt, ids, merged := pq.DequeueCombined()
	if merged || prompt.Text != "urgent" || len(ids) != 1 {
		t.Fatalf("steering turn wrong: merged=%v text=%q ids=%v", merged, prompt.Text, ids)
	}
	// Second turn: the two plain prompts combine.
	prompt, ids, merged = pq.DequeueCombined()
	if !merged || prompt.Text != "plain1\n\nplain2" || len(ids) != 2 {
		t.Fatalf("plain run wrong: merged=%v text=%q ids=%v", merged, prompt.Text, ids)
	}
}

func TestDequeueCombinedNoMergeWhenFrontIneligible(t *testing.T) {
	pq := NewPromptQueue()
	pq.Enqueue(EnqueuedPrompt{ID: "s1", Text: "wake up", Priority: PrioritySteering, Source: "cron"})
	pq.Enqueue(eqPrompt("plain"))

	// The steering prompt sorts to the front and is ineligible.
	prompt, ids, merged := pq.DequeueCombined()
	if merged {
		t.Fatal("synthetic front must not merge")
	}
	if prompt.Text != "wake up" || len(ids) != 1 {
		t.Fatalf("prompt=%+v ids=%v", prompt, ids)
	}
}

func TestDequeueCombinedFollowerWithImageStops(t *testing.T) {
	pq := NewPromptQueue()
	pq.Enqueue(eqPrompt("front"))
	pq.Enqueue(EnqueuedPrompt{
		ID: "img", Text: "look", Priority: PriorityNormal, Source: "user",
		Metadata: map[string]interface{}{"images": []string{"a.png"}},
	})
	pq.Enqueue(eqPrompt("tail"))

	_, _, merged := pq.DequeueCombined()
	if merged {
		t.Fatal("image follower must stop the run")
	}
	if got := pq.Len(); got != 2 {
		t.Fatalf("len = %d, want 2 (image + tail remain)", got)
	}
}

func TestDequeueCombinedEmpty(t *testing.T) {
	pq := NewPromptQueue()
	if _, _, merged := pq.DequeueCombined(); merged {
		t.Fatal("empty queue must not report a merge")
	}
}

func TestJoinSeparatorIsBlankLine(t *testing.T) {
	if !strings.Contains(JoinTexts([]string{"x", "y"}), "\n\n") {
		t.Fatal("separator should be a blank line")
	}
}
