package status

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestSnapshotJSONIsRedactedAndVersioned(t *testing.T) {
	s := New()
	s.Model = "provider/model"
	s.Permission.Mode = "ask"
	s.Permission.EffectiveRules = 2
	s.MCP = ComponentStatus{Configured: 1, State: "not_loaded"}
	s.Skills = ComponentStatus{Configured: 2, State: "available"}
	b, err := s.JSON()
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(b), `"schema_version": "1"`) {
		t.Fatalf("missing schema version: %s", b)
	}
	if !s.Permission.SecretRedacted {
		t.Fatal("snapshot must mark secret values as redacted")
	}
	var decoded Snapshot
	if err := json.Unmarshal(b, &decoded); err != nil {
		t.Fatal(err)
	}
	if decoded.Model != s.Model || decoded.Permission.Mode != "ask" {
		t.Fatalf("unexpected decoded snapshot: %+v", decoded)
	}
	if decoded.MCP.Configured != 1 || decoded.Skills.Configured != 2 {
		t.Fatalf("extension status was not preserved: %+v", decoded)
	}
}
