package daemon

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/GrayCodeAI/hawk/engine"
)

// SessionFactory creates a configured engine session for a given request.
// The caller (cmd package) provides this, wiring system prompts, tools, keys.
type SessionFactory func(req ChatRequest) (*engine.Session, error)

// Server is the hawk daemon HTTP server for programmatic/CI access.
type Server struct {
	addr       string
	mux        *http.ServeMux
	server     *http.Server
	startedAt  time.Time
	sessions   sync.Map // sessionID -> *Session
	newSession SessionFactory
}

// Config holds daemon configuration.
type Config struct {
	Port    int    `json:"port"`
	Host    string `json:"host"`
	LogFile string `json:"log_file"`
}

// DefaultConfig returns reasonable defaults.
func DefaultConfig() Config {
	return Config{
		Port: 4590,
		Host: "127.0.0.1",
	}
}

// ChatRequest is the JSON body for POST /v1/chat.
type ChatRequest struct {
	Prompt    string `json:"prompt"`
	SessionID string `json:"session_id,omitempty"`
	Model     string `json:"model,omitempty"`
	MaxTurns  int    `json:"max_turns,omitempty"`
	Autonomy  string `json:"autonomy,omitempty"`
	CWD       string `json:"cwd,omitempty"`
	Agent     string `json:"agent,omitempty"`
}

// ChatResponse is the JSON response from POST /v1/chat.
type ChatResponse struct {
	SessionID  string `json:"session_id"`
	Response   string `json:"response"`
	TokensIn   int    `json:"tokens_in"`
	TokensOut  int    `json:"tokens_out"`
	TurnsTaken int    `json:"turns_taken"`
	Duration   string `json:"duration"`
}

// Session tracks an active daemon session.
type Session struct {
	ID        string    `json:"id"`
	CreatedAt time.Time `json:"created_at"`
	LastUsed  time.Time `json:"last_used"`
	Turns     int       `json:"turns"`
	CWD       string    `json:"cwd"`
}

// HealthResponse is the JSON response from GET /v1/health.
type HealthResponse struct {
	Status    string `json:"status"`
	Version   string `json:"version"`
	Uptime    string `json:"uptime"`
	Sessions  int    `json:"active_sessions"`
	StartedAt string `json:"started_at"`
}

// New creates a new daemon server. If factory is nil, chat endpoint returns an error.
func New(cfg Config, factory SessionFactory) *Server {
	if cfg.Port <= 0 {
		cfg.Port = DefaultConfig().Port
	}
	if cfg.Host == "" {
		cfg.Host = DefaultConfig().Host
	}

	s := &Server{
		addr:       fmt.Sprintf("%s:%d", cfg.Host, cfg.Port),
		mux:        http.NewServeMux(),
		startedAt:  time.Now(),
		newSession: factory,
	}
	s.routes()
	s.server = &http.Server{
		Addr:         s.addr,
		Handler:      s.mux,
		ReadTimeout:  30 * time.Second,
		WriteTimeout: 300 * time.Second,
		IdleTimeout:  60 * time.Second,
	}
	return s
}

// Start begins serving in the background. Returns the listening address.
func (s *Server) Start() (string, error) {
	ln, err := net.Listen("tcp", s.addr)
	if err != nil {
		return "", fmt.Errorf("daemon listen: %w", err)
	}
	actualAddr := ln.Addr().String()
	s.addr = actualAddr

	go func() {
		if err := s.server.Serve(ln); err != nil && err != http.ErrServerClosed {
			slog.Error("daemon server error", "error", err)
		}
	}()

	if err := s.writePIDFile(); err != nil {
		slog.Warn("failed to write PID file", "error", err)
	}

	slog.Info("hawk daemon started", "addr", actualAddr)
	return actualAddr, nil
}

// Stop gracefully shuts down the daemon.
func (s *Server) Stop(ctx context.Context) error {
	_ = s.removePIDFile()
	return s.server.Shutdown(ctx)
}

// Addr returns the listening address.
func (s *Server) Addr() string {
	return s.addr
}

func (s *Server) routes() {
	s.mux.HandleFunc("GET /v1/health", s.handleHealth)
	s.mux.HandleFunc("POST /v1/chat", s.handleChat)
	s.mux.HandleFunc("GET /v1/sessions", s.handleListSessions)
	s.mux.HandleFunc("GET /v1/sessions/{id}", s.handleGetSession)
	s.mux.HandleFunc("GET /v1/sessions/{id}/messages", s.handleGetMessages)
	s.mux.HandleFunc("DELETE /v1/sessions/{id}", s.handleDeleteSession)
	s.mux.HandleFunc("GET /v1/stats", s.handleStats)
}

func (s *Server) handleHealth(w http.ResponseWriter, _ *http.Request) {
	sessionCount := 0
	s.sessions.Range(func(_, _ any) bool {
		sessionCount++
		return true
	})

	resp := HealthResponse{
		Status:    "ok",
		Version:   "0.1.0",
		Uptime:    time.Since(s.startedAt).Round(time.Second).String(),
		Sessions:  sessionCount,
		StartedAt: s.startedAt.Format(time.RFC3339),
	}
	writeJSON(w, http.StatusOK, resp)
}

func wantsSSE(r *http.Request) bool {
	return r.Header.Get("Accept") == "text/event-stream"
}

func (s *Server) handleChat(w http.ResponseWriter, r *http.Request) {
	var req ChatRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid JSON: " + err.Error()})
		return
	}
	if req.Prompt == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "prompt is required"})
		return
	}

	if s.newSession == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "engine not configured"})
		return
	}

	start := time.Now()

	sess, err := s.newSession(req)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "session create: " + err.Error()})
		return
	}

	// Set autonomy
	if req.Autonomy != "" {
		sess.Autonomy = engine.ParseAutonomyLevel(req.Autonomy)
	}

	// Auto-approve permissions based on autonomy (non-interactive)
	sess.PermissionFn = func(pr engine.PermissionRequest) {
		cfg := engine.PresetConfig(sess.Autonomy)
		allowed := !cfg.NeedsPermission(pr.ToolName, false)
		if pr.Response != nil {
			pr.Response <- allowed
		}
	}

	if req.MaxTurns > 0 {
		sess.MaxTurns = req.MaxTurns
	}

	sess.AddUser(req.Prompt)

	ctx := r.Context()
	events, err := sess.Stream(ctx)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "stream: " + err.Error()})
		return
	}

	// SSE streaming mode
	if wantsSSE(r) {
		w.Header().Set("Content-Type", "text/event-stream")
		w.Header().Set("Cache-Control", "no-cache")
		w.Header().Set("Connection", "keep-alive")
		flusher, _ := w.(http.Flusher)

		for ev := range events {
			switch ev.Type {
			case "content":
		_, _ = fmt.Fprintf(w, "data: %s\n\n", ev.Content)
			case "done":
		_, _ = fmt.Fprintf(w, "event: done\ndata: {}\n\n")
			}
			if flusher != nil {
				flusher.Flush()
			}
		}
		return
	}

	// Standard JSON response
	var response strings.Builder
	var totalIn, totalOut, turns int

	for ev := range events {
		switch ev.Type {
		case "content":
			response.WriteString(ev.Content)
		case "usage":
			if ev.Usage != nil {
				totalIn += ev.Usage.PromptTokens
				totalOut += ev.Usage.CompletionTokens
				turns++
			}
		}
	}

	sessionID := fmt.Sprintf("daemon-%d", start.UnixMilli())
	s.sessions.Store(sessionID, &Session{
		ID:        sessionID,
		CreatedAt: start,
		LastUsed:  time.Now(),
		Turns:     turns,
		CWD:       req.CWD,
	})

	writeJSON(w, http.StatusOK, ChatResponse{
		SessionID:  sessionID,
		Response:   response.String(),
		TokensIn:   totalIn,
		TokensOut:  totalOut,
		TurnsTaken: turns,
		Duration:   time.Since(start).Round(time.Millisecond).String(),
	})
}

func (s *Server) handleListSessions(w http.ResponseWriter, _ *http.Request) {
	var sessions []*Session
	s.sessions.Range(func(_, v any) bool {
		if sess, ok := v.(*Session); ok {
			sessions = append(sessions, sess)
		}
		return true
	})
	if sessions == nil {
		sessions = []*Session{}
	}
	writeJSON(w, http.StatusOK, sessions)
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func (s *Server) writePIDFile() error {
	dir := pidDir()
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	data := fmt.Sprintf(`{"pid":%d,"addr":"%s","started_at":"%s"}`,
		os.Getpid(), s.addr, s.startedAt.Format(time.RFC3339))
	return os.WriteFile(filepath.Join(dir, "daemon.json"), []byte(data), 0o644)
}

func (s *Server) removePIDFile() error {
	return os.Remove(filepath.Join(pidDir(), "daemon.json"))
}

func pidDir() string {
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".hawk", "run")
}
