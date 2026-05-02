package memory

import (
	"fmt"
	"strings"
)

// SleeptimeAgent runs a background LLM call to consolidate memory after conversation turns.
type SleeptimeAgent struct {
	Frequency int // run every N turns
	turnCount int
}

// NewSleeptimeAgent creates a SleeptimeAgent that triggers every frequency turns.
func NewSleeptimeAgent(frequency int) *SleeptimeAgent {
	if frequency < 1 {
		frequency = 5
	}
	return &SleeptimeAgent{Frequency: frequency}
}

// ShouldRun increments the turn counter and returns true when it's time to consolidate.
func (sa *SleeptimeAgent) ShouldRun() bool {
	sa.turnCount++
	return sa.turnCount%sa.Frequency == 0
}

// BuildConsolidationPrompt creates the prompt for the background memory agent.
func (sa *SleeptimeAgent) BuildConsolidationPrompt(transcript []string, memoryState string) string {
	return fmt.Sprintf(`You are a memory consolidation agent. Review the recent conversation and current memory state, then output structured memory updates.

<conversation>
%s
</conversation>

<current_memory>
%s
</current_memory>

Analyze the conversation and identify:
1. Important facts, user preferences, or corrections
2. Technical decisions or conventions established
3. Information that contradicts or updates existing memory

Respond with ONLY a JSON array of operations:
[
  {"op": "add", "type": "<convention|decision|preference|fact>", "content": "<what to remember>"},
  {"op": "update", "type": "<type>", "old": "<existing content substring>", "content": "<updated content>"},
  {"op": "remove", "type": "<type>", "content": "<what to forget>"}
]

If nothing worth remembering, respond with an empty array: []`,
		strings.Join(transcript, "\n"),
		memoryState,
	)
}
