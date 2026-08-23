package daemon

import (
	"net/http"
	"time"
)

// AgentLiveStatus is one agent session's machine-readable live state — the
// contract external mission-control dashboards (Luvus-style supervisors) poll
// to show blocked/working/done/idle without scraping terminal output.
type AgentLiveStatus struct {
	SessionID string    `json:"session_id"`
	Agent     string    `json:"agent,omitempty"`
	State     string    `json:"state"` // working | idle | stale
	Turns     int       `json:"turns"`
	CWD       string    `json:"cwd,omitempty"`
	LastUsed  time.Time `json:"last_used"`
}

// AgentStatusResponse is the JSON response from GET /v1/agent/status.
type AgentStatusResponse struct {
	GeneratedAt string            `json:"generated_at"`
	Agents      []AgentLiveStatus `json:"agents"`
}

// handleAgentStatus reports per-session live agent state derived from daemon
// ground truth: an in-flight generation (registered cancel) means "working";
// otherwise recency of last use distinguishes idle from stale.
func (s *Server) handleAgentStatus(w http.ResponseWriter, _ *http.Request) {
	s.cancelMu.Lock()
	inFlight := make(map[string]bool, len(s.cancels))
	for id := range s.cancels {
		inFlight[id] = true
	}
	s.cancelMu.Unlock()

	now := time.Now()
	resp := AgentStatusResponse{GeneratedAt: now.UTC().Format(time.RFC3339), Agents: []AgentLiveStatus{}}
	s.sessions.Range(func(_, v any) bool {
		sess, ok := v.(*Session)
		if !ok {
			return true
		}
		st := AgentLiveStatus{
			SessionID: sess.ID,
			Agent:     sess.Agent,
			State:     "idle",
			Turns:     sess.Turns,
			CWD:       sess.CWD,
			LastUsed:  sess.LastUsed,
		}
		switch {
		case inFlight[sess.ID]:
			st.State = "working"
		case now.Sub(sess.LastUsed) > 30*time.Minute:
			st.State = "stale"
		}
		resp.Agents = append(resp.Agents, st)
		return true
	})
	writeJSON(w, http.StatusOK, resp)
}
