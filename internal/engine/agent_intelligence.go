package engine

import (
	"context"
	"strings"
)

// AgentIntelligence provides smart routing, auto-spawning, and synthesis for agents.
type AgentIntelligence struct {
	ScaleClassifier func(string) TaskScale
}

// NewAgentIntelligence creates the intelligence layer.
func NewAgentIntelligence() *AgentIntelligence {
	return &AgentIntelligence{ScaleClassifier: ClassifyScale}
}

// SpawnDecision determines whether and how to parallelize a task.
type SpawnDecision struct {
	ShouldParallelize bool
	SubTasks          []SubTask
	Strategy          SpawnStrategy
}

// SubTask is a decomposed piece of work for a sub-agent.
type SubTask struct {
	ID        string
	Prompt    string
	Mode      SubAgentMode
	Priority  int      // higher = run first
	DependsOn []string // IDs of tasks this depends on
}

// SpawnStrategy defines how agents coordinate.
type SpawnStrategy int

const (
	StrategySequential SpawnStrategy = iota // one after another
	StrategyParallel                        // all at once, merge results
	StrategyPipeline                        // output of one feeds next
	StrategySingle                          // no decomposition needed
)

// AnalyzeForParallelism determines if a task should be split into parallel subtasks.
func (ai *AgentIntelligence) AnalyzeForParallelism(prompt string) SpawnDecision {
	scale := ai.ScaleClassifier(prompt)

	// Patches and minor tasks don't benefit from parallelism
	if scale <= ScaleMinor {
		return SpawnDecision{Strategy: StrategySingle}
	}

	// Detect parallelizable patterns
	subtasks := ai.decomposeTask(prompt, scale)
	if len(subtasks) <= 1 {
		return SpawnDecision{Strategy: StrategySingle}
	}

	// Check for dependencies
	hasDeps := false
	for _, st := range subtasks {
		if len(st.DependsOn) > 0 {
			hasDeps = true
			break
		}
	}

	strategy := StrategyParallel
	if hasDeps {
		strategy = StrategyPipeline
	}

	return SpawnDecision{
		ShouldParallelize: true,
		SubTasks:          subtasks,
		Strategy:          strategy,
	}
}

// SelectMode picks the optimal agent mode for a subtask.
func (ai *AgentIntelligence) SelectMode(subtask string) SubAgentMode {
	lower := strings.ToLower(subtask)

	// Read-only tasks → explore mode (cheaper, faster)
	readOnlyKeywords := []string{"find", "search", "list", "check", "read", "analyze", "look", "scan", "grep", "what is", "where is", "how many"}
	for _, kw := range readOnlyKeywords {
		if strings.Contains(lower, kw) {
			return SubAgentExplore
		}
	}

	// Write tasks → general mode
	return SubAgentGeneral
}

// decomposeTask splits a complex task into subtasks based on patterns.
func (ai *AgentIntelligence) decomposeTask(prompt string, scale TaskScale) []SubTask {
	lower := strings.ToLower(prompt)

	// Pattern: research then implement — pipeline (check first, higher priority)
	if (strings.Contains(lower, "research") || strings.Contains(lower, "analyze")) &&
		(strings.Contains(lower, "implement") || strings.Contains(lower, "build") || strings.Contains(lower, "create")) {
		return []SubTask{
			{ID: "research", Prompt: "Research and analyze: " + prompt, Mode: SubAgentExplore},
			{ID: "implement", Prompt: "Based on research, implement: " + prompt, Mode: SubAgentGeneral, DependsOn: []string{"research"}},
		}
	}

	// Pattern: multi-file refactor — pipeline
	if strings.Contains(lower, "refactor") && scale >= ScaleMajor {
		return []SubTask{
			{ID: "scan", Prompt: "Scan and identify all files that need changes for: " + prompt, Mode: SubAgentExplore},
			{ID: "plan", Prompt: "Create a refactoring plan based on scan results: " + prompt, Mode: SubAgentExplore, DependsOn: []string{"scan"}},
			{ID: "execute", Prompt: "Execute the refactoring plan: " + prompt, Mode: SubAgentGeneral, DependsOn: []string{"plan"}},
		}
	}

	// Pattern: "X and Y" — parallel independent tasks
	if strings.Contains(lower, " and ") && scale >= ScaleMajor {
		parts := splitOnConjunctions(prompt)
		if len(parts) >= 2 {
			var subtasks []SubTask
			for i, part := range parts {
				subtasks = append(subtasks, SubTask{
					ID:     string(rune('a' + i)),
					Prompt: strings.TrimSpace(part),
					Mode:   ai.SelectMode(part),
				})
			}
			return subtasks
		}
	}

	return nil
}

// MergeSynthesisPrompt generates a prompt to merge results from parallel agents.
func MergeSynthesisPrompt(subtasks []SubTask, results map[string]string) string {
	var sb strings.Builder
	sb.WriteString("Multiple agents worked on parts of this task. Synthesize their results into a coherent whole.\n\n")
	for _, st := range subtasks {
		if result, ok := results[st.ID]; ok {
			sb.WriteString("## Agent " + st.ID + " (" + string(st.Mode) + ")\n")
			sb.WriteString("Task: " + st.Prompt + "\n")
			sb.WriteString("Result:\n" + result + "\n\n")
		}
	}
	sb.WriteString("## Synthesis\nCombine the above into a unified response. Resolve any conflicts. Present the final answer.")
	return sb.String()
}

// SelfAwareness allows an agent to recognize its own limitations.
type SelfAwareness struct {
	MaxComplexity TaskScale // tasks above this get delegated
	Specialties   []string  // what this agent is good at
}

// ShouldDelegate returns true if the agent should pass this task to a more capable agent.
func (sa *SelfAwareness) ShouldDelegate(prompt string, currentScale TaskScale) bool {
	return currentScale > sa.MaxComplexity
}

// DelegationPrompt generates a prompt explaining why delegation is needed.
func (sa *SelfAwareness) DelegationPrompt(prompt string, reason string) string {
	return "I've determined this task exceeds my current scope. Reason: " + reason +
		"\n\nDelegating to a more capable agent with full tool access.\n\nOriginal task: " + prompt
}

func splitOnConjunctions(s string) []string {
	// Split on " and " but not inside quotes
	parts := strings.Split(s, " and ")
	if len(parts) >= 2 {
		return parts
	}
	// Try comma-separated
	parts = strings.Split(s, ", ")
	var result []string
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p != "" && p != "and" {
			result = append(result, p)
		}
	}
	return result
}

// ExecuteWithIntelligence runs a task with smart agent routing.
func (ai *AgentIntelligence) ExecuteWithIntelligence(ctx context.Context, prompt string, execFn func(context.Context, string, SubAgentMode) (string, error)) (string, error) {
	decision := ai.AnalyzeForParallelism(prompt)

	if !decision.ShouldParallelize {
		mode := ai.SelectMode(prompt)
		return execFn(ctx, prompt, mode)
	}

	// Execute based on strategy
	results := make(map[string]string)

	switch decision.Strategy {
	case StrategyParallel:
		// Run all independent tasks in parallel
		type result struct {
			id  string
			out string
			err error
		}
		ch := make(chan result, len(decision.SubTasks))
		for _, st := range decision.SubTasks {
			go func(task SubTask) {
				out, err := execFn(ctx, task.Prompt, task.Mode)
				ch <- result{id: task.ID, out: out, err: err}
			}(st)
		}
		for range decision.SubTasks {
			r := <-ch
			if r.err == nil {
				results[r.id] = r.out
			}
		}

	case StrategyPipeline:
		// Execute in dependency order
		completed := make(map[string]bool)
		for len(completed) < len(decision.SubTasks) {
			for _, st := range decision.SubTasks {
				if completed[st.ID] {
					continue
				}
				// Check deps
				depsReady := true
				for _, dep := range st.DependsOn {
					if !completed[dep] {
						depsReady = false
						break
					}
				}
				if !depsReady {
					continue
				}
				// Inject dependency results into prompt
				taskPrompt := st.Prompt
				for _, dep := range st.DependsOn {
					if r, ok := results[dep]; ok {
						taskPrompt += "\n\n[Context from previous step '" + dep + "']: " + r
					}
				}
				out, err := execFn(ctx, taskPrompt, st.Mode)
				if err == nil {
					results[st.ID] = out
				}
				completed[st.ID] = true
			}
		}
	}

	// Synthesize results
	return MergeSynthesisPrompt(decision.SubTasks, results), nil
}
