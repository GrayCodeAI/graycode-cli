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
}

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
		home, _ := os.UserHomeDir()
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
	"security": {"security", "vulnerability", "cve", "injection", "xss", "csrf", "auth", "authentication",
		"authorization", "encrypt", "secret", "owasp", "penetration", "exploit"},
	"testing": {"test", "spec", "assert", "mock", "stub", "coverage", "unit test", "integration test",
		"e2e", "benchmark", "fuzzing", "fixture"},
	"frontend": {"css", "html", "react", "vue", "angular", "component", "ui", "ux", "style",
		"responsive", "accessibility", "a11y", "dom", "browser", "tailwind"},
	"backend": {"api", "database", "sql", "server", "endpoint", "middleware", "rest", "grpc",
		"microservice", "cache", "queue", "migration"},
	"devops": {"deploy", "kubernetes", "k8s", "docker", "ci", "cd", "pipeline", "terraform",
		"ansible", "helm", "monitoring", "infrastructure", "aws", "gcp", "azure", "nginx"},
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

// parseFloat converts a string to float64, returning 0 on failure.
func parseFloat(s string) float64 {
	var f float64
	fmt.Sscanf(s, "%f", &f)
	return f
}

// parseInt converts a string to int, returning 0 on failure.
func parseInt(s string) int {
	var i int
	fmt.Sscanf(s, "%d", &i)
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
