// Package remote provides remote session support stubs.
// This is a foundation for SSH/direct-connect server capabilities.
package remote

import (
	"context"
	"fmt"
	"time"
)

// Session represents a remote hawk session.
type Session struct {
	ID       string    `json:"id"`
	Host     string    `json:"host"`
	Port     int       `json:"port"`
	User     string    `json:"user"`
	Auth     Auth      `json:"auth"`
	Created  time.Time `json:"created"`
	LastPing time.Time `json:"last_ping"`
}

// Auth describes authentication for a remote session.
type Auth struct {
	Type     string `json:"type"` // "key", "password", "agent"
	KeyPath  string `json:"key_path,omitempty"`
	Password string `json:"password,omitempty"`
}

// ServerConfig describes a remote hawk server.
type ServerConfig struct {
	Host        string        `json:"host"`
	Port        int           `json:"port"`
	TLSCert     string        `json:"tls_cert,omitempty"`
	TLSKey      string        `json:"tls_key,omitempty"`
	MaxSessions int           `json:"max_sessions"`
	Timeout     time.Duration `json:"timeout"`
}

// DefaultServerConfig returns a default server configuration.
func DefaultServerConfig() *ServerConfig {
	return &ServerConfig{
		Host:        "0.0.0.0",
		Port:        8080,
		MaxSessions: 100,
		Timeout:     30 * time.Minute,
	}
}

// Validate checks if the server config is valid.
func (c *ServerConfig) Validate() error {
	if c.Port <= 0 || c.Port > 65535 {
		return fmt.Errorf("invalid port: %d", c.Port)
	}
	if c.MaxSessions <= 0 {
		return fmt.Errorf("max_sessions must be positive")
	}
	return nil
}

// Manager manages remote sessions.
type Manager struct {
	sessions map[string]*Session
	config   *ServerConfig
}

// NewManager creates a new remote session manager.
func NewManager(config *ServerConfig) *Manager {
	return &Manager{
		sessions: make(map[string]*Session),
		config:   config,
	}
}

// List returns all active sessions.
func (m *Manager) List() []*Session {
	out := make([]*Session, 0, len(m.sessions))
	for _, s := range m.sessions {
		out = append(out, s)
	}
	return out
}

// Get returns a session by ID.
func (m *Manager) Get(id string) (*Session, bool) {
	s, ok := m.sessions[id]
	return s, ok
}

// Create creates a new remote session.
func (m *Manager) Create(host string, port int, user string, auth Auth) (*Session, error) {
	if len(m.sessions) >= m.config.MaxSessions {
		return nil, fmt.Errorf("max sessions reached")
	}

	s := &Session{
		ID:      generateID(),
		Host:    host,
		Port:    port,
		User:    user,
		Auth:    auth,
		Created: time.Now(),
	}
	m.sessions[s.ID] = s
	return s, nil
}

// Remove removes a session.
func (m *Manager) Remove(id string) {
	delete(m.sessions, id)
}

// Ping updates the last ping time for a session.
func (m *Manager) Ping(id string) {
	if s, ok := m.sessions[id]; ok {
		s.LastPing = time.Now()
	}
}

// Cleanup removes stale sessions.
func (m *Manager) Cleanup(timeout time.Duration) int {
	now := time.Now()
	removed := 0
	for id, s := range m.sessions {
		if now.Sub(s.LastPing) > timeout {
			delete(m.sessions, id)
			removed++
		}
	}
	return removed
}

func generateID() string {
	return fmt.Sprintf("remote-%d", time.Now().UnixNano())
}

// Connect establishes a connection to a remote hawk server.
// This is a stub for future SSH/WebSocket implementation.
func Connect(ctx context.Context, host string, port int, auth Auth) (*Session, error) {
	return nil, fmt.Errorf("remote connections not yet implemented")
}

// Serve starts a remote hawk server.
// This is a stub for future server implementation.
func Serve(ctx context.Context, config *ServerConfig) error {
	return fmt.Errorf("remote server not yet implemented")
}
