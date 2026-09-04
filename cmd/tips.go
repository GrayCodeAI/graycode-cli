package cmd

import (
	"encoding/json"
	"math/rand"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/GrayCodeAI/graycode-cli/internal/storage"
)

// Tip represents a single graycode usage tip.
type Tip struct {
	ID       string `json:"id"`
	Text     string `json:"text"`
	Category string `json:"category"`
}

// allTips returns the built-in tip registry.
func allTips() []Tip {
	return []Tip{
		{ID: "slash-help", Text: "Use /help to see all available commands.", Category: "basics"},
		{ID: "slash-compact", Text: "Use /compact to summarize and shrink your conversation history.", Category: "basics"},
		{ID: "slash-diff", Text: "Use /diff to review changes made during this session.", Category: "git"},
		{ID: "slash-commit", Text: "Use /commit to auto-commit changes with a generated message.", Category: "git"},
		{ID: "slash-doctor", Text: "Use /doctor to run diagnostics on your project.", Category: "project"},
		{ID: "slash-plan", Text: "Use /spec to gate Write/Edit/Bash behind a written spec before changes happen.", Category: "safety"},
		{ID: "tab-complete", Text: "Press Tab to autocomplete slash commands.", Category: "shortcuts"},
		{ID: "history-nav", Text: "Press Up/Down to navigate command history.", Category: "shortcuts"},
		{ID: "esc-cancel", Text: "Press Esc to cancel a running query.", Category: "shortcuts"},
		{ID: "ctrl-c-quit", Text: "Press Ctrl+C twice to quit graycode.", Category: "shortcuts"},
		{ID: "copy-chat", Text: "Ctrl+Shift+C or /copy copies chat; /copy input copies your draft; /mouse off enables click-drag select.", Category: "shortcuts"},
		{ID: "vim-mode", Text: "Use /vim to toggle vim-style keybindings.", Category: "editing"},
		{ID: "model-switch", Text: "Use /model <name> to switch LLM models on the fly.", Category: "config"},
		{ID: "provider-switch", Text: "Use /config provider <name> to change providers.", Category: "config"},
		{ID: "permissions", Text: "Use /autonomy allow <rule> to pre-approve tool patterns.", Category: "safety"},
		{ID: "autonomy-cycle", Text: "Press Ctrl+L to cycle autonomy tiers (Scout → Builder → Operator → Autonomous).", Category: "safety"},
		{ID: "session-resume", Text: "Use /resume <id> to pick up where you left off.", Category: "session"},
		{ID: "session-search", Text: "Use /search <query> to find across saved sessions.", Category: "session"},
		{ID: "slash-stats", Text: "Use /stats to view analytics for the past 30 days.", Category: "analytics"},
		{ID: "slash-cost", Text: "Use /cost to check token usage and API spend.", Category: "analytics"},
		{ID: "slash-review", Text: "Use /review to get a code review of current changes.", Category: "workflow"},
		{ID: "slash-init", Text: "Use /init to analyze a new project automatically.", Category: "project"},
		{ID: "add-dir", Text: "Use /add-dir <path> to add extra directories to context.", Category: "context"},
		{ID: "slash-memory", Text: "Use /memory for AGENTS.md; /harrier for graph memory; /ecosystem for eyrie/harrier/shrike status.", Category: "context"},
		{ID: "slash-rewind", Text: "Use /rewind to undo the last exchange.", Category: "session"},
		{ID: "slash-fork", Text: "Use /fork to branch off the current conversation.", Category: "session"},
		{ID: "slash-context", Text: "Use /context to see what the agent knows about your project.", Category: "context"},
		{ID: "slash-start", Text: "Use /start for guided setup: trust, mode, branch, first tasks.", Category: "basics"},
		{ID: "slash-mode-plan", Text: "Use /mode plan to research read-only, then /mode act to implement.", Category: "workflow"},
		{ID: "slash-isolation", Text: "Use /isolation workspace so shell runs under OS sandbox wrap.", Category: "safety"},
		{ID: "slash-trust", Text: "Use /trust add so project hooks and MCP can load (folder trust).", Category: "safety"},
		{ID: "slash-branch-agent", Text: "Use /branch-agent before big edits on main — creates graycode/agent-* branch.", Category: "git"},
		{ID: "tool-search-select", Text: "Use ToolSearch select:Impact (etc.) to unlock optional tools on the lazy surface.", Category: "tools"},
		{ID: "slash-auto-commit", Text: "Use /auto-commit on so Write/Edit create git commits automatically.", Category: "git"},
	}
}

// tipHistoryPath returns the path to the tip history file.
func tipHistoryPath() string {
	return filepath.Join(storage.StateDir(), "tip_history.json")
}

// tipHistory represents recently shown tip IDs with timestamps.
type tipHistory struct {
	Shown map[string]time.Time `json:"shown"`
}

func loadTipHistory() tipHistory {
	h := tipHistory{Shown: make(map[string]time.Time)}
	data, err := os.ReadFile(tipHistoryPath())
	if err != nil {
		return h
	}
	_ = json.Unmarshal(data, &h)
	if h.Shown == nil {
		h.Shown = make(map[string]time.Time)
	}
	return h
}

func saveTipHistory(h tipHistory) {
	_ = os.MkdirAll(storage.StateDir(), 0o750) // #nosec G301 -- state dir holds private user data, owner/group only
	data, _ := json.MarshalIndent(h, "", "  ")
	_ = os.WriteFile(tipHistoryPath(), data, 0o600) // #nosec G306 -- tip history is private user state
}

// recordTipShown marks a tip as recently shown.
func recordTipShown(id string) {
	h := loadTipHistory()
	h.Shown[id] = time.Now()
	saveTipHistory(h)
}

// nextTip returns a tip that hasn't been shown recently (within the last 24h),
// records it as shown, and returns the display text. If all tips have been
// shown recently, one is picked at random.
//
// The containerMode and lastCommand arguments drive context-aware filtering:
// tips relevant to the current mode or the user's most recent slash command
// are prioritized over generic ones. Pass false / "" for callers that don't
// have model state (e.g. tests).
func nextTip(containerMode bool, lastCommand string) string {
	return contextualTip(containerMode, lastCommand)
}

// contextualTip returns a tip relevant to the current context.
// containerMode tips are prioritized when in container mode; lastCommand
// matches tips whose Category aligns with the command's purpose.
func contextualTip(containerMode bool, lastCommand string) string {
	tips := allTips()
	if len(tips) == 0 {
		return ""
	}

	h := loadTipHistory()
	cooldown := 24 * time.Hour

	// Determine relevant categories based on context.
	relevantCategories := map[string]bool{}
	switch {
	case containerMode:
		relevantCategories["safety"] = true
		relevantCategories["shortcuts"] = true
	case strings.HasPrefix(lastCommand, "/commit"), strings.HasPrefix(lastCommand, "/diff"):
		relevantCategories["git"] = true
		relevantCategories["workflow"] = true
	case strings.HasPrefix(lastCommand, "/config"), strings.HasPrefix(lastCommand, "/model"):
		relevantCategories["config"] = true
		relevantCategories["basics"] = true
	case strings.HasPrefix(lastCommand, "/fork"), strings.HasPrefix(lastCommand, "/resume"):
		relevantCategories["session"] = true
		relevantCategories["context"] = true
	default:
		// No specific context — allow all categories.
		relevantCategories[""] = true
	}

	var candidates []Tip
	for _, tip := range tips {
		if last, ok := h.Shown[tip.ID]; ok && time.Since(last) <= cooldown {
			continue
		}
		// If we have a relevant category, prioritize matching tips.
		if !relevantCategories[""] && relevantCategories[tip.Category] {
			candidates = append(candidates, tip)
		}
	}

	// If no context-relevant candidates, fall back to all non-cooldown tips.
	if len(candidates) == 0 {
		for _, tip := range tips {
			if last, ok := h.Shown[tip.ID]; !ok || time.Since(last) > cooldown {
				candidates = append(candidates, tip)
			}
		}
	}

	// If still nothing, allow any tip.
	if len(candidates) == 0 {
		candidates = tips
	}

	chosen := candidates[rand.Intn(len(candidates))] // #nosec G404 -- non-cryptographic use (random tip selection)
	recordTipShown(chosen.ID)
	return chosen.Text
}
