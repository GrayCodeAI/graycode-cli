package engine

import (
	"fmt"
	"strings"
	"sync"
)

// HeadTailWindow implements a context window strategy that keeps the first N
// and last M messages, dropping the middle. Inspired by autogen's
// HeadAndTailChatCompletionContext.
type HeadTailWindow struct {
	HeadSize       int
	TailSize       int
	MaxTokens      int
	IncludeSummary bool
	mu             sync.Mutex
}

// WindowResult holds the result of applying the head-tail window strategy.
type WindowResult struct {
	Head        []WindowMessage
	Tail        []WindowMessage
	Dropped     int
	Summary     string
	TotalTokens int
}

// WindowMessage represents a single message in the context window.
type WindowMessage struct {
	Role         string
	Content      string
	Tokens       int
	Index        int
	IsToolResult bool
}

// NewHeadTailWindow creates a new HeadTailWindow with the given sizes.
// If headSize <= 0, defaults to 4. If tailSize <= 0, defaults to 12.
// maxTokens sets the token budget; if <= 0, no token limit is enforced.
func NewHeadTailWindow(headSize, tailSize, maxTokens int) *HeadTailWindow {
	if headSize <= 0 {
		headSize = 4
	}
	if tailSize <= 0 {
		tailSize = 12
	}
	return &HeadTailWindow{
		HeadSize:  headSize,
		TailSize:  tailSize,
		MaxTokens: maxTokens,
	}
}

// Apply keeps the first HeadSize messages and last TailSize messages, dropping
// the middle. If IncludeSummary is true, a brief summary of dropped messages is
// generated. The result is verified to fit within MaxTokens.
func (w *HeadTailWindow) Apply(messages []WindowMessage) *WindowResult {
	w.mu.Lock()
	defer w.mu.Unlock()

	total := len(messages)

	// If messages fit entirely within head+tail, no dropping needed.
	if total <= w.HeadSize+w.TailSize {
		allTokens := 0
		for _, m := range messages {
			allTokens += m.Tokens
		}
		return &WindowResult{
			Head:        messages,
			Tail:        nil,
			Dropped:     0,
			TotalTokens: allTokens,
		}
	}

	head := make([]WindowMessage, w.HeadSize)
	copy(head, messages[:w.HeadSize])

	tailStart := total - w.TailSize
	tail := make([]WindowMessage, w.TailSize)
	copy(tail, messages[tailStart:])

	dropped := messages[w.HeadSize:tailStart]
	droppedCount := len(dropped)

	var summary string
	if w.IncludeSummary && droppedCount > 0 {
		summary = summarizeDroppedWindow(dropped)
	}

	totalTokens := 0
	for _, m := range head {
		totalTokens += m.Tokens
	}
	for _, m := range tail {
		totalTokens += m.Tokens
	}

	result := &WindowResult{
		Head:        head,
		Tail:        tail,
		Dropped:     droppedCount,
		Summary:     summary,
		TotalTokens: totalTokens,
	}

	// If we exceed MaxTokens, trim tail from the front until we fit.
	if w.MaxTokens > 0 && result.TotalTokens > w.MaxTokens {
		for len(result.Tail) > 0 && result.TotalTokens > w.MaxTokens {
			result.TotalTokens -= result.Tail[0].Tokens
			result.Tail = result.Tail[1:]
			result.Dropped++
		}
		// If still over budget, trim head from the back.
		for len(result.Head) > 0 && result.TotalTokens > w.MaxTokens {
			result.TotalTokens -= result.Head[len(result.Head)-1].Tokens
			result.Head = result.Head[:len(result.Head)-1]
			result.Dropped++
		}
	}

	return result
}

// summarizeDroppedWindow generates a brief summary of the dropped messages,
// extracting key actions and decisions.
func summarizeDroppedWindow(dropped []WindowMessage) string {
	if len(dropped) == 0 {
		return ""
	}

	toolCalls := 0
	topics := make(map[string]bool)
	actions := []string{}

	for _, m := range dropped {
		if m.IsToolResult {
			toolCalls++
		}

		content := strings.ToLower(m.Content)

		// Extract topics from content heuristically.
		if strings.Contains(content, "auth") || strings.Contains(content, "authentication") {
			topics["auth"] = true
		}
		if strings.Contains(content, "test") {
			topics["tests"] = true
		}
		if strings.Contains(content, "bug") || strings.Contains(content, "fix") {
			topics["bug fixes"] = true
		}
		if strings.Contains(content, "refactor") {
			topics["refactoring"] = true
		}
		if strings.Contains(content, "deploy") || strings.Contains(content, "deployment") {
			topics["deployment"] = true
		}
		if strings.Contains(content, "config") || strings.Contains(content, "configuration") {
			topics["configuration"] = true
		}
		if strings.Contains(content, "error") || strings.Contains(content, "fail") {
			topics["error handling"] = true
		}
		if strings.Contains(content, "implement") || strings.Contains(content, "added") {
			topics["implementation"] = true
		}
	}

	// Build topic list.
	topicList := []string{}
	for t := range topics {
		topicList = append(topicList, t)
	}

	if len(topicList) > 0 {
		actions = append(actions, "discussed "+strings.Join(topicList, ", "))
	}

	if toolCalls > 0 {
		actions = append(actions, fmt.Sprintf("ran %d tool calls", toolCalls))
	}

	if len(actions) == 0 {
		return fmt.Sprintf("Earlier: %d messages exchanged", len(dropped))
	}

	return "Earlier: " + strings.Join(actions, ", ")
}

// AdaptiveSizes dynamically determines head and tail sizes based on token
// budget. More budget yields a larger tail (recent context matters more).
// Always returns at least 2 head + 4 tail.
func AdaptiveSizes(messages []WindowMessage, budget int) (head, tail int) {
	totalTokens := 0
	for _, m := range messages {
		totalTokens += m.Tokens
	}

	msgCount := len(messages)

	// Minimum guarantees.
	minHead := 2
	minTail := 4

	if msgCount <= minHead+minTail {
		return minHead, minTail
	}

	// Calculate available slots (messages that can be kept).
	avgTokens := 1
	if msgCount > 0 {
		avgTokens = totalTokens / msgCount
	}
	if avgTokens <= 0 {
		avgTokens = 1
	}

	availableSlots := budget / avgTokens
	if availableSlots > msgCount {
		availableSlots = msgCount
	}
	if availableSlots < minHead+minTail {
		return minHead, minTail
	}

	// Allocate 25% to head, 75% to tail (recent context is more valuable).
	head = availableSlots / 4
	tail = availableSlots - head

	// Enforce minimums.
	if head < minHead {
		head = minHead
	}
	if tail < minTail {
		tail = minTail
	}

	// Don't exceed message count.
	if head+tail > msgCount {
		tail = msgCount - head
		if tail < minTail {
			tail = minTail
			head = msgCount - tail
		}
	}

	return head, tail
}

// PreserveToolPairs ensures tool_use and tool_result messages stay together by
// adjusting window boundaries so pairs are not split. Returns the adjusted
// messages and number of extra messages included to preserve pairs.
func PreserveToolPairs(messages []WindowMessage, head, tail int) ([]WindowMessage, int) {
	total := len(messages)
	if total <= head+tail {
		return messages, 0
	}

	headEnd := head
	tailStart := total - tail

	extra := 0

	// Expand head forward if it ends in the middle of a tool pair.
	// A tool pair is: a non-tool-result message followed by a tool result.
	if headEnd < total && headEnd > 0 {
		// If the last message in head is not a tool result but the next one is,
		// include the tool result in head.
		for headEnd < tailStart && headEnd < total && messages[headEnd].IsToolResult {
			headEnd++
			extra++
		}
	}

	// Shrink tail start backward if it starts with a tool result without its
	// preceding tool_use.
	if tailStart > headEnd && tailStart < total && messages[tailStart].IsToolResult {
		// Include the preceding message (the tool_use).
		tailStart--
		extra++
	}

	// Build the result: head portion + tail portion.
	result := make([]WindowMessage, 0, headEnd+(total-tailStart))
	result = append(result, messages[:headEnd]...)
	result = append(result, messages[tailStart:]...)

	return result, extra
}

// FormatWindow returns a human-readable representation of the window result.
func FormatWindow(result *WindowResult) string {
	if result == nil {
		return "Context Window (head-tail): empty"
	}

	var sb strings.Builder
	sb.WriteString("Context Window (head-tail):\n")

	headCount := len(result.Head)
	tailCount := len(result.Tail)

	sb.WriteString(fmt.Sprintf("Head: %d messages (initial context)\n", headCount))

	if result.Dropped > 0 {
		sb.WriteString(fmt.Sprintf("[... %d messages dropped ...]\n", result.Dropped))
	}

	if result.Summary != "" {
		sb.WriteString(fmt.Sprintf("Summary: %s\n", result.Summary))
	}

	if tailCount > 0 {
		sb.WriteString(fmt.Sprintf("Tail: %d messages (recent)\n", tailCount))
	}

	totalMsgs := headCount + tailCount
	sb.WriteString(fmt.Sprintf("Total: %d messages, %s tokens", totalMsgs, formatWindowTokens(result.TotalTokens)))

	return sb.String()
}

// ShouldApply returns true if the total token count of messages exceeds
// maxTokens, indicating the window strategy should be applied.
func ShouldApply(messages []WindowMessage, maxTokens int) bool {
	if maxTokens <= 0 {
		return false
	}

	total := 0
	for _, m := range messages {
		total += m.Tokens
		if total > maxTokens {
			return true
		}
	}
	return false
}

// formatWindowTokens formats a token count with comma separators.
func formatWindowTokens(n int) string {
	if n < 1000 {
		return fmt.Sprintf("%d", n)
	}

	s := fmt.Sprintf("%d", n)
	result := []byte{}
	for i, c := range s {
		if i > 0 && (len(s)-i)%3 == 0 {
			result = append(result, ',')
		}
		result = append(result, byte(c))
	}
	return string(result)
}
