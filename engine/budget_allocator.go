package engine

import (
	"fmt"
	"math"
	"sort"
	"strings"
	"sync"
)

// BudgetAllocator dynamically distributes context window space across named
// allocations. It respects per-allocation min/max bounds, priority ordering,
// and supports runtime rebalancing when usage patterns shift.
type BudgetAllocator struct {
	TotalBudget   int
	OutputReserve int
	Allocations   map[string]*Allocation
	Priority      []string // allocation names sorted by priority (1=highest first)
	mu            sync.RWMutex
}

// Allocation represents a single named budget category with bounds and priority.
type Allocation struct {
	Name          string
	MinTokens     int
	MaxTokens     int // 0 or negative means unlimited
	CurrentTokens int
	Priority      int     // 1 = highest priority
	Flexible      bool    // can tokens be stolen from this allocation?
	Usage         float64 // 0.0 to 1.0, fraction of CurrentTokens actually used
}

// NewBudgetAllocator creates a new allocator with the given total budget and
// output reserve. The distributable budget is TotalBudget - OutputReserve.
func NewBudgetAllocator(totalBudget, outputReserve int) *BudgetAllocator {
	return &BudgetAllocator{
		TotalBudget:   totalBudget,
		OutputReserve: outputReserve,
		Allocations:   make(map[string]*Allocation),
		Priority:      nil,
	}
}

// Register adds a new allocation category to the budget allocator.
// Priority 1 is the highest priority. If maxTokens <= 0, it is treated as unlimited.
func (ba *BudgetAllocator) Register(name string, min, max, priority int, flexible bool) {
	ba.mu.Lock()
	defer ba.mu.Unlock()

	ba.Allocations[name] = &Allocation{
		Name:      name,
		MinTokens: min,
		MaxTokens: max,
		Priority:  priority,
		Flexible:  flexible,
	}

	ba.rebuildPriority()
}

// rebuildPriority rebuilds the priority-sorted name list. Must be called with lock held.
func (ba *BudgetAllocator) rebuildPriority() {
	names := make([]string, 0, len(ba.Allocations))
	for name := range ba.Allocations {
		names = append(names, name)
	}
	sort.Slice(names, func(i, j int) bool {
		return ba.Allocations[names[i]].Priority < ba.Allocations[names[j]].Priority
	})
	ba.Priority = names
}

// Allocate distributes the available budget (TotalBudget - OutputReserve) across
// all registered allocations. It first satisfies minimums, then distributes remaining
// tokens by priority order (lowest number = highest priority), respecting max caps.
// Any tokens that cannot be assigned (because all allocations hit their max) remain
// unallocated. Returns a map of allocation name to assigned token count.
func (ba *BudgetAllocator) Allocate() map[string]int {
	ba.mu.Lock()
	defer ba.mu.Unlock()

	available := ba.TotalBudget - ba.OutputReserve
	if available < 0 {
		available = 0
	}

	result := make(map[string]int, len(ba.Allocations))

	// Phase 1: assign minimums to all allocations.
	for _, name := range ba.Priority {
		alloc := ba.Allocations[name]
		min := alloc.MinTokens
		if min > available {
			min = available
		}
		result[name] = min
		available -= min
	}

	// Phase 2: distribute remaining by priority, up to max.
	// We iterate by priority (highest first) and give each allocation
	// as much as possible up to its max.
	for _, name := range ba.Priority {
		if available <= 0 {
			break
		}
		alloc := ba.Allocations[name]
		current := result[name]
		maxAllowed := ba.effectiveMax(alloc)

		room := maxAllowed - current
		if room <= 0 {
			continue
		}
		give := room
		if give > available {
			give = available
		}
		result[name] += give
		available -= give
	}

	// Phase 3: if there is still remaining budget (all hit their caps),
	// redistribute to flexible allocations with unlimited max (MaxTokens <= 0).
	if available > 0 {
		for _, name := range ba.Priority {
			if available <= 0 {
				break
			}
			alloc := ba.Allocations[name]
			if alloc.MaxTokens > 0 {
				continue // has a hard cap, already satisfied
			}
			// Unlimited allocation, give it extra.
			result[name] += available
			available = 0
		}
	}

	// Update CurrentTokens on each allocation.
	for name, tokens := range result {
		ba.Allocations[name].CurrentTokens = tokens
	}

	return result
}

// effectiveMax returns the effective maximum for an allocation.
// If MaxTokens <= 0, returns a very large number (effectively unlimited).
func (ba *BudgetAllocator) effectiveMax(alloc *Allocation) int {
	if alloc.MaxTokens <= 0 {
		return math.MaxInt32
	}
	return alloc.MaxTokens
}

// RequestMore attempts to acquire additional tokens for the named allocation by
// stealing from lower-priority flexible allocations. Returns the number of tokens
// actually granted (may be less than needed).
func (ba *BudgetAllocator) RequestMore(name string, needed int) int {
	ba.mu.Lock()
	defer ba.mu.Unlock()

	alloc, ok := ba.Allocations[name]
	if !ok || needed <= 0 {
		return 0
	}

	// Check if allocation can even accept more tokens.
	maxAllowed := ba.effectiveMax(alloc)
	room := maxAllowed - alloc.CurrentTokens
	if room <= 0 {
		return 0
	}
	if needed > room {
		needed = room
	}

	// Steal from lower-priority flexible allocations (higher priority number = lower priority).
	granted := 0

	// Iterate from lowest priority to highest to steal from the least important first.
	for i := len(ba.Priority) - 1; i >= 0; i-- {
		if granted >= needed {
			break
		}
		donorName := ba.Priority[i]
		if donorName == name {
			continue
		}
		donor := ba.Allocations[donorName]

		// Only steal from lower priority (higher number) and flexible allocations.
		if donor.Priority <= alloc.Priority {
			continue
		}
		if !donor.Flexible {
			continue
		}

		// Can steal down to donor's minimum.
		stealable := donor.CurrentTokens - donor.MinTokens
		if stealable <= 0 {
			continue
		}

		steal := needed - granted
		if steal > stealable {
			steal = stealable
		}

		donor.CurrentTokens -= steal
		granted += steal
	}

	alloc.CurrentTokens += granted
	return granted
}

// Release returns tokens from an allocation back to the pool (reduces CurrentTokens).
// It will not reduce below the allocation's minimum.
func (ba *BudgetAllocator) Release(name string, amount int) {
	ba.mu.Lock()
	defer ba.mu.Unlock()

	alloc, ok := ba.Allocations[name]
	if !ok || amount <= 0 {
		return
	}

	newTokens := alloc.CurrentTokens - amount
	if newTokens < alloc.MinTokens {
		newTokens = alloc.MinTokens
	}
	alloc.CurrentTokens = newTokens
}

// GetAvailable returns the number of tokens not currently assigned to any allocation.
func (ba *BudgetAllocator) GetAvailable() int {
	ba.mu.RLock()
	defer ba.mu.RUnlock()

	distributable := ba.TotalBudget - ba.OutputReserve
	used := 0
	for _, alloc := range ba.Allocations {
		used += alloc.CurrentTokens
	}

	available := distributable - used
	if available < 0 {
		return 0
	}
	return available
}

// Rebalance shrinks allocations with low usage and grows allocations that are
// at capacity. An allocation is considered underutilized if its Usage < 0.5
// and it has more than its minimum. An allocation needs growth if its Usage >= 0.9.
func (ba *BudgetAllocator) Rebalance() {
	ba.mu.Lock()
	defer ba.mu.Unlock()

	// Phase 1: Identify underutilized allocations and reclaim tokens.
	freed := 0
	for _, name := range ba.Priority {
		alloc := ba.Allocations[name]
		if !alloc.Flexible {
			continue
		}
		if alloc.Usage >= 0.5 {
			continue
		}
		if alloc.CurrentTokens <= alloc.MinTokens {
			continue
		}

		// Shrink to the midpoint between min and current, proportional to usage.
		// If usage is 0.0, shrink to minimum. If usage is 0.4, shrink less.
		// Target size: what would make usage ~70% at the used amount.
		usedTokens := int(float64(alloc.CurrentTokens) * alloc.Usage)
		targetSize := int(float64(usedTokens) / 0.7)
		if targetSize < alloc.MinTokens {
			targetSize = alloc.MinTokens
		}
		if targetSize >= alloc.CurrentTokens {
			continue
		}

		reclaim := alloc.CurrentTokens - targetSize
		alloc.CurrentTokens = targetSize
		freed += reclaim
	}

	// Phase 2: Distribute freed tokens to allocations at capacity (Usage >= 0.9).
	if freed <= 0 {
		return
	}

	for _, name := range ba.Priority {
		if freed <= 0 {
			break
		}
		alloc := ba.Allocations[name]
		if alloc.Usage < 0.9 {
			continue
		}

		maxAllowed := ba.effectiveMax(alloc)
		room := maxAllowed - alloc.CurrentTokens
		if room <= 0 {
			continue
		}

		give := freed
		if give > room {
			give = room
		}
		alloc.CurrentTokens += give
		freed -= give
	}
}

// DefaultAllocations registers the standard allocation categories for a coding agent.
func (ba *BudgetAllocator) DefaultAllocations() {
	ba.Register("system_prompt", 2000, 8000, 1, false)
	ba.Register("memory", 500, 4000, 2, true)
	ba.Register("conversation", 4000, 0, 3, false) // max=0 means unlimited
	ba.Register("repo_map", 1000, 8000, 4, true)
	ba.Register("tool_results", 2000, 20000, 5, true)
	ba.Register("readonly_ctx", 500, 10000, 6, true)
	ba.Register("goals", 200, 2000, 7, true)
}

// FormatBudget returns a formatted table showing all allocations, their bounds,
// current assignment, usage, and priority.
func (ba *BudgetAllocator) FormatBudget() string {
	ba.mu.RLock()
	defer ba.mu.RUnlock()

	var sb strings.Builder
	distributable := ba.TotalBudget - ba.OutputReserve

	sb.WriteString(fmt.Sprintf("Token Budget Allocator (Total: %d, Output Reserve: %d, Distributable: %d)\n",
		ba.TotalBudget, ba.OutputReserve, distributable))
	sb.WriteString(strings.Repeat("=", 80) + "\n")
	sb.WriteString(fmt.Sprintf("%-16s %6s %6s %8s %6s %5s %8s\n",
		"Name", "Min", "Max", "Current", "Usage", "Pri", "Flexible"))
	sb.WriteString(strings.Repeat("-", 80) + "\n")

	for _, name := range ba.Priority {
		alloc := ba.Allocations[name]
		maxStr := fmt.Sprintf("%d", alloc.MaxTokens)
		if alloc.MaxTokens <= 0 {
			maxStr = "∞"
		}
		flexStr := "no"
		if alloc.Flexible {
			flexStr = "yes"
		}
		sb.WriteString(fmt.Sprintf(
			"%-16s %6d %6s %8d %5.0f%% %5d %8s\n",
			alloc.Name,
			alloc.MinTokens,
			maxStr,
			alloc.CurrentTokens,
			alloc.Usage*100,
			alloc.Priority,
			flexStr,
		))
	}

	sb.WriteString(strings.Repeat("-", 80) + "\n")

	used := 0
	for _, alloc := range ba.Allocations {
		used += alloc.CurrentTokens
	}
	sb.WriteString(fmt.Sprintf("%-16s %6s %6s %8d\n", "TOTAL USED", "", "", used))
	sb.WriteString(fmt.Sprintf("%-16s %6s %6s %8d\n", "AVAILABLE", "", "", distributable-used))

	return sb.String()
}

// WouldExceed returns true if adding 'additional' tokens to the named allocation
// would cause it to exceed its maximum.
func (ba *BudgetAllocator) WouldExceed(name string, additional int) bool {
	ba.mu.RLock()
	defer ba.mu.RUnlock()

	alloc, ok := ba.Allocations[name]
	if !ok {
		return true // unknown allocation always "exceeds"
	}

	if alloc.MaxTokens <= 0 {
		return false // unlimited
	}

	return alloc.CurrentTokens+additional > alloc.MaxTokens
}

// SuggestCompaction analyzes current allocations and suggests which categories
// should be compacted to free space, based on usage and priority.
func (ba *BudgetAllocator) SuggestCompaction() string {
	ba.mu.RLock()
	defer ba.mu.RUnlock()

	distributable := ba.TotalBudget - ba.OutputReserve
	used := 0
	for _, alloc := range ba.Allocations {
		used += alloc.CurrentTokens
	}

	available := distributable - used
	usagePct := 0.0
	if distributable > 0 {
		usagePct = float64(used) * 100 / float64(distributable)
	}

	var sb strings.Builder

	if usagePct < 80 {
		sb.WriteString(fmt.Sprintf("Budget utilization: %.0f%% — no compaction needed.\n", usagePct))
		return sb.String()
	}

	sb.WriteString(fmt.Sprintf("Budget utilization: %.0f%% (available: %d tokens)\n", usagePct, available))
	sb.WriteString("Compaction suggestions:\n")

	// Suggest compacting low-priority flexible allocations with high usage.
	type candidate struct {
		name     string
		savings  int
		priority int
	}
	var candidates []candidate

	// Iterate from lowest priority to suggest those first.
	for i := len(ba.Priority) - 1; i >= 0; i-- {
		name := ba.Priority[i]
		alloc := ba.Allocations[name]
		if !alloc.Flexible {
			continue
		}
		if alloc.CurrentTokens <= alloc.MinTokens {
			continue
		}

		// Potential savings: reduce to 50% of current, but not below min.
		target := alloc.CurrentTokens / 2
		if target < alloc.MinTokens {
			target = alloc.MinTokens
		}
		savings := alloc.CurrentTokens - target
		if savings > 0 {
			candidates = append(candidates, candidate{
				name:     name,
				savings:  savings,
				priority: alloc.Priority,
			})
		}
	}

	if len(candidates) == 0 {
		sb.WriteString("  No flexible allocations available for compaction.\n")
		sb.WriteString("  Consider compacting conversation history.\n")
		return sb.String()
	}

	for _, c := range candidates {
		sb.WriteString(fmt.Sprintf("  - Compact %-16s: save ~%d tokens (priority %d)\n",
			c.name, c.savings, c.priority))
	}

	totalSavings := 0
	for _, c := range candidates {
		totalSavings += c.savings
	}
	sb.WriteString(fmt.Sprintf("  Total potential savings: ~%d tokens\n", totalSavings))

	return sb.String()
}
