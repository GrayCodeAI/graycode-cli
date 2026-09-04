package status

import (
	"encoding/json"
	"os"
	"strings"
	"time"
)

// Snapshot is a redacted, machine-readable view of the current runtime.
// Callers should populate only values they already own; building a snapshot
// must not start providers, MCP servers, or network operations.
type Snapshot struct {
	SchemaVersion   string           `json:"schema_version"`
	GraycodeVersion string           `json:"graycode_version,omitempty"`
	SessionID       string           `json:"session_id,omitempty"`
	Workspace       string           `json:"workspace,omitempty"`
	GitBranch       string           `json:"git_branch,omitempty"`
	Provider        string           `json:"provider,omitempty"`
	Model           string           `json:"model,omitempty"`
	Permission      PermissionStatus `json:"permission"`
	Budgets         BudgetStatus     `json:"budgets"`
	Subagents       []SubagentStatus `json:"active_subagents,omitempty"`
	Sessions        ComponentStatus  `json:"sessions"`
	MCP             ComponentStatus  `json:"mcp"`
	Skills          ComponentStatus  `json:"skills"`
	Hooks           ComponentStatus  `json:"hooks"`
	Recovery        string           `json:"session_recovery_state,omitempty"`
	ActiveGoal      string           `json:"active_goal,omitempty"`
	Swift           ComponentStatus  `json:"swift"`
	Warnings        []string         `json:"warnings,omitempty"`
	GeneratedAt     time.Time        `json:"generated_at"`
}

type PermissionStatus struct {
	Mode           string `json:"mode,omitempty"`
	AutonomyTier   string `json:"autonomy_tier,omitempty"`
	SandboxMode    string `json:"sandbox_mode,omitempty"`
	SandboxBackend string `json:"sandbox_backend,omitempty"`
	EffectiveRules int    `json:"effective_rules,omitempty"`
	SecretRedacted bool   `json:"secret_values_redacted"`
}

type BudgetStatus struct {
	TurnsUsed    int     `json:"turns_used,omitempty"`
	TurnsLimit   int     `json:"turns_limit,omitempty"`
	ToolsUsed    int     `json:"tool_calls_used,omitempty"`
	ToolsLimit   int     `json:"tool_calls_limit,omitempty"`
	TokensUsed   int     `json:"tokens_used,omitempty"`
	TokensLimit  int     `json:"tokens_limit,omitempty"`
	CostUSD      float64 `json:"cost_usd,omitempty"`
	CostLimitUSD float64 `json:"cost_limit_usd,omitempty"`
}

type SubagentStatus struct {
	ID        string `json:"id,omitempty"`
	ParentID  string `json:"parent_id,omitempty"`
	State     string `json:"state,omitempty"`
	Model     string `json:"model,omitempty"`
	Mode      string `json:"mode,omitempty"`
	Workspace string `json:"workspace,omitempty"`
}

type ComponentStatus struct {
	Configured int    `json:"configured,omitempty"`
	Active     int    `json:"active,omitempty"`
	State      string `json:"state,omitempty"`
}

// New returns a snapshot with the stable schema version and a redaction marker.
func New() Snapshot {
	return Snapshot{SchemaVersion: "1", GeneratedAt: time.Now().UTC(), Permission: PermissionStatus{SecretRedacted: true}}
}

// JSON returns stable indented JSON for CLI and API consumers.
func (s Snapshot) JSON() ([]byte, error) {
	return json.MarshalIndent(s, "", "  ")
}

// Workspace returns the current working directory without reading project
// configuration or contacting any external service.
func Workspace() string {
	wd, err := os.Getwd()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(wd)
}
