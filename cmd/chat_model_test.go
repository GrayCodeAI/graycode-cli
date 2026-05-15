package cmd

import (
	"strings"
	"testing"

	"github.com/GrayCodeAI/hawk/engine"
	"github.com/GrayCodeAI/hawk/tool"
)

func newTestChatModel() *chatModel {
	sess := engine.NewSession("", "test-model", "you are helpful", nil)
	sess.MaxTurns = 1
	sess.SetTestClient(engine.NewMockClientForTest())

	m := &chatModel{
		session:   sess,
		registry:  tool.NewRegistry(),
		partial:   &strings.Builder{},
		sessionID: "test-session",
		width:     120,
		height:    40,
		ref:       &progRef{},
	}
	return m
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
	}

	for _, cmd := range commands {
		t.Run(cmd, func(t *testing.T) {
			m := newTestChatModel()
			result, _ := m.handleCommand(cmd)
			if result == nil {
				t.Errorf("%s returned nil model", cmd)
			}
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

func TestChatModel_SafeCommands(t *testing.T) {
	commands := []string{
		"/config", "/history",
		"/yolo", "/sandbox",
		"/focus", "/pin", "/welcome",
		"/rename test-name",
		"/color blue", "/output-style markdown",
		"/vim", "/effort low",
		"/stale", "/sessions",
		"/power 5",
		"/stale", "/search test",
	}

	for _, cmd := range commands {
		t.Run(cmd, func(t *testing.T) {
			m := newTestChatModel()
			result, _ := m.handleCommand(cmd)
			if result == nil {
				t.Errorf("%s returned nil model", cmd)
			}
		})
	}
}
