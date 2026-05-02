package engine

import (
	"strings"
	"testing"
)

func TestNewContextBudget_SmallModel(t *testing.T) {
	b := NewContextBudget(32_000)

	if b.Total != 32_000 {
		t.Errorf("expected Total=32000, got %d", b.Total)
	}

	// Fixed allocations should hit their floors for a small model.
	if b.SystemPrompt < 3000 {
		t.Errorf("SystemPrompt should be at least 3000 for 32K model, got %d", b.SystemPrompt)
	}
	if b.ToolDefs < 2000 {
		t.Errorf("ToolDefs should be at least 2000 for 32K model, got %d", b.ToolDefs)
	}
	if b.RepoMap < 2000 {
		t.Errorf("RepoMap should be at least 2000, got %d", b.RepoMap)
	}
	if b.Memory < 1000 {
		t.Errorf("Memory should be at least 1000, got %d", b.Memory)
	}
	if b.Workspace < 300 {
		t.Errorf("Workspace should be at least 300, got %d", b.Workspace)
	}

	// Everything should sum to Total.
	sum := b.SystemPrompt + b.ToolDefs + b.RepoMap + b.Memory + b.Workspace +
		b.PreloadedFiles + b.Conversation + b.OutputReserve + b.SafetyMargin
	if sum != b.Total {
		t.Errorf("allocations should sum to Total (%d), got %d", b.Total, sum)
	}
}

func TestNewContextBudget_LargeModel(t *testing.T) {
	b := NewContextBudget(200_000)

	if b.Total != 200_000 {
		t.Errorf("expected Total=200000, got %d", b.Total)
	}

	// Fixed allocations should hit their ceilings for a large model.
	if b.SystemPrompt > 5000 {
		t.Errorf("SystemPrompt should be capped at 5000, got %d", b.SystemPrompt)
	}
	if b.ToolDefs > 3000 {
		t.Errorf("ToolDefs should be capped at 3000, got %d", b.ToolDefs)
	}
	if b.RepoMap > 4000 {
		t.Errorf("RepoMap should be capped at 4000, got %d", b.RepoMap)
	}
	if b.Memory > 2000 {
		t.Errorf("Memory should be capped at 2000, got %d", b.Memory)
	}
	if b.Workspace > 500 {
		t.Errorf("Workspace should be capped at 500, got %d", b.Workspace)
	}
	if b.OutputReserve > 20000 {
		t.Errorf("OutputReserve should be capped at 20000, got %d", b.OutputReserve)
	}
	if b.SafetyMargin > 15000 {
		t.Errorf("SafetyMargin should be capped at 15000, got %d", b.SafetyMargin)
	}

	// Large model should have substantial PreloadedFiles budget.
	if b.PreloadedFiles < 20000 {
		t.Errorf("PreloadedFiles should be at least 20000 for 200K model, got %d", b.PreloadedFiles)
	}

	sum := b.SystemPrompt + b.ToolDefs + b.RepoMap + b.Memory + b.Workspace +
		b.PreloadedFiles + b.Conversation + b.OutputReserve + b.SafetyMargin
	if sum != b.Total {
		t.Errorf("allocations should sum to Total (%d), got %d", b.Total, sum)
	}
}

func TestAllocate_ShortConversation(t *testing.T) {
	b := NewContextBudget(200_000)

	// Short conversation: should get maximum file budget.
	alloc := b.Allocate(0)

	if alloc.PreloadedFiles != b.PreloadedFiles {
		t.Errorf("empty conversation should get max file budget (%d), got %d",
			b.PreloadedFiles, alloc.PreloadedFiles)
	}

	// Remaining should be ~0 (properly allocated).
	if alloc.Remaining < 0 || alloc.Remaining > 1 {
		t.Errorf("Remaining should be ~0, got %d", alloc.Remaining)
	}
}

func TestAllocate_LongConversation_ReducesFiles(t *testing.T) {
	b := NewContextBudget(200_000)

	shortAlloc := b.Allocate(1000)
	longAlloc := b.Allocate(80_000)

	if longAlloc.PreloadedFiles >= shortAlloc.PreloadedFiles {
		t.Errorf("long conversation should reduce file budget: short=%d, long=%d",
			shortAlloc.PreloadedFiles, longAlloc.PreloadedFiles)
	}

	if longAlloc.Conversation < shortAlloc.Conversation {
		t.Errorf("long conversation should have more conversation space: short=%d, long=%d",
			shortAlloc.Conversation, longAlloc.Conversation)
	}
}

func TestAllocate_SmallModel_TightBudgets(t *testing.T) {
	b := NewContextBudget(32_000)
	alloc := b.Allocate(0)

	// All fields should be positive.
	if alloc.SystemPrompt <= 0 || alloc.ToolDefs <= 0 || alloc.RepoMap <= 0 ||
		alloc.Memory <= 0 || alloc.Workspace <= 0 || alloc.OutputReserve <= 0 ||
		alloc.SafetyMargin <= 0 {
		t.Error("all fixed allocations should be positive for 32K model")
	}

	// PreloadedFiles and Conversation should both be positive.
	if alloc.PreloadedFiles <= 0 {
		t.Errorf("PreloadedFiles should be positive, got %d", alloc.PreloadedFiles)
	}
	if alloc.Conversation <= 0 {
		t.Errorf("Conversation should be positive, got %d", alloc.Conversation)
	}
}

func TestShouldCompact_BelowThreshold(t *testing.T) {
	b := NewContextBudget(200_000)
	alloc := b.Allocate(0)

	// At half the conversation allocation, should not need compaction.
	half := alloc.Conversation / 2
	if b.ShouldCompact(half) {
		t.Errorf("should not need compaction at %d tokens (allocation: %d)",
			half, alloc.Conversation)
	}
}

func TestShouldCompact_AboveThreshold(t *testing.T) {
	b := NewContextBudget(200_000)

	// Use a conversation size large enough to exceed allocation even after
	// the adaptive file budget shrinks to its minimum.
	// Get the allocation at a very high conversation size to find the ceiling.
	hugeConv := 180_000
	alloc := b.Allocate(hugeConv)
	over := alloc.Conversation + 5000
	if !b.ShouldCompact(over) {
		t.Errorf("should need compaction at %d tokens (allocation: %d)",
			over, alloc.Conversation)
	}
}

func TestShouldCompact_SmallModel_TriggersEarlier(t *testing.T) {
	small := NewContextBudget(32_000)
	large := NewContextBudget(200_000)

	smallAlloc := small.Allocate(0)
	largeAlloc := large.Allocate(0)

	// Small model should have a lower compaction threshold than large model.
	if smallAlloc.Conversation >= largeAlloc.Conversation {
		t.Errorf("small model conversation budget (%d) should be less than large model (%d)",
			smallAlloc.Conversation, largeAlloc.Conversation)
	}
}

func TestShouldCompact_DynamicThreshold(t *testing.T) {
	b := NewContextBudget(200_000)

	// As conversation grows, the allocation shifts. ShouldCompact accounts for this.
	// At a moderate conversation size, the threshold adapts.
	moderate := 50_000
	alloc := b.Allocate(moderate)
	if moderate > alloc.Conversation && !b.ShouldCompact(moderate) {
		t.Error("ShouldCompact should reflect the dynamic allocation")
	}
}

func TestFilesBudget_DecreasesWithConversation(t *testing.T) {
	b := NewContextBudget(200_000)

	fb0 := b.FilesBudget(0)
	fb20k := b.FilesBudget(20_000)
	fb80k := b.FilesBudget(80_000)

	if fb20k > fb0 {
		t.Errorf("files budget at 20K (%d) should be <= at 0 (%d)", fb20k, fb0)
	}
	if fb80k > fb20k {
		t.Errorf("files budget at 80K (%d) should be <= at 20K (%d)", fb80k, fb20k)
	}
}

func TestFilesBudget_NeverNegative(t *testing.T) {
	b := NewContextBudget(32_000)

	// Even with a huge conversation, file budget should not go negative.
	fb := b.FilesBudget(100_000)
	if fb < 0 {
		t.Errorf("file budget should never be negative, got %d", fb)
	}
}

func TestUsageReport_ContainsAllCategories(t *testing.T) {
	b := NewContextBudget(200_000)
	report := b.UsageReport(5000)

	expected := []string{
		"Context Budget",
		"System Prompt",
		"Tool Definitions",
		"Repo Map",
		"Memory",
		"Workspace",
		"Preloaded Files",
		"Conversation",
		"Output Reserve",
		"Safety Margin",
		"Remaining",
	}
	for _, e := range expected {
		if !strings.Contains(report, e) {
			t.Errorf("report missing %q:\n%s", e, report)
		}
	}
}

func TestUsageReport_ShowsPercentages(t *testing.T) {
	b := NewContextBudget(200_000)
	report := b.UsageReport(0)

	if !strings.Contains(report, "%") {
		t.Errorf("report should contain percentages:\n%s", report)
	}
}

func TestUsageReport_CompactionWarning(t *testing.T) {
	b := NewContextBudget(200_000)

	// Use a large conversation size, then get its specific allocation to find
	// the actual threshold (adaptive allocation shifts as conversation grows).
	hugeConv := 180_000
	alloc := b.Allocate(hugeConv)
	overflowConv := alloc.Conversation + 5000

	report := b.UsageReport(overflowConv)
	if !strings.Contains(report, "COMPACT NEEDED") {
		t.Errorf("report should show COMPACT NEEDED when conversation exceeds allocation:\n%s", report)
	}
}

func TestUsageReport_NoWarningWhenOK(t *testing.T) {
	b := NewContextBudget(200_000)

	report := b.UsageReport(1000)
	if strings.Contains(report, "COMPACT NEEDED") {
		t.Errorf("report should not show COMPACT NEEDED for small conversation:\n%s", report)
	}
}

func TestUsageReport_ZeroConversation_NoConversationLine(t *testing.T) {
	b := NewContextBudget(200_000)
	report := b.UsageReport(0)

	// With zero conversation tokens, the "Conversation: X / Y tokens" line is omitted.
	if strings.Contains(report, "Conversation: 0 /") {
		t.Errorf("report should not show conversation usage line for 0 tokens:\n%s", report)
	}
}

func TestAllocation_SumsToTotal(t *testing.T) {
	sizes := []int{32_000, 64_000, 128_000, 200_000, 1_000_000}
	convos := []int{0, 5_000, 20_000, 80_000}

	for _, size := range sizes {
		b := NewContextBudget(size)
		for _, conv := range convos {
			alloc := b.Allocate(conv)
			sum := alloc.SystemPrompt + alloc.ToolDefs + alloc.RepoMap + alloc.Memory +
				alloc.Workspace + alloc.PreloadedFiles + alloc.Conversation +
				alloc.OutputReserve + alloc.SafetyMargin + alloc.Remaining
			if sum != b.Total {
				t.Errorf("size=%d conv=%d: allocation sum (%d) != Total (%d)",
					size, conv, sum, b.Total)
			}
		}
	}
}

func TestClamp(t *testing.T) {
	tests := []struct {
		v, lo, hi, want int
	}{
		{5, 1, 10, 5},   // within range
		{0, 1, 10, 1},   // below floor
		{15, 1, 10, 10},  // above ceiling
		{1, 1, 10, 1},   // at floor
		{10, 1, 10, 10},  // at ceiling
	}
	for _, tt := range tests {
		got := clamp(tt.v, tt.lo, tt.hi)
		if got != tt.want {
			t.Errorf("clamp(%d, %d, %d) = %d, want %d", tt.v, tt.lo, tt.hi, got, tt.want)
		}
	}
}

func TestContextBudget_VerySmallWindow(t *testing.T) {
	// Edge case: context window smaller than typical fixed allocations.
	b := NewContextBudget(8_000)

	// Should not panic and all values should be non-negative.
	alloc := b.Allocate(0)
	if alloc.SystemPrompt < 0 || alloc.ToolDefs < 0 || alloc.PreloadedFiles < 0 ||
		alloc.Conversation < 0 || alloc.OutputReserve < 0 {
		t.Error("no allocation should be negative for tiny context window")
	}
}

func TestContextBudget_MillionTokenModel(t *testing.T) {
	b := NewContextBudget(1_000_000)

	// Fixed allocations should all be at their ceilings.
	if b.SystemPrompt != 5000 {
		t.Errorf("expected SystemPrompt=5000 for 1M model, got %d", b.SystemPrompt)
	}
	if b.ToolDefs != 3000 {
		t.Errorf("expected ToolDefs=3000 for 1M model, got %d", b.ToolDefs)
	}
	if b.OutputReserve != 20000 {
		t.Errorf("expected OutputReserve=20000 for 1M model, got %d", b.OutputReserve)
	}
	if b.SafetyMargin != 15000 {
		t.Errorf("expected SafetyMargin=15000 for 1M model, got %d", b.SafetyMargin)
	}

	// Conversation space should be massive.
	alloc := b.Allocate(0)
	if alloc.Conversation < 500_000 {
		t.Errorf("1M model should have >500K conversation budget, got %d", alloc.Conversation)
	}
}

func TestAdaptiveFileBudget_Monotonic(t *testing.T) {
	b := NewContextBudget(200_000)

	// File budget should monotonically decrease (or stay flat) as conversation grows.
	prev := b.FilesBudget(0)
	for conv := 1000; conv <= 150_000; conv += 1000 {
		cur := b.FilesBudget(conv)
		if cur > prev {
			t.Errorf("file budget increased from %d to %d at conversation=%d",
				prev, cur, conv)
		}
		prev = cur
	}
}

func TestUsageReport_FormattedCorrectly(t *testing.T) {
	b := NewContextBudget(128_000)
	report := b.UsageReport(10_000)

	lines := strings.Split(report, "\n")
	if len(lines) < 12 {
		t.Errorf("report should have at least 12 lines, got %d:\n%s", len(lines), report)
	}

	// First line should contain total.
	if !strings.Contains(lines[0], "128000") {
		t.Errorf("first line should show total context size:\n%s", report)
	}
}
