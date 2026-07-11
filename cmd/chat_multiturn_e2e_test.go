package cmd

import (
	"context"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/GrayCodeAI/eyrie/credentials"
	hawkconfig "github.com/GrayCodeAI/hawk/internal/config"
	"github.com/GrayCodeAI/hawk/internal/engine"
)

func configureReadyChatState(t *testing.T) {
	t.Helper()
	isolateChatCommandSweepEnv(t)

	hawkconfig.InvalidateConfigUICache()
	store := &credentials.MapStore{}
	credentials.SetDefaultStore(store)
	t.Cleanup(func() {
		credentials.SetDefaultStore(nil)
		hawkconfig.InvalidateConfigUICache()
	})

	ctx := context.Background()
	if err := store.Set(ctx, credentials.AccountForEnv("OPENROUTER_API_KEY"), "sk-or-test-key-1234567890"); err != nil {
		t.Fatal(err)
	}
	if err := hawkconfig.SetActiveProvider(ctx, "openrouter"); err != nil {
		t.Fatal(err)
	}
	if err := hawkconfig.SetActiveModel(ctx, "openrouter/auto"); err != nil {
		t.Fatal(err)
	}
	hawkconfig.InvalidateConfigUICache()
	hawkconfig.RefreshConfigCredSnapshot(ctx)
}

func countMessagesByRole(msgs []displayMsg, role string) int {
	count := 0
	for _, msg := range msgs {
		if msg.role == role {
			count++
		}
	}
	return count
}

func lastMessageByRole(msgs []displayMsg, role string) string {
	for i := len(msgs) - 1; i >= 0; i-- {
		if msgs[i].role == role {
			return msgs[i].content
		}
	}
	return ""
}

func TestChatModel_MultiTurnQueuedConversationE2E(t *testing.T) {
	configureReadyChatState(t)

	m := newTestChatModel()
	m.session.SetModel("")
	m.session.SetProvider("")

	m.input.SetValue("first question")
	result, _ := m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	m = requireChatModel(t, result)
	if !m.waiting {
		t.Fatal("first enter should start a waiting chat turn")
	}
	if got := m.session.Provider(); got != "openrouter" {
		t.Fatalf("session provider = %q, want openrouter", got)
	}
	if got := m.session.Model(); got != "openrouter/auto" {
		t.Fatalf("session model = %q, want openrouter/auto", got)
	}
	if count := countMessagesByRole(m.messages, "user"); count != 1 {
		t.Fatalf("user message count after first enter = %d, want 1", count)
	}

	m.input.SetValue("second question")
	result, _ = m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	m = requireChatModel(t, result)
	if len(m.messageQueue) != 1 || m.messageQueue[0] != "second question" {
		t.Fatalf("queued messages = %v, want [second question]", m.messageQueue)
	}
	if got := lastSystemMessage(m.messages); !strings.Contains(got, "Queued: second question") {
		t.Fatalf("expected queued message notice, got %q", got)
	}

	result, _ = m.Update(streamChunkMsg("I fixed the failing test in auth_test.go"))
	m = requireChatModel(t, result)
	result, _ = m.Update(usageUpdateMsg{usage: &engine.StreamUsage{PromptTokens: 12, CompletionTokens: 6}})
	m = requireChatModel(t, result)
	result, _ = m.Update(streamDoneMsg{})
	m = requireChatModel(t, result)
	if !m.waiting {
		t.Fatal("queued second message should auto-start after first stream completes")
	}
	if len(m.messageQueue) != 0 {
		t.Fatalf("message queue should be drained, got %v", m.messageQueue)
	}
	if count := countMessagesByRole(m.messages, "assistant"); count != 1 {
		t.Fatalf("assistant message count after first done = %d, want 1", count)
	}
	if got := lastMessageByRole(m.messages, "user"); got != "second question" {
		t.Fatalf("last user message after queue release = %q, want second question", got)
	}
	if !m.ghostText.Active() {
		t.Fatal("assistant completion should populate ghost text suggestion")
	}

	result, _ = m.Update(streamChunkMsg("Tests passed after the fix."))
	m = requireChatModel(t, result)
	result, _ = m.Update(streamDoneMsg{})
	m = requireChatModel(t, result)
	if m.waiting {
		t.Fatal("second stream should finish the conversation")
	}
	if count := countMessagesByRole(m.messages, "user"); count != 2 {
		t.Fatalf("final user message count = %d, want 2", count)
	}
	if count := countMessagesByRole(m.messages, "assistant"); count != 2 {
		t.Fatalf("final assistant message count = %d, want 2", count)
	}
	if m.turnInputTokens != 0 || m.turnOutputTokens != 0 {
		t.Fatalf("queued second turn should reset per-turn tokens, got in=%d out=%d", m.turnInputTokens, m.turnOutputTokens)
	}

	result, cmd := m.Update(tea.KeyPressMsg{Code: tea.KeyUp})
	if cmd != nil {
		result, _ = result.(chatModel).Update(cmd())
	}
	m = requireChatModel(t, result)
	if got := m.input.Value(); got != "second question" {
		t.Fatalf("first history recall = %q, want second question", got)
	}
	result, cmd = m.Update(tea.KeyPressMsg{Code: tea.KeyUp})
	if cmd != nil {
		result, _ = result.(chatModel).Update(cmd())
	}
	m = requireChatModel(t, result)
	if got := m.input.Value(); got != "first question" {
		t.Fatalf("second history recall = %q, want first question", got)
	}
	result, cmd = m.Update(tea.KeyPressMsg{Code: tea.KeyDown})
	if cmd != nil {
		result, _ = result.(chatModel).Update(cmd())
	}
	m = requireChatModel(t, result)
	if got := m.input.Value(); got != "second question" {
		t.Fatalf("history down = %q, want second question", got)
	}
	result, cmd = m.Update(tea.KeyPressMsg{Code: tea.KeyDown})
	if cmd != nil {
		result, _ = result.(chatModel).Update(cmd())
	}
	m = requireChatModel(t, result)
	if got := m.input.Value(); got != "" {
		t.Fatalf("history should return to draft input, got %q", got)
	}

	m.viewDirty = true
	m.updateViewportContent()
	rendered := m.View().Content
	if !strings.Contains(rendered, "first question") || !strings.Contains(rendered, "Tests passed after the fix.") {
		t.Fatalf("rendered chat missing final transcript:\n%s", rendered)
	}
}
