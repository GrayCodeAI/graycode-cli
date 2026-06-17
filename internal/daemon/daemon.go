package daemon

import (
	"context"
	"crypto/subtle"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/GrayCodeAI/hawk/internal/engine"
	"github.com/GrayCodeAI/hawk/internal/home"
	"github.com/GrayCodeAI/hawk/internal/netutil"
)

const maxRequestBodyBytes = 1 << 20

// SessionFactory creates a configured engine session for a given request.
// The caller (cmd package) provides this, wiring system prompts, tools, keys.
type SessionFactory func(req ChatRequest) (*engine.Session, error)

// version is set via SetVersion from main.go at startup.
var version = "0.0.0"

// SetVersion propagates the canonical hawk version into the daemon.
func SetVersion(v string) { version = v }

// Server is the hawk daemon HTTP server for programmatic/CI access.
type Server struct {
	addr       string
	mux        *http.ServeMux
	server     *http.Server
	startedAt  time.Time
	sessions   sync.Map // sessionID -> *Session
	newSession SessionFactory
	apiKey     string
	gateways   *gatewayManager

	// readyFn reports whether the daemon's dependencies (session store and
	// provider connectivity) are initialized and the server can serve real
	// traffic. It is consulted by GET /v1/ready. When nil, readiness falls
	// back to "session factory wired" (see Ready). Set via SetReadyFn.
	readyMu sync.RWMutex
	readyFn func() (bool, string)
}

// ReadyResponse is the JSON response from GET /v1/ready.
type ReadyResponse struct {
	// Ready is true only when every dependency is initialized.
	Ready bool `json:"ready"`
	// Reason describes why the daemon is not ready (empty when ready).
	Reason string `json:"reason,omitempty"`
	// Uptime is the time since the server started.
	Uptime string `json:"uptime"`
}

// Config holds daemon configuration.
type Config struct {
	Port    int    `json:"port"`
	Host    string `json:"host"`
	LogFile string `json:"log_file"`
	APIKey  string `json:"api_key,omitempty"`
	// Gateways configures optional messaging bridges (Telegram/Discord/Slack).
	// All are disabled by default; the daemon starts normally when none are set.
	Gateways GatewaysConfig `json:"gateways,omitempty"`
}

// DefaultConfig returns reasonable defaults.
func DefaultConfig() Config {
	return Config{
		Port: 4590,
		Host: netutil.LoopbackHost,
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
	if cfg.Port < 0 {
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
		apiKey:     cfg.APIKey,
	}
	s.routes()
	// Build the messaging-bridge manager. The daemon URL is finalised in Start
	// (port 0 resolves to a real port there); we pass the configured addr now so
	// Slack can register its webhook route on the mux, and patch the forward URL
	// for poll-based gateways at Start time.
	s.gateways = newGatewayManager(cfg.Gateways, "http://"+s.addr, cfg.APIKey, s)
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
	if err := s.validateAuthConfig(); err != nil {
		return "", err
	}
	s.warnInsecureAuthConfig()

	ln, err := new(net.ListenConfig).Listen(context.Background(), "tcp", s.addr)
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

	// Point poll-based gateways at the now-resolved daemon URL and launch them.
	if s.gateways != nil {
		s.gateways.setDaemonURL("http://" + actualAddr)
		s.gateways.Start(context.Background())
	}

	slog.Info("hawk daemon started", "addr", actualAddr)
	return actualAddr, nil
}

// validateAuthConfig refuses to start the daemon with no API key on a
// non-loopback bind. The auth middleware (see auth) silently allows every
// request when apiKey == "", so a misconfigured production daemon would
// be wide open. The only safe no-key mode is loopback bind.
func (s *Server) validateAuthConfig() error {
	if s.apiKey != "" {
		return nil
	}
	host, _, err := net.SplitHostPort(s.addr)
	if err != nil {
		return fmt.Errorf("daemon: invalid bind address %q: %w", s.addr, err)
	}
	if !isLoopbackHost(host) {
		return fmt.Errorf("daemon: apiKey is empty and bind address %q is not loopback; refusing to start. Set Config.APIKey or bind to %s", s.addr, netutil.LoopbackHost)
	}
	return nil
}

// warnInsecureAuthConfig logs a WARN line when the daemon is started
// without an API key, even on a loopback bind. The user may not have
// intended to run an unauthenticated daemon.
func (s *Server) warnInsecureAuthConfig() {
	if s.apiKey != "" {
		return
	}
	slog.Warn(
		"hawk daemon started without API key authentication; only loopback access allowed",
		"addr", s.addr,
		"hint", "Set Config.APIKey to enable authentication, or keep the default loopback bind.",
	)
}

// isLoopbackHost reports whether host is a loopback address: an IP in
// 127.0.0.0/8 or ::1, the literal "localhost", or an empty string
// (which SplitHostPort returns when the address has no host part —
// treated as non-loopback to fail safe).
func isLoopbackHost(host string) bool {
	if host == "" || host == "localhost" {
		return host == "localhost" // "" is unsafe; "localhost" is loopback
	}
	if ip := net.ParseIP(host); ip != nil {
		return ip.IsLoopback()
	}
	return false
}

// Stop gracefully shuts down the daemon.
func (s *Server) Stop(ctx context.Context) error {
	if s.gateways != nil {
		s.gateways.Stop()
	}
	_ = s.removePIDFile()
	return s.server.Shutdown(ctx)
}

// Addr returns the listening address.
func (s *Server) Addr() string {
	return s.addr
}

// SetReadyFn installs a custom readiness probe. The probe should return
// (true, "") when all dependencies (session store, provider connectivity) are
// initialized, or (false, reason) otherwise. Passing nil restores the default
// probe. This is additive and safe to call concurrently with serving.
func (s *Server) SetReadyFn(fn func() (bool, string)) {
	s.readyMu.Lock()
	s.readyFn = fn
	s.readyMu.Unlock()
}

// ready evaluates readiness using the installed probe, falling back to the
// default check (session factory wired) when none is set.
func (s *Server) ready() (bool, string) {
	s.readyMu.RLock()
	fn := s.readyFn
	s.readyMu.RUnlock()
	if fn != nil {
		return fn()
	}
	// Default readiness: the session store map is always initialized once New
	// runs, so the remaining dependency is provider connectivity, which the
	// cmd layer wires as the session factory. No factory => cannot serve chat.
	if s.newSession == nil {
		return false, "engine not configured"
	}
	return true, ""
}

func (s *Server) routes() {
	s.mux.HandleFunc("GET /v1/health", s.handleHealth)
	s.mux.HandleFunc("GET /v1/ready", s.handleReady)
	s.mux.HandleFunc("POST /v1/chat", s.auth(s.handleChat))
	s.mux.HandleFunc("GET /v1/sessions", s.auth(s.handleListSessions))
	s.mux.HandleFunc("GET /v1/sessions/{id}", s.auth(s.handleGetSession))
	s.mux.HandleFunc("GET /v1/sessions/{id}/messages", s.auth(s.handleGetMessages))
	s.mux.HandleFunc("DELETE /v1/sessions/{id}", s.auth(s.handleDeleteSession))
	s.mux.HandleFunc("GET /v1/stats", s.auth(s.handleStats))
	s.RegisterReviewRoutes()
}

func (s *Server) auth(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		token := r.Header.Get("Authorization")
		token = strings.TrimPrefix(token, "Bearer ")
		if token == "" {
			token = r.Header.Get("X-API-Key")
		}
		if s.apiKey == "" {
			// No API key configured — always allow, but still perform a constant-time
			// comparison against a dummy value to avoid leaking "no auth configured" via timing.
			_ = constantTimeEqual(token, "no-auth-configured-dummy")
			next(w, r)
			return
		}
		if !constantTimeEqual(token, s.apiKey) {
			writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "unauthorized"})
			return
		}
		next(w, r)
	}
}

func constantTimeEqual(a, b string) bool {
	// Length mismatch check short-circuits via early return, which
	// leaks the length difference via timing. This is an accepted
	// trade-off for bearer-token authentication — tokens are fixed
	// length and the comparison result is not secret.
	if len(a) != len(b) {
		return false
	}
	return subtle.ConstantTimeCompare([]byte(a), []byte(b)) == 1
}

func decodeJSONBody(w http.ResponseWriter, r *http.Request, dst any) bool {
	r.Body = http.MaxBytesReader(w, r.Body, maxRequestBodyBytes)
	dec := json.NewDecoder(r.Body)
	dec.DisallowUnknownFields()
	if err := dec.Decode(dst); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid JSON"})
		return false
	}
	if err := dec.Decode(&struct{}{}); err != io.EOF {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "request body must contain a single JSON object"})
		return false
	}
	return true
}

func (s *Server) handleHealth(w http.ResponseWriter, _ *http.Request) {
	sessionCount := 0
	s.sessions.Range(func(_, _ any) bool {
		sessionCount++
		return true
	})

	resp := HealthResponse{
		Status:    "ok",
		Version:   version,
		Uptime:    time.Since(s.startedAt).Round(time.Second).String(),
		Sessions:  sessionCount,
		StartedAt: s.startedAt.Format(time.RFC3339),
	}
	writeJSON(w, http.StatusOK, resp)
}

// handleReady is the readiness probe. Unlike /v1/health (liveness, always 200
// while the process is up), /v1/ready returns 200 only when all dependencies
// are initialized and 503 otherwise, so orchestrators can gate traffic.
func (s *Server) handleReady(w http.ResponseWriter, _ *http.Request) {
	ok, reason := s.ready()
	resp := ReadyResponse{
		Ready:  ok,
		Reason: reason,
		Uptime: time.Since(s.startedAt).Round(time.Second).String(),
	}
	status := http.StatusOK
	if !ok {
		status = http.StatusServiceUnavailable
	}
	writeJSON(w, status, resp)
}

func wantsSSE(r *http.Request) bool {
	return r.Header.Get("Accept") == "text/event-stream"
}

func (s *Server) handleChat(w http.ResponseWriter, r *http.Request) {
	var req ChatRequest
	if !decodeJSONBody(w, r, &req) {
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
		slog.Error("session create failed", "err", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "session creation failed"})
		return
	}

	// Set autonomy
	if req.Autonomy != "" {
		sess.PermSvc().SetAutonomy(engine.ParseAutonomyLevel(req.Autonomy))
	}

	// Auto-approve permissions based on autonomy (non-interactive)
	sess.PermissionFn = func(pr engine.PermissionRequest) {
		cfg := engine.PresetConfig(sess.PermSvc().Autonomy())
		allowed := !cfg.NeedsPermission(pr.ToolName, false)
		if pr.Response != nil {
			pr.Response <- allowed
		}
	}

	if req.MaxTurns > 0 {
		if err := sess.SetMaxTurns(req.MaxTurns); err != nil {
			slog.Error("invalid max turns", "err", err, "max_turns", req.MaxTurns)
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid max_turns"})
			return
		}
	}

	sess.AddUser(req.Prompt)

	ctx := r.Context()
	events, err := sess.Stream(ctx)
	if err != nil {
		slog.Error("stream failed", "err", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "stream failed"})
		return
	}

	// SSE streaming mode
	if wantsSSE(r) {
		w.Header().Set("Content-Type", "text/event-stream")
		w.Header().Set("Cache-Control", "no-cache")
		w.Header().Set("Connection", "keep-alive")
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("X-Frame-Options", "DENY")
		flusher, _ := w.(http.Flusher)

		for ev := range events {
			switch ev.Type {
			case "content":
				// Per SSE spec, each line of a data field must be prefixed with "data:".
				// This prevents injection of fake events via newlines in LLM output.
				for _, line := range strings.Split(ev.Content, "\n") {
					_, _ = fmt.Fprintf(w, "data: %s\n", line)
				}
				_, _ = fmt.Fprint(w, "\n")
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
	data, err := json.Marshal(struct {
		PID          int    `json:"pid"`
		Addr         string `json:"addr"`
		StartedAt    string `json:"started_at"`
		AuthRequired bool   `json:"auth_required"`
	}{
		PID:          os.Getpid(),
		Addr:         s.addr,
		StartedAt:    s.startedAt.Format(time.RFC3339),
		AuthRequired: s.apiKey != "",
	})
	if err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(dir, "daemon.json"), append(data, '\n'), 0o600)
}

func (s *Server) removePIDFile() error {
	return os.Remove(filepath.Join(pidDir(), "daemon.json"))
}

func pidDir() string {
	home := home.Dir()
	return filepath.Join(home, ".hawk", "run")
}
