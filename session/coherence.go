package session

import (
	"fmt"
	"regexp"
	"strings"
	"sync"
)

type ConversationalAct string

const (
	ActQuestion  ConversationalAct = "question"
	ActInstruct  ConversationalAct = "instruct"
	ActCorrect   ConversationalAct = "correct"
	ActElaborate ConversationalAct = "elaborate"
	ActConfirm   ConversationalAct = "confirm"
	ActPivot     ConversationalAct = "pivot"
	ActExplore   ConversationalAct = "explore"
	ActUnknown   ConversationalAct = "unknown"
)

type SessionThread struct {
	ID                string   `json:"id"`
	Topic             string   `json:"topic"`
	Status            string   `json:"status"`
	StartedAtTurn     int      `json:"started_at_turn"`
	LastMentionedTurn int      `json:"last_mentioned_turn"`
	Decisions         []string `json:"decisions"`
	OpenQuestions     []string `json:"open_questions"`
}

type Pivot struct {
	Turn int    `json:"turn"`
	From string `json:"from"`
	To   string `json:"to"`
}

type CoherenceState struct {
	Threads         []*SessionThread  `json:"threads"`
	Pivots          []Pivot           `json:"pivots"`
	LastUpdatedTurn int               `json:"last_updated_turn"`
	CurrentAct      ConversationalAct `json:"current_act"`
	IntentSummary   string            `json:"intent_summary"`
}

type CoherenceTracker struct {
	mu             sync.RWMutex
	state          CoherenceState
	updateInterval int
	maxThreads     int
}

func NewCoherenceTracker(updateInterval, maxThreads int) *CoherenceTracker {
	if updateInterval <= 0 {
		updateInterval = 10
	}
	if maxThreads <= 0 {
		maxThreads = 5
	}
	return &CoherenceTracker{
		state:          CoherenceState{Threads: make([]*SessionThread, 0), Pivots: make([]Pivot, 0)},
		updateInterval: updateInterval,
		maxThreads:     maxThreads,
	}
}

func (ct *CoherenceTracker) ClassifyAct(message string) ConversationalAct {
	lower := strings.TrimSpace(strings.ToLower(message))

	if matchesAny(lower, `^(?:yes|yeah|yep|correct|right|perfect|exactly|looks good|lgtm)`) {
		return ActConfirm
	}
	if matchesAny(lower, `^(?:no[,.]?\s|nope|that's wrong|actually|wait|not what i|i meant)`) {
		return ActCorrect
	}
	if matchesAny(lower, `^(?:forget that|scratch that|instead|wait.*let's|never ?mind)`) {
		return ActPivot
	}
	if matchesAny(lower, `^(?:what if|could we|i'm wondering|hypothetically)`) {
		return ActExplore
	}
	if matchesAny(lower, `^(?:and also|additionally|specifically|what i mean|to clarify)`) {
		return ActElaborate
	}
	if strings.HasSuffix(lower, "?") || matchesAny(lower, `^(?:how|what|why|when|where|which|can you|do you|is there|does)`) {
		return ActQuestion
	}
	if matchesAny(lower, `^(?:build|create|add|change|update|fix|remove|delete|implement|write|make|set up|deploy|run|test)`) {
		return ActInstruct
	}

	return ActUnknown
}

func (ct *CoherenceTracker) UpdateIntent(message string, turn int) {
	ct.mu.Lock()
	defer ct.mu.Unlock()

	ct.state.CurrentAct = ct.ClassifyAct(message)
}

func (ct *CoherenceTracker) RecordPivot(turn int, from, to string) {
	ct.mu.Lock()
	defer ct.mu.Unlock()

	ct.state.Pivots = append(ct.state.Pivots, Pivot{Turn: turn, From: from, To: to})
	if len(ct.state.Pivots) > 5 {
		ct.state.Pivots = ct.state.Pivots[len(ct.state.Pivots)-5:]
	}
}

func (ct *CoherenceTracker) FormatForPrompt() string {
	ct.mu.RLock()
	defer ct.mu.RUnlock()

	var active []*SessionThread
	for _, t := range ct.state.Threads {
		if t.Status == "active" {
			active = append(active, t)
		}
	}

	if len(active) == 0 && ct.state.IntentSummary == "" {
		return ""
	}

	lines := []string{"Session context:"}
	for _, t := range active {
		lines = append(lines, fmt.Sprintf("- Topic: %s", t.Topic))
		if len(t.Decisions) > 0 {
			lines = append(lines, fmt.Sprintf("  Decided: %s", strings.Join(lastN(t.Decisions, 2), "; ")))
		}
		if len(t.OpenQuestions) > 0 {
			lines = append(lines, fmt.Sprintf("  Open: %s", t.OpenQuestions[0]))
		}
	}

	if len(ct.state.Pivots) > 0 {
		last := ct.state.Pivots[len(ct.state.Pivots)-1]
		lines = append(lines, fmt.Sprintf("- User pivoted to: %s", last.To))
	}

	return strings.Join(lines, "\n")
}

func (ct *CoherenceTracker) GetState() CoherenceState {
	ct.mu.RLock()
	defer ct.mu.RUnlock()
	return ct.state
}

func matchesAny(text, pattern string) bool {
	re, err := regexp.Compile(pattern)
	if err != nil {
		return false
	}
	return re.MatchString(text)
}

func lastN(s []string, n int) []string {
	if len(s) <= n {
		return s
	}
	return s[len(s)-n:]
}
