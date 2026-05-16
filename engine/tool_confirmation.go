package engine

import "strings"

// ToolRisk classifies the risk level of a tool invocation.
type ToolRisk int

const (
	RiskNone   ToolRisk = iota // auto-approve silently
	RiskLow                    // auto-approve with notification
	RiskMedium                 // ask user (default for unknown)
	RiskHigh                   // always ask, show warning
)

// ToolConfirmationRouter decides whether a tool call needs user approval
// based on the tool name and arguments. Designed for solo devs who want
// fast iteration without constant y/n prompts for safe operations.
type ToolConfirmationRouter struct {
	// Override allows per-tool risk overrides (tool name → risk level)
	Override map[string]ToolRisk
}

// NewToolConfirmationRouter creates a router with sensible defaults for coding.
func NewToolConfirmationRouter() *ToolConfirmationRouter {
	return &ToolConfirmationRouter{Override: make(map[string]ToolRisk)}
}

// read-only tools that never need approval
var safeTools = map[string]bool{
	"Read": true, "Grep": true, "Glob": true, "LS": true,
	"WebSearch": true, "WebFetch": true, "CodeSearch": true,
	"ToolSearch": true, "TodoRead": true, "TaskGet": true,
	"TaskList": true, "ListMcpResources": true, "ReadMcpResource": true,
	"Brief": true, "Diagnostics": true,
}

// tools that modify files but are generally safe in a dev context
var lowRiskTools = map[string]bool{
	"Write": true, "Edit": true, "MultiEdit": true,
	"NotebookEdit": true, "TodoWrite": true, "TaskUpdate": true,
}

// tools that execute code or have side effects
var mediumRiskTools = map[string]bool{
	"Bash": true, "Agent": true, "Workflow": true,
	"Download": true, "AgenticFetch": true,
}

// Classify determines the risk level of a tool call.
func (r *ToolConfirmationRouter) Classify(toolName string, args map[string]interface{}) ToolRisk {
	// Check overrides first
	if risk, ok := r.Override[toolName]; ok {
		return risk
	}

	if safeTools[toolName] {
		return RiskNone
	}
	if lowRiskTools[toolName] {
		return RiskLow
	}
	if mediumRiskTools[toolName] {
		// Bash commands get extra scrutiny
		if toolName == "Bash" {
			return r.classifyBashRisk(args)
		}
		return RiskMedium
	}
	return RiskMedium // unknown tools default to ask
}

// NeedsConfirmation returns true if the tool call should prompt the user.
func (r *ToolConfirmationRouter) NeedsConfirmation(toolName string, args map[string]interface{}) bool {
	risk := r.Classify(toolName, args)
	return risk >= RiskMedium
}

// classifyBashRisk inspects bash command content for dangerous patterns.
func (r *ToolConfirmationRouter) classifyBashRisk(args map[string]interface{}) ToolRisk {
	cmd, _ := args["command"].(string)
	if cmd == "" {
		return RiskMedium
	}
	lower := strings.ToLower(cmd)

	// High risk: destructive commands
	highRisk := []string{
		"rm -rf", "drop table", "git push --force", "git reset --hard",
		"chmod -R 777", "mkfs", "dd if=", "> /dev/",
	}
	for _, pat := range highRisk {
		if strings.Contains(lower, pat) {
			return RiskHigh
		}
	}

	// Low risk: common dev commands
	lowRisk := []string{
		"go test", "go build", "go vet", "npm test", "npm run",
		"cargo test", "cargo build", "pytest", "make", "cat ", "echo ",
		"git status", "git diff", "git log", "git branch", "ls ", "pwd",
		"head ", "tail ", "wc ", "grep ", "find ",
	}
	for _, pat := range lowRisk {
		if strings.HasPrefix(lower, pat) || strings.Contains(lower, pat) {
			return RiskLow
		}
	}

	return RiskMedium
}
