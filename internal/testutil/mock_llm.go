package testutil

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
)

// MockLLMServer provides a configurable mock LLM API server for testing.
type MockLLMServer struct {
	Server    *httptest.Server
	Responses []MockResponse
	Requests  []MockRequest
	mu        sync.Mutex
	idx       int
}

// MockResponse defines a canned response from the mock LLM.
type MockResponse struct {
	Content    string
	ToolUse    []ToolUseBlock
	StopReason string
	StatusCode int
}

// ToolUseBlock represents a tool call in the response.
type ToolUseBlock struct {
	ID    string
	Name  string
	Input map[string]interface{}
}

// MockRequest records a request made to the mock server.
type MockRequest struct {
	Method string
	Path   string
	Body   map[string]interface{}
}

// NewMockLLMServer creates a mock server that returns canned responses in order.
func NewMockLLMServer(t *testing.T, responses ...MockResponse) *MockLLMServer {
	t.Helper()
	m := &MockLLMServer{Responses: responses}
	m.Server = httptest.NewServer(http.HandlerFunc(m.handler))
	t.Cleanup(m.Server.Close)
	return m
}

func (m *MockLLMServer) handler(w http.ResponseWriter, r *http.Request) {
	m.mu.Lock()
	defer m.mu.Unlock()

	var body map[string]interface{}
	if r.Body != nil {
		_ = json.NewDecoder(r.Body).Decode(&body)
	}
	m.Requests = append(m.Requests, MockRequest{
		Method: r.Method,
		Path:   r.URL.Path,
		Body:   body,
	})

	if m.idx >= len(m.Responses) {
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = fmt.Fprintf(w, `{"error":"no more mock responses"}`)
		return
	}

	resp := m.Responses[m.idx]
	m.idx++

	if resp.StatusCode != 0 && resp.StatusCode != 200 {
		w.WriteHeader(resp.StatusCode)
		_, _ = fmt.Fprintf(w, `{"error":{"type":"error","message":"mock error"}}`)
		return
	}

	w.Header().Set("Content-Type", "application/json")

	// Build Anthropic-style response
	content := []map[string]interface{}{}
	if resp.Content != "" {
		content = append(content, map[string]interface{}{
			"type": "text",
			"text": resp.Content,
		})
	}
	for _, tu := range resp.ToolUse {
		content = append(content, map[string]interface{}{
			"type":  "tool_use",
			"id":    tu.ID,
			"name":  tu.Name,
			"input": tu.Input,
		})
	}

	stopReason := resp.StopReason
	if stopReason == "" {
		stopReason = "end_turn"
	}

	result := map[string]interface{}{
		"id":           fmt.Sprintf("msg_%d", m.idx),
		"type":         "message",
		"role":         "assistant",
		"content":      content,
		"model":        "mock-model",
		"stop_reason":  stopReason,
		"usage":        map[string]int{"input_tokens": 100, "output_tokens": 50},
	}

	_ = json.NewEncoder(w).Encode(result)
}

// URL returns the base URL of the mock server.
func (m *MockLLMServer) URL() string {
	return m.Server.URL
}

// RequestCount returns how many requests were made.
func (m *MockLLMServer) RequestCount() int {
	m.mu.Lock()
	defer m.mu.Unlock()
	return len(m.Requests)
}

// LastRequest returns the most recent request body.
func (m *MockLLMServer) LastRequest() MockRequest {
	m.mu.Lock()
	defer m.mu.Unlock()
	if len(m.Requests) == 0 {
		return MockRequest{}
	}
	return m.Requests[len(m.Requests)-1]
}

// SimpleTextResponse creates a mock response with just text content.
func SimpleTextResponse(text string) MockResponse {
	return MockResponse{Content: text}
}

// ToolUseResponse creates a mock response that calls a tool.
func ToolUseResponse(toolName string, input map[string]interface{}) MockResponse {
	return MockResponse{
		ToolUse: []ToolUseBlock{{
			ID:    "toolu_mock_1",
			Name:  toolName,
			Input: input,
		}},
		StopReason: "tool_use",
	}
}

// ErrorResponse creates a mock error response.
func ErrorResponse(statusCode int) MockResponse {
	return MockResponse{StatusCode: statusCode}
}

// ContainsString checks if any request body contains the given string.
func (m *MockLLMServer) ContainsString(s string) bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	for _, req := range m.Requests {
		data, _ := json.Marshal(req.Body)
		if strings.Contains(string(data), s) {
			return true
		}
	}
	return false
}
