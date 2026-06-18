// Package api provides an HTTP API server for hawk, consumable by SDKs.
package api

import (
	"context"
	"crypto/subtle"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"strings"
	"sync"
	"time"
)

const maxRequestBodyBytes = 1 << 20

// Version is the current hawk API surface version, exposed in the GET /version
// endpoint. It is wired at startup by main.go from the canonical version
// (the VERSION file at the repo root, injected via ldflags). The "dev"
// default applies only to local builds without ldflags.
var Version = "dev"

// SetVersion lets main.go propagate the canonical hawk version into this
// package without creating an import cycle with cmd.
func SetVersion(v string) { Version = v }

// Server is the HTTP API server for hawk.
type Server struct {
	addr   string
	mux    *http.ServeMux
	server *http.Server
	mu     sync.Mutex
	apiKey string
}

// ChatRequest is the request body for POST /chat.
type ChatRequest struct {
	Message string `json:"message"`
	Model   string `json:"model"`
}

// ChatResponse is the response body for POST /chat.
type ChatResponse struct {
	Response string `json:"response"`
}

// HealthResponse is the response body for GET /health.
type HealthResponse struct {
	Status string `json:"status"`
}

// VersionResponse is the response body for GET /version.
type VersionResponse struct {
	Version string `json:"version"`
}

// New creates a new API server listening on the given address.
func New(addr string) *Server {
	return NewWithAPIKey(addr, "")
}

// NewWithAPIKey creates a new API server and protects mutating endpoints when
// apiKey is non-empty.
func NewWithAPIKey(addr, apiKey string) *Server {
	mux := http.NewServeMux()
	s := &Server{
		addr:   addr,
		mux:    mux,
		apiKey: apiKey,
	}
	s.registerRoutes()
	return s
}

// registerRoutes sets up the HTTP endpoints.
func (s *Server) registerRoutes() {
	s.mux.HandleFunc("GET /health", securityHeaders(s.handleHealth))
	s.mux.HandleFunc("GET /version", securityHeaders(s.handleVersion))
	s.mux.HandleFunc("POST /chat", securityHeaders(s.auth(s.handleChat)))
}

// securityHeaders sets standard HTTP security headers on every response.
func securityHeaders(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("X-Frame-Options", "DENY")
		w.Header().Set("Cache-Control", "no-store")
		next(w, r)
	}
}

func (s *Server) auth(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if s.apiKey == "" {
			next(w, r)
			return
		}
		token := r.Header.Get("Authorization")
		token = strings.TrimPrefix(token, "Bearer ")
		if token == "" {
			token = r.Header.Get("X-API-Key")
		}
		if !constantTimeEqual(token, s.apiKey) {
			writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "unauthorized"})
			return
		}
		next(w, r)
	}
}

func constantTimeEqual(a, b string) bool {
	// Compare lengths in constant time first, then content.
	// This avoids null-byte padding which could produce false positives.
	if len(a) != len(b) {
		// Still do a comparison to avoid early-return timing leak.
		// Compare against a to consume the same time as a matching-length path.
		subtle.ConstantTimeCompare([]byte(a), []byte(a))
		return false
	}
	return subtle.ConstantTimeCompare([]byte(a), []byte(b)) == 1
}

// validateAuthConfig refuses to start the server with no API key on a
// non-loopback bind. The auth middleware silently allows every request when
// the API key is empty, so a misconfigured server would be wide open. The
// only safe no-key mode is loopback bind.
func (s *Server) validateAuthConfig() error {
	if s.apiKey != "" {
		return nil
	}
	host, _, err := net.SplitHostPort(s.addr)
	if err != nil {
		return fmt.Errorf("api: invalid bind address %q: %w", s.addr, err)
	}
	if !isLoopbackHost(host) {
		return fmt.Errorf("api: apiKey is empty and bind address %q is not loopback; refusing to start. Set apiKey or bind to 127.0.0.1", s.addr)
	}
	return nil
}

// isLoopbackHost reports whether host is a loopback address.
func isLoopbackHost(host string) bool {
	if host == "" || host == "localhost" {
		return host == "localhost" // "" is unsafe; "localhost" is loopback
	}
	if ip := net.ParseIP(host); ip != nil {
		return ip.IsLoopback()
	}
	return false
}

func decodeJSONBody(w http.ResponseWriter, r *http.Request, dst any) bool {
	r.Body = http.MaxBytesReader(w, r.Body, maxRequestBodyBytes)
	dec := json.NewDecoder(r.Body)
	dec.DisallowUnknownFields()
	if err := dec.Decode(dst); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid request body"})
		return false
	}
	if err := dec.Decode(&struct{}{}); err != io.EOF {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "request body must contain a single JSON object"})
		return false
	}
	return true
}

// Start starts the HTTP server. It blocks until the context is cancelled or an error occurs.
func (s *Server) Start(ctx context.Context) error {
	if err := s.validateAuthConfig(); err != nil {
		return err
	}
	s.mu.Lock()
	s.server = &http.Server{
		Addr:              s.addr,
		Handler:           s.mux,
		ReadHeaderTimeout: 10 * time.Second,
		ReadTimeout:       30 * time.Second,
		WriteTimeout:      30 * time.Second,
		IdleTimeout:       120 * time.Second,
	}
	s.mu.Unlock()

	ln, err := new(net.ListenConfig).Listen(ctx, "tcp", s.addr)
	if err != nil {
		return err
	}

	go func() {
		<-ctx.Done()
		_ = s.server.Shutdown(context.Background())
	}()

	err = s.server.Serve(ln)
	if err == http.ErrServerClosed {
		return nil
	}
	return err
}

// Stop gracefully shuts down the HTTP server with a 15-second timeout.
func (s *Server) Stop(ctx context.Context) error {
	s.mu.Lock()
	srv := s.server
	s.mu.Unlock()

	if srv == nil {
		return nil
	}
	// Use a bounded timeout so Stop cannot hang indefinitely if a
	// client keeps a connection open. The caller's ctx is respected
	// if it has a shorter deadline.
	shutdownCtx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()
	return srv.Shutdown(shutdownCtx)
}

// Handler returns the underlying http.Handler for testing purposes.
func (s *Server) Handler() http.Handler {
	return s.mux
}

func (s *Server) handleHealth(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, HealthResponse{Status: "ok"})
}

func (s *Server) handleVersion(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, VersionResponse{Version: Version})
}

func (s *Server) handleChat(w http.ResponseWriter, r *http.Request) {
	var req ChatRequest
	if !decodeJSONBody(w, r, &req) {
		return
	}
	if req.Message == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "message is required"})
		return
	}

	// Stub response
	resp := ChatResponse{
		Response: "This is a stub response. Model: " + req.Model + ", Message: " + req.Message,
	}
	writeJSON(w, http.StatusOK, resp)
}

func writeJSON(w http.ResponseWriter, status int, v interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}
