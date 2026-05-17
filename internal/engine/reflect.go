package engine

import "fmt"

// ReflectPrompt generates a prompt for session self-assessment.
func ReflectPrompt(sessionSummary string) string {
	return fmt.Sprintf(`Reflect on this session. Be honest and specific.

Session summary: %s

Answer:
1. **What went well** — tasks completed successfully, good decisions made
2. **What didn't go well** — mistakes, wasted time, wrong approaches
3. **What to improve** — specific actionable changes for next time
4. **Confidence** — how confident are you the output is correct? (1-10)
5. **One lesson** — the single most important takeaway from this session`, sessionSummary)
}

// SessionReflection holds the structured output of a reflection.
type SessionReflection struct {
	WentWell   string
	WentBadly  string
	ToImprove  string
	Confidence int
	Lesson     string
}
