package memory

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"time"
)

// SharedMemory enables real-time memory sharing between parallel agents
// in Hawk's mission mode. When one agent discovers something, all other
// agents in the same mission can see it immediately.
type SharedMemory struct {
	bridge    *HarrierBridge
	missionID string
	agentID   string
	mu        sync.RWMutex
	listeners []func(MemoryEvent)
}

// MemoryEvent represents a memory change that other agents should know about.
type MemoryEvent struct {
	Type      string    `json:"type"` // "added", "updated", "conflict"
	NodeType  string    `json:"node_type"`
	Content   string    `json:"content"`
	AgentID   string    `json:"agent_id"`
	MissionID string    `json:"mission_id"`
	Timestamp time.Time `json:"timestamp"`
}

// ConflictInfo describes a detected conflict between agent memories.
type ConflictInfo struct {
	ExistingContent string
	NewContent      string
	ExistingAgent   string
	NewAgent        string
	NodeType        string
}

// NewSharedMemory creates a shared memory instance for a specific mission.
func NewSharedMemory(bridge *HarrierBridge, missionID, agentID string) *SharedMemory {
	return &SharedMemory{
		bridge:    bridge,
		missionID: missionID,
		agentID:   agentID,
	}
}

// Share stores a memory visible to all agents in the same mission.
func (sm *SharedMemory) Share(content, nodeType string) error {
	if !sm.bridge.Ready() {
		return nil
	}
	sm.mu.Lock()
	defer sm.mu.Unlock()

	// Check for conflicts with existing shared memories
	conflict := sm.detectConflict(content, nodeType)
	if conflict != nil {
		sm.notifyListeners(MemoryEvent{
			Type:      "conflict",
			NodeType:  nodeType,
			Content:   fmt.Sprintf("CONFLICT: Agent %s says '%s' but Agent %s says '%s'", conflict.NewAgent, conflict.NewContent, conflict.ExistingAgent, conflict.ExistingContent),
			AgentID:   sm.agentID,
			MissionID: sm.missionID,
			Timestamp: time.Now(),
		})
	}

	err := sm.bridge.rememberProject(context.Background(), content, nodeType, "mission:"+sm.missionID, sm.agentID)
	if err != nil {
		return err
	}

	sm.notifyListeners(MemoryEvent{
		Type:      "added",
		NodeType:  nodeType,
		Content:   content,
		AgentID:   sm.agentID,
		MissionID: sm.missionID,
		Timestamp: time.Now(),
	})

	return nil
}

// Recall retrieves shared memories from the mission namespace.
func (sm *SharedMemory) Recall(query string, budget int) (string, error) {
	if !sm.bridge.Ready() {
		return "", nil
	}
	sm.mu.RLock()
	defer sm.mu.RUnlock()

	result, err := sm.bridge.recallProject(context.Background(), query, "mission:"+sm.missionID, budget, 10, 2)
	if err != nil || result == nil || len(result.Nodes) == 0 {
		return "", err
	}

	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("## Shared Mission Memory (%s)\n", sm.missionID))
	for _, node := range result.Nodes {
		agent := node.SourceAgent
		if agent == "" {
			agent = "unknown"
		}
		sb.WriteString(fmt.Sprintf("- [%s by %s] %s\n", node.Type, agent, node.Content))
	}
	return sb.String(), nil
}

// GetAllShared returns all memories in the mission namespace.
func (sm *SharedMemory) GetAllShared() ([]string, error) {
	if !sm.bridge.Ready() {
		return nil, nil
	}
	sm.mu.RLock()
	defer sm.mu.RUnlock()

	result, err := sm.bridge.recallProject(context.Background(), "", "mission:"+sm.missionID, 0, 20, 0)
	if err != nil || result == nil {
		return nil, err
	}

	var memories []string
	for _, node := range result.Nodes {
		memories = append(memories, fmt.Sprintf("[%s] %s", node.Type, node.Content))
	}
	return memories, nil
}

// OnMemoryEvent registers a listener for memory events from other agents.
func (sm *SharedMemory) OnMemoryEvent(fn func(MemoryEvent)) {
	sm.mu.Lock()
	defer sm.mu.Unlock()
	sm.listeners = append(sm.listeners, fn)
}

func (sm *SharedMemory) notifyListeners(event MemoryEvent) {
	for _, fn := range sm.listeners {
		go fn(event)
	}
}

func (sm *SharedMemory) detectConflict(newContent, nodeType string) *ConflictInfo {
	ctx := context.Background()

	// Search for existing memories of the same type in this mission
	result, err := sm.bridge.recallProject(ctx, newContent, "mission:"+sm.missionID, 0, 5, 1)
	if err != nil || result == nil {
		return nil
	}

	for _, existing := range result.Nodes {
		if existing.Type != nodeType {
			continue
		}
		if existing.SourceAgent == sm.agentID {
			continue
		}
		// Check if the content contradicts
		if isContradiction(existing.Content, newContent) {
			return &ConflictInfo{
				ExistingContent: existing.Content,
				NewContent:      newContent,
				ExistingAgent:   existing.SourceAgent,
				NewAgent:        sm.agentID,
				NodeType:        nodeType,
			}
		}
	}
	return nil
}

// isContradiction detects if two memory contents are likely contradictory.
func isContradiction(existing, incoming string) bool {
	existLower := strings.ToLower(existing)
	incomingLower := strings.ToLower(incoming)

	// Check for explicit negation patterns
	negations := []struct{ positive, negative string }{
		{"use ", "don't use "},
		{"use ", "avoid "},
		{"always ", "never "},
		{"prefer ", "avoid "},
		{"enable ", "disable "},
	}

	for _, n := range negations {
		if strings.Contains(existLower, n.positive) && strings.Contains(incomingLower, n.negative) {
			// Check if they're about the same subject
			existSubject := extractSubject(existLower, n.positive)
			incomingSubject := extractSubject(incomingLower, n.negative)
			if existSubject != "" && existSubject == incomingSubject {
				return true
			}
		}
		if strings.Contains(existLower, n.negative) && strings.Contains(incomingLower, n.positive) {
			existSubject := extractSubject(existLower, n.negative)
			incomingSubject := extractSubject(incomingLower, n.positive)
			if existSubject != "" && existSubject == incomingSubject {
				return true
			}
		}
	}

	// Check "X over Y" vs "Y over X" pattern
	if strings.Contains(existLower, " over ") && strings.Contains(incomingLower, " over ") {
		eParts := strings.SplitN(existLower, " over ", 2)
		iParts := strings.SplitN(incomingLower, " over ", 2)
		if len(eParts) == 2 && len(iParts) == 2 {
			if strings.TrimSpace(eParts[0]) == strings.TrimSpace(iParts[1]) &&
				strings.TrimSpace(eParts[1]) == strings.TrimSpace(iParts[0]) {
				return true
			}
		}
	}

	return false
}

func extractSubject(text, prefix string) string {
	idx := strings.Index(text, prefix)
	if idx < 0 {
		return ""
	}
	after := text[idx+len(prefix):]
	// Take the first word/phrase (up to punctuation or end)
	end := strings.IndexAny(after, ".,;!?)")
	if end < 0 {
		end = len(after)
	}
	if end > 50 {
		end = 50
	}
	return strings.TrimSpace(after[:end])
}
