// Package trace provides Langfuse tracing integration for LLM observability.
package trace

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"sync"
	"time"
)

// LangfuseClient sends traces to Langfuse for LLM observability.
type LangfuseClient struct {
	baseURL    string
	publicKey  string
	secretKey  string
	httpClient *http.Client
	mu         sync.Mutex
	batch      []event
	flushSize  int
}

type event struct {
	Type string      `json:"type"`
	Body interface{} `json:"body"`
}

// TraceEvent represents a single LLM call trace.
type TraceEvent struct {
	ID        string            `json:"id"`
	Name      string            `json:"name"`
	Input     string            `json:"input,omitempty"`
	Output    string            `json:"output,omitempty"`
	Model     string            `json:"model,omitempty"`
	StartTime time.Time         `json:"startTime"`
	EndTime   time.Time         `json:"endTime,omitempty"`
	Metadata  map[string]string `json:"metadata,omitempty"`
	Usage     *UsageInfo        `json:"usage,omitempty"`
}

// UsageInfo tracks token usage.
type UsageInfo struct {
	PromptTokens     int     `json:"promptTokens"`
	CompletionTokens int     `json:"completionTokens"`
	TotalTokens      int     `json:"totalTokens"`
	CostUSD          float64 `json:"costUSD,omitempty"`
}

// NewLangfuseClient creates a client from environment variables.
// Requires LANGFUSE_PUBLIC_KEY, LANGFUSE_SECRET_KEY, and optionally LANGFUSE_HOST.
func NewLangfuseClient() *LangfuseClient {
	host := os.Getenv("LANGFUSE_HOST")
	if host == "" {
		host = "https://cloud.langfuse.com"
	}
	return &LangfuseClient{
		baseURL:    host,
		publicKey:  os.Getenv("LANGFUSE_PUBLIC_KEY"),
		secretKey:  os.Getenv("LANGFUSE_SECRET_KEY"),
		httpClient: &http.Client{Timeout: 10 * time.Second},
		flushSize:  10,
	}
}

// Enabled reports whether Langfuse credentials are configured.
func (c *LangfuseClient) Enabled() bool {
	return c.publicKey != "" && c.secretKey != ""
}

// Trace records an LLM call event.
func (c *LangfuseClient) Trace(ctx context.Context, ev TraceEvent) {
	if !c.Enabled() {
		return
	}
	c.mu.Lock()
	c.batch = append(c.batch, event{Type: "trace-create", Body: ev})
	shouldFlush := len(c.batch) >= c.flushSize
	c.mu.Unlock()

	if shouldFlush {
		go c.Flush(ctx)
	}
}

// Flush sends all batched events to Langfuse.
func (c *LangfuseClient) Flush(ctx context.Context) error {
	c.mu.Lock()
	if len(c.batch) == 0 {
		c.mu.Unlock()
		return nil
	}
	events := c.batch
	c.batch = nil
	c.mu.Unlock()

	payload := map[string]interface{}{"batch": events}
	data, err := json.Marshal(payload)
	if err != nil {
		return err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+"/api/public/ingestion", bytes.NewReader(data))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	req.SetBasicAuth(c.publicKey, c.secretKey)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return err
	}
	resp.Body.Close()
	if resp.StatusCode >= 400 {
		return fmt.Errorf("langfuse: HTTP %d", resp.StatusCode)
	}
	return nil
}
