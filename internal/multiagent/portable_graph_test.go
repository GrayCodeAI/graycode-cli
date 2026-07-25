package mission

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/GrayCodeAI/hawk/internal/executiongraph"
)

func TestBuildPortableGraphIncludesWaveJoinOperations(t *testing.T) {
	m := New("execute graph", Config{RepoDir: "/tmp/repo", MaxWorkers: 2})
	m.Features = []Feature{
		{ID: "f1", Description: "A", ExpectedBehavior: "A", Status: FeatureCompleted},
		{ID: "f2", Description: "B", ExpectedBehavior: "B", Status: FeaturePending},
	}
	m.WaveJoins = []WaveJoin{{
		Wave:         1,
		FeatureIDs:   []string{"f1"},
		CompletedIDs: []string{"f1"},
		StartedAt:    time.Now().Add(-time.Minute),
		CompletedAt:  time.Now(),
		Summary:      "wave 1 joined 1 feature(s), 1 completed, 0 failed",
	}}

	export, err := m.buildPortableGraph(time.Now().UTC())
	if err != nil {
		t.Fatalf("buildPortableGraph() error = %v", err)
	}
	if export.SchemaVersion != executiongraph.SchemaVersion {
		t.Fatalf("schema version = %q", export.SchemaVersion)
	}
	var foundMission, foundWave bool
	for _, node := range export.Nodes {
		switch node.Attributes["entity_type"] {
		case "mission":
			foundMission = true
		case "wave_join":
			foundWave = true
			if node.Kind != "operations" {
				t.Fatalf("wave join kind = %q, want operations", node.Kind)
			}
		}
	}
	if !foundMission || !foundWave {
		t.Fatalf("graph missing mission/wave nodes: %#v", export.Nodes)
	}
}

func TestRunWavesPersistsPortableGraphArtifact(t *testing.T) {
	m := New("test", Config{RepoDir: "/tmp/repo", MaxWorkers: 2})
	tempDir := t.TempDir()
	m.Dir = tempDir
	m.Features = []Feature{
		{ID: "f1", Description: "A", ExpectedBehavior: "A", Status: FeaturePending},
	}

	workerFn := func(_ context.Context, feat *Feature, _ string, _ Config) (*Handoff, error) {
		return &Handoff{Summary: "done " + feat.ID, TestsPassed: true}, nil
	}
	if err := m.RunWaves(context.Background(), [][]string{{"f1"}}, workerFn); err != nil {
		t.Fatalf("RunWaves() error = %v", err)
	}

	data, err := os.ReadFile(filepath.Join(tempDir, portableGraphFilename))
	if err != nil {
		t.Fatalf("read graph artifact: %v", err)
	}
	if strings.Contains(string(data), `"ExpectedBehavior":"A"`) {
		t.Fatal("graph artifact leaked raw feature payload")
	}
	var export executiongraph.Export
	if err := json.Unmarshal(data, &export); err != nil {
		t.Fatalf("unmarshal graph artifact: %v", err)
	}
	if len(export.Nodes) == 0 || len(export.Events) == 0 {
		t.Fatalf("graph artifact incomplete: %#v", export)
	}
}
