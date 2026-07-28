package tool

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestDownloadTool_Name(t *testing.T) {
	dt := DownloadTool{}
	if dt.Name() != "Download" {
		t.Errorf("Name() = %q, want %q", dt.Name(), "Download")
	}
}

func TestDownloadTool_Description(t *testing.T) {
	dt := DownloadTool{}
	if dt.Description() == "" {
		t.Error("Description() should not be empty")
	}
}

func TestDownloadTool_Parameters(t *testing.T) {
	dt := DownloadTool{}
	params := dt.Parameters()
	if params == nil {
		t.Fatal("Parameters() returned nil")
	}
	if params["type"] != "object" {
		t.Errorf("type = %v, want object", params["type"])
	}
	props, ok := params["properties"].(map[string]interface{})
	if !ok {
		t.Fatal("properties is not a map")
	}
	if _, ok := props["url"]; !ok {
		t.Error("missing 'url' property")
	}
	if _, ok := props["destination"]; !ok {
		t.Error("missing 'destination' property")
	}
}

func TestDownloadTool_Aliases(t *testing.T) {
	dt := DownloadTool{}
	aliases := dt.Aliases()
	if len(aliases) == 0 {
		t.Error("Aliases() should not be empty")
	}
	found := false
	for _, a := range aliases {
		if a == "download" {
			found = true
		}
	}
	if !found {
		t.Error("aliases should contain 'download'")
	}
}

func TestDownloadTool_RiskLevel(t *testing.T) {
	dt := DownloadTool{}
	if dt.RiskLevel() != "medium" {
		t.Errorf("RiskLevel() = %q, want %q", dt.RiskLevel(), "medium")
	}
}

func TestDownloadTool_Execute_InvalidJSON(t *testing.T) {
	dt := DownloadTool{}
	ctx := WithSSRFSkip(context.Background())
	_, err := dt.Execute(ctx, []byte("not json"))
	if err == nil {
		t.Error("expected error for invalid JSON")
	}
}

func TestDownloadTool_Execute_MissingURL(t *testing.T) {
	dt := DownloadTool{}
	ctx := WithSSRFSkip(context.Background())
	input, _ := json.Marshal(map[string]string{
		"destination": "/tmp/test.txt",
	})
	_, err := dt.Execute(ctx, input)
	if err == nil {
		t.Error("expected error for missing URL")
	}
	if err.Error() != "url and destination are required" {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestDownloadTool_Execute_MissingDestination(t *testing.T) {
	dt := DownloadTool{}
	ctx := WithSSRFSkip(context.Background())
	input, _ := json.Marshal(map[string]string{
		"url": "http://example.com/file",
	})
	_, err := dt.Execute(ctx, input)
	if err == nil {
		t.Error("expected error for missing destination")
	}
}

func TestDownloadTool_Execute_BothEmpty(t *testing.T) {
	dt := DownloadTool{}
	ctx := WithSSRFSkip(context.Background())
	input, _ := json.Marshal(map[string]string{
		"url":         "",
		"destination": "",
	})
	_, err := dt.Execute(ctx, input)
	if err == nil {
		t.Error("expected error for empty url and destination")
	}
}

func TestDownloadTool_Execute_Success(t *testing.T) {
	// Set up a test HTTP server
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/plain")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("hello world"))
	}))
	defer server.Close()

	tmpDir := t.TempDir()
	dest := tmpDir + "/downloaded.txt"

	dt := DownloadTool{}
	ctx := WithSSRFSkip(context.Background())
	input, _ := json.Marshal(map[string]string{
		"url":         server.URL + "/file.txt",
		"destination": dest,
	})
	result, err := dt.Execute(ctx, input)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result == "" {
		t.Error("expected non-empty result")
	}
}

func TestDownloadTool_Execute_HTTPError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		w.Write([]byte("not found"))
	}))
	defer server.Close()

	tmpDir := t.TempDir()
	dest := tmpDir + "/downloaded.txt"

	dt := DownloadTool{}
	ctx := WithSSRFSkip(context.Background())
	input, _ := json.Marshal(map[string]string{
		"url":         server.URL + "/missing.txt",
		"destination": dest,
	})
	_, err := dt.Execute(ctx, input)
	if err == nil {
		t.Error("expected error for HTTP 404")
	}
}

func TestDownloadTool_Execute_CredentialContent(t *testing.T) {
	// Server returns content that looks like credentials
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/plain")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("password=sk-abc0123456789012345wxyz secret key"))
	}))
	defer server.Close()

	tmpDir := t.TempDir()
	dest := tmpDir + "/creds.txt"

	dt := DownloadTool{}
	ctx := WithSSRFSkip(context.Background())
	input, _ := json.Marshal(map[string]string{
		"url":         server.URL + "/config.txt",
		"destination": dest,
	})
	_, err := dt.Execute(ctx, input)
	if err == nil {
		t.Error("expected error for credential content")
	}
}

func TestDownloadTool_Execute_EmptyBody(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/octet-stream")
		w.WriteHeader(http.StatusOK)
		// Empty body
	}))
	defer server.Close()

	tmpDir := t.TempDir()
	dest := tmpDir + "/empty.txt"

	dt := DownloadTool{}
	ctx := WithSSRFSkip(context.Background())
	input, _ := json.Marshal(map[string]string{
		"url":         server.URL + "/empty",
		"destination": dest,
	})
	result, err := dt.Execute(ctx, input)
	if err != nil {
		t.Fatalf("unexpected error for empty body: %v", err)
	}
	if result == "" {
		t.Error("expected non-empty result message")
	}
}

func TestDownloadTool_Execute_BlockedScheme(t *testing.T) {
	dt := DownloadTool{}
	ctx := WithSSRFSkip(context.Background())
	input, _ := json.Marshal(map[string]string{
		"url":         "ftp://example.com/file",
		"destination": "/tmp/test.txt",
	})
	_, _ = dt.Execute(ctx, input)
	// With SSRF skip, the URL validation is skipped, so the error will come from the HTTP client
	// Without SSRF skip, it would be blocked. Since we use WithSSRFSkip, the error might be different.
	// Let's test without SSRF skip
	ctx2 := context.Background()
	_, err := dt.Execute(ctx2, input)
	if err == nil {
		t.Error("expected error for ftp:// URL without SSRF skip")
	}
}

func TestReadDownloadBodyRejectsOverflow(t *testing.T) {
	body, err := readDownloadBody(strings.NewReader("123456"), 5)
	if err == nil {
		t.Fatalf("readDownloadBody returned %q without an overflow error", body)
	}
}

func TestReadDownloadBodyAllowsExactLimit(t *testing.T) {
	body, err := readDownloadBody(strings.NewReader("12345"), 5)
	if err != nil {
		t.Fatalf("readDownloadBody returned error: %v", err)
	}
	if string(body) != "12345" {
		t.Fatalf("readDownloadBody = %q, want %q", body, "12345")
	}
}
