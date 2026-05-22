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

// buildDesignScreenshotPrompt creates the /design screenshot analysis prompt.
func buildDesignScreenshotPrompt(path string) string {
	return fmt.Sprintf(`You are in DESIGN SCREENSHOT mode. Analyze a screenshot for visual improvements.

## Screenshot path
%s

## Process
1. **Read the screenshot file** and describe what you see — layout, spacing, color palette, typography, component hierarchy.
2. **Audit against these dimensions**, rating each 1-5 and noting specific issues:
   - Visual hierarchy: Is the most important content prominent?
   - Spacing & alignment: Consistent padding, grid alignment?
   - Color & contrast: Accessible contrast ratios, meaningful color use?
   - Typography: Readable font sizes, appropriate line-height, limited font faces?
   - Consistency: Do elements of the same type look the same?
   - Cognitive load: Is information density appropriate for the task?
3. **List the top 3 issues** ranked by user impact.
4. **For each issue**, suggest a specific CSS/component change.
5. **If a reference design or brand guide exists** in the project, compare against it.

Output a structured report. Be specific — name exact colors, spacing values, and components.`, path)
}

// buildDesignSystemPrompt creates the /design system extraction prompt.
func buildDesignSystemPrompt(dir string) string {
	return fmt.Sprintf(`You are in DESIGN SYSTEM mode. Extract and scaffold a design system from existing UI code.

## Target directory
%s

## Process
1. **Scan the codebase** for existing UI components, stylesheets, and configuration files.
2. **Extract design tokens** by reading:
   - CSS/Tailwind variables and custom properties
   - Color values used across components (backgrounds, text, borders, accents)
   - Typography: font families, sizes, weights, line heights
   - Spacing: margins, paddings, grid gaps
   - Border radii, shadows, transitions
   - Breakpoints / media query values
3. **Detect patterns**: How are components structured? What prop patterns exist? Error/loading/empty states?
4. **Generate** a design system scaffold:
   - tokens.json: All extracted design tokens in a structured format
   - colors.md: Color palette with usage guidelines and contrast ratios
   - typography.md: Type scale with usage context
   - spacing.md: Spacing scale
   - components/index.md: Catalog of existing components with props and states
   - CONTRIBUTING.md: Design system contribution guidelines
5. **Flag inconsistencies**: Where do similar components use different spacing or colors?

Rules: Do not modify existing components. Only create documentation files. If no design tokens are detected, scaffold a minimal set based on framework defaults.`, dir)
}

// buildDesignComponentPrompt creates the /design component scaffolding prompt.
func buildDesignComponentPrompt(name, framework string) string {
	fw := framework
	if fw == "" {
		fw = "the project's framework"
	}
	return fmt.Sprintf(`You are in DESIGN COMPONENT mode. Scaffold a production-quality UI component.

## Component
%s

## Framework
%s

## Requirements
1. Read existing components in the project to match the style, patterns, and conventions.
2. Generate a complete component with:
   - TypeScript/PropTypes type definitions for all props
   - All visual variants (primary, secondary, outline, ghost, danger — as applicable)
   - All interactive states: default, hover, active, focus, disabled, loading
   - All sizes (sm, md, lg) where applicable
   - Error state, empty state (if content-bearing)
   - Responsive behavior where applicable
   - Dark mode support (if project has dark mode)
3. Accessibility (non-negotiable):
   - Semantic HTML elements
   - ARIA roles, labels, and attributes
   - Keyboard navigation (Enter, Escape, arrow keys for menus/dialogs)
   - Focus management (auto-focus, focus trap for modals)
   - Screen reader announcements via aria-live
4. Write tests:
   - Renders correctly with default props
   - Each variant renders without error
   - Each interactive state works
   - Keyboard interactions function
   - Accessibility assertions (jest-axe or equivalent)
5. Documentation: Add a brief usage example as a doc comment.

Rules: Follow the project's existing patterns exactly (CSS modules, Tailwind, styled-components, etc.). Do not add new dependencies unless absolutely necessary.`, name, fw)
}

func buildDesignRegressionPrompt(baseline, current string) string {
	return fmt.Sprintf(`You are in DESIGN REGRESSION mode. Compare current UI against a stored baseline and flag visual differences.

## Baseline reference
%s

## Current version
%s

## Process
1. Capture current state: Take a screenshot of the relevant page or component.
2. Compare against the baseline on these dimensions:
   - Layout shifts: Are elements in the same position?
   - Spacing changes: Are margins/padding consistent with baseline?
   - Color changes: Any unexpected color shifts — check contrast ratios still pass WCAG AA
   - Typography changes: Font sizes, weights, families consistent?
   - Content changes: Missing text, extra elements, different copy?
   - Responsive: Test at mobile, tablet, desktop breakpoints
3. Classify each difference:
   - Intentional: matches a recent change in git log
   - Regression: unintended visual change — needs fix
   - Unknown: cannot determine cause — flag for human review
4. Report: For each regression, provide:
   - The component and file location
   - Screenshot comparison description
   - Git commit that likely caused it (use git blame on relevant files)
   - Suggested fix

Begin by reading the current UI components and comparing against the baseline description.`, baseline, current)
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
