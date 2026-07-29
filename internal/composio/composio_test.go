package composio

import (
	"context"
	"encoding/json"
	"testing"
	"time"
)

// --- ComposioProvider Tests ---

func TestNewComposioProvider(t *testing.T) {
	t.Parallel()
	p := NewComposioProvider("test-key")
	if p == nil {
		t.Fatal("expected non-nil provider")
	}
	if p.ToolCount() != 0 {
		t.Errorf("expected 0 tools, got %d", p.ToolCount())
	}
	if p.Credentials() == nil {
		t.Error("expected non-nil credential manager")
	}
}

func TestComposioProviderRegisterTool(t *testing.T) {
	t.Parallel()
	p := NewComposioProvider("test-key")

	tool := &ComposioTool{
		Name:         "github_issues",
		Description:  "List and manage GitHub issues",
		Scope:        ScopeReadOnly,
		AuthRequired: true,
		Params:       map[string]interface{}{"repo_id": "string"},
		Tags:         []string{"github", "issues"},
		Category:     "github",
	}
	p.RegisterTool(tool)

	if p.ToolCount() != 1 {
		t.Errorf("expected 1 tool, got %d", p.ToolCount())
	}

	retrieved, ok := p.GetTool("github_issues")
	if !ok {
		t.Fatal("expected to find tool")
	}
	if retrieved.Description != "List and manage GitHub issues" {
		t.Errorf("expected description, got %q", retrieved.Description)
	}
}

func TestComposioProviderListTools(t *testing.T) {
	t.Parallel()
	p := NewComposioProvider("test-key")

	p.RegisterTool(&ComposioTool{Name: "tool1", Category: "github"})
	p.RegisterTool(&ComposioTool{Name: "tool2", Category: "slack"})

	tools := p.ListTools()
	if len(tools) != 2 {
		t.Errorf("expected 2 tools, got %d", len(tools))
	}
}

func TestComposioProviderSearchTools(t *testing.T) {
	t.Parallel()
	p := NewComposioProvider("test-key")

	p.RegisterTool(&ComposioTool{
		Name:        "github_issues",
		Description: "List GitHub issues",
		Tags:        []string{"github", "issues"},
		Category:    "github",
	})
	p.RegisterTool(&ComposioTool{
		Name:        "slack_messages",
		Description: "Send Slack messages",
		Tags:        []string{"slack", "messaging"},
		Category:    "slack",
	})

	// Search by name
	results := p.SearchTools("github")
	if len(results) != 1 {
		t.Errorf("expected 1 result for 'github', got %d", len(results))
	}
	if results[0].Name != "github_issues" {
		t.Errorf("expected 'github_issues', got %q", results[0].Name)
	}

	// Search by tag
	results = p.SearchTools("messaging")
	if len(results) != 1 {
		t.Errorf("expected 1 result for 'messaging', got %d", len(results))
	}

	// Empty query returns all
	results = p.SearchTools("")
	if len(results) != 2 {
		t.Errorf("expected 2 results for empty query, got %d", len(results))
	}

	// No match
	results = p.SearchTools("nonexistent")
	if len(results) != 0 {
		t.Errorf("expected 0 results for 'nonexistent', got %d", len(results))
	}
}

func TestComposioProviderGetToolNotFound(t *testing.T) {
	t.Parallel()
	p := NewComposioProvider("test-key")

	_, ok := p.GetTool("nonexistent")
	if ok {
		t.Error("expected false for nonexistent tool")
	}
}

// --- ComposioProvider ExecuteTool Tests ---

func TestComposioProviderExecuteToolNotFound(t *testing.T) {
	t.Parallel()
	p := NewComposioProvider("test-key")

	result, err := p.ExecuteTool(context.Background(), "nonexistent", nil)
	if err == nil {
		t.Error("expected error for nonexistent tool")
	}
	if result != nil {
		t.Error("expected nil result for nonexistent tool")
	}
}

func TestComposioProviderExecuteToolNoAuth(t *testing.T) {
	t.Parallel()
	p := NewComposioProvider("test-key")

	p.RegisterTool(&ComposioTool{
		Name:         "protected_tool",
		Description:  "A tool requiring auth",
		AuthRequired: true,
		Category:     "github",
	})

	result, err := p.ExecuteTool(context.Background(), "protected_tool", nil)
	if err != nil {
		t.Fatalf("ExecuteTool returned error: %v", err)
	}
	if result.Success {
		t.Error("expected success=false for tool without credentials")
	}
	if result.Error == "" {
		t.Error("expected error message for tool without credentials")
	}
}

func TestComposioProviderExecuteToolWithAuth(t *testing.T) {
	t.Parallel()
	p := NewComposioProvider("test-key")

	// Store a credential
	p.Credentials().Store(&Credential{
		ID:          "cred-1",
		ServiceName: "github",
		Type:        "oauth",
		Value:       "token123",
	})

	p.RegisterTool(&ComposioTool{
		Name:         "github_tool",
		Description:  "GitHub tool",
		AuthRequired: true,
		Category:     "github",
	})

	result, err := p.ExecuteTool(context.Background(), "github_tool", json.RawMessage(`{"repo": "test"}`))
	if err != nil {
		t.Fatalf("ExecuteTool returned error: %v", err)
	}
	if !result.Success {
		t.Errorf("expected success=true, got false: %s", result.Error)
	}
}

func TestComposioProviderExecuteToolNoAuthRequired(t *testing.T) {
	t.Parallel()
	p := NewComposioProvider("test-key")

	p.RegisterTool(&ComposioTool{
		Name:         "public_tool",
		Description:  "A public tool",
		AuthRequired: false,
		Category:     "public",
	})

	result, err := p.ExecuteTool(context.Background(), "public_tool", nil)
	if err != nil {
		t.Fatalf("ExecuteTool returned error: %v", err)
	}
	if !result.Success {
		t.Errorf("expected success=true, got false: %s", result.Error)
	}
}

// --- Credential Tests ---

func TestCredentialIsExpired(t *testing.T) {
	t.Parallel()

	c := &Credential{ID: "1", Value: "val"}
	if c.IsExpired() {
		t.Error("expected non-expired credential to not be expired")
	}

	c.ExpiresAt = time.Now().Add(-1 * time.Hour)
	if !c.IsExpired() {
		t.Error("expected expired credential to be expired")
	}
}

func TestCredentialManagerStoreAndGet(t *testing.T) {
	t.Parallel()
	cm := NewCredentialManager()

	c := &Credential{
		ID:          "cred-1",
		ServiceName: "github",
		Type:        "oauth",
		Value:       "token123",
	}
	cm.Store(c)

	if cm.Count() != 1 {
		t.Errorf("expected 1 credential, got %d", cm.Count())
	}

	retrieved := cm.Get("cred-1")
	if retrieved == nil {
		t.Fatal("expected to find credential")
	}
	if retrieved.Value != "token123" {
		t.Errorf("expected value 'token123', got %q", retrieved.Value)
	}
}

func TestCredentialManagerGetForService(t *testing.T) {
	t.Parallel()
	cm := NewCredentialManager()

	cm.Store(&Credential{
		ID:          "cred-1",
		ServiceName: "github",
		Type:        "oauth",
		Value:       "ghp_xxx",
	})
	cm.Store(&Credential{
		ID:          "cred-2",
		ServiceName: "slack",
		Type:        "oauth",
		Value:       "xoxb-yyy",
	})

	retrieved := cm.GetForService("github")
	if retrieved == nil {
		t.Fatal("expected to find github credential")
	}
	if retrieved.Value != "ghp_xxx" {
		t.Errorf("expected value 'ghp_xxx', got %q", retrieved.Value)
	}

	retrieved = cm.GetForService("nonexistent")
	if retrieved != nil {
		t.Error("expected nil for nonexistent service")
	}
}

func TestCredentialManagerList(t *testing.T) {
	t.Parallel()
	cm := NewCredentialManager()

	cm.Store(&Credential{ID: "cred-1", ServiceName: "github"})
	cm.Store(&Credential{ID: "cred-2", ServiceName: "slack"})

	list := cm.List()
	if len(list) != 2 {
		t.Errorf("expected 2 credentials, got %d", len(list))
	}
}

func TestCredentialManagerDelete(t *testing.T) {
	t.Parallel()
	cm := NewCredentialManager()

	cm.Store(&Credential{ID: "cred-1", ServiceName: "github"})

	if !cm.Delete("cred-1") {
		t.Error("expected Delete to return true")
	}
	if cm.Count() != 0 {
		t.Errorf("expected 0 credentials after delete, got %d", cm.Count())
	}

	if cm.Delete("nonexistent") {
		t.Error("expected Delete to return false for nonexistent")
	}
}

func TestCredentialManagerGetExpired(t *testing.T) {
	t.Parallel()
	cm := NewCredentialManager()

	cm.Store(&Credential{
		ID:          "cred-1",
		ServiceName: "github",
		Value:       "token",
		ExpiresAt:   time.Now().Add(-1 * time.Hour),
	})

	retrieved := cm.Get("cred-1")
	if retrieved != nil {
		t.Error("expected nil for expired credential")
	}
}

func TestCredentialManagerGetForServiceExpired(t *testing.T) {
	t.Parallel()
	cm := NewCredentialManager()

	cm.Store(&Credential{
		ID:          "cred-1",
		ServiceName: "github",
		Value:       "token",
		ExpiresAt:   time.Now().Add(-1 * time.Hour),
	})

	retrieved := cm.GetForService("github")
	if retrieved != nil {
		t.Error("expected nil for expired service credential")
	}
}

// --- Helper function tests ---

func TestLower(t *testing.T) {
	t.Parallel()
	if lower("HELLO") != "hello" {
		t.Error("expected 'hello'")
	}
	if lower("Hello World") != "hello world" {
		t.Error("expected 'hello world'")
	}
	if lower("") != "" {
		t.Error("expected empty string")
	}
}

func TestContains(t *testing.T) {
	t.Parallel()
	if !contains("Hello World", "world") {
		t.Error("expected 'Hello World' to contain 'world' (case-insensitive)")
	}
	if contains("Hello World", "xyz") {
		t.Error("expected 'Hello World' to not contain 'xyz'")
	}
	if !contains("test", "") {
		t.Error("expected any string to contain empty string")
	}
}

func TestMatchesSearch(t *testing.T) {
	t.Parallel()
	tool := &ComposioTool{
		Name:        "github_issues",
		Description: "List GitHub issues",
		Tags:        []string{"github", "issues"},
	}

	if !matchesSearch(tool, "github") {
		t.Error("expected match for 'github' in name")
	}
	if !matchesSearch(tool, "issues") {
		t.Error("expected match for 'issues' in description")
	}
	if !matchesSearch(tool, "LIST") {
		t.Error("expected case-insensitive match for 'LIST'")
	}
	if matchesSearch(tool, "nonexistent") {
		t.Error("expected no match for 'nonexistent'")
	}
}
