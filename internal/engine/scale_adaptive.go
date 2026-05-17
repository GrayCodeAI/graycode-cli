package engine

import "strings"

// TaskScale represents the complexity/scope of a user request.
type TaskScale int

const (
	ScalePatch TaskScale = iota // typo, one-liner, rename
	ScaleMinor                  // add error handling, small feature
	ScaleMajor                  // refactor module, multi-file change
	ScaleEpic                   // new system, architecture change
)

func (s TaskScale) String() string {
	switch s {
	case ScalePatch:
		return "patch"
	case ScaleMinor:
		return "minor"
	case ScaleMajor:
		return "major"
	case ScaleEpic:
		return "epic"
	default:
		return "unknown"
	}
}

// ScaleBehavior defines how hawk adjusts its behavior per scale.
type ScaleBehavior struct {
	Scale        TaskScale
	MaxTurns     int
	PlanRequired bool
	AutoApprove  bool   // auto-approve file edits
	ScanScope    string // "file", "module", "repo"
}

// ScaleBehaviors maps each scale to its behavior config.
var ScaleBehaviors = map[TaskScale]ScaleBehavior{
	ScalePatch: {Scale: ScalePatch, MaxTurns: 3, PlanRequired: false, AutoApprove: true, ScanScope: "file"},
	ScaleMinor: {Scale: ScaleMinor, MaxTurns: 10, PlanRequired: false, AutoApprove: false, ScanScope: "module"},
	ScaleMajor: {Scale: ScaleMajor, MaxTurns: 20, PlanRequired: true, AutoApprove: false, ScanScope: "module"},
	ScaleEpic:  {Scale: ScaleEpic, MaxTurns: 50, PlanRequired: true, AutoApprove: false, ScanScope: "repo"},
}

var (
	patchKeywords = []string{"fix", "typo", "rename", "bump", "update version", "remove unused", "delete", "correct"}
	minorKeywords = []string{"add", "implement", "handle", "support", "include", "extend", "improve"}
	majorKeywords = []string{"refactor", "redesign", "migrate", "rewrite", "restructure", "overhaul", "consolidate"}
	epicKeywords  = []string{"build", "create new", "design system", "architecture", "from scratch", "new service", "new module"}
)

// ClassifyScale determines the task scale from user input.
func ClassifyScale(prompt string) TaskScale {
	lower := strings.ToLower(prompt)
	words := strings.Fields(lower)

	// Check keywords in priority order (epic first)
	for _, kw := range epicKeywords {
		if strings.Contains(lower, kw) {
			return ScaleEpic
		}
	}
	for _, kw := range majorKeywords {
		if strings.Contains(lower, kw) {
			return ScaleMajor
		}
	}
	for _, kw := range minorKeywords {
		if strings.Contains(lower, kw) {
			return ScaleMinor
		}
	}
	for _, kw := range patchKeywords {
		if strings.Contains(lower, kw) {
			return ScalePatch
		}
	}

	// Heuristic: short prompts are likely patches, long ones are bigger
	if len(words) <= 5 {
		return ScalePatch
	}
	if len(words) <= 15 {
		return ScaleMinor
	}
	return ScaleMajor
}

// GetBehavior returns the behavior config for a given scale.
func GetBehavior(scale TaskScale) ScaleBehavior {
	if b, ok := ScaleBehaviors[scale]; ok {
		return b
	}
	return ScaleBehaviors[ScaleMinor]
}
