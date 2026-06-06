package agents

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/GrayCodeAI/hawk/internal/home"
)

// Persona represents an enhanced agent definition with specific skills,
// model preferences, and behavioral configuration.
type Persona struct {
	Name               string           `json:"name"`
	Description        string           `json:"description"`
	Model              string           `json:"model"`
	Provider           string           `json:"provider"`
	SystemPrompt       string           `json:"system_prompt"`
	Tools              []string         `json:"tools"`
	ExcludedTools      []string         `json:"excluded_tools"`
	Temperature        float64          `json:"temperature"`
	MaxTokens          int              `json:"max_tokens"`
	Expertise          []string         `json:"expertise"`
	CommunicationStyle string           `json:"communication_style"`
	Rules              []string         `json:"rules"`
	Examples           []PersonaExample `json:"examples"`
	CreatedAt          time.Time        `json:"created_at"`
	UsageCount         int              `json:"usage_count"`
	SuccessRate        float64          `json:"success_rate"`
	// Color is an optional display color for the persona (e.g. "blue",
	// "#ff8800"), used by UIs to distinguish agents. Mirrors Claude Code's
	// per-agent color frontmatter field.
	Color string `json:"color,omitempty"`
	// Hooks maps lifecycle event names (e.g. "pre_run", "post_run") to a
	// shell command or handler string to invoke for this agent. Mirrors
	// Claude Code's per-agent hooks.
	Hooks PersonaHooks `json:"hooks,omitempty"`
}

// PersonaHooks maps a lifecycle event name to the command/handler to run.
// Common keys include "pre_run", "post_run", and "on_error", but any key is
// permitted so the set can grow without breaking the loader.
type PersonaHooks map[string]string

// PersonaExample stores an input/output example for few-shot prompting.
type PersonaExample struct {
	Input   string `json:"input"`
	Output  string `json:"output"`
	Context string `json:"context"`
}

// PersonaRegistry manages a collection of personas loaded from disk.
type PersonaRegistry struct {
	Personas map[string]*Persona
	Dir      string
	mu       sync.RWMutex
}

// NewPersonaRegistry creates a new registry with the given storage directory.
// If dir is empty, it defaults to ~/.hawk/agents/.
func NewPersonaRegistry(dir string) *PersonaRegistry {
	if dir == "" {
		home := home.Dir()
		dir = filepath.Join(home, ".hawk", "agents")
	}
	return &PersonaRegistry{
		Personas: make(map[string]*Persona),
		Dir:      dir,
	}
}

// LoadAll reads all .md files from the registry directory and populates the Personas map.
func (r *PersonaRegistry) LoadAll() error {
	r.mu.Lock()
	defer r.mu.Unlock()

	entries, err := os.ReadDir(r.Dir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return fmt.Errorf("reading persona directory %s: %w", r.Dir, err)
	}

	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".md") {
			continue
		}
		path := filepath.Join(r.Dir, e.Name())
		p, err := ParsePersonaFile(path)
		if err != nil {
			continue
		}
		r.Personas[p.Name] = p
	}
	return nil
}

// Get retrieves a persona by name.
func (r *PersonaRegistry) Get(name string) (*Persona, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	p, ok := r.Personas[name]
	if !ok {
		return nil, fmt.Errorf("persona %q not found", name)
	}
	return p, nil
}

// Create saves a new persona to disk and adds it to the registry.
func (r *PersonaRegistry) Create(persona *Persona) error {
	if persona.Name == "" {
		return fmt.Errorf("persona name is required")
	}
	if persona.CreatedAt.IsZero() {
		persona.CreatedAt = time.Now()
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	if err := os.MkdirAll(r.Dir, 0o755); err != nil {
		return fmt.Errorf("creating persona directory: %w", err)
	}

	content := RenderPersonaFile(persona)
	path := filepath.Join(r.Dir, persona.Name+".md")
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		return fmt.Errorf("writing persona file: %w", err)
	}

	r.Personas[persona.Name] = persona
	return nil
}

// Delete removes a persona from the registry and disk.
func (r *PersonaRegistry) Delete(name string) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	if _, ok := r.Personas[name]; !ok {
		return fmt.Errorf("persona %q not found", name)
	}

	path := filepath.Join(r.Dir, name+".md")
	if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("removing persona file: %w", err)
	}

	delete(r.Personas, name)
	return nil
}

// List returns all personas sorted by name.
func (r *PersonaRegistry) List() []*Persona {
	r.mu.RLock()
	defer r.mu.RUnlock()

	result := make([]*Persona, 0, len(r.Personas))
	for _, p := range r.Personas {
		result = append(result, p)
	}
	sort.Slice(result, func(i, j int) bool {
		return result[i].Name < result[j].Name
	})
	return result
}

// BuildSystemPrompt constructs a full system prompt by combining the persona's
// configuration with project-specific context.
func BuildSystemPrompt(persona *Persona, projectContext string) string {
	var sb strings.Builder

	// Base system prompt
	if persona.SystemPrompt != "" {
		sb.WriteString(persona.SystemPrompt)
		sb.WriteString("\n\n")
	}

	// Expertise section
	if len(persona.Expertise) > 0 {
		sb.WriteString("## Expertise\n")
		sb.WriteString("You specialize in: ")
		sb.WriteString(strings.Join(persona.Expertise, ", "))
		sb.WriteString("\n\n")
	}

	// Communication style
	if persona.CommunicationStyle != "" {
		sb.WriteString("## Communication Style\n")
		switch persona.CommunicationStyle {
		case "concise":
			sb.WriteString("Be brief and to the point. Minimize explanations unless asked.\n")
		case "detailed":
			sb.WriteString("Provide thorough explanations with context and reasoning.\n")
		case "tutorial":
			sb.WriteString("Explain step by step as if teaching. Include context and rationale.\n")
		case "pair-programming":
			sb.WriteString("Collaborate interactively. Think aloud, ask clarifying questions, suggest alternatives.\n")
		default:
			sb.WriteString(persona.CommunicationStyle)
			sb.WriteString("\n")
		}
		sb.WriteString("\n")
	}

	// Rules
	if len(persona.Rules) > 0 {
		sb.WriteString("## Rules\n")
		for _, rule := range persona.Rules {
			sb.WriteString("- ")
			sb.WriteString(rule)
			sb.WriteString("\n")
		}
		sb.WriteString("\n")
	}

	// Examples
	if len(persona.Examples) > 0 {
		sb.WriteString("## Examples\n")
		for i, ex := range persona.Examples {
			sb.WriteString(fmt.Sprintf("### Example %d\n", i+1))
			if ex.Context != "" {
				sb.WriteString("Context: ")
				sb.WriteString(ex.Context)
				sb.WriteString("\n")
			}
			sb.WriteString("Input: ")
			sb.WriteString(ex.Input)
			sb.WriteString("\n")
			sb.WriteString("Output: ")
			sb.WriteString(ex.Output)
			sb.WriteString("\n\n")
		}
	}

	// Project context
	if projectContext != "" {
		sb.WriteString("## Project Context\n")
		sb.WriteString(projectContext)
		sb.WriteString("\n")
	}

	return strings.TrimRight(sb.String(), "\n")
}

// expertiseKeywords maps expertise domains to keywords for task matching.
var expertiseKeywords = map[string][]string{
	"security": {
		"security", "vulnerability", "cve", "injection", "xss", "csrf", "auth", "authentication",
		"authorization", "encrypt", "secret", "owasp", "penetration", "exploit",
	},
	"testing": {
		"test", "spec", "assert", "mock", "stub", "coverage", "unit test", "integration test",
		"e2e", "benchmark", "fuzzing", "fixture",
	},
	"frontend": {
		"css", "html", "react", "vue", "angular", "component", "ui", "ux", "style",
		"responsive", "accessibility", "a11y", "dom", "browser", "tailwind",
	},
	"backend": {
		"api", "database", "sql", "server", "endpoint", "middleware", "rest", "grpc",
		"microservice", "cache", "queue", "migration",
	},
	"devops": {
		"deploy", "kubernetes", "k8s", "docker", "ci", "cd", "pipeline", "terraform",
		"ansible", "helm", "monitoring", "infrastructure", "aws", "gcp", "azure", "nginx",
	},
	"planning": {
		"plan", "roadmap", "milestone", "sprint", "decompose", "breakdown", "phases",
		"strategy", "design doc", "scope", "estimate",
	},
	"performance": {
		"perf", "performance", "latency", "throughput", "benchmark", "profile", "optimize",
		"memory leak", "cpu", "slow", "bottleneck", "allocation",
	},
	"refactoring": {
		"refactor", "cleanup", "technical debt", "simplify", "extract", "reorganize",
		"modularize", "rename", "deduplicate", "restructure",
	},
	"documentation": {
		"docs", "documentation", "readme", "changelog", "api docs", "docstring", "comment",
		"guide", "tutorial", "explain", "annotate",
	},
	"tracing": {
		"trace", "debug", "log", "logging", "observability", "instrument", "span",
		"telemetry", "monitor", "diagnose", "stack trace",
	},
	"integration": {
		"integrate", "integration", "merge", "compat", "compatibility", "adapter", "bridge",
		"interface", "contract", "interop", "reconcile",
	},
}

// SelectPersona automatically selects the best persona for a given task
// by matching keywords in the task description against persona expertise.
func (r *PersonaRegistry) SelectPersona(task string) *Persona {
	r.mu.RLock()
	defer r.mu.RUnlock()

	taskLower := strings.ToLower(task)

	type scored struct {
		persona *Persona
		score   int
	}

	var candidates []scored

	for _, p := range r.Personas {
		score := 0
		for _, domain := range p.Expertise {
			keywords, ok := expertiseKeywords[domain]
			if !ok {
				continue
			}
			for _, kw := range keywords {
				if strings.Contains(taskLower, kw) {
					score++
				}
			}
		}
		if score > 0 {
			candidates = append(candidates, scored{persona: p, score: score})
		}
	}

	if len(candidates) == 0 {
		// Return default persona if available
		if p, ok := r.Personas["default"]; ok {
			return p
		}
		return nil
	}

	sort.Slice(candidates, func(i, j int) bool {
		return candidates[i].score > candidates[j].score
	})
	return candidates[0].persona
}

// ParsePersonaFile reads a persona definition from a markdown file with YAML frontmatter.
func ParsePersonaFile(path string) (*Persona, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	return parsePersonaContent(string(data), path)
}

// parsePersonaContent parses the raw content of a persona file.
func parsePersonaContent(content, path string) (*Persona, error) {
	content = strings.TrimSpace(content)
	if !strings.HasPrefix(content, "---") {
		return nil, fmt.Errorf("persona file must start with --- frontmatter")
	}

	// Find closing ---
	rest := content[3:]
	idx := strings.Index(rest, "\n---")
	if idx < 0 {
		return nil, fmt.Errorf("persona file missing closing --- for frontmatter")
	}

	frontmatter := rest[:idx]
	body := strings.TrimSpace(rest[idx+4:])

	p := &Persona{
		SystemPrompt: body,
	}

	// Parse frontmatter
	scanner := bufio.NewScanner(strings.NewReader(frontmatter))
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}
		key, val, ok := parseYAMLLine(line)
		if !ok {
			continue
		}
		switch key {
		case "name":
			p.Name = val
		case "description":
			p.Description = val
		case "model":
			p.Model = val
		case "provider":
			p.Provider = val
		case "temperature":
			p.Temperature = parseFloat(val)
		case "max_tokens":
			p.MaxTokens = parseInt(val)
		case "style":
			p.CommunicationStyle = val
		case "expertise":
			p.Expertise = parseYAMLList(val)
		case "tools":
			p.Tools = parseYAMLList(val)
		case "excluded_tools":
			p.ExcludedTools = parseYAMLList(val)
		case "rules":
			p.Rules = parseYAMLList(val)
		case "created_at":
			if t, err := time.Parse(time.RFC3339, val); err == nil {
				p.CreatedAt = t
			}
		case "usage_count":
			p.UsageCount = parseInt(val)
		case "success_rate":
			p.SuccessRate = parseFloat(val)
		case "color":
			p.Color = val
		case "hooks":
			if h := parseYAMLMap(val); len(h) > 0 {
				p.Hooks = h
			}
		}
	}

	// Parse rules from markdown body if present
	bodyRules := parseRulesFromBody(body)
	if len(bodyRules) > 0 && len(p.Rules) == 0 {
		p.Rules = bodyRules
	}

	// Parse examples from markdown body if present
	bodyExamples := parseExamplesFromBody(body)
	if len(bodyExamples) > 0 {
		p.Examples = bodyExamples
	}

	// Derive name from filename if not set
	if p.Name == "" {
		base := filepath.Base(path)
		p.Name = strings.TrimSuffix(base, filepath.Ext(base))
	}

	return p, nil
}

// RenderPersonaFile generates a markdown file with YAML frontmatter from a Persona.
func RenderPersonaFile(persona *Persona) string {
	var sb strings.Builder

	sb.WriteString("---\n")
	sb.WriteString(fmt.Sprintf("name: %s\n", persona.Name))
	if persona.Description != "" {
		sb.WriteString(fmt.Sprintf("description: %s\n", persona.Description))
	}
	if persona.Model != "" {
		sb.WriteString(fmt.Sprintf("model: %s\n", persona.Model))
	}
	if persona.Provider != "" {
		sb.WriteString(fmt.Sprintf("provider: %s\n", persona.Provider))
	}
	if len(persona.Expertise) > 0 {
		sb.WriteString(fmt.Sprintf("expertise: [%s]\n", strings.Join(persona.Expertise, ", ")))
	}
	if persona.CommunicationStyle != "" {
		sb.WriteString(fmt.Sprintf("style: %s\n", persona.CommunicationStyle))
	}
	if persona.Temperature != 0 {
		sb.WriteString(fmt.Sprintf("temperature: %.1f\n", persona.Temperature))
	}
	if persona.MaxTokens != 0 {
		sb.WriteString(fmt.Sprintf("max_tokens: %d\n", persona.MaxTokens))
	}
	if len(persona.Tools) > 0 {
		sb.WriteString(fmt.Sprintf("tools: [%s]\n", strings.Join(persona.Tools, ", ")))
	}
	if len(persona.ExcludedTools) > 0 {
		sb.WriteString(fmt.Sprintf("excluded_tools: [%s]\n", strings.Join(persona.ExcludedTools, ", ")))
	}
	if persona.Color != "" {
		sb.WriteString(fmt.Sprintf("color: %s\n", persona.Color))
	}
	if len(persona.Hooks) > 0 {
		sb.WriteString(fmt.Sprintf("hooks: {%s}\n", renderHooks(persona.Hooks)))
	}
	if !persona.CreatedAt.IsZero() {
		sb.WriteString(fmt.Sprintf("created_at: %s\n", persona.CreatedAt.Format(time.RFC3339)))
	}
	if persona.UsageCount > 0 {
		sb.WriteString(fmt.Sprintf("usage_count: %d\n", persona.UsageCount))
	}
	if persona.SuccessRate > 0 {
		sb.WriteString(fmt.Sprintf("success_rate: %.2f\n", persona.SuccessRate))
	}
	sb.WriteString("---\n")

	// Body: system prompt
	if persona.SystemPrompt != "" {
		sb.WriteString(persona.SystemPrompt)
		sb.WriteString("\n")
	}

	// Render rules in body if present
	if len(persona.Rules) > 0 {
		if persona.SystemPrompt != "" {
			sb.WriteString("\n")
		}
		sb.WriteString("## Rules\n")
		for _, rule := range persona.Rules {
			sb.WriteString("- ")
			sb.WriteString(rule)
			sb.WriteString("\n")
		}
	}

	// Render examples in body if present
	if len(persona.Examples) > 0 {
		sb.WriteString("\n## Examples\n")
		for i, ex := range persona.Examples {
			sb.WriteString(fmt.Sprintf("\n### Example %d\n", i+1))
			if ex.Context != "" {
				sb.WriteString(fmt.Sprintf("Context: %s\n", ex.Context))
			}
			sb.WriteString(fmt.Sprintf("Input: %s\n", ex.Input))
			sb.WriteString(fmt.Sprintf("Output: %s\n", ex.Output))
		}
	}

	return sb.String()
}

// BuiltinPersonas returns the set of built-in personas that are auto-created on first run.
func BuiltinPersonas() []*Persona {
	now := time.Now()
	return []*Persona{
		{
			Name:               "default",
			Description:        "Balanced general-purpose coding assistant",
			Model:              "",
			Temperature:        0.5,
			MaxTokens:          8192,
			Expertise:          []string{"backend", "frontend", "testing"},
			CommunicationStyle: "detailed",
			SystemPrompt:       "You are a skilled software engineer. Help with coding tasks across the full stack. Write clean, idiomatic code with appropriate tests.",
			Rules: []string{
				"Follow existing code style and conventions",
				"Include error handling",
				"Suggest tests for new functionality",
			},
			CreatedAt: now,
		},
		{
			Name:               "reviewer",
			Description:        "Security and correctness focused code reviewer",
			Model:              "claude-sonnet-4-6",
			Temperature:        0.2,
			Expertise:          []string{"security", "backend", "testing"},
			CommunicationStyle: "concise",
			Tools:              []string{"Read", "Grep", "Glob", "Bash"},
			SystemPrompt:       "You are a thorough code reviewer specializing in security and correctness. Analyze code changes for vulnerabilities, bugs, and improvements.",
			Rules: []string{
				"Always check for SQL injection and XSS",
				"Flag hardcoded secrets and credentials",
				"Verify proper input validation",
				"Check error handling completeness",
				"Look for race conditions in concurrent code",
			},
			CreatedAt: now,
		},
		{
			Name:               "architect",
			Description:        "High-level system design with minimal code",
			Model:              "claude-opus-4-6",
			Temperature:        0.7,
			MaxTokens:          16384,
			Expertise:          []string{"backend", "devops"},
			CommunicationStyle: "detailed",
			ExcludedTools:      []string{"Edit", "Write"},
			SystemPrompt:       "You are a software architect. Focus on system design, API contracts, and architectural decisions. Prefer diagrams and high-level descriptions over implementation details.",
			Rules: []string{
				"Prefer high-level design over implementation",
				"Consider scalability and maintainability",
				"Document trade-offs explicitly",
				"Suggest technology choices with rationale",
			},
			CreatedAt: now,
		},
		{
			Name:               "debugger",
			Description:        "Systematic bug hunter with diagnostic approach",
			Model:              "",
			Temperature:        0.3,
			Expertise:          []string{"backend", "testing"},
			CommunicationStyle: "detailed",
			SystemPrompt:       "You are a systematic debugger. Use a scientific approach: observe symptoms, form hypotheses, design experiments, and narrow down root causes methodically.",
			Rules: []string{
				"Start by reproducing the bug",
				"Form hypotheses before diving into code",
				"Use binary search to narrow down causes",
				"Check recent changes first",
				"Verify the fix does not introduce regressions",
			},
			Examples: []PersonaExample{
				{
					Input:   "The server returns 500 on login",
					Output:  "Let me systematically diagnose this: 1) Check server logs for the stack trace, 2) Reproduce with curl, 3) Identify the failing handler, 4) Trace the auth flow",
					Context: "Web application debugging",
				},
			},
			CreatedAt: now,
		},
		{
			Name:               "teacher",
			Description:        "Explains concepts with tutorial style",
			Model:              "",
			Temperature:        0.6,
			MaxTokens:          16384,
			Expertise:          []string{"frontend", "backend", "testing"},
			CommunicationStyle: "tutorial",
			SystemPrompt:       "You are a patient teacher and mentor. Explain concepts clearly with examples. Build understanding from fundamentals up. Use analogies to clarify complex ideas.",
			Rules: []string{
				"Explain the 'why' before the 'how'",
				"Use simple analogies for complex concepts",
				"Provide runnable examples",
				"Build from simple to complex",
				"Anticipate common misconceptions",
			},
			CreatedAt: now,
		},
		{
			Name:               "speed",
			Description:        "Fast and concise, uses cheapest model",
			Model:              "claude-haiku-3-5",
			Temperature:        0.3,
			MaxTokens:          4096,
			Expertise:          []string{"backend", "frontend"},
			CommunicationStyle: "concise",
			SystemPrompt:       "Be fast and direct. Provide minimal but correct answers. Skip explanations unless asked. Prioritize working code over perfect code.",
			Rules: []string{
				"Keep responses under 200 words when possible",
				"Skip preamble and get straight to code",
				"Only explain if explicitly asked",
				"Prefer simple solutions over clever ones",
			},
			CreatedAt: now,
		},
		{
			Name:               "planner",
			Description:        "Decomposes complex tasks into ordered, actionable steps",
			Temperature:        0.4,
			MaxTokens:          8192,
			Expertise:          []string{"planning", "backend"},
			CommunicationStyle: "detailed",
			ExcludedTools:      []string{"Edit", "Write"},
			SystemPrompt:       "You are a planning specialist. Break complex problems into clear, sequential, independently-testable steps. Identify dependencies and risks before any code is written.",
			Rules: []string{
				"Always identify dependencies between steps",
				"Estimate relative effort for each step",
				"Flag blockers and risks early",
				"Order steps to keep the build green at each stage",
			},
			CreatedAt: now,
		},
		{
			Name:               "executor",
			Description:        "Focused implementer that writes code to spec",
			Temperature:        0.3,
			MaxTokens:          8192,
			Expertise:          []string{"backend", "frontend"},
			CommunicationStyle: "concise",
			SystemPrompt:       "You are a focused implementer. Given a clear spec or plan, write correct, idiomatic code that satisfies the acceptance criteria. Do not expand scope beyond what is specified.",
			Rules: []string{
				"Implement exactly what the spec requires, no more",
				"Follow existing code style and conventions",
				"Run tests after each change",
				"Stop and ask if the spec is ambiguous",
			},
			CreatedAt: now,
		},
		{
			Name:               "critic",
			Description:        "Reviews plans and code for flaws before commitment",
			Model:              "claude-sonnet-4-6",
			Temperature:        0.2,
			Expertise:          []string{"backend", "testing", "security"},
			CommunicationStyle: "concise",
			ExcludedTools:      []string{"Edit", "Write"},
			SystemPrompt:       "You are a constructive critic. Examine plans and code for gaps, risks, edge cases, and over-engineering. Default to skepticism: assume there is a flaw and try to find it.",
			Rules: []string{
				"Identify what breaks if each step fails",
				"Flag missing edge cases and error paths",
				"Call out over-engineering and unnecessary complexity",
				"Suggest simpler alternatives when they exist",
			},
			CreatedAt: now,
		},
		{
			Name:               "security-reviewer",
			Description:        "Deep security-focused code reviewer",
			Model:              "claude-sonnet-4-6",
			Temperature:        0.2,
			MaxTokens:          8192,
			Expertise:          []string{"security", "backend"},
			CommunicationStyle: "concise",
			Tools:              []string{"Read", "Grep", "Glob", "Bash"},
			ExcludedTools:      []string{"Edit", "Write"},
			SystemPrompt:       "You are a security expert. Focus on the OWASP Top 10, secret handling, authentication and authorization flaws, and input validation. Assume hostile input.",
			Rules: []string{
				"Always check for injection (SQL, command, XSS)",
				"Flag hardcoded secrets and weak crypto",
				"Verify authentication and authorization on every entry point",
				"Check for insecure deserialization and SSRF",
			},
			CreatedAt: now,
		},
		{
			Name:               "test-engineer",
			Description:        "Generates tests and analyzes coverage",
			Temperature:        0.3,
			MaxTokens:          8192,
			Expertise:          []string{"testing", "backend"},
			CommunicationStyle: "detailed",
			SystemPrompt:       "You are a test engineer. Write thorough, maintainable tests that cover happy paths, edge cases, and failure modes. Prefer table-driven tests where idiomatic.",
			Rules: []string{
				"Cover happy path, edge cases, and error paths",
				"Make tests deterministic and isolated",
				"Use table-driven tests where the language supports them",
				"Test behavior, not implementation details",
			},
			CreatedAt: now,
		},
		{
			Name:               "tracer",
			Description:        "Debugging and trace analysis specialist",
			Temperature:        0.3,
			Expertise:          []string{"tracing", "testing", "backend"},
			CommunicationStyle: "detailed",
			SystemPrompt:       "You are an observability specialist. Diagnose issues by analyzing logs, traces, and telemetry. Add instrumentation where visibility is missing.",
			Rules: []string{
				"Follow the data: logs, traces, metrics before code",
				"Reconstruct the timeline of events",
				"Add instrumentation to fill visibility gaps",
				"Correlate across services using trace IDs",
			},
			CreatedAt: now,
		},
		{
			Name:               "verifier",
			Description:        "Validates implementations against specifications",
			Model:              "claude-sonnet-4-6",
			Temperature:        0.2,
			Expertise:          []string{"testing", "backend"},
			CommunicationStyle: "concise",
			Tools:              []string{"Read", "Grep", "Glob", "Bash"},
			SystemPrompt:       "You are a verification specialist. Given a spec and an implementation, confirm whether each acceptance criterion is met. Report concrete pass/fail evidence.",
			Rules: []string{
				"Check each acceptance criterion individually",
				"Provide evidence for every pass or fail verdict",
				"Run the actual tests rather than assuming",
				"Report partial completion honestly",
			},
			CreatedAt: now,
		},
		{
			Name:               "integrator",
			Description:        "Handles merges, integration, and compatibility",
			Temperature:        0.3,
			Expertise:          []string{"integration", "backend", "devops"},
			CommunicationStyle: "detailed",
			SystemPrompt:       "You are an integration specialist. Resolve merge conflicts, reconcile interfaces, and ensure components work together. Preserve backward compatibility where required.",
			Rules: []string{
				"Preserve backward compatibility unless told otherwise",
				"Verify interface contracts on both sides",
				"Resolve conflicts by understanding intent, not just text",
				"Run integration tests after merging",
			},
			CreatedAt: now,
		},
		{
			Name:               "documenter",
			Description:        "Writes documentation and changelogs",
			Temperature:        0.5,
			MaxTokens:          16384,
			Expertise:          []string{"documentation"},
			CommunicationStyle: "tutorial",
			SystemPrompt:       "You are a technical writer. Produce clear, accurate documentation: READMEs, API docs, changelogs, and inline comments. Write for the reader who knows nothing about the change.",
			Rules: []string{
				"Lead with what the reader needs to do",
				"Include runnable examples",
				"Keep changelogs user-facing, not commit-by-commit",
				"Document the 'why' for non-obvious decisions",
			},
			CreatedAt: now,
		},
		{
			Name:               "devops",
			Description:        "CI/CD, deployment, and infrastructure specialist",
			Temperature:        0.3,
			Expertise:          []string{"devops", "backend"},
			CommunicationStyle: "detailed",
			SystemPrompt:       "You are a DevOps engineer. Handle CI/CD pipelines, containerization, deployment, and infrastructure-as-code. Prioritize reproducibility, security, and observability.",
			Rules: []string{
				"Make builds reproducible and cacheable",
				"Never bake secrets into images or configs",
				"Add health checks and observability hooks",
				"Prefer declarative infrastructure-as-code",
			},
			CreatedAt: now,
		},
		{
			Name:               "performance",
			Description:        "Performance profiling and optimization specialist",
			Temperature:        0.3,
			Expertise:          []string{"performance", "backend"},
			CommunicationStyle: "detailed",
			SystemPrompt:       "You are a performance engineer. Profile before optimizing, measure after. Focus on algorithmic complexity, allocations, and hot paths. Avoid premature optimization.",
			Rules: []string{
				"Always measure before and after optimizing",
				"Identify the actual bottleneck with profiling",
				"Prefer algorithmic improvements over micro-optimizations",
				"Document the performance impact with numbers",
			},
			CreatedAt: now,
		},
		{
			Name:               "refactorer",
			Description:        "Code cleanup and refactoring specialist",
			Temperature:        0.3,
			Expertise:          []string{"refactoring", "backend", "frontend"},
			CommunicationStyle: "concise",
			SystemPrompt:       "You are a refactoring specialist. Improve code structure without changing behavior. Make small, atomic, test-backed changes. Reduce duplication and complexity.",
			Rules: []string{
				"Never change behavior during a refactor",
				"Make small atomic moves, test after each",
				"Reduce duplication and cyclomatic complexity",
				"Ensure tests pass before and after every step",
			},
			CreatedAt: now,
		},
		// --- Cavecrew personas (built into GrayCode Hawk) ---
		// Three compact, opinionated personas for multi-agent crews.
		// Each enforces a strict output format so downstream agents
		// can parse the output mechanically.
		{
			Name:               "cavecrew-investigator",
			Description:        "Compact code investigator with strict 6-word note format",
			Temperature:        0.2,
			Expertise:          []string{"tracing", "backend", "testing"},
			CommunicationStyle: "concise",
			Tools:              []string{"Read", "Grep", "Glob", "Bash"},
			SystemPrompt:       "You are a code investigator. Read code and produce compact notes in the strict format `path:line — symbol — note` where the note is at most 6 words. Every note MUST follow that exact format. No prose, no explanations, no commentary outside the notes. Maximum 20 notes per response. Each note must be on its own line.",
			Rules: []string{
				"Every note MUST be `path:line — symbol — note`",
				"Notes are at most 6 words after the dash",
				"Never use prose, paragraphs, or headings",
				"Skip files that don't relate to the question",
				"Order notes by importance, most useful first",
			},
			Examples: []PersonaExample{
				{
					Input:   "Where is the cache invalidated?",
					Output:  "internal/cache/cache.go:42 — Invalidate() — drops all keys\ninternal/api/handlers.go:88 — put() — calls cache.Invalidate",
					Context: "Investigating cache invalidation flow",
				},
			},
			CreatedAt: now,
		},
		{
			Name:               "cavecrew-builder",
			Description:        "Focused implementer that refuses multi-file sprawl",
			Temperature:        0.3,
			Expertise:          []string{"backend", "frontend", "testing"},
			CommunicationStyle: "concise",
			SystemPrompt:       "You are a focused implementer. Given a single-file scope, write correct, idiomatic code. You HARD-REFUSE to edit 3 or more files in one task; if the work spans more than 2 files, split the work into sub-tasks and ask the caller to assign them. Do not expand scope. Do not refactor adjacent code. Do not add dependencies. Do exactly what the spec says, no more.",
			Rules: []string{
				"Hard-refuse tasks that touch 3+ files; ask the caller to split",
				"Edit at most 2 files per task",
				"Do not refactor code outside the spec",
				"Do not add new dependencies without explicit approval",
				"Run tests after the change; report pass/fail",
				"Stop and ask if the spec is ambiguous",
			},
			CreatedAt: now,
		},
		{
			Name:               "cavecrew-reviewer",
			Description:        "Strict severity-coded reviewer with emoji verdicts",
			Temperature:        0.2,
			Expertise:          []string{"security", "backend", "testing", "refactoring"},
			CommunicationStyle: "concise",
			Tools:              []string{"Read", "Grep", "Glob", "Bash"},
			ExcludedTools:      []string{"Edit", "Write"},
			SystemPrompt:       "You are a strict reviewer. Examine the proposed change and report findings using ONLY severity emojis at the start of each line. The four severities are: 🔴 blocker (must fix before merge), 🟡 major (should fix soon), 🔵 minor (nit / style), ❓ question (clarify intent). Each finding is on its own line in the format `<emoji> path:line — note`. No prose, no headings, no summary paragraphs. Maximum 30 findings.",
			Rules: []string{
				"Every finding MUST start with one of 🔴 🟡 🔵 ❓",
				"Format: `<emoji> path:line — note`",
				"Blockers (🔴) only for security, correctness, or data-loss issues",
				"Majors (🟡) for performance, maintainability, or test gaps",
				"Minors (🔵) for style, naming, or nitpicks",
				"Questions (❓) for ambiguous intent; never assume",
				"No prose, no summary, no closing remarks",
			},
			Examples: []PersonaExample{
				{
					Input:   "Review the auth refactor in PR #42",
					Output:  "🔴 internal/auth/jwt.go:18 — signature never expires, no exp claim\n🟡 internal/auth/jwt.go:55 — error message leaks signing key prefix\n🔵 internal/auth/jwt.go:1 — package comment missing\n❓ internal/auth/jwt.go:30 — why HS256 instead of RS256?",
					Context: "Reviewing JWT auth refactor",
				},
			},
			CreatedAt: now,
		},
	}
}

// CavecrewPersonas returns just the three cavecrew personas
// (investigator, builder, reviewer) built into GrayCode Hawk.
// These are a strict, format-driven subset of the full BuiltinPersonas
// list; callers that want only the cavecrew subset can use this
// function instead of BuiltinPersonas.
func CavecrewPersonas() []*Persona {
	now := time.Now()
	return []*Persona{
		{
			Name:               "cavecrew-investigator",
			Description:        "Compact code investigator with strict 6-word note format",
			Temperature:        0.2,
			Expertise:          []string{"tracing", "backend", "testing"},
			CommunicationStyle: "concise",
			Tools:              []string{"Read", "Grep", "Glob", "Bash"},
			SystemPrompt:       "You are a code investigator. Read code and produce compact notes in the strict format `path:line — symbol — note` where the note is at most 6 words. Every note MUST follow that exact format. No prose, no explanations, no commentary outside the notes. Maximum 20 notes per response. Each note must be on its own line.",
			Rules: []string{
				"Every note MUST be `path:line — symbol — note`",
				"Notes are at most 6 words after the dash",
				"Never use prose, paragraphs, or headings",
				"Skip files that don't relate to the question",
				"Order notes by importance, most useful first",
			},
			Examples: []PersonaExample{
				{
					Input:   "Where is the cache invalidated?",
					Output:  "internal/cache/cache.go:42 — Invalidate() — drops all keys\ninternal/api/handlers.go:88 — put() — calls cache.Invalidate",
					Context: "Investigating cache invalidation flow",
				},
			},
			CreatedAt: now,
		},
		{
			Name:               "cavecrew-builder",
			Description:        "Focused implementer that refuses multi-file sprawl",
			Temperature:        0.3,
			Expertise:          []string{"backend", "frontend", "testing"},
			CommunicationStyle: "concise",
			SystemPrompt:       "You are a focused implementer. Given a single-file scope, write correct, idiomatic code. You HARD-REFUSE to edit 3 or more files in one task; if the work spans more than 2 files, split the work into sub-tasks and ask the caller to assign them. Do not expand scope. Do not refactor adjacent code. Do not add dependencies. Do exactly what the spec says, no more.",
			Rules: []string{
				"Hard-refuse tasks that touch 3+ files; ask the caller to split",
				"Edit at most 2 files per task",
				"Do not refactor code outside the spec",
				"Do not add new dependencies without explicit approval",
				"Run tests after the change; report pass/fail",
				"Stop and ask if the spec is ambiguous",
			},
			CreatedAt: now,
		},
		{
			Name:               "cavecrew-reviewer",
			Description:        "Strict severity-coded reviewer with emoji verdicts",
			Temperature:        0.2,
			Expertise:          []string{"security", "backend", "testing", "refactoring"},
			CommunicationStyle: "concise",
			Tools:              []string{"Read", "Grep", "Glob", "Bash"},
			ExcludedTools:      []string{"Edit", "Write"},
			SystemPrompt:       "You are a strict reviewer. Examine the proposed change and report findings using ONLY severity emojis at the start of each line. The four severities are: 🔴 blocker (must fix before merge), 🟡 major (should fix soon), 🔵 minor (nit / style), ❓ question (clarify intent). Each finding is on its own line in the format `<emoji> path:line — note`. No prose, no headings, no summary paragraphs. Maximum 30 findings.",
			Rules: []string{
				"Every finding MUST start with one of 🔴 🟡 🔵 ❓",
				"Format: `<emoji> path:line — note`",
				"Blockers (🔴) only for security, correctness, or data-loss issues",
				"Majors (🟡) for performance, maintainability, or test gaps",
				"Minors (🔵) for style, naming, or nitpicks",
				"Questions (❓) for ambiguous intent; never assume",
				"No prose, no summary, no closing remarks",
			},
			Examples: []PersonaExample{
				{
					Input:   "Review the auth refactor in PR #42",
					Output:  "🔴 internal/auth/jwt.go:18 — signature never expires, no exp claim\n🟡 internal/auth/jwt.go:55 — error message leaks signing key prefix\n🔵 internal/auth/jwt.go:1 — package comment missing\n❓ internal/auth/jwt.go:30 — why HS256 instead of RS256?",
					Context: "Reviewing JWT auth refactor",
				},
			},
			CreatedAt: now,
		},
	}
}

// EnsureBuiltins creates the built-in personas in the directory if they do not exist.
func (r *PersonaRegistry) EnsureBuiltins() error {
	if err := os.MkdirAll(r.Dir, 0o755); err != nil {
		return fmt.Errorf("creating persona directory: %w", err)
	}

	for _, p := range BuiltinPersonas() {
		path := filepath.Join(r.Dir, p.Name+".md")
		if _, err := os.Stat(path); err == nil {
			// Already exists, skip
			continue
		}
		content := RenderPersonaFile(p)
		if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
			return fmt.Errorf("writing built-in persona %s: %w", p.Name, err)
		}
	}
	return nil
}

// EnsureCavecrew creates just the three cavecrew personas
// (investigator, builder, reviewer) if they do not exist. Callers
// who want only the compact-format-driven subset can call this
// instead of EnsureBuiltins.
//
// Cavecrew personas are already part of BuiltinPersonas, so calling
// this is a no-op after EnsureBuiltins has run.
func (r *PersonaRegistry) EnsureCavecrew() error {
	if err := os.MkdirAll(r.Dir, 0o755); err != nil {
		return fmt.Errorf("creating persona directory: %w", err)
	}
	for _, p := range CavecrewPersonas() {
		path := filepath.Join(r.Dir, p.Name+".md")
		if _, err := os.Stat(path); err == nil {
			continue
		}
		content := RenderPersonaFile(p)
		if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
			return fmt.Errorf("writing cavecrew persona %s: %w", p.Name, err)
		}
	}
	return nil
}

// --- Helper functions ---

// parseYAMLList handles inline YAML lists like [a, b, c].
func parseYAMLList(val string) []string {
	val = strings.TrimSpace(val)
	if val == "" || val == "[]" {
		return nil
	}
	// Strip brackets
	if strings.HasPrefix(val, "[") && strings.HasSuffix(val, "]") {
		val = val[1 : len(val)-1]
	}
	parts := strings.Split(val, ",")
	result := make([]string, 0, len(parts))
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p != "" {
			result = append(result, p)
		}
	}
	return result
}

// parseYAMLMap parses an inline YAML flow map like
// `{pre_run: cmd a, post_run: cmd b}` into a map. Keys and values are trimmed
// and may be quoted. Entries without a colon are skipped. Returns nil for an
// empty or `{}` value.
//
// Note: this is a deliberately small parser for inline flow maps only; it does
// not support nested maps or values that themselves contain commas. That is
// sufficient for per-agent hook commands.
func parseYAMLMap(val string) map[string]string {
	val = strings.TrimSpace(val)
	if val == "" || val == "{}" {
		return nil
	}
	if strings.HasPrefix(val, "{") && strings.HasSuffix(val, "}") {
		val = val[1 : len(val)-1]
	}
	result := make(map[string]string)
	for _, pair := range strings.Split(val, ",") {
		pair = strings.TrimSpace(pair)
		if pair == "" {
			continue
		}
		idx := strings.Index(pair, ":")
		if idx < 0 {
			continue
		}
		k := strings.TrimSpace(pair[:idx])
		v := strings.TrimSpace(pair[idx+1:])
		v = trimQuotes(v)
		k = trimQuotes(k)
		if k != "" {
			result[k] = v
		}
	}
	if len(result) == 0 {
		return nil
	}
	return result
}

// trimQuotes removes a single pair of surrounding single or double quotes.
func trimQuotes(s string) string {
	if len(s) >= 2 {
		if (s[0] == '"' && s[len(s)-1] == '"') || (s[0] == '\'' && s[len(s)-1] == '\'') {
			return s[1 : len(s)-1]
		}
	}
	return s
}

// renderHooks renders a hooks map as an inline YAML flow map body (without the
// surrounding braces), with keys sorted for deterministic output.
func renderHooks(hooks map[string]string) string {
	keys := make([]string, 0, len(hooks))
	for k := range hooks {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	parts := make([]string, 0, len(keys))
	for _, k := range keys {
		parts = append(parts, fmt.Sprintf("%s: %s", k, hooks[k]))
	}
	return strings.Join(parts, ", ")
}

// parseFloat converts a string to float64, returning 0 on failure.
func parseFloat(s string) float64 {
	var f float64
	_, _ = fmt.Sscanf(s, "%f", &f)
	return f
}

// parseInt converts a string to int, returning 0 on failure.
func parseInt(s string) int {
	var i int
	_, _ = fmt.Sscanf(s, "%d", &i)
	return i
}

// parseRulesFromBody extracts rules from ## Rules section in markdown body.
func parseRulesFromBody(body string) []string {
	rulesRe := regexp.MustCompile(`(?m)^## Rules\s*\n((?:- .+\n?)*)`)
	match := rulesRe.FindStringSubmatch(body)
	if match == nil {
		return nil
	}

	var rules []string
	lines := strings.Split(match[1], "\n")
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "- ") {
			rules = append(rules, strings.TrimPrefix(line, "- "))
		}
	}
	return rules
}

// parseExamplesFromBody extracts examples from ## Examples section in markdown body.
func parseExamplesFromBody(body string) []PersonaExample {
	examplesRe := regexp.MustCompile(`(?m)^### Example \d+\n`)
	indices := examplesRe.FindAllStringIndex(body, -1)
	if len(indices) == 0 {
		return nil
	}

	var examples []PersonaExample
	for i, idx := range indices {
		var section string
		if i+1 < len(indices) {
			section = body[idx[1]:indices[i+1][0]]
		} else {
			section = body[idx[1]:]
		}

		ex := PersonaExample{}
		lines := strings.Split(section, "\n")
		for _, line := range lines {
			line = strings.TrimSpace(line)
			if strings.HasPrefix(line, "Context: ") {
				ex.Context = strings.TrimPrefix(line, "Context: ")
			} else if strings.HasPrefix(line, "Input: ") {
				ex.Input = strings.TrimPrefix(line, "Input: ")
			} else if strings.HasPrefix(line, "Output: ") {
				ex.Output = strings.TrimPrefix(line, "Output: ")
			}
		}
		if ex.Input != "" || ex.Output != "" {
			examples = append(examples, ex)
		}
	}
	return examples
}
