// Package conversationarc tracks a session's durable "arc": goals, decisions,
// milestones, and a current phase, persisted to a sidecar JSON file so it can
// be summarized and injected into later turns. It is the Go port of
// OpenClaude's conversation arc (.arc.json) memory.
//
// The phase is advanced deterministically from message keywords
// (init→exploring→implementing→reviewing→completed) and goals/decisions are
// extracted from user text by lightweight heuristics. The Summary() string is
// byte-stable: volatile per-request timestamps are stripped so an unchanged
// arc does not rewrite the model-visible summary every turn.
package conversationarc

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// Phase is the conversation's current lifecycle stage.
type Phase string

const (
	PhaseInit         Phase = "init"
	PhaseExploring    Phase = "exploring"
	PhaseImplementing Phase = "implementing"
	PhaseReviewing    Phase = "reviewing"
	PhaseCompleted    Phase = "completed"
)

// GoalStatus tracks a goal's lifecycle.
type GoalStatus string

const (
	GoalPending   GoalStatus = "pending"
	GoalActive    GoalStatus = "active"
	GoalCompleted GoalStatus = "completed"
	GoalAbandoned GoalStatus = "abandoned"
)

// Goal is a tracked objective.
type Goal struct {
	ID          string     `json:"id"`
	Description string     `json:"description"`
	Status      GoalStatus `json:"status"`
	CreatedAt   int64      `json:"createdAt"`
	CompletedAt *int64     `json:"completedAt,omitempty"`
}

// Decision is a recorded decision with optional rationale.
type Decision struct {
	ID          string `json:"id"`
	Description string `json:"description"`
	Rationale   string `json:"rationale,omitempty"`
	Timestamp   int64  `json:"timestamp"`
}

// Milestone is a recorded achievement.
type Milestone struct {
	ID          string `json:"id"`
	Description string `json:"description"`
	AchievedAt  int64  `json:"achievedAt"`
}

// Arc is the durable conversation summary.
type Arc struct {
	ID           string      `json:"id"`
	Goals        []Goal      `json:"goals"`
	Decisions    []Decision  `json:"decisions"`
	Milestones   []Milestone `json:"milestones"`
	CurrentPhase Phase       `json:"currentPhase"`
	StartTime    int64       `json:"startTime"`
	LastUpdate   int64       `json:"lastUpdateTime"`
}

const (
	defaultCap       = 50
	arcFileName      = ".arc.json"
	byteStableMarker = "<ignored>"
)

// phaseOrder is the monotonic phase ladder; a detected phase only advances.
var phaseOrder = []Phase{PhaseInit, PhaseExploring, PhaseImplementing, PhaseReviewing, PhaseCompleted}

// phaseKeywords drive deterministic phase detection from a message.
var phaseKeywords = map[Phase][]string{
	PhaseInit:         {"start", "begin", "help", "please"},
	PhaseExploring:    {"check", "find", "look", "what", "how", "where", "show"},
	PhaseImplementing: {"write", "create", "add", "fix", "update", "modify", "implement"},
	PhaseReviewing:    {"test", "review", "verify", "ensure"},
	PhaseCompleted:    {"done", "complete", "finished", "ready", "good"},
}

// New returns an empty arc with a fresh id and the init phase.
func New() *Arc {
	now := time.Now().UnixMilli()
	return &Arc{
		ID:           "arc",
		Goals:        []Goal{},
		Decisions:    []Decision{},
		Milestones:   []Milestone{},
		CurrentPhase: PhaseInit,
		StartTime:    now,
		LastUpdate:   now,
	}
}

// DetectPhase returns the highest phase whose keywords appear in text, or the
// current phase if none do. Callers may then advance via AdvancePhase.
func DetectPhase(text string) Phase {
	lower := strings.ToLower(text)
	best := -1
	for _, ph := range phaseOrder {
		for _, kw := range phaseKeywords[ph] {
			if strings.Contains(lower, kw) {
				if idx := indexOfPhase(ph); idx > best {
					best = idx
				}
				break
			}
		}
	}
	if best < 0 {
		return ""
	}
	return phaseOrder[best]
}

// AdvancePhase moves the arc to detected if it is later on the phase ladder.
// Returns true when the phase changed.
func (a *Arc) AdvancePhase(detected Phase) bool {
	if detected == "" {
		return false
	}
	cur := indexOfPhase(a.CurrentPhase)
	next := indexOfPhase(detected)
	if next > cur {
		a.CurrentPhase = detected
		a.touch()
		return true
	}
	return false
}

// AddGoal appends a pending goal (capped) and, if the arc is still in init,
// advances it to exploring.
func (a *Arc) AddGoal(description string) Goal {
	g := Goal{
		ID:          fmt.Sprintf("goal_%d", len(a.Goals)+1),
		Description: description,
		Status:      GoalPending,
		CreatedAt:   time.Now().UnixMilli(),
	}
	a.Goals = append(a.Goals, g)
	a.Goals = capGoals(a.Goals)
	if a.CurrentPhase == PhaseInit {
		a.CurrentPhase = PhaseExploring
	}
	a.touch()
	return g
}

// UpdateGoalStatus sets a goal's status by id, stamping completion time.
func (a *Arc) UpdateGoalStatus(id string, status GoalStatus) bool {
	for i := range a.Goals {
		if a.Goals[i].ID == id {
			a.Goals[i].Status = status
			if status == GoalCompleted {
				now := time.Now().UnixMilli()
				a.Goals[i].CompletedAt = &now
			}
			a.touch()
			return true
		}
	}
	return false
}

// AddDecision appends a decision (capped).
func (a *Arc) AddDecision(description, rationale string) Decision {
	d := Decision{
		ID:          fmt.Sprintf("decision_%d", len(a.Decisions)+1),
		Description: description,
		Rationale:   rationale,
		Timestamp:   time.Now().UnixMilli(),
	}
	a.Decisions = append(a.Decisions, d)
	a.Decisions = capDecisions(a.Decisions)
	a.touch()
	return d
}

// AddMilestone appends a milestone (capped).
func (a *Arc) AddMilestone(description string) Milestone {
	m := Milestone{
		ID:          fmt.Sprintf("milestone_%d", len(a.Milestones)+1),
		Description: description,
		AchievedAt:  time.Now().UnixMilli(),
	}
	a.Milestones = append(a.Milestones, m)
	a.Milestones = capMilestones(a.Milestones)
	a.touch()
	return m
}

// IsEmpty reports whether the arc has no tracked content worth summarizing
// (no goals/decisions/milestones and still in the init phase). Callers use it
// to skip injecting an empty summary.
func (a *Arc) IsEmpty() bool {
	return len(a.Goals) == 0 && len(a.Decisions) == 0 && len(a.Milestones) == 0 && a.CurrentPhase == PhaseInit
}

// Summary renders a model-visible, byte-stable arc summary. The volatile
// timestamp line is normalized so an unchanged arc produces identical output
// across turns (no prompt-cache churn).
func (a *Arc) Summary() string {
	var b strings.Builder
	fmt.Fprintf(&b, "conversation phase: %s\n", a.CurrentPhase)
	if len(a.Goals) > 0 {
		b.WriteString("goals:\n")
		for _, g := range a.Goals {
			fmt.Fprintf(&b, "  - [%s] %s\n", g.Status, g.Description)
		}
	}
	if len(a.Decisions) > 0 {
		b.WriteString("decisions:\n")
		for _, d := range a.Decisions {
			line := fmt.Sprintf("  - %s", d.Description)
			if d.Rationale != "" {
				line += fmt.Sprintf(" (%s)", d.Rationale)
			}
			b.WriteString(line)
			b.WriteByte('\n')
		}
	}
	if len(a.Milestones) > 0 {
		b.WriteString("milestones:\n")
		for _, m := range a.Milestones {
			fmt.Fprintf(&b, "  - %s\n", m.Description)
		}
	}
	// Byte-stable marker in place of a live timestamp.
	fmt.Fprintf(&b, "detectedAt: %s\n", byteStableMarker)
	return b.String()
}

// Load reads an arc from dir/.arc.json; returns nil if absent or malformed.
func Load(dir string) (*Arc, error) {
	path := filepath.Join(dir, arcFileName)
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	var a Arc
	if err := json.Unmarshal(data, &a); err != nil {
		return nil, nil
	}
	if a.CurrentPhase == "" || a.Goals == nil {
		return nil, nil
	}
	return &a, nil
}

// Save persists the arc to dir/.arc.json (creating dir). Writes are
// best-effort and non-fatal, like the source.
func (a *Arc) Save(dir string) error {
	a.touch()
	data, err := json.MarshalIndent(a, "", "  ")
	if err != nil {
		return err
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(dir, arcFileName), data, 0o600)
}

// Reset clears the arc's tracked state back to init.
func (a *Arc) Reset() {
	a.Goals = []Goal{}
	a.Decisions = []Decision{}
	a.Milestones = []Milestone{}
	a.CurrentPhase = PhaseInit
	a.touch()
}

func (a *Arc) touch() { a.LastUpdate = time.Now().UnixMilli() }

func indexOfPhase(p Phase) int {
	for i, ph := range phaseOrder {
		if ph == p {
			return i
		}
	}
	return -1
}

func capGoals(goals []Goal) []Goal {
	if len(goals) > defaultCap {
		return goals[len(goals)-defaultCap:]
	}
	return goals
}

func capDecisions(ds []Decision) []Decision {
	if len(ds) > defaultCap {
		return ds[len(ds)-defaultCap:]
	}
	return ds
}

func capMilestones(ms []Milestone) []Milestone {
	if len(ms) > defaultCap {
		return ms[len(ms)-defaultCap:]
	}
	return ms
}
