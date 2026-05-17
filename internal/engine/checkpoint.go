package engine

// CheckpointPhase represents a step in the pre-commit checkpoint review.
type CheckpointPhase int

const (
	CheckpointOrientation CheckpointPhase = iota // what changed and why
	CheckpointWalkthrough                        // file-by-file
	CheckpointDetail                             // edge cases, error paths
	CheckpointTesting                            // test adequacy
	CheckpointWrapup                             // ready to commit?
)

// CheckpointPrompts returns the system prompt for each checkpoint phase.
func CheckpointPrompts(phase CheckpointPhase, files []string) string {
	switch phase {
	case CheckpointOrientation:
		return `Review the recent changes. Answer:
1. What was the goal?
2. What files were modified?
3. Is the scope appropriate (no unrelated changes)?`

	case CheckpointWalkthrough:
		return `Walk through each changed file:
- What was the change?
- Does it make sense in context?
- Any obvious issues?
Files: ` + joinFiles(files)

	case CheckpointDetail:
		return `Deep inspection — look for:
- Edge cases not handled (nil, empty, overflow)
- Error paths that could panic or leak
- Concurrency issues (races, deadlocks)
- Boundary conditions`

	case CheckpointTesting:
		return `Evaluate test coverage:
- Are the changes tested?
- Are edge cases covered?
- Would you trust this to not break in production?
- What test would you add?`

	case CheckpointWrapup:
		return `Final verdict:
- READY TO COMMIT — no blocking issues
- NEEDS WORK — list what must be fixed first
- NEEDS DISCUSSION — architectural concerns to resolve`

	default:
		return ""
	}
}

func joinFiles(files []string) string {
	if len(files) == 0 {
		return "(no files specified)"
	}
	result := ""
	for _, f := range files {
		result += "\n- " + f
	}
	return result
}
