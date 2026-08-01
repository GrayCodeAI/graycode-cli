package daemon

import (
	"context"
	"crypto/rand"
	"crypto/subtle"
	"encoding/hex"
	"encoding/json"
	"errors"
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
	"github.com/GrayCodeAI/hawk/internal/netutil"
	hawksession "github.com/GrayCodeAI/hawk/internal/session"
	"github.com/GrayCodeAI/hawk/internal/storage"
)

const maxRequestBodyBytes = 1 << 20

// SessionFactory creates a configured engine session for a given request.
// The caller (cmd package) provides this, wiring system prompts, tools, keys.
type SessionFactory func(req ChatRequest) (*engine.Session, error)

// InvalidChatRequestError lets a SessionFactory reject a request without
// exposing internal construction errors as part of the public API. Factories
// should use it for user-controlled values such as an unknown agent persona.
type InvalidChatRequestError struct {
	Message string
	Err     error
}

func (e *InvalidChatRequestError) Error() string {
	if e.Err != nil {
		return e.Err.Error()
	}
	return e.Message
}

func (e *InvalidChatRequestError) Unwrap() error { return e.Err }

// version is set via SetVersion from main.go at startup.
var version = "0.0.0"

// SetVersion propagates the canonical hawk version into the daemon.
func SetVersion(v string) { version = v }

// Server is the hawk daemon HTTP server for programmatic/CI access.
type Server struct {
	addr      string
	mux       *http.ServeMux
	server    *http.Server
	startedAt time.Time
	sessions  sync.Map // sessionID -> *Session
	// A fixed stripe set serializes continuation and deletion for the same
	// durable ID without allowing arbitrary client-supplied IDs to grow a lock
	// map for the lifetime of the daemon.
	sessionLocks [64]sync.Mutex
	newSession   SessionFactory
	apiKey       string
	gateways     *gatewayManager

	// readyFn reports whether Eyrie's preflight/readiness dependencies are
	// initialized and the server can serve real traffic. It is consulted by
	// GET /v1/ready. A nil probe fails closed: a session factory alone does not
	// prove that Eyrie's catalog, credentials, and model selection are ready.
	readyMu sync.RWMutex
	readyFn func() (bool, string)

	// graphFactory projects a persisted Hawk session into the portable,
	// privacy-safe execution graph. The composition root supplies the builder;
	// the daemon owns only HTTP validation, authentication, and encoding.
	graphMu      sync.RWMutex
	graphFactory GraphFactory

	// routePatterns records every "METHOD /path" pattern registered on the
	// mux so tests can verify the HTTP surface matches api/openapi.yaml.
	routePatterns []string
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
	Agent     string    `json:"agent,omitempty"`
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
		Addr:              s.addr,
		Handler:           s.mux,
		ReadHeaderTimeout: 10 * time.Second,
		ReadTimeout:       30 * time.Second,
		WriteTimeout:      300 * time.Second,
		IdleTimeout:       60 * time.Second,
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
		defer func() {
			if r := recover(); r != nil {
				slog.Error("daemon server goroutine panicked", "recover", r)
			}
		}()
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

// SetGraphFactory installs the read-only execution-graph projection used by
// GET /v1/sessions/{id}/graph. Passing nil makes the endpoint unavailable.
func (s *Server) SetGraphFactory(factory GraphFactory) {
	s.graphMu.Lock()
	s.graphFactory = factory
	s.graphMu.Unlock()
}

// ready evaluates readiness using the installed Eyrie probe. It fails closed
// when the engine is absent or the composition root did not install a probe;
// merely having a factory does not prove provider readiness.
func (s *Server) ready() (bool, string) {
	s.readyMu.RLock()
	fn := s.readyFn
	s.readyMu.RUnlock()
	if fn != nil {
		return fn()
	}
	if s.newSession == nil {
		return false, "engine not configured"
	}
	return false, "Eyrie readiness probe not configured"
}

func (s *Server) routes() {
	s.handle("GET /v1/health", s.handleHealth)
	s.handle("GET /v1/ready", s.handleReady)
	s.handle("POST /v1/chat", s.auth(s.handleChat))
	s.handle("GET /v1/sessions", s.auth(s.handleListSessions))
	s.handle("GET /v1/sessions/{id}", s.auth(s.handleGetSession))
	s.handle("GET /v1/sessions/{id}/messages", s.auth(s.handleGetMessages))
	s.handle("GET /v1/sessions/{id}/graph", s.auth(s.handleGetSessionGraph))
	s.handle("DELETE /v1/sessions/{id}", s.auth(s.handleDeleteSession))
	s.handle("GET /v1/stats", s.auth(s.handleStats))
	s.RegisterReviewRoutes()
}

// handle registers a handler on the mux and records the pattern for
// spec-parity verification (see openapi_parity_test.go).
func (s *Server) handle(pattern string, h http.HandlerFunc) {
	s.routePatterns = append(s.routePatterns, pattern)
	s.mux.HandleFunc(pattern, h)
}

// RoutePatterns returns a copy of every registered "METHOD /path" pattern.
func (s *Server) RoutePatterns() []string {
	out := make([]string, len(s.routePatterns))
	copy(out, s.routePatterns)
	return out
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
	requestedID := strings.TrimSpace(req.SessionID)
	if requestedID != "" && !validSessionID(requestedID) {
		writeJSON(w, http.StatusBadRequest, ErrorResponse{
			Error: "invalid session id: must be 1-128 characters using letters, digits, dash, underscore, or dot (reserved: . and ..)",
			Code:  "invalid_session_id",
		})
		return
	}

	sessionID := requestedID
	if sessionID == "" {
		var idErr error
		sessionID, idErr = newSessionID()
		if idErr != nil {
			slog.Error("session id generation failed", "err", idErr)
			writeJSON(w, http.StatusInternalServerError, ErrorResponse{Error: "session creation failed"})
			return
		}
	}

	lock := s.sessionLock(sessionID)
	lock.Lock()
	defer lock.Unlock()

	var saved *hawksession.Session
	if requestedID != "" {
		var loadErr error
		saved, loadErr = hawksession.Load(sessionID)
		if loadErr != nil {
			if !errors.Is(loadErr, hawksession.ErrNotFound) {
				slog.Error("load continuation session failed", "err", loadErr, "session_id", sessionID)
				writeJSON(w, http.StatusInternalServerError, ErrorResponse{
					Error: "session load failed",
					Code:  "session_load_failed",
				})
				return
			}
			writeJSON(w, http.StatusNotFound, ErrorResponse{
				Error: "session not found",
				Code:  "session_not_found",
			})
			return
		}
		if strings.TrimSpace(req.Model) == "" {
			req.Model = saved.Model
		}
		if strings.TrimSpace(req.Agent) == "" {
			req.Agent = saved.Agent
		}
	}

	requestedCWD := strings.TrimSpace(req.CWD)
	switch {
	case requestedCWD != "":
		canonicalCWD, cwdErr := canonicalSessionCWD(requestedCWD)
		if cwdErr != nil {
			writeJSON(w, http.StatusBadRequest, ErrorResponse{
				Error:   "invalid cwd",
				Code:    "invalid_cwd",
				Details: cwdErr.Error(),
			})
			return
		}
		req.CWD = canonicalCWD
	case saved != nil && saved.CWD != "":
		// CWD is durable session metadata. Inherit it on continuation even
		// if that directory has since been removed.
		req.CWD = saved.CWD
	default:
		canonicalCWD, cwdErr := canonicalSessionCWD("")
		if cwdErr != nil {
			slog.Error("resolve daemon cwd failed", "err", cwdErr)
			writeJSON(w, http.StatusInternalServerError, ErrorResponse{Error: "session creation failed"})
			return
		}
		req.CWD = canonicalCWD
	}

	req.SessionID = sessionID
	req.Agent = strings.TrimSpace(req.Agent)

	sess, err := s.newSession(req)
	if err != nil {
		var requestErr *InvalidChatRequestError
		if errors.As(err, &requestErr) {
			writeJSON(w, http.StatusBadRequest, ErrorResponse{
				Error: requestErr.Message,
				Code:  "invalid_chat_request",
			})
			return
		}
		slog.Error("session create failed", "err", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "session creation failed"})
		return
	}
	if sess == nil {
		slog.Error("session factory returned nil session")
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "session creation failed"})
		return
	}
	if saved != nil {
		sess.LoadMessages(hawksession.ToRuntimeMessages(saved.Messages))
	}

	// Set autonomy
	if req.Autonomy != "" {
		sess.PermSvc().SetAutonomy(engine.ParseAutonomyLevel(req.Autonomy))
	}

	// Auto-approve permissions based on autonomy (non-interactive)
	sess.SetPermissionFn(func(pr engine.PermissionRequest) {
		cfg := engine.PresetConfig(sess.PermSvc().Autonomy())
		allowed := !cfg.NeedsPermission(pr.ToolName, false)
		if pr.Response != nil {
			pr.Response <- allowed
		}
	})

	if req.MaxTurns > 0 {
		if setErr := sess.SetMaxTurns(req.MaxTurns); setErr != nil {
			slog.Error("invalid max turns", "err", setErr, "max_turns", req.MaxTurns)
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid max_turns"})
			return
		}
	}

	sess.AddUser(req.Prompt)

	events, err := sess.Stream(r.Context())
	if err != nil {
		slog.Error("stream failed", "err", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "stream failed"})
		return
	}

	if wantsSSE(r) {
		// Publish the user turn before exposing the session ID in streaming
		// response headers. Even if the client disconnects mid-stream, the ID
		// it received remains retrievable and resumable.
		if saveErr := persistDaemonSession(sessionID, req, sess, saved, start); saveErr != nil {
			slog.Error("persist streaming session start failed", "err", saveErr, "session_id", sessionID)
			writeJSON(w, http.StatusInternalServerError, ErrorResponse{Error: "session persistence failed"})
			return
		}
		streamSSE(s, w, r, events, sessionID, req, sess, saved, start)
		return
	}

	writeJSONResponse(s, w, events, sessionID, req, sess, saved, start)
}

// streamSSE writes a streaming response as SSE events, observing client
// disconnect via r.Context().Done() so the handler does not keep pushing
// events to a dead connection.
func streamSSE(s *Server, w http.ResponseWriter, r *http.Request, events <-chan engine.StreamEvent, sessionID string, req ChatRequest, sess *engine.Session, saved *hawksession.Session, start time.Time) {
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("X-Hawk-Session-ID", sessionID)
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.Header().Set("X-Frame-Options", "DENY")
	flusher, _ := w.(http.Flusher)
	var totalIn, totalOut, turns int

	for {
		select {
		case <-r.Context().Done():
			slog.Info("SSE client disconnected", "session_id", sessionID)
			return
		case ev, ok := <-events:
			if !ok {
				if saveErr := persistDaemonSession(sessionID, req, sess, saved, start); saveErr != nil {
					slog.Error("persist streaming session failed", "err", saveErr, "session_id", sessionID)
					_, _ = fmt.Fprint(w, "event: error\ndata: session persistence failed\n\n")
					if flusher != nil {
						flusher.Flush()
					}
					return
				}
				s.trackSession(sessionID, req, saved, start, turns)
				doneData, _ := json.Marshal(map[string]interface{}{
					"session_id":  sessionID,
					"tokens_in":   totalIn,
					"tokens_out":  totalOut,
					"turns_taken": turns,
				})
				_, _ = fmt.Fprintf(w, "event: done\ndata: %s\n\n", doneData)
				if flusher != nil {
					flusher.Flush()
				}
				return
			}
			switch ev.Type {
			case "content":
				for _, line := range strings.Split(ev.Content, "\n") {
					_, _ = fmt.Fprintf(w, "data: %s\n", line)
				}
				_, _ = fmt.Fprint(w, "\n")
			case "usage":
				if ev.Usage != nil {
					totalIn += ev.Usage.PromptTokens
					totalOut += ev.Usage.CompletionTokens
					turns++
				}
			}
			if flusher != nil {
				flusher.Flush()
			}
		}
	}
}

// writeJSONResponse accumulates events and writes a single JSON response.
func writeJSONResponse(s *Server, w http.ResponseWriter, events <-chan engine.StreamEvent, sessionID string, req ChatRequest, sess *engine.Session, saved *hawksession.Session, start time.Time) {
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

	if saveErr := persistDaemonSession(sessionID, req, sess, saved, start); saveErr != nil {
		slog.Error("persist session failed", "err", saveErr, "session_id", sessionID)
		writeJSON(w, http.StatusInternalServerError, ErrorResponse{Error: "session persistence failed"})
		return
	}
	s.trackSession(sessionID, req, saved, start, turns)
	w.Header().Set("X-Hawk-Session-ID", sessionID)

	writeJSON(w, http.StatusOK, ChatResponse{
		SessionID:  sessionID,
		Response:   response.String(),
		TokensIn:   totalIn,
		TokensOut:  totalOut,
		TurnsTaken: turns,
		Duration:   time.Since(start).Round(time.Millisecond).String(),
	})
}

func validSessionID(id string) bool {
	return hawksession.ValidID(id)
}

func newSessionID() (string, error) {
	var entropy [16]byte
	if _, err := rand.Read(entropy[:]); err != nil {
		return "", err
	}
	return "daemon-" + hex.EncodeToString(entropy[:]), nil
}

func canonicalSessionCWD(cwd string) (string, error) {
	if cwd == "" {
		var err error
		cwd, err = os.Getwd()
		if err != nil {
			return "", err
		}
	}
	abs, err := filepath.Abs(cwd)
	if err != nil {
		return "", err
	}
	canonical, err := filepath.EvalSymlinks(abs)
	if err != nil {
		return "", err
	}
	info, err := os.Stat(canonical)
	if err != nil {
		return "", err
	}
	if !info.IsDir() {
		return "", fmt.Errorf("%s is not a directory", canonical)
	}
	return canonical, nil
}

func (s *Server) sessionLock(id string) *sync.Mutex {
	// FNV-1a is sufficient here: the hash only selects a synchronization
	// stripe; session IDs remain validated and are never trusted as paths.
	const offset64 = uint64(14695981039346656037)
	const prime64 = uint64(1099511628211)
	hash := offset64
	for i := 0; i < len(id); i++ {
		hash ^= uint64(id[i])
		hash *= prime64
	}
	return &s.sessionLocks[hash%uint64(len(s.sessionLocks))]
}

func persistDaemonSession(id string, req ChatRequest, sess *engine.Session, previous *hawksession.Session, startedAt time.Time) error {
	createdAt := startedAt
	name := ""
	if previous != nil {
		createdAt = previous.CreatedAt
		name = previous.Name
	}
	return hawksession.Save(&hawksession.Session{
		ID:        id,
		Model:     sess.Model(),
		Provider:  sess.Provider(),
		Agent:     req.Agent,
		CWD:       req.CWD,
		Name:      name,
		Messages:  hawksession.FromRuntimeMessages(sess.RawMessages()),
		CreatedAt: createdAt,
	})
}

func (s *Server) trackSession(id string, req ChatRequest, previous *hawksession.Session, startedAt time.Time, turns int) {
	createdAt := startedAt
	if previous != nil && !previous.CreatedAt.IsZero() {
		createdAt = previous.CreatedAt
	}
	if current, ok := s.sessions.Load(id); ok {
		if active, activeOK := current.(*Session); activeOK {
			turns += active.Turns
		}
	}
	s.sessions.Store(id, &Session{
		ID:        id,
		CreatedAt: createdAt,
		LastUsed:  time.Now(),
		Turns:     turns,
		CWD:       req.CWD,
		Agent:     req.Agent,
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
	if err := os.MkdirAll(dir, 0o750); err != nil {
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
	return storage.DaemonRunDir()
}
