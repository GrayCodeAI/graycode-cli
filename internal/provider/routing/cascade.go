package routing

import "strings"

// TaskType classifies a user message for model routing.
type TaskType string

const (
	TaskPlanning TaskType = "planning"
	TaskCoding   TaskType = "coding"
	TaskSummary  TaskType = "summary"
	TaskReview   TaskType = "review"
	TaskGeneral  TaskType = "general"
)

// CascadeRouter selects models based on task classification.
// It uses heuristic keyword matching to avoid LLM calls for routing.
type CascadeRouter struct {
	roles ModelRoles
}

// NewCascadeRouter creates a cascade router with the given role assignments.
func NewCascadeRouter(roles ModelRoles) *CascadeRouter {
	return &CascadeRouter{roles: roles}
}

// Route returns the model name for a given message. If hint is non-empty, it
// overrides the automatic classification.
func (cr *CascadeRouter) Route(message string, hint TaskType) string {
	task := hint
	if task == "" {
		task = ClassifyTask(message)
	}
	return cr.ModelForTask(task)
}

// ModelForTask returns the model assigned to the given task type.
func (cr *CascadeRouter) ModelForTask(task TaskType) string {
	switch task {
	case TaskPlanning:
		return cr.roles.ModelForRole(RolePlanner)
	case TaskCoding:
		return cr.roles.ModelForRole(RoleCoder)
	case TaskSummary:
		return cr.roles.ModelForRole(RoleCommit)
	case TaskReview:
		return cr.roles.ModelForRole(RoleReviewer)
	default:
		return cr.roles.ModelForRole(RoleCoder)
	}
}

// ClassifyTask uses keyword heuristics to determine the task type from a message.
func ClassifyTask(message string) TaskType {
	lower := strings.ToLower(message)

	if matchesAny(lower, planningSignals) {
		return TaskPlanning
	}
	if matchesAny(lower, reviewSignals) {
		return TaskReview
	}
	if matchesAny(lower, summarySignals) {
		return TaskSummary
	}
	if matchesAny(lower, codingSignals) {
		return TaskCoding
	}
	return TaskGeneral
}

var planningSignals = []string{
	"plan", "design", "architect", "how should we",
	"strategy", "approach", "break down", "decompose",
	"what's the best way", "outline", "roadmap",
}

var reviewSignals = []string{
	"review", "check this", "look at this",
	"any issues", "code review", "audit",
	"analyze this", "what do you think of",
}

var summarySignals = []string{
	"summarize", "tldr", "recap", "what happened",
	"brief", "short version", "commit message",
	"describe what", "changelog",
}

var codingSignals = []string{
	"implement", "write", "fix", "edit", "refactor",
	"add", "create", "build", "change", "update",
	"modify", "delete", "remove", "replace",
}

func matchesAny(text string, signals []string) bool {
	for _, sig := range signals {
		if strings.Contains(text, sig) {
			return true
		}
	}
	return false
}
