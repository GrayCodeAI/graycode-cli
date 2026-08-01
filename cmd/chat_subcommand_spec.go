package cmd

import (
	"fmt"
	"strings"

	tea "charm.land/bubbletea/v2"

	"github.com/GrayCodeAI/hawk/internal/engine"
	"github.com/GrayCodeAI/hawk/internal/spec"
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

func (s *specSubcommand) Usage() string {
	return "/spec [what to build] | /spec status | /spec reset | /spec config [set <field> <value>]"
}

func (s *specSubcommand) Handle(m *chatModel, args []string, text string) (tea.Model, tea.Cmd) {
	if m.session == nil {
		m.messages = append(m.messages, displayMsg{role: "error", content: "No active session."})
		return m, nil
	}
	arg := strings.TrimSpace(strings.TrimPrefix(text, "/spec"))

	switch {
	case arg == "":
		if m.specPicker == nil {
			m.specPicker = NewSpecPicker(m.width)
		}
		m.specPicker.Open(currentSpecStage(m.session))
		return m, nil

	case strings.EqualFold(arg, "status"):
		m.messages = append(m.messages, displayMsg{role: "system", content: fmt.Sprintf("Spec stage: %s", specStageLabel(m.session))})
		return m, nil

	case strings.EqualFold(arg, "reset"):
		m.session.PermSvc().ResetSpec()
		m.messages = append(m.messages, displayMsg{role: "system", content: "Spec workflow reset — Write/Edit/Bash follow the normal autonomy tier again."})
		return m, nil

	case strings.HasPrefix(strings.ToLower(arg), "config"):
		return handleSpecConfig(m, arg)
	}

	m.session.PermSvc().SetSpecStage(engine.SpecStageSpecify)
	m.messages = append(m.messages, displayMsg{role: "system", content: "Spec workflow started — Write/Edit/Bash are gated until spec.md, plan.md, and tasks.md are written and ApproveImplementation is approved."})
	if arg == "" {
		return m, nil
	}
	return m.startPromptCommand("/spec", arg)
}

// handleSpecConfig processes /spec config subcommands.
func handleSpecConfig(m *chatModel, arg string) (tea.Model, tea.Cmd) {
	rest := strings.TrimSpace(strings.TrimPrefix(arg, "config"))
	parts := strings.Fields(rest)

	if len(parts) == 0 {
		// No sub-command: show current config
		cfg := spec.LoadSpecConfig()
		msg := "Spec configuration:\n" + cfg.Format()
		msg += "\n\nUse `/spec config set <field> <value>` to set a field."
		msg += "\nUse `/spec config list` to see available fields."
		msg += "\nThe agent can also use the `SpecConfig` tool to read/update config."
		m.messages = append(m.messages, displayMsg{role: "system", content: msg})
		return m, nil
	}

	switch strings.ToLower(parts[0]) {
	case "list":
		var b strings.Builder
		b.WriteString("Available config fields:\n\n")
		for _, f := range spec.SpecConfigFields() {
			b.WriteString(fmt.Sprintf("  %s (%s)\n    %s\n", f.Label, f.Key, f.Help))
			if len(f.Examples) > 0 {
				b.WriteString(fmt.Sprintf("    Examples: %s\n", strings.Join(f.Examples, ", ")))
			}
			b.WriteString("\n")
		}
		b.WriteString("Use `/spec config set <field> <value>` to set.\n")
		b.WriteString("Use value 'ai' to let the AI decide.")
		m.messages = append(m.messages, displayMsg{role: "system", content: strings.TrimSpace(b.String())})
		return m, nil

	case "get":
		cfg := spec.LoadSpecConfig()
		m.messages = append(m.messages, displayMsg{role: "system", content: "Spec configuration:\n" + cfg.Format()})
		return m, nil

	case "set":
		if len(parts) < 2 {
			m.messages = append(m.messages, displayMsg{role: "error", content: "Usage: /spec config set <field> <value>    e.g. /spec config set language Go"})
			return m, nil
		}
		field := strings.ToLower(parts[1])
		value := strings.Join(parts[2:], " ")

		// Validate field
		valid := false
		for _, f := range spec.SpecConfigFields() {
			if f.Key == field {
				valid = true
				break
			}
		}
		if !valid {
			msg := fmt.Sprintf("Unknown field %q. Available: ", field)
			var names []string
			for _, f := range spec.SpecConfigFields() {
				names = append(names, f.Key)
			}
			msg += strings.Join(names, ", ")
			m.messages = append(m.messages, displayMsg{role: "error", content: msg})
			return m, nil
		}

		cfg := spec.LoadSpecConfig()
		switch field {
		case "language":
			cfg.Language = value
		case "framework":
			cfg.Framework = value
		case "methodology":
			cfg.Methodology = value
		case "architecture":
			cfg.Architecture = value
		case "repo_structure":
			cfg.RepoStructure = value
		case "custom_prompt":
			cfg.CustomPrompt = value
		}

		if err := spec.SaveSpecConfig(cfg); err != nil {
			m.messages = append(m.messages, displayMsg{role: "error", content: fmt.Sprintf("Failed to save config: %v", err)})
			return m, nil
		}
		m.messages = append(m.messages, displayMsg{role: "system", content: fmt.Sprintf("Updated spec config: %s = %s\n\n%s", field, value, cfg.Format())})
		return m, nil

	default:
		m.messages = append(m.messages, displayMsg{role: "error", content: fmt.Sprintf("Unknown config subcommand %q. Use: get, set, list", parts[0])})
		return m, nil
	}
}

func init() {
	subcommandRegistry.Register(&specSubcommand{})
}
