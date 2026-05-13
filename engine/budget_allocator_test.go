package engine

import (
	"strings"
	"sync"
	"testing"
)

func TestNewBudgetAllocator(t *testing.T) {
	ba := NewBudgetAllocator(128000, 16000)
	if ba.TotalBudget != 128000 {
		t.Errorf("TotalBudget = %d, want 128000", ba.TotalBudget)
	}
	if ba.OutputReserve != 16000 {
		t.Errorf("OutputReserve = %d, want 16000", ba.OutputReserve)
	}
	if ba.Allocations == nil {
		t.Fatal("Allocations map is nil")
	}
}

func TestRegister(t *testing.T) {
	ba := NewBudgetAllocator(100000, 10000)
	ba.Register("test", 500, 5000, 3, true)

	alloc, ok := ba.Allocations["test"]
	if !ok {
		t.Fatal("allocation 'test' not found after Register")
	}
	if alloc.Name != "test" {
		t.Errorf("Name = %q, want %q", alloc.Name, "test")
	}
	if alloc.MinTokens != 500 {
		t.Errorf("MinTokens = %d, want 500", alloc.MinTokens)
	}
	if alloc.MaxTokens != 5000 {
		t.Errorf("MaxTokens = %d, want 5000", alloc.MaxTokens)
	}
	if alloc.Priority != 3 {
		t.Errorf("Priority = %d, want 3", alloc.Priority)
	}
	if !alloc.Flexible {
		t.Error("Flexible = false, want true")
	}
}

func TestRegisterPrioritySorting(t *testing.T) {
	ba := NewBudgetAllocator(100000, 10000)
	ba.Register("low", 100, 1000, 5, true)
	ba.Register("high", 100, 1000, 1, false)
	ba.Register("mid", 100, 1000, 3, true)

	if len(ba.Priority) != 3 {
		t.Fatalf("Priority len = %d, want 3", len(ba.Priority))
	}
	if ba.Priority[0] != "high" {
		t.Errorf("Priority[0] = %q, want %q", ba.Priority[0], "high")
	}
	if ba.Priority[1] != "mid" {
		t.Errorf("Priority[1] = %q, want %q", ba.Priority[1], "mid")
	}
	if ba.Priority[2] != "low" {
		t.Errorf("Priority[2] = %q, want %q", ba.Priority[2], "low")
	}
}

func TestAllocateBasic(t *testing.T) {
	ba := NewBudgetAllocator(100000, 10000)
	// Distributable: 90000
	ba.Register("a", 1000, 5000, 1, false)
	ba.Register("b", 2000, 10000, 2, true)
	ba.Register("c", 500, 3000, 3, true)

	result := ba.Allocate()

	// All should get at least their minimum.
	if result["a"] < 1000 {
		t.Errorf("a = %d, want >= 1000", result["a"])
	}
	if result["b"] < 2000 {
		t.Errorf("b = %d, want >= 2000", result["b"])
	}
	if result["c"] < 500 {
		t.Errorf("c = %d, want >= 500", result["c"])
	}

	// None should exceed max.
	if result["a"] > 5000 {
		t.Errorf("a = %d, want <= 5000", result["a"])
	}
	if result["b"] > 10000 {
		t.Errorf("b = %d, want <= 10000", result["b"])
	}
	if result["c"] > 3000 {
		t.Errorf("c = %d, want <= 3000", result["c"])
	}

	// Total should not exceed distributable.
	total := 0
	for _, v := range result {
		total += v
	}
	if total > 90000 {
		t.Errorf("total allocated = %d, exceeds distributable 90000", total)
	}
}

func TestAllocateWithUnlimited(t *testing.T) {
	ba := NewBudgetAllocator(50000, 5000)
	// Distributable: 45000
	ba.Register("fixed", 1000, 2000, 1, false)
	ba.Register("unlimited", 1000, 0, 2, false) // max=0 means unlimited

	result := ba.Allocate()

	// Fixed should be at max (2000).
	if result["fixed"] != 2000 {
		t.Errorf("fixed = %d, want 2000", result["fixed"])
	}

	// Unlimited should get the rest: 45000 - 2000 = 43000.
	if result["unlimited"] != 43000 {
		t.Errorf("unlimited = %d, want 43000", result["unlimited"])
	}
}

func TestAllocateMinimumExceedsBudget(t *testing.T) {
	ba := NewBudgetAllocator(1000, 500)
	// Distributable: 500
	ba.Register("a", 300, 1000, 1, false)
	ba.Register("b", 300, 1000, 2, false)

	result := ba.Allocate()

	// a gets 300 (full min), b gets 200 (remaining).
	if result["a"] != 300 {
		t.Errorf("a = %d, want 300", result["a"])
	}
	if result["b"] != 200 {
		t.Errorf("b = %d, want 200", result["b"])
	}
}

func TestAllocateUpdatesCurrentTokens(t *testing.T) {
	ba := NewBudgetAllocator(50000, 5000)
	ba.Register("x", 1000, 5000, 1, true)

	ba.Allocate()

	if ba.Allocations["x"].CurrentTokens != 5000 {
		t.Errorf("CurrentTokens = %d, want 5000", ba.Allocations["x"].CurrentTokens)
	}
}

func TestRequestMore(t *testing.T) {
	ba := NewBudgetAllocator(100000, 10000)
	ba.Register("high", 1000, 20000, 1, false)
	ba.Register("low", 5000, 20000, 3, true)

	// Set up current tokens manually for this test.
	ba.Allocations["high"].CurrentTokens = 5000
	ba.Allocations["low"].CurrentTokens = 15000

	granted := ba.RequestMore("high", 8000)

	// Should steal from low (flexible, lower priority).
	// low can give up 15000 - 5000 = 10000, but we only need 8000.
	if granted != 8000 {
		t.Errorf("granted = %d, want 8000", granted)
	}
	if ba.Allocations["high"].CurrentTokens != 13000 {
		t.Errorf("high.CurrentTokens = %d, want 13000", ba.Allocations["high"].CurrentTokens)
	}
	if ba.Allocations["low"].CurrentTokens != 7000 {
		t.Errorf("low.CurrentTokens = %d, want 7000", ba.Allocations["low"].CurrentTokens)
	}
}

func TestRequestMoreDoesNotStealFromHigherPriority(t *testing.T) {
	ba := NewBudgetAllocator(100000, 10000)
	ba.Register("high", 1000, 20000, 1, true)
	ba.Register("low", 1000, 20000, 3, true)

	ba.Allocations["high"].CurrentTokens = 10000
	ba.Allocations["low"].CurrentTokens = 5000

	// low tries to request more — should NOT steal from high.
	granted := ba.RequestMore("low", 5000)
	if granted != 0 {
		t.Errorf("granted = %d, want 0 (should not steal from higher priority)", granted)
	}
}

func TestRequestMoreDoesNotStealFromInflexible(t *testing.T) {
	ba := NewBudgetAllocator(100000, 10000)
	ba.Register("high", 1000, 20000, 1, false)
	ba.Register("mid", 5000, 20000, 2, false) // not flexible
	ba.Register("low", 5000, 20000, 3, true)

	ba.Allocations["high"].CurrentTokens = 5000
	ba.Allocations["mid"].CurrentTokens = 15000
	ba.Allocations["low"].CurrentTokens = 8000

	granted := ba.RequestMore("high", 10000)

	// Can only steal from low (flexible, lower priority).
	// low can give 8000 - 5000 = 3000.
	if granted != 3000 {
		t.Errorf("granted = %d, want 3000", granted)
	}
	// mid should be untouched.
	if ba.Allocations["mid"].CurrentTokens != 15000 {
		t.Errorf("mid.CurrentTokens = %d, want 15000", ba.Allocations["mid"].CurrentTokens)
	}
}

func TestRequestMoreRespectsMax(t *testing.T) {
	ba := NewBudgetAllocator(100000, 10000)
	ba.Register("a", 1000, 6000, 1, false)
	ba.Register("b", 1000, 20000, 3, true)

	ba.Allocations["a"].CurrentTokens = 5000
	ba.Allocations["b"].CurrentTokens = 15000

	// a requests 5000 more but its max is 6000, only room for 1000.
	granted := ba.RequestMore("a", 5000)
	if granted != 1000 {
		t.Errorf("granted = %d, want 1000 (capped by max)", granted)
	}
}

func TestRequestMoreUnknownName(t *testing.T) {
	ba := NewBudgetAllocator(100000, 10000)
	granted := ba.RequestMore("nonexistent", 5000)
	if granted != 0 {
		t.Errorf("granted = %d, want 0 for unknown allocation", granted)
	}
}

func TestRelease(t *testing.T) {
	ba := NewBudgetAllocator(100000, 10000)
	ba.Register("test", 500, 10000, 1, true)
	ba.Allocations["test"].CurrentTokens = 8000

	ba.Release("test", 3000)
	if ba.Allocations["test"].CurrentTokens != 5000 {
		t.Errorf("CurrentTokens = %d, want 5000", ba.Allocations["test"].CurrentTokens)
	}
}

func TestReleaseDoesNotGoBelowMin(t *testing.T) {
	ba := NewBudgetAllocator(100000, 10000)
	ba.Register("test", 500, 10000, 1, true)
	ba.Allocations["test"].CurrentTokens = 1000

	ba.Release("test", 900)
	if ba.Allocations["test"].CurrentTokens != 500 {
		t.Errorf("CurrentTokens = %d, want 500 (min)", ba.Allocations["test"].CurrentTokens)
	}
}

func TestReleaseUnknownName(t *testing.T) {
	ba := NewBudgetAllocator(100000, 10000)
	// Should not panic.
	ba.Release("nonexistent", 1000)
}

func TestGetAvailable(t *testing.T) {
	ba := NewBudgetAllocator(100000, 10000)
	ba.Register("a", 1000, 5000, 1, false)
	ba.Register("b", 2000, 8000, 2, true)

	ba.Allocations["a"].CurrentTokens = 3000
	ba.Allocations["b"].CurrentTokens = 6000

	// Distributable = 90000, used = 9000, available = 81000.
	avail := ba.GetAvailable()
	if avail != 81000 {
		t.Errorf("GetAvailable = %d, want 81000", avail)
	}
}

func TestGetAvailableNoAllocations(t *testing.T) {
	ba := NewBudgetAllocator(50000, 5000)
	avail := ba.GetAvailable()
	if avail != 45000 {
		t.Errorf("GetAvailable = %d, want 45000", avail)
	}
}

func TestRebalance(t *testing.T) {
	ba := NewBudgetAllocator(100000, 10000)
	ba.Register("hungry", 1000, 20000, 1, true)
	ba.Register("wasteful", 500, 10000, 3, true)

	// hungry is at capacity (usage 95%), wasteful is underutilized (usage 20%).
	ba.Allocations["hungry"].CurrentTokens = 10000
	ba.Allocations["hungry"].Usage = 0.95
	ba.Allocations["wasteful"].CurrentTokens = 8000
	ba.Allocations["wasteful"].Usage = 0.20

	ba.Rebalance()

	// wasteful should have shrunk.
	if ba.Allocations["wasteful"].CurrentTokens >= 8000 {
		t.Errorf("wasteful.CurrentTokens = %d, should have decreased from 8000",
			ba.Allocations["wasteful"].CurrentTokens)
	}
	// hungry should have grown.
	if ba.Allocations["hungry"].CurrentTokens <= 10000 {
		t.Errorf("hungry.CurrentTokens = %d, should have increased from 10000",
			ba.Allocations["hungry"].CurrentTokens)
	}
}

func TestRebalanceNoChangeWhenAllBalanced(t *testing.T) {
	ba := NewBudgetAllocator(100000, 10000)
	ba.Register("a", 1000, 5000, 1, true)

	ba.Allocations["a"].CurrentTokens = 3000
	ba.Allocations["a"].Usage = 0.7 // between 0.5 and 0.9 — neither underutilized nor at capacity

	ba.Rebalance()

	if ba.Allocations["a"].CurrentTokens != 3000 {
		t.Errorf("a.CurrentTokens = %d, want 3000 (no change)", ba.Allocations["a"].CurrentTokens)
	}
}

func TestRebalanceInflexibleNotShrunk(t *testing.T) {
	ba := NewBudgetAllocator(100000, 10000)
	ba.Register("fixed", 1000, 10000, 1, false)
	ba.Register("hungry", 1000, 20000, 2, true)

	ba.Allocations["fixed"].CurrentTokens = 8000
	ba.Allocations["fixed"].Usage = 0.1 // very underutilized but NOT flexible
	ba.Allocations["hungry"].CurrentTokens = 5000
	ba.Allocations["hungry"].Usage = 0.95

	ba.Rebalance()

	// Fixed should remain unchanged.
	if ba.Allocations["fixed"].CurrentTokens != 8000 {
		t.Errorf("fixed.CurrentTokens = %d, want 8000 (inflexible)", ba.Allocations["fixed"].CurrentTokens)
	}
}

func TestDefaultAllocations(t *testing.T) {
	ba := NewBudgetAllocator(128000, 16000)
	ba.DefaultAllocations()

	expected := []string{
		"system_prompt", "memory", "conversation",
		"repo_map", "tool_results", "readonly_ctx", "goals",
	}

	for _, name := range expected {
		if _, ok := ba.Allocations[name]; !ok {
			t.Errorf("missing default allocation: %s", name)
		}
	}

	// Verify conversation is unlimited.
	conv := ba.Allocations["conversation"]
	if conv.MaxTokens != 0 {
		t.Errorf("conversation.MaxTokens = %d, want 0 (unlimited)", conv.MaxTokens)
	}

	// Verify priority ordering.
	if ba.Allocations["system_prompt"].Priority != 1 {
		t.Errorf("system_prompt priority = %d, want 1", ba.Allocations["system_prompt"].Priority)
	}
	if ba.Allocations["goals"].Priority != 7 {
		t.Errorf("goals priority = %d, want 7", ba.Allocations["goals"].Priority)
	}
}

func TestDefaultAllocationsAllocate(t *testing.T) {
	ba := NewBudgetAllocator(128000, 16000)
	ba.DefaultAllocations()

	result := ba.Allocate()

	total := 0
	for _, v := range result {
		total += v
	}

	distributable := 128000 - 16000
	if total > distributable {
		t.Errorf("total allocated %d exceeds distributable %d", total, distributable)
	}

	// Every allocation should have at least its minimum.
	for name, alloc := range ba.Allocations {
		if result[name] < alloc.MinTokens {
			t.Errorf("%s: allocated %d < min %d", name, result[name], alloc.MinTokens)
		}
	}
}

func TestFormatBudget(t *testing.T) {
	ba := NewBudgetAllocator(100000, 10000)
	ba.Register("system_prompt", 2000, 8000, 1, false)
	ba.Register("memory", 500, 4000, 2, true)
	ba.Allocate()

	output := ba.FormatBudget()

	if !strings.Contains(output, "Token Budget Allocator") {
		t.Error("FormatBudget missing header")
	}
	if !strings.Contains(output, "system_prompt") {
		t.Error("FormatBudget missing system_prompt")
	}
	if !strings.Contains(output, "memory") {
		t.Error("FormatBudget missing memory")
	}
	if !strings.Contains(output, "TOTAL USED") {
		t.Error("FormatBudget missing TOTAL USED")
	}
	if !strings.Contains(output, "AVAILABLE") {
		t.Error("FormatBudget missing AVAILABLE")
	}
}

func TestWouldExceed(t *testing.T) {
	ba := NewBudgetAllocator(100000, 10000)
	ba.Register("test", 500, 5000, 1, true)
	ba.Allocations["test"].CurrentTokens = 4000

	if !ba.WouldExceed("test", 1500) {
		t.Error("WouldExceed(test, 1500) = false, want true (4000+1500 > 5000)")
	}
	if ba.WouldExceed("test", 500) {
		t.Error("WouldExceed(test, 500) = true, want false (4000+500 <= 5000)")
	}
	if ba.WouldExceed("test", 1000) {
		t.Error("WouldExceed(test, 1000) = true, want false (4000+1000 == 5000)")
	}
}

func TestWouldExceedUnlimited(t *testing.T) {
	ba := NewBudgetAllocator(100000, 10000)
	ba.Register("unlimited", 500, 0, 1, false)
	ba.Allocations["unlimited"].CurrentTokens = 50000

	if ba.WouldExceed("unlimited", 100000) {
		t.Error("WouldExceed for unlimited allocation should always be false")
	}
}

func TestWouldExceedUnknown(t *testing.T) {
	ba := NewBudgetAllocator(100000, 10000)
	if !ba.WouldExceed("nonexistent", 1) {
		t.Error("WouldExceed for unknown allocation should be true")
	}
}

func TestSuggestCompactionLowUsage(t *testing.T) {
	ba := NewBudgetAllocator(100000, 10000)
	ba.Register("a", 1000, 5000, 1, true)
	ba.Allocations["a"].CurrentTokens = 3000

	// Total used = 3000, distributable = 90000, usage = 3.3% — no compaction.
	result := ba.SuggestCompaction()
	if !strings.Contains(result, "no compaction needed") {
		t.Errorf("expected no compaction needed, got: %s", result)
	}
}

func TestSuggestCompactionHighUsage(t *testing.T) {
	ba := NewBudgetAllocator(100000, 10000)
	// Distributable = 90000. Fill to >80%.
	ba.Register("conversation", 4000, 0, 1, false)
	ba.Register("tool_results", 2000, 20000, 2, true)
	ba.Register("readonly_ctx", 500, 10000, 3, true)

	ba.Allocations["conversation"].CurrentTokens = 50000
	ba.Allocations["tool_results"].CurrentTokens = 18000
	ba.Allocations["readonly_ctx"].CurrentTokens = 8000
	// Total used = 76000, 76000/90000 = 84.4%

	result := ba.SuggestCompaction()
	if strings.Contains(result, "no compaction needed") {
		t.Errorf("expected compaction suggestions, got: %s", result)
	}
	if !strings.Contains(result, "Compact") {
		t.Errorf("expected 'Compact' in suggestions, got: %s", result)
	}
	// Should suggest compacting flexible allocations.
	if !strings.Contains(result, "tool_results") && !strings.Contains(result, "readonly_ctx") {
		t.Errorf("expected flexible allocations in suggestions, got: %s", result)
	}
}

func TestBudgetAllocatorConcurrentAccess(t *testing.T) {
	ba := NewBudgetAllocator(100000, 10000)
	ba.DefaultAllocations()
	ba.Allocate()

	var wg sync.WaitGroup
	for i := 0; i < 50; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			ba.Allocate()
		}()
	}
	for i := 0; i < 50; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			ba.GetAvailable()
		}()
	}
	for i := 0; i < 20; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			ba.RequestMore("tool_results", 100)
		}()
	}
	for i := 0; i < 20; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			ba.Release("readonly_ctx", 50)
		}()
	}
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			ba.Rebalance()
		}()
	}
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			ba.FormatBudget()
		}()
	}
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			ba.WouldExceed("conversation", 1000)
		}()
	}
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			ba.SuggestCompaction()
		}()
	}
	wg.Wait()
}

func TestAllocateRespectsPriorityOrder(t *testing.T) {
	ba := NewBudgetAllocator(10000, 1000)
	// Distributable: 9000
	// Total mins: 500 + 500 + 500 = 1500, remaining: 7500
	// After priority allocation: high gets min(7500, 4500) = 4500, remaining 3000
	// mid gets min(3000, 4500) = 3000, remaining 0
	// low gets 0 extra.
	ba.Register("high", 500, 5000, 1, true)
	ba.Register("mid", 500, 5000, 2, true)
	ba.Register("low", 500, 5000, 3, true)

	result := ba.Allocate()

	// High priority should get more than low priority.
	if result["high"] <= result["low"] {
		t.Errorf("high(%d) should get more than low(%d)", result["high"], result["low"])
	}
	if result["high"] != 5000 {
		t.Errorf("high = %d, want 5000 (full max)", result["high"])
	}
}

func TestAllocateEmptyAllocator(t *testing.T) {
	ba := NewBudgetAllocator(100000, 10000)
	result := ba.Allocate()
	if len(result) != 0 {
		t.Errorf("expected empty result for allocator with no registrations, got %d entries", len(result))
	}
}

func TestRequestMoreZeroNeeded(t *testing.T) {
	ba := NewBudgetAllocator(100000, 10000)
	ba.Register("test", 1000, 5000, 1, true)
	ba.Allocations["test"].CurrentTokens = 3000

	granted := ba.RequestMore("test", 0)
	if granted != 0 {
		t.Errorf("granted = %d, want 0 for zero request", granted)
	}
}

func TestReleaseZeroAmount(t *testing.T) {
	ba := NewBudgetAllocator(100000, 10000)
	ba.Register("test", 500, 5000, 1, true)
	ba.Allocations["test"].CurrentTokens = 3000

	ba.Release("test", 0)
	if ba.Allocations["test"].CurrentTokens != 3000 {
		t.Errorf("CurrentTokens = %d, want 3000 (no change)", ba.Allocations["test"].CurrentTokens)
	}
}

func TestFullWorkflow(t *testing.T) {
	// Simulate a full workflow: create allocator, register defaults, allocate,
	// use some budget, request more, rebalance.
	ba := NewBudgetAllocator(128000, 16000)
	ba.DefaultAllocations()

	// Initial allocation.
	result := ba.Allocate()
	if len(result) != 7 {
		t.Fatalf("expected 7 allocations, got %d", len(result))
	}

	// Simulate usage.
	ba.Allocations["conversation"].Usage = 0.95
	ba.Allocations["tool_results"].Usage = 0.3
	ba.Allocations["readonly_ctx"].Usage = 0.2
	ba.Allocations["system_prompt"].Usage = 0.8
	ba.Allocations["memory"].Usage = 0.6
	ba.Allocations["repo_map"].Usage = 0.4
	ba.Allocations["goals"].Usage = 0.5

	// Request more for conversation.
	granted := ba.RequestMore("conversation", 5000)
	if granted <= 0 {
		t.Error("expected to steal some tokens for conversation")
	}

	// Rebalance.
	ba.Rebalance()

	// Check budget is still valid.
	_ = ba.FormatBudget()
	_ = ba.SuggestCompaction()
	_ = ba.GetAvailable()
}
