package onboarding

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestNeedsSetup_NoEnvKeys(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("HOME", dir)
	t.Setenv("ANTHROPIC_API_KEY", "")
	t.Setenv("OPENAI_API_KEY", "")
	t.Setenv("GEMINI_API_KEY", "")
	t.Setenv("OPENROUTER_API_KEY", "")
	t.Setenv("XAI_API_KEY", "")
	t.Setenv("GROQ_API_KEY", "")

	os.Unsetenv("ANTHROPIC_API_KEY")
	os.Unsetenv("OPENAI_API_KEY")
	os.Unsetenv("GEMINI_API_KEY")
	os.Unsetenv("OPENROUTER_API_KEY")
	os.Unsetenv("XAI_API_KEY")
	os.Unsetenv("GROQ_API_KEY")

	if !NeedsSetup() {
		t.Error("NeedsSetup() should be true when no keys are set")
	}
}

func TestNeedsSetup_WithAnthropicKey(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("HOME", dir)
	t.Setenv("ANTHROPIC_API_KEY", "sk-ant-test123456789")

	if NeedsSetup() {
		t.Error("NeedsSetup() should be false when ANTHROPIC_API_KEY is set")
	}
}

func TestNeedsSetup_WithOpenAIKey(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("HOME", dir)
	t.Setenv("OPENAI_API_KEY", "sk-test123456789")

	os.Unsetenv("ANTHROPIC_API_KEY")

	if NeedsSetup() {
		t.Error("NeedsSetup() should be false when OPENAI_API_KEY is set")
	}
}

func TestValidateAPIKey(t *testing.T) {
	tests := []struct {
		name     string
		provider string
		key      string
		valid    bool
	}{
		{"valid anthropic", "anthropic", "sk-ant-api01-abcdefghijklmnopqrstuvwxyz", true},
		{"valid openai", "openai", "sk-abcdefghijklmnopqrstuvwxyz123456", true},
		{"too short", "anthropic", "sk-ant", false},
		{"wrong prefix anthropic", "anthropic", "wrong-prefix-long-enough-key", false},
		{"wrong prefix openai", "openai", "not-sk-prefix-long-enough-key", false},
		{"unknown provider accepts any", "gemini", "any-key-long-enough-to-be-valid", true},
		{"empty key", "anthropic", "", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, valid := validateAPIKey(tt.provider, tt.key)
			if valid != tt.valid {
				t.Errorf("validateAPIKey(%q, %q) valid = %v, want %v", tt.provider, tt.key, valid, tt.valid)
			}
		})
	}
}

func TestSaveAPIKeyToEnvFile(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("HOME", dir)

	hawkDir := filepath.Join(dir, ".hawk")
	if err := os.MkdirAll(hawkDir, 0o755); err != nil {
		t.Fatal(err)
	}

	SaveAPIKeyToEnvFile("ANTHROPIC_API_KEY", "sk-ant-test123")

	path := filepath.Join(hawkDir, "env")
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("env file not created: %v", err)
	}

	content := string(data)
	if !strings.Contains(content, "ANTHROPIC_API_KEY") {
		t.Error("env file should contain key name")
	}
	if !strings.Contains(content, "sk-ant-test123") {
		t.Error("env file should contain key value")
	}

	info, _ := os.Stat(path)
	if info.Mode().Perm() != 0o600 {
		t.Errorf("env file permissions = %o, want 0600", info.Mode().Perm())
	}
}

func TestSaveAPIKeyToEnvFile_Append(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("HOME", dir)

	hawkDir := filepath.Join(dir, ".hawk")
	if err := os.MkdirAll(hawkDir, 0o755); err != nil {
		t.Fatal(err)
	}

	SaveAPIKeyToEnvFile("KEY1", "value1")
	SaveAPIKeyToEnvFile("KEY2", "value2")

	data, err := os.ReadFile(filepath.Join(hawkDir, "env"))
	if err != nil {
		t.Fatal(err)
	}

	content := string(data)
	if !strings.Contains(content, "KEY1") || !strings.Contains(content, "KEY2") {
		t.Error("env file should contain both keys after append")
	}
}

func TestWelcome(t *testing.T) {
	Welcome("1.0.0")
}

func TestColorConstants(t *testing.T) {
	if teal == "" {
		t.Error("teal color should not be empty")
	}
	if reset == "" {
		t.Error("reset should not be empty")
	}
	if bold == "" {
		t.Error("bold should not be empty")
	}
}
