package engine

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

// AdaptivePrompt adjusts system prompt sections based on user corrections.
// When the user says "don't do X" or "always do Y", the adaptive prompt
// system records it and injects it into future sessions.
type AdaptivePrompt struct {
	mu          sync.Mutex
	adjustments []PromptAdjustment
	path        string
}

// PromptAdjustment is a user-derived prompt modification.
type PromptAdjustment struct {
	Rule       string    `json:"rule"`        // "always use tabs" or "never add comments"
	Source     string    `json:"source"`      // user message that triggered this
	Polarity   string    `json:"polarity"`    // "do" or "dont"
	Confidence float64   `json:"confidence"`  // increases with reinforcement
	Active     bool      `json:"active"`
	CreatedAt  time.Time `json:"created_at"`
	LastUsed   time.Time `json:"last_used"`
}

// NewAdaptivePrompt creates an adaptive prompt backed by ~/.hawk/adaptive_prompt.json.
func NewAdaptivePrompt() *AdaptivePrompt {
	home, _ := os.UserHomeDir()
	ap := &AdaptivePrompt{
		path: filepath.Join(home, ".hawk", "adaptive_prompt.json"),
	}
	ap.load()
	return ap
}

// LearnFromFeedback extracts prompt adjustments from user corrections.
func (ap *AdaptivePrompt) LearnFromFeedback(userMessage string) {
	lower := strings.ToLower(userMessage)

	var rule, polarity string

	// Detect "don't" patterns
	dontPrefixes := []string{"don't ", "dont ", "do not ", "never ", "stop ", "avoid "}
	for _, prefix := range dontPrefixes {
		if strings.Contains(lower, prefix) {
			idx := strings.Index(lower, prefix)
			rest := strings.TrimSpace(userMessage[idx+len(prefix):])
			if end := strings.IndexAny(rest, ".!?\n"); end > 0 {
				rest = rest[:end]
			}
			if len(rest) > 5 && len(rest) < 200 {
				rule = rest
				polarity = "dont"
			}
			break
		}
	}

	// Detect "always" patterns
	if rule == "" {
		doPrefixes := []string{"always ", "make sure to ", "remember to ", "from now on "}
		for _, prefix := range doPrefixes {
			if strings.Contains(lower, prefix) {
				idx := strings.Index(lower, prefix)
				rest := strings.TrimSpace(userMessage[idx+len(prefix):])
				if end := strings.IndexAny(rest, ".!?\n"); end > 0 {
					rest = rest[:end]
				}
				if len(rest) > 5 && len(rest) < 200 {
					rule = rest
					polarity = "do"
				}
				break
			}
		}
	}

	if rule == "" {
		return
	}

	ap.mu.Lock()
	defer ap.mu.Unlock()

	// Check for existing similar rule
	for i, adj := range ap.adjustments {
		if strings.Contains(strings.ToLower(adj.Rule), strings.ToLower(rule)) ||
			strings.Contains(strings.ToLower(rule), strings.ToLower(adj.Rule)) {
			ap.adjustments[i].Confidence += 0.2
			if ap.adjustments[i].Confidence > 1.0 {
				ap.adjustments[i].Confidence = 1.0
			}
			ap.adjustments[i].LastUsed = time.Now()
			ap.save()
			return
		}
	}

	ap.adjustments = append(ap.adjustments, PromptAdjustment{
		Rule:       rule,
		Source:     userMessage,
		Polarity:   polarity,
		Confidence: 0.6,
		Active:     true,
		CreatedAt:  time.Now(),
		LastUsed:   time.Now(),
	})
	ap.save()
}

// FormatForPrompt returns active adjustments as system prompt rules.
func (ap *AdaptivePrompt) FormatForPrompt() string {
	ap.mu.Lock()
	defer ap.mu.Unlock()

	var dos, donts []string
	for _, adj := range ap.adjustments {
		if !adj.Active || adj.Confidence < 0.5 {
			continue
		}
		if adj.Polarity == "do" {
			dos = append(dos, adj.Rule)
		} else {
			donts = append(donts, adj.Rule)
		}
	}

	if len(dos) == 0 && len(donts) == 0 {
		return ""
	}

	var b strings.Builder
	b.WriteString("## User Preferences (learned from feedback)\n")
	for _, d := range dos {
		b.WriteString("- Always: " + d + "\n")
	}
	for _, d := range donts {
		b.WriteString("- Never: " + d + "\n")
	}
	return b.String()
}

// Count returns the number of active adjustments.
func (ap *AdaptivePrompt) Count() int {
	ap.mu.Lock()
	defer ap.mu.Unlock()
	count := 0
	for _, adj := range ap.adjustments {
		if adj.Active && adj.Confidence >= 0.5 {
			count++
		}
	}
	return count
}

func (ap *AdaptivePrompt) load() {
	data, err := os.ReadFile(ap.path)
	if err != nil {
		return
	}
	json.Unmarshal(data, &ap.adjustments)
}

func (ap *AdaptivePrompt) save() {
	dir := filepath.Dir(ap.path)
	os.MkdirAll(dir, 0o755)
	data, _ := json.Marshal(ap.adjustments)
	os.WriteFile(ap.path, data, 0o644)
}
