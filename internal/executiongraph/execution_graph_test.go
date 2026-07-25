package executiongraph

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	graphcontracts "github.com/GrayCodeAI/hawk-core-contracts/graph"
	policycontracts "github.com/GrayCodeAI/hawk-core-contracts/policy"
	typescontracts "github.com/GrayCodeAI/hawk-core-contracts/types"
	verifycontracts "github.com/GrayCodeAI/hawk-core-contracts/verify"
	"github.com/GrayCodeAI/hawk/internal/session"
	"github.com/GrayCodeAI/hawk/internal/taskruntime"
	"github.com/GrayCodeAI/hawk/internal/tool"
)

func TestBuildProjectsSessionToolCallsAndTraceCheckpoint(t *testing.T) {
	t.Parallel()

	generatedAt := time.Date(2026, time.July, 25, 1, 0, 0, 0, time.UTC)
	saved := &session.Session{
		ID:        "session-alpha",
		Model:     "model-a",
		Provider:  "provider-a",
		CreatedAt: generatedAt.Add(-time.Hour),
		UpdatedAt: generatedAt.Add(-time.Minute),
		Messages: []session.Message{
			{Role: "user", Content: "private prompt value"},
			{Role: "assistant", ToolUse: []session.ToolCall{{
				ID:        "tool-1",
				Name:      "Bash",
				Arguments: map[string]interface{}{"command": "private command value"},
			}}},
			{Role: "user", ToolResults: []session.ToolResult{{
				ToolUseID: "tool-1",
				Content:   "private result value",
			}}},
		},
	}

	export, err := Build(Input{
		Session:         saved,
		GeneratedAt:     generatedAt,
		Scope:           graphcontracts.Scope{RepositoryID: "hawk"},
		ProducerVersion: "test",
		TraceCheckpoints: []TraceCheckpointRef{{
			CheckpointID: "abc123def456",
		}},
	})
	if err != nil {
		t.Fatalf("Build() error = %v", err)
	}
	if export.SchemaVersion != SchemaVersion {
		t.Fatalf("SchemaVersion = %q, want %q", export.SchemaVersion, SchemaVersion)
	}
	if len(export.Nodes) != 4 {
		t.Fatalf("len(Nodes) = %d, want session, task request, tool call, checkpoint", len(export.Nodes))
	}
	if len(export.Edges) != 3 {
		t.Fatalf("len(Edges) = %d, want 3", len(export.Edges))
	}
	if len(export.Events) != 3 {
		t.Fatalf("len(Events) = %d, want 3", len(export.Events))
	}

	toolNode := findNode(export.Nodes, toolCallNodeID(saved.ID, "tool-1"))
	if toolNode == nil {
		t.Fatal("tool-call node was not exported")
	}
	if got := toolNode.Attributes["status"]; got != "completed" {
		t.Fatalf("tool status = %q, want completed", got)
	}
	if findNode(export.Nodes, "trace/checkpoint/abc123def456") == nil {
		t.Fatal("Trace checkpoint reference node was not exported")
	}

	encoded, err := json.Marshal(export)
	if err != nil {
		t.Fatalf("json.Marshal() error = %v", err)
	}
	for _, secret := range []string{"private prompt value", "private command value", "private result value"} {
		if strings.Contains(string(encoded), secret) {
			t.Fatalf("graph export leaked sensitive payload %q", secret)
		}
	}
}

func TestBuildProjectsAuthoritativeTraceSessionAndCheckpointLineage(t *testing.T) {
	t.Parallel()

	generatedAt := time.Date(2026, time.July, 25, 1, 30, 0, 0, time.UTC)
	saved := &session.Session{
		ID:        "hawk-session",
		CreatedAt: generatedAt.Add(-time.Hour),
	}
	export, err := Build(Input{
		Session: saved,
		TraceSessions: []TraceSessionRef{{
			SessionID: "trace-session",
			CreatedAt: generatedAt.Add(-50 * time.Minute),
		}},
		TraceCheckpoints: []TraceCheckpointRef{{
			CheckpointID:   "abc123def456",
			TraceSessionID: "trace-session",
			CreatedAt:      generatedAt.Add(-time.Minute),
		}},
		GeneratedAt: generatedAt,
		Scope:       graphcontracts.Scope{RepositoryID: "hawk"},
	})
	if err != nil {
		t.Fatalf("Build() error = %v", err)
	}
	if findNode(export.Nodes, "trace/session/trace-session") == nil {
		t.Fatal("authoritative Trace session node was not exported")
	}
	if findNode(export.Nodes, "trace/checkpoint/abc123def456") == nil {
		t.Fatal("authoritative Trace checkpoint node was not exported")
	}
	assertEdge(
		t,
		export.Edges,
		"hawk/session/hawk-session",
		"trace/session/trace-session",
		graphcontracts.EdgeReferences,
	)
	assertEdge(
		t,
		export.Edges,
		"hawk/session/hawk-session",
		"trace/checkpoint/abc123def456",
		graphcontracts.EdgeReferences,
	)
	assertEdge(
		t,
		export.Edges,
		"trace/session/trace-session",
		"trace/checkpoint/abc123def456",
		graphcontracts.EdgeProduced,
	)
}

func TestBuildProjectsTaskPolicyVerificationAndRuntimeState(t *testing.T) {
	t.Parallel()

	generatedAt := time.Date(2026, time.July, 25, 2, 0, 0, 0, time.UTC)
	parent := &tool.Task{
		ID:        "task_1",
		Subject:   "parent",
		Status:    tool.TaskStatusInProgress,
		CreatedAt: generatedAt.Add(-2 * time.Hour),
		UpdatedAt: generatedAt.Add(-time.Hour),
	}
	child := &tool.Task{
		ID:       "task_1.1",
		ParentID: "task_1",
		Subject:  "child",
		Status:   tool.TaskStatusCompleted,
		Dependencies: []tool.TaskDependency{
			{TargetID: "task_1", Type: "parent-child"},
			{TargetID: "task_1", Type: "blocks"},
		},
		CreatedAt: generatedAt.Add(-90 * time.Minute),
		UpdatedAt: generatedAt.Add(-30 * time.Minute),
	}
	runtimeTask := &taskruntime.Task{
		ID:        "agent-1",
		Kind:      taskruntime.KindAgent,
		Prompt:    "private runtime task",
		Status:    taskruntime.StatusCompleted,
		Output:    "private runtime output",
		StartedAt: generatedAt.Add(-20 * time.Minute),
		DoneAt:    generatedAt.Add(-10 * time.Minute),
	}
	childRef := graphcontracts.Ref{
		Kind: graphcontracts.NodeExecution,
		ID:   structuredTaskNodeID(child.ID),
	}

	export, err := Build(Input{
		Tasks:        []*tool.Task{child, parent},
		RuntimeTasks: []*taskruntime.Task{runtimeTask},
		PolicyObservations: []PolicyObservation{{
			ID:         "policy-1",
			Subject:    childRef,
			Verdict:    policycontracts.Deny("private policy reason", "protected-branch"),
			OccurredAt: generatedAt.Add(-25 * time.Minute),
		}},
		Verifications: []VerificationObservation{{
			ID:      "verify-1",
			Subject: childRef,
			Report: &verifycontracts.Report{
				Target: "private verification target",
				FailOn: typescontracts.SeverityHigh,
				Findings: []verifycontracts.Finding{{
					Check:    "test",
					Severity: typescontracts.SeverityHigh,
					Message:  "private finding",
				}},
			},
			OccurredAt: generatedAt.Add(-5 * time.Minute),
		}},
		GeneratedAt: generatedAt,
		Scope:       graphcontracts.Scope{RepositoryID: "hawk"},
	})
	if err != nil {
		t.Fatalf("Build() error = %v", err)
	}
	if len(export.Nodes) != 5 {
		t.Fatalf("len(Nodes) = %d, want 5", len(export.Nodes))
	}
	assertEdgeKind(t, export.Edges, graphcontracts.EdgeContains)
	assertEdgeKind(t, export.Edges, graphcontracts.EdgeDependsOn)
	assertEdgeKind(t, export.Edges, graphcontracts.EdgeGovernedBy)
	assertEdgeKind(t, export.Edges, graphcontracts.EdgeValidatedBy)

	encoded, err := json.Marshal(export)
	if err != nil {
		t.Fatalf("json.Marshal() error = %v", err)
	}
	for _, secret := range []string{
		"private runtime task",
		"private runtime output",
		"private policy reason",
		"private verification target",
		"private finding",
	} {
		if strings.Contains(string(encoded), secret) {
			t.Fatalf("graph export leaked sensitive payload %q", secret)
		}
	}
}

func TestBuildRejectsDanglingGraphSubject(t *testing.T) {
	t.Parallel()

	_, err := Build(Input{
		GeneratedAt: time.Date(2026, time.July, 25, 3, 0, 0, 0, time.UTC),
		PolicyObservations: []PolicyObservation{{
			ID:      "policy-1",
			Subject: graphcontracts.Ref{Kind: graphcontracts.NodeExecution, ID: "missing"},
			Verdict: policycontracts.Allow("safe"),
		}},
	})
	if err == nil || !strings.Contains(err.Error(), "unknown source node") {
		t.Fatalf("Build() error = %v, want dangling-subject error", err)
	}
}

func TestBuildRequiresGeneratedTime(t *testing.T) {
	t.Parallel()

	if _, err := Build(Input{}); err == nil {
		t.Fatal("Build() error = nil, want generated-time error")
	}
}

func TestBuildSanitizesUntrustedDigestFields(t *testing.T) {
	t.Parallel()

	export, err := Build(Input{
		GeneratedAt: time.Date(2026, time.July, 25, 3, 30, 0, 0, time.UTC),
		PolicyObservations: []PolicyObservation{{
			ID:           "policy-1",
			Verdict:      policycontracts.Allow(""),
			ReasonSHA256: "private value pretending to be a digest",
		}},
		Verifications: []VerificationObservation{{
			ID: "verify-1",
			Summary: &VerificationSummary{
				MaxSeverity:  "info",
				TargetSHA256: "private target pretending to be a digest",
			},
		}},
	})
	if err != nil {
		t.Fatalf("Build() error = %v", err)
	}
	encoded, err := json.Marshal(export)
	if err != nil {
		t.Fatalf("json.Marshal() error = %v", err)
	}
	for _, secret := range []string{
		"private value pretending to be a digest",
		"private target pretending to be a digest",
	} {
		if strings.Contains(string(encoded), secret) {
			t.Fatalf("graph export leaked untrusted digest field %q", secret)
		}
	}
}

func TestBuildMergesRetrievedKnowledgeContext(t *testing.T) {
	t.Parallel()

	generatedAt := time.Date(2026, time.July, 25, 3, 45, 0, 0, time.UTC)
	saved := &session.Session{
		ID:        "context-session",
		CreatedAt: generatedAt.Add(-time.Hour),
	}
	knowledge := graphcontracts.Node{
		ID:        "yaad/memory/memory-1",
		Kind:      graphcontracts.NodeKnowledge,
		CreatedAt: generatedAt.Add(-time.Minute),
		Provenance: graphcontracts.Provenance{
			Producer: "yaad",
		},
		Attributes: map[string]string{
			"data_classification": "metadata_only",
			"content_sha256":      "abc123",
		},
	}
	export, err := Build(Input{
		Session: saved,
		ContextObservations: []ContextObservation{{
			ID:      "context-1",
			Subject: graphcontracts.Ref{Kind: graphcontracts.NodeExecution, ID: "hawk/session/" + saved.ID},
			Nodes:   []graphcontracts.Node{knowledge},
		}},
		GeneratedAt: generatedAt,
		Scope:       graphcontracts.Scope{RepositoryID: "hawk"},
	})
	if err != nil {
		t.Fatalf("Build() error = %v", err)
	}
	if findNode(export.Nodes, knowledge.ID) == nil {
		t.Fatal("retrieved knowledge node was not merged")
	}
	assertEdgeKind(t, export.Edges, graphcontracts.EdgeReferences)
}

func TestBuildMergesQualityGraph(t *testing.T) {
	t.Parallel()

	generatedAt := time.Date(2026, time.July, 25, 4, 0, 0, 0, time.UTC)
	saved := &session.Session{
		ID:        "quality-session",
		CreatedAt: generatedAt.Add(-time.Hour),
	}
	report := graphcontracts.Node{
		ID:        "inspect/report/report-1",
		Kind:      graphcontracts.NodeQuality,
		CreatedAt: generatedAt.Add(-time.Minute),
		Provenance: graphcontracts.Provenance{
			Producer: "inspect",
		},
		Attributes: map[string]string{"entity": "report"},
	}
	finding := graphcontracts.Node{
		ID:        "inspect/finding/finding-1",
		Kind:      graphcontracts.NodeQuality,
		CreatedAt: generatedAt.Add(-time.Minute),
		Provenance: graphcontracts.Provenance{
			Producer: "inspect",
		},
		Attributes: map[string]string{"entity": "finding"},
	}
	contains := graphcontracts.Edge{
		ID:        "inspect/contains/edge-1",
		Kind:      graphcontracts.EdgeContains,
		From:      graphcontracts.Ref{Kind: report.Kind, ID: report.ID},
		To:        graphcontracts.Ref{Kind: finding.Kind, ID: finding.ID},
		CreatedAt: generatedAt.Add(-time.Minute),
		Provenance: graphcontracts.Provenance{
			Producer: "inspect",
		},
	}
	export, err := Build(Input{
		Session: saved,
		QualityObservations: []QualityObservation{{
			ID:      "quality-1",
			Subject: graphcontracts.Ref{Kind: graphcontracts.NodeExecution, ID: "hawk/session/" + saved.ID},
			Nodes:   []graphcontracts.Node{report, finding},
			Edges:   []graphcontracts.Edge{contains},
		}},
		GeneratedAt: generatedAt,
		Scope:       graphcontracts.Scope{RepositoryID: "hawk"},
	})
	if err != nil {
		t.Fatalf("Build() error = %v", err)
	}
	if findNode(export.Nodes, report.ID) == nil || findNode(export.Nodes, finding.ID) == nil {
		t.Fatal("quality subgraph was not merged")
	}
	assertEdgeKind(t, export.Edges, graphcontracts.EdgeContains)
	assertEdgeKind(t, export.Edges, graphcontracts.EdgeValidatedBy)
}

func TestBuildMergesMixedRuntimeGraph(t *testing.T) {
	t.Parallel()
	at := time.Date(2026, time.July, 25, 5, 0, 0, 0, time.UTC)
	saved := &session.Session{ID: "runtime-session", CreatedAt: at.Add(-time.Hour)}
	nodes := []graphcontracts.Node{
		{
			ID: "tok/compression/one", Kind: graphcontracts.NodeOperations, CreatedAt: at,
			Provenance: graphcontracts.Provenance{Producer: "tok"},
			Attributes: map[string]string{"entity": "compression"},
		},
		{
			ID: "tok/budget/one", Kind: graphcontracts.NodePolicy, CreatedAt: at,
			Provenance: graphcontracts.Provenance{Producer: "tok"},
			Attributes: map[string]string{"entity": "budget_decision"},
		},
		{
			ID: "tok/redaction/one", Kind: graphcontracts.NodeQuality, CreatedAt: at,
			Provenance: graphcontracts.Provenance{Producer: "tok"},
			Attributes: map[string]string{"entity": "redaction"},
		},
	}
	export, err := Build(Input{
		Session: saved,
		RuntimeObservations: []RuntimeObservation{{
			ID: "tok-1", Subject: graphcontracts.Ref{Kind: graphcontracts.NodeExecution, ID: "hawk/session/" + saved.ID},
			Nodes: nodes, OccurredAt: at,
		}},
		GeneratedAt: at, Scope: graphcontracts.Scope{RepositoryID: "hawk"},
	})
	if err != nil {
		t.Fatalf("Build() error = %v", err)
	}
	assertEdgeKind(t, export.Edges, graphcontracts.EdgeProduced)
	assertEdgeKind(t, export.Edges, graphcontracts.EdgeGovernedBy)
	assertEdgeKind(t, export.Edges, graphcontracts.EdgeValidatedBy)
}

func findNode(nodes []graphcontracts.Node, id string) *graphcontracts.Node {
	for i := range nodes {
		if nodes[i].ID == id {
			return &nodes[i]
		}
	}
	return nil
}

func assertEdgeKind(t *testing.T, edges []graphcontracts.Edge, want graphcontracts.EdgeKind) {
	t.Helper()
	for _, edge := range edges {
		if edge.Kind == want {
			return
		}
	}
	t.Fatalf("edge kind %q not found in %#v", want, edges)
}

func assertEdge(
	t *testing.T,
	edges []graphcontracts.Edge,
	from, to string,
	kind graphcontracts.EdgeKind,
) {
	t.Helper()
	for _, edge := range edges {
		if edge.From.ID == from && edge.To.ID == to && edge.Kind == kind {
			return
		}
	}
	t.Fatalf("edge %q -[%s]-> %q not found in %#v", from, kind, to, edges)
}
