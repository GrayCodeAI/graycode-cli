package engine

import (
	"strings"
	"sync"
	"testing"
)

type recallBox struct {
	mu    sync.Mutex
	value string
}

func (b *recallBox) set(s string) {
	b.mu.Lock()
	b.value = s
	b.mu.Unlock()
}

func (b *recallBox) get() string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.value
}

func TestMemoryIncrementalFirstCallFull(t *testing.T) {
	b := &recallBox{value: "remembered X"}
	mi, err := newMemoryIncremental(b.get)
	if err != nil {
		t.Fatalf("newMemoryIncremental: %v", err)
	}
	content, changed := mi.prepare()
	if !changed {
		t.Fatal("first call should report changed")
	}
	if !strings.Contains(content, "remembered X") {
		t.Fatalf("content = %q", content)
	}
}

func TestMemoryIncrementalUnchangedSkips(t *testing.T) {
	b := &recallBox{value: "remembered X"}
	mi, _ := newMemoryIncremental(b.get)
	if _, _ = mi.prepare(); true {
		// first call establishes baseline
	}
	content, changed := mi.prepare()
	if changed {
		t.Fatalf("unchanged recall should not rewrite, got changed content=%q", content)
	}
	if content != "" {
		t.Fatalf("expected empty content for unchanged, got %q", content)
	}
}

func TestMemoryIncrementalChangeRewrites(t *testing.T) {
	b := &recallBox{value: "remembered X"}
	mi, _ := newMemoryIncremental(b.get)
	mi.prepare() // baseline

	b.set("remembered Y")
	content, changed := mi.prepare()
	if !changed {
		t.Fatal("changed recall should report changed")
	}
	if !strings.Contains(content, "remembered Y") {
		t.Fatalf("content = %q", content)
	}

	// And it stabilizes: next call is unchanged.
	_, changed = mi.prepare()
	if changed {
		t.Fatal("should stabilize after applying change")
	}
}

func TestMemoryIncrementalNilRecall(t *testing.T) {
	mi, err := newMemoryIncremental(nil)
	if err != nil {
		t.Fatalf("newMemoryIncremental(nil): %v", err)
	}
	content, changed := mi.prepare()
	if !changed {
		t.Fatal("should initialize even with nil recall")
	}
	if content != "" {
		t.Fatalf("expected empty content for nil recall, got %q", content)
	}
}

func TestIncrementalContextEnabledFlag(t *testing.T) {
	t.Setenv("HAWK_INCREMENTAL_CONTEXT", "1")
	if !incrementalContextEnabled() {
		t.Fatal("expected enabled with HAWK_INCREMENTAL_CONTEXT=1")
	}
	t.Setenv("HAWK_INCREMENTAL_CONTEXT", "0")
	if incrementalContextEnabled() {
		t.Fatal("expected disabled with HAWK_INCREMENTAL_CONTEXT=0")
	}
}
