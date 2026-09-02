package daemon

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"strings"
	"time"
	"unicode"

	"github.com/GrayCodeAI/hawk/internal/executiongraph"
	"github.com/GrayCodeAI/hawk/internal/session"
)

const (
	maxGraphRepositoryIDLength = 256
	maxGraphSwiftCheckpoints   = 64
)

// GraphRequest is the bounded, read-only input to a daemon graph projection.
type GraphRequest struct {
	SessionID          string
	RepositoryID       string
	SwiftCheckpointIDs []string
	GeneratedAt        time.Time
}

// GraphFactory projects one persisted session into Hawk's portable graph.
type GraphFactory func(context.Context, GraphRequest) (executiongraph.Export, error)

// handleGetSessionGraph handles GET /v1/sessions/{id}/graph.
func (s *Server) handleGetSessionGraph(w http.ResponseWriter, r *http.Request) {
	id := strings.TrimSpace(r.PathValue("id"))
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

	repositoryID := strings.TrimSpace(r.URL.Query().Get("repository"))
	if !validGraphRepositoryID(repositoryID) {
		writeJSON(w, http.StatusBadRequest, ErrorResponse{
			Error: "invalid repository scope",
			Code:  "invalid_repository",
		})
		return
	}
	checkpointIDs := r.URL.Query()["swift_checkpoint"]
	if len(checkpointIDs) > maxGraphSwiftCheckpoints {
		writeJSON(w, http.StatusBadRequest, ErrorResponse{
			Error: "too many Swift checkpoint IDs",
			Code:  "too_many_swift_checkpoints",
		})
		return
	}
	for i := range checkpointIDs {
		checkpointIDs[i] = strings.TrimSpace(checkpointIDs[i])
		if !validGraphSwiftCheckpointID(checkpointIDs[i]) {
			writeJSON(w, http.StatusBadRequest, ErrorResponse{
				Error: "invalid Swift checkpoint ID",
				Code:  "invalid_swift_checkpoint",
			})
			return
		}
	}

	s.graphMu.RLock()
	factory := s.graphFactory
	s.graphMu.RUnlock()
	if factory == nil {
		writeJSON(w, http.StatusServiceUnavailable, ErrorResponse{
			Error: "execution graph is not configured",
			Code:  "graph_unavailable",
		})
		return
	}

	export, err := factory(r.Context(), GraphRequest{
		SessionID:          id,
		RepositoryID:       repositoryID,
		SwiftCheckpointIDs: append([]string(nil), checkpointIDs...),
		GeneratedAt:        time.Now().UTC(),
	})
	if err != nil {
		if errors.Is(err, session.ErrNotFound) {
			writeJSON(w, http.StatusNotFound, ErrorResponse{
				Error: "session not found",
				Code:  "not_found",
			})
			return
		}
		slog.Error("execution graph projection failed", "session_id", id, "error", err)
		writeJSON(w, http.StatusInternalServerError, ErrorResponse{
			Error: "execution graph projection failed",
			Code:  "graph_projection_failed",
		})
		return
	}
	writeJSON(w, http.StatusOK, export)
}

func validGraphRepositoryID(value string) bool {
	if len(value) > maxGraphRepositoryIDLength {
		return false
	}
	for _, current := range value {
		if unicode.IsControl(current) {
			return false
		}
	}
	return true
}

func validGraphSwiftCheckpointID(value string) bool {
	if len(value) != 12 {
		return false
	}
	for _, current := range value {
		if (current < '0' || current > '9') && (current < 'a' || current > 'f') {
			return false
		}
	}
	return true
}
