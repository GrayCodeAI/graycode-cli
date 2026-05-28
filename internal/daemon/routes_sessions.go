package daemon

import (
	"net/http"
	"os"
	"path/filepath"
	"strconv"

	"github.com/GrayCodeAI/hawk/internal/session"
)

// handleGetSession handles GET /v1/sessions/{id} — get session detail.
func (s *Server) handleGetSession(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if id == "" {
		writeJSON(w, http.StatusBadRequest, ErrorResponse{
			Error: "session id is required",
			Code:  "missing_id",
		})
		return
	}

	sess, err := session.Load(id)
	if err != nil {
		writeJSON(w, http.StatusNotFound, ErrorResponse{
			Error:   "session not found",
			Code:    "not_found",
			Details: err.Error(),
		})
		return
	}

	// Count tool calls across all messages
	toolCalls := 0
	for _, msg := range sess.Messages {
		toolCalls += len(msg.ToolUse)
	}

	writeJSON(w, http.StatusOK, SessionDetailResponse{
		ID:           sess.ID,
		CreatedAt:    sess.CreatedAt,
		UpdatedAt:    sess.UpdatedAt,
		Model:        sess.Model,
		Provider:     sess.Provider,
		CWD:          sess.CWD,
		Name:         sess.Name,
		MessageCount: len(sess.Messages),
		ToolCalls:    toolCalls,
	})
}

// handleGetMessages handles GET /v1/sessions/{id}/messages — get session messages with pagination.
func (s *Server) handleGetMessages(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if id == "" {
		writeJSON(w, http.StatusBadRequest, ErrorResponse{
			Error: "session id is required",
			Code:  "missing_id",
		})
		return
	}

	// Parse pagination params
	offset := 0
	limit := 50
	if v := r.URL.Query().Get("offset"); v != "" {
		if parsed, err := strconv.Atoi(v); err == nil && parsed >= 0 {
			offset = parsed
		}
	}
	if v := r.URL.Query().Get("limit"); v != "" {
		if parsed, err := strconv.Atoi(v); err == nil && parsed > 0 {
			limit = parsed
		}
	}
	const maxLimit = 10000
	if limit > maxLimit {
		limit = maxLimit
	}

	sess, err := session.Load(id)
	if err != nil {
		writeJSON(w, http.StatusNotFound, ErrorResponse{
			Error:   "session not found",
			Code:    "not_found",
			Details: err.Error(),
		})
		return
	}

	total := len(sess.Messages)

	// Slice messages by offset/limit
	start := offset
	if start > total {
		start = total
	}
	end := start + limit
	if end > total {
		end = total
	}

	messages := make([]MessageResponse, 0, end-start)
	for _, msg := range sess.Messages[start:end] {
		mr := MessageResponse{
			Role:    msg.Role,
			Content: msg.Content,
		}
		if len(msg.ToolUse) > 0 {
			mr.ToolUse = msg.ToolUse
		}
		if len(msg.ToolResults) > 0 {
			mr.ToolResult = msg.ToolResults
		}
		messages = append(messages, mr)
	}

	writeJSON(w, http.StatusOK, PaginatedResponse{
		Data:    messages,
		Total:   total,
		Offset:  offset,
		Limit:   limit,
		HasMore: end < total,
	})
}

// handleDeleteSession handles DELETE /v1/sessions/{id} — delete a session.
func (s *Server) handleDeleteSession(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if id == "" {
		writeJSON(w, http.StatusBadRequest, ErrorResponse{
			Error: "session id is required",
			Code:  "missing_id",
		})
		return
	}

	// Validate session ID contains only safe characters (alphanumeric, dash, underscore, dot).
	for _, c := range id {
		if !((c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') || (c >= '0' && c <= '9') || c == '-' || c == '_' || c == '.') {
			writeJSON(w, http.StatusBadRequest, ErrorResponse{
				Error: "invalid session id: only alphanumeric, dash, underscore, and dot are allowed",
				Code:  "invalid_id",
			})
			return
		}
	}

	home, err := os.UserHomeDir()
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, ErrorResponse{
			Error:   "failed to determine sessions directory",
			Code:    "internal_error",
			Details: err.Error(),
		})
		return
	}

	sessionsDir := filepath.Join(home, ".hawk", "sessions")
	jsonlPath := filepath.Join(sessionsDir, id+".jsonl")
	jsonPath := filepath.Join(sessionsDir, id+".json")

	// Try to remove JSONL file first, then legacy JSON
	removedAny := false
	if err := os.Remove(jsonlPath); err == nil {
		removedAny = true
	}
	if err := os.Remove(jsonPath); err == nil {
		removedAny = true
	}

	if !removedAny {
		writeJSON(w, http.StatusNotFound, ErrorResponse{
			Error: "session not found",
			Code:  "not_found",
		})
		return
	}

	// Also remove from in-memory sessions map if present
	s.sessions.Delete(id)

	w.WriteHeader(http.StatusNoContent)
}
