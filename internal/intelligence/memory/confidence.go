package memory

import (
	"context"
	"sync"
	"time"

	"github.com/GrayCodeAI/yaad/storage"
)

// ConfidenceTracker adjusts memory confidence based on session outcomes.
// When a session succeeds, accessed memories get a confidence boost.
// When a session fails, accessed memories get flagged for review.
type ConfidenceTracker struct {
	bridge      *YaadBridge
	accessed    map[string]time.Time // nodeID → last access time
	mu          sync.Mutex
	boostAmount float64
	penaltyRate float64
}

// NewConfidenceTracker creates a tracker that adjusts confidence from outcomes.
func NewConfidenceTracker(bridge *YaadBridge) *ConfidenceTracker {
	return &ConfidenceTracker{
		bridge:      bridge,
		accessed:    make(map[string]time.Time),
		boostAmount: 0.15,
		penaltyRate: 0.05,
	}
}

// RecordAccess notes that a memory was accessed/used during this session.
func (ct *ConfidenceTracker) RecordAccess(nodeIDs ...string) {
	ct.mu.Lock()
	defer ct.mu.Unlock()
	now := time.Now()
	for _, id := range nodeIDs {
		ct.accessed[id] = now
	}
}

// RecordRecall tracks memories returned by a recall operation.
func (ct *ConfidenceTracker) RecordRecall(results []CompactResult) {
	ct.mu.Lock()
	defer ct.mu.Unlock()
	now := time.Now()
	for _, r := range results {
		ct.accessed[r.ID] = now
	}
}

// OnSessionSuccess boosts confidence for all memories accessed during this session.
func (ct *ConfidenceTracker) OnSessionSuccess() {
	ct.mu.Lock()
	ids := make([]string, 0, len(ct.accessed))
	for id := range ct.accessed {
		ids = append(ids, id)
	}
	ct.mu.Unlock()

	if !ct.bridge.Ready() || len(ids) == 0 {
		return
	}

	for _, id := range ids {
		ct.boostNode(id, ct.boostAmount)
	}
}

// OnSessionFailure applies a small confidence penalty to accessed memories,
// signaling they may contain incorrect or misleading information.
func (ct *ConfidenceTracker) OnSessionFailure() {
	ct.mu.Lock()
	ids := make([]string, 0, len(ct.accessed))
	for id := range ct.accessed {
		ids = append(ids, id)
	}
	ct.mu.Unlock()

	if !ct.bridge.Ready() || len(ids) == 0 {
		return
	}

	for _, id := range ids {
		ct.penalizeNode(id, ct.penaltyRate)
	}
}

// Reset clears access tracking (call at session start).
func (ct *ConfidenceTracker) Reset() {
	ct.mu.Lock()
	defer ct.mu.Unlock()
	ct.accessed = make(map[string]time.Time)
}

// AccessedCount returns how many unique memories were accessed.
func (ct *ConfidenceTracker) AccessedCount() int {
	ct.mu.Lock()
	defer ct.mu.Unlock()
	return len(ct.accessed)
}

func (ct *ConfidenceTracker) boostNode(id string, amount float64) {
	ct.bridge.mu.Lock()
	defer ct.bridge.mu.Unlock()

	if !ct.bridge.ready {
		return
	}

	node, err := ct.bridge.store.GetNode(context.Background(), id)
	if err != nil || node == nil {
		return
	}

	newConf := node.Confidence + amount
	if newConf > 1.0 {
		newConf = 1.0
	}
	node.Confidence = newConf
	_ = ct.bridge.store.UpdateNode(context.Background(), node)
}

func (ct *ConfidenceTracker) penalizeNode(id string, rate float64) {
	ct.bridge.mu.Lock()
	defer ct.bridge.mu.Unlock()

	if !ct.bridge.ready {
		return
	}

	node, err := ct.bridge.store.GetNode(context.Background(), id)
	if err != nil || node == nil {
		return
	}

	// Don't penalize pinned nodes
	if node.Pinned {
		return
	}

	newConf := node.Confidence - rate
	if newConf < 0.1 {
		newConf = 0.1
	}
	node.Confidence = newConf
	_ = ct.bridge.store.UpdateNode(context.Background(), node)
}

// BoostByType boosts all memories of a given type (useful for post-success reinforcement).
func (ct *ConfidenceTracker) BoostByType(nodeType string, amount float64) {
	if !ct.bridge.Ready() {
		return
	}
	nodes, err := ct.bridge.store.ListNodes(context.Background(), storage.NodeFilter{
		Type:  nodeType,
		Limit: 50,
	})
	if err != nil {
		return
	}
	for _, n := range nodes {
		ct.boostNode(n.ID, amount)
	}
}
