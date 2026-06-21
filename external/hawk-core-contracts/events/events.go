package events

import "time"

// ToolEvent represents a normalized tool event emitted by Hawk workflows.
type ToolEvent struct {
	ToolName   string                 `json:"tool_name"`
	ToolInput  map[string]interface{} `json:"tool_input,omitempty"`
	CWD        string                 `json:"cwd,omitempty"`
	Timestamp  time.Time              `json:"timestamp"`
	SessionID  string                 `json:"session_id,omitempty"`
	Transcript string                 `json:"transcript,omitempty"`
}

// TraceEvent represents a normalized trace record for model/runtime activity.
type TraceEvent struct {
	ID        string            `json:"id"`
	Name      string            `json:"name"`
	Input     string            `json:"input,omitempty"`
	Output    string            `json:"output,omitempty"`
	Model     string            `json:"model,omitempty"`
	StartTime time.Time         `json:"start_time"`
	EndTime   time.Time         `json:"end_time,omitempty"`
	Metadata  map[string]string `json:"metadata,omitempty"`
	Usage     *UsageInfo        `json:"usage,omitempty"`
}

// UsageInfo captures token and cost information for a trace event.
type UsageInfo struct {
	PromptTokens     int     `json:"prompt_tokens"`
	CompletionTokens int     `json:"completion_tokens"`
	TotalTokens      int     `json:"total_tokens"`
	CostUSD          float64 `json:"cost_usd,omitempty"`
}
