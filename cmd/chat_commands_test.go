package cmd

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	hawkconfig "github.com/GrayCodeAI/hawk/internal/config"
	"github.com/GrayCodeAI/hawk/internal/engine"
	"github.com/GrayCodeAI/hawk/internal/tool"
)

func TestAdditionalDirContextLoadsInstructions(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "AGENTS.md"), []byte("extra instructions"), 0o644); err != nil {
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

func TestHandleCommandEmptyInputIsNoOp(t *testing.T) {
	m := &chatModel{}
	model, cmd := m.handleCommand("  \n\t")
	if cmd != nil {
		t.Fatal("empty command should not schedule a tea command")
	}
	if model != m || len(m.messages) != 0 {
		t.Fatal("empty command should leave the model unchanged")
	}
}

func TestLocalSlashCommands(t *testing.T) {
	preserveCLICompilerVersionState(t)
	version = "test-version"
	sess := engine.NewSession("openai", "gpt-4o", "base", tool.NewRegistry(tool.LSTool{}))
	m := &chatModel{
		session:   sess,
		registry:  tool.NewRegistry(tool.LSTool{}),
		settings:  hawkconfig.Settings{MCPServers: []hawkconfig.MCPServerConfig{{Name: "demo", Command: "demo-mcp"}}},
		sessionID: "test",
		width:     80,
		height:    24,
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
	// Verify at least some messages were appended (commands didn't panic)
	if len(m.messages) < 5 {
		t.Fatalf("expected at least 5 messages from 5 commands, got %d", len(m.messages))
	}
}

func TestDiagnosticSummaries(t *testing.T) {
	preserveCLICompilerVersionState(t)
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

func TestQuestionMarkAndHelpAliases(t *testing.T) {
	sess := engine.NewSession("openai", "gpt-4o", "base", tool.NewRegistry())
	m := &chatModel{session: sess, registry: tool.NewRegistry(), sessionID: "test"}
	for _, input := range []string{"?", "? help", "?help", "help", "? commit"} {
		m.messages = nil
		model, _ := m.handleCommand(input)
		cm := model.(*chatModel)
		if len(cm.messages) == 0 {
			t.Fatalf("expected message output for alias %q, got 0", input)
		}
	}
}
