package cmd

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/GrayCodeAI/hawk/internal/session"
	"github.com/GrayCodeAI/hawk/internal/storage"
)

func writeRedactedChatMarkdownExport(m *chatModel) (string, error) {
	data, err := redactedChatMarkdownExport(m)
	if err != nil {
		return "", err
	}
	exportDir := filepath.Join(storage.StateDir(), "exports")
	if err := os.MkdirAll(exportDir, 0o700); err != nil {
		return "", err
	}
	_ = os.Chmod(exportDir, 0o700) // #nosec G302 -- directory needs owner execute bit for traversal; not a file permission
	exportPath := filepath.Join(exportDir, m.sessionID+".md")
	if err := os.WriteFile(exportPath, data, 0o600); err != nil {
		return "", err
	}
	return exportPath, nil
}

func redactedChatMarkdownExport(m *chatModel) ([]byte, error) {
	if m == nil || m.session == nil {
		return nil, fmt.Errorf("no active session")
	}
	msgs := session.FromRuntimeMessages(m.session.RawMessages())
	if len(msgs) == 0 {
		msgs = displayMessagesToSessionMessages(m.messages)
	}
	return session.Export(&session.Session{
		ID:        m.sessionID,
		Model:     m.session.Model(),
		Provider:  m.session.Provider(),
		Messages:  msgs,
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}, "md", true)
}

func displayMessagesToSessionMessages(messages []displayMsg) []session.Message {
	out := make([]session.Message, 0, len(messages))
	for _, msg := range messages {
		switch msg.role {
		case "user", "assistant", "system":
			out = append(out, session.Message{Role: msg.role, Content: msg.content})
		}
	}
	return out
}

// exportSession exports the current session in the specified format (md, json, or txt).
// Returns the file path and any error.
func exportSession(m *chatModel, format string) (string, error) {
	switch format {
	case "json":
		return exportSessionJSON(m)
	case "txt":
		return exportSessionTxt(m)
	default:
		return exportSessionMarkdown(m)
	}
}

// exportSessionMarkdown exports the session as a Markdown file.
func exportSessionMarkdown(m *chatModel) (string, error) {
	if m == nil || m.session == nil {
		return "", fmt.Errorf("no active session")
	}
	msgs := session.FromRuntimeMessages(m.session.RawMessages())
	if len(msgs) == 0 {
		msgs = displayMessagesToSessionMessages(m.messages)
	}
	data, err := session.Export(&session.Session{
		ID:        m.sessionID,
		Model:     m.session.Model(),
		Provider:  m.session.Provider(),
		Messages:  msgs,
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}, "md", true)
	if err != nil {
		return "", err
	}
	exportDir := filepath.Join(storage.StateDir(), "exports")
	if err := os.MkdirAll(exportDir, 0o700); err != nil {
		return "", err
	}
	_ = os.Chmod(exportDir, 0o700)
	exportPath := filepath.Join(exportDir, m.sessionID+".md")
	if err := os.WriteFile(exportPath, data, 0o600); err != nil {
		return "", err
	}
	return exportPath, nil
}

// exportSessionJSON exports the session as a structured JSON file.
func exportSessionJSON(m *chatModel) (string, error) {
	if m == nil || m.session == nil {
		return "", fmt.Errorf("no active session")
	}
	type exportedMessage struct {
		Role    string `json:"role"`
		Content string `json:"content"`
	}
	type exportedSession struct {
		SessionID    string            `json:"session_id"`
		Model        string            `json:"model,omitempty"`
		Provider     string            `json:"provider,omitempty"`
		ExportedAt   string            `json:"exported_at"`
		MessageCount int               `json:"message_count"`
		Messages     []exportedMessage `json:"messages"`
	}
	var msgs []exportedMessage
	for _, msg := range m.messages {
		if msg.role == "welcome" || msg.content == "" {
			continue
		}
		msgs = append(msgs, exportedMessage{
			Role:    msg.role,
			Content: msg.content,
		})
	}
	export := exportedSession{
		SessionID:    m.sessionID,
		Model:        m.session.Model(),
		Provider:     m.session.Provider(),
		ExportedAt:   time.Now().Format(time.RFC3339),
		MessageCount: len(msgs),
		Messages:     msgs,
	}
	data, err := json.MarshalIndent(export, "", "  ")
	if err != nil {
		return "", fmt.Errorf("failed to marshal session: %w", err)
	}
	exportDir := filepath.Join(storage.StateDir(), "exports")
	if err := os.MkdirAll(exportDir, 0o700); err != nil {
		return "", err
	}
	_ = os.Chmod(exportDir, 0o700)
	exportPath := filepath.Join(exportDir, m.sessionID+".json")
	if err := os.WriteFile(exportPath, data, 0o600); err != nil {
		return "", err
	}
	return exportPath, nil
}

// exportSessionTxt exports the session as a plain text file.
func exportSessionTxt(m *chatModel) (string, error) {
	if m == nil || m.session == nil {
		return "", fmt.Errorf("no active session")
	}
	var b strings.Builder
	b.WriteString(fmt.Sprintf("Hawk Session: %s\n", m.sessionID))
	b.WriteString(fmt.Sprintf("Model: %s/%s\n", m.session.Provider(), m.session.Model()))
	b.WriteString(fmt.Sprintf("Exported: %s\n\n", time.Now().Format(time.RFC3339)))
	b.WriteString(strings.Repeat("=", 60) + "\n\n")
	for _, msg := range m.messages {
		switch msg.role {
		case "welcome":
			continue
		case "user":
			b.WriteString(fmt.Sprintf("[User]\n%s\n\n", msg.content))
		case "assistant":
			b.WriteString(fmt.Sprintf("[Assistant]\n%s\n\n", msg.content))
		case "system":
			b.WriteString(fmt.Sprintf("[System]\n%s\n\n", msg.content))
		case "error":
			b.WriteString(fmt.Sprintf("[Error]\n%s\n\n", msg.content))
		case "tool_use":
			b.WriteString(fmt.Sprintf("[Tool: %s]\n\n", msg.content))
		case "tool_result":
			b.WriteString(fmt.Sprintf("[Tool Result]\n%s\n\n", msg.content))
		default:
			if msg.content != "" {
				b.WriteString(fmt.Sprintf("%s\n\n", msg.content))
			}
		}
	}
	exportDir := filepath.Join(storage.StateDir(), "exports")
	if err := os.MkdirAll(exportDir, 0o700); err != nil {
		return "", err
	}
	_ = os.Chmod(exportDir, 0o700)
	exportPath := filepath.Join(exportDir, m.sessionID+".txt")
	if err := os.WriteFile(exportPath, []byte(b.String()), 0o600); err != nil {
		return "", err
	}
	return exportPath, nil
}
