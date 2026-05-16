package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestParseBasicModelfile(t *testing.T) {
	content := `# This is a comment
FROM claude-sonnet-4-6
PARAMETER temperature 0.7
PARAMETER top_p 0.9
PARAMETER max_tokens 4096
SYSTEM "You are a helpful assistant."
MESSAGE user "Hello"
MESSAGE assistant "Hi there!"
LICENSE "MIT"
ADAPTER /path/to/lora
`
	parser := NewModelfileParser()
	cfg, err := parser.Parse(content)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if cfg.From != "claude-sonnet-4-6" {
		t.Errorf("From = %q, want %q", cfg.From, "claude-sonnet-4-6")
	}

	if temp, ok := cfg.Parameters["temperature"]; !ok || temp != 0.7 {
		t.Errorf("temperature = %v, want 0.7", temp)
	}
	if topP, ok := cfg.Parameters["top_p"]; !ok || topP != 0.9 {
		t.Errorf("top_p = %v, want 0.9", topP)
	}
	if maxTok, ok := cfg.Parameters["max_tokens"]; !ok || maxTok != 4096 {
		t.Errorf("max_tokens = %v, want 4096", maxTok)
	}

	if cfg.System != "You are a helpful assistant." {
		t.Errorf("System = %q, want %q", cfg.System, "You are a helpful assistant.")
	}

	if len(cfg.Messages) != 2 {
		t.Fatalf("len(Messages) = %d, want 2", len(cfg.Messages))
	}
	if cfg.Messages[0].Role != "user" || cfg.Messages[0].Content != "Hello" {
		t.Errorf("Messages[0] = %+v, want user/Hello", cfg.Messages[0])
	}
	if cfg.Messages[1].Role != "assistant" || cfg.Messages[1].Content != "Hi there!" {
		t.Errorf("Messages[1] = %+v, want assistant/Hi there!", cfg.Messages[1])
	}

	if cfg.License != "MIT" {
		t.Errorf("License = %q, want %q", cfg.License, "MIT")
	}

	if len(cfg.Adapters) != 1 || cfg.Adapters[0] != "/path/to/lora" {
		t.Errorf("Adapters = %v, want [/path/to/lora]", cfg.Adapters)
	}
}

func TestParseMultilineSystem(t *testing.T) {
	content := `FROM base-model
SYSTEM """You are a coding assistant.
You write clean code.
You follow best practices."""
`
	parser := NewModelfileParser()
	cfg, err := parser.Parse(content)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	expected := "You are a coding assistant.\nYou write clean code.\nYou follow best practices."
	if cfg.System != expected {
		t.Errorf("System = %q, want %q", cfg.System, expected)
	}
}

func TestParseTripleQuoteSameLine(t *testing.T) {
	content := `FROM model
SYSTEM """single line triple quoted"""
`
	parser := NewModelfileParser()
	cfg, err := parser.Parse(content)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if cfg.System != "single line triple quoted" {
		t.Errorf("System = %q, want %q", cfg.System, "single line triple quoted")
	}
}

func TestParseStopParameter(t *testing.T) {
	content := "FROM model\nPARAMETER stop \"```\"\n"
	parser := NewModelfileParser()
	cfg, err := parser.Parse(content)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if stop, ok := cfg.Parameters["stop"]; !ok || stop != "```" {
		t.Errorf("stop = %v, want \"```\"", stop)
	}
}

func TestParseTemplate(t *testing.T) {
	content := `FROM model
TEMPLATE "{{.System}}\n{{.Prompt}}"
`
	parser := NewModelfileParser()
	cfg, err := parser.Parse(content)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if cfg.Template != `{{.System}}\n{{.Prompt}}` {
		t.Errorf("Template = %q, want %q", cfg.Template, `{{.System}}\n{{.Prompt}}`)
	}
}

func TestParseErrorMissingFromValue(t *testing.T) {
	content := "FROM\n"
	parser := NewModelfileParser()
	_, err := parser.Parse(content)
	if err == nil {
		t.Fatal("expected error for empty FROM")
	}
	if !strings.Contains(err.Error(), "FROM requires a model name") {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestParseErrorUnknownDirective(t *testing.T) {
	content := "FROM model\nFOOBAR something\n"
	parser := NewModelfileParser()
	_, err := parser.Parse(content)
	if err == nil {
		t.Fatal("expected error for unknown directive")
	}
	if !strings.Contains(err.Error(), "unknown directive") {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestParseErrorUnterminatedTripleQuote(t *testing.T) {
	content := "FROM model\nSYSTEM \"\"\"unterminated\n"
	parser := NewModelfileParser()
	_, err := parser.Parse(content)
	if err == nil {
		t.Fatal("expected error for unterminated triple quote")
	}
	if !strings.Contains(err.Error(), "unterminated") {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestParseErrorParameterMissingValue(t *testing.T) {
	content := "FROM model\nPARAMETER temperature\n"
	parser := NewModelfileParser()
	_, err := parser.Parse(content)
	if err == nil {
		t.Fatal("expected error for PARAMETER missing value")
	}
}

func TestParseFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "Modelfile")
	err := os.WriteFile(path, []byte("FROM test-model\nPARAMETER temperature 0.5\n"), 0o644)
	if err != nil {
		t.Fatalf("failed to write file: %v", err)
	}

	parser := NewModelfileParser()
	cfg, err := parser.ParseFile(path)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.From != "test-model" {
		t.Errorf("From = %q, want %q", cfg.From, "test-model")
	}
}

func TestParseFileNotFound(t *testing.T) {
	parser := NewModelfileParser()
	_, err := parser.ParseFile("/nonexistent/path/Modelfile")
	if err == nil {
		t.Fatal("expected error for missing file")
	}
}

func TestValidateFromRequired(t *testing.T) {
	parser := NewModelfileParser()
	cfg := &ModelConfig{Parameters: make(map[string]interface{})}
	issues := parser.Validate(cfg)

	found := false
	for _, issue := range issues {
		if issue == "FROM is required" {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("expected 'FROM is required' in issues, got %v", issues)
	}
}

func TestValidateTemperatureRange(t *testing.T) {
	parser := NewModelfileParser()

	tests := []struct {
		name  string
		temp  interface{}
		valid bool
	}{
		{"zero", 0.0, true},
		{"mid", 1.0, true},
		{"max", 2.0, true},
		{"negative", -0.1, false},
		{"too_high", 2.1, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := &ModelConfig{
				From:       "model",
				Parameters: map[string]interface{}{"temperature": tt.temp},
			}
			issues := parser.Validate(cfg)
			hasIssue := false
			for _, issue := range issues {
				if strings.Contains(issue, "temperature") {
					hasIssue = true
					break
				}
			}
			if tt.valid && hasIssue {
				t.Errorf("temperature %v should be valid, got issues: %v", tt.temp, issues)
			}
			if !tt.valid && !hasIssue {
				t.Errorf("temperature %v should be invalid, got no issue", tt.temp)
			}
		})
	}
}

func TestValidateUnknownParameter(t *testing.T) {
	parser := NewModelfileParser()
	cfg := &ModelConfig{
		From:       "model",
		Parameters: map[string]interface{}{"bogus_param": 1.0},
	}
	issues := parser.Validate(cfg)

	found := false
	for _, issue := range issues {
		if strings.Contains(issue, "unknown parameter") && strings.Contains(issue, "bogus_param") {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("expected unknown parameter issue, got %v", issues)
	}
}

func TestValidateInvalidRole(t *testing.T) {
	parser := NewModelfileParser()
	cfg := &ModelConfig{
		From:       "model",
		Parameters: make(map[string]interface{}),
		Messages:   []ModelMessage{{Role: "invalid_role", Content: "test"}},
	}
	issues := parser.Validate(cfg)

	found := false
	for _, issue := range issues {
		if strings.Contains(issue, "invalid message role") {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("expected invalid role issue, got %v", issues)
	}
}

func TestValidateValidConfig(t *testing.T) {
	parser := NewModelfileParser()
	cfg := &ModelConfig{
		From: "model",
		Parameters: map[string]interface{}{
			"temperature": 0.5,
			"top_p":       0.9,
		},
		Messages: []ModelMessage{{Role: "user", Content: "hello"}},
	}
	issues := parser.Validate(cfg)
	if len(issues) != 0 {
		t.Errorf("expected no issues, got %v", issues)
	}
}

func TestRender(t *testing.T) {
	parser := NewModelfileParser()
	cfg := &ModelConfig{
		From: "claude-sonnet-4-6",
		Parameters: map[string]interface{}{
			"temperature": 0.7,
		},
		System:   "You are helpful.",
		Messages: []ModelMessage{{Role: "user", Content: "Hi"}},
		License:  "MIT",
		Adapters: []string{"/path/to/adapter"},
	}

	rendered := parser.Render(cfg)

	if !strings.Contains(rendered, "FROM claude-sonnet-4-6") {
		t.Error("rendered missing FROM")
	}
	if !strings.Contains(rendered, "PARAMETER temperature") {
		t.Error("rendered missing PARAMETER temperature")
	}
	if !strings.Contains(rendered, "SYSTEM") {
		t.Error("rendered missing SYSTEM")
	}
	if !strings.Contains(rendered, "MESSAGE user") {
		t.Error("rendered missing MESSAGE")
	}
	if !strings.Contains(rendered, "LICENSE") {
		t.Error("rendered missing LICENSE")
	}
	if !strings.Contains(rendered, "ADAPTER /path/to/adapter") {
		t.Error("rendered missing ADAPTER")
	}
}

func TestRenderRoundTrip(t *testing.T) {
	original := `FROM claude-sonnet-4-6
PARAMETER temperature 0.7
SYSTEM "You are helpful."
MESSAGE user "Hello"
MESSAGE assistant "Hi there"
LICENSE "MIT"
ADAPTER /path/to/lora
`
	parser := NewModelfileParser()
	cfg, err := parser.Parse(original)
	if err != nil {
		t.Fatalf("parse error: %v", err)
	}

	rendered := parser.Render(cfg)
	cfg2, err := parser.Parse(rendered)
	if err != nil {
		t.Fatalf("re-parse error: %v", err)
	}

	if cfg.From != cfg2.From {
		t.Errorf("From mismatch: %q vs %q", cfg.From, cfg2.From)
	}
	if cfg.System != cfg2.System {
		t.Errorf("System mismatch: %q vs %q", cfg.System, cfg2.System)
	}
	if len(cfg.Messages) != len(cfg2.Messages) {
		t.Errorf("Messages len mismatch: %d vs %d", len(cfg.Messages), len(cfg2.Messages))
	}
}

func TestToProviderConfig(t *testing.T) {
	parser := NewModelfileParser()
	cfg := &ModelConfig{
		From: "claude-sonnet-4-6",
		Parameters: map[string]interface{}{
			"temperature": 0.7,
			"max_tokens":  4096,
		},
		System:   "You are helpful.",
		Template: "{{.System}}\n{{.Prompt}}",
		Messages: []ModelMessage{
			{Role: "user", Content: "Hi"},
			{Role: "assistant", Content: "Hello"},
		},
		Adapters: []string{"/adapter1"},
	}

	pc := parser.ToProviderConfig(cfg)

	if pc["model"] != "claude-sonnet-4-6" {
		t.Errorf("model = %v, want claude-sonnet-4-6", pc["model"])
	}
	if pc["system_prompt"] != "You are helpful." {
		t.Errorf("system_prompt = %v", pc["system_prompt"])
	}
	if pc["template"] != "{{.System}}\n{{.Prompt}}" {
		t.Errorf("template = %v", pc["template"])
	}

	params, ok := pc["parameters"].(map[string]interface{})
	if !ok {
		t.Fatal("parameters not a map")
	}
	if params["temperature"] != 0.7 {
		t.Errorf("params temperature = %v", params["temperature"])
	}

	msgs, ok := pc["messages"].([]map[string]string)
	if !ok {
		t.Fatal("messages not a slice of maps")
	}
	if len(msgs) != 2 {
		t.Fatalf("messages len = %d, want 2", len(msgs))
	}

	adapters, ok := pc["adapters"].([]string)
	if !ok {
		t.Fatal("adapters not a string slice")
	}
	if len(adapters) != 1 || adapters[0] != "/adapter1" {
		t.Errorf("adapters = %v", adapters)
	}
}

func TestMergeConfigs(t *testing.T) {
	parser := NewModelfileParser()
	base := &ModelConfig{
		From: "base-model",
		Parameters: map[string]interface{}{
			"temperature": 0.5,
			"max_tokens":  2048,
		},
		System:   "Base system prompt.",
		Messages: []ModelMessage{{Role: "user", Content: "base msg"}},
		Adapters: []string{"/base/adapter"},
	}

	override := &ModelConfig{
		From: "override-model",
		Parameters: map[string]interface{}{
			"temperature": 0.9,
		},
		System:   "Override system.",
		Messages: []ModelMessage{{Role: "user", Content: "override msg"}},
	}

	merged := parser.MergeConfigs(base, override)

	if merged.From != "override-model" {
		t.Errorf("From = %q, want %q", merged.From, "override-model")
	}
	if merged.System != "Override system." {
		t.Errorf("System = %q, want %q", merged.System, "Override system.")
	}
	if merged.Parameters["temperature"] != 0.9 {
		t.Errorf("temperature = %v, want 0.9", merged.Parameters["temperature"])
	}
	if merged.Parameters["max_tokens"] != 2048 {
		t.Errorf("max_tokens = %v, want 2048 (from base)", merged.Parameters["max_tokens"])
	}
	if len(merged.Messages) != 1 || merged.Messages[0].Content != "override msg" {
		t.Errorf("Messages = %v, want override messages", merged.Messages)
	}
}

func TestMergeConfigsBaseRetained(t *testing.T) {
	parser := NewModelfileParser()
	base := &ModelConfig{
		From: "base-model",
		Parameters: map[string]interface{}{
			"temperature": 0.5,
			"top_p":       0.8,
		},
		System: "Base system.",
	}

	override := &ModelConfig{
		Parameters: map[string]interface{}{
			"temperature": 0.3,
		},
	}

	merged := parser.MergeConfigs(base, override)

	if merged.From != "base-model" {
		t.Errorf("From should come from base when override is empty")
	}
	if merged.System != "Base system." {
		t.Errorf("System should come from base when override is empty")
	}
	if merged.Parameters["top_p"] != 0.8 {
		t.Errorf("top_p should be retained from base")
	}
	if merged.Parameters["temperature"] != 0.3 {
		t.Errorf("temperature should be overridden")
	}
}

func TestDefaultModelConfigs(t *testing.T) {
	defaults := DefaultModelConfigs()

	names := []string{"coding", "creative", "precise"}
	for _, name := range names {
		cfg, ok := defaults[name]
		if !ok {
			t.Errorf("missing default config %q", name)
			continue
		}
		if cfg.From == "" {
			t.Errorf("%s: From is empty", name)
		}
		if cfg.System == "" {
			t.Errorf("%s: System is empty", name)
		}
	}

	coding := defaults["coding"]
	if coding.Parameters["temperature"] != 0.2 {
		t.Errorf("coding temperature = %v, want 0.2", coding.Parameters["temperature"])
	}

	creative := defaults["creative"]
	if creative.Parameters["temperature"] != 0.9 {
		t.Errorf("creative temperature = %v, want 0.9", creative.Parameters["temperature"])
	}

	precise := defaults["precise"]
	if precise.Parameters["temperature"] != 0.0 {
		t.Errorf("precise temperature = %v, want 0.0", precise.Parameters["temperature"])
	}
	if precise.Parameters["top_p"] != 0.1 {
		t.Errorf("precise top_p = %v, want 0.1", precise.Parameters["top_p"])
	}
}

func TestFormatConfig(t *testing.T) {
	cfg := &ModelConfig{
		From: "claude-sonnet-4-6",
		Parameters: map[string]interface{}{
			"temperature": 0.2,
			"max_tokens":  4096,
		},
		System: "You are a coding assistant that writes clean code.",
		Messages: []ModelMessage{
			{Role: "user", Content: "example"},
			{Role: "assistant", Content: "response"},
		},
	}

	output := FormatConfig(cfg)

	if !strings.Contains(output, "Model Configuration:") {
		t.Error("missing header")
	}
	if !strings.Contains(output, "Base: claude-sonnet-4-6") {
		t.Error("missing base model")
	}
	if !strings.Contains(output, "Parameters:") {
		t.Error("missing parameters")
	}
	if !strings.Contains(output, "System:") {
		t.Error("missing system")
	}
	if !strings.Contains(output, "2 examples") {
		t.Error("missing message count")
	}
}

func TestFormatConfigTruncatesLongSystem(t *testing.T) {
	cfg := &ModelConfig{
		From:       "model",
		Parameters: make(map[string]interface{}),
		System:     strings.Repeat("a", 100),
	}

	output := FormatConfig(cfg)
	if !strings.Contains(output, "...") {
		t.Error("long system prompt should be truncated with ...")
	}
}

func TestParseCaseInsensitiveDirectives(t *testing.T) {
	content := `from model-name
parameter temperature 0.5
system "hello"
`
	parser := NewModelfileParser()
	cfg, err := parser.Parse(content)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.From != "model-name" {
		t.Errorf("From = %q, want %q", cfg.From, "model-name")
	}
	if cfg.Parameters["temperature"] != 0.5 {
		t.Errorf("temperature = %v, want 0.5", cfg.Parameters["temperature"])
	}
}

func TestParseCommentsAndBlankLines(t *testing.T) {
	content := `
# Header comment
FROM model

# Params section
PARAMETER temperature 0.5

# End
`
	parser := NewModelfileParser()
	cfg, err := parser.Parse(content)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.From != "model" {
		t.Errorf("From = %q, want %q", cfg.From, "model")
	}
}

func TestParseMultilineTemplate(t *testing.T) {
	content := `FROM model
TEMPLATE """{{.System}}
---
{{.Prompt}}"""
`
	parser := NewModelfileParser()
	cfg, err := parser.Parse(content)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	expected := "{{.System}}\n---\n{{.Prompt}}"
	if cfg.System == expected {
		// System should not be set, template should
	}
	if cfg.Template != expected {
		t.Errorf("Template = %q, want %q", cfg.Template, expected)
	}
}

func TestRenderMultilineSystem(t *testing.T) {
	parser := NewModelfileParser()
	cfg := &ModelConfig{
		From:       "model",
		Parameters: make(map[string]interface{}),
		System:     "line one\nline two\nline three",
	}

	rendered := parser.Render(cfg)
	if !strings.Contains(rendered, `"""`) {
		t.Error("multiline system should use triple quotes")
	}

	// Verify round-trip.
	cfg2, err := parser.Parse(rendered)
	if err != nil {
		t.Fatalf("re-parse error: %v", err)
	}
	if cfg2.System != cfg.System {
		t.Errorf("System mismatch after round-trip: %q vs %q", cfg2.System, cfg.System)
	}
}

func TestToProviderConfigMinimal(t *testing.T) {
	parser := NewModelfileParser()
	cfg := &ModelConfig{
		From:       "model",
		Parameters: make(map[string]interface{}),
	}

	pc := parser.ToProviderConfig(cfg)
	if pc["model"] != "model" {
		t.Errorf("model = %v", pc["model"])
	}
	if _, ok := pc["system_prompt"]; ok {
		t.Error("should not have system_prompt when empty")
	}
	if _, ok := pc["messages"]; ok {
		t.Error("should not have messages when empty")
	}
	if _, ok := pc["parameters"]; ok {
		t.Error("should not have parameters when empty")
	}
}

func TestParseMultipleAdapters(t *testing.T) {
	content := `FROM model
ADAPTER /path/one
ADAPTER /path/two
ADAPTER /path/three
`
	parser := NewModelfileParser()
	cfg, err := parser.Parse(content)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(cfg.Adapters) != 3 {
		t.Errorf("len(Adapters) = %d, want 3", len(cfg.Adapters))
	}
}
