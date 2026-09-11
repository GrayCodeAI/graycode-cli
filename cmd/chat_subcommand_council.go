package cmd

import (
	"context"
	"fmt"
	"strings"

	tea "charm.land/bubbletea/v2"

	"github.com/GrayCodeAI/hawk/internal/engine"
	"github.com/GrayCodeAI/hawk/internal/ui/icons"
)

// councilSubcommand implements the /council slash command. It
// runs a 3-stage multi-model discussion: query all council models,
// rank the responses, then have the chairman synthesize.
type councilSubcommand struct{}

func (co *councilSubcommand) Name() string      { return "council" }
func (co *councilSubcommand) Aliases() []string { return nil }
func (co *councilSubcommand) Description() string {
	return "convene a multi-model council to discuss a question"
}
func (co *councilSubcommand) Usage() string { return "/council <question>" }
func (co *councilSubcommand) Handle(m *chatModel, args []string, text string) (tea.Model, tea.Cmd) {
	if len(args) < 1 {
		m.messages = append(m.messages, displayMsg{role: "error", content: "Usage: /council <question>"})
		return m, nil
	}
	query := strings.TrimSpace(strings.TrimPrefix(text, "/council"))
	cfg := engine.CouncilConfig{
		Models:   engine.DefaultCouncilModels(),
		Chairman: m.session.Model(),
	}
	m.messages = append(m.messages, displayMsg{role: "system", content: fmt.Sprintf("%s Council convened: %s (chairman: %s)", icons.Robot(), strings.Join(cfg.Models, ", "), cfg.Chairman)})
	m.messages = append(m.messages, displayMsg{role: "system", content: "Stage 1: Querying all models..."})

	result, err := engine.RunCouncil(context.Background(), query, cfg, m.session)
	if err != nil {
		m.messages = append(m.messages, displayMsg{role: "error", content: "Council failed: " + err.Error()})
		return m, nil
	}

	for _, r := range result.Responses {
		preview := truncateWithEllipsis(r.Response, 203)
		m.messages = append(m.messages, displayMsg{role: "system", content: fmt.Sprintf("  [%s] %s", r.Model, preview)})
	}

	m.messages = append(m.messages, displayMsg{role: "system", content: "Stage 2: Ranking responses..."})
	for _, r := range result.Rankings {
		preview := truncateWithEllipsis(r.Ranking, 203)
		m.messages = append(m.messages, displayMsg{role: "system", content: fmt.Sprintf("  [%s] %s", r.Model, preview)})
	}

	m.messages = append(m.messages, displayMsg{role: "system", content: "Stage 3: Chairman synthesizing..."})
	m.messages = append(m.messages, displayMsg{role: "assistant", content: result.Synthesis})
	return m, nil
}

func init() {
	subcommandRegistry.Register(&councilSubcommand{})
}
