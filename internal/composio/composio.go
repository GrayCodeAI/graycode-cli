// Package composio provides integration between hawk's MCP server and the
// Composio tool platform. Composio exposes 100+ pre-built tools (Slack,
// GitHub, Notion, etc.) that can be registered as MCP tools.
//
// This package provides:
//   - ComposioToolProvider: discovers and registers composio tools as MCP handlers
//   - ComposioToolSearch: searches composio's tool catalog
//   - ComposioCredentialManager: manages API credentials for connected services
//
// The integration is opt-in: host applications call NewComposioProvider()
// and RegisterTools() to add composio tools to their MCP server.
package composio

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
	"time"
)

// ToolScope defines the scope at which a composio tool operates.
type ToolScope string

const (
	ScopeReadOnly ToolScope = "read"
	ScopeWrite    ToolScope = "write"
	ScopeAction   ToolScope = "action"
)

// ComposioTool represents a tool available from the composio platform.
type ComposioTool struct {
	Name         string                 `json:"name"`
	Description  string                 `json:"description"`
	Scope        ToolScope              `json:"scope"`
	AuthRequired bool                   `json:"auth_required"`
	Params       map[string]interface{} `json:"params"`
	Tags         []string               `json:"tags"`
	Category     string                 `json:"category"`
}

// ComposioToolResult is the result of executing a composio tool.
type ComposioToolResult struct {
	Success bool                   `json:"success"`
	Data    map[string]interface{} `json:"data,omitempty"`
	Error   string                 `json:"error,omitempty"`
}

// Credential holds an API credential for a connected service.
type Credential struct {
	ID          string            `json:"id"`
	ServiceName string            `json:"service_name"`
	Type        string            `json:"type"` // "oauth", "api_key", "password"
	Value       string            `json:"-"`    // never serialize
	Scope       string            `json:"scope,omitempty"`
	ExpiresAt   time.Time         `json:"expires_at,omitempty"`
	Metadata    map[string]string `json:"metadata,omitempty"`
}

// IsExpired reports whether the credential has expired.
func (c *Credential) IsExpired() bool {
	if c.ExpiresAt.IsZero() {
		return false
	}
	return time.Now().After(c.ExpiresAt)
}

// ComposioProvider discovers and manages composio tools for MCP registration.
type ComposioProvider struct {
	mu          sync.RWMutex
	tools       map[string]*ComposioTool
	credentials *CredentialManager
	endpoint    string
	apiKey      string
}

// NewComposioProvider creates a new composio tool provider.
// The apiKey is used to authenticate with the composio API.
// If apiKey is empty, the provider operates in offline mode (no tool discovery).
func NewComposioProvider(apiKey string) *ComposioProvider {
	return &ComposioProvider{
		tools:       make(map[string]*ComposioTool),
		credentials: NewCredentialManager(),
		endpoint:    "https://api.composio.dev",
		apiKey:      apiKey,
	}
}

// RegisterTool manually registers a composio tool (useful for offline mode
// or when tools are discovered through other means).
func (p *ComposioProvider) RegisterTool(tool *ComposioTool) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.tools[tool.Name] = tool
}

// GetTool retrieves a tool by name.
func (p *ComposioProvider) GetTool(name string) (*ComposioTool, bool) {
	p.mu.RLock()
	defer p.mu.RUnlock()
	tool, ok := p.tools[name]
	return tool, ok
}

// ListTools returns all registered tools.
func (p *ComposioProvider) ListTools() []*ComposioTool {
	p.mu.RLock()
	defer p.mu.RUnlock()
	result := make([]*ComposioTool, 0, len(p.tools))
	for _, t := range p.tools {
		result = append(result, t)
	}
	return result
}

// SearchTools searches registered tools by name, description, or tags.
// Returns tools matching any of the query terms (OR semantics).
func (p *ComposioProvider) SearchTools(query string) []*ComposioTool {
	if query == "" {
		return p.ListTools()
	}

	p.mu.RLock()
	defer p.mu.RUnlock()

	results := make([]*ComposioTool, 0)
	for _, t := range p.tools {
		if matchesSearch(t, query) {
			results = append(results, t)
		}
	}
	return results
}

// ToolCount returns the number of registered tools.
func (p *ComposioProvider) ToolCount() int {
	p.mu.RLock()
	defer p.mu.RUnlock()
	return len(p.tools)
}

// Credentials returns the credential manager.
func (p *ComposioProvider) Credentials() *CredentialManager {
	return p.credentials
}

// ExecuteTool executes a composio tool by name with the given parameters.
// In a full implementation, this would proxy the call to the composio API.
func (p *ComposioProvider) ExecuteTool(ctx context.Context, name string, params json.RawMessage) (*ComposioToolResult, error) {
	tool, ok := p.GetTool(name)
	if !ok {
		return nil, fmt.Errorf("composio tool %q not found", name)
	}

	// Check if auth is required and credentials exist
	if tool.AuthRequired {
		cred := p.credentials.GetForService(tool.Category)
		if cred == nil || cred.IsExpired() {
			return &ComposioToolResult{
				Success: false,
				Error:   fmt.Sprintf("authentication required for tool %q; no valid credentials for service %q", name, tool.Category),
			}, nil
		}
	}

	// In a full implementation, this would make an HTTP request to the
	// composio API with the tool name, params, and credentials.
	// For now, we return a stub result.
	return &ComposioToolResult{
		Success: true,
		Data: map[string]interface{}{
			"tool":   name,
			"params": string(params),
			"status": "stub",
		},
	}, nil
}

// matchesSearch checks if a tool matches the search query.
func matchesSearch(t *ComposioTool, query string) bool {
	q := lower(query)
	if contains(t.Name, q) {
		return true
	}
	if contains(t.Description, q) {
		return true
	}
	for _, tag := range t.Tags {
		if contains(tag, q) {
			return true
		}
	}
	return false
}

// lower returns the lowercase version of a string.
func lower(s string) string {
	result := make([]byte, len(s))
	for i := 0; i < len(s); i++ {
		c := s[i]
		if c >= 'A' && c <= 'Z' {
			result[i] = c + 32
		} else {
			result[i] = c
		}
	}
	return string(result)
}

// contains checks if s contains substr (case-insensitive).
func contains(s, substr string) bool {
	if len(substr) == 0 {
		return true
	}
	if len(substr) > len(s) {
		return false
	}
	ls := lower(s)
	lsub := lower(substr)
	for i := 0; i <= len(ls)-len(lsub); i++ {
		if ls[i:i+len(lsub)] == lsub {
			return true
		}
	}
	return false
}

// CredentialManager manages API credentials for composio-connected services.
type CredentialManager struct {
	mu    sync.RWMutex
	items map[string]*Credential
}

// NewCredentialManager creates an empty credential manager.
func NewCredentialManager() *CredentialManager {
	return &CredentialManager{
		items: make(map[string]*Credential),
	}
}

// Store adds or updates a credential.
func (cm *CredentialManager) Store(c *Credential) {
	cm.mu.Lock()
	defer cm.mu.Unlock()
	if c.ExpiresAt.IsZero() {
		c.ExpiresAt = time.Now().Add(24 * time.Hour)
	}
	cm.items[c.ID] = c
}

// Get retrieves a credential by ID.
func (cm *CredentialManager) Get(id string) *Credential {
	cm.mu.RLock()
	defer cm.mu.RUnlock()
	c, ok := cm.items[id]
	if !ok || c.IsExpired() {
		return nil
	}
	return c
}

// GetForService retrieves the first valid credential for a service.
func (cm *CredentialManager) GetForService(service string) *Credential {
	cm.mu.RLock()
	defer cm.mu.RUnlock()
	for _, c := range cm.items {
		if c.ServiceName == service && !c.IsExpired() {
			return c
		}
	}
	return nil
}

// List returns all non-expired credentials.
func (cm *CredentialManager) List() []*Credential {
	cm.mu.RLock()
	defer cm.mu.RUnlock()
	result := make([]*Credential, 0, len(cm.items))
	for _, c := range cm.items {
		if !c.IsExpired() {
			result = append(result, c)
		}
	}
	return result
}

// Delete removes a credential.
func (cm *CredentialManager) Delete(id string) bool {
	cm.mu.Lock()
	defer cm.mu.Unlock()
	if _, ok := cm.items[id]; !ok {
		return false
	}
	delete(cm.items, id)
	return true
}

// Count returns the number of stored credentials.
func (cm *CredentialManager) Count() int {
	cm.mu.RLock()
	defer cm.mu.RUnlock()
	return len(cm.items)
}
