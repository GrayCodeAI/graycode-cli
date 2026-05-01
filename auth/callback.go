package auth

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"time"
)

// CallbackServer is a temporary localhost HTTP server that captures OAuth redirects.
type CallbackServer struct {
	server        *http.Server
	listener      net.Listener
	port          int
	expectedState string
	codeChan      chan string
	errChan       chan error
}

// NewCallbackServer creates a new OAuth callback server.
func NewCallbackServer(expectedState string) *CallbackServer {
	return &CallbackServer{
		expectedState: expectedState,
		codeChan:      make(chan string, 1),
		errChan:       make(chan error, 1),
	}
}

// Start starts the callback server on an OS-assigned port.
func (s *CallbackServer) Start() (int, error) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return 0, fmt.Errorf("binding callback listener: %w", err)
	}
	s.listener = listener
	s.port = listener.Addr().(*net.TCPAddr).Port

	mux := http.NewServeMux()
	mux.HandleFunc("/callback", s.handleCallback)

	s.server = &http.Server{
		Handler:      mux,
		ReadTimeout:  10 * time.Second,
		WriteTimeout: 10 * time.Second,
	}

	go func() {
		if err := s.server.Serve(listener); err != nil && err != http.ErrServerClosed {
			s.errChan <- err
		}
	}()

	return s.port, nil
}

// Port returns the port the server is listening on.
func (s *CallbackServer) Port() int {
	return s.port
}

// WaitForCode blocks until the OAuth callback delivers a code or the context is cancelled.
func (s *CallbackServer) WaitForCode(ctx context.Context) (string, error) {
	select {
	case code := <-s.codeChan:
		return code, nil
	case err := <-s.errChan:
		return "", err
	case <-ctx.Done():
		return "", ctx.Err()
	}
}

// Close shuts down the callback server.
func (s *CallbackServer) Close() {
	if s.server != nil {
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		s.server.Shutdown(ctx)
	}
}

func (s *CallbackServer) handleCallback(w http.ResponseWriter, r *http.Request) {
	query := r.URL.Query()

	if errMsg := query.Get("error"); errMsg != "" {
		desc := query.Get("error_description")
		s.errChan <- fmt.Errorf("oauth error: %s - %s", errMsg, desc)
		http.Error(w, "Authentication failed: "+errMsg, http.StatusBadRequest)
		return
	}

	state := query.Get("state")
	if state != s.expectedState {
		s.errChan <- fmt.Errorf("state mismatch: expected %q, got %q", s.expectedState, state)
		http.Error(w, "Invalid state parameter", http.StatusBadRequest)
		return
	}

	code := query.Get("code")
	if code == "" {
		s.errChan <- fmt.Errorf("no code in callback")
		http.Error(w, "No authorization code received", http.StatusBadRequest)
		return
	}

	w.Header().Set("Content-Type", "text/html")
	fmt.Fprint(w, `<!DOCTYPE html><html><body>
<h1>Authentication successful!</h1>
<p>You can close this window and return to the terminal.</p>
<script>window.close()</script>
</body></html>`)

	s.codeChan <- code
}
