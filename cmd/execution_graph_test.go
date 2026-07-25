package cmd

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	graphcontracts "github.com/GrayCodeAI/hawk-core-contracts/graph"
	policycontracts "github.com/GrayCodeAI/hawk-core-contracts/policy"
	"github.com/GrayCodeAI/hawk/internal/executiongraph"
	"github.com/GrayCodeAI/hawk/internal/graphjournal"
	"github.com/GrayCodeAI/hawk/internal/session"
)

type stubTraceCorrelationResolver struct {
	correlation traceCorrelation
	err         error
}

func (s stubTraceCorrelationResolver) Resolve(
	context.Context,
	string,
) (traceCorrelation, error) {
	return s.correlation, s.err
}

func TestExecutionGraphCommandIsVisible(t *testing.T) {
	t.Parallel()

	command, _, err := rootCmd.Find([]string{"graph", "export"})
	if err != nil {
		t.Fatalf("find graph export: %v", err)
	}
	if command.Hidden {
		t.Fatal("graph export command should be visible")
	}
}

func TestLoadMissionGraphExportValidatesTopology(t *testing.T) {
	t.Parallel()

	now := time.Date(2026, time.July, 25, 7, 0, 0, 0, time.UTC)
	mission := graphcontracts.Ref{Kind: graphcontracts.NodeExecution, ID: "hawk/mission/m1"}
	export := executiongraph.Export{
		SchemaVersion: executiongraph.SchemaVersion,
		GeneratedAt:   now,
		Nodes: []graphcontracts.Node{{
			ID: mission.ID, Kind: mission.Kind, CreatedAt: now,
			Provenance: graphcontracts.Provenance{Producer: "hawk"},
		}},
		Events: []graphcontracts.Event{{
			ID: "hawk/event/mission/m1/created", Type: graphcontracts.EventCreated,
			Subject: mission, OccurredAt: now,
			Provenance: graphcontracts.Provenance{Producer: "hawk"},
		}},
	}
	dir := t.TempDir()
	data, err := json.Marshal(export)
	if err != nil {
		t.Fatalf("marshal graph: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "mission-graph.json"), data, 0o600); err != nil {
		t.Fatalf("write graph: %v", err)
	}
	loaded, err := loadMissionGraphExport(dir)
	if err != nil {
		t.Fatalf("loadMissionGraphExport() error = %v", err)
	}
	if len(loaded.Nodes) != 1 || len(loaded.Events) != 1 {
		t.Fatalf("loaded graph = %#v", loaded)
	}

	export.Events[0].Subject.ID = "hawk/mission/missing"
	data, _ = json.Marshal(export)
	if err := os.WriteFile(filepath.Join(dir, "mission-graph.json"), data, 0o600); err != nil {
		t.Fatalf("rewrite graph: %v", err)
	}
	if _, err := loadMissionGraphExport(dir); err == nil {
		t.Fatal("loadMissionGraphExport() error = nil for dangling event")
	}
}

func TestExecutionGraphRepositoryID(t *testing.T) {
	t.Parallel()

	if got := executionGraphRepositoryID("custom", "/work/hawk"); got != "custom" {
		t.Fatalf("override repository ID = %q, want custom", got)
	}
	if got := executionGraphRepositoryID("", "/work/hawk"); got != "hawk" {
		t.Fatalf("derived repository ID = %q, want hawk", got)
	}
}

func TestValidateTraceCheckpointID(t *testing.T) {
	t.Parallel()

	if err := validateTraceCheckpointID("abc123def456"); err != nil {
		t.Fatalf("valid checkpoint rejected: %v", err)
	}
	for _, value := range []string{"", "ABC123DEF456", "not-hex-0000", "abc123"} {
		if err := validateTraceCheckpointID(value); err == nil {
			t.Fatalf("validateTraceCheckpointID(%q) error = nil", value)
		}
	}
}

func TestExecutionGraphExportCommand(t *testing.T) {
	t.Setenv("HAWK_STATE_DIR", t.TempDir())

	saved := &session.Session{
		ID:        "graph-command-session",
		CWD:       "/workspace/hawk",
		CreatedAt: time.Date(2026, time.July, 25, 4, 0, 0, 0, time.UTC),
		Messages: []session.Message{{
			Role:    "user",
			Content: "do not expose this prompt",
		}},
	}
	if err := session.Save(saved); err != nil {
		t.Fatalf("session.Save() error = %v", err)
	}
	if err := graphjournal.AppendPolicy(
		saved.ID,
		"unpersisted-tool",
		"permission",
		policycontracts.Deny("do not expose this policy reason", "test-rule"),
		saved.CreatedAt.Add(time.Second),
	); err != nil {
		t.Fatalf("graphjournal.AppendPolicy() error = %v", err)
	}
	if err := graphjournal.AppendVerification(
		saved.ID,
		"unpersisted-tool",
		"verify-plan-execution",
		false,
		0,
		"info",
		"do not expose this target",
		saved.CreatedAt.Add(2*time.Second),
	); err != nil {
		t.Fatalf("graphjournal.AppendVerification() error = %v", err)
	}
	contextNode := graphcontracts.Node{
		ID:        "yaad/memory/context-1",
		Kind:      graphcontracts.NodeKnowledge,
		CreatedAt: saved.CreatedAt,
		Provenance: graphcontracts.Provenance{
			Producer: "yaad",
		},
		Attributes: map[string]string{
			"data_classification": "metadata_only",
			"content_sha256":      "abc123",
		},
	}
	if err := graphjournal.AppendContextGraph(
		saved.ID,
		"yaad",
		"",
		[]graphcontracts.Node{contextNode},
		nil,
		nil,
		saved.CreatedAt.Add(3*time.Second),
	); err != nil {
		t.Fatalf("graphjournal.AppendContextGraph() error = %v", err)
	}
	qualityNode := graphcontracts.Node{
		ID:        "inspect/report/quality-1",
		Kind:      graphcontracts.NodeQuality,
		CreatedAt: saved.CreatedAt,
		Provenance: graphcontracts.Provenance{
			Producer: "inspect",
		},
		Attributes: map[string]string{"entity": "report"},
	}
	if err := graphjournal.AppendQualityGraph(
		saved.ID,
		"",
		"inspect",
		"inspect",
		[]graphcontracts.Node{qualityNode},
		nil,
		nil,
		saved.CreatedAt.Add(4*time.Second),
	); err != nil {
		t.Fatalf("graphjournal.AppendQualityGraph() error = %v", err)
	}
	runtimeNode := graphcontracts.Node{
		ID: "tok/compression/runtime-1", Kind: graphcontracts.NodeOperations,
		CreatedAt:  saved.CreatedAt,
		Provenance: graphcontracts.Provenance{Producer: "tok"},
		Attributes: map[string]string{"entity": "compression"},
	}
	if err := graphjournal.AppendRuntimeGraph(
		saved.ID, "", "context-compaction", "tok",
		[]graphcontracts.Node{runtimeNode}, nil, nil,
		saved.CreatedAt.Add(5*time.Second),
	); err != nil {
		t.Fatalf("graphjournal.AppendRuntimeGraph() error = %v", err)
	}

	command := newExecutionGraphCmd()
	var output bytes.Buffer
	command.SetOut(&output)
	command.SetErr(&output)
	command.SetArgs([]string{
		"export",
		saved.ID,
		"--trace-checkpoint",
		"abc123def456",
	})
	if err := command.Execute(); err != nil {
		t.Fatalf("graph export command error = %v", err)
	}

	var export executiongraph.Export
	if err := json.Unmarshal(output.Bytes(), &export); err != nil {
		t.Fatalf("decode graph export: %v\n%s", err, output.String())
	}
	if export.SchemaVersion != executiongraph.SchemaVersion {
		t.Fatalf("SchemaVersion = %q, want %q", export.SchemaVersion, executiongraph.SchemaVersion)
	}
	if export.Scope.RepositoryID != "hawk" {
		t.Fatalf("Scope.RepositoryID = %q, want hawk", export.Scope.RepositoryID)
	}
	if output.String() == "" {
		t.Fatal("graph export command produced no output")
	}
	for _, secret := range []string{
		"do not expose this prompt",
		"do not expose this policy reason",
		"do not expose this target",
	} {
		if bytes.Contains(output.Bytes(), []byte(secret)) {
			t.Fatalf("graph export command leaked %q", secret)
		}
	}
	if !hasExportNodePrefix(export, "hawk/policy/") {
		t.Fatal("graph export omitted automatic policy observation")
	}
	if !hasExportNodePrefix(export, "hawk/verification/") {
		t.Fatal("graph export omitted automatic verification observation")
	}
	if !hasExportNodePrefix(export, "yaad/memory/") {
		t.Fatal("graph export omitted retrieved Yaad context")
	}
	if !hasExportNodePrefix(export, "inspect/report/") {
		t.Fatal("graph export omitted Inspect quality report")
	}
	if !hasExportNodePrefix(export, "tok/compression/") {
		t.Fatal("graph export omitted Tok compression operation")
	}
}

func TestBuildExecutionGraphExportComposesAuthoritativeTraceCorrelation(t *testing.T) {
	t.Setenv("HAWK_STATE_DIR", t.TempDir())

	now := time.Date(2026, time.July, 25, 6, 0, 0, 0, time.UTC)
	saved := &session.Session{
		ID:        "hawk-correlated-session",
		CWD:       "/workspace/hawk",
		CreatedAt: now.Add(-time.Hour),
		UpdatedAt: now,
	}
	if err := session.Save(saved); err != nil {
		t.Fatalf("session.Save() error = %v", err)
	}
	resolver := stubTraceCorrelationResolver{correlation: traceCorrelation{
		SchemaVersion:            traceCorrelationSchemaVersion,
		HawkSessionID:            saved.ID,
		CheckpointLookupComplete: true,
		Matches: []traceCorrelationMatch{
			{
				TraceSessionID: "trace-beta",
				CheckpointIDs:  []string{"bbbbbbbbbbbb"},
				StartedAt:      now.Add(-30 * time.Minute),
			},
			{
				TraceSessionID: "trace-alpha",
				CheckpointIDs:  []string{"aaaaaaaaaaaa", "abc123def456"},
				StartedAt:      now.Add(-40 * time.Minute),
			},
		},
	}}

	export, err := buildExecutionGraphExportWithTrace(
		[]string{saved.ID},
		"",
		[]string{"abc123def456"},
		now,
		resolver,
	)
	if err != nil {
		t.Fatalf("buildExecutionGraphExportWithTrace() error = %v", err)
	}
	for _, nodeID := range []string{
		"trace/session/trace-alpha",
		"trace/session/trace-beta",
		"trace/checkpoint/aaaaaaaaaaaa",
		"trace/checkpoint/abc123def456",
		"trace/checkpoint/bbbbbbbbbbbb",
	} {
		if findExportNode(export, nodeID) == nil {
			t.Fatalf("authoritative Trace node %q was not exported", nodeID)
		}
	}
	assertExportEdge(
		t,
		export,
		"hawk/session/"+saved.ID,
		"trace/session/trace-alpha",
		graphcontracts.EdgeReferences,
	)
	assertExportEdge(
		t,
		export,
		"trace/session/trace-alpha",
		"trace/checkpoint/abc123def456",
		graphcontracts.EdgeProduced,
	)
}

func TestBuildExecutionGraphExportTraceLookupFailureIsFailOpen(t *testing.T) {
	t.Setenv("HAWK_STATE_DIR", t.TempDir())

	now := time.Date(2026, time.July, 25, 6, 30, 0, 0, time.UTC)
	saved := &session.Session{
		ID:        "hawk-trace-fail-open",
		CWD:       "/workspace/hawk",
		CreatedAt: now.Add(-time.Hour),
		UpdatedAt: now,
	}
	if err := session.Save(saved); err != nil {
		t.Fatalf("session.Save() error = %v", err)
	}
	export, err := buildExecutionGraphExportWithTrace(
		[]string{saved.ID},
		"",
		[]string{"abc123def456"},
		now,
		stubTraceCorrelationResolver{err: errors.New("Trace is unavailable")},
	)
	if err != nil {
		t.Fatalf("Trace lookup should not block graph export: %v", err)
	}
	if findExportNode(export, "trace/checkpoint/abc123def456") == nil {
		t.Fatal("explicit checkpoint reference was lost when automatic lookup failed")
	}
	if findExportNode(export, "trace/session/trace-untrusted") != nil {
		t.Fatal("failed lookup produced a speculative Trace session")
	}
}

func hasExportNodePrefix(export executiongraph.Export, prefix string) bool {
	for _, node := range export.Nodes {
		if strings.HasPrefix(node.ID, prefix) {
			return true
		}
	}
	return false
}

func findExportNode(export executiongraph.Export, id string) *graphcontracts.Node {
	for i := range export.Nodes {
		if export.Nodes[i].ID == id {
			return &export.Nodes[i]
		}
	}
	return nil
}

func assertExportEdge(
	t *testing.T,
	export executiongraph.Export,
	from, to string,
	kind graphcontracts.EdgeKind,
) {
	t.Helper()
	for _, edge := range export.Edges {
		if edge.From.ID == from && edge.To.ID == to && edge.Kind == kind {
			return
		}
	}
	t.Fatalf("edge %q -[%s]-> %q was not exported", from, kind, to)
}
