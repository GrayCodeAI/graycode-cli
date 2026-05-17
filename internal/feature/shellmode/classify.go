package shellmode

import (
	"os/exec"
	"strings"
	"sync"
)

// Classification is the result of input analysis.
type Classification int

const (
	ClassShell   Classification = iota // execute in shell
	ClassAgent                         // route to AI agent
	ClassNeutral                       // undetermined
)

// agentWords are conversational words that always route to agent.
var agentWords = map[string]bool{
	// affirmations
	"yes": true, "yeah": true, "yep": true, "sure": true, "ok": true, "okay": true,
	"absolutely": true, "definitely": true, "certainly": true, "correct": true, "exactly": true,
	"perfect": true, "agreed": true, "lgtm": true,
	// negations
	"no": true, "nope": true, "nah": true, "never": true, "wrong": true,
	// gratitude
	"thanks": true, "thank": true, "thx": true, "ty": true, "cheers": true,
	// reactions
	"great": true, "good": true, "nice": true, "cool": true, "awesome": true,
	"amazing": true, "wonderful": true, "brilliant": true, "excellent": true,
	// greetings
	"hey": true, "hi": true, "hello": true, "bye": true,
	// conversational
	"please": true, "sorry": true, "hmm": true, "well": true,
	// action/intent
	"explain": true, "elaborate": true, "clarify": true, "summarize": true,
	"describe": true, "show": true, "tell": true,
	// question words
	"why": true, "how": true, "what": true, "when": true, "where": true, "who": true, "which": true,
	// programming verbs (not real commands)
	"refactor": true, "optimize": true, "scaffold": true,
}

// shellReservedWords pass `command -v` but are never standalone commands.
var shellReservedWords = map[string]bool{
	"do": true, "done": true, "then": true, "else": true, "elif": true,
	"fi": true, "esac": true, "in": true, "select": true, "function": true,
}

// ClassifyInput determines whether input should go to shell or AI agent.
// This is the single source of truth for routing decisions in auto mode.
func ClassifyInput(input string) Classification {
	trimmed := strings.TrimSpace(input)
	if trimmed == "" {
		return ClassNeutral
	}

	words := strings.Fields(trimmed)
	firstWord := strings.ToLower(words[0])

	// Layer 0: Agent words — always route to agent
	if agentWords[firstWord] {
		return ClassAgent
	}

	// Layer 1: Shell reserved words — route to agent
	if shellReservedWords[firstWord] {
		return ClassAgent
	}

	// Layer 2: Check if first word is a valid command
	isCmd := isValidCommand(firstWord)

	if isCmd {
		// Single valid command word → shell
		return ClassShell
	}

	// Not a command
	if len(words) == 1 {
		// Single unknown word → likely a typo or conversational; route to agent
		return ClassAgent
	}

	// Multiple words, first is not a command → agent (natural language)
	return ClassAgent
}

// cmdCache caches exec.LookPath results to avoid repeated PATH scans.
var (
	cmdCacheMu sync.RWMutex
	cmdCache   = make(map[string]bool)
)

// isValidCommand checks if a word exists as a command in PATH (cached).
func isValidCommand(word string) bool {
	cmdCacheMu.RLock()
	if v, ok := cmdCache[word]; ok {
		cmdCacheMu.RUnlock()
		return v
	}
	cmdCacheMu.RUnlock()
	_, err := exec.LookPath(word)
	result := err == nil
	cmdCacheMu.Lock()
	cmdCache[word] = result
	cmdCacheMu.Unlock()
	return result
}
