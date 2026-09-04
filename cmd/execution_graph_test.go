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

	graphcontracts "github.com/GrayCodeAI/graycode-cli/internal/contracts/graph"
	policycontracts "github.com/GrayCodeAI/graycode-cli/internal/contracts/policy"
	"github.com/GrayCodeAI/graycode-cli/internal/executiongraph"
	"github.com/GrayCodeAI/graycode-cli/internal/graphjournal"
	"github.com/GrayCodeAI/graycode-cli/internal/session"
)

type stubSwiftCorrelationResolver struct {
	correlation swiftCorrelation
	err         error
}

func (s stubSwiftCorrelationResolver) Resolve(
	context.Context,
	string,
) (swiftCorrelation, error) {
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
	mission := graphcontracts.Ref{Kind: graphcontracts.NodeExecution, ID: "graycode/mission/m1"}
	export := executiongraph.Export{
		SchemaVersion: executiongraph.SchemaVersion,
		GeneratedAt:   now,
		Nodes: []graphcontracts.Node{{
			ID: mission.ID, Kind: mission.Kind, CreatedAt: now,
			Provenance: graphcontracts.Provenance{Producer: "graycode"},
		}},
		Events: []graphcontracts.Event{{
			ID: "graycode/event/mission/m1/created", Type: graphcontracts.EventCreated,
			Subject: mission, OccurredAt: now,
			Provenance: graphcontracts.Provenance{Producer: "graycode"},
		}},
	}
	dir := t.TempDir()
	data, err := json.Marshal(export)
	if err != nil {
		t.Fatalf("marshal graph: %v", err)
	}
	if writeErr := os.WriteFile(filepath.Join(dir, "mission-graph.json"), data, 0o600); writeErr != nil {
		t.Fatalf("write graph: %v", writeErr)
	}
	loaded, err := loadMissionGraphExport(dir)
	if err != nil {
		t.Fatalf("loadMissionGraphExport() error = %v", err)
	}
	if len(loaded.Nodes) != 1 || len(loaded.Events) != 1 {
		t.Fatalf("loaded graph = %#v", loaded)
	}

	export.Events[0].Subject.ID = "graycode/mission/missing"
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

	if got := executionGraphRepositoryID("custom", "/work/graycode"); got != "custom" {
		t.Fatalf("override repository ID = %q, want custom", got)
	}
	if got := executionGraphRepositoryID("", "/work/graycode"); got != "graycode" {
		t.Fatalf("derived repository ID = %q, want graycode", got)
	}
}

func TestValidateSwiftCheckpointID(t *testing.T) {
	t.Parallel()

	if err := validateSwiftCheckpointID("abc123def456"); err != nil {
		t.Fatalf("valid checkpoint rejected: %v", err)
	}
	for _, value := range []string{"", "ABC123DEF456", "not-hex-0000", "abc123"} {
		if err := validateSwiftCheckpointID(value); err == nil {
			t.Fatalf("validateSwiftCheckpointID(%q) error = nil", value)
		}
	}
}

func TestExecutionGraphExportCommand(t *testing.T) {
	t.Setenv("GRAYCODE_STATE_DIR", t.TempDir())

	saved := &session.Session{
		ID:        "graph-command-session",
		CWD:       "/workspace/graycode",
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
		ID:        "harrier/memory/context-1",
		Kind:      graphcontracts.NodeKnowledge,
		CreatedAt: saved.CreatedAt,
		Provenance: graphcontracts.Provenance{
			Producer: "harrier",
		},
		Attributes: map[string]string{
			"data_classification": "metadata_only",
			"content_sha256":      "abc123",
		},
	}
	if err := graphjournal.AppendContextGraph(
		saved.ID,
		"harrier",
		"",
		[]graphcontracts.Node{contextNode},
		nil,
		nil,
		saved.CreatedAt.Add(3*time.Second),
	); err != nil {
		t.Fatalf("graphjournal.AppendContextGraph() error = %v", err)
	}
	qualityNode := graphcontracts.Node{
		ID:        "merlin/report/quality-1",
		Kind:      graphcontracts.NodeQuality,
		CreatedAt: saved.CreatedAt,
		Provenance: graphcontracts.Provenance{
			Producer: "merlin",
		},
		Attributes: map[string]string{"entity": "report"},
	}
	if err := graphjournal.AppendQualityGraph(
		saved.ID,
		"",
		"merlin",
		"merlin",
		[]graphcontracts.Node{qualityNode},
		nil,
		nil,
		saved.CreatedAt.Add(4*time.Second),
	); err != nil {
		t.Fatalf("graphjournal.AppendQualityGraph() error = %v", err)
	}
	runtimeNode := graphcontracts.Node{
		ID: "shrike/compression/runtime-1", Kind: graphcontracts.NodeOperations,
		CreatedAt:  saved.CreatedAt,
		Provenance: graphcontracts.Provenance{Producer: "shrike"},
		Attributes: map[string]string{"entity": "compression"},
	}
	if err := graphjournal.AppendRuntimeGraph(
		saved.ID, "", "context-compaction", "shrike",
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
		"--swift-checkpoint",
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
	if export.Scope.RepositoryID != "graycode" {
		t.Fatalf("Scope.RepositoryID = %q, want graycode", export.Scope.RepositoryID)
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
	if !hasExportNodePrefix(export, "graycode/policy/") {
		t.Fatal("graph export omitted automatic policy observation")
	}
	if !hasExportNodePrefix(export, "graycode/verification/") {
		t.Fatal("graph export omitted automatic verification observation")
	}
	if !hasExportNodePrefix(export, "harrier/memory/") {
		t.Fatal("graph export omitted retrieved Harrier context")
	}
	if !hasExportNodePrefix(export, "merlin/report/") {
		t.Fatal("graph export omitted Merlin quality report")
	}
	if !hasExportNodePrefix(export, "shrike/compression/") {
		t.Fatal("graph export omitted Shrike compression operation")
	}
}

func TestBuildExecutionGraphExportComposesAuthoritativeSwiftCorrelation(t *testing.T) {
	t.Setenv("GRAYCODE_STATE_DIR", t.TempDir())

	now := time.Date(2026, time.July, 25, 6, 0, 0, 0, time.UTC)
	saved := &session.Session{
		ID:        "graycode-correlated-session",
		CWD:       "/workspace/graycode",
		CreatedAt: now.Add(-time.Hour),
		UpdatedAt: now,
	}
	if err := session.Save(saved); err != nil {
		t.Fatalf("session.Save() error = %v", err)
	}
	resolver := stubSwiftCorrelationResolver{correlation: swiftCorrelation{
		SchemaVersion:            swiftCorrelationSchemaVersion,
		GraycodeSessionID:        saved.ID,
		CheckpointLookupComplete: true,
		Matches: []swiftCorrelationMatch{
			{
				SwiftSessionID: "swift-beta",
				CheckpointIDs:  []string{"bbbbbbbbbbbb"},
				StartedAt:      now.Add(-30 * time.Minute),
			},
			{
				SwiftSessionID: "swift-alpha",
				CheckpointIDs:  []string{"aaaaaaaaaaaa", "abc123def456"},
				StartedAt:      now.Add(-40 * time.Minute),
			},
		},
	}}

	export, err := buildExecutionGraphExportWithSwift(
		[]string{saved.ID},
		"",
		[]string{"abc123def456"},
		now,
		resolver,
	)
	if err != nil {
		t.Fatalf("buildExecutionGraphExportWithSwift() error = %v", err)
	}
	for _, nodeID := range []string{
		"swift/session/swift-alpha",
		"swift/session/swift-beta",
		"swift/checkpoint/aaaaaaaaaaaa",
		"swift/checkpoint/abc123def456",
		"swift/checkpoint/bbbbbbbbbbbb",
	} {
		if findExportNode(export, nodeID) == nil {
			t.Fatalf("authoritative Swift node %q was not exported", nodeID)
		}
	}
	assertExportEdge(
		t,
		export,
		"graycode/session/"+saved.ID,
		"swift/session/swift-alpha",
		graphcontracts.EdgeReferences,
	)
	assertExportEdge(
		t,
		export,
		"swift/session/swift-alpha",
		"swift/checkpoint/abc123def456",
		graphcontracts.EdgeProduced,
	)
}

func TestBuildExecutionGraphExportSwiftLookupFailureIsFailOpen(t *testing.T) {
	t.Setenv("GRAYCODE_STATE_DIR", t.TempDir())

	now := time.Date(2026, time.July, 25, 6, 30, 0, 0, time.UTC)
	saved := &session.Session{
		ID:        "graycode-swift-fail-open",
		CWD:       "/workspace/graycode",
		CreatedAt: now.Add(-time.Hour),
		UpdatedAt: now,
	}
	if err := session.Save(saved); err != nil {
		t.Fatalf("session.Save() error = %v", err)
	}
	export, err := buildExecutionGraphExportWithSwift(
		[]string{saved.ID},
		"",
		[]string{"abc123def456"},
		now,
		stubSwiftCorrelationResolver{err: errors.New("Swift is unavailable")},
	)
	if err != nil {
		t.Fatalf("Swift lookup should not block graph export: %v", err)
	}
	if findExportNode(export, "swift/checkpoint/abc123def456") == nil {
		t.Fatal("explicit checkpoint reference was lost when automatic lookup failed")
	}
	if findExportNode(export, "swift/session/swift-untrusted") != nil {
		t.Fatal("failed lookup produced a speculative Swift session")
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
