package spec

import (
	"fmt"
	"os"
	"path/filepath"

	"gopkg.in/yaml.v3"
)

// Artifact defines a single document in the spec-driven development DAG.
type Artifact struct {
	ID          string   `yaml:"id"`
	Generates   string   `yaml:"generates"`             // file name or glob pattern
	Description string   `yaml:"description,omitempty"` // human-readable purpose
	Template    string   `yaml:"template,omitempty"`    // template file reference
	Instruction string   `yaml:"instruction,omitempty"` // AI prompt for creating this artifact
	Requires    []string `yaml:"requires"`              // artifact IDs that must exist first
}

// ApplyPhase defines the final implementation phase of the workflow.
type ApplyPhase struct {
	Requires    []string `yaml:"requires"`
	Tracks      string   `yaml:"tracks"`
	Instruction string   `yaml:"instruction,omitempty"`
}

// Schema defines a spec-driven development workflow as a DAG of artifacts.
type Schema struct {
	Name        string      `yaml:"name"`
	Version     int         `yaml:"version"`
	Description string      `yaml:"description,omitempty"`
	Artifacts   []Artifact  `yaml:"artifacts"`
	Apply       *ApplyPhase `yaml:"apply,omitempty"`
}

// DefaultSchema is the built-in schema derived from spec/openspec/schema.yaml.
var DefaultSchema = Schema{
	Name:        "spec-driven",
	Version:     1,
	Description: "Default workflow: proposal → specs → design → tasks",
	Artifacts: []Artifact{
		{
			ID:          "proposal",
			Generates:   "proposal.md",
			Description: "Initial proposal document outlining the change",
			Template:    "proposal.md",
			Instruction: "Create the proposal document that establishes WHY this change is needed.\n\nSections:\n- **Why**: 1-2 sentences on the problem or opportunity.\n- **What Changes**: Bullet list of changes, mark breaking changes with **BREAKING**.\n- **Capabilities**: New and modified capabilities with kebab-case identifiers.\n- **Impact**: Affected code, APIs, dependencies, or systems.\n\nKeep it concise (1-2 pages). Focus on the 'why' not the 'how'.",
			Requires:    nil,
		},
		{
			ID:          "specs",
			Generates:   "specs.md",
			Description: "Detailed specifications for the change",
			Template:    "spec.md",
			Instruction: "Create specification files that define WHAT the system should do.\n\nUse delta operations:\n- **ADDED Requirements**: New capabilities\n- **MODIFIED Requirements**: Changed behavior with full updated content\n- **REMOVED Requirements**: Deprecated features with reason and migration\n\nFormat:\n- Requirements: `### Requirement: <name>`\n- Use SHALL/MUST for normative requirements.\n- Scenarios: `#### Scenario: <name>` with WHEN/THEN format.\n- Every requirement MUST have at least one scenario.",
			Requires:    []string{"proposal"},
		},
		{
			ID:          "design",
			Generates:   "design.md",
			Description: "Technical design document with implementation details",
			Template:    "design.md",
			Instruction: "Create the design document that explains HOW to implement the change.\n\nSections:\n- **Context**: Background, current state, constraints.\n- **Goals / Non-Goals**: What this design achieves and explicitly excludes.\n- **Decisions**: Key technical choices with rationale and alternatives considered.\n- **Risks / Trade-offs**: Known limitations, things that could go wrong.\n- **Migration Plan**: Steps to deploy, rollback strategy.\n- **Open Questions**: Outstanding decisions or unknowns to resolve.\n\nFocus on architecture and approach, not line-by-line implementation.",
			Requires:    []string{"proposal"},
		},
		{
			ID:          "tasks",
			Generates:   "tasks.md",
			Description: "Implementation checklist with trackable tasks",
			Template:    "tasks.md",
			Instruction: "Create the task list that breaks down the implementation work.\n\n- Group related tasks under ## numbered headings.\n- Each task: `- [ ] X.Y Task description`\n- Tasks should be small enough to complete in one session.\n- Order tasks by dependency (what must be done first?).\n- Each task should be verifiable.",
			Requires:    []string{"specs", "design"},
		},
	},
	Apply: &ApplyPhase{
		Requires:    []string{"tasks"},
		Tracks:      "tasks.md",
		Instruction: "Read context files, work through pending tasks, mark complete as you go.\nPause if you hit blockers or need clarification.",
	},
}

// LoadSchema reads a schema YAML file from disk.
func LoadSchema(path string) (*Schema, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read schema %s: %w", path, err)
	}
	var s Schema
	if err := yaml.Unmarshal(data, &s); err != nil {
		return nil, fmt.Errorf("parse schema %s: %w", path, err)
	}
	if s.Name == "" {
		return nil, fmt.Errorf("schema %s: name is required", path)
	}
	if len(s.Artifacts) == 0 {
		return nil, fmt.Errorf("schema %s: at least one artifact is required", path)
	}
	return &s, nil
}

// FindSchema searches well-known paths for a schema by name.
// Resolution order: project overrides, then built-in defaults.
func FindSchema(name string) (*Schema, error) {
	if name == "" || name == "spec-driven" {
		s := DefaultSchema
		return &s, nil
	}
	cwd, err := os.Getwd()
	if err == nil {
		projDir := filepath.Join(cwd, ".hawk", "spec-schemas")
		path := filepath.Join(projDir, name+".yaml")
		if _, err := os.Stat(path); err == nil {
			return LoadSchema(path)
		}
	}
	return nil, fmt.Errorf("schema %q not found", name)
}

// ArtifactByID returns the artifact with the given ID, or nil.
func (s *Schema) ArtifactByID(id string) *Artifact {
	for i := range s.Artifacts {
		if s.Artifacts[i].ID == id {
			return &s.Artifacts[i]
		}
	}
	return nil
}

// IDs returns all artifact IDs in order.
func (s *Schema) IDs() []string {
	ids := make([]string, len(s.Artifacts))
	for i, a := range s.Artifacts {
		ids[i] = a.ID
	}
	return ids
}
