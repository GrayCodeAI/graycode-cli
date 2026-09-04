// Vendored from github.com/GrayCodeAI/eagle/graph at v0.0.0-20260902153929-5877bed17503 (MIT, Copyright (c) 2026 GrayCode AI).
// The upstream repository no longer exists; this copy is owned by Graycode as its contract surface.
// Package graph defines the portable graph vocabulary shared across graycode-eco.
//
// The package contains data contracts only. Individual repositories retain
// ownership of their graph storage, projections, and runtime behavior.
//
// Graph Engineering Patterns:
// This package implements the foundational patterns from LangGraph,
// Microsoft AutoGen, and Google ADK for multi-agent orchestration as
// nodes, edges, and shared state.
package graph

import (
	"fmt"
	"strings"
	"time"
)

// NodeType represents the specific type/behavior of a node in the graph
type NodeType string

const (
	// NodeTypeAgent represents an autonomous agent that can make decisions
	NodeTypeAgent NodeType = "agent"
	// NodeTypeTool represents a tool/function that agents can invoke
	NodeTypeTool NodeType = "tool"
	// NodeTypeFunction represents a function node (deterministic operation)
	NodeTypeFunction NodeType = "function"
	// NodeTypeStart represents the entry point of a graph
	NodeTypeStart NodeType = "start"
	// NodeTypeEnd represents the termination point of a graph
	NodeTypeEnd NodeType = "end"
	// NodeTypeRouter represents a conditional routing node
	NodeTypeRouter NodeType = "router"
	// NodeTypeQuality represents a code quality analysis node
	NodeTypeQuality NodeType = "quality"
	// NodeTypeExecution represents an execution journal node
	NodeTypeExecution NodeType = "execution"
	// NodeTypeOperations represents an operations orchestration node
	NodeTypeOperations NodeType = "operations"
	// NodeTypeSystem is an alias for NodeKindSystem for backward compatibility
	NodeTypeSystem NodeType = "system"
)

// NodeKind classifies a node by the ecosystem view to which it belongs.
type NodeKind string

const (
	NodeSystem     NodeKind = "system"
	NodeKnowledge  NodeKind = "knowledge"
	NodeExecution  NodeKind = "execution"
	NodePolicy     NodeKind = "policy"
	NodeQuality    NodeKind = "quality"
	NodeOperations NodeKind = "operations"
)

// ParseNodeKind normalizes a graph node kind.
func ParseNodeKind(s string) (NodeKind, error) {
	switch NodeKind(strings.ToLower(strings.TrimSpace(s))) {
	case NodeSystem, NodeKnowledge, NodeExecution, NodePolicy, NodeQuality, NodeOperations:
		return NodeKind(strings.ToLower(strings.TrimSpace(s))), nil
	default:
		return "", fmt.Errorf("graph: unknown node kind %q", s)
	}
}

// ParseNodeType normalizes a graph node type.
func ParseNodeType(s string) (NodeType, error) {
	switch NodeType(strings.ToLower(strings.TrimSpace(s))) {
	case NodeTypeAgent, NodeTypeTool, NodeTypeFunction, NodeTypeStart, NodeTypeEnd, NodeTypeRouter, NodeTypeQuality, NodeTypeExecution, NodeTypeOperations:
		return NodeType(strings.ToLower(strings.TrimSpace(s))), nil
	default:
		return "", fmt.Errorf("graph: unknown node type %q", s)
	}
}

// NodeSpec describes a node in an orchestration graph.
type NodeSpec struct {
	ID          string            `json:"id"`
	Type        NodeType          `json:"type"`
	Name        string            `json:"name,omitempty"`
	Description string            `json:"description,omitempty"`
	Config      map[string]string `json:"config,omitempty"`
}

// EdgeKind identifies a portable relationship between two graph nodes.
type EdgeKind string

const (
	EdgeContains    EdgeKind = "contains"
	EdgeDependsOn   EdgeKind = "depends_on"
	EdgeReferences  EdgeKind = "references"
	EdgeProduced    EdgeKind = "produced"
	EdgeGovernedBy  EdgeKind = "governed_by"
	EdgeValidatedBy EdgeKind = "validated_by"
)

// EdgeCondition represents a conditional edge in orchestration graphs.
// When non-empty, the edge is only followed if the condition evaluates to true.
type EdgeCondition struct {
	Expression string            `json:"expression,omitempty"`
	Variables  map[string]string `json:"variables,omitempty"`
}

// EdgeSpec describes an edge in an orchestration graph.
type EdgeSpec struct {
	From      string         `json:"from"`
	To        string         `json:"to"`
	Condition *EdgeCondition `json:"condition,omitempty"`
	Weight    float64        `json:"weight,omitempty"`
}

// GraphSpec describes a complete orchestration graph.
type GraphSpec struct {
	ID          string            `json:"id"`
	Name        string            `json:"name"`
	Description string            `json:"description,omitempty"`
	Nodes       []NodeSpec        `json:"nodes"`
	Edges       []EdgeSpec        `json:"edges"`
	Metadata    map[string]string `json:"metadata,omitempty"`
}

// ParseEdgeKind normalizes a graph edge kind.
func ParseEdgeKind(s string) (EdgeKind, error) {
	switch EdgeKind(strings.ToLower(strings.TrimSpace(s))) {
	case EdgeContains, EdgeDependsOn, EdgeReferences, EdgeProduced, EdgeGovernedBy, EdgeValidatedBy:
		return EdgeKind(strings.ToLower(strings.TrimSpace(s))), nil
	default:
		return "", fmt.Errorf("graph: unknown edge kind %q", s)
	}
}

// EventType identifies a lifecycle event for a graph subject.
type EventType string

const (
	EventCreated      EventType = "created"
	EventUpdated      EventType = "updated"
	EventTransitioned EventType = "transitioned"
	EventObserved     EventType = "observed"
	EventDeleted      EventType = "deleted"
)

// ParseEventType normalizes a graph event type.
func ParseEventType(s string) (EventType, error) {
	switch EventType(strings.ToLower(strings.TrimSpace(s))) {
	case EventCreated, EventUpdated, EventTransitioned, EventObserved, EventDeleted:
		return EventType(strings.ToLower(strings.TrimSpace(s))), nil
	default:
		return "", fmt.Errorf("graph: unknown event type %q", s)
	}
}

// Scope limits a graph fact to its authorized ecosystem boundary. Empty fields
// are valid for local-only or global facts.
type Scope struct {
	TenantID     string `json:"tenant_id,omitempty"`
	ProjectID    string `json:"project_id,omitempty"`
	RepositoryID string `json:"repository_id,omitempty"`
}

// Ref identifies a graph node without embedding its mutable attributes.
type Ref struct {
	Kind NodeKind `json:"kind"`
	ID   string   `json:"id"`
}

// Validate reports whether r can safely identify a graph node.
func (r Ref) Validate() error {
	if _, err := ParseNodeKind(string(r.Kind)); err != nil {
		return err
	}
	if strings.TrimSpace(r.ID) == "" {
		return fmt.Errorf("graph: reference ID is required")
	}
	return nil
}

// ArtifactRef points to immutable evidence outside the contract payload.
type ArtifactRef struct {
	URI       string `json:"uri"`
	Digest    string `json:"digest,omitempty"`
	MediaType string `json:"media_type,omitempty"`
}

// Provenance identifies the producer and evidence for a graph fact.
type Provenance struct {
	Producer string        `json:"producer"`
	Version  string        `json:"version,omitempty"`
	SourceID string        `json:"source_id,omitempty"`
	Evidence []ArtifactRef `json:"evidence,omitempty"`
}

// Validate reports whether p can support an auditable graph fact.
func (p Provenance) Validate() error {
	if strings.TrimSpace(p.Producer) == "" {
		return fmt.Errorf("graph: provenance producer is required")
	}
	for i, evidence := range p.Evidence {
		if strings.TrimSpace(evidence.URI) == "" {
			return fmt.Errorf("graph: evidence[%d] URI is required", i)
		}
	}
	return nil
}

// Node is a typed, temporal graph fact. Attributes are intentionally bounded
// to strings; large or sensitive data belongs in ArtifactRef evidence.
type Node struct {
	ID          string            `json:"id"`
	Kind        NodeKind          `json:"kind"`
	Scope       Scope             `json:"scope,omitempty"`
	CreatedAt   time.Time         `json:"created_at"`
	EffectiveAt time.Time         `json:"effective_at,omitempty"`
	Provenance  Provenance        `json:"provenance"`
	Attributes  map[string]string `json:"attributes,omitempty"`
}

// Validate reports whether n satisfies the minimum shared graph contract.
func (n Node) Validate() error {
	if strings.TrimSpace(n.ID) == "" {
		return fmt.Errorf("graph: node ID is required")
	}
	if _, err := ParseNodeKind(string(n.Kind)); err != nil {
		return err
	}
	if n.CreatedAt.IsZero() {
		return fmt.Errorf("graph: node created_at is required")
	}
	return n.Provenance.Validate()
}

// Validate reports whether s satisfies the minimum node spec contract.
func (s NodeSpec) Validate() error {
	if strings.TrimSpace(s.ID) == "" {
		return fmt.Errorf("graph: node spec ID is required")
	}
	if _, err := ParseNodeType(string(s.Type)); err != nil {
		return err
	}
	return nil
}

// Validate reports whether s satisfies the minimum edge spec contract.
func (s EdgeSpec) Validate() error {
	if strings.TrimSpace(s.From) == "" {
		return fmt.Errorf("graph: edge spec from is required")
	}
	if strings.TrimSpace(s.To) == "" {
		return fmt.Errorf("graph: edge spec to is required")
	}
	return nil
}

// Validate reports whether s satisfies the minimum graph spec contract.
func (s GraphSpec) Validate() error {
	if strings.TrimSpace(s.ID) == "" {
		return fmt.Errorf("graph: graph spec ID is required")
	}
	if len(s.Nodes) == 0 {
		return fmt.Errorf("graph: graph spec must have at least one node")
	}
	for i, node := range s.Nodes {
		if err := node.Validate(); err != nil {
			return fmt.Errorf("graph: node[%d]: %w", i, err)
		}
	}
	for i, edge := range s.Edges {
		if err := edge.Validate(); err != nil {
			return fmt.Errorf("graph: edge[%d]: %w", i, err)
		}
	}
	return nil
}

// Edge is a typed, temporal relationship between two graph nodes.
type Edge struct {
	ID          string            `json:"id"`
	Kind        EdgeKind          `json:"kind"`
	From        Ref               `json:"from"`
	To          Ref               `json:"to"`
	Scope       Scope             `json:"scope,omitempty"`
	CreatedAt   time.Time         `json:"created_at"`
	EffectiveAt time.Time         `json:"effective_at,omitempty"`
	Provenance  Provenance        `json:"provenance"`
	Attributes  map[string]string `json:"attributes,omitempty"`
}

// Validate reports whether e satisfies the minimum shared graph contract.
func (e Edge) Validate() error {
	if strings.TrimSpace(e.ID) == "" {
		return fmt.Errorf("graph: edge ID is required")
	}
	if _, err := ParseEdgeKind(string(e.Kind)); err != nil {
		return err
	}
	if err := e.From.Validate(); err != nil {
		return fmt.Errorf("graph: edge from: %w", err)
	}
	if err := e.To.Validate(); err != nil {
		return fmt.Errorf("graph: edge to: %w", err)
	}
	if e.CreatedAt.IsZero() {
		return fmt.Errorf("graph: edge created_at is required")
	}
	return e.Provenance.Validate()
}

// Event records an immutable lifecycle observation for a graph subject.
type Event struct {
	ID             string     `json:"id"`
	Type           EventType  `json:"type"`
	Subject        Ref        `json:"subject"`
	Scope          Scope      `json:"scope,omitempty"`
	OccurredAt     time.Time  `json:"occurred_at"`
	CorrelationID  string     `json:"correlation_id,omitempty"`
	CausationID    string     `json:"causation_id,omitempty"`
	IdempotencyKey string     `json:"idempotency_key,omitempty"`
	Provenance     Provenance `json:"provenance"`
}

// Validate reports whether e satisfies the minimum shared graph event contract.
func (e Event) Validate() error {
	if strings.TrimSpace(e.ID) == "" {
		return fmt.Errorf("graph: event ID is required")
	}
	if _, err := ParseEventType(string(e.Type)); err != nil {
		return err
	}
	if err := e.Subject.Validate(); err != nil {
		return fmt.Errorf("graph: event subject: %w", err)
	}
	if e.OccurredAt.IsZero() {
		return fmt.Errorf("graph: event occurred_at is required")
	}
	return e.Provenance.Validate()
}
