package swarm

import (
	"fmt"
	"os"
	"strings"
)

// IsCoordinatorMode checks if the environment is configured for multi-agent coordination.
func IsCoordinatorMode() bool {
	return os.Getenv("HAWK_CODE_COORDINATOR_MODE") == "1"
}

// MatchSessionMode reconciles the current environment mode with a stored session mode.
// Returns a warning message if modes differ, or empty string if they match.
func MatchSessionMode(storedMode string) string {
	currentMode := "standard"
	if IsCoordinatorMode() {
		currentMode = "coordinator"
	}
	if storedMode == currentMode || storedMode == "" {
		return ""
	}
	return fmt.Sprintf("Session was created in %s mode but current environment is %s mode", storedMode, currentMode)
}

// WorkerToolMode determines what tools workers can access.
type WorkerToolMode string

const (
	WorkerToolsSimple WorkerToolMode = "simple"
	WorkerToolsFull   WorkerToolMode = "full"
)

var simpleWorkerTools = []string{"Bash", "Read", "Edit", "Write", "Grep", "Glob", "LS"}

var excludedFromWorkers = map[string]bool{
	"TeamCreate": true,
	"TeamDelete": true,
	"CronCreate": true,
	"CronDelete": true,
}

// GetWorkerTools returns the list of tools available to worker agents.
func GetWorkerTools(mode WorkerToolMode, allTools []string, mcpTools []string) []string {
	if mode == WorkerToolsSimple {
		return simpleWorkerTools
	}

	var tools []string
	for _, t := range allTools {
		if !excludedFromWorkers[t] {
			tools = append(tools, t)
		}
	}
	tools = append(tools, mcpTools...)
	return tools
}

// GetCoordinatorUserContext generates context about the worker environment.
func GetCoordinatorUserContext(mcpClients []string, scratchpadDir string) map[string]string {
	ctx := map[string]string{
		"role": "coordinator",
	}
	if len(mcpClients) > 0 {
		ctx["mcp_servers"] = strings.Join(mcpClients, ", ")
	}
	if scratchpadDir != "" {
		ctx["scratchpad_dir"] = scratchpadDir
	}
	return ctx
}

// GetCoordinatorSystemPrompt returns the system prompt for coordinator mode.
func GetCoordinatorSystemPrompt() string {
	return `You are a coordinator agent that orchestrates multiple worker agents to accomplish complex tasks.

## Your Role
- You are the director. You research, plan, delegate, and synthesize.
- Workers execute specific tasks under your guidance.
- You MUST understand findings before delegating follow-up work.

## Principles

### Parallel Work
- Spawn workers concurrently for independent tasks.
- Use a single message with multiple Agent tool calls when tasks don't depend on each other.

### Never Delegate Understanding
- Don't write "based on your findings, fix the bug" — that pushes synthesis onto the worker.
- Always include file paths, line numbers, and what specifically to change in worker prompts.

### Worker Continuation vs New Spawn
- High context overlap with a running worker → continue via SendMessage.
- Fresh perspective needed or low overlap → spawn new Agent.

### Stopped Worker Recovery
- If a worker goes off-track, use TaskStop to halt it.
- Then continue with a corrected spec via SendMessage, or spawn a new worker.

## Coordination Flow
1. Research: spawn exploratory workers to understand the problem
2. Synthesis: review findings, identify approach
3. Implementation: spawn workers for independent changes
4. Verification: confirm changes work together

## Task Notifications
Workers report back via structured notifications. Monitor TaskList for progress.`
}
