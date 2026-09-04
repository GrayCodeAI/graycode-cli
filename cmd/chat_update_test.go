package cmd

import (
	"errors"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/GrayCodeAI/graycode-cli/internal/engine"
	"github.com/GrayCodeAI/graycode-cli/internal/sandbox"
)

// TestContainerStatusErrFallsBackToHostAutonomy preserves the historical test
// name while asserting the new fail-closed Docker-only contract.
func TestContainerStatusErrFallsBackToHostAutonomy(t *testing.T) {
	m := newTestChatModel()
	m.containerEnabled = true

	next, _ := m.Update(containerStatusMsg{err: errors.New("docker not running")})
	cm := requireChatModel(t, next)

	if !cm.containerEnabled {
		t.Fatal("container requirement must remain enabled after Docker failure")
	}
	if !cm.session.ContainerRequired() {
		t.Fatal("session must remain fail-closed when Docker is unavailable")
	}
	if cm.session.Tools().ContainerExecutor() != nil {
		t.Fatal("failed container must not leave an executor attached")
	}
	if !cm.containerRetryable {
		t.Fatal("Docker failure should remain retryable")
	}
}

// TestContainerStatusErrDoesNotClobberExplicitAutonomy ensures a Docker
// lifecycle failure does not mutate an explicitly configured autonomy tier.
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

func TestCtrlCTwiceExitsAndReleasesContainer(t *testing.T) {
	m := newTestChatModel()
	m.containerSandbox = sandbox.NewContainerSandbox(t.TempDir())

	first, _ := m.Update(tea.KeyPressMsg{Code: 'c', Mod: tea.ModCtrl})
	cm := requireChatModel(t, first)
	if cm.quitting {
		t.Fatal("first Ctrl+C must not mark the CLI as quitting")
	}

	cm.lastCtrlC = time.Now()
	second, secondCmd := cm.Update(tea.KeyPressMsg{Code: 'c', Mod: tea.ModCtrl})
	exited := requireChatModel(t, second)
	if !exited.quitting {
		t.Fatal("second Ctrl+C should exit the CLI")
	}
	if exited.containerSandbox != nil {
		t.Fatal("Ctrl+C exit should release the Docker container")
	}
	if secondCmd == nil {
		t.Fatal("second Ctrl+C should return tea.Quit")
	}
	if _, ok := secondCmd().(tea.QuitMsg); !ok {
		t.Fatalf("second Ctrl+C command = %T, want tea.QuitMsg", secondCmd())
	}
}

func TestCtrlCTwiceWhileStreamingCancelsThenExits(t *testing.T) {
	m := newTestChatModel()
	m.waiting = true
	cancelled := false
	m.cancel = func() { cancelled = true }

	first, firstCmd := m.Update(tea.KeyPressMsg{Code: 'c', Mod: tea.ModCtrl})
	cm := requireChatModel(t, first)
	if !cancelled {
		t.Fatal("first Ctrl+C should cancel the active stream")
	}
	if firstCmd != nil {
		t.Fatal("first Ctrl+C should cancel without quitting")
	}
	if cm.waiting {
		t.Fatal("first Ctrl+C should leave the model idle")
	}

	second, secondCmd := cm.Update(tea.KeyPressMsg{Code: 'c', Mod: tea.ModCtrl})
	exited := requireChatModel(t, second)
	if !exited.quitting {
		t.Fatal("second Ctrl+C should exit after cancelling the stream")
	}
	if secondCmd == nil {
		t.Fatal("second Ctrl+C should return tea.Quit")
	}
	if _, ok := secondCmd().(tea.QuitMsg); !ok {
		t.Fatalf("second Ctrl+C command = %T, want tea.QuitMsg", secondCmd())
	}
}

func TestExitAndQuitCommandsExitAndReleaseContainer(t *testing.T) {
	for _, command := range []string{"/exit", "/quit"} {
		t.Run(command, func(t *testing.T) {
			m := newTestChatModel()
			m.containerSandbox = sandbox.NewContainerSandbox(t.TempDir())

			next, cmd := m.handleSessionCommand(command, nil, command)
			cm := requireChatModel(t, next)
			if !cm.quitting {
				t.Fatalf("%s should mark the CLI as quitting", command)
			}
			if cm.containerSandbox != nil {
				t.Fatalf("%s should release the Docker container", command)
			}
			if cmd == nil {
				t.Fatalf("%s should return tea.Quit", command)
			}
			if _, ok := cmd().(tea.QuitMsg); !ok {
				t.Fatalf("%s command = %T, want tea.QuitMsg", command, cmd())
			}
		})
	}
}
