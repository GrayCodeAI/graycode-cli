package cmd

import (
	"encoding/json"
	"math/rand"
	"os"
	"path/filepath"
	"time"
)

// Tip represents a single hawk usage tip.
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
		{ID: "slash-plan", Text: "Use /plan to enter read-only mode for safe exploration.", Category: "safety"},
		{ID: "tab-complete", Text: "Press Tab to autocomplete slash commands.", Category: "shortcuts"},
		{ID: "history-nav", Text: "Press Up/Down to navigate command history.", Category: "shortcuts"},
		{ID: "esc-cancel", Text: "Press Esc to cancel a running query.", Category: "shortcuts"},
		{ID: "ctrl-c-quit", Text: "Press Ctrl+C twice to quit hawk.", Category: "shortcuts"},
		{ID: "vim-mode", Text: "Use /vim to toggle vim-style keybindings.", Category: "editing"},
		{ID: "model-switch", Text: "Use /model <name> to switch LLM models on the fly.", Category: "config"},
		{ID: "provider-switch", Text: "Use /config provider <name> to change providers.", Category: "config"},
		{ID: "permissions", Text: "Use /permissions allow <rule> to pre-approve tool patterns.", Category: "safety"},
		{ID: "session-resume", Text: "Use /resume <id> to pick up where you left off.", Category: "session"},
		{ID: "session-search", Text: "Use /search <query> to find across saved sessions.", Category: "session"},
		{ID: "slash-stats", Text: "Use /stats to view analytics for the past 30 days.", Category: "analytics"},
		{ID: "slash-cost", Text: "Use /cost to check token usage and API spend.", Category: "analytics"},
		{ID: "slash-review", Text: "Use /review to get a code review of current changes.", Category: "workflow"},
		{ID: "slash-init", Text: "Use /init to analyze a new project automatically.", Category: "project"},
		{ID: "add-dir", Text: "Use /add-dir <path> to add extra directories to context.", Category: "context"},
		{ID: "slash-memory", Text: "Use /memory to view loaded AGENTS.md project instructions.", Category: "context"},
		{ID: "slash-rewind", Text: "Use /rewind to undo the last exchange.", Category: "session"},
		{ID: "slash-fork", Text: "Use /fork to branch off the current conversation.", Category: "session"},
		{ID: "slash-context", Text: "Use /context to see what the agent knows about your project.", Category: "context"},
	}
}

// tipHistoryPath returns the path to the tip history file.
func tipHistoryPath() string {
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".hawk", "tip_history.json")
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
	home, _ := os.UserHomeDir()
	_ = os.MkdirAll(filepath.Join(home, ".hawk"), 0o755)
	data, _ := json.MarshalIndent(h, "", "  ")
	_ = os.WriteFile(tipHistoryPath(), data, 0o644)
}

// recordTipShown marks a tip as recently shown.
func recordTipShown(id string) {
	h := loadTipHistory()
	h.Shown[id] = time.Now()
	saveTipHistory(h)
}

// nextTip returns a tip that hasn't been shown recently (within the last 24h).
// If all tips have been shown recently, one is picked at random.
func nextTip() string {
	tips := allTips()
	if len(tips) == 0 {
		return ""
	}

	h := loadTipHistory()
	cooldown := 24 * time.Hour

	var candidates []Tip
	for _, tip := range tips {
		if last, ok := h.Shown[tip.ID]; !ok || time.Since(last) > cooldown {
			candidates = append(candidates, tip)
		}
	}

	if len(candidates) == 0 {
		candidates = tips
	}

	chosen := candidates[rand.Intn(len(candidates))]
	return chosen.Text
}
