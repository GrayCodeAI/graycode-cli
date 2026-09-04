// Vendored from github.com/GrayCodeAI/eagle/agent at v0.0.0-20260902153929-5877bed17503 (MIT, Copyright (c) 2026 GrayCode AI).
// The upstream repository no longer exists; this copy is owned by Graycode as its contract surface.
package agent

import "strings"

// Hook event names for lifecycle and tool gates.
//
// Graycode may accept vendor aliases (Claude/Cursor); normalize to these
// canonical names at the boundary. Constants are shared so graycode, plugins,
// and SDKs agree on wire/event vocabulary without importing engines.
const (
	HookPreToolUse        = "PreToolUse"
	HookPostToolUse       = "PostToolUse"
	HookUserPromptSubmit  = "UserPromptSubmit"
	HookSessionStart      = "SessionStart"
	HookSessionEnd        = "SessionEnd"
	HookStop              = "Stop"
	HookSubagentStart     = "SubagentStart"
	HookSubagentStop      = "SubagentStop"
	HookNotification      = "Notification"
	HookPermissionRequest = "PermissionRequest"
	HookPreCompact        = "PreCompact"
	HookFailure           = "Failure"
)

// VendorHookAliases maps common third-party hook names to canonical Graycode names.
// Keys are lower-case for case-insensitive lookup.
var VendorHookAliases = map[string]string{
	"pretooluse":         HookPreToolUse,
	"pre_tool_use":       HookPreToolUse,
	"posttooluse":        HookPostToolUse,
	"post_tool_use":      HookPostToolUse,
	"userpromptsubmit":   HookUserPromptSubmit,
	"user_prompt_submit": HookUserPromptSubmit,
	"sessionstart":       HookSessionStart,
	"session_start":      HookSessionStart,
	"sessionend":         HookSessionEnd,
	"session_end":        HookSessionEnd,
	"stop":               HookStop,
	"subagentstart":      HookSubagentStart,
	"subagent_start":     HookSubagentStart,
	"subagentstop":       HookSubagentStop,
	"subagent_stop":      HookSubagentStop,
	"notification":       HookNotification,
	"permissionrequest":  HookPermissionRequest,
	"permission_request": HookPermissionRequest,
	"precompact":         HookPreCompact,
	"pre_compact":        HookPreCompact,
	"failure":            HookFailure,
	"onerror":            HookFailure,
	"on_error":           HookFailure,
}

// CanonicalHookEvent returns the Graycode canonical event name for s, or "" if unknown.
func CanonicalHookEvent(s string) string {
	if s == "" {
		return ""
	}
	switch s {
	case HookPreToolUse, HookPostToolUse, HookUserPromptSubmit, HookSessionStart,
		HookSessionEnd, HookStop, HookSubagentStart, HookSubagentStop,
		HookNotification, HookPermissionRequest, HookPreCompact, HookFailure:
		return s
	}
	if c, ok := VendorHookAliases[strings.ToLower(strings.TrimSpace(s))]; ok {
		return c
	}
	return ""
}
