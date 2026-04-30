package engine

// PermissionRequest is sent from engine to TUI when a tool needs approval.
type PermissionRequest struct {
	ToolName string
	ToolID   string
	Summary  string // human-readable description of what the tool will do
	Response chan bool
}

// toolNeedsPermission returns true for tools that modify state.
func toolNeedsPermission(name string) bool {
	switch name {
	case "bash", "file_write", "file_edit":
		return true
	default:
		return false
	}
}

// toolSummary generates a human-readable summary of what a tool call will do.
func toolSummary(name string, args map[string]interface{}) string {
	switch name {
	case "bash":
		if cmd, ok := args["command"].(string); ok {
			if len(cmd) > 120 {
				cmd = cmd[:120] + "..."
			}
			return "Run: " + cmd
		}
	case "file_write":
		if p, ok := args["path"].(string); ok {
			return "Write file: " + p
		}
	case "file_edit":
		if p, ok := args["path"].(string); ok {
			return "Edit file: " + p
		}
	}
	return name
}
