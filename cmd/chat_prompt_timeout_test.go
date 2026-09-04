package cmd

import (
	"strings"
	"testing"

	contracts "github.com/GrayCodeAI/graycode-cli/internal/contracts/policy"
	"github.com/GrayCodeAI/graycode-cli/internal/engine"
)

func TestPermissionPromptTimeoutClearsStaleState(t *testing.T) {
	m := newTestChatModel()
	req := engine.PermissionRequest{
		PermissionRequest: contracts.PermissionRequest{
			ToolName: "Bash",
			Summary:  "run git status",
		},
		Response: make(chan bool, 1),
	}

	next, cmd := m.Update(permissionAskMsg{req: req})
	cm := requireChatModel(t, next)
	if cm.permReq == nil {
		t.Fatal("expected permission request to be active")
	}
	if cmd == nil {
		t.Fatal("expected timeout command for permission prompt")
	}
	seq := cm.permReqSeq

	next, _ = cm.Update(permissionPromptTimeoutMsg{seq: seq})
	cm = requireChatModel(t, next)
	if cm.permReq != nil {
		t.Fatal("expected timed-out permission request to be cleared")
	}
	if got := lastSystemMessage(cm.messages); !strings.Contains(got, "Permission prompt timed out") {
		t.Fatalf("unexpected timeout message: %q", got)
	}
}

func TestAskUserPromptTimeoutClearsStaleState(t *testing.T) {
	m := newTestChatModel()
	msg := askUserMsg{
		question: "Continue?",
		response: make(chan string, 1),
	}

	next, cmd := m.Update(msg)
	cm := requireChatModel(t, next)
	if cm.askReq == nil {
		t.Fatal("expected ask-user request to be active")
	}
	if cmd == nil {
		t.Fatal("expected timeout command for ask-user prompt")
	}
	seq := cm.askReqSeq

	next, _ = cm.Update(askUserPromptTimeoutMsg{seq: seq})
	cm = requireChatModel(t, next)
	if cm.askReq != nil {
		t.Fatal("expected timed-out ask-user request to be cleared")
	}
	if got := lastSystemMessage(cm.messages); !strings.Contains(got, "Question timed out") {
		t.Fatalf("unexpected timeout message: %q", got)
	}
}

func TestPromptTimeoutIgnoresNewerPrompt(t *testing.T) {
	m := newTestChatModel()

	next, _ := m.Update(askUserMsg{question: "First?", response: make(chan string, 1)})
	cm := requireChatModel(t, next)
	firstSeq := cm.askReqSeq

	next, _ = cm.Update(askUserMsg{question: "Second?", response: make(chan string, 1)})
	cm = requireChatModel(t, next)
	secondSeq := cm.askReqSeq
	if secondSeq == firstSeq {
		t.Fatal("expected newer ask-user prompt sequence")
	}

	next, _ = cm.Update(askUserPromptTimeoutMsg{seq: firstSeq})
	cm = requireChatModel(t, next)
	if cm.askReq == nil {
		t.Fatal("stale timeout should not clear newer ask-user prompt")
	}
}

func TestStreamErrClearsInteractivePromptState(t *testing.T) {
	m := newTestChatModel()
	req := engine.PermissionRequest{
		PermissionRequest: contracts.PermissionRequest{
			ToolName: "Bash",
			Summary:  "run git status",
		},
		Response: make(chan bool, 1),
	}
	next, _ := m.Update(permissionAskMsg{req: req})
	cm := requireChatModel(t, next)
	next, _ = cm.Update(askUserMsg{question: "Continue?", response: make(chan string, 1)})
	cm = requireChatModel(t, next)

	next, _ = cm.Update(streamErrMsg{err: errNoInteractivePromptInput})
	cm = requireChatModel(t, next)
	if cm.permReq != nil || cm.askReq != nil {
		t.Fatal("stream error should clear stale interactive prompt state")
	}
}
