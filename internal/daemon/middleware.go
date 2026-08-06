package daemon

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"log/slog"
	"net/http"
	"time"

	"github.com/GrayCodeAI/hawk/internal/feature"
)

// requestIDKey is the context key for the request ID.
type requestIDKey struct{}

// RequestIDFromContext extracts the request ID from the context.
func RequestIDFromContext(ctx context.Context) string {
	if v, ok := ctx.Value(requestIDKey{}).(string); ok {
		return v
	}
	return ""
}

// generateRequestID generates a random 16-byte hex request ID.
func generateRequestID() string {
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		return fmt.Sprintf("req-%d", time.Now().UnixNano())
	}
	return hex.EncodeToString(b[:])
}

// requestIDMiddleware assigns a unique request ID to every incoming request,
// stores it in the context, and adds it to the response as X-Request-ID.
func (s *Server) requestIDMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		id := r.Header.Get("X-Request-ID")
		if id == "" {
			id = generateRequestID()
		}
		ctx := context.WithValue(r.Context(), requestIDKey{}, id)
		w.Header().Set("X-Request-ID", id)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

// loggingMiddleware logs each HTTP request with method, path, remote IP,
// status code, and duration. It wraps the response writer to capture the
// status code.
func (s *Server) loggingMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		ww := &responseWriter{ResponseWriter: w, status: http.StatusOK}
		defer func() {
			duration := time.Since(start)
			reqID := RequestIDFromContext(r.Context())
			slog.Info(
				"http_request",
				"method", r.Method,
				"path", r.URL.Path,
				"remote", clientIP(r),
				"status", ww.status,
				"duration_ms", duration.Milliseconds(),
				"request_id", reqID,
				"user_agent", r.Header.Get("User-Agent"),
			)
			s.metrics.Timer("http.request_duration_ms").Record(duration)
		}()
		next.ServeHTTP(ww, r)
	})
}

// securityHeadersMiddleware adds standard security headers to every response.
func (s *Server) securityHeadersMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("X-Frame-Options", "DENY")
		w.Header().Set("Referrer-Policy", "strict-origin-when-cross-origin")
		w.Header().Set("X-XSS-Protection", "1; mode=block")

		// Content-Security-Policy: allow nothing inline by default.
		// Daemon serves JSON/JSONP/SSE — no inline scripts or styles needed.
		w.Header().Set("Content-Security-Policy", "default-src 'none'; frame-ancestors 'none'")

		// HSTS: only set when bound to non-loopback or TLS.
		if r.TLS != nil || r.Header.Get("X-Forwarded-Proto") == "https" {
			w.Header().Set("Strict-Transport-Security", "max-age=31536000; includeSubDomains")
		}

		// Prevent caching of sensitive responses.
		if r.Method == http.MethodPost || r.Method == http.MethodDelete {
			w.Header().Set("Cache-Control", "no-store")
		}

		next.ServeHTTP(w, r)
	})
}

// corsMiddleware adds CORS headers for cross-origin requests.
// When CORSOrigins includes "*", all origins are allowed (useful for
// development). In production, specify exact origins.
func (s *Server) corsMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if len(s.corsOrigins) == 0 {
			next.ServeHTTP(w, r)
			return
		}

		origin := r.Header.Get("Origin")
		if origin == "" {
			next.ServeHTTP(w, r)
			return
		}

		allowed := s.isCORSSettingAllowed(origin)
		if allowed {
			w.Header().Set("Access-Control-Allow-Origin", origin)
			w.Header().Set("Access-Control-Allow-Credentials", "true")
			w.Header().Set("Vary", "Origin")
		}

		if r.Method == http.MethodOptions {
			w.Header().Set("Access-Control-Allow-Methods", "GET, POST, DELETE, OPTIONS")
			w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization, X-API-Key, X-Request-ID")
			w.Header().Set("Access-Control-Max-Age", "86400")
			w.WriteHeader(http.StatusOK)
			return
		}

		next.ServeHTTP(w, r)
	})
}

// responseWriter wraps http.ResponseWriter to capture the status code.
type responseWriter struct {
	http.ResponseWriter
	status int
}

func (rw *responseWriter) WriteHeader(code int) {
	rw.status = code
	rw.ResponseWriter.WriteHeader(code)
}

func (rw *responseWriter) Write(b []byte) (int, error) {
	return rw.ResponseWriter.Write(b)
}

// isCORSSettingAllowed reports whether the given origin is permitted
// by the configured CORS origins.
func (s *Server) isCORSSettingAllowed(origin string) bool {
	for _, o := range s.corsOrigins {
		if o == "*" {
			return true
		}
		if o == origin {
			return true
		}
	}
	return false
}

// installMiddleware wraps the server's mux with the standard middleware
// stack: request IDs → security headers (if enabled) → CORS (if enabled) →
// request logging. This is called after routes() to ensure all routes are
// registered.
func (s *Server) installMiddleware() {
	handler := http.Handler(s.mux)
	handler = s.requestIDMiddleware(handler)
	if feature.Enabled(feature.SecurityHeaders) {
		handler = s.securityHeadersMiddleware(handler)
	}
	// CORS is active when the feature flag is on AND origins are configured.
	if feature.Enabled(feature.CORS) && len(s.corsOrigins) > 0 {
		handler = s.corsMiddleware(handler)
	}
	handler = s.loggingMiddleware(handler)
	s.server.Handler = handler
}
