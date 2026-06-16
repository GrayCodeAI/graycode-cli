package cmd

import (
	"sort"
	"sync"

	tea "github.com/charmbracelet/bubbletea"
)

// ChatSubcommand is a single slash-command handler. Implementations
// live in their own file (cmd/chat_subcommand_<name>.go) and are
// registered via SubcommandRegistry.Register. The interface is the
// foundation for decomposing chat_commands.go (1745 lines as of
// 2026-06) into one file per command.
//
// Migration path from the existing handleCommand switch statement:
//   1. Create a new file cmd/chat_subcommand_<name>.go with a
//      type implementing ChatSubcommand.
//   2. Implement the existing handler logic in Handle().
//   3. Register the subcommand in init() or in a package-level
//      subcommand registry.
//   4. Replace the case in handleCommand with a lookup against
//      the registry.
//
// The interface lives in this file (not chat_commands.go) so that
// subcommand implementations can be defined in any file without
// modifying chat_commands.go.
type ChatSubcommand interface {
	// Name is the canonical command name WITHOUT the leading slash.
	// e.g. "help" for "/help". Names are lowercase.
	Name() string

	// Aliases are alternative names that dispatch to the same
	// implementation. e.g. "exit" and "quit" both map to the
	// session command. Empty for no aliases.
	Aliases() []string

	// Description is a one-line help string shown in /help output.
	// Keep under 60 characters to fit the help column.
	Description() string

	// Usage is shown when the user provides invalid arguments.
	// Empty for argument-free commands.
	Usage() string

	// Handle dispatches the command. The chat model is the
	// receiver for state access. args is the parsed argument list
	// (without the command name); text is the original raw text
	// (including the command name) for cases that need to
	// re-parse (e.g., quoted strings).
	//
	// The returned tea.Model is the (possibly new) model; tea.Cmd
	// is an optional side-effect to enqueue (tea.Quit for /quit).
	Handle(m *chatModel, args []string, text string) (tea.Model, tea.Cmd)
}

// SubcommandRegistry is the canonical index mapping slash command
// names (and their aliases) to ChatSubcommand implementations. It's
// safe for concurrent use.
type SubcommandRegistry struct {
	mu      sync.RWMutex
	primary map[string]ChatSubcommand
	aliasOf map[string]string // alias -> primary name
}

// NewSubcommandRegistry creates an empty registry. Subcommands are
// registered via Register() (typically from per-file init() funcs
// or from a single aggregate init that imports each subcommand).
func NewSubcommandRegistry() *SubcommandRegistry {
	return &SubcommandRegistry{
		primary: make(map[string]ChatSubcommand),
		aliasOf: make(map[string]string),
	}
}

// Register adds a subcommand to the registry. The primary name and
// all aliases are indexed. If a name is already registered, this
// is a no-op (the existing entry is kept) — duplicate registration
// is treated as a configuration error but doesn't panic, so test
// ordering and re-init don't blow up the binary.
func (r *SubcommandRegistry) Register(cmd ChatSubcommand) {
	if cmd == nil {
		return
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	name := cmd.Name()
	if _, exists := r.primary[name]; exists {
		return // duplicate
	}
	r.primary[name] = cmd
	for _, alias := range cmd.Aliases() {
		r.aliasOf[alias] = name
	}
}

// Lookup returns the subcommand for a slash name (without the
// leading slash). The second return is false if neither the name
// nor any of its aliases is registered.
func (r *SubcommandRegistry) Lookup(name string) (ChatSubcommand, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	if cmd, ok := r.primary[name]; ok {
		return cmd, true
	}
	if primary, ok := r.aliasOf[name]; ok {
		if cmd, ok := r.primary[primary]; ok {
			return cmd, true
		}
	}
	return nil, false
}

// Names returns all primary command names in sorted order. Used by
// /help and /commands to enumerate available subcommands.
func (r *SubcommandRegistry) Names() []string {
	r.mu.RLock()
	defer r.mu.RUnlock()
	names := make([]string, 0, len(r.primary))
	for n := range r.primary {
		names = append(names, n)
	}
	sort.Strings(names)
	return names
}

// All returns all registered subcommands (deduplicated by primary
// name). Used by /help to render the full help table.
func (r *SubcommandRegistry) All() []ChatSubcommand {
	r.mu.RLock()
	defer r.mu.RUnlock()
	out := make([]ChatSubcommand, 0, len(r.primary))
	for _, cmd := range r.primary {
		out = append(out, cmd)
	}
	// Sort by name for deterministic help output.
	sort.Slice(out, func(i, j int) bool {
		return out[i].Name() < out[j].Name()
	})
	return out
}

// Size returns the number of primary subcommands (excluding
// aliases). Used by tests and by /commands to show a count.
func (r *SubcommandRegistry) Size() int {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return len(r.primary)
}
