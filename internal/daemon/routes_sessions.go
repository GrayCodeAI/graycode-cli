package daemon

import (
	"errors"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"strconv"

	"github.com/GrayCodeAI/graycode-cli/internal/session"
	"github.com/GrayCodeAI/graycode-cli/internal/storage"
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
	if !validSessionID(id) {
		writeJSON(w, http.StatusBadRequest, ErrorResponse{
			Error: "invalid session id",
			Code:  "invalid_id",
		})
		return
	}

	sess, err := session.Load(id)
	if err != nil {
		writeSessionLoadError(w, id, err)
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
		Agent:        sess.Agent,
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
	if !validSessionID(id) {
		writeJSON(w, http.StatusBadRequest, ErrorResponse{
			Error: "invalid session id",
			Code:  "invalid_id",
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
		writeSessionLoadError(w, id, err)
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

func writeSessionLoadError(w http.ResponseWriter, id string, err error) {
	if errors.Is(err, session.ErrNotFound) {
		writeJSON(w, http.StatusNotFound, ErrorResponse{
			Error: "session not found",
			Code:  "not_found",
		})
		return
	}
	slog.Error("load persisted session failed", "err", err, "session_id", id)
	writeJSON(w, http.StatusInternalServerError, ErrorResponse{
		Error: "session load failed",
		Code:  "session_load_failed",
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

	if !validSessionID(id) {
		writeJSON(w, http.StatusBadRequest, ErrorResponse{
			Error: "invalid session id: use 1-128 alphanumeric, dash, underscore, or dot characters",
			Code:  "invalid_id",
		})
		return
	}

	lock := s.sessionLock(id)
	lock.Lock()
	defer lock.Unlock()

	sessionsDir := storage.SessionsDir()
	jsonlPath := filepath.Join(sessionsDir, id+".jsonl")
	jsonPath := filepath.Join(sessionsDir, id+".json")

	// Try to remove JSONL file first, then legacy JSON
	removedAny := false
	if err := os.Remove(jsonlPath); err == nil {
		removedAny = true
	} else if !errors.Is(err, os.ErrNotExist) {
		slog.Error("delete persisted session failed", "err", err, "session_id", id, "format", "jsonl")
		writeJSON(w, http.StatusInternalServerError, ErrorResponse{Error: "session deletion failed", Code: "session_delete_failed"})
		return
	}
	if err := os.Remove(jsonPath); err == nil {
		removedAny = true
	} else if !errors.Is(err, os.ErrNotExist) {
		slog.Error("delete persisted session failed", "err", err, "session_id", id, "format", "json")
		writeJSON(w, http.StatusInternalServerError, ErrorResponse{Error: "session deletion failed", Code: "session_delete_failed"})
		return
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
