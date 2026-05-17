package engine

// InvestigatePhase represents a step in structured debugging.
type InvestigatePhase int

const (
	InvestigateReproduce InvestigatePhase = iota // reproduce the issue
	InvestigateIsolate                           // narrow down the cause
	InvestigateRootCause                         // identify root cause
	InvestigateFix                               // propose and apply fix
	InvestigateVerify                            // confirm fix works
)

func (p InvestigatePhase) String() string {
	switch p {
	case InvestigateReproduce:
		return "reproduce"
	case InvestigateIsolate:
		return "isolate"
	case InvestigateRootCause:
		return "root-cause"
	case InvestigateFix:
		return "fix"
	case InvestigateVerify:
		return "verify"
	default:
		return "unknown"
	}
}

// InvestigatePrompt returns the prompt for each investigation phase.
func InvestigatePrompt(phase InvestigatePhase, context string) string {
	switch phase {
	case InvestigateReproduce:
		return `PHASE: Reproduce
` + context + `

Steps:
1. Identify the exact command/action that triggers the bug
2. Run it and capture the error output
3. Confirm it fails consistently (not flaky)
4. Document: input, expected output, actual output`

	case InvestigateIsolate:
		return `PHASE: Isolate
Narrow down the cause:
1. Which file(s) are involved?
2. Which function is the entry point for this code path?
3. Add minimal logging/prints to trace execution
4. Binary search: comment out code to find the failing section`

	case InvestigateRootCause:
		return `PHASE: Root Cause
You've isolated the area. Now identify WHY:
1. What assumption is violated?
2. What state is unexpected?
3. Is this a logic error, data error, or timing error?
4. State the root cause in one sentence.`

	case InvestigateFix:
		return `PHASE: Fix
Root cause identified. Now fix it:
1. Make the minimal change that addresses the root cause
2. Don't fix symptoms — fix the cause
3. Consider: does this fix introduce new edge cases?
4. Apply the fix.`

	case InvestigateVerify:
		return `PHASE: Verify
1. Run the original reproduction steps — does it pass now?
2. Run the full test suite — no regressions?
3. Add a test that would have caught this bug
4. Summarize: what was wrong, what was fixed, what test was added`

	default:
		return ""
	}
}
