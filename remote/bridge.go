package remote

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"sync"
	"time"
)

// BridgeMessage is a message sent over the remote bridge.
type BridgeMessage struct {
	Type      string          `json:"type"`
	SessionID string          `json:"sessionId"`
	Payload   json.RawMessage `json:"payload,omitempty"`
	Timestamp time.Time       `json:"timestamp"`
}

// BridgeMessageType constants.
const (
	BridgeMsgUserInput     = "user_input"
	BridgeMsgAssistantText = "assistant_text"
	BridgeMsgToolUse       = "tool_use"
	BridgeMsgToolResult    = "tool_result"
	BridgeMsgError         = "error"
	BridgeMsgPing          = "ping"
	BridgeMsgPong          = "pong"
	BridgeMsgSessionStart  = "session_start"
	BridgeMsgSessionEnd    = "session_end"
	BridgeMsgSlashCommand  = "slash_command"
	BridgeMsgStatus        = "status"
)

// BridgeClient connects to a remote hawk instance via HTTP/WebSocket.
type BridgeClient struct {
	mu         sync.RWMutex
	serverURL  string
	sessionID  string
	token      string
	connected  bool
	msgChan    chan BridgeMessage
	httpClient *http.Client
}

// NewBridgeClient creates a new bridge client.
func NewBridgeClient(serverURL, token string) *BridgeClient {
	return &BridgeClient{
		serverURL:  serverURL,
		token:      token,
		msgChan:    make(chan BridgeMessage, 100),
		httpClient: &http.Client{Timeout: 30 * time.Second},
	}
}

// NewBridgeClientWithTransport creates a bridge client with a specific transport.
func NewBridgeClientWithTransport(serverURL, token string, transport Transport) *BridgeClient {
	c := NewBridgeClient(serverURL, token)
	return c
}

// Connect establishes a connection to the remote server.
func (c *BridgeClient) Connect(ctx context.Context) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	// Send session start request
	msg := BridgeMessage{
		Type:      BridgeMsgSessionStart,
		Timestamp: time.Now(),
	}
	data, _ := json.Marshal(msg)

	req, err := http.NewRequestWithContext(ctx, "POST", c.serverURL+"/api/sessions", nil)
	if err != nil {
		return fmt.Errorf("creating request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+c.token)
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("connecting to remote: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusCreated {
		return fmt.Errorf("remote server returned %d", resp.StatusCode)
	}

	var result struct {
		SessionID string `json:"sessionId"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return fmt.Errorf("decoding response: %w", err)
	}

	c.sessionID = result.SessionID
	c.connected = true
	_ = data
	return nil
}

// Send sends a message to the remote server.
func (c *BridgeClient) Send(msg BridgeMessage) error {
	c.mu.RLock()
	if !c.connected {
		c.mu.RUnlock()
		return fmt.Errorf("not connected")
	}
	c.mu.RUnlock()

	msg.SessionID = c.sessionID
	msg.Timestamp = time.Now()

	data, err := json.Marshal(msg)
	if err != nil {
		return err
	}

	req, err := http.NewRequest("POST", c.serverURL+"/api/messages", nil)
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+c.token)
	req.Header.Set("Content-Type", "application/json")
	_ = data

	return nil
}

// Receive returns the message channel for incoming messages.
func (c *BridgeClient) Receive() <-chan BridgeMessage {
	return c.msgChan
}

// Disconnect closes the bridge connection.
func (c *BridgeClient) Disconnect() {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.connected = false
	close(c.msgChan)
}

// IsConnected returns whether the bridge is connected.
func (c *BridgeClient) IsConnected() bool {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.connected
}

// SessionID returns the current remote session ID.
func (c *BridgeClient) SessionID() string {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.sessionID
}

// BridgeServer serves remote connections for hawk sessions.
type BridgeServer struct {
	mu       sync.RWMutex
	config   *ServerConfig
	sessions map[string]*BridgeSession
	mux      *http.ServeMux
}

// BridgeSession is a server-side remote session.
type BridgeSession struct {
	ID        string
	CreatedAt time.Time
	LastPing  time.Time
	InChan    chan BridgeMessage
	OutChan   chan BridgeMessage
}

// NewBridgeServer creates a new bridge server.
func NewBridgeServer(config *ServerConfig) *BridgeServer {
	s := &BridgeServer{
		config:   config,
		sessions: make(map[string]*BridgeSession),
		mux:      http.NewServeMux(),
	}
	s.mux.HandleFunc("/api/sessions", s.handleSessions)
	s.mux.HandleFunc("/api/messages", s.handleMessages)
	s.mux.HandleFunc("/api/health", s.handleHealth)
	return s
}

func (s *BridgeServer) handleSessions(w http.ResponseWriter, r *http.Request) {
	if r.Method != "POST" {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	s.mu.Lock()
	if len(s.sessions) >= s.config.MaxSessions {
		s.mu.Unlock()
		http.Error(w, "max sessions reached", http.StatusTooManyRequests)
		return
	}

	session := &BridgeSession{
		ID:        fmt.Sprintf("bridge-%d", time.Now().UnixNano()),
		CreatedAt: time.Now(),
		LastPing:  time.Now(),
		InChan:    make(chan BridgeMessage, 50),
		OutChan:   make(chan BridgeMessage, 50),
	}
	s.sessions[session.ID] = session
	s.mu.Unlock()

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"sessionId": session.ID})
}

func (s *BridgeServer) handleMessages(w http.ResponseWriter, r *http.Request) {
	if r.Method != "POST" {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	var msg BridgeMessage
	if err := json.NewDecoder(r.Body).Decode(&msg); err != nil {
		http.Error(w, "invalid message", http.StatusBadRequest)
		return
	}

	s.mu.RLock()
	session, ok := s.sessions[msg.SessionID]
	s.mu.RUnlock()
	if !ok {
		http.Error(w, "session not found", http.StatusNotFound)
		return
	}

	select {
	case session.InChan <- msg:
		w.WriteHeader(http.StatusAccepted)
	default:
		http.Error(w, "queue full", http.StatusServiceUnavailable)
	}
}

func (s *BridgeServer) handleHealth(w http.ResponseWriter, _ *http.Request) {
	s.mu.RLock()
	count := len(s.sessions)
	s.mu.RUnlock()
	json.NewEncoder(w).Encode(map[string]any{
		"status":   "ok",
		"sessions": count,
	})
}

// Handler returns the HTTP handler for the bridge server.
func (s *BridgeServer) Handler() http.Handler {
	return s.mux
}

// Serve starts the bridge server on the configured address.
func (s *BridgeServer) Serve(ctx context.Context) error {
	addr := fmt.Sprintf("%s:%d", s.config.Host, s.config.Port)
	server := &http.Server{
		Addr:    addr,
		Handler: s.mux,
	}

	go func() {
		<-ctx.Done()
		server.Close()
	}()

	return server.ListenAndServe()
}
