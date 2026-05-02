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

const summaryTemplate = `Now provide your summary inside <summary> tags with the following sections:

1. **Primary Request & Intent**: What is the user trying to accomplish? What's the goal?

2. **Key Technical Concepts**: Important architectural decisions, patterns, or domain concepts discussed.

3. **Files & Code**: List all files that were read, created, or modified. Include relevant code snippets, function signatures, and the rationale for changes.

4. **Errors & Fixes**: Any errors encountered, their root causes, and how they were resolved.

5. **Problem Solving**: Approaches that worked, approaches that didn't, and why.

6. **All User Messages**: Reproduce ALL non-tool-result user messages verbatim. These contain instructions and feedback critical for continuity.

7. **Pending Tasks**: Anything the user asked for that hasn't been completed yet.

8. **Current Work**: What was being worked on most recently? Include specific details.

9. **Lookup Hints**: List 3-5 specific topics or keywords the agent should search for in conversation history if it needs details that were summarized away. Format as a bullet list of search queries.

10. **Next Step**: Based on the most recent user messages, what should happen next? Include direct quotes from the user if they gave specific direction.`

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
