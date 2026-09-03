package session

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"html"
	"regexp"
	"strings"
	"time"
)

// ExportFormat defines the output format for session exports.
type ExportFormat string

const (
	// FormatMarkdown exports as a readable conversation log.
	FormatMarkdown ExportFormat = "markdown"
	// FormatJSON exports as full structured JSON.
	FormatJSON ExportFormat = "json"
	// FormatHTML exports as a styled HTML page.
	FormatHTML ExportFormat = "html"
	// FormatReplay exports as JSONL for reproducible replay.
	FormatReplay ExportFormat = "replay"
	// FormatClaude imports from Claude Code JSONL.
	FormatClaude ExportFormat = "claude"
	// FormatAider imports from Aider chat history.
	FormatAider ExportFormat = "aider"
	// FormatCursor imports from Cursor JSON logs.
	FormatCursor ExportFormat = "cursor"
	// FormatOpenAI imports/exports standard OpenAI Chat JSON.
	FormatOpenAI ExportFormat = "openai"
)

// SessionExporter configures how sessions are exported.
type SessionExporter struct {
	IncludeToolResults  bool
	IncludeSystemPrompt bool
	MaxMessages         int // 0 = all
	RedactSecrets       bool
}

// ExportedSession is a portable representation of a session.
type ExportedSession struct {
	ID        string             `json:"id"`
	Model     string             `json:"model"`
	Provider  string             `json:"provider"`
	CreatedAt time.Time          `json:"created_at"`
	Messages  []ExportedMessage  `json:"messages"`
	Metadata  map[string]string  `json:"metadata,omitempty"`
	Stats     SessionExportStats `json:"stats"`
}

// ExportedMessage is a single message within an exported session.
type ExportedMessage struct {
	Role       string    `json:"role"`
	Content    string    `json:"content"`
	Timestamp  time.Time `json:"timestamp"`
	ToolName   string    `json:"tool_name,omitempty"`
	ToolResult string    `json:"tool_result,omitempty"`
	TokenCount int       `json:"token_count,omitempty"`
}

// SessionExportStats summarizes session metrics.
type SessionExportStats struct {
	TotalMessages     int           `json:"total_messages"`
	UserMessages      int           `json:"user_messages"`
	AssistantMessages int           `json:"assistant_messages"`
	ToolCalls         int           `json:"tool_calls"`
	TotalTokens       int           `json:"total_tokens"`
	Duration          time.Duration `json:"duration"`
}

// NewSessionExporter creates a SessionExporter with default settings.
func NewSessionExporter() *SessionExporter {
	return &SessionExporter{
		IncludeToolResults:  true,
		IncludeSystemPrompt: false,
		MaxMessages:         0,
		RedactSecrets:       false,
	}
}

// Export dispatches to the format-specific renderer and returns the result.
func (e *SessionExporter) Export(session *ExportedSession, format ExportFormat) (string, error) {
	if session == nil {
		return "", fmt.Errorf("session is nil")
	}

	sess := e.prepareSession(session)

	switch format {
	case FormatMarkdown:
		return ExportMarkdown(sess), nil
	case FormatJSON:
		return ExportJSON(sess), nil
	case FormatHTML:
		return ExportHTML(sess), nil
	case FormatReplay:
		return ExportReplay(sess), nil
	case FormatOpenAI:
		return formatOpenAI(sess)
	default:
		return "", fmt.Errorf("unsupported export format: %s", string(format))
	}
}

// prepareSession applies exporter settings (truncation, redaction, filtering).
func (e *SessionExporter) prepareSession(session *ExportedSession) *ExportedSession {
	// Work on a copy to avoid mutating the original.
	copy := *session
	msgs := make([]ExportedMessage, 0, len(session.Messages))

	for _, m := range session.Messages {
		if !e.IncludeSystemPrompt && m.Role == "system" {
			continue
		}
		if !e.IncludeToolResults && m.ToolResult != "" {
			mc := m
			mc.ToolResult = ""
			msgs = append(msgs, mc)
			continue
		}
		msgs = append(msgs, m)
	}

	if e.MaxMessages > 0 && len(msgs) > e.MaxMessages {
		msgs = msgs[:e.MaxMessages]
	}

	copy.Messages = msgs

	if e.RedactSecrets {
		redacted := RedactSensitive(&copy)
		return redacted
	}

	return &copy
}

// ExportMarkdown renders a session as a readable Markdown conversation log.
func ExportMarkdown(session *ExportedSession) string {
	if session == nil {
		return ""
	}

	var b strings.Builder

	b.WriteString(fmt.Sprintf("# Session: %s\n", session.ID))
	b.WriteString(fmt.Sprintf("Model: %s | Provider: %s\n", session.Model, session.Provider))
	b.WriteString(fmt.Sprintf("Date: %s | Duration: %s\n",
		session.CreatedAt.UTC().Format("2006-01-02 15:04 UTC"),
		formatDuration(session.Stats.Duration)))
	b.WriteString(fmt.Sprintf("Tokens: %s | Messages: %d\n",
		formatNumber(session.Stats.TotalTokens), session.Stats.TotalMessages))
	b.WriteString("\n---\n\n")

	for _, msg := range session.Messages {
		switch msg.Role {
		case "user":
			b.WriteString("## User\n")
			b.WriteString(msg.Content)
			b.WriteString("\n\n")
		case "assistant":
			b.WriteString("## Assistant\n")
			b.WriteString(msg.Content)
			b.WriteString("\n\n")
			if msg.ToolName != "" {
				b.WriteString(fmt.Sprintf("### Tool: %s\n", msg.ToolName))
				if msg.ToolResult != "" {
					b.WriteString("```\n")
					b.WriteString(msg.ToolResult)
					b.WriteString("\n```\n\n")
				}
			}
		case "system":
			b.WriteString("## System\n")
			b.WriteString(msg.Content)
			b.WriteString("\n\n")
		case "tool":
			b.WriteString(fmt.Sprintf("### Tool: %s\n", msg.ToolName))
			if msg.ToolResult != "" {
				b.WriteString("```\n")
				b.WriteString(msg.ToolResult)
				b.WriteString("\n```\n\n")
			}
		}
	}

	b.WriteString("---\n")
	return b.String()
}

// ExportHTML renders a session as a styled HTML page with dark/light theme support.
func ExportHTML(session *ExportedSession) string {
	if session == nil {
		return ""
	}

	var b strings.Builder

	b.WriteString(`<!DOCTYPE html>
<html lang="en">
<head>
<meta charset="UTF-8">
<meta name="viewport" content="width=device-width, initial-scale=1.0">
<title>Session: ` + html.EscapeString(session.ID) + `</title>
<style>
:root {
  --bg: #ffffff;
  --fg: #1a1a1a;
  --msg-user-bg: #e8f4fd;
  --msg-assistant-bg: #f0f0f0;
  --tool-bg: #f8f8e0;
  --border: #ddd;
  --code-bg: #282c34;
  --code-fg: #abb2bf;
}
@media (prefers-color-scheme: dark) {
  :root {
    --bg: #1a1a2e;
    --fg: #e0e0e0;
    --msg-user-bg: #1e3a5f;
    --msg-assistant-bg: #2a2a3e;
    --tool-bg: #2e2e1e;
    --border: #444;
    --code-bg: #0d1117;
    --code-fg: #c9d1d9;
  }
}
body { font-family: -apple-system, BlinkMacSystemFont, 'Segoe UI', sans-serif; background: var(--bg); color: var(--fg); max-width: 800px; margin: 0 auto; padding: 20px; line-height: 1.6; }
.header { border-bottom: 2px solid var(--border); padding-bottom: 16px; margin-bottom: 24px; }
.header h1 { margin: 0 0 8px 0; }
.stats { font-size: 0.9em; opacity: 0.8; }
.message { margin-bottom: 16px; padding: 12px 16px; border-radius: 8px; border: 1px solid var(--border); }
.message.user { background: var(--msg-user-bg); }
.message.assistant { background: var(--msg-assistant-bg); }
.message .role { font-weight: bold; font-size: 0.85em; text-transform: uppercase; margin-bottom: 4px; }
.tool-call { background: var(--tool-bg); border-radius: 4px; padding: 8px 12px; margin-top: 8px; }
.tool-call summary { cursor: pointer; font-weight: bold; font-size: 0.9em; }
pre { background: var(--code-bg); color: var(--code-fg); padding: 12px; border-radius: 6px; overflow-x: auto; }
code { font-family: 'SF Mono', 'Fira Code', monospace; font-size: 0.9em; }
</style>
</head>
<body>
<div class="header">
<h1>Session: ` + html.EscapeString(session.ID) + `</h1>
<div class="stats">
<span>Model: ` + html.EscapeString(session.Model) + `</span> |
<span>Provider: ` + html.EscapeString(session.Provider) + `</span> |
<span>Date: ` + session.CreatedAt.UTC().Format("2006-01-02 15:04 UTC") + `</span> |
<span>Duration: ` + formatDuration(session.Stats.Duration) + `</span> |
<span>Tokens: ` + formatNumber(session.Stats.TotalTokens) + `</span> |
<span>Messages: ` + fmt.Sprintf("%d", session.Stats.TotalMessages) + `</span>
</div>
</div>
`)

	for _, msg := range session.Messages {
		role := html.EscapeString(msg.Role)
		content := html.EscapeString(msg.Content)

		b.WriteString(fmt.Sprintf(`<div class="message %s">`, role))
		b.WriteString(fmt.Sprintf(`<div class="role">%s</div>`, role))
		b.WriteString(fmt.Sprintf(`<div class="content"><p>%s</p></div>`, content))

		if msg.ToolName != "" && msg.ToolResult != "" {
			b.WriteString(`<details class="tool-call">`)
			b.WriteString(fmt.Sprintf(`<summary>Tool: %s</summary>`, html.EscapeString(msg.ToolName)))
			b.WriteString(fmt.Sprintf(`<pre><code>%s</code></pre>`, html.EscapeString(msg.ToolResult)))
			b.WriteString(`</details>`)
		}

		b.WriteString("</div>\n")
	}

	b.WriteString(`</body>
</html>`)

	return b.String()
}

// ExportJSON renders a session as full structured JSON suitable for re-import.
func ExportJSON(session *ExportedSession) string {
	if session == nil {
		return "{}"
	}

	data, err := json.MarshalIndent(session, "", "  ")
	if err != nil {
		return "{}"
	}
	return string(data)
}

// replayEntry is a single line in the JSONL replay format.
type replayEntry struct {
	Seq        int       `json:"seq"`
	Timestamp  time.Time `json:"timestamp"`
	DeltaMs    int64     `json:"delta_ms"`
	Role       string    `json:"role"`
	Content    string    `json:"content"`
	ToolName   string    `json:"tool_name,omitempty"`
	ToolResult string    `json:"tool_result,omitempty"`
	TokenCount int       `json:"token_count,omitempty"`
}

// ExportReplay renders a session as JSONL suitable for step-by-step replay.
func ExportReplay(session *ExportedSession) string {
	if session == nil {
		return ""
	}

	var b strings.Builder
	var prevTime time.Time

	for i, msg := range session.Messages {
		deltaMs := int64(0)
		if i > 0 && !prevTime.IsZero() && !msg.Timestamp.IsZero() {
			deltaMs = msg.Timestamp.Sub(prevTime).Milliseconds()
		}
		prevTime = msg.Timestamp

		entry := replayEntry{
			Seq:        i + 1,
			Timestamp:  msg.Timestamp,
			DeltaMs:    deltaMs,
			Role:       msg.Role,
			Content:    msg.Content,
			ToolName:   msg.ToolName,
			ToolResult: msg.ToolResult,
			TokenCount: msg.TokenCount,
		}

		data, err := json.Marshal(entry)
		if err != nil {
			continue
		}
		b.Write(data)
		b.WriteByte('\n')
	}

	return b.String()
}

// Import parses an exported session from the given data and format.
func Import(data string, format ExportFormat) (*ExportedSession, error) {
	switch format {
	case FormatJSON:
		return importJSON(data)
	case FormatReplay:
		return importReplay(data)
	case FormatClaude:
		return ImportFromClaude(data)
	case FormatAider:
		return ImportFromAider(data)
	case FormatCursor:
		return ImportFromCursor(data)
	case FormatOpenAI:
		return ImportFromOpenAI(data)
	default:
		return nil, fmt.Errorf("import not supported for format: %s", string(format))
	}
}

func importJSON(data string) (*ExportedSession, error) {
	var session ExportedSession
	if err := json.Unmarshal([]byte(data), &session); err != nil {
		return nil, fmt.Errorf("failed to parse JSON session: %w", err)
	}
	return &session, nil
}

func importReplay(data string) (*ExportedSession, error) {
	lines := strings.Split(strings.TrimSpace(data), "\n")
	if len(lines) == 0 {
		return nil, fmt.Errorf("empty replay data")
	}

	session := &ExportedSession{
		Messages: make([]ExportedMessage, 0, len(lines)),
	}

	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}

		var entry replayEntry
		if err := json.Unmarshal([]byte(line), &entry); err != nil {
			return nil, fmt.Errorf("failed to parse replay entry: %w", err)
		}

		msg := ExportedMessage{
			Role:       entry.Role,
			Content:    entry.Content,
			Timestamp:  entry.Timestamp,
			ToolName:   entry.ToolName,
			ToolResult: entry.ToolResult,
			TokenCount: entry.TokenCount,
		}
		session.Messages = append(session.Messages, msg)
	}

	// Derive basic metadata from messages.
	if len(session.Messages) > 0 {
		session.CreatedAt = session.Messages[0].Timestamp
	}
	session.Stats = CalculateStats(session.Messages)

	return session, nil
}

// claudeJSONLEntry represents an entry in Claude Code's JSONL session format.
type claudeJSONLEntry struct {
	Type       string `json:"type"`
	Role       string `json:"role"`
	Content    string `json:"content"`
	Model      string `json:"model,omitempty"`
	Timestamp  string `json:"timestamp,omitempty"`
	ToolName   string `json:"tool_name,omitempty"`
	ToolResult string `json:"tool_result,omitempty"`
}

// ImportFromClaude imports a session from Claude Code's JSONL format.
func ImportFromClaude(jsonlData string) (*ExportedSession, error) {
	lines := strings.Split(strings.TrimSpace(jsonlData), "\n")
	if len(lines) == 0 {
		return nil, fmt.Errorf("empty claude session data")
	}

	session := &ExportedSession{
		Provider: "anthropic",
		Messages: make([]ExportedMessage, 0, len(lines)),
		Metadata: map[string]string{"source": "claude-code"},
	}

	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}

		var entry claudeJSONLEntry
		if err := json.Unmarshal([]byte(line), &entry); err != nil {
			continue // Skip malformed lines.
		}

		if entry.Model != "" && session.Model == "" {
			session.Model = entry.Model
		}

		var ts time.Time
		if entry.Timestamp != "" {
			parsed, err := time.Parse(time.RFC3339, entry.Timestamp)
			if err == nil {
				ts = parsed
			}
		}

		msg := ExportedMessage{
			Role:       entry.Role,
			Content:    entry.Content,
			Timestamp:  ts,
			ToolName:   entry.ToolName,
			ToolResult: entry.ToolResult,
		}

		if msg.Role != "" {
			session.Messages = append(session.Messages, msg)
		}
	}

	if len(session.Messages) > 0 {
		session.CreatedAt = session.Messages[0].Timestamp
	}
	session.Stats = CalculateStats(session.Messages)

	return session, nil
}

// ImportFromAider imports a session from Aider's chat history format.
// Aider uses a Markdown-like format with role markers like "#### user" and "#### assistant".
func ImportFromAider(historyData string) (*ExportedSession, error) {
	if strings.TrimSpace(historyData) == "" {
		return nil, fmt.Errorf("empty aider history data")
	}

	session := &ExportedSession{
		Provider: "unknown",
		Messages: make([]ExportedMessage, 0),
		Metadata: map[string]string{"source": "aider"},
	}

	lines := strings.Split(historyData, "\n")
	var currentRole string
	var contentLines []string

	flushMessage := func() {
		if currentRole == "" {
			return
		}
		content := strings.TrimSpace(strings.Join(contentLines, "\n"))
		if content != "" {
			session.Messages = append(session.Messages, ExportedMessage{
				Role:    currentRole,
				Content: content,
			})
		}
		contentLines = nil
	}

	for _, line := range lines {
		trimmed := strings.TrimSpace(line)

		if strings.HasPrefix(trimmed, "#### ") {
			flushMessage()
			rolePart := strings.TrimPrefix(trimmed, "#### ")
			rolePart = strings.ToLower(strings.TrimSpace(rolePart))
			switch {
			case strings.Contains(rolePart, "user"):
				currentRole = "user"
			case strings.Contains(rolePart, "assistant"):
				currentRole = "assistant"
			case strings.Contains(rolePart, "system"):
				currentRole = "system"
			default:
				currentRole = rolePart
			}
			contentLines = nil
			continue
		}

		if currentRole != "" {
			contentLines = append(contentLines, line)
		}
	}
	flushMessage()

	session.Stats = CalculateStats(session.Messages)

	return session, nil
}

type cursorMsg struct {
	Speaker   string `json:"speaker"` // "human" | "ai" | "system" | "user" | "assistant"
	Text      string `json:"text"`
	BubbleID  string `json:"bubbleId,omitempty"`
	Timestamp int64  `json:"timestamp,omitempty"` // Unix ms
	ModelType string `json:"modelType,omitempty"`
}

type cursorJSON struct {
	ConversationID string      `json:"conversationId,omitempty"`
	Model          string      `json:"model,omitempty"`
	Messages       []cursorMsg `json:"messages,omitempty"`
}

// ImportFromCursor imports a session from Cursor's chat export JSON format.
func ImportFromCursor(data string) (*ExportedSession, error) {
	trimmed := strings.TrimSpace(data)
	if trimmed == "" {
		return nil, fmt.Errorf("empty cursor session data")
	}

	session := &ExportedSession{
		Provider: "cursor",
		Messages: make([]ExportedMessage, 0),
		Metadata: map[string]string{"source": "cursor"},
	}

	// Try object with "messages" field
	var cj cursorJSON
	if err := json.Unmarshal([]byte(trimmed), &cj); err == nil && len(cj.Messages) > 0 {
		session.ID = cj.ConversationID
		session.Model = cj.Model
		for _, m := range cj.Messages {
			role := normalizeCursorRole(m.Speaker)
			if role == "" || strings.TrimSpace(m.Text) == "" {
				continue
			}
			var ts time.Time
			if m.Timestamp > 0 {
				ts = time.UnixMilli(m.Timestamp)
			}
			session.Messages = append(session.Messages, ExportedMessage{
				Role:      role,
				Content:   m.Text,
				Timestamp: ts,
			})
		}
	} else {
		// Try array of cursorMsg
		var msgs []cursorMsg
		if err := json.Unmarshal([]byte(trimmed), &msgs); err == nil && len(msgs) > 0 {
			for _, m := range msgs {
				role := normalizeCursorRole(m.Speaker)
				if role == "" || strings.TrimSpace(m.Text) == "" {
					continue
				}
				var ts time.Time
				if m.Timestamp > 0 {
					ts = time.UnixMilli(m.Timestamp)
				}
				session.Messages = append(session.Messages, ExportedMessage{
					Role:      role,
					Content:   m.Text,
					Timestamp: ts,
				})
			}
		} else {
			return nil, fmt.Errorf("failed to parse cursor session JSON")
		}
	}

	if len(session.Messages) == 0 {
		return nil, fmt.Errorf("no valid messages found in cursor session")
	}

	session.CreatedAt = session.Messages[0].Timestamp
	session.Stats = CalculateStats(session.Messages)
	return session, nil
}

func normalizeCursorRole(speaker string) string {
	switch strings.ToLower(strings.TrimSpace(speaker)) {
	case "human", "user":
		return "user"
	case "ai", "assistant", "bot":
		return "assistant"
	case "system":
		return "system"
	default:
		return "user"
	}
}

type openAIChatJSON struct {
	Model    string      `json:"model,omitempty"`
	Messages []openAIMsg `json:"messages"`
}

type openAIMsg struct {
	Role       string           `json:"role"`
	Content    string           `json:"content"`
	Name       string           `json:"name,omitempty"`
	ToolCalls  []openAIToolCall `json:"tool_calls,omitempty"`
	ToolCallID string           `json:"tool_call_id,omitempty"`
}

type openAIToolCall struct {
	ID       string `json:"id"`
	Type     string `json:"type"`
	Function struct {
		Name      string `json:"name"`
		Arguments string `json:"arguments"`
	} `json:"function"`
}

// ImportFromOpenAI imports a session from standard OpenAI Chat Completions JSON.
func ImportFromOpenAI(data string) (*ExportedSession, error) {
	trimmed := strings.TrimSpace(data)
	if trimmed == "" {
		return nil, fmt.Errorf("empty openai session data")
	}

	session := &ExportedSession{
		Provider: "openai",
		Messages: make([]ExportedMessage, 0),
		Metadata: map[string]string{"source": "openai"},
	}

	var chatJSON openAIChatJSON
	if err := json.Unmarshal([]byte(trimmed), &chatJSON); err == nil && len(chatJSON.Messages) > 0 {
		session.Model = chatJSON.Model
		for _, m := range chatJSON.Messages {
			msg := ExportedMessage{
				Role:    m.Role,
				Content: m.Content,
			}
			if len(m.ToolCalls) > 0 {
				msg.ToolName = m.ToolCalls[0].Function.Name
			}
			if m.Role == "tool" {
				msg.ToolResult = m.Content
			}
			session.Messages = append(session.Messages, msg)
		}
	} else {
		var rawMsgs []openAIMsg
		if err := json.Unmarshal([]byte(trimmed), &rawMsgs); err == nil && len(rawMsgs) > 0 {
			for _, m := range rawMsgs {
				msg := ExportedMessage{
					Role:    m.Role,
					Content: m.Content,
				}
				if len(m.ToolCalls) > 0 {
					msg.ToolName = m.ToolCalls[0].Function.Name
				}
				if m.Role == "tool" {
					msg.ToolResult = m.Content
				}
				session.Messages = append(session.Messages, msg)
			}
		} else {
			return nil, fmt.Errorf("failed to parse openai session JSON")
		}
	}

	if len(session.Messages) == 0 {
		return nil, fmt.Errorf("no valid messages found in openai session")
	}

	session.Stats = CalculateStats(session.Messages)
	return session, nil
}

func formatOpenAI(session *ExportedSession) (string, error) {
	if session == nil {
		return "", fmt.Errorf("session is nil")
	}

	out := openAIChatJSON{
		Model:    session.Model,
		Messages: make([]openAIMsg, 0, len(session.Messages)),
	}

	for _, m := range session.Messages {
		out.Messages = append(out.Messages, openAIMsg{
			Role:    m.Role,
			Content: m.Content,
		})
	}

	data, err := json.MarshalIndent(out, "", "  ")
	if err != nil {
		return "", fmt.Errorf("failed to format OpenAI JSON: %w", err)
	}
	return string(data), nil
}

// secretPatterns defines regex patterns for sensitive data detection.
var secretPatterns = []*regexp.Regexp{
	regexp.MustCompile(`(?i)(api[_-]?key|apikey)\s*[:=]\s*["']?([A-Za-z0-9_\-]{16,})["']?`),
	regexp.MustCompile(`(?i)(secret|token|password|passwd|pwd)\s*[:=]\s*["']?([^\s"']{8,})["']?`),
	regexp.MustCompile(`(?i)(bearer)\s+([A-Za-z0-9_\-\.]{20,})`),
	regexp.MustCompile(`sk-[A-Za-z0-9_\-]{20,}`),
	regexp.MustCompile(`ghp_[A-Za-z0-9]{36,}`),
	regexp.MustCompile(`gho_[A-Za-z0-9]{36,}`),
	regexp.MustCompile(`github_pat_[A-Za-z0-9_]{22,}`),
	regexp.MustCompile(`xox[baprs]-[A-Za-z0-9\-]{10,}`),
	regexp.MustCompile(`AKIA[0-9A-Z]{16}`),
	regexp.MustCompile(`-----BEGIN (RSA |EC |DSA )?PRIVATE KEY-----`),
}

// RedactSensitive returns a copy of the session with sensitive values replaced by [REDACTED].
func RedactSensitive(session *ExportedSession) *ExportedSession {
	if session == nil {
		return nil
	}

	redacted := *session
	redacted.Messages = make([]ExportedMessage, len(session.Messages))

	for i, msg := range session.Messages {
		redacted.Messages[i] = ExportedMessage{
			Role:       msg.Role,
			Content:    redactString(msg.Content),
			Timestamp:  msg.Timestamp,
			ToolName:   msg.ToolName,
			ToolResult: redactString(msg.ToolResult),
			TokenCount: msg.TokenCount,
		}
	}

	if session.Metadata != nil {
		redacted.Metadata = make(map[string]string, len(session.Metadata))
		for k, v := range session.Metadata {
			redacted.Metadata[k] = redactString(v)
		}
	}

	return &redacted
}

func redactString(s string) string {
	if s == "" {
		return s
	}
	result := s
	for _, pat := range secretPatterns {
		result = pat.ReplaceAllStringFunc(result, func(match string) string {
			return "[REDACTED]"
		})
	}
	return result
}

// GenerateShareLink creates a deterministic share ID from the session content hash.
func GenerateShareLink(session *ExportedSession) string {
	if session == nil {
		return ""
	}

	h := sha256.New()
	h.Write([]byte(session.ID))
	h.Write([]byte(session.Model))
	h.Write([]byte(session.Provider))
	h.Write([]byte(session.CreatedAt.UTC().Format(time.RFC3339)))
	for _, msg := range session.Messages {
		h.Write([]byte(msg.Role))
		h.Write([]byte(msg.Content))
	}

	hash := hex.EncodeToString(h.Sum(nil))
	return fmt.Sprintf("graycode://share/%s", hash[:16])
}

// CalculateStats computes session statistics from a slice of messages.
func CalculateStats(messages []ExportedMessage) SessionExportStats {
	stats := SessionExportStats{
		TotalMessages: len(messages),
	}

	for _, m := range messages {
		switch m.Role {
		case "user":
			stats.UserMessages++
		case "assistant":
			stats.AssistantMessages++
		}
		if m.ToolName != "" {
			stats.ToolCalls++
		}
		stats.TotalTokens += m.TokenCount
	}

	if len(messages) >= 2 {
		first := messages[0].Timestamp
		last := messages[len(messages)-1].Timestamp
		if !first.IsZero() && !last.IsZero() {
			stats.Duration = last.Sub(first)
		}
	}

	return stats
}

// Export renders a session in the specified format, optionally redacting secrets.
func Export(s *Session, format string, redact bool) ([]byte, error) {
	if s == nil {
		return nil, fmt.Errorf("session is nil")
	}
	if redact {
		s = redactSessionMessages(s)
	}
	switch format {
	case "json":
		return exportSessionJSON(s)
	case "md":
		return exportSessionMarkdown(s)
	default:
		return nil, fmt.Errorf("unsupported export format: %s", format)
	}
}

// RedactSecrets replaces detected secrets in text with [REDACTED].
func RedactSecrets(text string) string {
	return redactString(text)
}

func redactSessionMessages(s *Session) *Session {
	cp := *s
	cp.Messages = make([]Message, len(s.Messages))
	for i, msg := range s.Messages {
		cp.Messages[i] = Message{
			Role:    msg.Role,
			Content: redactString(msg.Content),
		}
		if len(msg.ToolUse) > 0 {
			cp.Messages[i].ToolUse = make([]ToolCall, len(msg.ToolUse))
			for j, tc := range msg.ToolUse {
				cp.Messages[i].ToolUse[j] = ToolCall{
					ID:        tc.ID,
					Name:      tc.Name,
					Arguments: redactToolArguments(tc.Arguments),
				}
			}
		}
		if len(msg.ToolResults) > 0 {
			cp.Messages[i].ToolResults = make([]ToolResult, len(msg.ToolResults))
			for j, tr := range msg.ToolResults {
				cp.Messages[i].ToolResults[j] = ToolResult{
					ToolUseID: tr.ToolUseID,
					Content:   redactString(tr.Content),
					IsError:   tr.IsError,
				}
			}
		}
	}
	return &cp
}

func redactToolArguments(args map[string]interface{}) map[string]interface{} {
	if args == nil {
		return nil
	}
	out := make(map[string]interface{}, len(args))
	for k, v := range args {
		out[k] = redactValue(v)
	}
	return out
}

// redactValue recursively redacts secrets in string values, descending into
// nested maps and slices. Tool arguments are JSON-decoded, so a secret can be
// buried inside a structured value (e.g. {"env": {"API_TOKEN": "…"}} or
// {"keys": ["sk-…"]}); redacting only top-level strings would leak those.
func redactValue(v interface{}) interface{} {
	switch val := v.(type) {
	case string:
		return redactString(val)
	case map[string]interface{}:
		out := make(map[string]interface{}, len(val))
		for k, item := range val {
			out[k] = redactValue(item)
		}
		return out
	case []interface{}:
		out := make([]interface{}, len(val))
		for i, item := range val {
			out[i] = redactValue(item)
		}
		return out
	default:
		return v
	}
}

func exportSessionJSON(s *Session) ([]byte, error) {
	return json.MarshalIndent(s, "", "  ")
}

func exportSessionMarkdown(s *Session) ([]byte, error) {
	var b strings.Builder

	b.WriteString("# Session\n\n")
	b.WriteString(fmt.Sprintf("- **ID:** %s\n", s.ID))
	b.WriteString(fmt.Sprintf("- **Model:** %s\n", s.Model))
	b.WriteString(fmt.Sprintf("- **Provider:** %s\n", s.Provider))
	if s.CWD != "" {
		b.WriteString(fmt.Sprintf("- **CWD:** %s\n", s.CWD))
	}
	if s.Name != "" {
		b.WriteString(fmt.Sprintf("- **Name:** %s\n", s.Name))
	}
	b.WriteString(fmt.Sprintf("- **Created:** %s\n", s.CreatedAt.Format("2006-01-02 15:04:05")))
	b.WriteString(fmt.Sprintf("- **Updated:** %s\n", s.UpdatedAt.Format("2006-01-02 15:04:05")))
	b.WriteString("\n## Messages\n\n")

	for _, msg := range s.Messages {
		b.WriteString(fmt.Sprintf("### %s\n\n", msg.Role))
		if msg.Content != "" {
			if msg.Role == "assistant" {
				b.WriteString(fmt.Sprintf("```\n%s\n```\n\n", msg.Content))
			} else {
				b.WriteString(msg.Content + "\n\n")
			}
		}
		if len(msg.ToolUse) > 0 {
			for _, tc := range msg.ToolUse {
				b.WriteString(fmt.Sprintf("**Tool Call:** %s\n", tc.Name))
				if len(tc.Arguments) > 0 {
					argJSON, _ := json.MarshalIndent(tc.Arguments, "", "  ")
					b.WriteString(fmt.Sprintf("```json\n%s\n```\n\n", string(argJSON)))
				}
			}
		}
		for _, tr := range msg.ToolResults {
			b.WriteString("**Tool Result:**\n")
			b.WriteString(fmt.Sprintf("```\n%s\n```\n\n", tr.Content))
		}
	}

	return []byte(b.String()), nil
}

// formatDuration renders a duration in a human-readable short form.
func formatDuration(d time.Duration) string {
	if d == 0 {
		return "0s"
	}
	if d < time.Minute {
		return fmt.Sprintf("%ds", int(d.Seconds()))
	}
	if d < time.Hour {
		mins := int(d.Minutes())
		secs := int(d.Seconds()) % 60
		if secs == 0 {
			return fmt.Sprintf("%dm", mins)
		}
		return fmt.Sprintf("%dm%ds", mins, secs)
	}
	hours := int(d.Hours())
	mins := int(d.Minutes()) % 60
	return fmt.Sprintf("%dh%dm", hours, mins)
}

// formatNumber adds comma separators to large numbers.
func formatNumber(n int) string {
	if n < 0 {
		return "-" + formatNumber(-n)
	}
	s := fmt.Sprintf("%d", n)
	if len(s) <= 3 {
		return s
	}

	var result strings.Builder
	remainder := len(s) % 3
	if remainder > 0 {
		result.WriteString(s[:remainder])
		if len(s) > remainder {
			result.WriteByte(',')
		}
	}
	for i := remainder; i < len(s); i += 3 {
		if i > remainder {
			result.WriteByte(',')
		}
		result.WriteString(s[i : i+3])
	}
	return result.String()
}
