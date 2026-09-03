package cmd

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/GrayCodeAI/graycode-cli/internal/multiagent/agents"
)

// sampleAgentMarkdown returns a minimal valid agent definition.
func sampleAgentMarkdown(name, description, model string) string {
	return "---\nname: " + name + "\ndescription: " + description + "\nmodel: " + model + "\n---\n# " + name + "\n\nPrompt body for " + name + ".\n"
}

func TestAgentList_JSON_Empty(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("GRAYCODE_STATE_DIR", filepath.Join(dir, "state"))
	if err := os.MkdirAll(filepath.Join(dir, "state", "agents"), 0o755); err != nil {
		t.Fatal(err)
	}

	old := agentListJSON
	t.Cleanup(func() { agentListJSON = old })
	agentListJSON = true

	var buf bytes.Buffer
	agentListCmd.SetOut(&buf)
	agentListCmd.SetErr(&buf)

	if err := agentListCmd.RunE(agentListCmd, nil); err != nil {
		t.Fatalf("runAgentList returned error: %v", err)
	}

	var decoded []*agents.Agent
	if err := json.Unmarshal(buf.Bytes(), &decoded); err != nil {
		t.Fatalf("decoding JSON output %q: %v", buf.String(), err)
	}
	if decoded == nil {
		t.Fatal("expected a JSON array (even if empty), got null")
	}
	if len(decoded) != 0 {
		t.Fatalf("expected 0 agents, got %d", len(decoded))
	}
}

func TestAgentList_JSON(t *testing.T) {
	dir := t.TempDir()
	stateDir := filepath.Join(dir, "state")
	t.Setenv("GRAYCODE_STATE_DIR", stateDir)
	agentDir := filepath.Join(stateDir, "agents")
	if err := os.MkdirAll(agentDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(agentDir, "worker.md"), []byte(sampleAgentMarkdown("worker", "Does work", "claude-sonnet-4-6")), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(agentDir, "reviewer.md"), []byte(sampleAgentMarkdown("reviewer", "Reviews code", "")), 0o644); err != nil {
		t.Fatal(err)
	}

	old := agentListJSON
	t.Cleanup(func() { agentListJSON = old })
	agentListJSON = true

	var buf bytes.Buffer
	agentListCmd.SetOut(&buf)
	agentListCmd.SetErr(&buf)

	if err := agentListCmd.RunE(agentListCmd, nil); err != nil {
		t.Fatalf("runAgentList returned error: %v", err)
	}

	var decoded []*agents.Agent
	if err := json.Unmarshal(buf.Bytes(), &decoded); err != nil {
		t.Fatalf("decoding JSON output %q: %v", buf.String(), err)
	}
	if len(decoded) != 2 {
		t.Fatalf("expected 2 agents, got %d", len(decoded))
	}
	found := map[string]*agents.Agent{}
	for _, a := range decoded {
		found[a.Name] = a
	}
	if found["worker"] == nil {
		t.Fatal("missing worker agent in JSON output")
	}
	if found["worker"].Model != "claude-sonnet-4-6" {
		t.Errorf("worker model = %q, want claude-sonnet-4-6", found["worker"].Model)
	}
	if found["reviewer"] == nil {
		t.Fatal("missing reviewer agent in JSON output")
	}
	if found["reviewer"].Model != "" {
		t.Errorf("reviewer model = %q, want empty (inherit)", found["reviewer"].Model)
	}
}
