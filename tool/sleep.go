package tool

import (
	"context"
	"encoding/json"
	"fmt"
	"time"
)

// SleepTool pauses execution for a specified duration.
type SleepTool struct{}

func (SleepTool) Name() string      { return "Sleep" }
func (SleepTool) Aliases() []string { return []string{"sleep"} }
func (SleepTool) Description() string {
	return "Pause execution for a specified number of seconds."
}
func (SleepTool) Parameters() map[string]interface{} {
	return map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"seconds": map[string]interface{}{"type": "number", "description": "Duration to sleep in seconds (max 300)"},
		},
		"required": []string{"seconds"},
	}
}

func (SleepTool) Execute(ctx context.Context, input json.RawMessage) (string, error) {
	var p struct {
		Seconds float64 `json:"seconds"`
	}
	if err := json.Unmarshal(input, &p); err != nil {
		return "", err
	}
	if p.Seconds <= 0 {
		return "", fmt.Errorf("seconds must be positive")
	}
	if p.Seconds > 300 {
		p.Seconds = 300
	}

	dur := time.Duration(p.Seconds * float64(time.Second))
	select {
	case <-time.After(dur):
		return fmt.Sprintf("Slept for %.1f seconds.", p.Seconds), nil
	case <-ctx.Done():
		return "Sleep interrupted.", ctx.Err()
	}
}
