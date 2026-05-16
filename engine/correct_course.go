package engine

// FailureLayer identifies where a problem originated.
type FailureLayer int

const (
	LayerIntent         FailureLayer = iota // user's request was unclear/wrong
	LayerSpec                               // spec/plan was flawed
	LayerImplementation                     // code is wrong but spec was right
)

func (l FailureLayer) String() string {
	switch l {
	case LayerIntent:
		return "intent"
	case LayerSpec:
		return "spec"
	case LayerImplementation:
		return "implementation"
	default:
		return "unknown"
	}
}

// CorrectCoursePrompt generates a prompt to diagnose where things went wrong.
func CorrectCoursePrompt(originalIntent, currentState, problem string) string {
	return `Something went wrong. Diagnose which layer the failure came from.

**Original intent:** ` + originalIntent + `
**Current state:** ` + currentState + `
**Problem:** ` + problem + `

Diagnose:
1. **Intent layer** — Was the original request unclear, contradictory, or wrong?
   → If yes: we need to re-clarify with the user.

2. **Spec layer** — Was the plan/spec flawed? Did it miss a requirement or make a wrong assumption?
   → If yes: regenerate the spec, don't patch the code.

3. **Implementation layer** — Was the code wrong but the spec was correct?
   → If yes: fix the code locally.

RESPOND WITH:
- **Layer:** intent | spec | implementation
- **Diagnosis:** what specifically went wrong at that layer
- **Action:** what to do next`
}
