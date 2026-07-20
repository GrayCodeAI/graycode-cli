package cmd

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"charm.land/bubbles/v2/textarea"
	"charm.land/bubbles/v2/viewport"
	lipgloss "charm.land/lipgloss/v2"
	"github.com/GrayCodeAI/hawk/internal/provider/gateway"
	"github.com/GrayCodeAI/hawk/internal/bridge/sessioncapture"
	hawkconfig "github.com/GrayCodeAI/hawk/internal/config"
	"github.com/GrayCodeAI/hawk/internal/engine"
	"github.com/GrayCodeAI/hawk/internal/feature/shellmode"
	"github.com/GrayCodeAI/hawk/internal/session"
	"github.com/GrayCodeAI/hawk/internal/storage"
	"github.com/GrayCodeAI/hawk/internal/tool"
)

func newTestChatModel() *chatModel {
	sess := engine.NewSession("", "test-model", "you are helpful", nil)
	sess.PermSvc().SetMaxTurns(1)
	sess.SetTestClient(engine.NewMockClientForTest())

	m := &chatModel{
		input:             textarea.New(),
		viewport:          viewport.New(viewport.WithWidth(120), viewport.WithHeight(12)),
		session:           sess,
		registry:          tool.NewRegistry(),
		partial:           &strings.Builder{},
		sessionID:         "test-session",
		width:             120,
		height:            40,
		ref:               &progRef{},
		modeManager:       shellmode.NewModeManager(),
		termCtx:           sessioncapture.NewTerminalContext(),
		ghostText:         NewGhostText(),
		inputIndicator:    &InputIndicator{},
		hintsLoader:       engine.NewHintsLoader(),
		selfImprover:      engine.NewSelfImprover(),
		codingSoul:        engine.LoadCodingSoul(),
		brailleSpinner:    NewBrailleSpinner(SpinnerHawk, "Thinking"),
		testStreamStarter: func() {},
	}
	return m
}

func TestNewTestChatModel_DisablesAsyncStreamLauncher(t *testing.T) {
	m := newTestChatModel()
	m.startStream()
	if m.cancel != nil {
		t.Fatal("test model should not start a background stream")
	}
}

func isolateChatCommandSweepEnv(t *testing.T) {
	t.Helper()
	root := t.TempDir()
	storage.SetTestDirs(t, root)
	isolateCredentialHome(t)
	hawkconfig.InvalidateConfigUICache()
	gateway.SetDefaultStore(&gateway.MapStore{})
	restoreThemeGlobals(t)
	t.Cleanup(func() {
		gateway.SetDefaultStore(nil)
		hawkconfig.InvalidateConfigUICache()
	})
}

// restoreThemeGlobals snapshots every package-level color var that
// ApplyTheme mutates and restores it when the test finishes, so commands
// like "/theme dark" cannot leak themed globals into unrelated tests
// (e.g. TestAdaptiveNeutralsPreserveDarkAppearance).
func restoreThemeGlobals(t *testing.T) {
	t.Helper()
	savedHawk, savedSuccess, savedWarn, savedErr, savedInfo := hawkColor, successTeal, warnAmber, errorCoral, infoSky
	savedTool, savedAgent, savedDone, savedContainer := toolGold, agentGold, doneGreen, containerBlue
	savedInspect, savedEdit, savedRun, savedTrust := tierInspect, tierEdit, tierRun, tierTrust
	savedHudBorder, savedHudLabel := hudBorderPurple, hudLabelPink
	savedCost, savedBranch, savedToken, savedCwd := costViolet, branchYellow, tokenSage, cwdBlue
	savedPrimary, savedMuted, savedPlaceholder, savedDisabled := textPrimary, textMuted, textPlaceholder, textDisabled
	savedBorderDim, savedBgCode := borderDim, bgCode
	hasDark := lipgloss.HasDarkBackground(os.Stdin, os.Stdout)
	t.Cleanup(func() {
		hawkColor, successTeal, warnAmber, errorCoral, infoSky = savedHawk, savedSuccess, savedWarn, savedErr, savedInfo
		toolGold, agentGold, doneGreen, containerBlue = savedTool, savedAgent, savedDone, savedContainer
		tierInspect, tierEdit, tierRun, tierTrust = savedInspect, savedEdit, savedRun, savedTrust
		hudBorderPurple, hudLabelPink = savedHudBorder, savedHudLabel
		costViolet, branchYellow, tokenSage, cwdBlue = savedCost, savedBranch, savedToken, savedCwd
		textPrimary, textMuted, textPlaceholder, textDisabled = savedPrimary, savedMuted, savedPlaceholder, savedDisabled
		borderDim, bgCode = savedBorderDim, savedBgCode
		// HasDarkBackground is now a function (no args), SetHasDarkBackground removed in v2
		_ = hasDark
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
		"/sandbox", "/autonomy",
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

func TestChatModel_SaveSessionPersistsPersistenceMessages(t *testing.T) {
	isolateChatCommandSweepEnv(t)
	m := newTestChatModel()
	m.session.AddUser("hello")
	m.session.AddAssistant("hi")

	m.saveSession()

	saved, err := session.Load(m.sessionID)
	if err != nil {
		t.Fatalf("Load(%q) after saveSession() error = %v", m.sessionID, err)
	}
	if len(saved.Messages) != 2 {
		t.Fatalf("saved messages = %d, want 2", len(saved.Messages))
	}
	if saved.Messages[0].Role != "user" || saved.Messages[0].Content != "hello" {
		t.Fatalf("saved.Messages[0] = %#v, want user hello", saved.Messages[0])
	}
	if saved.Messages[1].Role != "assistant" || saved.Messages[1].Content != "hi" {
		t.Fatalf("saved.Messages[1] = %#v, want assistant hi", saved.Messages[1])
	}
}

func TestChatModel_SlashExportRedactsAndPrivatizesFile(t *testing.T) {
	isolateChatCommandSweepEnv(t)
	m := newTestChatModel()
	secret := "sk-1234567890abcdefghijklmnop"
	m.session.AddUser("my key is " + secret)

	result, _ := m.handleCommand("/export")
	cm := requireChatModel(t, result)
	last := cm.messages[len(cm.messages)-1]
	if !strings.Contains(last.content, "Exported to:") {
		t.Fatalf("export message = %q", last.content)
	}
	exportPath := filepath.Join(storage.StateDir(), "exports", cm.sessionID+".md")
	data, err := os.ReadFile(exportPath)
	if err != nil {
		t.Fatalf("read export: %v", err)
	}
	if strings.Contains(string(data), secret) {
		t.Fatalf("export contains unredacted secret: %s", data)
	}
	if !strings.Contains(string(data), "[REDACTED]") {
		t.Fatalf("export missing redaction marker: %s", data)
	}
	info, err := os.Stat(exportPath)
	if err != nil {
		t.Fatalf("stat export: %v", err)
	}
	if got := info.Mode().Perm(); got != 0o600 {
		t.Fatalf("export file mode = %v, want 0600", got)
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
