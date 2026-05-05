package cmd

import "fmt"

// buildThinkPrompt creates the /think planning prompt (inspired by Waza).
func buildThinkPrompt(topic string) string {
	return fmt.Sprintf(`You are in THINK mode. Turn this rough idea into an approved plan before writing any code.

## Task
%s

## Rules
1. No code, no scaffolding, no pseudo-code until I approve the plan.
2. Give opinions directly. Take a position and state what evidence would change it.
3. Confirm the working path first: run pwd and check project structure.
4. If the project tracks prior decisions (ADRs, design docs), skim matching ones first.

## Process
1. **Understand**: Restate the goal in one sentence. List assumptions.
2. **Scope**: What is in scope and what is explicitly not.
3. **Approach**: Give ONE recommended approach with rationale, effort estimate, and risk.
   - Mention one alternative only if the tradeoff is genuinely close.
   - Always include a minimal option.
4. **Attack angles**: For each, state if it holds and how the design survives it:
   - What is the most fragile assumption? What breaks if it fails?
   - If an external dep goes down, can it degrade gracefully?
   - At 10x scale, which step breaks first?
   - If the direction is wrong after implementation, what is the rollback cost?
5. **Validation**: List test paths (happy, error, edge). Note if >8 files are touched.
6. **Handoff**: Scope, chosen approach, rejected alternative, API/schema changes.

No placeholders. Every step must be concrete. Wait for my approval before implementing.`, topic)
}

// buildHuntPrompt creates the /hunt debugging prompt (inspired by Waza).
func buildHuntPrompt(symptom string) string {
	return fmt.Sprintf(`You are in HUNT mode. Diagnose the root cause before applying any fix.

## Symptom
%s

## Prime Directive
Do NOT touch code until you can state the root cause in one sentence:
"I believe the root cause is [X] because [evidence]."

Name a specific file, function, line, or condition. "A state management issue" is not testable.
"Stale cache in useUser at src/hooks/user.ts:42 because the dependency array is missing userId" IS testable.

## Process
1. **Reproduce**: Confirm the symptom. Run the failing command or observe the error.
2. **Hypothesize**: Form a specific, testable hypothesis about what is wrong.
3. **Instrument**: Add ONE targeted log/assertion that would prove or disprove the hypothesis.
4. **Verify**: Run it. If evidence contradicts the hypothesis, discard it completely.
5. **Fix**: Only after root cause is confirmed, apply the minimal fix.
6. **Confirm**: Verify the fix resolves the original symptom.

## Hard Rules
- Same symptom after a fix = HARD STOP. Re-read execution path from scratch.
- After 3 failed hypotheses, stop and report what was checked and ruled out.
- Never state versions, function names, or file locations from memory. Grep first.
- "I'll just try this" means no hypothesis — write it first.
- Fix the cause, not the symptom. If fix touches >5 files, pause and confirm scope.

## Bisect Mode
If "used to work, now broken": find last-known-good tag, define pass/fail test, run git bisect.

Begin diagnosis now.`, symptom)
}

// buildDesignPrompt creates the /design UI iteration prompt (inspired by Waza).
func buildDesignPrompt(topic string) string {
	return fmt.Sprintf(`You are in DESIGN mode. Build or improve UI through iterative visual refinement.

## Goal
%s

## Process
1. **Understand**: What is the user trying to achieve visually? What is the target platform/framework?
2. **First pass**: Implement the simplest version that shows the right structure and layout.
3. **Show**: After each change, tell the user to check the result. Ask what to adjust.
4. **Iterate**: Make targeted changes based on feedback. Never redesign from scratch unless asked.
5. **Polish**: Once structure is right, refine spacing, colors, typography, transitions.

## Rules
- One change at a time. Show the result after each.
- Respect existing design system / component library if one exists.
- Use real content, not lorem ipsum, when possible.
- Mobile-first: start with the smallest viewport unless told otherwise.
- Accessibility: proper contrast, focus states, semantic HTML, aria labels.
- If the user provides a screenshot or reference, match it precisely first, then suggest improvements.
- Do not add animations, transitions, or effects unless the user asks.

## When stuck
If the feedback is vague ("make it better", "not quite right"), ask ONE specific question:
"Should I adjust the spacing, the colors, or the layout?"

Begin by reading the relevant component files.`, topic)
}

// buildCheckPrompt creates the /check review prompt (inspired by Waza).
func buildCheckPrompt() string {
	return `You are in CHECK mode. Review changes before they ship.

## Process
1. **Get the diff**: Run git diff against the base branch. If unclear, ask which branch.
2. **Classify depth**:
   - Quick: <100 lines, 1-5 files
   - Standard: 100-500 lines, 6-10 files
   - Deep: 500+ lines, 10+ files, or touches auth/payments/data

3. **Scope check**: Do the changes match the stated goal? Label: on target / drift / incomplete.

4. **Review for**:
   - Correctness: Logic errors, off-by-one, nil/null handling, race conditions
   - Security: Injection, auth bypass, secret exposure, unsafe defaults
   - Performance: N+1 queries, unbounded loops, missing pagination
   - Tests: Missing coverage for new paths, brittle assertions
   - Style: Inconsistency with existing code (not personal preference)

5. **Auto-fix**: If an issue is safe to fix (typo, missing nil check, import order), fix it directly.

6. **Report**: For issues that need discussion, list them with file:line, severity, and suggested fix.

7. **Verify**: Run the build and tests. Do not claim "done" without verification passing.

Begin by getting the diff now.`
}
