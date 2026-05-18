package daemon

import (
	"fmt"
	"net/http"
	"os/exec"
	"strings"
)

// ReviewRequest is the JSON body for POST /v1/review.
type ReviewRequest struct {
	SHA        string `json:"sha"`
	Background bool   `json:"background,omitempty"`
	Model      string `json:"model,omitempty"`
	Concerns   string `json:"concerns,omitempty"`
}

// ReviewResponse is the JSON response from POST /v1/review.
type ReviewResponse struct {
	ID      int64  `json:"id"`
	SHA     string `json:"sha"`
	Status  string `json:"status"`
	Message string `json:"message,omitempty"`
}

// RegisterReviewRoutes adds review endpoints to the daemon.
// Called from routes() if review support is enabled.
func (s *Server) RegisterReviewRoutes() {
	s.mux.HandleFunc("POST /v1/review", s.auth(s.handleReview))
	s.mux.HandleFunc("GET /v1/review/status", s.auth(s.handleReviewStatus))
}

func (s *Server) handleReview(w http.ResponseWriter, r *http.Request) {
	var req ReviewRequest
	if !decodeJSONBody(w, r, &req) {
		return
	}
	if req.SHA == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "sha is required"})
		return
	}

	// Trigger review asynchronously via hawk review run.
	go func() {
		args := []string{"review", "run", req.SHA, "--background"}
		if req.Model != "" {
			args = append(args, "--model", req.Model)
		}
		if req.Concerns != "" {
			args = append(args, "--concerns", req.Concerns)
		}
		_ = exec.Command("hawk", args...).Run()
	}()

	resp := ReviewResponse{
		SHA:     req.SHA,
		Status:  "queued",
		Message: fmt.Sprintf("Review queued for %s", req.SHA[:minLen(len(req.SHA), 8)]),
	}
	writeJSON(w, http.StatusAccepted, resp)
}

func (s *Server) handleReviewStatus(w http.ResponseWriter, _ *http.Request) {
	// Run hawk review status and return output.
	out, err := exec.Command("hawk", "review", "status").Output()
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": strings.TrimSpace(string(out))})
}

func minLen(a, b int) int {
	if a < b {
		return a
	}
	return b
}
