package engine

import (
	"fmt"
	"strings"
	"time"
)

// Spec is a frozen specification that defines what to build.
type Spec struct {
	Title       string
	Goal        string
	Files       []string
	Criteria    []string // acceptance criteria
	Assumptions []string
	OutOfScope  []string
	CreatedAt   time.Time
	Approved    bool
}

// SpecGeneratePrompt creates the prompt to generate a spec from user intent.
func SpecGeneratePrompt(intent string) string {
	return fmt.Sprintf(`Generate a frozen specification for this task. Be precise and complete.

USER INTENT: %s

Respond in this EXACT format:

## Spec: <title>
**Goal:** <one sentence>
**Files affected:** <comma-separated list>
**Acceptance criteria:**
1. <criterion>
2. <criterion>
3. <criterion>
**Assumptions:**
- <assumption>
**NOT in scope:** <what this does NOT include>

Rules:
- Be specific, not vague
- Each acceptance criterion must be testable
- List ALL assumptions (don't hide them)
- Explicitly state what's out of scope`, intent)
}

// FormatSpec renders a spec for display.
func (s *Spec) Format() string {
	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("## Spec: %s\n", s.Title))
	sb.WriteString(fmt.Sprintf("**Goal:** %s\n", s.Goal))
	sb.WriteString(fmt.Sprintf("**Files:** %s\n", strings.Join(s.Files, ", ")))
	sb.WriteString("**Acceptance criteria:**\n")
	for i, c := range s.Criteria {
		sb.WriteString(fmt.Sprintf("  %d. %s\n", i+1, c))
	}
	if len(s.Assumptions) > 0 {
		sb.WriteString("**Assumptions:**\n")
		for _, a := range s.Assumptions {
			sb.WriteString("  - " + a + "\n")
		}
	}
	if len(s.OutOfScope) > 0 {
		sb.WriteString("**NOT in scope:** " + strings.Join(s.OutOfScope, ", ") + "\n")
	}
	status := "⏳ PENDING APPROVAL"
	if s.Approved {
		status = "✅ APPROVED"
	}
	sb.WriteString("\n" + status)
	return sb.String()
}

// ImplementFromSpecPrompt generates the implementation prompt constrained by the spec.
func ImplementFromSpecPrompt(spec *Spec) string {
	return fmt.Sprintf(`Implement the following specification. Do NOT deviate from it.

%s

RULES:
- Only modify files listed in the spec
- Every acceptance criterion must be satisfied
- Verify each assumption before proceeding
- If an assumption is wrong, STOP and report it
- Do NOT add features not in the spec`, spec.Format())
}
