package tool

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
	"time"
)

// MessageRouting describes a routed message between agents.
type MessageRouting struct {
	Sender  string `json:"sender"`
	Target  string `json:"target"`
	Summary string `json:"summary,omitempty"`
	Content string `json:"content,omitempty"`
}

// Mailbox stores messages for agents and teammates.
type Mailbox struct {
	mu       sync.RWMutex
	messages []MessageRouting
}

var globalMailbox = &Mailbox{}

func GetMailbox() *Mailbox { return globalMailbox }

func (m *Mailbox) Send(msg MessageRouting) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.messages = append(m.messages, msg)
}

func (m *Mailbox) ReadFor(target string) []MessageRouting {
	m.mu.RLock()
	defer m.mu.RUnlock()
	var out []MessageRouting
	for _, msg := range m.messages {
		if msg.Target == target || msg.Target == "*" {
			out = append(out, msg)
		}
	}
	return out
}

// SendMessageTool sends a message between teammates in a swarm.
type SendMessageTool struct{}

func (SendMessageTool) Name() string        { return "SendMessage" }
func (SendMessageTool) Aliases() []string   { return []string{"send_message"} }
func (SendMessageTool) Description() string { return "Send a message to a teammate in a team/swarm" }
func (SendMessageTool) Parameters() map[string]interface{} {
	return map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"to": map[string]interface{}{
				"type":        "string",
				"description": "Recipient: teammate name, \"*\" for broadcast to all teammates, or \"user\" for the human",
			},
			"message": map[string]interface{}{
				"type":        "string",
				"description": "The message content. Supports markdown formatting.",
			},
			"summary": map[string]interface{}{
				"type":        "string",
				"description": "A 5-10 word summary shown as a preview in the UI",
			},
			"attachments": map[string]interface{}{
				"type":        "array",
				"items":       map[string]interface{}{"type": "string"},
				"description": "Optional file paths to attach (images, diffs, logs)",
			},
			"status": map[string]interface{}{
				"type":        "string",
				"enum":        []string{"normal", "proactive"},
				"description": "Use 'proactive' when surfacing something the user hasn't asked for",
			},
		},
		"required": []string{"to", "message"},
	}
}

func (SendMessageTool) Execute(ctx context.Context, input json.RawMessage) (string, error) {
	var p struct {
		To          string   `json:"to"`
		Message     string   `json:"message"`
		Summary     string   `json:"summary"`
		Attachments []string `json:"attachments"`
		Status      string   `json:"status"`
	}
	if err := json.Unmarshal(input, &p); err != nil {
		return "", err
	}
	if p.To == "" {
		return "", fmt.Errorf("'to' is required")
	}
	if p.Message == "" {
		return "", fmt.Errorf("'message' is required")
	}

	routing := MessageRouting{
		Sender:  "assistant",
		Target:  p.To,
		Summary: p.Summary,
		Content: p.Message,
	}
	globalMailbox.Send(routing)

	out, _ := json.Marshal(map[string]any{
		"message":     p.Message,
		"attachments": p.Attachments,
		"sentAt":      time.Now().UTC().Format(time.RFC3339),
	})
	return string(out), nil
}

// SleepTool waits for a specified duration without holding a shell process.
type SleepTool struct{}

func (SleepTool) Name() string        { return "Sleep" }
func (SleepTool) Aliases() []string   { return []string{"sleep"} }
func (SleepTool) Description() string { return "Wait for a specified duration" }
func (SleepTool) Parameters() map[string]interface{} {
	return map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"duration_ms": map[string]interface{}{
				"type":        "number",
				"description": "Duration to sleep in milliseconds (max 300000 = 5 minutes)",
			},
		},
		"required": []string{"duration_ms"},
	}
}

func (SleepTool) Execute(ctx context.Context, input json.RawMessage) (string, error) {
	var p struct {
		DurationMs int64 `json:"duration_ms"`
	}
	if err := json.Unmarshal(input, &p); err != nil {
		return "", err
	}
	if p.DurationMs <= 0 {
		return "", fmt.Errorf("duration_ms must be positive")
	}
	const maxSleep = 300_000
	if p.DurationMs > maxSleep {
		p.DurationMs = maxSleep
	}

	timer := time.NewTimer(time.Duration(p.DurationMs) * time.Millisecond)
	defer timer.Stop()

	select {
	case <-timer.C:
		return fmt.Sprintf("Slept for %d ms", p.DurationMs), nil
	case <-ctx.Done():
		return "Sleep interrupted", nil
	}
}
