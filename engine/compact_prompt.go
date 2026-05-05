package engine

// CompactPrompt provides the system and user prompts used during LLM-based compaction.
// Ported from hawk-archive src/services/compact/prompt.ts.

const noToolsPreamble = `CRITICAL: Respond with TEXT ONLY. Do NOT call any tools.

- Do NOT use Read, Bash, Grep, Glob, Edit, Write, or ANY other tool.
- You already have all the context you need in the conversation above.
- Tool calls will be REJECTED and will waste your only turn — you will fail the task.
- Your entire response must be plain text: an <analysis> block followed by a <summary> block.

`

const detailedAnalysisBase = `Before providing your final summary, wrap your analysis in <analysis> tags to organize your thoughts and ensure you've covered all necessary points. In your analysis process:

1. Chronologically analyze each message and section of the conversation. For each section thoroughly identify:
   - The user's explicit requests and intents
   - Your approach to addressing the user's requests
   - Key decisions, technical concepts and code patterns
   - Specific details like:
     - file names
     - full code snippets
     - function signatures
     - file edits
   - Errors that you ran into and how you fixed them
   - Pay special attention to specific user feedback that you received, especially if the user told you to do something differently.
2. Double-check for technical accuracy and completeness, addressing each required element thoroughly.`

const detailedAnalysisPartial = `Before providing your final summary, wrap your analysis in <analysis> tags to organize your thoughts and ensure you've covered all necessary points. In your analysis process:

1. Analyze the recent messages chronologically. For each section thoroughly identify:
   - The user's explicit requests and intents
   - Your approach to addressing the user's requests
   - Key decisions, technical concepts and code patterns
   - Files and code that were viewed or modified
   - Errors encountered and how they were resolved
2. Double-check for technical accuracy and completeness.`

const summaryTemplate = `Now provide your summary inside <summary> tags using EXACTLY this structure. Keep section order unchanged.

## Goal
- [single-sentence task summary — what the user is trying to accomplish]

## Constraints & Preferences
- [user constraints, coding style preferences, specific instructions, or "(none)"]

## Progress
### Done
- [completed work with file paths and brief description]

### In Progress
- [current work — what was being worked on most recently with specific details]

### Blocked
- [blockers, errors not yet resolved, or "(none)"]

## Files Modified
- [list every file read/created/modified with one-line description of change]

## Key Decisions
- [architectural decisions, patterns chosen, approaches rejected and why]

## Errors & Fixes
- [errors encountered, root causes, resolutions — or "(none)"]

## User Instructions (verbatim)
- [reproduce key non-trivial user messages/feedback that affect future work]

## Next Step
- [based on most recent user messages, what should happen next — include direct quotes if user gave specific direction]`

// BuildCompactPrompt constructs the full compaction prompt for LLM-based summarization.
func BuildCompactPrompt(variant CompactVariant) string {
	var analysis string
	switch variant {
	case CompactPartial:
		analysis = detailedAnalysisPartial
	default:
		analysis = detailedAnalysisBase
	}
	return noToolsPreamble + analysis + "\n\n" + summaryTemplate
}

// CompactVariant determines which compaction prompt style to use.
type CompactVariant int

const (
	CompactBase    CompactVariant = iota // Full conversation
	CompactPartial                       // Recent messages only
	CompactUpTo                          // Prefix summarization
)

// FormatCompactSummary strips the <analysis> drafting block and extracts the <summary> content.
func FormatCompactSummary(raw string) string {
	// Strip <analysis>...</analysis> block
	start := indexOf(raw, "<analysis>")
	end := indexOf(raw, "</analysis>")
	if start >= 0 && end > start {
		raw = raw[:start] + raw[end+len("</analysis>"):]
	}

	// Extract <summary>...</summary> content
	sumStart := indexOf(raw, "<summary>")
	sumEnd := indexOf(raw, "</summary>")
	if sumStart >= 0 && sumEnd > sumStart {
		return raw[sumStart+len("<summary>") : sumEnd]
	}

	// If no tags, return as-is (fallback)
	return raw
}

func indexOf(s, substr string) int {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return i
		}
	}
	return -1
}
