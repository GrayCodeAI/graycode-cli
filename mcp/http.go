package mcp

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"
	"time"
)

// HTTPServer represents an MCP server connected via HTTP or SSE transport.
type HTTPServer struct {
	Name    string
	URL     string
	Headers map[string]string
	Type    string // "http" or "sse"
	client  *http.Client
	mu      sync.Mutex
	nextID  int
}

// ConnectHTTP connects to an MCP server via HTTP streamable transport.
func ConnectHTTP(ctx context.Context, name, url string, headers map[string]string) (*HTTPServer, error) {
	s := &HTTPServer{
		Name:    name,
		URL:     url,
		Headers: headers,
		Type:    "http",
		client:  &http.Client{Timeout: 60 * time.Second},
	}
	// Initialize
	_, err := s.Call(ctx, "initialize", map[string]interface{}{
		"protocolVersion": "2024-11-05",
		"capabilities":    map[string]interface{}{},
		"clientInfo":      map[string]interface{}{"name": "hawk", "version": "1.0.0"},
	})
	if err != nil {
		return nil, fmt.Errorf("mcp http init: %w", err)
	}
	return s, nil
}

// ConnectSSE connects to an MCP server via Server-Sent Events transport.
func ConnectSSE(ctx context.Context, name, url string, headers map[string]string) (*HTTPServer, error) {
	s := &HTTPServer{
		Name:    name,
		URL:     url,
		Headers: headers,
		Type:    "sse",
		client:  &http.Client{Timeout: 120 * time.Second},
	}
	_, err := s.Call(ctx, "initialize", map[string]interface{}{
		"protocolVersion": "2024-11-05",
		"capabilities":    map[string]interface{}{},
		"clientInfo":      map[string]interface{}{"name": "hawk", "version": "1.0.0"},
	})
	if err != nil {
		return nil, fmt.Errorf("mcp sse init: %w", err)
	}
	return s, nil
}

// Call sends a JSON-RPC request and returns the result.
func (s *HTTPServer) Call(ctx context.Context, method string, params interface{}) (json.RawMessage, error) {
	s.mu.Lock()
	s.nextID++
	id := s.nextID
	s.mu.Unlock()

	req := jsonrpcRequest{JSONRPC: "2.0", ID: id, Method: method, Params: params}
	body, _ := json.Marshal(req)

	httpReq, err := http.NewRequestWithContext(ctx, "POST", s.URL, bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	httpReq.Header.Set("Content-Type", "application/json")
	for k, v := range s.Headers {
		httpReq.Header.Set(k, v)
	}
	if s.Type == "sse" {
		httpReq.Header.Set("Accept", "text/event-stream")
	}

	resp, err := s.client.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("mcp %s request: %w", s.Type, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		data, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("mcp %s: HTTP %d: %s", s.Type, resp.StatusCode, string(data))
	}

	// SSE: parse event stream for the result
	if s.Type == "sse" && strings.Contains(resp.Header.Get("Content-Type"), "text/event-stream") {
		return s.parseSSEResponse(resp.Body, id)
	}

	// HTTP: parse JSON-RPC response directly
	var rpcResp jsonrpcResponse
	if err := json.NewDecoder(resp.Body).Decode(&rpcResp); err != nil {
		return nil, fmt.Errorf("mcp %s decode: %w", s.Type, err)
	}
	if rpcResp.Error != nil {
		return nil, fmt.Errorf("mcp %s error %d: %s", s.Type, rpcResp.Error.Code, rpcResp.Error.Message)
	}
	return rpcResp.Result, nil
}

func (s *HTTPServer) parseSSEResponse(body io.Reader, id int) (json.RawMessage, error) {
	scanner := bufio.NewScanner(body)
	for scanner.Scan() {
		line := scanner.Text()
		if !strings.HasPrefix(line, "data: ") {
			continue
		}
		data := strings.TrimPrefix(line, "data: ")
		var rpcResp jsonrpcResponse
		if err := json.Unmarshal([]byte(data), &rpcResp); err != nil {
			continue
		}
		if rpcResp.ID == id {
			if rpcResp.Error != nil {
				return nil, fmt.Errorf("mcp sse error %d: %s", rpcResp.Error.Code, rpcResp.Error.Message)
			}
			return rpcResp.Result, nil
		}
	}
	return nil, fmt.Errorf("mcp sse: no response for request %d", id)
}

// ListTools returns tools from the HTTP/SSE MCP server.
func (s *HTTPServer) ListTools(ctx context.Context) ([]Tool, error) {
	result, err := s.Call(ctx, "tools/list", nil)
	if err != nil {
		return nil, err
	}
	var resp struct {
		Tools []Tool `json:"tools"`
	}
	if err := json.Unmarshal(result, &resp); err != nil {
		return nil, err
	}
	return resp.Tools, nil
}

// CallTool invokes a tool on the HTTP/SSE MCP server.
func (s *HTTPServer) CallTool(ctx context.Context, name string, args map[string]interface{}) (string, error) {
	result, err := s.Call(ctx, "tools/call", map[string]interface{}{
		"name":      name,
		"arguments": args,
	})
	if err != nil {
		return "", err
	}
	var resp struct {
		Content []struct {
			Type string `json:"type"`
			Text string `json:"text"`
		} `json:"content"`
	}
	if err := json.Unmarshal(result, &resp); err != nil {
		return string(result), nil
	}
	var texts []string
	for _, c := range resp.Content {
		if c.Text != "" {
			texts = append(texts, c.Text)
		}
	}
	return strings.Join(texts, "\n"), nil
}

// Close is a no-op for HTTP/SSE servers (no persistent connection).
func (s *HTTPServer) Close() error { return nil }
