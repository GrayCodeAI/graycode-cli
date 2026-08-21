package daemon

import (
	"net/http"

	"github.com/GrayCodeAI/hawk/internal/status"
)

// handleStatus returns a process-local, redacted daemon snapshot. It does not
// initialize providers or MCP servers and is safe to call during startup.
func (s *Server) handleStatus(w http.ResponseWriter, _ *http.Request) {
	snapshot := status.New()
	snapshot.HawkVersion = version
	snapshot.Workspace = status.Workspace()
	if s.startedAt.IsZero() {
		snapshot.Recovery = "not_started"
	} else {
		snapshot.Recovery = "available"
	}
	active := 0
	s.sessions.Range(func(_, _ any) bool {
		active++
		return true
	})
	snapshot.Subagents = nil
	snapshot.Sessions = status.ComponentStatus{Active: active, State: "available"}
	snapshot.Warnings = []string{}
	snapshot.MCP.State = "not_loaded"
	snapshot.Hooks.State = "process_local"
	snapshot.Trace.State = "available"
	writeJSON(w, http.StatusOK, snapshot)
}
