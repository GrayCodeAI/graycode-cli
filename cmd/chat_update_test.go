package cmd

import (
	"errors"
	"testing"

	"github.com/GrayCodeAI/hawk/internal/engine"
)

// TestContainerStatusErrFallsBackToHostAutonomy covers the async fallback
// path (Site B): when Docker turns out to be unavailable, the session
// should fall back to host mode and pick up DefaultHostAutonomy instead of
// staying at the implicit Supervised zero-value.
func TestContainerStatusErrFallsBackToHostAutonomy(t *testing.T) {
	m := newTestChatModel()
	m.containerEnabled = true

	next, _ := m.Update(containerStatusMsg{err: errors.New("docker not running")})
	cm := requireChatModel(t, next)

	if cm.containerEnabled {
		t.Fatal("expected containerEnabled to be false after container error")
	}
	if got := cm.session.PermSvc().Autonomy(); got != DefaultHostAutonomy {
		t.Fatalf("got autonomy %v, want %v (DefaultHostAutonomy)", got, DefaultHostAutonomy)
	}
}

// TestContainerStatusErrDoesNotClobberExplicitAutonomy ensures the fallback
// only applies the default when nothing was explicitly configured.
func TestContainerStatusErrDoesNotClobberExplicitAutonomy(t *testing.T) {
	m := newTestChatModel()
	m.containerEnabled = true
	m.session.PermSvc().SetAutonomy(engine.AutonomyYOLO)

	next, _ := m.Update(containerStatusMsg{err: errors.New("docker not running")})
	cm := requireChatModel(t, next)

	if got := cm.session.PermSvc().Autonomy(); got != engine.AutonomyYOLO {
		t.Fatalf("got autonomy %v, want AutonomyYOLO preserved", got)
	}
}
