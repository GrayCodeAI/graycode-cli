package memory

import (
	"context"
	"strings"
	"sync"

	yaadEngine "github.com/GrayCodeAI/yaad/engine"
	"github.com/GrayCodeAI/yaad/storage"
)

// CrossProjectMemory manages global user-level memories that transfer across
// all projects. Things like coding style preferences, favorite libraries,
// and workflow patterns are stored with scope:"global" and injected regardless
// of which project the user is working in.
type CrossProjectMemory struct {
	bridge *YaadBridge
	mu     sync.Mutex
}

// NewCrossProjectMemory creates a cross-project memory manager.
func NewCrossProjectMemory(bridge *YaadBridge) *CrossProjectMemory {
	return &CrossProjectMemory{bridge: bridge}
}

// StoreGlobal saves a memory with global scope (applies to all projects).
func (cp *CrossProjectMemory) StoreGlobal(content, nodeType string) error {
	if !cp.bridge.Ready() {
		return nil
	}
	cp.mu.Lock()
	defer cp.mu.Unlock()

	if !yaadEngine.IsValidNodeType(nodeType) {
		nodeType = "preference"
	}

	_, err := cp.bridge.engine.Remember(context.Background(), yaadEngine.RememberInput{
		Type:    nodeType,
		Content: content,
		Scope:   "global",
		Project: "__global__",
	})
	return err
}

// RecallGlobal retrieves global memories relevant to a query.
func (cp *CrossProjectMemory) RecallGlobal(query string, budget int) (string, error) {
	if !cp.bridge.Ready() {
		return "", nil
	}
	cp.mu.Lock()
	defer cp.mu.Unlock()

	result, err := cp.bridge.engine.Recall(context.Background(), yaadEngine.RecallOpts{
		Query:   query,
		Budget:  budget,
		Limit:   10,
		Depth:   1,
		Project: "__global__",
	})
	if err != nil || result == nil || len(result.Nodes) == 0 {
		return "", err
	}

	var sb strings.Builder
	for i, node := range result.Nodes {
		if i > 0 {
			sb.WriteString("\n")
		}
		sb.WriteString("[" + node.Type + "] " + node.Content)
	}
	return sb.String(), nil
}

// GetPreferences returns all global preference memories.
func (cp *CrossProjectMemory) GetPreferences() ([]string, error) {
	if !cp.bridge.Ready() {
		return nil, nil
	}
	cp.mu.Lock()
	defer cp.mu.Unlock()

	nodes, err := cp.bridge.store.ListNodes(context.Background(), storage.NodeFilter{
		Type:  "preference",
		Scope: "global",
		Limit: 50,
	})
	if err != nil {
		return nil, err
	}

	var prefs []string
	for _, n := range nodes {
		prefs = append(prefs, n.Content)
	}
	return prefs, nil
}

// GetConventions returns all global coding conventions.
func (cp *CrossProjectMemory) GetConventions() ([]string, error) {
	if !cp.bridge.Ready() {
		return nil, nil
	}
	cp.mu.Lock()
	defer cp.mu.Unlock()

	nodes, err := cp.bridge.store.ListNodes(context.Background(), storage.NodeFilter{
		Type:  "convention",
		Scope: "global",
		Limit: 50,
	})
	if err != nil {
		return nil, err
	}

	var convs []string
	for _, n := range nodes {
		convs = append(convs, n.Content)
	}
	return convs, nil
}

// InjectGlobalContext builds a context string from global memories for prompt injection.
func (cp *CrossProjectMemory) InjectGlobalContext(budget int) string {
	if !cp.bridge.Ready() {
		return ""
	}
	cp.mu.Lock()
	defer cp.mu.Unlock()

	// Get preferences and global conventions
	nodes, err := cp.bridge.store.ListNodes(context.Background(), storage.NodeFilter{
		Scope:         "global",
		Limit:         20,
		MinConfidence: 0.5,
	})
	if err != nil || len(nodes) == 0 {
		return ""
	}

	var sb strings.Builder
	sb.WriteString("## User Preferences (Global)\n")
	tokenEstimate := 0
	for _, n := range nodes {
		line := "- [" + n.Type + "] " + n.Content + "\n"
		lineTokens := len(line) / 4
		if tokenEstimate+lineTokens > budget {
			break
		}
		sb.WriteString(line)
		tokenEstimate += lineTokens
	}

	return sb.String()
}

// DetectGlobalPatterns analyzes project-local memories and promotes recurring
// patterns to global scope. E.g., if the user uses "tabs" in 3+ projects,
// promote to global preference.
func (cp *CrossProjectMemory) DetectGlobalPatterns(projectMemories []string) []string {
	// Look for patterns that should be global
	var promoted []string
	globalIndicators := []string{
		"always", "never", "prefer", "i like", "i use", "my style",
		"i want", "coding style", "formatting",
	}

	for _, mem := range projectMemories {
		lower := strings.ToLower(mem)
		for _, indicator := range globalIndicators {
			if strings.Contains(lower, indicator) {
				promoted = append(promoted, mem)
				break
			}
		}
	}
	return promoted
}

// PromoteToGlobal takes project-local memories and promotes them to global scope.
func (cp *CrossProjectMemory) PromoteToGlobal(memories []string) error {
	for _, mem := range memories {
		if err := cp.StoreGlobal(mem, "preference"); err != nil {
			return err
		}
	}
	return nil
}
