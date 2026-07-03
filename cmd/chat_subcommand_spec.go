package cmd

import (
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/GrayCodeAI/hawk/internal/engine"
)

// specSubcommand implements the /spec slash command: starts (or reports
// the status of) the independent spec-driven workflow, which gates
// Write/Edit/Bash until a spec/plan/tasks are written and the user
// approves moving to implementation. Independent of the /autonomy trust
// tier — works at any tier, including Autonomous.
type specSubcommand struct{}

func (s *specSubcommand) Name() string      { return "spec" }
func (s *specSubcommand) Aliases() []string { return nil }
func (s *specSubcommand) Description() string {
	return "start or check the spec-driven workflow (gates Write/Edit/Bash)"
}

func (s *specSubcommand) Usage() string { return "/spec [what to build] | /spec status | /spec reset" }

func (s *specSubcommand) Handle(m *chatModel, args []string, text string) (tea.Model, tea.Cmd) {
	if m.session == nil {
		m.messages = append(m.messages, displayMsg{role: "error", content: "No active session."})
		return m, nil
	}
	arg := strings.TrimSpace(strings.TrimPrefix(text, "/spec"))

	switch strings.ToLower(arg) {
	case "":
		if m.specPicker == nil {
			m.specPicker = NewSpecPicker(m.width)
		}
		m.specPicker.Open(currentSpecStage(m.session))
		return m, nil
	case "status":
		m.messages = append(m.messages, displayMsg{role: "system", content: fmt.Sprintf("Spec stage: %s", specStageLabel(m.session))})
		return m, nil
	case "reset":
		m.session.PermSvc().SetSpecStage(engine.SpecStageNone)
		m.messages = append(m.messages, displayMsg{role: "system", content: "Spec workflow reset — Write/Edit/Bash follow the normal autonomy tier again."})
		return m, nil
	}

	m.session.PermSvc().SetSpecStage(engine.SpecStageSpecify)
	m.messages = append(m.messages, displayMsg{role: "system", content: "Spec workflow started — Write/Edit/Bash are gated until spec.md, plan.md, and tasks.md are written and ApproveImplementation is approved."})
	if arg == "" {
		return m, nil
	}
	return m.startPromptCommand("/spec", arg)
}

func init() {
	subcommandRegistry.Register(&specSubcommand{})
}
