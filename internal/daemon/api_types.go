package daemon

import "time"

// ErrorResponse is the standard error envelope.
type ErrorResponse struct {
	Error   string `json:"error"`
	Code    string `json:"code,omitempty"`
	Details string `json:"details,omitempty"`
}

// PaginatedResponse wraps paginated list results.
type PaginatedResponse struct {
	Data    interface{} `json:"data"`
	Total   int         `json:"total"`
	Offset  int         `json:"offset"`
	Limit   int         `json:"limit"`
	HasMore bool        `json:"has_more"`
}

// SessionDetailResponse is the response for GET /v1/sessions/{id}.
type SessionDetailResponse struct {
	ID           string    `json:"id"`
	CreatedAt    time.Time `json:"created_at"`
	UpdatedAt    time.Time `json:"updated_at"`
	Model        string    `json:"model"`
	Provider     string    `json:"provider"`
	Agent        string    `json:"agent,omitempty"`
	CWD          string    `json:"cwd"`
	Name         string    `json:"name"`
	MessageCount int       `json:"message_count"`
	ToolCalls    int       `json:"tool_calls"`
}

// MessageResponse is a message in GET /v1/sessions/{id}/messages.
type MessageResponse struct {
	Role       string      `json:"role"`
	Content    string      `json:"content,omitempty"`
	ToolUse    interface{} `json:"tool_use,omitempty"`
	ToolResult interface{} `json:"tool_results,omitempty"`
}

// StatsResponse is the response for GET /v1/stats.
type StatsResponse struct {
	TotalSessions  int             `json:"total_sessions"`
	TotalMessages  int             `json:"total_messages"`
	TotalToolCalls int             `json:"total_tool_calls"`
	TotalCostUSD   float64         `json:"total_cost_usd"`
	ActiveDays     int             `json:"active_days"`
	Models         []ModelStatResp `json:"models"`
}

// ModelStatResp is per-model statistics within StatsResponse.
type ModelStatResp struct {
	Model    string  `json:"model"`
	Requests int     `json:"requests"`
	CostUSD  float64 `json:"cost_usd"`
}
