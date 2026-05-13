package engine

import (
	"strings"
	"sync"
	"testing"
	"time"
)

func TestNewContextDecay(t *testing.T) {
	t.Run("custom half-life", func(t *testing.T) {
		cd := NewContextDecay(10 * time.Minute)
		if cd.HalfLife != 10*time.Minute {
			t.Errorf("expected half-life 10m, got %v", cd.HalfLife)
		}
		if cd.MinWeight != 0.1 {
			t.Errorf("expected min weight 0.1, got %f", cd.MinWeight)
		}
	})

	t.Run("default half-life on zero", func(t *testing.T) {
		cd := NewContextDecay(0)
		if cd.HalfLife != 30*time.Minute {
			t.Errorf("expected default 30m, got %v", cd.HalfLife)
		}
	})

	t.Run("default half-life on negative", func(t *testing.T) {
		cd := NewContextDecay(-5 * time.Minute)
		if cd.HalfLife != 30*time.Minute {
			t.Errorf("expected default 30m, got %v", cd.HalfLife)
		}
	})
}

func TestAdd(t *testing.T) {
	cd := NewContextDecay(30 * time.Minute)

	id := cd.Add("Project uses JWT auth", "architecture", 50)
	if id == "" {
		t.Fatal("expected non-empty ID")
	}
	if !strings.HasPrefix(id, "ctx_") {
		t.Errorf("expected ID prefix 'ctx_', got %s", id)
	}
	if len(cd.Entries) != 1 {
		t.Fatalf("expected 1 entry, got %d", len(cd.Entries))
	}

	entry := cd.Entries[0]
	if entry.Content != "Project uses JWT auth" {
		t.Errorf("wrong content: %s", entry.Content)
	}
	if entry.Category != "architecture" {
		t.Errorf("wrong category: %s", entry.Category)
	}
	if entry.Tokens != 50 {
		t.Errorf("wrong tokens: %d", entry.Tokens)
	}
	if entry.Weight != 1.0 {
		t.Errorf("expected initial weight 1.0, got %f", entry.Weight)
	}
	if entry.Pinned {
		t.Error("new entry should not be pinned")
	}
}

func TestGet(t *testing.T) {
	cd := NewContextDecay(30 * time.Minute)

	id := cd.Add("test content", "general", 10)

	entry, weight := cd.Get(id)
	if entry == nil {
		t.Fatal("expected entry, got nil")
	}
	if entry.Content != "test content" {
		t.Errorf("wrong content: %s", entry.Content)
	}
	// Weight should be very close to 1.0 since we just added it
	if weight < 0.99 {
		t.Errorf("expected weight ~1.0, got %f", weight)
	}

	// Non-existent ID
	entry, weight = cd.Get("nonexistent")
	if entry != nil {
		t.Error("expected nil for non-existent ID")
	}
	if weight != 0 {
		t.Errorf("expected weight 0 for non-existent, got %f", weight)
	}
}

func TestApplyDecay(t *testing.T) {
	cd := NewContextDecay(1 * time.Millisecond) // Very short half-life for testing

	cd.Add("old content", "general", 10)

	// Wait for several half-lives
	time.Sleep(5 * time.Millisecond)

	cd.ApplyDecay()

	if len(cd.Entries) == 0 {
		t.Fatal("entries should still exist")
	}

	// After 5 half-lives, weight should be very low (0.5^5 = 0.03125)
	// But clamped to MinWeight (0.1)
	if cd.Entries[0].Weight > 0.11 {
		t.Errorf("expected decayed weight near MinWeight, got %f", cd.Entries[0].Weight)
	}
}

func TestApplyDecay_Pinned(t *testing.T) {
	cd := NewContextDecay(1 * time.Millisecond)

	id := cd.Add("pinned content", "core", 20)
	cd.Pin(id)

	time.Sleep(5 * time.Millisecond)
	cd.ApplyDecay()

	if cd.Entries[0].Weight != 1.0 {
		t.Errorf("pinned entry weight should be 1.0, got %f", cd.Entries[0].Weight)
	}
}

func TestAccess(t *testing.T) {
	cd := NewContextDecay(1 * time.Millisecond)

	id := cd.Add("accessed content", "general", 10)

	// Let it decay
	time.Sleep(3 * time.Millisecond)
	cd.ApplyDecay()

	weightBefore := cd.Entries[0].Weight

	// Access should boost weight
	cd.Access(id)

	if cd.Entries[0].Weight <= weightBefore {
		t.Errorf("access should boost weight: before=%f, after=%f", weightBefore, cd.Entries[0].Weight)
	}
	if cd.Entries[0].AccessCount != 1 {
		t.Errorf("expected access count 1, got %d", cd.Entries[0].AccessCount)
	}
}

func TestAccess_NonExistent(t *testing.T) {
	cd := NewContextDecay(30 * time.Minute)
	// Should not panic
	cd.Access("nonexistent")
}

func TestPin(t *testing.T) {
	cd := NewContextDecay(30 * time.Minute)

	id := cd.Add("pin me", "core", 15)
	cd.Pin(id)

	entry, weight := cd.Get(id)
	if !entry.Pinned {
		t.Error("entry should be pinned")
	}
	if weight != 1.0 {
		t.Errorf("pinned weight should be 1.0, got %f", weight)
	}
}

func TestUnpin(t *testing.T) {
	cd := NewContextDecay(30 * time.Minute)

	id := cd.Add("unpin me", "core", 15)
	cd.Pin(id)
	cd.Unpin(id)

	entry, _ := cd.Get(id)
	if entry.Pinned {
		t.Error("entry should not be pinned after unpin")
	}
}

func TestGetTopN(t *testing.T) {
	cd := NewContextDecay(30 * time.Minute)

	cd.Add("first", "general", 10)
	cd.Add("second", "general", 10)
	cd.Add("third", "general", 10)

	top := cd.GetTopN(2)
	if len(top) != 2 {
		t.Fatalf("expected 2 entries, got %d", len(top))
	}

	// All are fresh so weights should all be ~1.0
	for _, entry := range top {
		if entry.Weight < 0.99 {
			t.Errorf("expected fresh entry weight ~1.0, got %f", entry.Weight)
		}
	}
}

func TestGetTopN_MoreThanAvailable(t *testing.T) {
	cd := NewContextDecay(30 * time.Minute)
	cd.Add("only one", "general", 10)

	top := cd.GetTopN(5)
	if len(top) != 1 {
		t.Fatalf("expected 1 entry, got %d", len(top))
	}
}

func TestGetTopN_PinnedFirst(t *testing.T) {
	cd := NewContextDecay(1 * time.Millisecond)

	id1 := cd.Add("decaying", "general", 10)
	_ = id1
	time.Sleep(5 * time.Millisecond)

	id2 := cd.Add("pinned", "core", 10)
	cd.Pin(id2)

	top := cd.GetTopN(2)
	if top[0].Content != "pinned" {
		t.Errorf("expected pinned entry first, got %s", top[0].Content)
	}
}

func TestGetByBudget(t *testing.T) {
	cd := NewContextDecay(30 * time.Minute)

	cd.Add("small entry", "general", 10)
	cd.Add("medium entry", "general", 50)
	cd.Add("large entry", "general", 100)

	// Budget of 60 should fit small (10) + medium (50) = 60
	result := cd.GetByBudget(60)

	totalTokens := 0
	for _, e := range result {
		totalTokens += e.Tokens
	}
	if totalTokens > 60 {
		t.Errorf("exceeded budget: %d tokens > 60", totalTokens)
	}
	if len(result) < 2 {
		t.Errorf("expected at least 2 entries within budget, got %d", len(result))
	}
}

func TestGetByBudget_Empty(t *testing.T) {
	cd := NewContextDecay(30 * time.Minute)
	result := cd.GetByBudget(100)
	if len(result) != 0 {
		t.Errorf("expected 0 entries, got %d", len(result))
	}
}

func TestGetByBudget_ZeroBudget(t *testing.T) {
	cd := NewContextDecay(30 * time.Minute)
	cd.Add("content", "general", 10)

	result := cd.GetByBudget(0)
	if len(result) != 0 {
		t.Errorf("expected 0 entries with zero budget, got %d", len(result))
	}
}

func TestPrune(t *testing.T) {
	cd := NewContextDecay(1 * time.Millisecond)

	cd.Add("will decay", "general", 10)
	id2 := cd.Add("will be pinned", "core", 10)
	cd.Pin(id2)

	time.Sleep(10 * time.Millisecond)

	removed := cd.Prune(0.5)
	if removed != 1 {
		t.Errorf("expected 1 removed, got %d", removed)
	}
	if len(cd.Entries) != 1 {
		t.Errorf("expected 1 remaining, got %d", len(cd.Entries))
	}
	if cd.Entries[0].Content != "will be pinned" {
		t.Errorf("wrong entry survived: %s", cd.Entries[0].Content)
	}
}

func TestPrune_NoneRemoved(t *testing.T) {
	cd := NewContextDecay(30 * time.Minute)
	cd.Add("fresh", "general", 10)

	removed := cd.Prune(0.5)
	if removed != 0 {
		t.Errorf("expected 0 removed, got %d", removed)
	}
}

func TestBuildContext(t *testing.T) {
	cd := NewContextDecay(30 * time.Minute)

	cd.Add("Project uses JWT auth", "architecture", 30)
	id2 := cd.Add("Code style: tabs, 100 char lines", "style", 25)
	cd.Pin(id2)

	result := cd.BuildContext(100)
	if !strings.Contains(result, "Context (decayed):") {
		t.Error("missing header")
	}
	if !strings.Contains(result, "Project uses JWT auth") {
		t.Error("missing entry content")
	}
}

func TestFormatEntries(t *testing.T) {
	cd := NewContextDecay(30 * time.Minute)

	entries := []DecayEntry{
		{Content: "Pinned entry", Weight: 1.0, Pinned: true},
		{Content: "Normal entry", Weight: 0.85, Pinned: false},
		{Content: "Fading entry", Weight: 0.12, Pinned: false},
	}

	result := cd.FormatEntries(entries)

	if !strings.Contains(result, "Context (decayed):") {
		t.Error("missing header")
	}
	if !strings.Contains(result, "\U0001f4cc") {
		t.Error("missing pin emoji for pinned entry")
	}
	if !strings.Contains(result, "(fading)") {
		t.Error("missing fading indicator")
	}
	if !strings.Contains(result, "[1.00]") {
		t.Error("missing weight for pinned entry")
	}
	if !strings.Contains(result, "[0.85]") {
		t.Error("missing weight for normal entry")
	}
}

func TestFormatEntries_Empty(t *testing.T) {
	cd := NewContextDecay(30 * time.Minute)
	result := cd.FormatEntries(nil)
	if !strings.Contains(result, "(empty)") {
		t.Error("expected empty indicator")
	}
}

func TestStats(t *testing.T) {
	cd := NewContextDecay(30 * time.Minute)

	stats := cd.Stats()
	if stats.TotalEntries != 0 {
		t.Errorf("expected 0 entries, got %d", stats.TotalEntries)
	}

	cd.Add("first", "general", 10)
	time.Sleep(1 * time.Millisecond)
	id2 := cd.Add("second", "core", 20)
	cd.Pin(id2)
	cd.Add("third", "general", 30)

	stats = cd.Stats()
	if stats.TotalEntries != 3 {
		t.Errorf("expected 3 entries, got %d", stats.TotalEntries)
	}
	if stats.PinnedCount != 1 {
		t.Errorf("expected 1 pinned, got %d", stats.PinnedCount)
	}
	if stats.TotalTokens != 60 {
		t.Errorf("expected 60 total tokens, got %d", stats.TotalTokens)
	}
	if stats.AvgWeight < 0.9 {
		t.Errorf("expected high avg weight for fresh entries, got %f", stats.AvgWeight)
	}
	if stats.Oldest.After(stats.Newest) {
		t.Error("oldest should be before newest")
	}
}

func TestConcurrentAccess(t *testing.T) {
	cd := NewContextDecay(30 * time.Minute)

	// Add some entries
	ids := make([]string, 10)
	for i := 0; i < 10; i++ {
		ids[i] = cd.Add("content", "general", 10)
	}

	// Concurrent reads and writes
	var wg sync.WaitGroup
	for i := 0; i < 50; i++ {
		wg.Add(1)
		go func(n int) {
			defer wg.Done()
			switch n % 5 {
			case 0:
				cd.Add("new content", "general", 5)
			case 1:
				cd.Get(ids[n%10])
			case 2:
				cd.Access(ids[n%10])
			case 3:
				cd.ApplyDecay()
			case 4:
				cd.GetTopN(3)
			}
		}(i)
	}
	wg.Wait()
}

func TestDecayOverTime(t *testing.T) {
	cd := NewContextDecay(10 * time.Millisecond)

	id := cd.Add("decaying content", "general", 10)

	// Immediately, weight should be ~1.0
	_, w0 := cd.Get(id)
	if w0 < 0.99 {
		t.Errorf("initial weight should be ~1.0, got %f", w0)
	}

	// After one half-life, weight should be ~0.5
	time.Sleep(10 * time.Millisecond)
	_, w1 := cd.Get(id)
	if w1 > 0.6 || w1 < 0.4 {
		t.Errorf("after one half-life, weight should be ~0.5, got %f", w1)
	}

	// After two half-lives, weight should be ~0.25
	time.Sleep(10 * time.Millisecond)
	_, w2 := cd.Get(id)
	if w2 > 0.35 || w2 < 0.15 {
		t.Errorf("after two half-lives, weight should be ~0.25, got %f", w2)
	}
}

func TestMinWeightFloor(t *testing.T) {
	cd := NewContextDecay(1 * time.Millisecond)
	cd.MinWeight = 0.05

	id := cd.Add("content", "general", 10)

	// Wait many half-lives
	time.Sleep(20 * time.Millisecond)

	_, w := cd.Get(id)
	if w < 0.05 {
		t.Errorf("weight should not go below MinWeight 0.05, got %f", w)
	}
}

func TestMultipleAdds_UniqueIDs(t *testing.T) {
	cd := NewContextDecay(30 * time.Minute)

	ids := make(map[string]bool)
	for i := 0; i < 100; i++ {
		id := cd.Add("content", "general", 10)
		if ids[id] {
			t.Fatalf("duplicate ID generated: %s", id)
		}
		ids[id] = true
	}
}

func TestGetByBudget_SkipsLargeEntries(t *testing.T) {
	cd := NewContextDecay(30 * time.Minute)

	cd.Add("huge entry", "general", 1000)
	cd.Add("tiny entry", "general", 5)

	result := cd.GetByBudget(50)

	// Should only contain the tiny entry
	found := false
	for _, e := range result {
		if e.Content == "tiny entry" {
			found = true
		}
		if e.Content == "huge entry" {
			t.Error("huge entry should not fit in budget")
		}
	}
	if !found {
		t.Error("tiny entry should fit in budget")
	}
}
