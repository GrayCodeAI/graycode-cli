package engine

import (
	"fmt"
	"strings"
	"sync"
	"time"
)

// ConversationMessage represents a single message in a conversation.
type ConversationMessage struct {
	Role      string
	Content   string
	Timestamp time.Time
}

// ConversationConfig holds configuration for a ConversationManager.
type ConversationConfig struct {
	MaxHistory        int
	SummarizeThreshold int
	SystemPrompt       string
}

// ConversationState represents the full state of a conversation at a point in time.
type ConversationState struct {
	Messages   []ConversationMessage
	TokenCount int
	Summary    string
}

// ConversationManager manages a conversation lifecycle, bridging eyrie's
// conversation management into hawk's chat session flow. It is safe for
// concurrent use.
type ConversationManager struct {
	mu          sync.RWMutex
	config      ConversationConfig
	messages    []ConversationMessage
	summary     string
	tokenCount  int
}

// NewConversationManager creates a new ConversationManager with the given config.
// If SystemPrompt is set, it is automatically added as the first message.
func NewConversationManager(config ConversationConfig) *ConversationManager {
	cm := &ConversationManager{
		config: config,
	}

	if config.SystemPrompt != "" {
		msg := ConversationMessage{
			Role:      "system",
			Content:   config.SystemPrompt,
			Timestamp: time.Now(),
		}
		cm.messages = append(cm.messages, msg)
		cm.tokenCount = estimateTokens(config.SystemPrompt)
	}

	return cm
}

// AddMessage appends a new message to the conversation history and updates the
// running token estimate. Returns the added message.
func (cm *ConversationManager) AddMessage(role, content string) ConversationMessage {
	cm.mu.Lock()
	defer cm.mu.Unlock()

	msg := ConversationMessage{
		Role:      role,
		Content:   content,
		Timestamp: time.Now(),
	}

	cm.messages = append(cm.messages, msg)
	cm.tokenCount += estimateTokens(content)

	return msg
}

// GetHistory returns a copy of the current messages.
func (cm *ConversationManager) GetHistory() []ConversationMessage {
	cm.mu.RLock()
	defer cm.mu.RUnlock()

	out := make([]ConversationMessage, len(cm.messages))
	copy(out, cm.messages)
	return out
}

// GetState returns a snapshot of the full conversation state including messages,
// token count, and any active summary.
func (cm *ConversationManager) GetState() ConversationState {
	cm.mu.RLock()
	defer cm.mu.RUnlock()

	msgs := make([]ConversationMessage, len(cm.messages))
	copy(msgs, cm.messages)

	return ConversationState{
		Messages:   msgs,
		TokenCount: cm.tokenCount,
		Summary:    cm.summary,
	}
}

// TrimHistory removes the oldest non-system messages so that at most maxMessages
// total messages remain. The system prompt (if present) is always preserved.
func (cm *ConversationManager) TrimHistory(maxMessages int) {
	cm.mu.Lock()
	defer cm.mu.Unlock()

	if maxMessages <= 0 || len(cm.messages) <= maxMessages {
		return
	}

	hasSystem := len(cm.messages) > 0 && cm.messages[0].Role == "system"

	var systemPrefix []ConversationMessage
	if hasSystem {
		systemPrefix = append(systemPrefix, cm.messages[0])
	}

	keep := maxMessages - len(systemPrefix)
	if keep <= 0 {
		keep = 1
	}

	// Keep the most recent `keep` non-system messages.
	trimmed := cm.messages[len(cm.messages)-keep:]
	cm.messages = append(systemPrefix, trimmed...)

	// Recalculate token count.
	cm.tokenCount = 0
	for _, m := range cm.messages {
		cm.tokenCount += estimateTokens(m.Content)
	}
}

// ShouldSummarize reports whether the message count exceeds the configured
// SummarizeThreshold. A threshold of 0 disables summarization.
func (cm *ConversationManager) ShouldSummarize() bool {
	cm.mu.RLock()
	defer cm.mu.RUnlock()

	if cm.config.SummarizeThreshold <= 0 {
		return false
	}
	return len(cm.messages) > cm.config.SummarizeThreshold
}

// BuildSummary produces a condensed summary of all non-system, non-recent
// messages. The summary is stored internally and also returned. Older messages
// that have been summarized can be trimmed by the caller if desired.
func (cm *ConversationManager) BuildSummary() string {
	cm.mu.Lock()
	defer cm.mu.Unlock()

	hasSystem := len(cm.messages) > 0 && cm.messages[0].Role == "system"

	// Summarize all messages except system prompt and the most recent few.
	cutoff := 10
	if hasSystem {
		cutoff = 11
	}
	if cutoff > len(cm.messages) {
		cutoff = len(cm.messages)
	}

	var oldest []ConversationMessage
	if hasSystem && len(cm.messages) > cutoff {
		oldest = cm.messages[1 : len(cm.messages)-cutoff+1]
	} else if !hasSystem && len(cm.messages) > cutoff {
		oldest = cm.messages[:len(cm.messages)-cutoff]
	}

	if len(oldest) == 0 {
		return cm.summary
	}

	var sb strings.Builder
	sb.WriteString("Summary of earlier conversation:\n")
	for _, m := range oldest {
		sb.WriteString(fmt.Sprintf("[%s] %s\n", m.Role, truncateContent(m.Content, 200)))
	}

	cm.summary = sb.String()
	return cm.summary
}

// Reset clears all messages, summary, and token count. If the config has a
// system prompt, it is re-added.
func (cm *ConversationManager) Reset() {
	cm.mu.Lock()
	defer cm.mu.Unlock()

	cm.messages = nil
	cm.summary = ""
	cm.tokenCount = 0

	if cm.config.SystemPrompt != "" {
		msg := ConversationMessage{
			Role:      "system",
			Content:   cm.config.SystemPrompt,
			Timestamp: time.Now(),
		}
		cm.messages = append(cm.messages, msg)
		cm.tokenCount = estimateTokens(cm.config.SystemPrompt)
	}
}

// ExportMessages returns the conversation as a slice of maps suitable for API
// calls (e.g., {"role": "user", "content": "..."}).
func (cm *ConversationManager) ExportMessages() []map[string]string {
	cm.mu.RLock()
	defer cm.mu.RUnlock()

	out := make([]map[string]string, 0, len(cm.messages))
	for _, m := range cm.messages {
		out = append(out, map[string]string{
			"role":    m.Role,
			"content": m.Content,
		})
	}
	return out
}

// TokenEstimate returns the current rough token count estimate (characters / 4).
func (cm *ConversationManager) TokenEstimate() int {
	cm.mu.RLock()
	defer cm.mu.RUnlock()
	return cm.tokenCount
}

// truncateContent truncates content to maxLen characters, appending "..." if truncated.
func truncateContent(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen] + "..."
}
