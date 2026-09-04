// Package trace provides Langfuse tracing integration for LLM observability.
package oteltrace

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"sync"
	"time"

	contracts "github.com/GrayCodeAI/graycode-cli/internal/contracts/events"
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
type TraceEvent = contracts.TraceEvent

// UsageInfo tracks token usage.
type UsageInfo = contracts.UsageInfo

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
		go func() {
			if err := c.Flush(ctx); err != nil {
				slog.Warn("langfuse flush failed", "error", err)
			}
		}()
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
		c.requeue(events)
		return err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+"/api/public/ingestion", bytes.NewReader(data))
	if err != nil {
		c.requeue(events)
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	req.SetBasicAuth(c.publicKey, c.secretKey)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		c.requeue(events)
		return err
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode >= 400 {
		c.requeue(events)
		return fmt.Errorf("langfuse: HTTP %d", resp.StatusCode)
	}
	return nil
}

// requeue puts events that failed to send back at the front of the batch so a
// transient failure does not silently drop telemetry. The batch is capped at
// flushSize*2 to avoid unbounded growth under a persistent outage.
func (c *LangfuseClient) requeue(events []event) {
	if len(events) == 0 {
		return
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if len(c.batch)+len(events) > c.flushSize*2 {
		return // drop rather than grow without bound; the failure was already logged
	}
	c.batch = append(events, c.batch...)
}
