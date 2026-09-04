package compact

import (
	"fmt"
	"strings"

	"github.com/GrayCodeAI/graycode-cli/internal/types"
)

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

// incrementalUpdateTemplate instructs the model to merge the new conversation
// into a prior summary rather than re-summarizing from scratch. This preserves
// previously-captured context and avoids the cost of re-deriving it, while
// folding in only the progress made since the last compaction.
const incrementalUpdateTemplate = `A previous summary of this conversation exists (shown below in <previous-summary> tags).

Update that summary in place to reflect the NEW messages that were added after it. Do NOT re-derive facts already captured. Follow these rules:

1. Preserve the existing sections and their structure EXACTLY (## Goal, ## Constraints & Preferences, ## Progress, ## Files Modified, ## Key Decisions, ## Errors & Fixes, ## User Instructions, ## Next Step).
2. Update each section only where the new messages add information:
   - ## Goal: keep unless the user redefined the goal.
   - ## Progress: add new Done/In Progress/Blocked entries; keep existing ones.
   - ## Files Modified: add any newly read/created/modified files.
   - ## Key Decisions: add new decisions; keep prior ones.
   - ## Errors & Fixes: add newly encountered errors; keep prior ones.
   - ## User Instructions (verbatim): append any new non-trivial user directions.
   - ## Next Step: replace with the most recent next step.
3. If the new messages add no information to a section, leave it unchanged (do not empty it).

Provide the fully updated summary inside <summary> tags using EXACTLY this structure. Keep section order unchanged.

<previous-summary>
%s
</previous-summary>`

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

// BuildIncrementalCompactPrompt builds a prompt that merges the new
// conversation into an existing prior summary instead of re-summarizing from
// scratch. priorSummary is the previously generated structured summary.
func BuildIncrementalCompactPrompt(priorSummary string) string {
	if priorSummary == "" {
		return BuildCompactPrompt(CompactBase)
	}
	return noToolsPreamble + detailedAnalysisPartial + "\n\n" +
		fmt.Sprintf(incrementalUpdateTemplate, priorSummary)
}

// PriorSummaryPrefix is the marker prefix graycode prepends to a persisted
// conversation summary message.
const PriorSummaryPrefix = "[Conversation summary]"

// ExtractPriorSummary extracts the previously generated summary text from the
// first message of a conversation if one was persisted by an earlier
// compaction. It returns "" when no prior summary is present.
func ExtractPriorSummary(msgs []types.EyrieMessage) string {
	for _, m := range msgs {
		if m.Role != "user" {
			continue
		}
		if strings.HasPrefix(m.Content, PriorSummaryPrefix) {
			body := strings.TrimPrefix(m.Content, PriorSummaryPrefix)
			body = strings.TrimSpace(body)
			// Stop at the continuation marker that follows the summary body.
			if idx := strings.Index(body, "[Continue from the recent messages below.]"); idx >= 0 {
				body = body[:idx]
			}
			return strings.TrimSpace(body)
		}
	}
	return ""
}

type CompactVariant int

const (
	CompactBase CompactVariant = iota
	CompactPartial
	CompactUpTo
)

func FormatCompactSummary(raw string) string {
	start := indexOf(raw, "<analysis>")
	end := indexOf(raw, "</analysis>")
	if start >= 0 && end > start {
		raw = raw[:start] + raw[end+len("</analysis>"):]
	}

	sumStart := indexOf(raw, "<summary>")
	sumEnd := indexOf(raw, "</summary>")
	if sumStart >= 0 && sumEnd > sumStart {
		return raw[sumStart+len("<summary>") : sumEnd]
	}

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
