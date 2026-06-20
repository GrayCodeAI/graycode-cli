package config

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/GrayCodeAI/eyrie/credentials"
	"github.com/GrayCodeAI/hawk/internal/testutil"
)

func TestLoadAgentsMD(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "AGENTS.md"), []byte("test instructions"), 0o644)

	// Change to temp dir
	orig, _ := os.Getwd()
	os.Chdir(dir)
	defer os.Chdir(orig)

	md := LoadAgentsMD()
	if md != "test instructions" {
		t.Fatalf("got %q", md)
	}
}

func TestLoadAgentsMDMissing(t *testing.T) {
	dir := t.TempDir()
	orig, _ := os.Getwd()
	os.Chdir(dir)
	defer os.Chdir(orig)

	md := LoadAgentsMD()
	if md != "" {
		t.Fatalf("expected empty, got %q", md)
	}
}

func TestBuildContext(t *testing.T) {
	ctx := BuildContext()
	if !strings.Contains(ctx, "Working directory:") {
		t.Fatal("expected Working directory in context")
	}
}

func TestBuildContextWithDirs(t *testing.T) {
	root := t.TempDir()
	extra := t.TempDir()
	os.WriteFile(filepath.Join(extra, "AGENTS.md"), []byte("extra instructions"), 0o644)

	orig, _ := os.Getwd()
	os.Chdir(root)
	defer os.Chdir(orig)

	ctx := BuildContextWithDirs([]string{extra})
	if !strings.Contains(ctx, "Additional directory:") {
		t.Fatal("expected additional directory in context")
	}
	if !strings.Contains(ctx, "extra instructions") {
		t.Fatal("expected additional directory AGENTS.md instructions")
	}
}

func TestLoadSettingsWithJSONOverride(t *testing.T) {
	testutil.IsolateStorage(t)
	settings, err := LoadSettingsWithOverride(`{"model":"test-model","allowedTools":["Read"],"disallowedTools":["Write"]}`)
	if err != nil {
		t.Fatal(err)
	}
	if settings.Model != "test-model" {
		t.Fatalf("got model %q", settings.Model)
	}
	if len(settings.AllowedTools) != 1 || settings.AllowedTools[0] != "Read" {
		t.Fatalf("unexpected allowedTools: %v", settings.AllowedTools)
	}
	if len(settings.DisallowedTools) != 1 || settings.DisallowedTools[0] != "Write" {
		t.Fatalf("unexpected disallowedTools: %v", settings.DisallowedTools)
	}
}

func TestLoadSettingsAcceptsArchiveCamelCase(t *testing.T) {
	testutil.IsolateStorage(t)
	settings, err := LoadSettingsWithOverride(`{
		"autoAllow":["Read"],
		"maxBudgetUSD":1.25,
		"customHeaders":{"x-test":"yes"},
		"mcpServers":[{"name":"demo","command":"demo-mcp","args":["--stdio"]}],
		"allowed_tools":["Bash"],
		"disallowed_tools":["Write"]
	}`)
	if err != nil {
		t.Fatal(err)
	}
	if settings.MaxBudgetUSD != 1.25 {
		t.Fatalf("unexpected settings: %#v", settings)
	}
	if len(settings.AutoAllow) != 1 || settings.AutoAllow[0] != "Read" {
		t.Fatalf("unexpected autoAllow: %v", settings.AutoAllow)
	}
	if settings.CustomHeaders["x-test"] != "yes" {
		t.Fatalf("unexpected customHeaders: %v", settings.CustomHeaders)
	}
	if len(settings.MCPServers) != 1 || settings.MCPServers[0].Name != "demo" {
		t.Fatalf("unexpected mcpServers: %v", settings.MCPServers)
	}
	if len(settings.AllowedTools) != 1 || settings.AllowedTools[0] != "Bash" {
		t.Fatalf("unexpected allowedTools: %v", settings.AllowedTools)
	}
	if len(settings.DisallowedTools) != 1 || settings.DisallowedTools[0] != "Write" {
		t.Fatalf("unexpected disallowedTools: %v", settings.DisallowedTools)
	}
}

func TestLoadSettingsUsesUserConfigOnly(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	configDir := filepath.Join(home, "config")
	t.Setenv("HAWK_CONFIG_DIR", configDir)
	if err := os.MkdirAll(configDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(configDir, "settings.json"), []byte(`{"model":"global","allowedTools":["Read"]}`), 0o644); err != nil {
		t.Fatal(err)
	}

	settings := LoadSettings()
	if got := ActiveModel(context.TODO()); got != "global" {
		t.Fatalf("expected global model in eyrie, got %q (settings.model=%q)", got, settings.Model)
	}
	if settings.Model != "" {
		t.Fatalf("model must not remain in hawk settings.json, got %q", settings.Model)
	}
	if len(settings.AllowedTools) != 1 || settings.AllowedTools[0] != "Read" {
		t.Fatalf("expected global allowedTools, got %v", settings.AllowedTools)
	}
	if len(settings.DisallowedTools) != 0 {
		t.Fatalf("unexpected disallowedTools, got %v", settings.DisallowedTools)
	}
	if len(settings.MCPServers) != 0 {
		t.Fatalf("unexpected mcpServers, got %v", settings.MCPServers)
	}
}

func TestSetGlobalSettingAndSettingValue(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	if err := SetGlobalSetting("model", "test-model"); err != nil {
		t.Fatal(err)
	}
	if err := SetGlobalSetting("allowedTools", "Read, Write"); err != nil {
		t.Fatal(err)
	}
	if err := SetGlobalSetting("maxBudgetUSD", "2.5"); err != nil {
		t.Fatal(err)
	}
	// Hawk: API keys rejected from settings file
	if err := SetGlobalSetting("apiKey.openai", "sk-test"); err == nil {
		t.Fatal("expected error setting api key in settings")
	}

	settings := LoadGlobalSettings()
	if got := ActiveModel(context.TODO()); got != "test-model" {
		t.Fatalf("unexpected active model: %q (settings.model=%q)", got, settings.Model)
	}
	if settings.Model != "" {
		t.Fatalf("model must not be stored in settings.json, got %q", settings.Model)
	}
	if got, ok := SettingValue(settings, "model"); !ok || got != "test-model" {
		t.Fatalf("unexpected model setting value: %q ok=%v", got, ok)
	}
	if got, ok := SettingValue(settings, "allowed_tools"); !ok || got != "Read, Write" {
		t.Fatalf("unexpected allowedTools value: %q ok=%v", got, ok)
	}
	if got, ok := SettingValue(settings, "max_budget_usd"); !ok || got != "2.5" {
		t.Fatalf("unexpected max budget value: %q ok=%v", got, ok)
	}
	// API key status from OS secret store
	store := &credentials.MapStore{}
	credentials.SetDefaultStore(store)
	t.Cleanup(func() { credentials.SetDefaultStore(nil) })
	_ = store.Set(context.Background(), credentials.AccountForEnv("OPENAI_API_KEY"), "sk-test")
	if got, ok := SettingValue(settings, "apiKey.openai"); !ok || got != "set" {
		t.Fatalf("unexpected provider API key status: %q ok=%v", got, ok)
	}
}

func TestLoadAgentsMD_VisibleFile(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "AGENTS.md"), []byte("project instructions"), 0o644)

	md := LoadAgentsMDFrom(dir)
	if md != "project instructions" {
		t.Fatalf("expected AGENTS.md content, got %q", md)
	}
}

func TestLoadAgentDir_Agent(t *testing.T) {
	dir := t.TempDir()
	os.MkdirAll(filepath.Join(dir, ".agent"), 0o755)

	orig, _ := os.Getwd()
	os.Chdir(dir)
	defer os.Chdir(orig)

	got := LoadAgentDir()
	if !strings.HasSuffix(got, ".agent") || got == "" {
		t.Fatalf("expected path ending in .agent, got %q", got)
	}
}

func TestLoadAgentDir_Neither(t *testing.T) {
	dir := t.TempDir()

	orig, _ := os.Getwd()
	os.Chdir(dir)
	defer os.Chdir(orig)

	got := LoadAgentDir()
	if got != "" {
		t.Fatalf("expected empty, got %q", got)
	}
}
