package api

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestHealthEndpoint(t *testing.T) {
	srv := New(":0")
	req := httptest.NewRequest(http.MethodGet, "/health", nil)
	w := httptest.NewRecorder()

	srv.Handler().ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}

	var resp HealthResponse
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if resp.Status != "ok" {
		t.Fatalf("expected status 'ok', got %q", resp.Status)
	}
}

func TestVersionEndpoint(t *testing.T) {
	srv := New(":0")
	req := httptest.NewRequest(http.MethodGet, "/version", nil)
	w := httptest.NewRecorder()

	srv.Handler().ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}

	var resp VersionResponse
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if resp.Version != Version {
		t.Fatalf("expected version %q, got %q", Version, resp.Version)
	}
}

func TestChatEndpoint_Success(t *testing.T) {
	srv := New(":0")

	body := ChatRequest{Message: "hello", Model: "gpt-4"}
	b, _ := json.Marshal(body)
	req := httptest.NewRequest(http.MethodPost, "/chat", bytes.NewReader(b))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	srv.Handler().ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}

	var resp ChatResponse
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if resp.Response == "" {
		t.Fatal("expected non-empty response")
	}
}

func TestChatEndpoint_EmptyMessage(t *testing.T) {
	srv := New(":0")

	body := ChatRequest{Message: "", Model: "gpt-4"}
	b, _ := json.Marshal(body)
	req := httptest.NewRequest(http.MethodPost, "/chat", bytes.NewReader(b))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	srv.Handler().ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", w.Code)
	}
}

func TestChatEndpoint_InvalidJSON(t *testing.T) {
	srv := New(":0")

	req := httptest.NewRequest(http.MethodPost, "/chat", bytes.NewReader([]byte("not json")))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	srv.Handler().ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", w.Code)
	}
}

func TestHealthEndpoint_ContentType(t *testing.T) {
	srv := New(":0")
	req := httptest.NewRequest(http.MethodGet, "/health", nil)
	w := httptest.NewRecorder()

	srv.Handler().ServeHTTP(w, req)

	ct := w.Header().Get("Content-Type")
	if ct != "application/json" {
		t.Fatalf("expected Content-Type 'application/json', got %q", ct)
	}
}

func TestWriteJSON(t *testing.T) {
	w := httptest.NewRecorder()
	data := map[string]string{"key": "value"}
	writeJSON(w, http.StatusCreated, data)

	if w.Code != http.StatusCreated {
		t.Errorf("status = %d, want 201", w.Code)
	}
	if w.Header().Get("Content-Type") != "application/json" {
		t.Error("Content-Type should be application/json")
	}
	var result map[string]string
	if err := json.NewDecoder(w.Body).Decode(&result); err != nil {
		t.Fatalf("decode error: %v", err)
	}
	if result["key"] != "value" {
		t.Errorf("key = %q, want 'value'", result["key"])
	}
}

func TestNew(t *testing.T) {
	srv := New(":8080")
	if srv == nil {
		t.Fatal("New returned nil")
	}
	if srv.Handler() == nil {
		t.Error("Handler() should not be nil")
	}
}

func TestChatEndpoint_MethodNotAllowed(t *testing.T) {
	srv := New(":0")
	req := httptest.NewRequest(http.MethodGet, "/chat", nil)
	w := httptest.NewRecorder()

	srv.Handler().ServeHTTP(w, req)

	// GET on /chat should return error (405 or similar)
	if w.Code == http.StatusOK {
		t.Error("GET /chat should not return 200")
	}
}

func TestUnknownEndpoint(t *testing.T) {
	srv := New(":0")
	req := httptest.NewRequest(http.MethodGet, "/unknown-path", nil)
	w := httptest.NewRecorder()

	srv.Handler().ServeHTTP(w, req)

	if w.Code == http.StatusOK {
		t.Error("unknown path should not return 200")
	}
}
