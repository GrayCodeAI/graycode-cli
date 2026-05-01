package cmd

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	hawkconfig "github.com/GrayCodeAI/hawk/config"
	"github.com/GrayCodeAI/hawk/engine"
	"github.com/GrayCodeAI/hawk/tool"
)

func TestAdditionalDirContextLoadsInstructions(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "HAWK.md"), []byte("extra instructions"), 0o644); err != nil {
		t.Fatal(err)
	}

	abs, block, err := additionalDirContext(dir)
	if err != nil {
		t.Fatal(err)
	}
	if abs == "" || !filepath.IsAbs(abs) {
		t.Fatalf("expected absolute directory, got %q", abs)
	}
	if !strings.Contains(block, "Additional directory: "+abs) || !strings.Contains(block, "extra instructions") {
		t.Fatalf("unexpected context block: %s", block)
	}
}

func TestHandleCommandAddDir(t *testing.T) {
	oldAddDirs := addDirs
	addDirs = nil
	t.Cleanup(func() { addDirs = oldAddDirs })

	dir := t.TempDir()
	sess := engine.NewSession("openai", "gpt-4o", "base", tool.NewRegistry())
	m := &chatModel{session: sess, registry: tool.NewRegistry(), sessionID: "test"}

	model, cmd := m.handleCommand("/add-dir " + dir)
	if cmd != nil {
		t.Fatal("expected no tea command")
	}
	got := model.(*chatModel)
	if len(addDirs) != 1 || addDirs[0] == "" {
		t.Fatalf("expected addDirs to include directory, got %v", addDirs)
	}
	if len(got.messages) == 0 || !strings.Contains(got.messages[len(got.messages)-1].content, "Added directory to context") {
		t.Fatalf("expected add-dir status message, got %#v", got.messages)
	}
}

func TestLocalSlashCommands(t *testing.T) {
	version = "test-version"
	sess := engine.NewSession("openai", "gpt-4o", "base", tool.NewRegistry(tool.LSTool{}))
	m := &chatModel{
		session:   sess,
		registry:  tool.NewRegistry(tool.LSTool{}),
		settings:  hawkconfig.Settings{MCPServers: []hawkconfig.MCPServerConfig{{Name: "demo", Command: "demo-mcp"}}},
		sessionID: "test",
	}

	for _, input := range []string{"/version", "/env", "/mcp", "/tools", "/welcome"} {
		model, cmd := m.handleCommand(input)
		if cmd != nil {
			t.Fatalf("%s returned unexpected tea command", input)
		}
		m = model.(*chatModel)
		if len(m.messages) == 0 {
			t.Fatalf("%s did not append a message", input)
		}
	}
	if !strings.Contains(m.messages[len(m.messages)-1].content, "Skills") {
		t.Fatalf("expected expanded welcome message, got %s", m.messages[len(m.messages)-1].content)
	}
}

func TestDiagnosticSummaries(t *testing.T) {
	version = "test-version"
	settings := hawkconfig.Settings{
		Provider: "openai",
		Model:    "gpt-4o",
		MCPServers: []hawkconfig.MCPServerConfig{
			{Name: "demo", Command: "demo-mcp", Args: []string{"--stdio"}},
		},
	}

	report := doctorReport(settings)
	if !strings.Contains(report, "Hawk doctor") || !strings.Contains(report, "Built-in tools") {
		t.Fatalf("unexpected doctor report: %s", report)
	}
	if summary := mcpConfigSummary(settings); !strings.Contains(summary, "demo") {
		t.Fatalf("unexpected mcp summary: %s", summary)
	}
	if tools := builtInToolsSummary(); !strings.Contains(tools, "Bash") || !strings.Contains(tools, "LS") {
		t.Fatalf("unexpected tools summary: %s", tools)
	}
}
