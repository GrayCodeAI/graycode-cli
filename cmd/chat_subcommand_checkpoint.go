package cmd

import (
	tea "charm.land/bubbletea/v2"

	"github.com/GrayCodeAI/graycode-cli/internal/engine"
)

// checkpointSubcommand implements the /checkpoint slash command.
// It asks the model to checkpoint its progress on the current task.
type checkpointSubcommand struct{}

func (c *checkpointSubcommand) Name() string        { return "checkpoint" }
func (c *checkpointSubcommand) Aliases() []string   { return nil }
func (c *checkpointSubcommand) Description() string { return "checkpoint progress on the current task" }
func (c *checkpointSubcommand) Usage() string       { return "" }
func (c *checkpointSubcommand) Handle(m *chatModel, args []string, text string) (tea.Model, tea.Cmd) {
	return m.startPromptCommand("/checkpoint", engine.CheckpointPrompts(engine.CheckpointOrientation, nil))
}

func init() {
	subcommandRegistry.Register(&checkpointSubcommand{})
}
