package agents

import (
	"fmt"
	"os"
	"path/filepath"

	"gopkg.in/yaml.v3"
)

// This file adds an ADDITIVE loader for CrewAI-style pure-YAML agent and task
// definitions (agents.yaml / tasks.yaml) living in the project directory. It
// complements — and never replaces — the existing markdown-frontmatter persona
// loader in persona.go and the markdown agent loader in agents.go.
//
// The YAML schema mirrors the CrewAI convention where each top-level key is the
// agent/task identifier and the value is a mapping of attributes:
//
//	# agents.yaml
//	researcher:
//	  role: Senior Researcher
//	  goal: Uncover cutting-edge developments
//	  backstory: You are a seasoned researcher...
//	  llm: claude-sonnet-4-6
//	  tools: [WebSearch, WebFetch]
//
//	# tasks.yaml
//	research_task:
//	  description: Research the latest in {topic}
//	  expected_output: A bullet-point report
//	  agent: researcher
//
// Both files are optional; a missing file is not an error.

// YAMLAgentSpec is the CrewAI-style description of a single agent. All fields
// are optional so partially specified files still load.
type YAMLAgentSpec struct {
	Role      string   `yaml:"role"`
	Goal      string   `yaml:"goal"`
	Backstory string   `yaml:"backstory"`
	LLM       string   `yaml:"llm"`
	Model     string   `yaml:"model"` // alias for LLM
	Provider  string   `yaml:"provider"`
	Tools     []string `yaml:"tools"`
}

// YAMLTaskSpec is the CrewAI-style description of a single task.
type YAMLTaskSpec struct {
	Description    string   `yaml:"description"`
	ExpectedOutput string   `yaml:"expected_output"`
	Agent          string   `yaml:"agent"`
	Tools          []string `yaml:"tools"`
}

// YAMLConfig is the parsed result of loading agents.yaml and tasks.yaml.
type YAMLConfig struct {
	Agents map[string]YAMLAgentSpec
	Tasks  map[string]YAMLTaskSpec
}

// ParseAgentsYAML parses the contents of an agents.yaml file.
func ParseAgentsYAML(data []byte) (map[string]YAMLAgentSpec, error) {
	specs := map[string]YAMLAgentSpec{}
	if len(data) == 0 {
		return specs, nil
	}
	if err := yaml.Unmarshal(data, &specs); err != nil {
		return nil, fmt.Errorf("parsing agents.yaml: %w", err)
	}
	return specs, nil
}

// ParseTasksYAML parses the contents of a tasks.yaml file.
func ParseTasksYAML(data []byte) (map[string]YAMLTaskSpec, error) {
	specs := map[string]YAMLTaskSpec{}
	if len(data) == 0 {
		return specs, nil
	}
	if err := yaml.Unmarshal(data, &specs); err != nil {
		return nil, fmt.Errorf("parsing tasks.yaml: %w", err)
	}
	return specs, nil
}

// LoadYAMLConfig loads agents.yaml and tasks.yaml from dir. Missing files are
// treated as empty (not an error); only malformed YAML is reported.
func LoadYAMLConfig(dir string) (*YAMLConfig, error) {
	cfg := &YAMLConfig{
		Agents: map[string]YAMLAgentSpec{},
		Tasks:  map[string]YAMLTaskSpec{},
	}

	// #nosec G304 -- dir is caller-supplied project directory, fixed filename joined internally
	if data, err := os.ReadFile(filepath.Join(dir, "agents.yaml")); err == nil {
		agents, perr := ParseAgentsYAML(data)
		if perr != nil {
			return nil, perr
		}
		cfg.Agents = agents
	} else if !os.IsNotExist(err) {
		return nil, fmt.Errorf("reading agents.yaml: %w", err)
	}

	// #nosec G304 -- dir is caller-supplied project directory, fixed filename joined internally
	if data, err := os.ReadFile(filepath.Join(dir, "tasks.yaml")); err == nil {
		tasks, perr := ParseTasksYAML(data)
		if perr != nil {
			return nil, perr
		}
		cfg.Tasks = tasks
	} else if !os.IsNotExist(err) {
		return nil, fmt.Errorf("reading tasks.yaml: %w", err)
	}

	return cfg, nil
}

// ToPersona converts a CrewAI-style YAML agent spec into a Persona. The agent
// key from the YAML map becomes the persona Name. role/goal/backstory are
// assembled into a SystemPrompt mirroring CrewAI's prompt template.
func (s YAMLAgentSpec) ToPersona(name string) *Persona {
	model := s.LLM
	if model == "" {
		model = s.Model
	}
	p := &Persona{
		Name:         name,
		Description:  s.Role,
		Model:        model,
		Provider:     s.Provider,
		SystemPrompt: buildPersonaPrompt(s),
		Tools:        append([]string(nil), s.Tools...),
	}
	return p
}

// buildPersonaPrompt renders the CrewAI role/goal/backstory triple into a
// single system prompt string.
func buildPersonaPrompt(s YAMLAgentSpec) string {
	out := ""
	if s.Role != "" {
		out += "You are " + s.Role + ".\n"
	}
	if s.Backstory != "" {
		out += "\n" + s.Backstory + "\n"
	}
	if s.Goal != "" {
		out += "\nYour goal: " + s.Goal + "\n"
	}
	return out
}

// PersonasFromYAML loads agents.yaml from dir and returns the resulting
// personas keyed by name. Returns an empty map (not an error) when the file is
// absent.
func PersonasFromYAML(dir string) (map[string]*Persona, error) {
	cfg, err := LoadYAMLConfig(dir)
	if err != nil {
		return nil, err
	}
	personas := make(map[string]*Persona, len(cfg.Agents))
	for name, spec := range cfg.Agents {
		personas[name] = spec.ToPersona(name)
	}
	return personas, nil
}

// LoadYAMLInto loads CrewAI-style agents.yaml from dir and merges the resulting
// personas into the registry. Existing personas with the same name are NOT
// overwritten unless overwrite is true, so markdown personas take precedence by
// default. Returns the number of personas added.
func (r *PersonaRegistry) LoadYAMLInto(dir string, overwrite bool) (int, error) {
	personas, err := PersonasFromYAML(dir)
	if err != nil {
		return 0, err
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.Personas == nil {
		r.Personas = make(map[string]*Persona)
	}
	added := 0
	for name, p := range personas {
		if _, exists := r.Personas[name]; exists && !overwrite {
			continue
		}
		r.Personas[name] = p
		added++
	}
	return added, nil
}
