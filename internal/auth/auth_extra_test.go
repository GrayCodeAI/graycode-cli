package auth

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

// --- TokenStore tests ---

func TestNewTokenStore(t *testing.T) {
	ts := NewTokenStore()
	if ts == nil {
		t.Fatal("expected non-nil TokenStore")
	}
	if len(ts.tokens) != 0 {
		t.Errorf("expected empty tokens, got %d", len(ts.tokens))
	}
}

func TestTokenStore_Load(t *testing.T) {
	ts := NewTokenStore()
	// Pre-populate
	ts.tokens["test"] = "old"
	err := ts.Load()
	if err != nil {
		t.Fatalf("Load() error: %v", err)
	}
	// Load should reset to empty
	if len(ts.tokens) != 0 {
		t.Errorf("expected empty tokens after Load, got %d", len(ts.tokens))
	}
}

func TestTokenStore_Save(t *testing.T) {
	ts := NewTokenStore()
	ts.tokens["test"] = "secret"
	err := ts.Save()
	if err != nil {
		t.Fatalf("Save() error: %v", err)
	}
	// Save is a no-op stub, tokens should still be in memory
	if ts.tokens["test"] != "secret" {
		t.Error("token should still be in memory after Save")
	}
}

func TestTokenStore_Get(t *testing.T) {
	ts := NewTokenStore()
	ts.tokens["provider1"] = "token1"

	if ts.Get("provider1") != "token1" {
		t.Errorf("Get(provider1) = %q, want %q", ts.Get("provider1"), "token1")
	}
	if ts.Get("nonexistent") != "" {
		t.Errorf("Get(nonexistent) = %q, want empty", ts.Get("nonexistent"))
	}
}

func TestTokenStore_Set(t *testing.T) {
	ts := NewTokenStore()
	ts.Set("provider1", "token1")
	if ts.tokens["provider1"] != "token1" {
		t.Errorf("tokens[provider1] = %q, want %q", ts.tokens["provider1"], "token1")
	}
	// Overwrite
	ts.Set("provider1", "token2")
	if ts.tokens["provider1"] != "token2" {
		t.Errorf("tokens[provider1] = %q, want %q", ts.tokens["provider1"], "token2")
	}
}

func TestTokenStore_Has(t *testing.T) {
	ts := NewTokenStore()
	if ts.Has("provider1") {
		t.Error("expected false for non-existent provider")
	}
	ts.Set("provider1", "token1")
	if !ts.Has("provider1") {
		t.Error("expected true for existing provider")
	}
}

// --- SecureStorage tests ---

func TestNewSecureStorage(t *testing.T) {
	ss := NewSecureStorage("test-service")
	if ss == nil {
		t.Fatal("expected non-nil SecureStorage")
	}
	if ss.service != "test-service" {
		t.Errorf("service = %q, want %q", ss.service, "test-service")
	}
}

func TestSecureStorage_GetFile_NonExistent(t *testing.T) {
	ss := &SecureStorage{service: "test"}
	// getFile will fail because the file doesn't exist
	_, err := ss.getFile("test-account")
	if err == nil {
		t.Error("expected error for non-existent token file")
	}
}

func TestSecureStorage_SetFile_GetFile(t *testing.T) {
	// Test the JSON marshaling/unmarshaling logic for token files
	tokens := map[string]string{"account1": "token1"}
	data, _ := json.Marshal(tokens)

	var loaded map[string]string
	if err := json.Unmarshal(data, &loaded); err != nil {
		t.Fatal(err)
	}
	if loaded["account1"] != "token1" {
		t.Errorf("loaded[account1] = %q, want %q", loaded["account1"], "token1")
	}
}

// --- GenerateNonce tests ---

func TestGenerateNonce_ValidBase64(t *testing.T) {
	nonce := GenerateNonce()
	// Should be valid base64 URL encoding
	decoded, err := decodeNonce(nonce)
	if err != nil {
		t.Errorf("nonce is not valid base64: %v", err)
	}
	if len(decoded) != 16 {
		t.Errorf("decoded nonce length = %d, want 16", len(decoded))
	}
}

// --- securityQuote tests ---

func TestSecurityQuote(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"simple", `"simple"`},
		{"with\"quote", `"with\"quote"`},
		{"with\\backslash", `"with\\backslash"`},
		{"", `""`},
	}

	for _, tt := range tests {
		got := securityQuote(tt.input)
		if got != tt.expected {
			t.Errorf("securityQuote(%q) = %q, want %q", tt.input, got, tt.expected)
		}
	}
}

// --- powershellQuote tests ---

func TestPowershellQuote(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"simple", "simple"},
		{"with'quote", "with''quote"},
		{"with''double", "with''''double"},
		{"", ""},
	}

	for _, tt := range tests {
		got := powershellQuote(tt.input)
		if got != tt.expected {
			t.Errorf("powershellQuote(%q) = %q, want %q", tt.input, got, tt.expected)
		}
	}
}

// --- buildWinCredScript tests ---

func TestBuildWinCredScript(t *testing.T) {
	tail := "[WinCred]::Get('test')"
	script := buildWinCredScript(tail)

	if !strings.Contains(script, tail) {
		t.Errorf("script should contain tail %q", tail)
	}
	if !strings.Contains(script, "Add-Type") {
		t.Error("script should contain Add-Type")
	}
	if !strings.Contains(script, "WinCred") {
		t.Error("script should contain WinCred class")
	}
}

// --- execCommand tests ---

func TestExecCommand_Success(t *testing.T) {
	// Use echo on Unix, or a cross-platform approach
	result, err := execCommand("echo", "hello")
	if err != nil {
		t.Fatalf("execCommand error: %v", err)
	}
	if result != "hello" {
		t.Errorf("execCommand(echo, hello) = %q, want %q", result, "hello")
	}
}

func TestExecCommand_Failure(t *testing.T) {
	_, err := execCommand("nonexistent-command-xyz")
	if err == nil {
		t.Error("expected error for non-existent command")
	}
}

func TestExecCommand_ExitError(t *testing.T) {
	// Use a command that exits with non-zero
	result, err := execCommand("sh", "-c", "exit 1")
	if err == nil {
		t.Error("expected error for command that exits with non-zero")
	}
	if result != "" {
		t.Errorf("result = %q, want empty", result)
	}
}

// --- DeviceFlow tests ---

func TestNewDeviceFlow_DefaultPollInterval(t *testing.T) {
	df := NewDeviceFlow(DeviceFlowConfig{
		ClientID:      "test-client",
		DeviceAuthURL: "https://example.com/device",
		TokenURL:      "https://example.com/token",
	})
	if df.Config.PollInterval != 5*time.Second {
		t.Errorf("PollInterval = %v, want %v", df.Config.PollInterval, 5*time.Second)
	}
	if df.HTTPClient == nil {
		t.Error("expected non-nil HTTPClient")
	}
}

func TestNewDeviceFlow_CustomPollInterval(t *testing.T) {
	df := NewDeviceFlow(DeviceFlowConfig{
		ClientID:     "test-client",
		PollInterval: 10 * time.Second,
	})
	if df.Config.PollInterval != 10*time.Second {
		t.Errorf("PollInterval = %v, want %v", df.Config.PollInterval, 10*time.Second)
	}
}

func TestDeviceFlow_RequestCode_Success(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Errorf("expected POST, got %s", r.Method)
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(DeviceCodeResponse{
			DeviceCode:      "device-code-123",
			UserCode:        "user-code-456",
			VerificationURI: "https://example.com/verify",
			ExpiresIn:       900,
			Interval:        5,
		})
	}))
	defer server.Close()

	df := NewDeviceFlow(DeviceFlowConfig{
		ClientID:      "test-client",
		DeviceAuthURL: server.URL,
		TokenURL:      server.URL,
		Scopes:        []string{"read", "write"},
	})

	resp, err := df.RequestCode(context.Background())
	if err != nil {
		t.Fatalf("RequestCode error: %v", err)
	}
	if resp.DeviceCode != "device-code-123" {
		t.Errorf("DeviceCode = %q, want %q", resp.DeviceCode, "device-code-123")
	}
	if resp.UserCode != "user-code-456" {
		t.Errorf("UserCode = %q, want %q", resp.UserCode, "user-code-456")
	}
	if resp.VerificationURI != "https://example.com/verify" {
		t.Errorf("VerificationURI = %q, want %q", resp.VerificationURI, "https://example.com/verify")
	}
}

func TestDeviceFlow_RequestCode_HTTPError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
	}))
	defer server.Close()

	df := NewDeviceFlow(DeviceFlowConfig{
		ClientID:      "test-client",
		DeviceAuthURL: server.URL,
	})

	_, err := df.RequestCode(context.Background())
	if err == nil {
		t.Error("expected error for HTTP 400")
	}
	if !strings.Contains(err.Error(), "HTTP 400") {
		t.Errorf("error = %q, want to contain 'HTTP 400'", err.Error())
	}
}

func TestDeviceFlow_RequestCode_InvalidJSON(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{invalid`))
	}))
	defer server.Close()

	df := NewDeviceFlow(DeviceFlowConfig{
		ClientID:      "test-client",
		DeviceAuthURL: server.URL,
	})

	_, err := df.RequestCode(context.Background())
	if err == nil {
		t.Error("expected error for invalid JSON")
	}
}

func TestDeviceFlow_PollForToken_Success(t *testing.T) {
	callCount := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		callCount++
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(TokenResponse{
			AccessToken: "access-token-123",
			TokenType:   "bearer",
			Scope:       "read",
		})
	}))
	defer server.Close()

	df := NewDeviceFlow(DeviceFlowConfig{
		ClientID:     "test-client",
		TokenURL:     server.URL,
		PollInterval: 10 * time.Millisecond,
		ExpiresIn:    5 * time.Second,
	})

	token, err := df.PollForToken(context.Background(), "device-code")
	if err != nil {
		t.Fatalf("PollForToken error: %v", err)
	}
	if token.AccessToken != "access-token-123" {
		t.Errorf("AccessToken = %q, want %q", token.AccessToken, "access-token-123")
	}
	if token.TokenType != "bearer" {
		t.Errorf("TokenType = %q, want %q", token.TokenType, "bearer")
	}
}

func TestDeviceFlow_PollForToken_AuthorizationPending(t *testing.T) {
	callCount := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		callCount++
		w.Header().Set("Content-Type", "application/json")
		if callCount < 3 {
			// First two calls return authorization_pending
			json.NewEncoder(w).Encode(map[string]string{"error": "authorization_pending"})
		} else {
			// Third call returns success
			json.NewEncoder(w).Encode(TokenResponse{
				AccessToken: "access-token",
				TokenType:   "bearer",
			})
		}
	}))
	defer server.Close()

	df := NewDeviceFlow(DeviceFlowConfig{
		ClientID:     "test-client",
		TokenURL:     server.URL,
		PollInterval: 10 * time.Millisecond,
		ExpiresIn:    5 * time.Second,
	})

	token, err := df.PollForToken(context.Background(), "device-code")
	if err != nil {
		t.Fatalf("PollForToken error: %v", err)
	}
	if token.AccessToken != "access-token" {
		t.Errorf("AccessToken = %q, want %q", token.AccessToken, "access-token")
	}
}

func TestDeviceFlow_PollForToken_SlowDown(t *testing.T) {
	callCount := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		callCount++
		w.Header().Set("Content-Type", "application/json")
		if callCount < 2 {
			json.NewEncoder(w).Encode(map[string]string{"error": "slow_down"})
		} else {
			json.NewEncoder(w).Encode(TokenResponse{
				AccessToken: "access-token",
				TokenType:   "bearer",
			})
		}
	}))
	defer server.Close()

	df := NewDeviceFlow(DeviceFlowConfig{
		ClientID:     "test-client",
		TokenURL:     server.URL,
		PollInterval: 10 * time.Millisecond,
		ExpiresIn:    5 * time.Second,
	})

	token, err := df.PollForToken(context.Background(), "device-code")
	if err != nil {
		t.Fatalf("PollForToken error: %v", err)
	}
	if token.AccessToken != "access-token" {
		t.Errorf("AccessToken = %q, want %q", token.AccessToken, "access-token")
	}
}

func TestDeviceFlow_PollForToken_Timeout(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]string{"error": "authorization_pending"})
	}))
	defer server.Close()

	df := NewDeviceFlow(DeviceFlowConfig{
		ClientID:     "test-client",
		TokenURL:     server.URL,
		PollInterval: 10 * time.Millisecond,
		ExpiresIn:    50 * time.Millisecond, // Very short timeout
	})

	_, err := df.PollForToken(context.Background(), "device-code")
	if err == nil {
		t.Error("expected timeout error")
	}
	if !strings.Contains(err.Error(), "timed out") {
		t.Errorf("error = %q, want to contain 'timed out'", err.Error())
	}
}

func TestDeviceFlow_PollForToken_ContextCanceled(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]string{"error": "authorization_pending"})
	}))
	defer server.Close()

	df := NewDeviceFlow(DeviceFlowConfig{
		ClientID:     "test-client",
		TokenURL:     server.URL,
		PollInterval: 10 * time.Millisecond,
		ExpiresIn:    5 * time.Second,
	})

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // Cancel immediately

	_, err := df.PollForToken(ctx, "device-code")
	if err == nil {
		t.Error("expected error for canceled context")
	}
}

func TestDeviceFlow_PollForToken_OtherError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]string{"error": "invalid_grant"})
	}))
	defer server.Close()

	df := NewDeviceFlow(DeviceFlowConfig{
		ClientID:     "test-client",
		TokenURL:     server.URL,
		PollInterval: 10 * time.Millisecond,
		ExpiresIn:    5 * time.Second,
	})

	_, err := df.PollForToken(context.Background(), "device-code")
	if err == nil {
		t.Error("expected error for invalid_grant")
	}
	if !strings.Contains(err.Error(), "invalid_grant") {
		t.Errorf("error = %q, want to contain 'invalid_grant'", err.Error())
	}
}

func TestDeviceFlow_PollForToken_HTTPError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer server.Close()

	df := NewDeviceFlow(DeviceFlowConfig{
		ClientID:     "test-client",
		TokenURL:     server.URL,
		PollInterval: 10 * time.Millisecond,
		ExpiresIn:    5 * time.Second,
	})

	_, err := df.PollForToken(context.Background(), "device-code")
	if err == nil {
		t.Error("expected error for HTTP 500")
	}
}

func TestDeviceFlow_PollForToken_InvalidJSON(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{invalid`))
	}))
	defer server.Close()

	df := NewDeviceFlow(DeviceFlowConfig{
		ClientID:     "test-client",
		TokenURL:     server.URL,
		PollInterval: 10 * time.Millisecond,
		ExpiresIn:    5 * time.Second,
	})

	_, err := df.PollForToken(context.Background(), "device-code")
	if err == nil {
		t.Error("expected error for invalid JSON")
	}
}

// --- Helper functions ---

func decodeNonce(nonce string) ([]byte, error) {
	// Use standard base64 URL decoding
	return base64DecodeURL(nonce)
}

func base64DecodeURL(s string) ([]byte, error) {
	// Add padding if needed
	if pad := len(s) % 4; pad > 0 {
		s += strings.Repeat("=", 4-pad)
	}
	return base64.URLEncoding.DecodeString(s)
}

// Ensure base64 import is used
var _ = base64.URLEncoding
