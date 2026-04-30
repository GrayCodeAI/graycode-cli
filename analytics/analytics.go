package analytics

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"
)

// Event represents an analytics event.
type Event struct {
	Name       string                 `json:"name"`
	Timestamp  time.Time              `json:"timestamp"`
	SessionID  string                 `json:"session_id,omitempty"`
	Properties map[string]interface{} `json:"properties,omitempty"`
}

func analyticsDir() string {
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".hawk", "analytics")
}

// LogEvent logs an analytics event.
func LogEvent(name, sessionID string, properties map[string]interface{}) {
	event := Event{
		Name:       name,
		Timestamp:  time.Now(),
		SessionID:  sessionID,
		Properties: properties,
	}
	_ = os.MkdirAll(analyticsDir(), 0o755)
	data, _ := json.Marshal(event)
	f, _ := os.OpenFile(filepath.Join(analyticsDir(), "events.jsonl"), os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if f != nil {
		defer f.Close()
		_, _ = f.Write(data)
		_, _ = f.WriteString("\n")
	}
}

// SessionTrace tracks session lifecycle events.
type SessionTrace struct {
	SessionID    string    `json:"session_id"`
	StartTime    time.Time `json:"start_time"`
	EndTime      time.Time `json:"end_time,omitempty"`
	Provider     string    `json:"provider"`
	Model        string    `json:"model"`
	MessageCount int       `json:"message_count"`
	ToolCalls    int       `json:"tool_calls"`
	CostUSD      float64   `json:"cost_usd"`
}

// SaveTrace saves a session trace.
func SaveTrace(t *SessionTrace) error {
	_ = os.MkdirAll(analyticsDir(), 0o755)
	data, _ := json.Marshal(t)
	f, err := os.OpenFile(filepath.Join(analyticsDir(), "traces.jsonl"), os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		return err
	}
	defer f.Close()
	if _, err := f.Write(data); err != nil {
		return err
	}
	_, _ = f.WriteString("\n")
	return nil
}

// GetTraces returns all session traces.
func GetTraces() ([]*SessionTrace, error) {
	data, err := os.ReadFile(filepath.Join(analyticsDir(), "traces.jsonl"))
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	var traces []*SessionTrace
	for _, line := range splitLines(string(data)) {
		if line == "" {
			continue
		}
		var t SessionTrace
		if err := json.Unmarshal([]byte(line), &t); err == nil {
			traces = append(traces, &t)
		}
	}
	return traces, nil
}

func splitLines(s string) []string {
	var out []string
	start := 0
	for i := 0; i < len(s); i++ {
		if s[i] == '\n' {
			out = append(out, s[start:i])
			start = i + 1
		}
	}
	if start < len(s) {
		out = append(out, s[start:])
	}
	return out
}

// Summary returns a formatted analytics summary.
func Summary() string {
	traces, err := GetTraces()
	if err != nil || len(traces) == 0 {
		return "No analytics data available."
	}
	var totalCost float64
	var totalMessages int
	var totalToolCalls int
	providerCount := make(map[string]int)
	for _, t := range traces {
		totalCost += t.CostUSD
		totalMessages += t.MessageCount
		totalToolCalls += t.ToolCalls
		providerCount[t.Provider]++
	}
	return fmt.Sprintf("Sessions: %d\nTotal cost: $%.4f\nMessages: %d\nTool calls: %d\nProviders: %v",
		len(traces), totalCost, totalMessages, totalToolCalls, providerCount)
}
