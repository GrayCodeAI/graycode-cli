package session

import (
	"crypto/rand"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"sync"
	"time"
)

// ConversationNode is one message in Hawk's product-owned conversation graph.
type ConversationNode struct {
	ID        string            `json:"id"`
	ParentID  string            `json:"parent_id,omitempty"`
	Role      string            `json:"role"`
	Content   string            `json:"content,omitempty"`
	Model     string            `json:"model,omitempty"`
	CreatedAt time.Time         `json:"created_at"`
	Metadata  map[string]string `json:"metadata,omitempty"`
}

type conversationGraphState struct {
	Version   int                          `json:"version"`
	SessionID string                       `json:"session_id"`
	HeadID    string                       `json:"head_id,omitempty"`
	Nodes     map[string]*ConversationNode `json:"nodes"`
}

// ConversationGraph persists branching product history independently of the
// LLM runtime. It is safe for concurrent use.
type ConversationGraph struct {
	mu    sync.RWMutex
	path  string
	state conversationGraphState
}

// OpenConversationGraph opens or creates a graph at path.
func OpenConversationGraph(path, sessionID string) (*ConversationGraph, error) {
	if path == "" || sessionID == "" {
		return nil, fmt.Errorf("conversation graph: path and session id are required")
	}
	g := &ConversationGraph{path: path, state: conversationGraphState{Version: 1, SessionID: sessionID, Nodes: make(map[string]*ConversationNode)}}
	data, err := os.ReadFile(path) // #nosec G304 -- path is supplied by Hawk's session storage composition root
	if err == nil {
		if decodeErr := json.Unmarshal(data, &g.state); decodeErr != nil {
			return nil, fmt.Errorf("conversation graph: decode: %w", decodeErr)
		}
		if g.state.SessionID != sessionID {
			return nil, fmt.Errorf("conversation graph: session mismatch")
		}
		if g.state.Nodes == nil {
			g.state.Nodes = make(map[string]*ConversationNode)
		}
	} else if !os.IsNotExist(err) {
		return nil, fmt.Errorf("conversation graph: read: %w", err)
	}
	return g, nil
}

// Empty reports whether the graph has no nodes.
func (g *ConversationGraph) Empty() bool {
	g.mu.RLock()
	defer g.mu.RUnlock()
	return len(g.state.Nodes) == 0
}

// Append adds a child node and advances the graph head.
func (g *ConversationGraph) Append(parentID, role, content string) (*ConversationNode, error) {
	g.mu.Lock()
	defer g.mu.Unlock()
	if parentID != "" {
		if _, ok := g.state.Nodes[parentID]; !ok {
			return nil, fmt.Errorf("conversation graph: parent %q not found", parentID)
		}
	}
	node := &ConversationNode{ID: conversationNodeID(), ParentID: parentID, Role: role, Content: content, CreatedAt: time.Now().UTC()}
	g.state.Nodes[node.ID] = node
	g.state.HeadID = node.ID
	if err := g.persistLocked(); err != nil {
		delete(g.state.Nodes, node.ID)
		return nil, err
	}
	return cloneConversationNode(node), nil
}

// Fork creates an alternative node at the same parent as nodeID and makes it
// the active head.
func (g *ConversationGraph) Fork(nodeID string) (*ConversationNode, error) {
	g.mu.Lock()
	defer g.mu.Unlock()
	source, ok := g.state.Nodes[nodeID]
	if !ok {
		return nil, fmt.Errorf("conversation graph: node %q not found", nodeID)
	}
	fork := cloneConversationNode(source)
	fork.ID = conversationNodeID()
	fork.CreatedAt = time.Now().UTC()
	if fork.Metadata == nil {
		fork.Metadata = make(map[string]string)
	}
	fork.Metadata["forked_from"] = nodeID
	g.state.Nodes[fork.ID] = fork
	g.state.HeadID = fork.ID
	if err := g.persistLocked(); err != nil {
		delete(g.state.Nodes, fork.ID)
		return nil, err
	}
	return cloneConversationNode(fork), nil
}

// History returns the root-to-node path.
func (g *ConversationGraph) History(nodeID string) ([]*ConversationNode, error) {
	g.mu.RLock()
	defer g.mu.RUnlock()
	var reversed []*ConversationNode
	seen := make(map[string]bool)
	for nodeID != "" {
		if seen[nodeID] {
			return nil, fmt.Errorf("conversation graph: cycle at %q", nodeID)
		}
		seen[nodeID] = true
		node, ok := g.state.Nodes[nodeID]
		if !ok {
			return nil, fmt.Errorf("conversation graph: node %q not found", nodeID)
		}
		reversed = append(reversed, cloneConversationNode(node))
		nodeID = node.ParentID
	}
	out := make([]*ConversationNode, len(reversed))
	for i := range reversed {
		out[len(reversed)-1-i] = reversed[i]
	}
	return out, nil
}

// Branches lists direct children of nodeID. An empty nodeID lists roots.
func (g *ConversationGraph) Branches(nodeID string) ([]*ConversationNode, error) {
	g.mu.RLock()
	defer g.mu.RUnlock()
	if nodeID != "" {
		if _, ok := g.state.Nodes[nodeID]; !ok {
			return nil, fmt.Errorf("conversation graph: node %q not found", nodeID)
		}
	}
	var out []*ConversationNode
	for _, node := range g.state.Nodes {
		if node.ParentID == nodeID {
			out = append(out, cloneConversationNode(node))
		}
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].CreatedAt.Equal(out[j].CreatedAt) {
			return out[i].ID < out[j].ID
		}
		return out[i].CreatedAt.Before(out[j].CreatedAt)
	})
	return out, nil
}

// Head returns the active branch head.
func (g *ConversationGraph) Head() (*ConversationNode, error) {
	g.mu.RLock()
	defer g.mu.RUnlock()
	if g.state.HeadID == "" {
		return nil, fmt.Errorf("conversation graph: no head")
	}
	node, ok := g.state.Nodes[g.state.HeadID]
	if !ok {
		return nil, fmt.Errorf("conversation graph: head not found")
	}
	return cloneConversationNode(node), nil
}

// SetHead switches the active branch.
func (g *ConversationGraph) SetHead(nodeID string) error {
	g.mu.Lock()
	defer g.mu.Unlock()
	if _, ok := g.state.Nodes[nodeID]; !ok {
		return fmt.Errorf("conversation graph: node %q not found", nodeID)
	}
	previous := g.state.HeadID
	g.state.HeadID = nodeID
	if err := g.persistLocked(); err != nil {
		g.state.HeadID = previous
		return err
	}
	return nil
}

// Close flushes graph state.
func (g *ConversationGraph) Close() error {
	g.mu.Lock()
	defer g.mu.Unlock()
	return g.persistLocked()
}

func (g *ConversationGraph) persistLocked() error {
	if err := os.MkdirAll(filepath.Dir(g.path), 0o750); err != nil {
		return fmt.Errorf("conversation graph: create directory: %w", err)
	}
	data, err := json.MarshalIndent(g.state, "", "  ")
	if err != nil {
		return fmt.Errorf("conversation graph: encode: %w", err)
	}
	tmp := g.path + ".tmp"
	if err := os.WriteFile(tmp, data, 0o600); err != nil {
		return fmt.Errorf("conversation graph: write: %w", err)
	}
	if err := os.Rename(tmp, g.path); err != nil {
		_ = os.Remove(tmp)
		return fmt.Errorf("conversation graph: replace: %w", err)
	}
	return nil
}

func cloneConversationNode(node *ConversationNode) *ConversationNode {
	if node == nil {
		return nil
	}
	copy := *node
	if node.Metadata != nil {
		copy.Metadata = make(map[string]string, len(node.Metadata))
		for key, value := range node.Metadata {
			copy.Metadata[key] = value
		}
	}
	return &copy
}

func conversationNodeID() string {
	bytes := make([]byte, 16)
	_, _ = rand.Read(bytes)
	return fmt.Sprintf("%x", bytes)
}
