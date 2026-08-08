package cmd

import (
	"strings"
	"testing"

	"github.com/GrayCodeAI/hawk/internal/engine"
)

func TestYOLOConfirm_PendingConsumesNextInput(t *testing.T) {
	m := newTestChatModel()
	m.pendingYOLOConfirm = true

	// Non-matching input cancels and stays on the previous tier.
	m.input.SetValue("nope")
	next, _ := m.submitUserMessage()
	if next.pendingYOLOConfirm {
		t.Error("pendingYOLOConfirm should be cleared after any submission")
	}
	if next.session.PermSvc().Autonomy() == engine.AutonomyYOLO {
		t.Error("autonomy should NOT be YOLO after a non-matching confirmation")
	}
	if len(next.messages) == 0 || !strings.Contains(next.messages[len(next.messages)-1].content, "cancelled") {
		t.Errorf("expected a cancellation message, got %d messages", len(next.messages))
	}
}

func TestYOLOConfirm_ExactTokenEnables(t *testing.T) {
	m := newTestChatModel()
	m.pendingYOLOConfirm = true

	m.input.SetValue(yoloConfirmToken)
	next, _ := m.submitUserMessage()
	if next.pendingYOLOConfirm {
		t.Error("pendingYOLOConfirm should be cleared after confirmation")
	}
	if next.session.PermSvc().Autonomy() != engine.AutonomyYOLO {
		t.Errorf("autonomy = %v, want YOLO after matching confirmation", next.session.PermSvc().Autonomy())
	}
}

func TestYOLOConfirm_CaseInsensitive(t *testing.T) {
	m := newTestChatModel()
	m.pendingYOLOConfirm = true

	m.input.SetValue(strings.ToUpper(yoloConfirmToken))
	next, _ := m.submitUserMessage()
	if next.session.PermSvc().Autonomy() != engine.AutonomyYOLO {
		t.Error("confirmation token should match case-insensitively")
	}
}

func TestYOLOConfirm_DoesNotLeakIntoNormalSubmit(t *testing.T) {
	m := newTestChatModel()
	m.pendingYOLOConfirm = false

	m.input.SetValue(yoloConfirmToken)
	next, _ := m.submitUserMessage()
	if next.session.PermSvc().Autonomy() == engine.AutonomyYOLO {
		t.Error("autonomy should not change without a pending confirmation")
	}
}
