package agents

import (
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
