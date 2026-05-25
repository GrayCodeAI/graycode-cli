package engine

import "fmt"

// BrainstormPhase represents a step in guided brainstorming.
type BrainstormPhase int

const (
	BrainstormSetup    BrainstormPhase = iota // define the problem space
	BrainstormDiverge                         // generate ideas (quantity over quality)
	BrainstormOrganize                        // cluster and categorize
	BrainstormEvaluate                        // score and prioritize
	BrainstormConverge                        // select and refine top ideas
)

func (p BrainstormPhase) String() string {
	switch p {
	case BrainstormSetup:
		return "setup"
	case BrainstormDiverge:
		return "diverge"
	case BrainstormOrganize:
		return "organize"
	case BrainstormEvaluate:
		return "evaluate"
	case BrainstormConverge:
		return "converge"
	default:
		return "unknown"
	}
}

// BrainstormSession tracks a brainstorming session.
type BrainstormSession struct {
	Topic    string
	Phase    BrainstormPhase
	Ideas    []string
	Clusters map[string][]string
	TopPicks []string
}

// NewBrainstormSession starts a new brainstorming session.
func NewBrainstormSession(topic string) *BrainstormSession {
	return &BrainstormSession{
		Topic:    topic,
		Phase:    BrainstormSetup,
		Clusters: make(map[string][]string),
	}
}

// BrainstormPrompt returns the facilitation prompt for each phase.
func BrainstormPrompt(phase BrainstormPhase, topic string, context string) string {
	switch phase {
	case BrainstormSetup:
		return fmt.Sprintf(`You are a brainstorming facilitator. The user wants to explore: "%s"

SETUP PHASE:
1. Restate the problem/opportunity in one clear sentence
2. Ask 2-3 clarifying questions to bound the space:
   - What constraints exist? (time, tech, budget)
   - Who is this for?
   - What does success look like?
3. Once answered, move to divergent thinking.`, topic)

	case BrainstormDiverge:
		return fmt.Sprintf(`DIVERGE PHASE — Generate ideas for: "%s"
%s

RULES:
- Quantity over quality — aim for 10-15 ideas
- No judgment yet — wild ideas welcome
- Build on previous ideas (yes-and)
- Mix practical and ambitious
- One line per idea, numbered

Go:`, topic, context)

	case BrainstormOrganize:
		return `ORGANIZE PHASE — Cluster the ideas into 3-5 themes.

For each cluster:
- Name it (2-3 words)
- List which ideas belong
- One-sentence summary of the theme

Don't evaluate yet — just group.`

	case BrainstormEvaluate:
		return `EVALUATE PHASE — Score each cluster on:
- **Feasibility** (1-5): Can a individual dev build this in reasonable time?
- **Impact** (1-5): How much value does this deliver?
- **Novelty** (1-5): How differentiated is this?

Format: | Cluster | Feasibility | Impact | Novelty | Total |`

	case BrainstormConverge:
		return `CONVERGE PHASE — Pick the top 1-3 ideas and refine them:

For each winner:
1. **What**: One paragraph description
2. **Why**: Why this over the alternatives
3. **First step**: The very next action to take
4. **Risk**: What could go wrong

End with: "Ready to start? Which one should we build?"`

	default:
		return ""
	}
}
