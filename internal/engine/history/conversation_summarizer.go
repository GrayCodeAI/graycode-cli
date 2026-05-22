package history

import (
	"fmt"
	"regexp"
	"sort"
	"strings"
	"sync"
)

// SummaryLevel defines the granularity of a conversation summary.
type SummaryLevel string

const (
	// SummaryOneLine produces a single concise sentence.
	SummaryOneLine SummaryLevel = "one_line"
	// SummaryParagraph produces a 3-5 sentence overview.
	SummaryParagraph SummaryLevel = "paragraph"
	// SummaryDetailed produces a full multi-section summary.
	SummaryDetailed SummaryLevel = "detailed"
	// SummaryStructured populates all fields of a Summary struct.
	SummaryStructured SummaryLevel = "structured"
)

// SumMessage represents a single message in the conversation for summarization.
type SumMessage struct {
	Role     string
	Content  string
	ToolName string
	IsError  bool
}

// Summary holds a structured summary of a conversation at the requested level.
type Summary struct {
	Level          string
	Content        string
	Topics         []string
	Decisions      []string
	FilesDiscussed []string
	ToolsUsed      map[string]int
	TokensSaved    int
}

// ConversationSummarizer produces concise summaries of conversation messages
// at different granularities for compaction, session titles, and cross-session context.
type ConversationSummarizer struct {
	mu sync.Mutex
}

// NewConversationSummarizer creates a new ConversationSummarizer.
func NewConversationSummarizer() *ConversationSummarizer {
	return &ConversationSummarizer{}
}

// Summarize produces a Summary at the requested level from the given messages.
func (cs *ConversationSummarizer) Summarize(messages []SumMessage, level SummaryLevel) *Summary {
	cs.mu.Lock()
	defer cs.mu.Unlock()

	switch level {
	case SummaryOneLine:
		return &Summary{
			Level:   string(SummaryOneLine),
			Content: cs.oneLineUnlocked(messages),
		}
	case SummaryParagraph:
		return &Summary{
			Level:   string(SummaryParagraph),
			Content: cs.paragraphUnlocked(messages),
			Topics:  extractTopics(messages),
		}
	case SummaryDetailed:
		return &Summary{
			Level:          string(SummaryDetailed),
			Content:        cs.detailedUnlocked(messages),
			Topics:         extractTopics(messages),
			Decisions:      extractDecisions(messages),
			FilesDiscussed: extractFilesDiscussed(messages),
			ToolsUsed:      extractToolsUsed(messages),
		}
	case SummaryStructured:
		return cs.structuredUnlocked(messages)
	default:
		return &Summary{
			Level:   string(level),
			Content: cs.oneLineUnlocked(messages),
		}
	}
}

// OneLine produces a single concise sentence summarizing the conversation.
func (cs *ConversationSummarizer) OneLine(messages []SumMessage) string {
	cs.mu.Lock()
	defer cs.mu.Unlock()
	return cs.oneLineUnlocked(messages)
}

func (cs *ConversationSummarizer) oneLineUnlocked(messages []SumMessage) string {
	if len(messages) == 0 {
		return "Empty conversation"
	}

	topics := extractTopics(messages)
	files := extractFilesDiscussed(messages)
	tools := extractToolsUsed(messages)
	errors := countErrors(messages)

	// Build a concise one-liner
	var parts []string

	if len(topics) > 0 {
		limit := 3
		if len(topics) < limit {
			limit = len(topics)
		}
		action := inferAction(messages)
		parts = append(parts, action+" "+strings.Join(topics[:limit], ", "))
	}

	if errors > 0 {
		parts = append(parts, fmt.Sprintf("fixed %d bug%s", errors, pluralS(errors)))
	}

	toolCount := 0
	for _, c := range tools {
		toolCount += c
	}

	if len(files) > 0 {
		parts = append(parts, fmt.Sprintf("%d file%s modified", len(files), pluralS(len(files))))
	}

	if len(parts) == 0 {
		return fmt.Sprintf("Conversation with %d messages", len(messages))
	}

	return strings.Join(parts, ", ")
}

// Paragraph produces a 3-5 sentence summary covering what happened.
func (cs *ConversationSummarizer) Paragraph(messages []SumMessage) string {
	cs.mu.Lock()
	defer cs.mu.Unlock()
	return cs.paragraphUnlocked(messages)
}

func (cs *ConversationSummarizer) paragraphUnlocked(messages []SumMessage) string {
	if len(messages) == 0 {
		return "No messages to summarize."
	}

	topics := extractTopics(messages)
	decisions := extractDecisions(messages)
	files := extractFilesDiscussed(messages)
	errors := countErrors(messages)

	var sentences []string

	// Opening sentence: what was the conversation about
	if len(topics) > 0 {
		sentences = append(sentences, fmt.Sprintf("The conversation focused on %s.", strings.Join(topics, ", ")))
	} else {
		sentences = append(sentences, fmt.Sprintf("A conversation of %d messages took place.", len(messages)))
	}

	// Files touched
	if len(files) > 0 {
		limit := 5
		if len(files) < limit {
			limit = len(files)
		}
		sentences = append(sentences, fmt.Sprintf("Files discussed: %s.", strings.Join(files[:limit], ", ")))
	}

	// Decisions
	if len(decisions) > 0 {
		limit := 2
		if len(decisions) < limit {
			limit = len(decisions)
		}
		sentences = append(sentences, fmt.Sprintf("Key decisions: %s.", strings.Join(decisions[:limit], "; ")))
	}

	// Errors
	if errors > 0 {
		sentences = append(sentences, fmt.Sprintf("%d error%s encountered and addressed.", errors, pluralS(errors)))
	}

	// Closing sentence
	userMsgs := countRole(messages, "user")
	assistantMsgs := countRole(messages, "assistant")
	sentences = append(sentences, fmt.Sprintf("The exchange involved %d user and %d assistant messages.", userMsgs, assistantMsgs))

	return strings.Join(sentences, " ")
}

// Detailed produces a full multi-section summary.
func (cs *ConversationSummarizer) Detailed(messages []SumMessage) string {
	cs.mu.Lock()
	defer cs.mu.Unlock()
	return cs.detailedUnlocked(messages)
}

func (cs *ConversationSummarizer) detailedUnlocked(messages []SumMessage) string {
	if len(messages) == 0 {
		return "No messages to summarize."
	}

	topics := extractTopics(messages)
	decisions := extractDecisions(messages)
	files := extractFilesDiscussed(messages)
	tools := extractToolsUsed(messages)
	errors := countErrors(messages)

	var sb strings.Builder

	sb.WriteString("## Overview\n")
	if len(topics) > 0 {
		sb.WriteString(fmt.Sprintf("Topics: %s\n", strings.Join(topics, ", ")))
	}
	sb.WriteString(fmt.Sprintf("Messages: %d total (%d user, %d assistant)\n",
		len(messages), countRole(messages, "user"), countRole(messages, "assistant")))
	sb.WriteString("\n")

	if len(files) > 0 {
		sb.WriteString("## Files Discussed\n")
		for _, f := range files {
			sb.WriteString(fmt.Sprintf("- %s\n", f))
		}
		sb.WriteString("\n")
	}

	if len(decisions) > 0 {
		sb.WriteString("## Decisions\n")
		for _, d := range decisions {
			sb.WriteString(fmt.Sprintf("- %s\n", d))
		}
		sb.WriteString("\n")
	}

	if len(tools) > 0 {
		sb.WriteString("## Tools Used\n")
		for name, count := range tools {
			sb.WriteString(fmt.Sprintf("- %s: %d call%s\n", name, count, pluralS(count)))
		}
		sb.WriteString("\n")
	}

	if errors > 0 {
		sb.WriteString("## Errors\n")
		sb.WriteString(fmt.Sprintf("%d error%s encountered during the conversation.\n", errors, pluralS(errors)))
		sb.WriteString("\n")
	}

	return strings.TrimSpace(sb.String())
}

// Structured produces a Summary with all fields populated.
func (cs *ConversationSummarizer) Structured(messages []SumMessage) *Summary {
	cs.mu.Lock()
	defer cs.mu.Unlock()
	return cs.structuredUnlocked(messages)
}

func (cs *ConversationSummarizer) structuredUnlocked(messages []SumMessage) *Summary {
	topics := extractTopics(messages)
	decisions := extractDecisions(messages)
	files := extractFilesDiscussed(messages)
	tools := extractToolsUsed(messages)

	// Estimate token savings: original token count approximation minus summary tokens
	originalTokens := 0
	for _, m := range messages {
		originalTokens += summarizerEstimateTokens(m.Content)
	}
	summaryContent := cs.oneLineUnlocked(messages)
	savedTokens := originalTokens - summarizerEstimateTokens(summaryContent)
	if savedTokens < 0 {
		savedTokens = 0
	}

	return &Summary{
		Level:          string(SummaryStructured),
		Content:        summaryContent,
		Topics:         topics,
		Decisions:      decisions,
		FilesDiscussed: files,
		ToolsUsed:      tools,
		TokensSaved:    savedTokens,
	}
}

// ExtractTopics identifies what was discussed in the conversation.
func (cs *ConversationSummarizer) ExtractTopics(messages []SumMessage) []string {
	cs.mu.Lock()
	defer cs.mu.Unlock()
	return extractTopics(messages)
}

// ExtractDecisions identifies key decisions made during the conversation.
func (cs *ConversationSummarizer) ExtractDecisions(messages []SumMessage) []string {
	cs.mu.Lock()
	defer cs.mu.Unlock()
	return extractDecisions(messages)
}

// ExtractFilesDiscussed identifies files mentioned or modified in the conversation.
func (cs *ConversationSummarizer) ExtractFilesDiscussed(messages []SumMessage) []string {
	cs.mu.Lock()
	defer cs.mu.Unlock()
	return extractFilesDiscussed(messages)
}

// GenerateTitle produces a short title suitable for labeling a session.
func (cs *ConversationSummarizer) GenerateTitle(messages []SumMessage) string {
	cs.mu.Lock()
	defer cs.mu.Unlock()

	if len(messages) == 0 {
		return "Empty Session"
	}

	topics := extractTopics(messages)
	if len(topics) == 0 {
		// Fall back to first user message
		for _, m := range messages {
			if m.Role == "user" && m.Content != "" {
				title := firstSentence(m.Content)
				if len(title) > 60 {
					title = title[:57] + "..."
				}
				return title
			}
		}
		return "Conversation Session"
	}

	// Combine top topics into a title
	action := inferAction(messages)
	limit := 2
	if len(topics) < limit {
		limit = len(topics)
	}

	title := capitalize(action) + " " + strings.Join(topics[:limit], " & ")

	errors := countErrors(messages)
	if errors > 0 && !strings.Contains(strings.ToLower(title), "fix") {
		title += " & Bug Fixes"
	}

	if len(title) > 70 {
		title = title[:67] + "..."
	}

	return title
}

// FormatSummary formats a Summary into a human-readable string.
func (cs *ConversationSummarizer) FormatSummary(summary *Summary) string {
	if summary == nil {
		return ""
	}

	var sb strings.Builder

	sb.WriteString(fmt.Sprintf("[%s] %s\n", summary.Level, summary.Content))

	if len(summary.Topics) > 0 {
		sb.WriteString(fmt.Sprintf("Topics: %s\n", strings.Join(summary.Topics, ", ")))
	}

	if len(summary.Decisions) > 0 {
		sb.WriteString(fmt.Sprintf("Decisions: %s\n", strings.Join(summary.Decisions, "; ")))
	}

	if len(summary.FilesDiscussed) > 0 {
		sb.WriteString(fmt.Sprintf("Files: %s\n", strings.Join(summary.FilesDiscussed, ", ")))
	}

	if len(summary.ToolsUsed) > 0 {
		var toolStrs []string
		for name, count := range summary.ToolsUsed {
			toolStrs = append(toolStrs, fmt.Sprintf("%s(%d)", name, count))
		}
		sort.Strings(toolStrs)
		sb.WriteString(fmt.Sprintf("Tools: %s\n", strings.Join(toolStrs, ", ")))
	}

	if summary.TokensSaved > 0 {
		sb.WriteString(fmt.Sprintf("Tokens saved: %d\n", summary.TokensSaved))
	}

	return strings.TrimSpace(sb.String())
}

// CompareMessages describes what changed between two message states.
func (cs *ConversationSummarizer) CompareMessages(before, after []SumMessage) string {
	cs.mu.Lock()
	defer cs.mu.Unlock()

	if len(before) == 0 && len(after) == 0 {
		return "No messages in either state."
	}

	if len(before) == 0 {
		return fmt.Sprintf("New conversation started with %d messages.", len(after))
	}

	if len(after) == 0 {
		return "Conversation was cleared."
	}

	var changes []string

	// Message count change
	diff := len(after) - len(before)
	if diff > 0 {
		changes = append(changes, fmt.Sprintf("%d new message%s added", diff, pluralS(diff)))
	} else if diff < 0 {
		changes = append(changes, fmt.Sprintf("%d message%s removed (compaction)", -diff, pluralS(-diff)))
	}

	// Topic changes
	topicsBefore := extractTopics(before)
	topicsAfter := extractTopics(after)
	newTopics := diffStrings(topicsBefore, topicsAfter)
	if len(newTopics) > 0 {
		changes = append(changes, fmt.Sprintf("new topics: %s", strings.Join(newTopics, ", ")))
	}

	// File changes
	filesBefore := extractFilesDiscussed(before)
	filesAfter := extractFilesDiscussed(after)
	newFiles := diffStrings(filesBefore, filesAfter)
	if len(newFiles) > 0 {
		changes = append(changes, fmt.Sprintf("new files discussed: %s", strings.Join(newFiles, ", ")))
	}

	// Error changes
	errorsBefore := countErrors(before)
	errorsAfter := countErrors(after)
	if errorsAfter > errorsBefore {
		changes = append(changes, fmt.Sprintf("%d new error%s", errorsAfter-errorsBefore, pluralS(errorsAfter-errorsBefore)))
	}

	if len(changes) == 0 {
		return "No significant changes detected."
	}

	return strings.Join(changes, "; ") + "."
}

// --- internal helpers ---

// filePattern matches common file paths in conversation content.
var filePattern = regexp.MustCompile(`(?:^|\s|` + "`" + `)([a-zA-Z0-9_\-./]+\.[a-zA-Z]{1,10})(?:\s|$|` + "`" + `|:|\))`)

// decisionPatterns match language indicating a decision was made.
var decisionPatterns = []*regexp.Regexp{
	regexp.MustCompile(`(?i)(?:let'?s|we should|I'?ll|going to|decided to|will use|chose|choosing|switched to)\s+(.+?)(?:\.|$)`),
	regexp.MustCompile(`(?i)(?:use|using|prefer|adopt)\s+(\S+)\s+(?:instead of|over|rather than)\s+(\S+)`),
	regexp.MustCompile(`(?i)(?:add|adding|implement|implementing)\s+(.+?)(?:\.|$)`),
}

// topicKeywords maps keywords found in messages to normalized topic names.
var topicKeywords = map[string]string{
	"auth":          "authentication",
	"jwt":           "JWT auth",
	"oauth":         "OAuth",
	"test":          "testing",
	"tests":         "testing",
	"testing":       "testing",
	"config":        "configuration",
	"configuration": "configuration",
	"database":      "database",
	"db":            "database",
	"sql":           "database",
	"api":           "API",
	"endpoint":      "API",
	"rest":          "API",
	"docker":        "Docker",
	"container":     "Docker",
	"deploy":        "deployment",
	"deployment":    "deployment",
	"ci":            "CI/CD",
	"cd":            "CI/CD",
	"pipeline":      "CI/CD",
	"refactor":      "refactoring",
	"refactoring":   "refactoring",
	"bug":           "bug fixing",
	"fix":           "bug fixing",
	"debug":         "debugging",
	"performance":   "performance",
	"cache":         "caching",
	"caching":       "caching",
	"security":      "security",
	"lint":          "linting",
	"format":        "formatting",
	"migration":     "migration",
	"logging":       "logging",
	"error":         "error handling",
	"middleware":    "middleware",
	"route":         "routing",
	"routing":       "routing",
}

func extractTopics(messages []SumMessage) []string {
	counts := make(map[string]int)

	for _, m := range messages {
		lower := strings.ToLower(m.Content)
		words := strings.Fields(lower)
		seen := make(map[string]bool)
		for _, w := range words {
			// Clean punctuation
			w = strings.TrimRight(w, ".,;:!?\"'`()[]{}#")
			w = strings.TrimLeft(w, "\"'`()[]{}#")
			if topic, ok := topicKeywords[w]; ok && !seen[topic] {
				counts[topic]++
				seen[topic] = true
			}
		}
	}

	if len(counts) == 0 {
		return nil
	}

	// Sort by frequency descending
	type topicCount struct {
		topic string
		count int
	}
	var sorted []topicCount
	for t, c := range counts {
		sorted = append(sorted, topicCount{t, c})
	}
	sort.Slice(sorted, func(i, j int) bool {
		if sorted[i].count == sorted[j].count {
			return sorted[i].topic < sorted[j].topic
		}
		return sorted[i].count > sorted[j].count
	})

	// Return top topics (up to 5)
	limit := 5
	if len(sorted) < limit {
		limit = len(sorted)
	}
	result := make([]string, limit)
	for i := 0; i < limit; i++ {
		result[i] = sorted[i].topic
	}
	return result
}

func extractDecisions(messages []SumMessage) []string {
	var decisions []string
	seen := make(map[string]bool)

	for _, m := range messages {
		if m.Role != "assistant" && m.Role != "user" {
			continue
		}
		for _, pat := range decisionPatterns {
			matches := pat.FindAllStringSubmatch(m.Content, -1)
			for _, match := range matches {
				if len(match) >= 2 {
					decision := strings.TrimSpace(match[0])
					// Normalize and deduplicate
					key := strings.ToLower(decision)
					if len(key) < 10 || len(key) > 200 {
						continue
					}
					if !seen[key] {
						seen[key] = true
						decisions = append(decisions, decision)
					}
				}
			}
		}
	}

	// Limit to top 10
	if len(decisions) > 10 {
		decisions = decisions[:10]
	}
	return decisions
}

func extractFilesDiscussed(messages []SumMessage) []string {
	seen := make(map[string]bool)
	var files []string

	for _, m := range messages {
		matches := filePattern.FindAllStringSubmatch(m.Content, -1)
		for _, match := range matches {
			if len(match) >= 2 {
				file := match[1]
				// Filter out common false positives
				if isLikelyFile(file) && !seen[file] {
					seen[file] = true
					files = append(files, file)
				}
			}
		}
	}

	return files
}

func extractToolsUsed(messages []SumMessage) map[string]int {
	tools := make(map[string]int)
	for _, m := range messages {
		if m.ToolName != "" {
			tools[m.ToolName]++
		}
	}
	return tools
}

func countErrors(messages []SumMessage) int {
	count := 0
	for _, m := range messages {
		if m.IsError {
			count++
		}
	}
	return count
}

func countRole(messages []SumMessage, role string) int {
	count := 0
	for _, m := range messages {
		if m.Role == role {
			count++
		}
	}
	return count
}

func inferAction(messages []SumMessage) string {
	// Look at what the user asked for in early messages
	actionCounts := map[string]int{
		"Implemented": 0,
		"Fixed":       0,
		"Refactored":  0,
		"Discussed":   0,
		"Configured":  0,
		"Added":       0,
	}

	for _, m := range messages {
		lower := strings.ToLower(m.Content)
		if strings.Contains(lower, "implement") || strings.Contains(lower, "create") || strings.Contains(lower, "build") {
			actionCounts["Implemented"]++
		}
		if strings.Contains(lower, "fix") || strings.Contains(lower, "bug") || strings.Contains(lower, "error") {
			actionCounts["Fixed"]++
		}
		if strings.Contains(lower, "refactor") || strings.Contains(lower, "clean") || strings.Contains(lower, "reorganize") {
			actionCounts["Refactored"]++
		}
		if strings.Contains(lower, "config") || strings.Contains(lower, "setup") || strings.Contains(lower, "install") {
			actionCounts["Configured"]++
		}
		if strings.Contains(lower, "add") || strings.Contains(lower, "new") {
			actionCounts["Added"]++
		}
	}

	// Pick the most common action
	best := "Discussed"
	bestCount := 0
	for action, count := range actionCounts {
		if count > bestCount {
			best = action
			bestCount = count
		}
	}
	return best
}

func isLikelyFile(s string) bool {
	// Must have a dot-separated extension
	parts := strings.Split(s, ".")
	if len(parts) < 2 {
		return false
	}
	ext := parts[len(parts)-1]
	// Common file extensions
	validExts := map[string]bool{
		"go": true, "py": true, "js": true, "ts": true, "tsx": true, "jsx": true,
		"java": true, "rb": true, "rs": true, "c": true, "h": true, "cpp": true,
		"yaml": true, "yml": true, "json": true, "toml": true, "xml": true,
		"md": true, "txt": true, "sql": true, "sh": true, "bash": true,
		"html": true, "css": true, "scss": true, "less": true,
		"proto": true, "graphql": true, "mod": true, "sum": true,
		"conf": true, "cfg": true, "ini": true, "env": true,
		"dockerfile": true, "lock": true,
	}
	return validExts[strings.ToLower(ext)]
}

func firstSentence(s string) string {
	s = strings.TrimSpace(s)
	// Take up to first newline
	if idx := strings.IndexByte(s, '\n'); idx >= 0 {
		s = s[:idx]
	}
	// Take up to first period followed by space
	if idx := strings.Index(s, ". "); idx >= 0 {
		s = s[:idx]
	}
	return strings.TrimSpace(s)
}

func capitalize(s string) string {
	if s == "" {
		return s
	}
	return strings.ToUpper(s[:1]) + s[1:]
}

func pluralS(n int) string {
	if n == 1 {
		return ""
	}
	return "s"
}

func summarizerEstimateTokens(s string) int {
	// Rough approximation: ~4 characters per token
	return len(s) / 4
}

func diffStrings(before, after []string) []string {
	set := make(map[string]bool)
	for _, s := range before {
		set[s] = true
	}
	var diff []string
	for _, s := range after {
		if !set[s] {
			diff = append(diff, s)
		}
	}
	return diff
}
