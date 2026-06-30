package cmd

import (
	"strings"
	"testing"
	"time"

	"github.com/GrayCodeAI/eyrie/credentials"
	"github.com/GrayCodeAI/hawk/internal/bridge/sessioncapture"
	hawkconfig "github.com/GrayCodeAI/hawk/internal/config"
	"github.com/GrayCodeAI/hawk/internal/engine"
	"github.com/GrayCodeAI/hawk/internal/feature/shellmode"
	"github.com/GrayCodeAI/hawk/internal/storage"
	"github.com/GrayCodeAI/hawk/internal/tool"
	"github.com/charmbracelet/bubbles/textarea"
	"github.com/charmbracelet/bubbles/viewport"
)

func newTestChatModel() *chatModel {
	sess := engine.NewSession("", "test-model", "you are helpful", nil)
	sess.PermSvc().SetMaxTurns(1)
	sess.SetTestClient(engine.NewMockClientForTest())

	m := &chatModel{
		input:          textarea.New(),
		viewport:       viewport.New(120, 12),
		session:        sess,
		registry:       tool.NewRegistry(),
		partial:        &strings.Builder{},
		sessionID:      "test-session",
		width:          120,
		height:         40,
		ref:            &progRef{},
		modeManager:    shellmode.NewModeManager(),
		termCtx:        sessioncapture.NewTerminalContext(),
		ghostText:      NewGhostText(),
		inputIndicator: &InputIndicator{},
		hintsLoader:    engine.NewHintsLoader(),
		selfImprover:   engine.NewSelfImprover(),
		codingSoul:     engine.LoadCodingSoul(),
		brailleSpinner: NewBrailleSpinner(SpinnerHawk, "Thinking"),
	}
	return m
}

func isolateChatCommandSweepEnv(t *testing.T) {
	t.Helper()
	root := t.TempDir()
	storage.SetTestDirs(t, root)
	isolateCredentialHome(t)
	hawkconfig.InvalidateConfigUICache()
	credentials.SetDefaultStore(&credentials.MapStore{})
	t.Cleanup(func() {
		credentials.SetDefaultStore(nil)
		hawkconfig.InvalidateConfigUICache()
	})
}

func TestChatModel_SlashHelp(t *testing.T) {
	m := newTestChatModel()
	result, _ := m.handleCommand("/help")
	if result == nil {
		t.Fatal("handleCommand(/help) returned nil model")
	}
	cm := result.(*chatModel)
	if len(cm.messages) == 0 {
		t.Error("/help should add a system message")
	}
}

func TestChatModel_SlashVersion(t *testing.T) {
	SetVersion("1.0.0-test")
	m := newTestChatModel()
	result, _ := m.handleCommand("/version")
	cm := result.(*chatModel)
	found := false
	for _, msg := range cm.messages {
		if strings.Contains(msg.content, "1.0.0") {
			found = true
			break
		}
	}
	if !found {
		t.Error("/version should display version string")
	}
}

func TestChatModel_SlashClear(t *testing.T) {
	m := newTestChatModel()
	m.messages = append(m.messages, displayMsg{role: "user", content: "hello"})
	m.messages = append(m.messages, displayMsg{role: "assistant", content: "hi"})

	result, _ := m.handleCommand("/clear")
	cm := result.(*chatModel)
	if len(cm.messages) > 1 {
		t.Errorf("/clear should clear messages, got %d", len(cm.messages))
	}
}

func TestFormatQuitResumeMessage(t *testing.T) {
	got := formatQuitResumeMessage("44418bdd52745678")
	want := "Thank you for using Hawk!\n\nTo resume this session, run: hawk --resume 44418bdd52745678\n"
	if got != want {
		t.Fatalf("quit message mismatch:\nwant %q\ngot  %q", want, got)
	}
}

func TestFormatQuitResumeMessage_NoSession(t *testing.T) {
	got := formatQuitResumeMessage("")
	want := "Thank you for using Hawk!\n"
	if got != want {
		t.Fatalf("quit message mismatch:\nwant %q\ngot  %q", want, got)
	}
}

func TestChatModel_SlashModel(t *testing.T) {
	m := newTestChatModel()
	result, _ := m.handleCommand("/model")
	cm := result.(*chatModel)
	if len(cm.messages) == 0 && !cm.configOpen {
		t.Error("/model should either add a message or open config")
	}
}

func TestChatModel_SlashCost(t *testing.T) {
	m := newTestChatModel()
	result, _ := m.handleCommand("/cost")
	cm := result.(*chatModel)
	if len(cm.messages) == 0 {
		t.Error("/cost should add a message")
	}
}

func TestChatModel_SlashTokens(t *testing.T) {
	m := newTestChatModel()
	result, _ := m.handleCommand("/tokens")
	cm := result.(*chatModel)
	if len(cm.messages) == 0 {
		t.Error("/tokens should add a message")
	}
}

func TestChatModel_SlashTools(t *testing.T) {
	m := newTestChatModel()
	result, _ := m.handleCommand("/tools")
	cm := result.(*chatModel)
	if len(cm.messages) == 0 {
		t.Error("/tools should list tools")
	}
}

func TestChatModel_SlashStatus(t *testing.T) {
	m := newTestChatModel()
	result, _ := m.handleCommand("/status")
	cm := result.(*chatModel)
	if len(cm.messages) == 0 {
		t.Error("/status should show session info")
	}
}

func TestChatModel_SlashUnknown(t *testing.T) {
	m := newTestChatModel()
	result, _ := m.handleCommand("/nonexistent-command-xyz")
	cm := result.(*chatModel)
	found := false
	for _, msg := range cm.messages {
		if strings.Contains(msg.content, "unknown") || strings.Contains(msg.content, "Unknown") || msg.role == "error" {
			found = true
			break
		}
	}
	if !found {
		t.Error("/nonexistent should show unknown command message")
	}
}

func TestChatModel_ManyCommands(t *testing.T) {
	commands := []string{
		"/context", "/env", "/hooks", "/stats",
		"/compact", "/diff", "/branch", "/vim",
		"/power", "/fast", "/effort",
		"/memory", "/plugins", "/mcp",
		"/sandbox", "/permissions",
		"/usage", "/metrics", "/integrity",
		"/keybindings", "/cron", "/tasks",
		"/files", "/branches", "/provider-status",
		"/output-style plain",
		"/copy", "/export", "/fork",
		"/rewind", "/undo", "/taste",
		"/theme dark", "/btw hello",
		"/focus src/", "/pin",
		"/rename test-session",
		"/tag important", "/color green",
		"/clean", "/clear", "/cost",
		"/drop main.go", "/history",
		"/model", "/new", "/quit",
		"/session", "/share", "/skills",
		"/snapshot", "/stale", "/status",
		"/statusline", "/tokens", "/tools",
		"/upgrade", "/version", "/welcome",
		"/yolo", "/voice", "/agents",
		"/audit", "/dream",
		"/release-notes", "/reload-plugins",
		"/remote-env", "/render",
		"/add main.go", "/add-dir .",
		"/compress", "/loop",
		"/feedback", "/plugin",
		"/pr-comments", "/thinkback",
		"/think-back", "/thinkback-play",
	}

	for _, cmd := range commands {
		t.Run(cmd, func(t *testing.T) {
			isolateChatCommandSweepEnv(t)
			m := newTestChatModel()
			result, _ := m.handleCommand(cmd)
			if result == nil {
				t.Errorf("%s returned nil model", cmd)
			}
			cm := requireChatModel(t, result)
			if cm.cancel != nil {
				cm.cancel()
			}
			if cm.loopCancel != nil {
				cm.loopCancel()
			}
			time.Sleep(10 * time.Millisecond)
		})
	}
}

func TestChatModel_SlashNew(t *testing.T) {
	m := newTestChatModel()
	m.messages = append(m.messages, displayMsg{role: "user", content: "old"})
	result, _ := m.handleCommand("/new")
	if result == nil {
		t.Error("/new returned nil")
	}
}

func TestChatModel_SlashCopy(t *testing.T) {
	m := newTestChatModel()
	m.messages = append(m.messages, displayMsg{role: "assistant", content: "copy this"})
	result, _ := m.handleCommand("/copy")
	if result == nil {
		t.Error("/copy returned nil")
	}
}

func TestChatModel_SlashExport(t *testing.T) {
	m := newTestChatModel()
	m.messages = append(m.messages, displayMsg{role: "user", content: "hello"})
	result, _ := m.handleCommand("/export")
	if result == nil {
		t.Error("/export returned nil")
	}
}

func TestChatModel_StreamingCommands(t *testing.T) {
	// These trigger startStream; cancel promptly so the test doesn't leak workers.
	commands := []string{
		"/doctor", "/commit", "/review",
		"/summary", "/security-review",
		"/bughunter", "/check", "/hunt",
		"/design", "/ultrareview",
	}

	for _, cmd := range commands {
		t.Run(cmd, func(t *testing.T) {
			isolateChatCommandSweepEnv(t)
			m := newTestChatModel()
			m.session.AddUser("some context for the command")
			result, _ := m.handleCommand(cmd)
			if result == nil {
				t.Errorf("%s returned nil model", cmd)
			}
			cm := requireChatModel(t, result)
			if cm.cancel != nil {
				cm.cancel()
			}
			time.Sleep(10 * time.Millisecond)
		})
	}
}
