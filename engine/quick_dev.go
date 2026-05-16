package engine

// QuickDevPhase represents a step in the quick-dev workflow.
type QuickDevPhase int

const (
	QuickDevClarify   QuickDevPhase = iota // compress intent into one goal
	QuickDevRoute                          // one-shot vs planned
	QuickDevImplement                      // execute
	QuickDevReview                         // adversarial self-review
	QuickDevPresent                        // show results
)

func (p QuickDevPhase) String() string {
	switch p {
	case QuickDevClarify:
		return "clarify"
	case QuickDevRoute:
		return "route"
	case QuickDevImplement:
		return "implement"
	case QuickDevReview:
		return "review"
	case QuickDevPresent:
		return "present"
	default:
		return "unknown"
	}
}

// QuickDevState tracks the current state of a quick-dev workflow.
type QuickDevState struct {
	Phase       QuickDevPhase
	Intent      string // compressed user intent
	IsOneShot   bool   // true = skip planning
	Spec        string // frozen spec (if planned path)
	FilesChanged []string
}

// QuickDevClarifyPrompt returns the prompt for intent clarification.
func QuickDevClarifyPrompt(userInput string) string {
	return `The user wants to make a change. Compress their intent into ONE clear, unambiguous goal.

User said: "` + userInput + `"

Respond with:
1. **Goal** (one sentence, no ambiguity)
2. **Scope** (which files/modules are affected)
3. **Route** — is this a ONE-SHOT (trivial, <3 files, no design decisions) or PLANNED (needs a spec first)?

If ONE-SHOT: proceed directly to implementation.
If PLANNED: write a brief spec (what to change, acceptance criteria) and wait for approval.`
}

// QuickDevReviewPrompt returns the prompt for self-review after implementation.
func QuickDevReviewPrompt(filesChanged []string) string {
	return ReviewPrompt(filesChanged)
}

// QuickDevPresentPrompt returns the prompt for presenting results.
func QuickDevPresentPrompt() string {
	return `Summarize what was done in 3-5 bullet points:
- What changed (files, functions)
- What was the intent
- Any concerns or follow-up items
- Suggested next step (test, commit, review)`
}
