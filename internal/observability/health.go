package analytics

// SessionHealth represents the computed health of a session.
type SessionHealth struct {
	Score   int            `json:"score"`
	Grade   string         `json:"grade"`
	Signals map[string]int `json:"signals"`
}

// ComputeSessionHealth scores a session from 0-100 based on penalty signals.
func ComputeSessionHealth(toolCalls, toolErrors, editRetries, compactions, midTaskCompactions int, outcome string) SessionHealth {
	score := 100
	signals := map[string]int{}

	switch outcome {
	case "errored":
		signals["outcome_errored"] = -30
		score += signals["outcome_errored"]
	case "abandoned":
		signals["outcome_abandoned"] = -15
		score += signals["outcome_abandoned"]
	}

	if p := toolErrors * -5; p < 0 {
		signals["tool_error"] = p
		score += p
	}
	if p := editRetries * -5; p < 0 {
		signals["edit_retry"] = p
		score += p
	}
	if p := midTaskCompactions * -8; p < 0 {
		signals["mid_task_compaction"] = p
		score += p
	}
	if p := compactions * -2; p < 0 {
		signals["compaction"] = p
		score += p
	}

	if score < 0 {
		score = 0
	}

	return SessionHealth{Score: score, Grade: gradeFromScore(score), Signals: signals}
}

func gradeFromScore(s int) string {
	switch {
	case s >= 90:
		return "A"
	case s >= 80:
		return "B"
	case s >= 70:
		return "C"
	case s >= 60:
		return "D"
	default:
		return "F"
	}
}

// DetectMidTaskCompaction returns true if any tool name appears in both windows,
// indicating compaction interrupted active work.
func DetectMidTaskCompaction(toolsBefore, toolsAfter []string) bool {
	set := make(map[string]struct{}, len(toolsBefore))
	for _, t := range toolsBefore {
		set[t] = struct{}{}
	}
	for _, t := range toolsAfter {
		if _, ok := set[t]; ok {
			return true
		}
	}
	return false
}
