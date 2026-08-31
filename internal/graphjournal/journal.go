// Package graphjournal persists privacy-safe observations produced by Hawk's
// runtime. It is an append-only evidence stream; policy and verification
// components remain the source of truth for their decisions.
package graphjournal

import (
	"bufio"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	graphcontracts "github.com/GrayCodeAI/eagle/graph"
	policycontracts "github.com/GrayCodeAI/eagle/policy"
	"github.com/GrayCodeAI/hawk/internal/storage"
)

const (
	SchemaVersion = "hawk.graph-observation/v1"
	KindPolicy    = "policy"
	KindVerify    = "verification"
	KindContext   = "context"
	KindQuality   = "quality"
	KindRuntime   = "runtime"
)

var appendMu sync.Mutex

// Entry is one sanitized runtime observation. It intentionally contains no
// tool arguments, outputs, prompts, policy reasons, or verification details.
type Entry struct {
	SchemaVersion string               `json:"schema_version"`
	ID            string               `json:"id"`
	Kind          string               `json:"kind"`
	SessionID     string               `json:"session_id"`
	ToolCallID    string               `json:"tool_call_id,omitempty"`
	Stage         string               `json:"stage,omitempty"`
	OccurredAt    time.Time            `json:"occurred_at"`
	Policy        *PolicySummary       `json:"policy,omitempty"`
	Verification  *VerificationSummary `json:"verification,omitempty"`
	Context       *ContextGraph        `json:"context,omitempty"`
	Quality       *QualityGraph        `json:"quality,omitempty"`
	Runtime       *RuntimeGraph        `json:"runtime,omitempty"`
}

// PolicySummary is the export-safe portion of a permission verdict.
type PolicySummary struct {
	Allowed      bool                 `json:"allowed"`
	Risk         policycontracts.Risk `json:"risk"`
	Confidence   float64              `json:"confidence"`
	Rule         string               `json:"rule,omitempty"`
	Source       string               `json:"source,omitempty"`
	ReasonSHA256 string               `json:"reason_sha256,omitempty"`
}

// VerificationSummary records aggregate quality evidence without retaining
// targets, individual findings, expected values, or tool output.
type VerificationSummary struct {
	Failed       bool   `json:"failed"`
	FindingCount int    `json:"finding_count"`
	MaxSeverity  string `json:"max_severity"`
	TargetSHA256 string `json:"target_sha256,omitempty"`
}

// ContextGraph is a portable metadata-only subgraph selected for inference.
type ContextGraph struct {
	Source      string                 `json:"source"`
	QuerySHA256 string                 `json:"query_sha256,omitempty"`
	Nodes       []graphcontracts.Node  `json:"nodes"`
	Edges       []graphcontracts.Edge  `json:"edges"`
	Events      []graphcontracts.Event `json:"events"`
}

// QualityGraph is a portable metadata-only graph produced by a verification
// engine such as Inspect.
type QualityGraph struct {
	Source string                 `json:"source"`
	Nodes  []graphcontracts.Node  `json:"nodes"`
	Edges  []graphcontracts.Edge  `json:"edges"`
	Events []graphcontracts.Event `json:"events"`
}

// RuntimeGraph is a mixed portable subgraph for operations, policy, and
// quality facts emitted by runtime support engines such as Shrike.
type RuntimeGraph struct {
	Source string                 `json:"source"`
	Nodes  []graphcontracts.Node  `json:"nodes"`
	Edges  []graphcontracts.Edge  `json:"edges"`
	Events []graphcontracts.Event `json:"events"`
}

// AppendPolicy records one final policy stage outcome.
func AppendPolicy(sessionID, toolCallID, stage string, verdict policycontracts.PermissionVerdict, occurredAt time.Time) error {
	sessionID = strings.TrimSpace(sessionID)
	if sessionID == "" {
		return nil
	}
	entry := Entry{
		SchemaVersion: SchemaVersion,
		Kind:          KindPolicy,
		SessionID:     sessionID,
		ToolCallID:    strings.TrimSpace(toolCallID),
		Stage:         strings.TrimSpace(stage),
		OccurredAt:    graphTime(occurredAt),
		Policy: &PolicySummary{
			Allowed:      verdict.Allowed,
			Risk:         verdict.Risk,
			Confidence:   verdict.Confidence,
			Rule:         strings.TrimSpace(verdict.Rule),
			Source:       strings.TrimSpace(verdict.Source),
			ReasonSHA256: digest(verdict.Reason),
		},
	}
	entry.ID = entryID(entry)
	return appendEntry(entry)
}

// AppendVerification records aggregate verification evidence.
func AppendVerification(
	sessionID, toolCallID, stage string,
	failed bool,
	findingCount int,
	maxSeverity, target string,
	occurredAt time.Time,
) error {
	sessionID = strings.TrimSpace(sessionID)
	if sessionID == "" {
		return nil
	}
	if findingCount < 0 {
		findingCount = 0
	}
	entry := Entry{
		SchemaVersion: SchemaVersion,
		Kind:          KindVerify,
		SessionID:     sessionID,
		ToolCallID:    strings.TrimSpace(toolCallID),
		Stage:         strings.TrimSpace(stage),
		OccurredAt:    graphTime(occurredAt),
		Verification: &VerificationSummary{
			Failed:       failed,
			FindingCount: findingCount,
			MaxSeverity:  strings.TrimSpace(maxSeverity),
			TargetSHA256: digest(target),
		},
	}
	entry.ID = entryID(entry)
	return appendEntry(entry)
}

// AppendContextGraph records the exact portable context subgraph selected by a
// retrieval engine. The caller must provide graph-contract-valid facts.
func AppendContextGraph(
	sessionID, source, querySHA256 string,
	nodes []graphcontracts.Node,
	edges []graphcontracts.Edge,
	events []graphcontracts.Event,
	occurredAt time.Time,
) error {
	sessionID = strings.TrimSpace(sessionID)
	if sessionID == "" || len(nodes) == 0 {
		return nil
	}
	if err := validatePortableGraph(nodes, edges, events, graphcontracts.NodeKnowledge); err != nil {
		return fmt.Errorf("validate context graph: %w", err)
	}
	entry := Entry{
		SchemaVersion: SchemaVersion,
		Kind:          KindContext,
		SessionID:     sessionID,
		Stage:         "inference-context",
		OccurredAt:    graphTime(occurredAt),
		Context: &ContextGraph{
			Source:      strings.TrimSpace(source),
			QuerySHA256: normalizedSHA256(querySHA256),
			Nodes:       append([]graphcontracts.Node(nil), nodes...),
			Edges:       append([]graphcontracts.Edge(nil), edges...),
			Events:      append([]graphcontracts.Event(nil), events...),
		},
	}
	entry.ID = entryID(entry)
	return appendEntry(entry)
}

// AppendQualityGraph records a portable quality subgraph for later composition
// into the Hawk execution graph.
func AppendQualityGraph(
	sessionID, toolCallID, stage, source string,
	nodes []graphcontracts.Node,
	edges []graphcontracts.Edge,
	events []graphcontracts.Event,
	occurredAt time.Time,
) error {
	sessionID = strings.TrimSpace(sessionID)
	if sessionID == "" || len(nodes) == 0 {
		return nil
	}
	if err := validatePortableGraph(nodes, edges, events, graphcontracts.NodeQuality); err != nil {
		return fmt.Errorf("validate quality graph: %w", err)
	}
	entry := Entry{
		SchemaVersion: SchemaVersion,
		Kind:          KindQuality,
		SessionID:     sessionID,
		ToolCallID:    strings.TrimSpace(toolCallID),
		Stage:         strings.TrimSpace(stage),
		OccurredAt:    graphTime(occurredAt),
		Quality: &QualityGraph{
			Source: strings.TrimSpace(source),
			Nodes:  append([]graphcontracts.Node(nil), nodes...),
			Edges:  append([]graphcontracts.Edge(nil), edges...),
			Events: append([]graphcontracts.Event(nil), events...),
		},
	}
	entry.ID = entryID(entry)
	return appendEntry(entry)
}

// AppendRuntimeGraph records a mixed operations/policy/quality subgraph.
func AppendRuntimeGraph(
	sessionID, toolCallID, stage, source string,
	nodes []graphcontracts.Node,
	edges []graphcontracts.Edge,
	events []graphcontracts.Event,
	occurredAt time.Time,
) error {
	sessionID = strings.TrimSpace(sessionID)
	if sessionID == "" || len(nodes) == 0 {
		return nil
	}
	allowed := map[graphcontracts.NodeKind]bool{
		graphcontracts.NodeOperations: true,
		graphcontracts.NodePolicy:     true,
		graphcontracts.NodeQuality:    true,
	}
	if err := validatePortableGraphKinds(nodes, edges, events, allowed); err != nil {
		return fmt.Errorf("validate runtime graph: %w", err)
	}
	entry := Entry{
		SchemaVersion: SchemaVersion,
		Kind:          KindRuntime,
		SessionID:     sessionID,
		ToolCallID:    strings.TrimSpace(toolCallID),
		Stage:         strings.TrimSpace(stage),
		OccurredAt:    graphTime(occurredAt),
		Runtime: &RuntimeGraph{
			Source: strings.TrimSpace(source),
			Nodes:  append([]graphcontracts.Node(nil), nodes...),
			Edges:  append([]graphcontracts.Edge(nil), edges...),
			Events: append([]graphcontracts.Event(nil), events...),
		},
	}
	entry.ID = entryID(entry)
	return appendEntry(entry)
}

func validatePortableGraph(
	nodes []graphcontracts.Node,
	edges []graphcontracts.Edge,
	events []graphcontracts.Event,
	expectedKind graphcontracts.NodeKind,
) error {
	return validatePortableGraphKinds(
		nodes,
		edges,
		events,
		map[graphcontracts.NodeKind]bool{expectedKind: true},
	)
}

func validatePortableGraphKinds(
	nodes []graphcontracts.Node,
	edges []graphcontracts.Edge,
	events []graphcontracts.Event,
	allowedKinds map[graphcontracts.NodeKind]bool,
) error {
	refs := make(map[string]graphcontracts.NodeKind, len(nodes))
	for _, node := range nodes {
		if err := node.Validate(); err != nil {
			return fmt.Errorf("node: %w", err)
		}
		if !allowedKinds[node.Kind] {
			return fmt.Errorf("node %q has disallowed kind %q", node.ID, node.Kind)
		}
		if _, exists := refs[node.ID]; exists {
			return fmt.Errorf("duplicate node ID %q", node.ID)
		}
		refs[node.ID] = node.Kind
	}
	for _, edge := range edges {
		if err := edge.Validate(); err != nil {
			return fmt.Errorf("edge: %w", err)
		}
		if refs[edge.From.ID] != edge.From.Kind {
			return fmt.Errorf("edge %q has unknown from reference %q", edge.ID, edge.From.ID)
		}
		if refs[edge.To.ID] != edge.To.Kind {
			return fmt.Errorf("edge %q has unknown to reference %q", edge.ID, edge.To.ID)
		}
	}
	for _, event := range events {
		if err := event.Validate(); err != nil {
			return fmt.Errorf("event: %w", err)
		}
		if refs[event.Subject.ID] != event.Subject.Kind {
			return fmt.Errorf("event %q has unknown subject %q", event.ID, event.Subject.ID)
		}
	}
	return nil
}

// Load returns all valid observations for a session in append order.
func Load(sessionID string) ([]Entry, error) {
	sessionID = strings.TrimSpace(sessionID)
	if sessionID == "" {
		return nil, nil
	}
	file, err := os.Open(pathFor(sessionID)) // #nosec G304 -- path is a digest under Hawk state
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("open graph observation journal: %w", err)
	}
	defer func() {
		_ = file.Close()
	}()

	entries := make([]Entry, 0)
	scanner := bufio.NewScanner(file)
	scanner.Buffer(make([]byte, 4096), 16*1024*1024)
	for scanner.Scan() {
		var entry Entry
		if err := json.Unmarshal(scanner.Bytes(), &entry); err != nil {
			return nil, fmt.Errorf("decode graph observation journal: %w", err)
		}
		if entry.SchemaVersion != SchemaVersion || entry.SessionID != sessionID {
			continue
		}
		entries = append(entries, entry)
	}
	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("read graph observation journal: %w", err)
	}
	return entries, nil
}

func appendEntry(entry Entry) error {
	data, err := json.Marshal(entry)
	if err != nil {
		return fmt.Errorf("encode graph observation: %w", err)
	}
	data = append(data, '\n')

	appendMu.Lock()
	defer appendMu.Unlock()

	dir := filepath.Dir(pathFor(entry.SessionID))
	if mkdirErr := os.MkdirAll(dir, 0o700); mkdirErr != nil {
		return fmt.Errorf("create graph observation directory: %w", mkdirErr)
	}
	file, err := os.OpenFile(pathFor(entry.SessionID), os.O_WRONLY|os.O_CREATE|os.O_APPEND, 0o600) // #nosec G304 -- path is a digest under Hawk state
	if err != nil {
		return fmt.Errorf("open graph observation journal for append: %w", err)
	}
	if _, writeErr := file.Write(data); writeErr != nil {
		_ = file.Close()
		return fmt.Errorf("append graph observation: %w", writeErr)
	}
	if closeErr := file.Close(); closeErr != nil {
		return fmt.Errorf("close graph observation journal: %w", closeErr)
	}
	return nil
}

func pathFor(sessionID string) string {
	return filepath.Join(storage.StateDir(), "graph-observations", digest(sessionID)+".jsonl")
}

func entryID(entry Entry) string {
	material := strings.Join([]string{
		entry.SessionID,
		entry.ToolCallID,
		entry.Stage,
		entry.Kind,
		entry.OccurredAt.Format(time.RFC3339Nano),
	}, "\x00")
	return digest(material)[:20]
}

func graphTime(value time.Time) time.Time {
	if value.IsZero() {
		return time.Now().UTC()
	}
	return value.UTC()
}

func digest(value string) string {
	sum := sha256.Sum256([]byte(value))
	return hex.EncodeToString(sum[:])
}

func normalizedSHA256(value string) string {
	value = strings.TrimSpace(value)
	if len(value) != sha256.Size*2 {
		return ""
	}
	decoded, err := hex.DecodeString(value)
	if err != nil || hex.EncodeToString(decoded) != value {
		return ""
	}
	return value
}
