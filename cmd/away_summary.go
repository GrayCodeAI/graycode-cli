package cmd

import (
	"fmt"
	"strings"
	"time"
)

const awayThreshold = 5 * time.Minute
const awayMinMessages = 5

// awaySummary generates a 1-3 sentence recap of session context when the user
// returns after inactivity. Returns empty string if idle time is less than 5
// minutes or there are fewer than 5 messages.
func awaySummary(messages []displayMsg, lastActivity time.Time) string {
	idle := time.Since(lastActivity)
	if idle < awayThreshold {
		return ""
	}
	if len(messages) < awayMinMessages {
		return ""
	}

	// Collect the most recent user and assistant messages for a recap.
	var userTopics []string
	var assistantSummaries []string
	var toolsUsed []string

	// Look at the last 10 messages for context
	start := 0
	if len(messages) > 10 {
		start = len(messages) - 10
	}

	for _, msg := range messages[start:] {
		switch msg.role {
		case "user":
			if len(msg.content) > 80 {
				userTopics = append(userTopics, msg.content[:80]+"...")
			} else {
				userTopics = append(userTopics, msg.content)
			}
		case "assistant":
			if len(msg.content) > 100 {
				assistantSummaries = append(assistantSummaries, msg.content[:100]+"...")
			} else {
				assistantSummaries = append(assistantSummaries, msg.content)
			}
		case "tool_use":
			toolsUsed = append(toolsUsed, msg.content)
		}
	}

	var parts []string

	idleMin := int(idle.Minutes())
	parts = append(parts, fmt.Sprintf("You were away for %d minutes.", idleMin))

	if len(userTopics) > 0 {
		last := userTopics[len(userTopics)-1]
		parts = append(parts, fmt.Sprintf("Last topic: %q.", last))
	}

	if len(toolsUsed) > 0 {
		unique := uniqueStrings(toolsUsed)
		if len(unique) > 3 {
			unique = unique[:3]
		}
		parts = append(parts, fmt.Sprintf("Tools used: %s.", strings.Join(unique, ", ")))
	}

	return strings.Join(parts, " ")
}

func uniqueStrings(ss []string) []string {
	seen := make(map[string]bool)
	var out []string
	for _, s := range ss {
		if !seen[s] {
			seen[s] = true
			out = append(out, s)
		}
	}
	return out
}
