package agents

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const agentsYAML = `researcher:
  role: Senior Researcher
  goal: Uncover cutting-edge developments in AI
  backstory: You are a seasoned researcher with a knack for finding signal.
  llm: claude-sonnet-4-6
  tools: [WebSearch, WebFetch]
writer:
  role: Technical Writer
  goal: Turn findings into clear prose
  provider: anthropic
  model: claude-haiku-4-5
`

const tasksYAML = `research_task:
  description: Research the latest in {topic}
  expected_output: A bullet-point report
  agent: researcher
`

func TestParseAgentsYAML(t *testing.T) {
	specs, err := ParseAgentsYAML([]byte(agentsYAML))
	if err != nil {
		t.Fatalf("parse error: %v", err)
	}
	if len(specs) != 2 {
		t.Fatalf("expected 2 agents, got %d", len(specs))
	}
	r := specs["researcher"]
	if r.Role != "Senior Researcher" {
		t.Errorf("role = %q", r.Role)
	}
	if r.LLM != "claude-sonnet-4-6" {
		t.Errorf("llm = %q", r.LLM)
	}
	if len(r.Tools) != 2 || r.Tools[0] != "WebSearch" {
		t.Errorf("tools = %v", r.Tools)
	}
}

func TestYAMLAgentSpec_ToPersona(t *testing.T) {
	specs, _ := ParseAgentsYAML([]byte(agentsYAML))
	p := specs["researcher"].ToPersona("researcher")
	if p.Name != "researcher" {
		t.Errorf("name = %q", p.Name)
	}
	if p.Model != "claude-sonnet-4-6" {
		t.Errorf("model = %q", p.Model)
	}
	if p.Description != "Senior Researcher" {
		t.Errorf("description = %q", p.Description)
	}
	if !strings.Contains(p.SystemPrompt, "Senior Researcher") ||
		!strings.Contains(p.SystemPrompt, "Uncover cutting-edge") {
		t.Errorf("system prompt missing role/goal: %q", p.SystemPrompt)
	}
	if len(p.Tools) != 2 {
		t.Errorf("tools = %v", p.Tools)
	}

	// "model" key is honored when "llm" is absent.
	w := specs["writer"].ToPersona("writer")
	if w.Model != "claude-haiku-4-5" {
		t.Errorf("writer model = %q", w.Model)
	}
	if w.Provider != "anthropic" {
		t.Errorf("writer provider = %q", w.Provider)
	}
}

func TestLoadYAMLConfig_MissingFilesNotError(t *testing.T) {
	dir := t.TempDir()
	cfg, err := LoadYAMLConfig(dir)
	if err != nil {
		t.Fatalf("missing files should not error: %v", err)
	}
	if len(cfg.Agents) != 0 || len(cfg.Tasks) != 0 {
		t.Fatalf("expected empty config, got %+v", cfg)
	}
}

func TestLoadYAMLConfig_LoadsBoth(t *testing.T) {
	dir := t.TempDir()
	mustWrite(t, filepath.Join(dir, "agents.yaml"), agentsYAML)
	mustWrite(t, filepath.Join(dir, "tasks.yaml"), tasksYAML)

	cfg, err := LoadYAMLConfig(dir)
	if err != nil {
		t.Fatalf("load error: %v", err)
	}
	if len(cfg.Agents) != 2 {
		t.Fatalf("expected 2 agents, got %d", len(cfg.Agents))
	}
	rt, ok := cfg.Tasks["research_task"]
	if !ok {
		t.Fatal("research_task not loaded")
	}
	if rt.Agent != "researcher" || rt.ExpectedOutput != "A bullet-point report" {
		t.Errorf("task = %+v", rt)
	}
}

func TestLoadYAMLInto_Registry(t *testing.T) {
	dir := t.TempDir()
	mustWrite(t, filepath.Join(dir, "agents.yaml"), agentsYAML)

	reg := NewPersonaRegistry(t.TempDir())
	added, err := reg.LoadYAMLInto(dir, false)
	if err != nil {
		t.Fatalf("LoadYAMLInto error: %v", err)
	}
	if added != 2 {
		t.Fatalf("expected 2 personas added, got %d", added)
	}
	p, err := reg.Get("researcher")
	if err != nil {
		t.Fatalf("researcher not in registry: %v", err)
	}
	if p.Model != "claude-sonnet-4-6" {
		t.Errorf("model = %q", p.Model)
	}
}

func TestLoadYAMLInto_DoesNotOverwriteByDefault(t *testing.T) {
	dir := t.TempDir()
	mustWrite(t, filepath.Join(dir, "agents.yaml"), agentsYAML)

	reg := NewPersonaRegistry(t.TempDir())
	// Pre-seed a markdown-style persona with the same name.
	reg.Personas["researcher"] = &Persona{Name: "researcher", Model: "from-markdown"}

	added, err := reg.LoadYAMLInto(dir, false)
	if err != nil {
		t.Fatalf("error: %v", err)
	}
	// Only "writer" is new; "researcher" is preserved.
	if added != 1 {
		t.Fatalf("expected 1 added (writer), got %d", added)
	}
	p, _ := reg.Get("researcher")
	if p.Model != "from-markdown" {
		t.Errorf("markdown persona should take precedence, got model %q", p.Model)
	}

	// With overwrite=true the YAML version replaces it.
	added2, _ := reg.LoadYAMLInto(dir, true)
	if added2 != 2 {
		t.Fatalf("expected 2 added with overwrite, got %d", added2)
	}
	p2, _ := reg.Get("researcher")
	if p2.Model != "claude-sonnet-4-6" {
		t.Errorf("overwrite should apply YAML model, got %q", p2.Model)
	}
}

func TestParseAgentsYAML_Malformed(t *testing.T) {
	if _, err := ParseAgentsYAML([]byte("\t: not valid : yaml :")); err == nil {
		t.Fatal("expected error for malformed YAML")
	}
}

func mustWrite(t *testing.T, path, content string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
}
