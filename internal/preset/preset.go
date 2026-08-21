// Package preset implements agent preset configurations (DSH preset/agent-presets parity).
package preset

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
)

// Source represents where a preset was loaded from.
type Source string

const (
	SourceBuiltin Source = "builtin"
	SourceUser    Source = "user"
	SourceProject Source = "project"
)

// Preset defines a reusable agent configuration manifest.
type Preset struct {
	Name           string            `json:"name" yaml:"name"`
	Description    string            `json:"description" yaml:"description"`
	SystemPrompt   string            `json:"system_prompt,omitempty" yaml:"system_prompt,omitempty"`
	Model          string            `json:"model,omitempty" yaml:"model,omitempty"`
	SubagentType   string            `json:"subagent_type,omitempty" yaml:"subagent_type,omitempty"`
	CapabilityMode string            `json:"capability_mode,omitempty" yaml:"capability_mode,omitempty"`
	SandboxMode    string            `json:"sandbox_mode,omitempty" yaml:"sandbox_mode,omitempty"`
	Tools          []string          `json:"tools,omitempty" yaml:"tools,omitempty"`
	DenyTools      []string          `json:"deny_tools,omitempty" yaml:"deny_tools,omitempty"`
	Env            map[string]string `json:"env,omitempty" yaml:"env,omitempty"`
	Source         Source            `json:"source,omitempty" yaml:"source,omitempty"`
}

// Registry manages loaded agent presets with scope-based precedence (project > user > builtin).
type Registry struct {
	mu      sync.RWMutex
	presets map[string]Preset
}

var (
	defaultRegistry     *Registry
	defaultRegistryOnce sync.Once
)

// Default returns the process-wide preset registry loaded with built-ins.
func Default() *Registry {
	defaultRegistryOnce.Do(func() {
		defaultRegistry = NewRegistry()
		defaultRegistry.RegisterBuiltins()
	})
	return defaultRegistry
}

// NewRegistry creates a new empty preset registry.
func NewRegistry() *Registry {
	return &Registry{
		presets: make(map[string]Preset),
	}
}

// Register registers a preset. Project presets override user presets which override builtins.
func (r *Registry) Register(p Preset) error {
	if p.Name == "" {
		return fmt.Errorf("preset name cannot be empty")
	}
	r.mu.Lock()
	defer r.mu.Unlock()

	normName := strings.ToLower(strings.TrimSpace(p.Name))
	p.Name = normName

	if existing, ok := r.presets[normName]; ok {
		// Respect precedence: project > user > builtin
		if shouldOverride(existing.Source, p.Source) {
			r.presets[normName] = p
		}
		return nil
	}

	r.presets[normName] = p
	return nil
}

// Get looks up a preset by name.
func (r *Registry) Get(name string) (Preset, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	normName := strings.ToLower(strings.TrimSpace(name))
	p, ok := r.presets[normName]
	return p, ok
}

// List returns all registered presets.
func (r *Registry) List() []Preset {
	r.mu.RLock()
	defer r.mu.RUnlock()

	list := make([]Preset, 0, len(r.presets))
	for _, p := range r.presets {
		list = append(list, p)
	}
	return list
}

// RegisterBuiltins registers standard core agent presets.
func (r *Registry) RegisterBuiltins() {
	builtins := []Preset{
		{
			Name:           "code-reviewer",
			Description:    "Reviews code changes for security, style, and correctness",
			SubagentType:   "explore",
			CapabilityMode: "read-only",
			SandboxMode:    "strict",
			SystemPrompt:   "You are an expert code reviewer. Analyze changes carefully for correctness, edge cases, performance, and security.",
			Tools:          []string{"Read", "Grep", "Glob", "LS"},
			DenyTools:      []string{"Write", "Patch", "DeleteFile"},
			Source:         SourceBuiltin,
		},
		{
			Name:           "security-auditor",
			Description:    "Audits codebase for secrets, vulnerabilities, and risky patterns",
			SubagentType:   "explore",
			CapabilityMode: "read-only",
			SandboxMode:    "strict",
			SystemPrompt:   "You are a cybersecurity auditor. Scan the codebase for hardcoded credentials, injection risks, and insecure configurations.",
			Tools:          []string{"Read", "Grep", "Glob", "LS", "CodeSearch"},
			DenyTools:      []string{"Write", "Patch", "Bash"},
			Source:         SourceBuiltin,
		},
		{
			Name:           "architect",
			Description:    "System architecture and implementation planning specialist",
			SubagentType:   "plan",
			CapabilityMode: "read-only",
			SandboxMode:    "workspace",
			SystemPrompt:   "You are a senior software architect. Analyze requirements, assess system structure, and produce structured implementation plans.",
			Tools:          []string{"Read", "Grep", "Glob", "LS", "CodeSearch"},
			DenyTools:      []string{"Write", "DeleteFile"},
			Source:         SourceBuiltin,
		},
		{
			Name:           "pair-programmer",
			Description:    "General interactive assistant with full coding capabilities",
			SubagentType:   "general-purpose",
			CapabilityMode: "all",
			SandboxMode:    "workspace",
			SystemPrompt:   "You are a collaborative pair programmer. Write high-quality, verified code following project conventions.",
			Source:         SourceBuiltin,
		},
		{
			Name:           "debugger",
			Description:    "Root-cause diagnostic specialist for test and runtime failures",
			SubagentType:   "general-purpose",
			CapabilityMode: "read-write",
			SandboxMode:    "workspace",
			SystemPrompt:   "You are a diagnostic debugging specialist. Investigate failures, inspect logs and call traces, and craft precise fixes.",
			Source:         SourceBuiltin,
		},
	}

	for _, p := range builtins {
		_ = r.Register(p)
	}
}

// LoadFromDir scans a directory for JSON/YAML preset definitions.
func (r *Registry) LoadFromDir(dir string, source Source) error {
	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}

	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		ext := strings.ToLower(filepath.Ext(entry.Name()))
		if ext != ".json" && ext != ".yaml" && ext != ".yml" {
			continue
		}

		path := filepath.Join(dir, entry.Name())
		data, err := os.ReadFile(path)
		if err != nil {
			continue
		}

		var p Preset
		if ext == ".json" {
			if err := json.Unmarshal(data, &p); err != nil {
				continue
			}
		} else {
			// Minimal fallback for simple YAML
			if err := parseSimpleYAML(data, &p); err != nil {
				continue
			}
		}

		if p.Name == "" {
			p.Name = strings.TrimSuffix(entry.Name(), ext)
		}
		p.Source = source
		_ = r.Register(p)
	}

	return nil
}

func shouldOverride(current, candidate Source) bool {
	weight := map[Source]int{
		SourceBuiltin: 1,
		SourceUser:    2,
		SourceProject: 3,
	}
	return weight[candidate] >= weight[current]
}

func parseSimpleYAML(data []byte, p *Preset) error {
	lines := strings.Split(string(data), "\n")
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" || strings.HasPrefix(trimmed, "#") {
			continue
		}
		parts := strings.SplitN(trimmed, ":", 2)
		if len(parts) != 2 {
			continue
		}
		key := strings.ToLower(strings.TrimSpace(parts[0]))
		val := strings.Trim(strings.TrimSpace(parts[1]), `"'`)

		switch key {
		case "name":
			p.Name = val
		case "description":
			p.Description = val
		case "system_prompt":
			p.SystemPrompt = val
		case "model":
			p.Model = val
		case "subagent_type":
			p.SubagentType = val
		case "capability_mode":
			p.CapabilityMode = val
		case "sandbox_mode":
			p.SandboxMode = val
		}
	}
	return nil
}
