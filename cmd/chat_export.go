package cmd

import (
	"fmt"
	"os"
	"path/filepath"
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
	_ = os.Chmod(exportDir, 0o700)
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
