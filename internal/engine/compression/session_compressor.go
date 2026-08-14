package compression

import (
	"fmt"
	"strings"
	"sync"
)

// CompressStrategy defines the compression approach to use.
type CompressStrategy string

const (
	// StrategySummarize compresses by summarizing all old messages into blocks.
	StrategySummarize CompressStrategy = "summarize"
	// StrategySelective keeps high-importance messages and summarizes the rest.
	StrategySelective CompressStrategy = "selective"
	// StrategyTiered applies different levels of compression based on recency.
	StrategyTiered CompressStrategy = "tiered"
	// StrategySemantic groups messages by topic and keeps conclusions.
	StrategySemantic CompressStrategy = "semantic"
)

// CompressMessage represents a single message in the conversation for compression.
type CompressMessage struct {
	Role         string
	Content      string
	ToolName     string
	IsToolResult bool
	Importance   float64
	Tokens       int
}

// CompressedBlock represents a group of messages that have been compressed into a summary.
type CompressedBlock struct {
	Summary         string
	OriginalCount   int
	TokensSaved     int
	KeyFacts        []string
	ToolCallSummary string
	FilesDiscussed  []string
}

// CompressionResult holds statistics and details about a compression operation.
type CompressionResult struct {
	Original          int
	Compressed        int
	TokensSaved       int
	Blocks            []CompressedBlock
	PreservedMessages int
}

// SessionCompressor performs intelligent session compression using configurable strategies.
type SessionCompressor struct {
	Strategy         CompressStrategy
	PreservePatterns []string
	MinMessages      int
	mu               sync.Mutex
}

// NewSessionCompressor creates a new compressor with the given strategy.
func NewSessionCompressor(strategy CompressStrategy) *SessionCompressor {
	return &SessionCompressor{
		Strategy:         strategy,
		PreservePatterns: []string{},
		MinMessages:      8,
	}
}

// Compress applies the configured strategy to reduce messages to fit within the token budget.
// It returns the compression result with statistics and the new compressed message list.
func (sc *SessionCompressor) Compress(messages []CompressMessage, budget int) (*CompressionResult, []CompressMessage, error) {
	sc.mu.Lock()
	defer sc.mu.Unlock()

	if len(messages) == 0 {
		return &CompressionResult{}, []CompressMessage{}, nil
	}

	// Score importance for all messages
	for i := range messages {
		messages[i].Importance = ScoreImportance(messages[i], i, len(messages))
	}

	var compressed []CompressMessage

	switch sc.Strategy {
	case StrategySummarize:
		compressed = sc.summarizeCompress(messages, budget)
	case StrategySelective:
		compressed = SelectiveCompress(messages, budget)
	case StrategyTiered:
		compressed = TieredCompress(messages, budget)
	case StrategySemantic:
		compressed = SemanticCompress(messages, budget)
	default:
		compressed = SelectiveCompress(messages, budget)
	}

	// Calculate stats
	originalTokens := 0
	for _, m := range messages {
		originalTokens += m.Tokens
	}
	compressedTokens := 0
	for _, m := range compressed {
		compressedTokens += m.Tokens
	}

	// Build blocks from summarized sections
	blocks := sc.buildBlocks(messages, compressed)

	preserved := 0
	for _, c := range compressed {
		if c.Importance >= 0.9 {
			preserved++
		}
	}

	result := &CompressionResult{
		Original:          len(messages),
		Compressed:        len(compressed),
		TokensSaved:       originalTokens - compressedTokens,
		Blocks:            blocks,
		PreservedMessages: preserved,
	}

	return result, compressed, nil
}

// ScoreImportance assigns an importance score to a message based on its content and position.
func ScoreImportance(msg CompressMessage, position int, total int) float64 {
	// First user message always kept
	if position == 0 && msg.Role == "user" {
		return 1.0
	}

	// Last N messages always kept (last 20% or at least 5)
	keepLast := total / 5
	if keepLast < 5 {
		keepLast = 5
	}
	if position >= total-keepLast {
		return 1.0
	}

	// Tool errors are important context
	if msg.IsToolResult && containsError(msg.Content) {
		return 0.9
	}

	// Messages with decisions
	if containsDecision(msg.Content) {
		return 0.8
	}

	// Messages with code
	if containsCode(msg.Content) {
		return 0.7
	}

	// Generic chat
	if msg.Role == "user" || msg.Role == "assistant" {
		if !msg.IsToolResult && msg.ToolName == "" {
			return 0.4
		}
	}

	// Routine tool results (success)
	if msg.IsToolResult && !containsError(msg.Content) {
		return 0.3
	}

	return 0.5
}

// SummarizeBlock creates a CompressedBlock from a group of messages by extracting key information.
func SummarizeBlock(messages []CompressMessage) *CompressedBlock {
	if len(messages) == 0 {
		return &CompressedBlock{}
	}

	keyFacts := ExtractKeyFacts(messages)
	files := extractFiles(messages)
	toolCalls := summarizeToolCalls(messages)

	totalTokens := 0
	for _, m := range messages {
		totalTokens += m.Tokens
	}

	// Generate one-line summary based on content
	summary := generateBlockSummary(messages)

	// Estimate tokens saved (summary is much shorter)
	summaryTokens := estimateStringTokens(summary)
	for _, f := range keyFacts {
		summaryTokens += estimateStringTokens(f)
	}

	return &CompressedBlock{
		Summary:         summary,
		OriginalCount:   len(messages),
		TokensSaved:     totalTokens - summaryTokens,
		KeyFacts:        keyFacts,
		ToolCallSummary: toolCalls,
		FilesDiscussed:  files,
	}
}

// SelectiveCompress keeps high-importance messages verbatim and summarizes low-importance blocks.
func SelectiveCompress(messages []CompressMessage, budget int) []CompressMessage {
	if totalTokens(messages) <= budget {
		return messages
	}

	result := make([]CompressMessage, 0, len(messages))
	var lowBlock []CompressMessage

	for i, msg := range messages {
		// Ensure tool_use/tool_result pairs stay together
		if msg.Importance >= 0.7 || isPartOfToolPair(messages, i) {
			// Flush any pending low-importance block
			if len(lowBlock) > 0 {
				summary := createSummaryMessage(lowBlock)
				result = append(result, summary)
				lowBlock = nil
			}
			result = append(result, msg)
		} else {
			lowBlock = append(lowBlock, msg)
		}
	}

	// Flush remaining low block
	if len(lowBlock) > 0 {
		summary := createSummaryMessage(lowBlock)
		result = append(result, summary)
	}

	return result
}

// TieredCompress applies different compression levels based on message recency.
// Recent (last 20%): keep verbatim
// Middle (20-60%): selective (keep important, summarize rest)
// Old (60-100%): aggressive summary
func TieredCompress(messages []CompressMessage, budget int) []CompressMessage {
	if totalTokens(messages) <= budget {
		return messages
	}

	n := len(messages)
	recentStart := n - n/5   // last 20%
	middleStart := n - 3*n/5 // 20-60% from end

	if recentStart < 0 {
		recentStart = 0
	}
	if middleStart < 0 {
		middleStart = 0
	}

	result := make([]CompressMessage, 0)

	// Old section (0 to middleStart): aggressive summary
	if middleStart > 0 {
		oldMessages := messages[:middleStart]
		if len(oldMessages) > 0 {
			summary := createAggressiveSummary(oldMessages)
			result = append(result, summary)
		}
	}

	// Middle section (middleStart to recentStart): selective
	if middleStart < recentStart {
		middleMessages := messages[middleStart:recentStart]
		selective := selectiveKeep(middleMessages)
		result = append(result, selective...)
	}

	// Recent section (recentStart to end): keep verbatim
	if recentStart < n {
		result = append(result, messages[recentStart:]...)
	}

	return result
}

// SemanticCompress groups messages by topic/task, keeps conclusions, and summarizes journeys.
func SemanticCompress(messages []CompressMessage, budget int) []CompressMessage {
	if totalTokens(messages) <= budget {
		return messages
	}

	// Group messages by topic boundaries
	groups := groupByTopic(messages)

	result := make([]CompressMessage, 0)

	for _, group := range groups {
		if len(group.messages) == 0 {
			continue
		}

		// Check if this is a recent group (contains messages from last 20%).
		// Use the members' original indices (carried by the group) instead of
		// content equality, which is ambiguous when the same text recurs.
		lastIdx := -1
		for _, idx := range group.indices {
			if idx > lastIdx {
				lastIdx = idx
			}
		}

		isRecent := lastIdx >= len(messages)-len(messages)/5

		if isRecent {
			// Keep recent topics verbatim
			result = append(result, group.messages...)
		} else if len(group.messages) <= 2 {
			// Short groups kept as-is
			result = append(result, group.messages...)
		} else {
			// Keep the conclusion (last message) and summarize the journey
			journeySummary := createSummaryMessage(group.messages[:len(group.messages)-1])
			result = append(result, journeySummary)
			result = append(result, group.messages[len(group.messages)-1])
		}
	}

	return result
}

// ExtractKeyFacts pulls out important facts from a group of messages:
// decisions made, files modified, errors encountered and resolved, conventions established.
func ExtractKeyFacts(messages []CompressMessage) []string {
	facts := make([]string, 0)
	seen := make(map[string]bool)

	for _, msg := range messages {
		content := msg.Content

		// Decisions made
		if containsDecision(content) {
			fact := extractDecisionFact(content)
			if fact != "" && !seen[fact] {
				facts = append(facts, fact)
				seen[fact] = true
			}
		}

		// Files modified
		files := extractFileReferences(content)
		for _, f := range files {
			fact := "Modified: " + f
			if !seen[fact] {
				facts = append(facts, fact)
				seen[fact] = true
			}
		}

		// Errors encountered and resolved
		if containsError(content) {
			fact := extractErrorFact(content)
			if fact != "" && !seen[fact] {
				facts = append(facts, fact)
				seen[fact] = true
			}
		}

		// Conventions established
		if containsConvention(content) {
			fact := extractConventionFact(content)
			if fact != "" && !seen[fact] {
				facts = append(facts, fact)
				seen[fact] = true
			}
		}
	}

	return facts
}

// FormatCompressed produces a human-readable summary of the compression result.
func FormatCompressed(result *CompressionResult) string {
	if result == nil {
		return ""
	}

	reduction := 0
	if result.Original > 0 {
		reduction = 100 - (result.Compressed*100)/result.Original
	}

	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("Compression: %d messages → %d (%d%% reduction, saved %s tokens)\n",
		result.Original, result.Compressed, reduction, formatCompressedTokens(result.TokensSaved)))
	sb.WriteString(fmt.Sprintf("Preserved: last %d messages verbatim\n", result.PreservedMessages))

	if len(result.Blocks) > 0 {
		blockNames := make([]string, 0, len(result.Blocks))
		for _, b := range result.Blocks {
			if b.Summary != "" {
				blockNames = append(blockNames, b.Summary)
			}
		}
		if len(blockNames) > 0 {
			sb.WriteString(fmt.Sprintf("Summarized: %d blocks (%s)\n", len(result.Blocks), strings.Join(blockNames, ", ")))
		} else {
			sb.WriteString(fmt.Sprintf("Summarized: %d blocks\n", len(result.Blocks)))
		}
	}

	totalFacts := 0
	for _, b := range result.Blocks {
		totalFacts += len(b.KeyFacts)
	}
	if totalFacts > 0 {
		sb.WriteString(fmt.Sprintf("Key facts retained: %d\n", totalFacts))
	}

	return sb.String()
}

// --- Internal helpers ---

func (sc *SessionCompressor) summarizeCompress(messages []CompressMessage, budget int) []CompressMessage {
	if totalTokens(messages) <= budget {
		return messages
	}

	n := len(messages)
	keep := sc.MinMessages
	if keep > n {
		keep = n
	}

	// Keep last MinMessages verbatim
	recent := messages[n-keep:]

	// Summarize everything else
	old := messages[:n-keep]
	if len(old) == 0 {
		return recent
	}

	summary := createSummaryMessage(old)
	result := make([]CompressMessage, 0, keep+1)
	result = append(result, summary)
	result = append(result, recent...)
	return result
}

func (sc *SessionCompressor) buildBlocks(original, compressed []CompressMessage) []CompressedBlock {
	blocks := make([]CompressedBlock, 0)

	// Find summary messages in compressed output (those with [Summary] prefix)
	for _, msg := range compressed {
		if strings.HasPrefix(msg.Content, "[Summary:") || strings.HasPrefix(msg.Content, "[Compressed:") {
			block := CompressedBlock{
				Summary:       extractSummaryLabel(msg.Content),
				OriginalCount: countOriginalForSummary(original, msg),
				TokensSaved:   msg.Tokens, // approximate
				KeyFacts:      extractFactsFromSummary(msg.Content),
			}
			blocks = append(blocks, block)
		}
	}

	return blocks
}

func totalTokens(messages []CompressMessage) int {
	total := 0
	for _, m := range messages {
		total += m.Tokens
	}
	return total
}

func containsError(content string) bool {
	lower := strings.ToLower(content)
	return strings.Contains(lower, "error") ||
		strings.Contains(lower, "failed") ||
		strings.Contains(lower, "panic") ||
		strings.Contains(lower, "fatal") ||
		strings.Contains(lower, "exception")
}

func containsCode(content string) bool {
	return strings.Contains(content, "```") ||
		strings.Contains(content, "func ") ||
		strings.Contains(content, "def ") ||
		strings.Contains(content, "class ") ||
		strings.Contains(content, "import ")
}

func containsDecision(content string) bool {
	lower := strings.ToLower(content)
	return strings.Contains(lower, "decided") ||
		strings.Contains(lower, "let's use") ||
		strings.Contains(lower, "we'll go with") ||
		strings.Contains(lower, "the approach") ||
		strings.Contains(lower, "instead of") ||
		strings.Contains(lower, "chosen") ||
		strings.Contains(lower, "will use")
}

func containsConvention(content string) bool {
	lower := strings.ToLower(content)
	return strings.Contains(lower, "convention") ||
		strings.Contains(lower, "always use") ||
		strings.Contains(lower, "naming pattern") ||
		strings.Contains(lower, "style guide") ||
		strings.Contains(lower, "standard is")
}

func isPartOfToolPair(messages []CompressMessage, idx int) bool {
	msg := messages[idx]

	// If this is a tool call, check if next message is tool result
	if msg.ToolName != "" && !msg.IsToolResult {
		if idx+1 < len(messages) && messages[idx+1].IsToolResult {
			return true
		}
	}

	// If this is a tool result, check if previous message is tool call
	if msg.IsToolResult {
		if idx > 0 && messages[idx-1].ToolName != "" && !messages[idx-1].IsToolResult {
			return true
		}
	}

	return false
}

func createSummaryMessage(messages []CompressMessage) CompressMessage {
	if len(messages) == 0 {
		return CompressMessage{
			Role:    "system",
			Content: "[Summary: empty block]",
			Tokens:  5,
		}
	}

	facts := ExtractKeyFacts(messages)
	files := extractFiles(messages)
	toolCount := countToolCalls(messages)

	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("[Summary: %d messages compressed]\n", len(messages)))

	if toolCount > 0 {
		sb.WriteString(fmt.Sprintf("Tool calls: %d\n", toolCount))
	}
	if len(files) > 0 {
		sb.WriteString(fmt.Sprintf("Files: %s\n", strings.Join(files, ", ")))
	}
	if len(facts) > 0 {
		sb.WriteString("Key facts:\n")
		for _, f := range facts {
			sb.WriteString(fmt.Sprintf("- %s\n", f))
		}
	}

	content := sb.String()
	return CompressMessage{
		Role:       "system",
		Content:    content,
		Importance: 0.6,
		Tokens:     estimateStringTokens(content),
	}
}

func createAggressiveSummary(messages []CompressMessage) CompressMessage {
	if len(messages) == 0 {
		return CompressMessage{
			Role:    "system",
			Content: "[Compressed: empty section]",
			Tokens:  5,
		}
	}

	facts := ExtractKeyFacts(messages)
	files := extractFiles(messages)

	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("[Compressed: %d messages into brief summary]\n", len(messages)))

	if len(files) > 0 {
		limit := 5
		if len(files) < limit {
			limit = len(files)
		}
		sb.WriteString(fmt.Sprintf("Files touched: %s\n", strings.Join(files[:limit], ", ")))
	}
	if len(facts) > 0 {
		limit := 3
		if len(facts) < limit {
			limit = len(facts)
		}
		for _, f := range facts[:limit] {
			sb.WriteString(fmt.Sprintf("- %s\n", f))
		}
	}

	content := sb.String()
	return CompressMessage{
		Role:       "system",
		Content:    content,
		Importance: 0.5,
		Tokens:     estimateStringTokens(content),
	}
}

func selectiveKeep(messages []CompressMessage) []CompressMessage {
	result := make([]CompressMessage, 0)
	var lowBlock []CompressMessage

	for _, msg := range messages {
		if msg.Importance >= 0.6 {
			if len(lowBlock) > 1 {
				summary := createSummaryMessage(lowBlock)
				result = append(result, summary)
			} else if len(lowBlock) == 1 {
				result = append(result, lowBlock[0])
			}
			lowBlock = nil
			result = append(result, msg)
		} else {
			lowBlock = append(lowBlock, msg)
		}
	}

	if len(lowBlock) > 1 {
		summary := createSummaryMessage(lowBlock)
		result = append(result, summary)
	} else if len(lowBlock) == 1 {
		result = append(result, lowBlock[0])
	}

	return result
}

// topicGroup is a group of messages that share a topic, paired with each
// member's original index in the source slice. Carrying the indices avoids
// identifying members by content equality (which is ambiguous when the same
// text appears more than once — e.g. repeated "ok" or identical tool results).
type topicGroup struct {
	messages []CompressMessage
	indices  []int
}

func groupByTopic(messages []CompressMessage) []topicGroup {
	if len(messages) == 0 {
		return nil
	}

	groups := make([]topicGroup, 0)
	current := topicGroup{
		messages: []CompressMessage{messages[0]},
		indices:  []int{0},
	}

	for i := 1; i < len(messages); i++ {
		// Topic boundary heuristics:
		// - User message after assistant message signals potential new topic
		// - Change in files being discussed
		// - Significant gap in tool usage patterns
		if isTopicBoundary(messages[i-1], messages[i]) {
			groups = append(groups, current)
			current = topicGroup{
				messages: []CompressMessage{messages[i]},
				indices:  []int{i},
			}
		} else {
			current.messages = append(current.messages, messages[i])
			current.indices = append(current.indices, i)
		}
	}

	if len(current.messages) > 0 {
		groups = append(groups, current)
	}

	return groups
}

func isTopicBoundary(prev, curr CompressMessage) bool {
	// New user message after assistant response often marks a topic shift
	if prev.Role == "assistant" && curr.Role == "user" {
		// Check if the user message seems to start a new task
		lower := strings.ToLower(curr.Content)
		newTopicSignals := []string{"now ", "next ", "can you ", "please ", "let's ", "switch to", "moving on"}
		for _, signal := range newTopicSignals {
			if strings.HasPrefix(lower, signal) {
				return true
			}
		}
	}

	// Transition from tool results back to user input
	if prev.IsToolResult && curr.Role == "user" && !curr.IsToolResult {
		return true
	}

	return false
}

func extractFiles(messages []CompressMessage) []string {
	files := make([]string, 0)
	seen := make(map[string]bool)

	for _, msg := range messages {
		refs := extractFileReferences(msg.Content)
		for _, f := range refs {
			if !seen[f] {
				files = append(files, f)
				seen[f] = true
			}
		}
	}

	return files
}

func extractFileReferences(content string) []string {
	files := make([]string, 0)

	// Look for common file path patterns
	words := strings.Fields(content)
	for _, w := range words {
		// Strip surrounding punctuation
		w = strings.Trim(w, "\"'`(),;:")
		if looksLikeFilePath(w) {
			files = append(files, w)
		}
	}

	return files
}

func looksLikeFilePath(s string) bool {
	// Must contain a dot and slash, or end with a known extension
	if strings.Contains(s, "/") && strings.Contains(s, ".") {
		return true
	}
	extensions := []string{".go", ".py", ".js", ".ts", ".rs", ".c", ".h", ".java", ".rb", ".yaml", ".yml", ".json", ".toml"}
	for _, ext := range extensions {
		if strings.HasSuffix(s, ext) && len(s) > len(ext)+1 {
			return true
		}
	}
	return false
}

func summarizeToolCalls(messages []CompressMessage) string {
	tools := make(map[string]int)
	for _, msg := range messages {
		if msg.ToolName != "" {
			tools[msg.ToolName]++
		}
	}

	if len(tools) == 0 {
		return ""
	}

	parts := make([]string, 0, len(tools))
	for name, count := range tools {
		parts = append(parts, fmt.Sprintf("%s(%d)", name, count))
	}
	return strings.Join(parts, ", ")
}

func countToolCalls(messages []CompressMessage) int {
	count := 0
	for _, msg := range messages {
		if msg.ToolName != "" && !msg.IsToolResult {
			count++
		}
	}
	return count
}

func generateBlockSummary(messages []CompressMessage) string {
	// Determine the dominant activity
	toolCount := countToolCalls(messages)
	hasErrors := false
	hasCode := false

	for _, msg := range messages {
		if containsError(msg.Content) {
			hasErrors = true
		}
		if containsCode(msg.Content) {
			hasCode = true
		}
	}

	if hasErrors && toolCount > 0 {
		return "debugging"
	}
	if toolCount > 3 {
		return "tool executions"
	}
	if hasCode {
		return "code discussion"
	}
	return "planning"
}

func extractDecisionFact(content string) string {
	lines := strings.Split(content, "\n")
	for _, line := range lines {
		lower := strings.ToLower(line)
		if strings.Contains(lower, "decided") ||
			strings.Contains(lower, "let's use") ||
			strings.Contains(lower, "we'll go with") ||
			strings.Contains(lower, "will use") {
			trimmed := strings.TrimSpace(line)
			if len(trimmed) > 100 {
				// Rune-safe truncation: never split a multibyte UTF-8 sequence.
				if runes := []rune(trimmed); len(runes) > 100 {
					trimmed = string(runes[:100]) + "..."
				}
			}
			return "Decision: " + trimmed
		}
	}
	return ""
}

func extractErrorFact(content string) string {
	lines := strings.Split(content, "\n")
	for _, line := range lines {
		lower := strings.ToLower(line)
		if strings.Contains(lower, "error") || strings.Contains(lower, "failed") {
			trimmed := strings.TrimSpace(line)
			if len(trimmed) > 100 {
				// Rune-safe truncation: never split a multibyte UTF-8 sequence.
				if runes := []rune(trimmed); len(runes) > 100 {
					trimmed = string(runes[:100]) + "..."
				}
			}
			return "Error: " + trimmed
		}
	}
	return ""
}

func extractConventionFact(content string) string {
	lines := strings.Split(content, "\n")
	for _, line := range lines {
		lower := strings.ToLower(line)
		if strings.Contains(lower, "convention") ||
			strings.Contains(lower, "always use") ||
			strings.Contains(lower, "standard is") {
			trimmed := strings.TrimSpace(line)
			if len(trimmed) > 100 {
				// Rune-safe truncation: never split a multibyte UTF-8 sequence.
				if runes := []rune(trimmed); len(runes) > 100 {
					trimmed = string(runes[:100]) + "..."
				}
			}
			return "Convention: " + trimmed
		}
	}
	return ""
}

func extractSummaryLabel(content string) string {
	// Extract the label from [Summary: ...] or [Compressed: ...]
	if idx := strings.Index(content, "]"); idx > 0 {
		label := content[1:idx]
		// Remove the prefix
		label = strings.TrimPrefix(label, "Summary: ")
		label = strings.TrimPrefix(label, "Compressed: ")
		return label
	}
	return "block"
}

func countOriginalForSummary(original []CompressMessage, summary CompressMessage) int {
	// Parse count from content if available
	content := summary.Content
	if strings.Contains(content, "messages compressed") || strings.Contains(content, "messages into") {
		var count int
		if _, err := fmt.Sscanf(extractCountStr(content), "%d", &count); err == nil {
			return count
		}
	}
	return len(original)
}

func extractCountStr(content string) string {
	// Find the number in patterns like "N messages"
	words := strings.Fields(content)
	for i, w := range words {
		if i+1 < len(words) && words[i+1] == "messages" {
			return w
		}
	}
	return "0"
}

func extractFactsFromSummary(content string) []string {
	facts := make([]string, 0)
	lines := strings.Split(content, "\n")
	for _, line := range lines {
		if strings.HasPrefix(line, "- ") {
			facts = append(facts, strings.TrimPrefix(line, "- "))
		}
	}
	return facts
}

func estimateStringTokens(s string) int {
	// Rough estimate: ~4 chars per token
	tokens := len(s) / 4
	if tokens < 1 {
		tokens = 1
	}
	return tokens
}

func formatCompressedTokens(tokens int) string {
	if tokens >= 1000 {
		return fmt.Sprintf("%d,%03d", tokens/1000, tokens%1000)
	}
	return fmt.Sprintf("%d", tokens)
}
