// Package api provides an HTTP API server for hawk, consumable by SDKs.
package api

import (
	"context"
	"encoding/json"
	"net"
	"net/http"
	"sync"
)

// Version is the current hawk API version.
const Version = "0.2.0"

// Server is the HTTP API server for hawk.
type Server struct {
	addr   string
	mux    *http.ServeMux
	server *http.Server
	mu     sync.Mutex
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
	mux := http.NewServeMux()
	s := &Server{
		addr: addr,
		mux:  mux,
	}
	s.registerRoutes()
	return s
}

// registerRoutes sets up the HTTP endpoints.
func (s *Server) registerRoutes() {
	s.mux.HandleFunc("GET /health", s.handleHealth)
	s.mux.HandleFunc("GET /version", s.handleVersion)
	s.mux.HandleFunc("POST /chat", s.handleChat)
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
		s.server.Shutdown(context.Background())
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
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, `{"error":"invalid request body"}`, http.StatusBadRequest)
		return
	}
	if req.Message == "" {
		http.Error(w, `{"error":"message is required"}`, http.StatusBadRequest)
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
	json.NewEncoder(w).Encode(v)
}
