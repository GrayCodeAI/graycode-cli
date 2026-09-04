package cmd

import (
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	contracts "github.com/GrayCodeAI/graycode-cli/internal/contracts/policy"
	"github.com/GrayCodeAI/graycode-cli/internal/engine"
)

func TestPermissionAlwaysAllowDoesNotNilDeref(t *testing.T) {
	m := newTestChatModel()
	req := engine.PermissionRequest{
		PermissionRequest: contracts.PermissionRequest{
			ToolName: "Bash",
			Summary:  "git -C /tmp status",
		},
		Response: make(chan bool, 1),
	}

	next, _ := m.Update(permissionAskMsg{req: req})
	cm := requireChatModel(t, next)
	if cm.permReq == nil {
		t.Fatal("expected active permission request")
	}

	next, _ = cm.Update(tea.KeyPressMsg{Code: 'a', Text: "a"})
	cm = requireChatModel(t, next)
	if cm.permReq != nil {
		t.Fatal("expected permission request cleared after always-allow")
	}
	select {
	case allowed := <-req.Response:
		if !allowed {
			t.Fatal("expected always-allow to approve the request")
		}
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for permission response")
	}
	if got := lastSystemMessage(cm.messages); !strings.Contains(got, "Always allowed: Bash") {
		t.Fatalf("unexpected always-allow message: %q", got)
	}
	decision := cm.session.PermSvc().Memory().Check("Bash", "anything")
	if decision == nil || !*decision {
		t.Fatal("expected Bash:* always-allow rule to be recorded")
	}
}

func TestPermissionAlwaysDenyDoesNotNilDeref(t *testing.T) {
	m := newTestChatModel()
	req := engine.PermissionRequest{
		PermissionRequest: contracts.PermissionRequest{
			ToolName: "Bash",
			Summary:  "rm -rf /",
		},
		Response: make(chan bool, 1),
	}

	next, _ := m.Update(permissionAskMsg{req: req})
	cm := requireChatModel(t, next)

	next, _ = cm.Update(tea.KeyPressMsg{Code: 'd', Text: "d"})
	cm = requireChatModel(t, next)
	if cm.permReq != nil {
		t.Fatal("expected permission request cleared after always-deny")
	}
	select {
	case allowed := <-req.Response:
		if allowed {
			t.Fatal("expected always-deny to reject the request")
		}
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for permission response")
	}
	if got := lastSystemMessage(cm.messages); !strings.Contains(got, "Always denied: Bash") {
		t.Fatalf("unexpected always-deny message: %q", got)
	}
}
