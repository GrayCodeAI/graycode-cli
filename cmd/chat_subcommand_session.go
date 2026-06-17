package cmd

import (
	tea "github.com/charmbracelet/bubbletea"
)

// sessionSubcommand is a single ChatSubcommand that dispatches to
// m.handleSessionCommand for all the session-management commands
// (/clear, /compact, /diff, /recover, /resume, /history, /quit, /exit).
//
// This is a "thin wrapper" migration: rather than splitting every
// session command into its own file (8+ files for marginal benefit),
// we wrap the existing handleSessionCommand dispatch behind a single
// ChatSubcommand implementation. The aliases cover all the public
// session command names.
type sessionSubcommand struct{}

func (s *sessionSubcommand) Name() string      { return "clear" }
func (s *sessionSubcommand) Aliases() []string {
	return []string{"compact", "diff", "recover", "resume", "history", "quit", "exit"}
}
func (s *sessionSubcommand) Description() string {
	return "session management: clear, compact, diff, recover, resume, history, quit, exit"
}
func (s *sessionSubcommand) Usage() string { return "" }
func (s *sessionSubcommand) Handle(m *chatModel, args []string, text string) (tea.Model, tea.Cmd) {
	name := ""
	if len(text) > 0 && text[0] == '/' {
		for _, c := range []string{"/clear", "/compact", "/diff", "/recover", "/resume", "/history", "/quit", "/exit"} {
			if len(text) >= len(c) && text[:len(c)] == c {
				name = c
				break
			}
		}
	}
	if name == "" {
		name = "/" + s.Name()
	}
	return m.handleSessionCommand(name, args, text)
}

func init() {
	subcommandRegistry.Register(&sessionSubcommand{})
}
