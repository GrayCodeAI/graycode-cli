package engine

import (
	"fmt"
	"strconv"
	"strings"
	"sync"
)

// MagicCommand represents a REPL magic command (prefixed with %).
type MagicCommand struct {
	Name        string
	Description string
	Handler     func(session *Session, args string) string
}

// MagicRegistry holds registered magic commands.
type MagicRegistry struct {
	commands map[string]*MagicCommand
}

// NewMagicRegistry creates a registry with all built-in magic commands.
func NewMagicRegistry() *MagicRegistry {
	r := &MagicRegistry{commands: make(map[string]*MagicCommand)}
	r.registerBuiltin()
	return r
}

// Get returns the magic command for the given name, or nil if not found.
func (r *MagicRegistry) Get(name string) *MagicCommand {
	return r.commands[name]
}

// List returns all registered magic commands sorted by name.
func (r *MagicRegistry) List() []*MagicCommand {
	cmds := make([]*MagicCommand, 0, len(r.commands))
	for _, c := range r.commands {
		cmds = append(cmds, c)
	}
	return cmds
}

// Execute looks up the magic command and runs it. Returns an error message if
// the command is not found.
func (r *MagicRegistry) Execute(name string, session *Session, args string) string {
	cmd := r.Get(name)
	if cmd == nil {
		return fmt.Sprintf("Unknown magic command: %%%s\nType %%help to see available commands.", name)
	}
	return cmd.Handler(session, args)
}

// Register adds a custom magic command to the registry.
func (r *MagicRegistry) Register(cmd *MagicCommand) {
	r.commands[cmd.Name] = cmd
}

// IsMagic returns true if the input starts with %.
func IsMagic(input string) bool {
	return strings.HasPrefix(strings.TrimSpace(input), "%")
}

// ParseMagic splits "%name args..." into (name, args).
func ParseMagic(input string) (name, args string) {
	input = strings.TrimSpace(input)
	input = strings.TrimPrefix(input, "%")
	parts := strings.SplitN(input, " ", 2)
	name = strings.TrimSpace(parts[0])
	if len(parts) > 1 {
		args = strings.TrimSpace(parts[1])
	}
	return
}

// DefaultMagicRegistry is the package-level registry populated lazily.
var (
	defaultMagicOnce     sync.Once
	defaultMagicRegistry *MagicRegistry
)

// DefaultMagicRegistry returns the package-level registry, initializing it on
// first call. This avoids an init-time cycle with magicHelp referencing the
// registry before it is fully constructed.
func DefaultMagicRegistry() *MagicRegistry {
	defaultMagicOnce.Do(func() {
		defaultMagicRegistry = NewMagicRegistry()
	})
	return defaultMagicRegistry
}

func (r *MagicRegistry) registerBuiltin() {
	r.Register(&MagicCommand{
		Name:        "reset",
		Description: "Clear conversation history and start fresh",
		Handler:     magicReset,
	})
	r.Register(&MagicCommand{
		Name:        "undo",
		Description: "Remove last N messages (default 1)",
		Handler:     magicUndo,
	})
	r.Register(&MagicCommand{
		Name:        "tokens",
		Description: "Show current token usage breakdown",
		Handler:     magicTokens,
	})
	r.Register(&MagicCommand{
		Name:        "verbose",
		Description: "Toggle verbose mode (show tool calls, timing, token counts)",
		Handler:     magicVerbose,
	})
	r.Register(&MagicCommand{
		Name:        "model",
		Description: "Show current model or switch model",
		Handler:     magicModel,
	})
	r.Register(&MagicCommand{
		Name:        "cost",
		Description: "Show session cost breakdown",
		Handler:     magicCost,
	})
	r.Register(&MagicCommand{
		Name:        "compact",
		Description: "Force conversation compaction",
		Handler:     magicCompact,
	})
	r.Register(&MagicCommand{
		Name:        "help",
		Description: "Show all available magic commands",
		Handler:     magicHelp,
	})
}

// --- Built-in magic command handlers ---

func magicReset(session *Session, _ string) string {
	session.mu.Lock()
	count := len(session.messages)
	session.messages = nil
	session.mu.Unlock()
	return fmt.Sprintf("Conversation reset. Cleared %d messages.", count)
}

func magicUndo(session *Session, args string) string {
	n := 1
	if args != "" {
		parsed, err := strconv.Atoi(strings.TrimSpace(args))
		if err != nil || parsed <= 0 {
			return "Usage: %undo [N] — N must be a positive integer"
		}
		n = parsed
	}

	session.mu.Lock()
	defer session.mu.Unlock()

	total := len(session.messages)
	if total == 0 {
		return "No messages to undo."
	}
	if n > total {
		n = total
	}
	session.messages = session.messages[:total-n]
	return fmt.Sprintf("Removed last %d message(s). %d remaining.", n, len(session.messages))
}

func magicTokens(session *Session, _ string) string {
	session.mu.RLock()
	defer session.mu.RUnlock()

	totalTokens := 0
	for _, msg := range session.messages {
		totalTokens += len(msg.Content) / 4 // rough estimate: ~4 chars per token
	}

	input := session.Cost.PromptTokens
	output := session.Cost.CompletionTokens
	cacheRead := session.Cost.CacheReadTokens
	cacheWrite := session.Cost.CacheWriteTokens
	total := input + output
	budget := session.MaxBudgetUSD

	var sb strings.Builder
	sb.WriteString("Token Usage\n")
	sb.WriteString("───────────\n")
	fmt.Fprintf(&sb, "  Input:       %d\n", input)
	fmt.Fprintf(&sb, "  Output:      %d\n", output)
	if cacheRead > 0 || cacheWrite > 0 {
		fmt.Fprintf(&sb, "  Cache read:  %d\n", cacheRead)
		fmt.Fprintf(&sb, "  Cache write: %d\n", cacheWrite)
	}
	fmt.Fprintf(&sb, "  Total:       %d\n", total)
	fmt.Fprintf(&sb, "  Messages:    %d\n", len(session.messages))
	fmt.Fprintf(&sb, "  Est. context: ~%d tokens\n", totalTokens)
	if budget > 0 {
		spent := session.Cost.TotalCostUSD
		remaining := budget - spent
		fmt.Fprintf(&sb, "  Budget:      $%.4f remaining of $%.4f\n", remaining, budget)
	}
	return sb.String()
}

func magicVerbose(session *Session, _ string) string {
	session.mu.Lock()
	session.Verbose = !session.Verbose
	state := session.Verbose
	session.mu.Unlock()

	if state {
		return "Verbose mode ON — tool calls, timing, and token counts will be shown."
	}
	return "Verbose mode OFF."
}

func magicModel(session *Session, args string) string {
	if args == "" {
		return fmt.Sprintf("Current model: %s (provider: %s)", session.Model(), session.Provider())
	}
	newModel := strings.TrimSpace(args)
	session.SetModel(newModel)
	return fmt.Sprintf("Model switched to: %s", newModel)
}

func magicCost(session *Session, _ string) string {
	session.mu.RLock()
	defer session.mu.RUnlock()

	input := session.Cost.PromptTokens
	output := session.Cost.CompletionTokens
	cacheRead := session.Cost.CacheReadTokens
	cacheWrite := session.Cost.CacheWriteTokens
	totalCost := session.Cost.TotalCostUSD
	model := session.Cost.Model

	var sb strings.Builder
	sb.WriteString("Cost Breakdown\n")
	sb.WriteString("──────────────\n")
	fmt.Fprintf(&sb, "  Model:         %s\n", model)
	fmt.Fprintf(&sb, "  Input tokens:  %d\n", input)
	fmt.Fprintf(&sb, "  Output tokens: %d\n", output)
	if cacheRead > 0 || cacheWrite > 0 {
		fmt.Fprintf(&sb, "  Cache read:    %d\n", cacheRead)
		fmt.Fprintf(&sb, "  Cache write:   %d\n", cacheWrite)
	}
	fmt.Fprintf(&sb, "  Total cost:    $%.4f\n", totalCost)

	if session.MaxBudgetUSD > 0 {
		remaining := session.MaxBudgetUSD - totalCost
		pct := (totalCost / session.MaxBudgetUSD) * 100
		fmt.Fprintf(&sb, "  Budget used:   %.1f%%\n", pct)
		fmt.Fprintf(&sb, "  Remaining:     $%.4f of $%.4f\n", remaining, session.MaxBudgetUSD)
	}
	return sb.String()
}

func magicCompact(session *Session, _ string) string {
	before := session.MessageCount()
	session.SmartCompact()
	after := session.MessageCount()
	return fmt.Sprintf("Compacted: %d → %d messages.", before, after)
}

func magicHelp(_ *Session, _ string) string {
	cmds := DefaultMagicRegistry().List()
	var sb strings.Builder
	sb.WriteString("Magic Commands\n")
	sb.WriteString("──────────────\n")
	for _, cmd := range cmds {
		fmt.Fprintf(&sb, "  %%%-10s  %s\n", cmd.Name, cmd.Description)
	}
	sb.WriteString("\nUsage: %<command> [args]\n")
	return sb.String()
}
