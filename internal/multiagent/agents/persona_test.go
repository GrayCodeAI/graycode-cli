package agents

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

const samplePersonaFile = `---
name: security-reviewer
description: Security-focused code reviewer
model: claude-sonnet-4-6
provider: anthropic
expertise: [security, backend]
style: concise
temperature: 0.2
max_tokens: 8192
tools: [Read, Grep, Glob, Bash]
excluded_tools: [Write]
---
You are a security expert. Focus on OWASP Top 10 vulnerabilities.

## Rules
- Always check for SQL injection
- Flag hardcoded secrets
- Verify input validation

## Examples

### Example 1
Context: Code review
Input: Review this login handler
Output: I will check for auth bypass, credential handling, and session management
`

func TestParsePersonaFile_FullYAML(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "security-reviewer.md")
	if err := os.WriteFile(path, []byte(samplePersonaFile), 0o644); err != nil {
		t.Fatal(err)
	}

	p, err := ParsePersonaFile(path)
	if err != nil {
		t.Fatalf("ParsePersonaFile failed: %v", err)
	}

	if p.Name != "security-reviewer" {
		t.Errorf("expected name 'security-reviewer', got %q", p.Name)
	}
	if p.Description != "Security-focused code reviewer" {
		t.Errorf("unexpected description: %q", p.Description)
	}
	if p.Model != "claude-sonnet-4-6" {
		t.Errorf("expected model 'claude-sonnet-4-6', got %q", p.Model)
	}
	if p.Provider != "anthropic" {
		t.Errorf("expected provider 'anthropic', got %q", p.Provider)
	}
	if p.CommunicationStyle != "concise" {
		t.Errorf("expected style 'concise', got %q", p.CommunicationStyle)
	}
	if p.Temperature != 0.2 {
		t.Errorf("expected temperature 0.2, got %f", p.Temperature)
	}
	if p.MaxTokens != 8192 {
		t.Errorf("expected max_tokens 8192, got %d", p.MaxTokens)
	}

	// Check expertise
	if len(p.Expertise) != 2 {
		t.Fatalf("expected 2 expertise items, got %d", len(p.Expertise))
	}
	if p.Expertise[0] != "security" || p.Expertise[1] != "backend" {
		t.Errorf("unexpected expertise: %v", p.Expertise)
	}

	// Check tools
	if len(p.Tools) != 4 {
		t.Fatalf("expected 4 tools, got %d: %v", len(p.Tools), p.Tools)
	}
	if p.Tools[0] != "Read" {
		t.Errorf("expected first tool 'Read', got %q", p.Tools[0])
	}

	// Check excluded tools
	if len(p.ExcludedTools) != 1 || p.ExcludedTools[0] != "Write" {
		t.Errorf("unexpected excluded_tools: %v", p.ExcludedTools)
	}

	// Check rules parsed from body
	if len(p.Rules) < 2 {
		t.Fatalf("expected at least 2 rules, got %d", len(p.Rules))
	}
	if p.Rules[0] != "Always check for SQL injection" {
		t.Errorf("unexpected first rule: %q", p.Rules[0])
	}

	// Check examples parsed from body
	if len(p.Examples) != 1 {
		t.Fatalf("expected 1 example, got %d", len(p.Examples))
	}
	if p.Examples[0].Context != "Code review" {
		t.Errorf("unexpected example context: %q", p.Examples[0].Context)
	}
	if p.Examples[0].Input != "Review this login handler" {
		t.Errorf("unexpected example input: %q", p.Examples[0].Input)
	}

	// System prompt should contain the text before rules
	if !strings.Contains(p.SystemPrompt, "OWASP Top 10") {
		t.Error("system prompt should contain persona text")
	}
}

func TestRenderPersonaFile_RoundTrip(t *testing.T) {
	original := &Persona{
		Name:               "test-persona",
		Description:        "A test persona",
		Model:              "claude-sonnet-4-6",
		Provider:           "anthropic",
		Temperature:        0.5,
		MaxTokens:          4096,
		Expertise:          []string{"backend", "testing"},
		CommunicationStyle: "detailed",
		Tools:              []string{"Read", "Edit"},
		ExcludedTools:      []string{"Bash"},
		SystemPrompt:       "You are a test helper.",
		Rules:              []string{"Write clear assertions", "Test edge cases"},
		Examples: []PersonaExample{
			{
				Input:   "Test this function",
				Output:  "Here are the test cases...",
				Context: "Unit testing",
			},
		},
		CreatedAt: time.Date(2025, 1, 15, 10, 0, 0, 0, time.UTC),
	}

	rendered := RenderPersonaFile(original)

	// Verify the rendered content has frontmatter
	if !strings.HasPrefix(rendered, "---\n") {
		t.Error("rendered file should start with ---")
	}
	if !strings.Contains(rendered, "name: test-persona") {
		t.Error("rendered file should contain name")
	}
	if !strings.Contains(rendered, "model: claude-sonnet-4-6") {
		t.Error("rendered file should contain model")
	}
	if !strings.Contains(rendered, "expertise: [backend, testing]") {
		t.Error("rendered file should contain expertise")
	}
	if !strings.Contains(rendered, "temperature: 0.5") {
		t.Error("rendered file should contain temperature")
	}

	// Parse it back
	dir := t.TempDir()
	path := filepath.Join(dir, "test-persona.md")
	if err := os.WriteFile(path, []byte(rendered), 0o644); err != nil {
		t.Fatal(err)
	}

	parsed, err := ParsePersonaFile(path)
	if err != nil {
		t.Fatalf("round-trip parse failed: %v", err)
	}

	if parsed.Name != original.Name {
		t.Errorf("name mismatch: got %q, want %q", parsed.Name, original.Name)
	}
	if parsed.Model != original.Model {
		t.Errorf("model mismatch: got %q, want %q", parsed.Model, original.Model)
	}
	if parsed.Provider != original.Provider {
		t.Errorf("provider mismatch: got %q, want %q", parsed.Provider, original.Provider)
	}
	if parsed.Temperature != original.Temperature {
		t.Errorf("temperature mismatch: got %f, want %f", parsed.Temperature, original.Temperature)
	}
	if parsed.MaxTokens != original.MaxTokens {
		t.Errorf("max_tokens mismatch: got %d, want %d", parsed.MaxTokens, original.MaxTokens)
	}
	if len(parsed.Expertise) != len(original.Expertise) {
		t.Errorf("expertise length mismatch: got %d, want %d", len(parsed.Expertise), len(original.Expertise))
	}
	if len(parsed.Tools) != len(original.Tools) {
		t.Errorf("tools length mismatch: got %d, want %d", len(parsed.Tools), len(original.Tools))
	}
	if len(parsed.Rules) != len(original.Rules) {
		t.Errorf("rules length mismatch: got %d, want %d", len(parsed.Rules), len(original.Rules))
	}
	if len(parsed.Examples) != len(original.Examples) {
		t.Errorf("examples length mismatch: got %d, want %d", len(parsed.Examples), len(original.Examples))
	}
}

func TestSelectPersona_Security(t *testing.T) {
	r := NewPersonaRegistry(t.TempDir())
	r.Personas["security"] = &Persona{
		Name:      "security",
		Expertise: []string{"security"},
	}
	r.Personas["tester"] = &Persona{
		Name:      "tester",
		Expertise: []string{"testing"},
	}
	r.Personas["devops"] = &Persona{
		Name:      "devops",
		Expertise: []string{"devops"},
	}

	// Security task
	p := r.SelectPersona("fix security vulnerability in auth handler")
	if p == nil || p.Name != "security" {
		t.Errorf("expected security persona, got %v", p)
	}
}

func TestSelectPersona_Testing(t *testing.T) {
	r := NewPersonaRegistry(t.TempDir())
	r.Personas["security"] = &Persona{
		Name:      "security",
		Expertise: []string{"security"},
	}
	r.Personas["tester"] = &Persona{
		Name:      "tester",
		Expertise: []string{"testing"},
	}

	// Testing task
	p := r.SelectPersona("write unit tests for the parser")
	if p == nil || p.Name != "tester" {
		t.Errorf("expected tester persona, got %v", p)
	}
}

func TestSelectPersona_DevOps(t *testing.T) {
	r := NewPersonaRegistry(t.TempDir())
	r.Personas["devops"] = &Persona{
		Name:      "devops",
		Expertise: []string{"devops"},
	}
	r.Personas["backend"] = &Persona{
		Name:      "backend",
		Expertise: []string{"backend"},
	}

	// DevOps task
	p := r.SelectPersona("deploy to kubernetes cluster")
	if p == nil || p.Name != "devops" {
		t.Errorf("expected devops persona, got %v", p)
	}
}

func TestSelectPersona_FallsBackToDefault(t *testing.T) {
	r := NewPersonaRegistry(t.TempDir())
	r.Personas["default"] = &Persona{
		Name:      "default",
		Expertise: []string{},
	}
	r.Personas["security"] = &Persona{
		Name:      "security",
		Expertise: []string{"security"},
	}

	// No keyword match
	p := r.SelectPersona("do something random and unrelated")
	if p == nil || p.Name != "default" {
		t.Errorf("expected default persona as fallback, got %v", p)
	}
}

func TestSelectPersona_NoMatch(t *testing.T) {
	r := NewPersonaRegistry(t.TempDir())
	r.Personas["security"] = &Persona{
		Name:      "security",
		Expertise: []string{"security"},
	}

	// No match and no default
	p := r.SelectPersona("play some music")
	if p != nil {
		t.Errorf("expected nil when no match and no default, got %v", p)
	}
}

func TestBuildSystemPrompt_IncludesAllComponents(t *testing.T) {
	p := &Persona{
		Name:               "test",
		SystemPrompt:       "You are a test assistant.",
		Expertise:          []string{"backend", "testing"},
		CommunicationStyle: "concise",
		Rules:              []string{"Rule one", "Rule two"},
		Examples: []PersonaExample{
			{
				Input:   "example input",
				Output:  "example output",
				Context: "example context",
			},
		},
	}

	result := BuildSystemPrompt(p, "This is a Go project using REST APIs.")

	// Should contain system prompt
	if !strings.Contains(result, "You are a test assistant.") {
		t.Error("should contain system prompt")
	}

	// Should contain expertise
	if !strings.Contains(result, "backend, testing") {
		t.Error("should contain expertise")
	}

	// Should contain communication style
	if !strings.Contains(result, "brief and to the point") {
		t.Error("should contain communication style for 'concise'")
	}

	// Should contain rules
	if !strings.Contains(result, "- Rule one") {
		t.Error("should contain rules")
	}
	if !strings.Contains(result, "- Rule two") {
		t.Error("should contain rule two")
	}

	// Should contain examples
	if !strings.Contains(result, "example input") {
		t.Error("should contain example input")
	}
	if !strings.Contains(result, "example output") {
		t.Error("should contain example output")
	}
	if !strings.Contains(result, "example context") {
		t.Error("should contain example context")
	}

	// Should contain project context
	if !strings.Contains(result, "This is a Go project using REST APIs.") {
		t.Error("should contain project context")
	}
}

func TestBuildSystemPrompt_EmptyPersona(t *testing.T) {
	p := &Persona{Name: "empty"}
	result := BuildSystemPrompt(p, "")
	if result != "" {
		t.Errorf("expected empty prompt for empty persona, got %q", result)
	}
}

func TestCreateGetDelete_Lifecycle(t *testing.T) {
	dir := t.TempDir()
	r := NewPersonaRegistry(dir)

	// Create
	p := &Persona{
		Name:         "lifecycle-test",
		Description:  "Test persona for lifecycle",
		Model:        "claude-sonnet-4-6",
		Expertise:    []string{"testing"},
		SystemPrompt: "You are a lifecycle test.",
	}
	if err := r.Create(p); err != nil {
		t.Fatalf("Create failed: %v", err)
	}

	// Verify file exists
	path := filepath.Join(dir, "lifecycle-test.md")
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("persona file not created: %v", err)
	}

	// Get
	got, err := r.Get("lifecycle-test")
	if err != nil {
		t.Fatalf("Get failed: %v", err)
	}
	if got.Name != "lifecycle-test" {
		t.Errorf("expected name 'lifecycle-test', got %q", got.Name)
	}
	if got.Model != "claude-sonnet-4-6" {
		t.Errorf("expected model 'claude-sonnet-4-6', got %q", got.Model)
	}

	// Delete
	if err := r.Delete("lifecycle-test"); err != nil {
		t.Fatalf("Delete failed: %v", err)
	}

	// Verify gone
	if _, err := r.Get("lifecycle-test"); err == nil {
		t.Error("expected error after delete, got nil")
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Error("persona file should be deleted from disk")
	}
}

func TestCreate_EmptyName(t *testing.T) {
	dir := t.TempDir()
	r := NewPersonaRegistry(dir)

	err := r.Create(&Persona{})
	if err == nil {
		t.Error("expected error for empty name")
	}
}

func TestDelete_NotFound(t *testing.T) {
	dir := t.TempDir()
	r := NewPersonaRegistry(dir)

	err := r.Delete("nonexistent")
	if err == nil {
		t.Error("expected error deleting nonexistent persona")
	}
}

func TestList_ReturnsAllPersonas(t *testing.T) {
	dir := t.TempDir()
	r := NewPersonaRegistry(dir)

	r.Personas["alpha"] = &Persona{Name: "alpha"}
	r.Personas["beta"] = &Persona{Name: "beta"}
	r.Personas["gamma"] = &Persona{Name: "gamma"}

	list := r.List()
	if len(list) != 3 {
		t.Fatalf("expected 3 personas, got %d", len(list))
	}

	// Should be sorted
	if list[0].Name != "alpha" || list[1].Name != "beta" || list[2].Name != "gamma" {
		t.Errorf("expected sorted order, got %s, %s, %s", list[0].Name, list[1].Name, list[2].Name)
	}
}

func TestList_Empty(t *testing.T) {
	dir := t.TempDir()
	r := NewPersonaRegistry(dir)

	list := r.List()
	if len(list) != 0 {
		t.Errorf("expected empty list, got %d", len(list))
	}
}

func TestBuiltinPersonas_AreValid(t *testing.T) {
	builtins := BuiltinPersonas()

	expectedNames := map[string]bool{
		"default":               false,
		"reviewer":              false,
		"architect":             false,
		"debugger":              false,
		"teacher":               false,
		"speed":                 false,
		"planner":               false,
		"executor":              false,
		"critic":                false,
		"security-reviewer":     false,
		"test-engineer":         false,
		"tracer":                false,
		"verifier":              false,
		"validator":             false,
		"integrator":            false,
		"documenter":            false,
		"devops":                false,
		"performance":           false,
		"refactorer":            false,
		"cavecrew-investigator": false,
		"cavecrew-builder":      false,
		"cavecrew-reviewer":     false,
	}

	for _, p := range builtins {
		if p.Name == "" {
			t.Error("built-in persona has empty name")
		}
		if p.Description == "" {
			t.Errorf("built-in persona %q has empty description", p.Name)
		}
		if p.SystemPrompt == "" {
			t.Errorf("built-in persona %q has empty system prompt", p.Name)
		}
		if len(p.Expertise) == 0 {
			t.Errorf("built-in persona %q has no expertise", p.Name)
		}
		if p.CommunicationStyle == "" {
			t.Errorf("built-in persona %q has no communication style", p.Name)
		}
		if p.CreatedAt.IsZero() {
			t.Errorf("built-in persona %q has zero CreatedAt", p.Name)
		}
		if _, ok := expectedNames[p.Name]; ok {
			expectedNames[p.Name] = true
		} else {
			t.Errorf("unexpected built-in persona: %q", p.Name)
		}
	}

	for name, found := range expectedNames {
		if !found {
			t.Errorf("expected built-in persona %q not found", name)
		}
	}
}

func TestSelectPersona_NewDomains(t *testing.T) {
	r := NewPersonaRegistry(t.TempDir())
	for _, p := range BuiltinPersonas() {
		r.Personas[p.Name] = p
	}

	cases := []struct {
		task       string
		wantDomain string // expertise the selected persona should include
	}{
		{"profile and optimize this slow benchmark with high latency", "performance"},
		{"refactor this module to reduce technical debt and simplify", "refactoring"},
		{"write the readme and api docs with a tutorial guide", "documentation"},
		{"add observability: trace spans and structured logging", "tracing"},
	}

	for _, c := range cases {
		p := r.SelectPersona(c.task)
		if p == nil {
			t.Errorf("task %q selected nil persona", c.task)
			continue
		}
		found := false
		for _, e := range p.Expertise {
			if e == c.wantDomain {
				found = true
			}
		}
		if !found {
			t.Errorf("task %q selected %q (expertise %v), expected domain %q",
				c.task, p.Name, p.Expertise, c.wantDomain)
		}
	}
}

func TestBuiltinPersonas_Count(t *testing.T) {
	if got := len(BuiltinPersonas()); got != 22 {
		t.Errorf("expected 22 built-in personas, got %d", got)
	}
}

func TestCavecrewPersonas_ReturnsThree(t *testing.T) {
	crew := CavecrewPersonas()
	if len(crew) != 3 {
		t.Fatalf("expected 3 cavecrew personas, got %d", len(crew))
	}
	want := []string{"cavecrew-investigator", "cavecrew-builder", "cavecrew-reviewer"}
	for i, p := range crew {
		if p.Name != want[i] {
			t.Errorf("expected %d-th persona %q, got %q", i, want[i], p.Name)
		}
		if p.Description == "" {
			t.Errorf("cavecrew persona %q has empty description", p.Name)
		}
		if p.SystemPrompt == "" {
			t.Errorf("cavecrew persona %q has empty system prompt", p.Name)
		}
		if len(p.Rules) == 0 {
			t.Errorf("cavecrew persona %q has no rules", p.Name)
		}
	}
}

func TestCavecrewPersonas_AreInBuiltinList(t *testing.T) {
	// Cavecrew personas must be a subset of BuiltinPersonas so
	// EnsureBuiltins auto-creates them on first run.
	builtins := map[string]bool{}
	for _, p := range BuiltinPersonas() {
		builtins[p.Name] = true
	}
	for _, p := range CavecrewPersonas() {
		if !builtins[p.Name] {
			t.Errorf("cavecrew persona %q missing from BuiltinPersonas", p.Name)
		}
	}
}

func TestEnsureCavecrew_WritesFiles(t *testing.T) {
	dir := t.TempDir()
	r := NewPersonaRegistry(dir)
	if err := r.EnsureCavecrew(); err != nil {
		t.Fatalf("EnsureCavecrew: %v", err)
	}
	for _, want := range []string{"cavecrew-investigator.md", "cavecrew-builder.md", "cavecrew-reviewer.md"} {
		path := filepath.Join(dir, want)
		if _, err := os.Stat(path); err != nil {
			t.Errorf("expected file %s: %v", want, err)
		}
	}
}

func TestLoadAll_FromDirectory(t *testing.T) {
	dir := t.TempDir()

	// Write some persona files
	file1 := `---
name: persona-one
description: First persona
expertise: [backend]
style: concise
temperature: 0.3
---
You are persona one.
`
	file2 := `---
name: persona-two
description: Second persona
expertise: [frontend]
style: detailed
temperature: 0.7
---
You are persona two.
`
	if err := os.WriteFile(filepath.Join(dir, "persona-one.md"), []byte(file1), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "persona-two.md"), []byte(file2), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "not-a-persona.txt"), []byte("ignored"), 0o644); err != nil {
		t.Fatal(err)
	}

	r := NewPersonaRegistry(dir)
	if err := r.LoadAll(); err != nil {
		t.Fatalf("LoadAll failed: %v", err)
	}

	if len(r.Personas) != 2 {
		t.Fatalf("expected 2 personas, got %d", len(r.Personas))
	}

	p1, err := r.Get("persona-one")
	if err != nil {
		t.Fatalf("Get persona-one failed: %v", err)
	}
	if p1.Description != "First persona" {
		t.Errorf("unexpected description: %q", p1.Description)
	}
	if p1.Temperature != 0.3 {
		t.Errorf("unexpected temperature: %f", p1.Temperature)
	}

	p2, err := r.Get("persona-two")
	if err != nil {
		t.Fatalf("Get persona-two failed: %v", err)
	}
	if p2.CommunicationStyle != "detailed" {
		t.Errorf("unexpected style: %q", p2.CommunicationStyle)
	}
}

func TestLoadAll_NonexistentDirectory(t *testing.T) {
	r := NewPersonaRegistry("/tmp/nonexistent-persona-dir-xyz123")
	err := r.LoadAll()
	if err != nil {
		t.Errorf("LoadAll should not error on nonexistent dir, got: %v", err)
	}
	if len(r.Personas) != 0 {
		t.Error("should have no personas loaded")
	}
}

func TestParsePersonaFile_MissingFile(t *testing.T) {
	_, err := ParsePersonaFile("/tmp/nonexistent-persona-file-xyz.md")
	if err == nil {
		t.Error("expected error for missing file")
	}
}

func TestParsePersonaFile_InvalidYAML(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "invalid.md")

	// No frontmatter at all
	os.WriteFile(path, []byte("just plain text without frontmatter"), 0o644)
	_, err := ParsePersonaFile(path)
	if err == nil {
		t.Error("expected error for content without frontmatter")
	}
}

func TestParsePersonaFile_MissingClosingFrontmatter(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "unclosed.md")

	content := "---\nname: broken\ndescription: no closing\n"
	os.WriteFile(path, []byte(content), 0o644)
	_, err := ParsePersonaFile(path)
	if err == nil {
		t.Error("expected error for missing closing frontmatter")
	}
}

func TestParsePersonaFile_NameFromFilename(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "my-custom-agent.md")

	content := "---\ndescription: has no name field\n---\nBody text"
	os.WriteFile(path, []byte(content), 0o644)

	p, err := ParsePersonaFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if p.Name != "my-custom-agent" {
		t.Errorf("expected name from filename, got %q", p.Name)
	}
}

func TestNewPersonaRegistry_DefaultDir(t *testing.T) {
	r := NewPersonaRegistry("")
	if r.Dir == "" {
		t.Error("default dir should not be empty")
	}
	if !strings.Contains(r.Dir, ".hawk") {
		t.Errorf("default dir should contain .hawk, got %q", r.Dir)
	}
}

func TestEnsureBuiltins(t *testing.T) {
	dir := t.TempDir()
	r := NewPersonaRegistry(dir)

	if err := r.EnsureBuiltins(); err != nil {
		t.Fatalf("EnsureBuiltins failed: %v", err)
	}

	// Check files were created
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}

	expectedFiles := map[string]bool{
		"default.md":   false,
		"reviewer.md":  false,
		"architect.md": false,
		"debugger.md":  false,
		"teacher.md":   false,
		"speed.md":     false,
	}

	for _, e := range entries {
		if _, ok := expectedFiles[e.Name()]; ok {
			expectedFiles[e.Name()] = true
		}
	}

	for name, found := range expectedFiles {
		if !found {
			t.Errorf("expected built-in file %q not found", name)
		}
	}

	// Calling again should not overwrite existing files
	// Modify a file and verify it is not overwritten
	customContent := "---\nname: default\ndescription: custom\n---\nCustom prompt."
	os.WriteFile(filepath.Join(dir, "default.md"), []byte(customContent), 0o644)

	if err := r.EnsureBuiltins(); err != nil {
		t.Fatal(err)
	}

	data, _ := os.ReadFile(filepath.Join(dir, "default.md"))
	if !strings.Contains(string(data), "Custom prompt.") {
		t.Error("EnsureBuiltins should not overwrite existing files")
	}
}

func TestBuildSystemPrompt_AllStyles(t *testing.T) {
	styles := map[string]string{
		"concise":          "brief and to the point",
		"detailed":         "thorough explanations",
		"tutorial":         "step by step",
		"pair-programming": "Collaborate interactively",
	}

	for style, expected := range styles {
		p := &Persona{
			Name:               "test",
			SystemPrompt:       "Base prompt.",
			CommunicationStyle: style,
		}
		result := BuildSystemPrompt(p, "")
		if !strings.Contains(result, expected) {
			t.Errorf("style %q: expected to contain %q, got: %s", style, expected, result)
		}
	}
}

func TestPersonaRegistry_ConcurrentAccess(t *testing.T) {
	dir := t.TempDir()
	r := NewPersonaRegistry(dir)

	// Pre-populate
	for i := 0; i < 10; i++ {
		name := fmt.Sprintf("persona-%d", i)
		r.Personas[name] = &Persona{
			Name:      name,
			Expertise: []string{"backend"},
		}
	}

	// Concurrent reads
	done := make(chan bool, 20)
	for i := 0; i < 10; i++ {
		go func() {
			_ = r.List()
			done <- true
		}()
		go func(idx int) {
			name := fmt.Sprintf("persona-%d", idx)
			_, _ = r.Get(name)
			done <- true
		}(i)
	}

	for i := 0; i < 20; i++ {
		<-done
	}
}

func TestSelectPersona_MultipleKeywordMatch(t *testing.T) {
	r := NewPersonaRegistry(t.TempDir())
	r.Personas["security"] = &Persona{
		Name:      "security",
		Expertise: []string{"security"},
	}
	r.Personas["full-stack"] = &Persona{
		Name:      "full-stack",
		Expertise: []string{"security", "backend"},
	}

	// Task that matches both security and backend keywords
	p := r.SelectPersona("fix SQL injection vulnerability in the API endpoint")
	if p == nil {
		t.Fatal("expected a persona match")
	}
	// full-stack should win because it matches both security + backend keywords
	if p.Name != "full-stack" {
		t.Errorf("expected full-stack (more keyword matches), got %q", p.Name)
	}
}

func TestParseYAMLList(t *testing.T) {
	tests := []struct {
		input    string
		expected []string
	}{
		{"[a, b, c]", []string{"a", "b", "c"}},
		{"[single]", []string{"single"}},
		{"[]", nil},
		{"", nil},
		{"[Read, Grep, Glob, Bash]", []string{"Read", "Grep", "Glob", "Bash"}},
	}

	for _, tt := range tests {
		result := parseYAMLList(tt.input)
		if len(result) != len(tt.expected) {
			t.Errorf("parseYAMLList(%q): got %v, want %v", tt.input, result, tt.expected)
			continue
		}
		for i := range result {
			if result[i] != tt.expected[i] {
				t.Errorf("parseYAMLList(%q)[%d]: got %q, want %q", tt.input, i, result[i], tt.expected[i])
			}
		}
	}
}

// Ensure unused imports are referenced
var _ = time.Now

const colorHooksPersonaFile = `---
name: colorful
description: Persona with color and hooks
color: blue
hooks: {pre_run: echo start, post_run: echo done}
expertise: [backend]
---
You are a colorful agent.
`

func TestParsePersonaFile_ColorAndHooks(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "colorful.md")
	if err := os.WriteFile(path, []byte(colorHooksPersonaFile), 0o644); err != nil {
		t.Fatal(err)
	}

	p, err := ParsePersonaFile(path)
	if err != nil {
		t.Fatalf("ParsePersonaFile failed: %v", err)
	}

	if p.Color != "blue" {
		t.Errorf("expected color 'blue', got %q", p.Color)
	}
	if len(p.Hooks) != 2 {
		t.Fatalf("expected 2 hooks, got %d: %v", len(p.Hooks), p.Hooks)
	}
	if got := p.Hooks["pre_run"]; got != "echo start" {
		t.Errorf("pre_run hook = %q, want %q", got, "echo start")
	}
	if got := p.Hooks["post_run"]; got != "echo done" {
		t.Errorf("post_run hook = %q, want %q", got, "echo done")
	}
}

func TestParsePersonaFile_NoColorHooks(t *testing.T) {
	// Existing personas without color/hooks must still parse cleanly.
	dir := t.TempDir()
	path := filepath.Join(dir, "plain.md")
	if err := os.WriteFile(path, []byte(samplePersonaFile), 0o644); err != nil {
		t.Fatal(err)
	}
	p, err := ParsePersonaFile(path)
	if err != nil {
		t.Fatalf("ParsePersonaFile failed: %v", err)
	}
	if p.Color != "" {
		t.Errorf("expected empty color, got %q", p.Color)
	}
	if p.Hooks != nil {
		t.Errorf("expected nil hooks, got %v", p.Hooks)
	}
}

func TestRenderPersonaFile_ColorHooksRoundTrip(t *testing.T) {
	orig := &Persona{
		Name:      "rt",
		Color:     "#ff8800",
		Hooks:     PersonaHooks{"pre_run": "setup.sh", "on_error": "alert.sh"},
		Expertise: []string{"backend"},
	}
	rendered := RenderPersonaFile(orig)
	if !strings.Contains(rendered, "color: #ff8800") {
		t.Errorf("rendered output missing color line:\n%s", rendered)
	}
	if !strings.Contains(rendered, "hooks: {") {
		t.Errorf("rendered output missing hooks line:\n%s", rendered)
	}

	got, err := parsePersonaContent(rendered, "rt.md")
	if err != nil {
		t.Fatalf("re-parse failed: %v", err)
	}
	if got.Color != orig.Color {
		t.Errorf("round-trip color = %q, want %q", got.Color, orig.Color)
	}
	if len(got.Hooks) != len(orig.Hooks) {
		t.Fatalf("round-trip hooks count = %d, want %d", len(got.Hooks), len(orig.Hooks))
	}
	for k, v := range orig.Hooks {
		if got.Hooks[k] != v {
			t.Errorf("round-trip hook %q = %q, want %q", k, got.Hooks[k], v)
		}
	}
}

func TestParseYAMLMap(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want map[string]string
	}{
		{"empty", "", nil},
		{"empty braces", "{}", nil},
		{"single", "{a: b}", map[string]string{"a": "b"}},
		{"multi", "{a: b, c: d}", map[string]string{"a": "b", "c": "d"}},
		{"no braces", "a: b", map[string]string{"a": "b"}},
		{"quoted value", `{a: "b c"}`, map[string]string{"a": "b c"}},
		{"skips bad entry", "{a: b, junk}", map[string]string{"a": "b"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := parseYAMLMap(tt.in)
			if len(got) != len(tt.want) {
				t.Fatalf("parseYAMLMap(%q) = %v, want %v", tt.in, got, tt.want)
			}
			for k, v := range tt.want {
				if got[k] != v {
					t.Errorf("parseYAMLMap(%q)[%q] = %q, want %q", tt.in, k, got[k], v)
				}
			}
		})
	}
}
