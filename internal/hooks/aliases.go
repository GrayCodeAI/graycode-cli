package hooks

import (
	"strings"

	agentcontracts "github.com/GrayCodeAI/hawk-core-contracts/agent"
)

// Extended EventType values (Year 0 PACK-04). Existing snake_case constants in
// hooks.go remain the primary values used by Registry.Execute.
const (
	// Vendor-style aliases (Grok/Claude/Cursor vocabulary).
	EventPreToolUse       EventType = "PreToolUse"
	EventPostToolUse      EventType = "PostToolUse"
	EventSubagentStart    EventType = "subagent_start"
	EventSubagentStop     EventType = "subagent_stop"
	EventStop             EventType = "stop"
	EventFailure          EventType = "failure"
	EventUserPromptSubmit EventType = "user_prompt_submit"
)

// CanonicalEvent normalizes vendor aliases to Hawk's primary event strings
// used by the decision-hook matcher and registry.
func CanonicalEvent(s string) string {
	if s == "" {
		return ""
	}
	if c := agentcontracts.CanonicalHookEvent(s); c != "" {
		switch c {
		case agentcontracts.HookPreToolUse:
			return string(EventPreTool)
		case agentcontracts.HookPostToolUse:
			return string(EventPostTool)
		case agentcontracts.HookPreCompact:
			return string(EventPreCompact)
		case agentcontracts.HookSessionStart:
			return string(EventSessionStart)
		case agentcontracts.HookSessionEnd:
			return string(EventSessionEnd)
		case agentcontracts.HookUserPromptSubmit:
			return string(EventUserPromptSubmit)
		case agentcontracts.HookSubagentStart:
			return string(EventSubagentStart)
		case agentcontracts.HookSubagentStop:
			return string(EventSubagentStop)
		case agentcontracts.HookStop:
			return string(EventStop)
		case agentcontracts.HookFailure:
			return string(EventFailure)
		case agentcontracts.HookPermissionRequest:
			return string(EventPermissionAsk)
		case agentcontracts.HookNotification:
			return "notification"
		}
	}
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "pre_tool", "pretool", "pretooluse", "pre_tool_use":
		return string(EventPreTool)
	case "post_tool", "posttool", "posttooluse", "post_tool_use":
		return string(EventPostTool)
	case "pre_query":
		return string(EventPreQuery)
	case "post_query":
		return string(EventPostQuery)
	case "pre_compact":
		return string(EventPreCompact)
	case "post_compact":
		return string(EventPostCompact)
	case "session_start":
		return string(EventSessionStart)
	case "session_end":
		return string(EventSessionEnd)
	case "permission_ask":
		return string(EventPermissionAsk)
	case "error", "failure", "on_error", "onerror":
		return string(EventFailure)
	case "subagent_start", "subagentstart":
		return string(EventSubagentStart)
	case "subagent_stop", "subagentstop":
		return string(EventSubagentStop)
	case "stop":
		return string(EventStop)
	case "user_prompt_submit", "userpromptsubmit":
		return string(EventUserPromptSubmit)
	case "user_prompt_queued", "userpromptqueued":
		return string(EventUserPromptQueued)
	case "turn_started", "turnstarted":
		return string(EventTurnStarted)
	case "post_tool_failure", "posttoolfailure":
		return string(EventPostToolFailure)
	case "permission_result", "permissionresult":
		return string(EventPermissionResult)
	case "session_heartbeat", "sessionheartbeat":
		return string(EventSessionHeartbeat)
	case "task_started", "taskstarted":
		return string(EventSubagentTask)
	case "stop_failure", "stopfailure":
		return string(EventStopFailure)
	case "interrupt":
		return string(EventInterrupt)
	case "notification":
		return string(EventNotification)
	default:
		return s
	}
}

// EventsMatch reports whether a matcher event equals a runtime event after
// canonicalization.
func EventsMatch(matcherEvent, runtimeEvent string) bool {
	return CanonicalEvent(matcherEvent) == CanonicalEvent(runtimeEvent)
}
