// Package executiongraph implements LangGraph-style state graph patterns
// for multi-agent orchestration.
package executiongraph

import (
	"context"
	"fmt"
	"sync"
	"time"
)

// StateGraph represents a LangGraph-style state graph for agent orchestration.
// It manages nodes, edges, and shared state with checkpointing support.
type StateGraph struct {
	mu           sync.RWMutex
	nodes        map[string]*GraphNode
	edges        map[string][]*GraphEdge
	state        *GraphState
	checkpointer *Checkpointer
}

// GraphNode represents a node in the orchestration graph.
type GraphNode struct {
	ID       string
	Type     string
	Name     string
	Handler  NodeHandler
	Metadata map[string]interface{}
}

// GraphEdge represents a connection between nodes with optional conditions.
type GraphEdge struct {
	From      string
	To        string
	Condition EdgeCondition
}

// EdgeCondition determines which edge to follow based on state.
type EdgeCondition func(state *GraphState) (string, error)

// NodeHandler processes a node and returns the result.
type NodeHandler func(ctx context.Context, state *GraphState) (*NodeResult, error)

// NodeResult contains the result of a node execution.
type NodeResult struct {
	NextNode string
	State    map[string]interface{}
	Messages []Message
	Error    error
}

// GraphState holds the shared state across the graph execution.
type GraphState struct {
	Data      map[string]interface{}
	Messages  []Message
	Context   context.Context
	Version   int64
	Timestamp time.Time
}

// Message represents a message in the conversation.
type Message struct {
	Role      string
	Content   string
	Metadata  map[string]interface{}
	Timestamp time.Time
}

// Checkpointer provides time-travel debugging and state persistence.
type Checkpointer struct {
	mu          sync.RWMutex
	checkpoints map[int64]*GraphState
}

// NewStateGraph creates a new state graph.
func NewStateGraph() *StateGraph {
	return &StateGraph{
		nodes:        make(map[string]*GraphNode),
		edges:        make(map[string][]*GraphEdge),
		state:        &GraphState{Data: make(map[string]interface{})},
		checkpointer: &Checkpointer{checkpoints: make(map[int64]*GraphState)},
	}
}

// AddNode adds a node to the graph.
func (g *StateGraph) AddNode(node *GraphNode) *StateGraph {
	g.mu.Lock()
	defer g.mu.Unlock()
	g.nodes[node.ID] = node
	return g
}

// AddEdge adds an edge between two nodes.
func (g *StateGraph) AddEdge(from, to string, condition EdgeCondition) *StateGraph {
	g.mu.Lock()
	defer g.mu.Unlock()
	edge := &GraphEdge{From: from, To: to, Condition: condition}
	g.edges[from] = append(g.edges[from], edge)
	return g
}

// Compile validates the graph structure.
func (g *StateGraph) Compile() error {
	g.mu.RLock()
	defer g.mu.RUnlock()

	for _, edges := range g.edges {
		for _, edge := range edges {
			if _, ok := g.nodes[edge.From]; !ok {
				return fmt.Errorf("edge from non-existent node: %s", edge.From)
			}
			if _, ok := g.nodes[edge.To]; !ok {
				return fmt.Errorf("edge to non-existent node: %s", edge.To)
			}
		}
	}

	if _, ok := g.nodes["start"]; !ok {
		return fmt.Errorf("graph must have a start node")
	}

	return nil
}

// Invoke runs the graph from start to end.
func (g *StateGraph) Invoke(ctx context.Context, initialState *GraphState) (*GraphState, error) {
	if err := g.Compile(); err != nil {
		return nil, err
	}

	g.mu.Lock()
	g.state = initialState
	g.mu.Unlock()

	currentNode := "start"

	for currentNode != "end" {
		g.mu.RLock()
		node, ok := g.nodes[currentNode]
		g.mu.RUnlock()

		if !ok {
			return nil, fmt.Errorf("node not found: %s", currentNode)
		}

		// Save checkpoint before executing node
		g.checkpointer.Save(g.state)

		// Execute node handler
		result, err := node.Handler(ctx, g.state)
		if err != nil {
			return nil, fmt.Errorf("node %s failed: %w", currentNode, err)
		}

		// Update state with result
		g.mu.Lock()
		for k, v := range result.State {
			g.state.Data[k] = v
		}
		g.state.Messages = append(g.state.Messages, result.Messages...)
		g.state.Version++
		g.state.Timestamp = time.Now()
		g.mu.Unlock()

		// Determine next node
		if result.NextNode != "" {
			currentNode = result.NextNode
			continue
		}

		// Check edges for conditional routing
		g.mu.RLock()
		edges := g.edges[currentNode]
		g.mu.RUnlock()

		if len(edges) == 0 {
			break
		}

		if len(edges) == 1 && edges[0].Condition == nil {
			currentNode = edges[0].To
			continue
		}

		// Evaluate conditions
		nextNode := ""
		for _, edge := range edges {
			if edge.Condition == nil {
				continue
			}
			target, err := edge.Condition(g.state)
			if err != nil {
				return nil, fmt.Errorf("condition evaluation failed: %w", err)
			}
			if target != "" {
				nextNode = target
				break
			}
		}

		if nextNode == "" {
			break
		}
		currentNode = nextNode
	}

	return g.state, nil
}

// Save saves a checkpoint.
func (c *Checkpointer) Save(state *GraphState) {
	c.mu.Lock()
	defer c.mu.Unlock()

	checkpoint := &GraphState{
		Data:      copyMap(state.Data),
		Messages:  copyMessages(state.Messages),
		Version:   state.Version,
		Timestamp: state.Timestamp,
	}
	c.checkpoints[state.Version] = checkpoint
}

// Load loads a checkpoint by version.
func (c *Checkpointer) Load(version int64) (*GraphState, error) {
	c.mu.RLock()
	defer c.mu.RUnlock()

	checkpoint, ok := c.checkpoints[version]
	if !ok {
		return nil, fmt.Errorf("checkpoint not found: %d", version)
	}
	return checkpoint, nil
}

// GetHistory returns all checkpoints.
func (c *Checkpointer) GetHistory() []*GraphState {
	c.mu.RLock()
	defer c.mu.RUnlock()

	history := make([]*GraphState, 0, len(c.checkpoints))
	for _, cp := range c.checkpoints {
		history = append(history, cp)
	}
	return history
}

func copyMap(m map[string]interface{}) map[string]interface{} {
	result := make(map[string]interface{})
	for k, v := range m {
		result[k] = v
	}
	return result
}

func copyMessages(msgs []Message) []Message {
	result := make([]Message, len(msgs))
	copy(result, msgs)
	return result
}
