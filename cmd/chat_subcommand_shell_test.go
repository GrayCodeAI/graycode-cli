package cmd

import (
	"strings"
	"testing"

	"github.com/GrayCodeAI/graycode-cli/internal/engine"
)

func TestRunSubcommand_UsesBashToolForSafeCommand(t *testing.T) {
	m := newTestChatModel()
	m.session.PermSvc().SetAutonomy(engine.AutonomyYOLO)

	result, _ := (&runSubcommand{}).Handle(m, []string{"printf", "hello"}, "/run printf hello")
	cm := requireChatModel(t, result)

	if len(cm.messages) == 0 {
		t.Fatal("expected display message")
	}
	last := cm.messages[len(cm.messages)-1]
	if last.role != "system" {
		t.Fatalf("last role = %q, want system", last.role)
	}
	if !strings.Contains(last.content, "$ printf hello") || !strings.Contains(last.content, "hello") {
		t.Fatalf("display output = %q, want command and hello", last.content)
	}
	msgs := cm.session.RawMessages()
	if len(msgs) != 1 || !strings.Contains(msgs[0].Content, "hello") {
		t.Fatalf("session messages = %#v, want command output context", msgs)
	}
}

func TestRunSubcommand_RespectsSpecStageGate(t *testing.T) {
	m := newTestChatModel()
	m.session.PermSvc().SetSpecStage(engine.SpecStageSpecify)

	result, _ := (&runSubcommand{}).Handle(m, []string{"printf", "hello"}, "/run printf hello")
	cm := requireChatModel(t, result)

	if len(cm.messages) == 0 {
		t.Fatal("expected display message")
	}
	last := cm.messages[len(cm.messages)-1]
	if last.role != "error" {
		t.Fatalf("last role = %q, want error", last.role)
	}
	if !strings.Contains(last.content, "Spec stage active") {
		t.Fatalf("display output = %q, want spec-gate denial", last.content)
	}
	if got := len(cm.session.RawMessages()); got != 0 {
		t.Fatalf("session messages = %d, want 0 after denied command", got)
	}
}

func TestRunSubcommand_BashToolBlocksEnvDumpEvenWhenBypassed(t *testing.T) {
	m := newTestChatModel()
	m.session.PermSvc().SetAutonomy(engine.AutonomyYOLO)

	result, _ := (&runSubcommand{}).Handle(m, []string{"env"}, "/run env")
	cm := requireChatModel(t, result)

	if len(cm.messages) == 0 {
		t.Fatal("expected display message")
	}
	last := cm.messages[len(cm.messages)-1]
	if last.role != "error" {
		t.Fatalf("last role = %q, want error", last.role)
	}
	if !strings.Contains(last.content, "blocked: dumping environment variables") {
		t.Fatalf("display output = %q, want BashTool env dump block", last.content)
	}
}

func TestTestSubcommand_DetectsNonzeroExit(t *testing.T) {
	m := newTestChatModel()
	m.session.PermSvc().SetAutonomy(engine.AutonomyYOLO)

	result, _ := (&testSubcommand{}).Handle(m, []string{"false"}, "/test false")
	cm := requireChatModel(t, result)

	if len(cm.messages) == 0 {
		t.Fatal("expected display message")
	}
	last := cm.messages[len(cm.messages)-1]
	if last.role != "system" {
		t.Fatalf("last role = %q, want system", last.role)
	}
	if !strings.Contains(last.content, "Tests failed:") || !strings.Contains(last.content, "exit code") {
		t.Fatalf("display output = %q, want failed test output", last.content)
	}
	msgs := cm.session.RawMessages()
	if len(msgs) != 1 || !strings.Contains(msgs[0].Content, "Please fix these test failures") {
		t.Fatalf("session messages = %#v, want failure context", msgs)
	}
}
