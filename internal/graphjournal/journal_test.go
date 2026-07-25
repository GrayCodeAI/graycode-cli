package graphjournal

import (
	"encoding/json"
	"os"
	"strings"
	"testing"
	"time"

	graphcontracts "github.com/GrayCodeAI/hawk-core-contracts/graph"
	policycontracts "github.com/GrayCodeAI/hawk-core-contracts/policy"
)

func TestJournalRoundTripDoesNotPersistSensitivePayloads(t *testing.T) {
	t.Setenv("HAWK_STATE_DIR", t.TempDir())
	occurredAt := time.Date(2026, time.July, 25, 4, 0, 0, 0, time.UTC)

	verdict := policycontracts.Deny("private policy reason", "protected-path")
	if err := AppendPolicy("session-1", "tool-1", "permission", verdict, occurredAt); err != nil {
		t.Fatalf("AppendPolicy() error = %v", err)
	}
	if err := AppendVerification(
		"session-1",
		"tool-1",
		"verify-plan",
		true,
		2,
		"medium",
		"private target",
		occurredAt.Add(time.Second),
	); err != nil {
		t.Fatalf("AppendVerification() error = %v", err)
	}

	entries, err := Load("session-1")
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if len(entries) != 2 {
		t.Fatalf("len(entries) = %d, want 2", len(entries))
	}
	if entries[0].Policy == nil || entries[0].Policy.Allowed {
		t.Fatal("policy denial was not preserved")
	}
	if entries[1].Verification == nil || entries[1].Verification.FindingCount != 2 {
		t.Fatal("verification summary was not preserved")
	}

	data, err := os.ReadFile(pathFor("session-1"))
	if err != nil {
		t.Fatalf("read journal: %v", err)
	}
	for _, secret := range []string{"private policy reason", "private target"} {
		if strings.Contains(string(data), secret) {
			t.Fatalf("journal leaked %q", secret)
		}
	}
	for _, entry := range entries {
		if _, err := json.Marshal(entry); err != nil {
			t.Fatalf("entry is not JSON encodable: %v", err)
		}
	}
}

func TestAppendContextGraphRoundTrip(t *testing.T) {
	t.Setenv("HAWK_STATE_DIR", t.TempDir())
	occurredAt := time.Date(2026, time.July, 25, 4, 30, 0, 0, time.UTC)
	node := graphcontracts.Node{
		ID:        "yaad/memory/memory-1",
		Kind:      graphcontracts.NodeKnowledge,
		CreatedAt: occurredAt.Add(-time.Hour),
		Provenance: graphcontracts.Provenance{
			Producer: "yaad",
		},
		Attributes: map[string]string{
			"data_classification": "metadata_only",
			"content_sha256":      digest("private memory content"),
		},
	}
	if err := AppendContextGraph(
		"session-1",
		"yaad",
		digest("private query"),
		[]graphcontracts.Node{node},
		nil,
		nil,
		occurredAt,
	); err != nil {
		t.Fatalf("AppendContextGraph() error = %v", err)
	}
	entries, err := Load("session-1")
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if len(entries) != 1 || entries[0].Context == nil {
		t.Fatalf("context entries = %#v, want one", entries)
	}
	data, err := json.Marshal(entries)
	if err != nil {
		t.Fatalf("json.Marshal() error = %v", err)
	}
	for _, secret := range []string{"private memory content", "private query"} {
		if strings.Contains(string(data), secret) {
			t.Fatalf("context journal leaked %q", secret)
		}
	}
}

func TestAppendQualityGraphRoundTrip(t *testing.T) {
	t.Setenv("HAWK_STATE_DIR", t.TempDir())
	occurredAt := time.Date(2026, time.July, 25, 5, 0, 0, 0, time.UTC)
	node := graphcontracts.Node{
		ID:        "inspect/report/report-1",
		Kind:      graphcontracts.NodeQuality,
		CreatedAt: occurredAt,
		Provenance: graphcontracts.Provenance{
			Producer: "inspect",
		},
		Attributes: map[string]string{
			"entity":        "report",
			"target_digest": digest("private target"),
		},
	}
	if err := AppendQualityGraph(
		"session-1",
		"tool-1",
		"inspect",
		"inspect",
		[]graphcontracts.Node{node},
		nil,
		nil,
		occurredAt,
	); err != nil {
		t.Fatalf("AppendQualityGraph() error = %v", err)
	}
	entries, err := Load("session-1")
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if len(entries) != 1 || entries[0].Quality == nil {
		t.Fatalf("quality entries = %#v, want one", entries)
	}
	data, err := json.Marshal(entries)
	if err != nil {
		t.Fatalf("json.Marshal() error = %v", err)
	}
	if strings.Contains(string(data), "private target") {
		t.Fatal("quality journal leaked private target")
	}
}

func TestLoadMissingJournalReturnsEmpty(t *testing.T) {
	t.Setenv("HAWK_STATE_DIR", t.TempDir())
	entries, err := Load("missing")
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if len(entries) != 0 {
		t.Fatalf("len(entries) = %d, want 0", len(entries))
	}
}
