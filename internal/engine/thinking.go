package engine

import (
	"fmt"
	"strings"
	"sync"
	"time"
)

// ThinkingPhase represents a phase in the agent's reasoning process.
type ThinkingPhase string

const (
	PhaseUnderstand ThinkingPhase = "understand"
	PhasePlan       ThinkingPhase = "plan"
	PhaseExecute    ThinkingPhase = "execute"
	PhaseVerify     ThinkingPhase = "verify"
	PhaseReflect    ThinkingPhase = "reflect"
)

// ThinkingStep represents a single step in the thinking process.
type ThinkingStep struct {
	Phase        ThinkingPhase
	Content      string
	Confidence   float64
	Alternatives []string
	Duration     time.Duration
	Timestamp    time.Time
}

// ThinkingProtocol structures the agent's reasoning process, making it
// explicit when the agent is planning vs executing.
type ThinkingProtocol struct {
	Enabled      bool
	Visible      bool
	Steps        []ThinkingStep
	CurrentPhase ThinkingPhase
	mu           sync.RWMutex

	phaseStart time.Time
}

// NewThinkingProtocol creates a new ThinkingProtocol with defaults.
func NewThinkingProtocol() *ThinkingProtocol {
	return &ThinkingProtocol{
		Enabled: true,
		Visible: true,
		Steps:   make([]ThinkingStep, 0),
	}
}

// StartPhase transitions to a new thinking phase.
func (tp *ThinkingProtocol) StartPhase(phase ThinkingPhase) {
	tp.mu.Lock()
	defer tp.mu.Unlock()

	tp.CurrentPhase = phase
	tp.phaseStart = time.Now()
}

// AddThought records a thinking step in the current phase.
func (tp *ThinkingProtocol) AddThought(content string, confidence float64) {
	tp.mu.Lock()
	defer tp.mu.Unlock()

	step := ThinkingStep{
		Phase:        tp.CurrentPhase,
		Content:      content,
		Confidence:   confidence,
		Alternatives: make([]string, 0),
		Duration:     time.Since(tp.phaseStart),
		Timestamp:    time.Now(),
	}
	tp.Steps = append(tp.Steps, step)
}

// AddAlternative records an alternative approach considered for a thought.
func (tp *ThinkingProtocol) AddAlternative(thought, alternative string) {
	tp.mu.Lock()
	defer tp.mu.Unlock()

	for i := len(tp.Steps) - 1; i >= 0; i-- {
		if tp.Steps[i].Content == thought {
			tp.Steps[i].Alternatives = append(tp.Steps[i].Alternatives, alternative)
			return
		}
	}
}

// BuildThinkingPrompt generates a structured thinking prompt for a task.
func (tp *ThinkingProtocol) BuildThinkingPrompt(task string) string {
	return fmt.Sprintf(`Before implementing, think through this systematically:

1. UNDERSTAND: What exactly is being asked? What are the constraints?
2. PLAN: What's the approach? What files need changing? In what order?
3. RISKS: What could go wrong? What are the edge cases?
4. EXECUTE: Implement step by step.
5. VERIFY: How will you know it's correct?

Task: %s

Begin with your understanding, then plan, then execute.`, task)
}

// ParseThinking extracts thinking steps from an LLM response.
// It looks for phase markers like "Understanding:", "Plan:", "Risks:", etc.
// It also handles <thinking> tags if the model supports them.
func (tp *ThinkingProtocol) ParseThinking(response string) []ThinkingStep {
	var steps []ThinkingStep

	// Handle <thinking> tags
	thinkingContent := response
	if start := strings.Index(response, "<thinking>"); start != -1 {
		if end := strings.Index(response, "</thinking>"); end != -1 {
			thinkingContent = response[start+len("<thinking>") : end]
		}
	}

	// Phase markers to look for
	markers := []struct {
		label string
		phase ThinkingPhase
	}{
		{"Understanding:", PhaseUnderstand},
		{"UNDERSTAND:", PhaseUnderstand},
		{"Plan:", PhasePlan},
		{"PLAN:", PhasePlan},
		{"Risks:", PhaseVerify},
		{"RISKS:", PhaseVerify},
		{"Execute:", PhaseExecute},
		{"EXECUTE:", PhaseExecute},
		{"Verify:", PhaseVerify},
		{"VERIFY:", PhaseVerify},
	}

	lines := strings.Split(thinkingContent, "\n")
	var currentPhase ThinkingPhase
	var currentContent []string

	flushCurrent := func() {
		if currentPhase != "" && len(currentContent) > 0 {
			content := strings.TrimSpace(strings.Join(currentContent, "\n"))
			if content != "" {
				steps = append(steps, ThinkingStep{
					Phase:        currentPhase,
					Content:      content,
					Confidence:   0.8,
					Alternatives: make([]string, 0),
					Timestamp:    time.Now(),
				})
			}
		}
		currentContent = nil
	}

	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		matched := false
		for _, m := range markers {
			if strings.HasPrefix(trimmed, m.label) {
				flushCurrent()
				currentPhase = m.phase
				// Include any text after the marker on the same line
				remainder := strings.TrimSpace(trimmed[len(m.label):])
				if remainder != "" {
					currentContent = append(currentContent, remainder)
				}
				matched = true
				break
			}
		}
		if !matched && currentPhase != "" {
			currentContent = append(currentContent, line)
		}
	}
	flushCurrent()

	return steps
}

// ShouldThinkFirst applies heuristics to determine if a task needs
// structured thinking before execution.
func (tp *ThinkingProtocol) ShouldThinkFirst(task string) bool {
	// Simple questions don't need thinking
	if len(task) < 20 {
		return false
	}

	// >100 words in prompt → yes
	words := strings.Fields(task)
	if len(words) > 100 {
		return true
	}

	// Keywords that indicate complexity
	complexKeywords := []string{
		"refactor", "redesign", "migrate", "implement",
		"multiple files", "multi-file", "architecture",
		"integrate", "overhaul", "rewrite",
	}
	lower := strings.ToLower(task)
	for _, kw := range complexKeywords {
		if strings.Contains(lower, kw) {
			return true
		}
	}

	// Multi-file changes → yes
	fileIndicators := []string{".go", ".ts", ".py", ".js", ".rs"}
	fileCount := 0
	for _, ind := range fileIndicators {
		fileCount += strings.Count(lower, ind)
	}
	if fileCount > 1 {
		return true
	}

	return false
}

// FormatThinking formats thinking steps for display to the user.
func (tp *ThinkingProtocol) FormatThinking(steps []ThinkingStep) string {
	if len(steps) == 0 {
		return ""
	}

	var sb strings.Builder
	sb.WriteString("\xf0\x9f\x92\xad Thinking Process:\n")

	// Group by phase
	phaseOrder := []ThinkingPhase{PhaseUnderstand, PhasePlan, PhaseExecute, PhaseVerify, PhaseReflect}
	phaseLabels := map[ThinkingPhase]string{
		PhaseUnderstand: "Understanding",
		PhasePlan:       "Plan",
		PhaseExecute:    "Execute",
		PhaseVerify:     "Verify",
		PhaseReflect:    "Reflect",
	}

	grouped := make(map[ThinkingPhase][]ThinkingStep)
	for _, step := range steps {
		grouped[step.Phase] = append(grouped[step.Phase], step)
	}

	for _, phase := range phaseOrder {
		phaseSteps, exists := grouped[phase]
		if !exists || len(phaseSteps) == 0 {
			continue
		}

		// Use the highest confidence for the phase header
		maxConf := 0.0
		for _, s := range phaseSteps {
			if s.Confidence > maxConf {
				maxConf = s.Confidence
			}
		}

		sb.WriteString(fmt.Sprintf("\n%s (confidence: %.2f):\n", phaseLabels[phase], maxConf))

		for _, step := range phaseSteps {
			// Indent each line of content
			for _, line := range strings.Split(step.Content, "\n") {
				trimmed := strings.TrimSpace(line)
				if trimmed != "" {
					sb.WriteString(fmt.Sprintf("  %s\n", trimmed))
				}
			}
			for _, alt := range step.Alternatives {
				sb.WriteString(fmt.Sprintf("  Alternative considered: %s\n", alt))
			}
		}
	}

	return sb.String()
}

// SummarizeThinking provides a one-line summary of the thinking process.
func (tp *ThinkingProtocol) SummarizeThinking() string {
	tp.mu.RLock()
	defer tp.mu.RUnlock()

	if len(tp.Steps) == 0 {
		return "No thinking recorded."
	}

	phases := make(map[ThinkingPhase]bool)
	totalConf := 0.0
	for _, s := range tp.Steps {
		phases[s.Phase] = true
		totalConf += s.Confidence
	}
	avgConf := totalConf / float64(len(tp.Steps))

	phaseNames := make([]string, 0, len(phases))
	phaseOrder := []ThinkingPhase{PhaseUnderstand, PhasePlan, PhaseExecute, PhaseVerify, PhaseReflect}
	for _, p := range phaseOrder {
		if phases[p] {
			phaseNames = append(phaseNames, string(p))
		}
	}

	return fmt.Sprintf("Thought through %d steps across phases [%s] (avg confidence: %.2f)",
		len(tp.Steps), strings.Join(phaseNames, ", "), avgConf)
}

// GetPhaseHistory returns all thinking steps grouped by phase.
func (tp *ThinkingProtocol) GetPhaseHistory() map[ThinkingPhase][]ThinkingStep {
	tp.mu.RLock()
	defer tp.mu.RUnlock()

	result := make(map[ThinkingPhase][]ThinkingStep)
	for _, step := range tp.Steps {
		result[step.Phase] = append(result[step.Phase], step)
	}
	return result
}

// ResetForNewTask clears all thinking state for a new task.
func (tp *ThinkingProtocol) ResetForNewTask() {
	tp.mu.Lock()
	defer tp.mu.Unlock()

	tp.Steps = make([]ThinkingStep, 0)
	tp.CurrentPhase = ""
	tp.phaseStart = time.Time{}
}
