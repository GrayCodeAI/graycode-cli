package cmd

import (
	tea "charm.land/bubbletea/v2"
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

func (s *sessionSubcommand) Name() string { return "clear" }
func (s *sessionSubcommand) Aliases() []string {
	return []string{"compact", "diff", "recover", "resume", "history", "quit", "exit"}
}

func (s *sessionSubcommand) Description() string {
	return "session management: clear, compact, diff, recover, resume, history, quit, exit"
}
func (s *sessionSubcommand) Usage() string { return "" }

// sessionSubcommandNames is the ordered list of slash names covered
// by sessionSubcommand. Exposed for tests and help rendering.
var sessionSubcommandNames = []string{"/clear", "/compact", "/diff", "/recover", "/resume", "/history", "/quit", "/exit"}

// resolveSessionName inspects the raw slash text and returns the
// matching command name (with the leading "/"). Falls back to
// "/<primary>" when text doesn't start with a slash.
func resolveSessionName(text string, primary string) string {
	if len(text) > 0 && text[0] == '/' {
		for _, c := range sessionSubcommandNames {
			if len(text) >= len(c) && text[:len(c)] == c {
				return c
			}
		}
	}
	return "/" + primary
}

// buildSessionParts reconstructs the full parts slice that
// handleSessionCommand expects: parts[0] is the command name,
// parts[1:] is the post-name argument list. Exposed for testing
// the dispatcher contract (see M6 in the code review).
func buildSessionParts(name string, args []string) []string {
	parts := make([]string, 0, 1+len(args))
	parts = append(parts, name)
	parts = append(parts, args...)
	return parts
}

func (s *sessionSubcommand) Handle(m *chatModel, args []string, text string) (tea.Model, tea.Cmd) {
	name := resolveSessionName(text, s.Name())
	// handleSessionCommand expects parts[0] to be the command name (e.g.
	// "/recover"), with the remaining entries being the post-name args.
	// The dispatcher hands us the post-name slice; reconstruct the
	// full parts slice so /recover <id>, /resume <id>, /tag <label>, etc.
	// receive the trailing argument.
	parts := buildSessionParts(name, args)
	return m.handleSessionCommand(name, parts, text)
}

func init() {
	subcommandRegistry.Register(&sessionSubcommand{})
}
