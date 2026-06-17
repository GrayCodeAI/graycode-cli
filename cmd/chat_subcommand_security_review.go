package cmd

import (
	tea "github.com/charmbracelet/bubbletea"
)

// securityReviewSubcommand implements the /security-review slash
// command. It prompts the model to review the repo for security
// risks.
type securityReviewSubcommand struct{}

func (s *securityReviewSubcommand) Name() string        { return "security-review" }
func (s *securityReviewSubcommand) Aliases() []string   { return nil }
func (s *securityReviewSubcommand) Description() string { return "review the repository for security risks" }
func (s *securityReviewSubcommand) Usage() string       { return "" }
func (s *securityReviewSubcommand) Handle(m *chatModel, args []string, text string) (tea.Model, tea.Cmd) {
	return m.startPromptCommand("/security-review", "Review the repository for security risks. Focus on command execution, file permissions, secret exposure, network access, authentication, and unsafe defaults.")
}

func init() {
	subcommandRegistry.Register(&securityReviewSubcommand{})
}
