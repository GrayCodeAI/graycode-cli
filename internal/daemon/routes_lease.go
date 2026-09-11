package daemon

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"net/http"

	"github.com/GrayCodeAI/hawk/internal/session"
)

// handleAcquireLease creates or refreshes a single-owner lease on a session,
// returning a writer fence token. The fence is persisted on the session so any
// owner can confirm current ownership; release requires the matching fence.
func (s *Server) handleAcquireLease(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if id == "" || !validSessionID(id) {
		writeJSON(w, http.StatusBadRequest, ErrorResponse{Error: "invalid session id", Code: "invalid_id"})
		return
	}
	fence, err := newFenceToken()
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, ErrorResponse{Error: "lease token generation failed"})
		return
	}
	// Load-or-create an empty durable session so the fence persists.
	sess, err := session.Load(id)
	if err != nil && !errors.Is(err, session.ErrNotFound) {
		writeJSON(w, http.StatusInternalServerError, ErrorResponse{Error: "failed to load session"})
		return
	}
	if sess == nil {
		sess = &session.Session{ID: id}
	}
	sess.SetFence(fence)
	if err := session.Save(sess); err != nil {
		writeJSON(w, http.StatusInternalServerError, ErrorResponse{Error: "failed to persist lease"})
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"session_id": id, "fence": fence})
}

// handleReleaseLease releases a lease only when the presenter's fence matches
// the current owner, preventing an expired owner from clearing a newer one.
func (s *Server) handleReleaseLease(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if id == "" || !validSessionID(id) {
		writeJSON(w, http.StatusBadRequest, ErrorResponse{Error: "invalid session id", Code: "invalid_id"})
		return
	}
	presented := r.URL.Query().Get("fence")
	current := session.FenceOf(id)
	if current == "" {
		writeJSON(w, http.StatusOK, map[string]string{"session_id": id, "released": "true"})
		return
	}
	if presented == "" || presented != current {
		writeJSON(w, http.StatusConflict, ErrorResponse{Error: "lease is owned by another fence", Code: "lease_owned"})
		return
	}
	sess, err := session.Load(id)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, ErrorResponse{Error: "failed to load session"})
		return
	}
	sess.SetFence("")
	if err := session.Save(sess); err != nil {
		writeJSON(w, http.StatusInternalServerError, ErrorResponse{Error: "failed to persist lease release"})
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"session_id": id, "released": "true"})
}

// newFenceToken returns a fresh random writer fence token.
func newFenceToken() (string, error) {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}
