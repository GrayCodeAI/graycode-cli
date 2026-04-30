package tool

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"sync"
	"time"
)

// MCPAuthState tracks OAuth state for an MCP server.
type MCPAuthState struct {
	ServerName string `json:"serverName"`
	AuthURL    string `json:"authUrl,omitempty"`
	Status     string `json:"status"` // "pending", "authenticated", "error"
	Error      string `json:"error,omitempty"`
}

// MCPAuthManager handles OAuth flows for MCP servers.
type MCPAuthManager struct {
	mu     sync.RWMutex
	states map[string]*MCPAuthState
}

var globalMCPAuthManager = &MCPAuthManager{states: make(map[string]*MCPAuthState)}

func GetMCPAuthManager() *MCPAuthManager { return globalMCPAuthManager }

func (m *MCPAuthManager) StartAuth(serverName, serverURL string) (*MCPAuthState, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	// Build OAuth discovery URL
	parsed, err := url.Parse(serverURL)
	if err != nil {
		return nil, fmt.Errorf("invalid server URL: %w", err)
	}

	// Attempt to discover OAuth endpoint
	wellKnown := fmt.Sprintf("%s://%s/.well-known/oauth-authorization-server", parsed.Scheme, parsed.Host)
	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Get(wellKnown)
	if err != nil || resp.StatusCode != 200 {
		state := &MCPAuthState{
			ServerName: serverName,
			Status:     "unsupported",
			Error:      "Server does not support OAuth authentication",
		}
		m.states[serverName] = state
		return state, nil
	}
	defer resp.Body.Close()

	var oauthConfig struct {
		AuthorizationEndpoint string `json:"authorization_endpoint"`
		TokenEndpoint         string `json:"token_endpoint"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&oauthConfig); err != nil {
		return nil, fmt.Errorf("parsing OAuth config: %w", err)
	}

	authURL := oauthConfig.AuthorizationEndpoint
	if authURL == "" {
		authURL = fmt.Sprintf("%s://%s/oauth/authorize", parsed.Scheme, parsed.Host)
	}

	state := &MCPAuthState{
		ServerName: serverName,
		AuthURL:    authURL,
		Status:     "pending",
	}
	m.states[serverName] = state
	return state, nil
}

func (m *MCPAuthManager) GetState(serverName string) (*MCPAuthState, bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	s, ok := m.states[serverName]
	return s, ok
}

func (m *MCPAuthManager) SetAuthenticated(serverName string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if s, ok := m.states[serverName]; ok {
		s.Status = "authenticated"
	}
}

// McpAuthTool initiates OAuth authentication for an MCP server.
type McpAuthTool struct{}

func (McpAuthTool) Name() string        { return "McpAuth" }
func (McpAuthTool) Aliases() []string   { return []string{"mcp_auth"} }
func (McpAuthTool) Description() string {
	return "Start OAuth authentication for an MCP server that requires authorization"
}
func (McpAuthTool) Parameters() map[string]interface{} {
	return map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"server_name": map[string]interface{}{
				"type":        "string",
				"description": "Name of the MCP server to authenticate",
			},
			"server_url": map[string]interface{}{
				"type":        "string",
				"description": "URL of the MCP server",
			},
		},
		"required": []string{"server_name", "server_url"},
	}
}

func (McpAuthTool) Execute(ctx context.Context, input json.RawMessage) (string, error) {
	var p struct {
		ServerName string `json:"server_name"`
		ServerURL  string `json:"server_url"`
	}
	if err := json.Unmarshal(input, &p); err != nil {
		return "", err
	}
	if p.ServerName == "" {
		return "", fmt.Errorf("server_name is required")
	}
	if p.ServerURL == "" {
		return "", fmt.Errorf("server_url is required")
	}

	state, err := globalMCPAuthManager.StartAuth(p.ServerName, p.ServerURL)
	if err != nil {
		return "", err
	}

	out, _ := json.Marshal(map[string]any{
		"status":  state.Status,
		"message": formatAuthMessage(state),
		"authUrl": state.AuthURL,
	})
	return string(out), nil
}

func formatAuthMessage(state *MCPAuthState) string {
	switch state.Status {
	case "pending":
		return fmt.Sprintf("Please visit the following URL to authorize: %s", state.AuthURL)
	case "unsupported":
		return fmt.Sprintf("Server %q does not support OAuth authentication", state.ServerName)
	case "authenticated":
		return fmt.Sprintf("Server %q is already authenticated", state.ServerName)
	default:
		return state.Error
	}
}
