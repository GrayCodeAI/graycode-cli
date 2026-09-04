package cmd

import (
	"fmt"
	"strings"

	tea "charm.land/bubbletea/v2"

	graycodeconfig "github.com/GrayCodeAI/graycode-cli/internal/config"
)

// modelSubcommand implements the /model slash command. It shows the
// model picker or sets the active model for the session.
type modelSubcommand struct{}

func (mo *modelSubcommand) Name() string      { return "model" }
func (mo *modelSubcommand) Aliases() []string { return nil }
func (mo *modelSubcommand) Description() string {
	return "browse models (t toggles Think) or set the active model"
}
func (mo *modelSubcommand) Usage() string { return "/model [model-name|set <model-name>]" }
func (mo *modelSubcommand) Handle(m *chatModel, args []string, text string) (tea.Model, tea.Cmd) {
	if len(args) == 0 {
		next, cmd := m.openConfigAtTab(configTabModels)
		*m = next
		return m, cmd
	}
	arg := strings.TrimSpace(strings.TrimPrefix(text, "/model"))
	arg = strings.TrimSpace(strings.TrimPrefix(arg, "set"))
	if arg == "" {
		m.messages = append(m.messages, displayMsg{role: "error", content: "Usage: /model <model-name> or /model set <model-name>\nOpen /model with no args to browse; press t to toggle Think."})
		return m, nil
	}
	var selected *configModelOption
	known := configModelChoices(m.configModelOptions, false)
	if len(known) > 0 {
		found := false
		for i, k := range known {
			if strings.EqualFold(k, arg) || strings.EqualFold(m.configModelOptions[i].ID, arg) {
				arg = m.configModelOptions[i].ID
				opt := m.configModelOptions[i]
				selected = &opt
				found = true
				break
			}
		}
		if !found {
			hint := "Unknown model: " + arg + "\nAvailable models for " + m.session.Provider() + ":\n"
			max := 10
			if len(known) < max {
				max = len(known)
			}
			for _, k := range known[:max] {
				hint += "  " + k + "\n"
			}
			if len(known) > 10 {
				hint += fmt.Sprintf("  ... and %d more (use /model to browse)", len(known)-10)
			}
			m.messages = append(m.messages, displayMsg{role: "error", content: hint})
			return m, nil
		}
	}
	if graycodeconfig.DeploymentRoutingEnabled(m.settings) {
		arg = graycodeconfig.ResolveCanonicalModel(arg)
	}
	prevModel := m.session.Model()
	if strings.EqualFold(strings.TrimSpace(prevModel), strings.TrimSpace(arg)) {
		m.messages = append(m.messages, displayMsg{role: "system", content: fmt.Sprintf("Already using %s — no change.", prevModel)})
		return m, nil
	}
	if err := graycodeconfig.SetGlobalSetting("model", arg); err != nil {
		m.messages = append(m.messages, displayMsg{role: "error", content: err.Error()})
		return m, nil
	}
	msgCount := len(m.session.RawMessages())
	m.session.SetModel(arg)
	if selected != nil {
		m.applyModelThinkingPref(*selected)
	} else {
		provider := ""
		if m.session != nil {
			provider = m.session.Provider()
		}
		m.session.SetThinkingEnabled(graycodeconfig.ResolveThinkingForModel(graycodeconfig.LoadSettings(), arg, provider))
	}
	thinkLabel := graycodeconfig.FormatModelThinkingLabel(
		selected != nil && graycodeconfig.ModelCapabilitySupportsThinking(selected.Capabilities),
		graycodeconfig.ThinkingPrefForModel(graycodeconfig.LoadSettings(), arg),
		m.session.Provider(),
	)
	m.messages = append(m.messages, displayMsg{role: "system", content: fmt.Sprintf(
		"Model switched: %s → %s (Think: %s)\nConversation history preserved (%d messages); new requests use the new model.\nSaved in eyrie (provider.json). Use /model and press t to toggle Think.",
		prevModel, m.session.Model(), thinkLabel, msgCount,
	)})
	return m, nil
}

func init() {
	subcommandRegistry.Register(&modelSubcommand{})
}
