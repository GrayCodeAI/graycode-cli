// Package api provides an HTTP API server for hawk, consumable by SDKs.
package api

import (
	"context"
	"crypto/subtle"
	"encoding/json"
	"io"
	"net"
	"net/http"
	"strings"
	"sync"
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
	s.mux.HandleFunc("GET /health", s.handleHealth)
	s.mux.HandleFunc("GET /version", s.handleVersion)
	s.mux.HandleFunc("POST /chat", s.auth(s.handleChat))
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
	s.mu.Lock()
	s.server = &http.Server{
		Addr:    s.addr,
		Handler: s.mux,
	}
	s.mu.Unlock()

	ln, err := net.Listen("tcp", s.addr)
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

// Stop gracefully shuts down the HTTP server.
func (s *Server) Stop(ctx context.Context) error {
	s.mu.Lock()
	srv := s.server
	s.mu.Unlock()

	if srv == nil {
		return nil
	}
	return srv.Shutdown(ctx)
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
