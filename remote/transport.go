package remote

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"sync"
	"time"
)

// Transport is the interface for bridge communication channels.
type Transport interface {
	Connect(ctx context.Context, url string, headers http.Header) error
	Send(msg BridgeMessage) error
	Receive() <-chan BridgeMessage
	Close() error
	IsConnected() bool
}

// HTTPTransport implements Transport over HTTP polling.
type HTTPTransport struct {
	mu         sync.RWMutex
	baseURL    string
	headers    http.Header
	client     *http.Client
	msgChan    chan BridgeMessage
	connected  bool
	pollCancel context.CancelFunc
	sessionID  string
}

// NewHTTPTransport creates an HTTP-based transport.
func NewHTTPTransport() *HTTPTransport {
	return &HTTPTransport{
		client:  &http.Client{Timeout: 30 * time.Second},
		msgChan: make(chan BridgeMessage, 100),
	}
}

func (t *HTTPTransport) Connect(ctx context.Context, url string, headers http.Header) error {
	t.mu.Lock()
	t.baseURL = url
	t.headers = headers
	t.connected = true
	t.mu.Unlock()

	pollCtx, cancel := context.WithCancel(ctx)
	t.pollCancel = cancel
	go t.pollLoop(pollCtx)

	return nil
}

func (t *HTTPTransport) Send(msg BridgeMessage) error {
	t.mu.RLock()
	if !t.connected {
		t.mu.RUnlock()
		return fmt.Errorf("not connected")
	}
	baseURL := t.baseURL
	headers := t.headers
	t.mu.RUnlock()

	data, err := json.Marshal(msg)
	if err != nil {
		return err
	}

	req, err := http.NewRequest("POST", baseURL+"/api/messages", jsonReader(data))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	for k, v := range headers {
		req.Header[k] = v
	}

	resp, err := t.client.Do(req)
	if err != nil {
		return err
	}
	resp.Body.Close()

	if resp.StatusCode >= 400 {
		return fmt.Errorf("send failed: %d", resp.StatusCode)
	}
	return nil
}

func (t *HTTPTransport) Receive() <-chan BridgeMessage {
	return t.msgChan
}

func (t *HTTPTransport) Close() error {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.connected = false
	if t.pollCancel != nil {
		t.pollCancel()
	}
	return nil
}

func (t *HTTPTransport) IsConnected() bool {
	t.mu.RLock()
	defer t.mu.RUnlock()
	return t.connected
}

func (t *HTTPTransport) pollLoop(ctx context.Context) {
	ticker := time.NewTicker(2 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			t.poll(ctx)
		}
	}
}

func (t *HTTPTransport) poll(ctx context.Context) {
	t.mu.RLock()
	baseURL := t.baseURL
	headers := t.headers
	sessionID := t.sessionID
	t.mu.RUnlock()

	url := baseURL + "/api/messages/poll"
	if sessionID != "" {
		url += "?session_id=" + sessionID
	}

	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return
	}
	for k, v := range headers {
		req.Header[k] = v
	}

	resp, err := t.client.Do(req)
	if err != nil {
		return
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return
	}

	var msgs []BridgeMessage
	if err := json.NewDecoder(resp.Body).Decode(&msgs); err != nil {
		return
	}

	for _, msg := range msgs {
		select {
		case t.msgChan <- msg:
		default:
		}
	}
}

// SSETransport implements Transport using Server-Sent Events for reads
// and HTTP POST for writes.
type SSETransport struct {
	mu        sync.RWMutex
	baseURL   string
	headers   http.Header
	client    *http.Client
	msgChan   chan BridgeMessage
	connected bool
	cancel    context.CancelFunc
}

// NewSSETransport creates an SSE-based transport.
func NewSSETransport() *SSETransport {
	return &SSETransport{
		client:  &http.Client{Timeout: 0}, // no timeout for SSE
		msgChan: make(chan BridgeMessage, 100),
	}
}

func (t *SSETransport) Connect(ctx context.Context, url string, headers http.Header) error {
	t.mu.Lock()
	t.baseURL = url
	t.headers = headers
	t.connected = true
	t.mu.Unlock()

	sseCtx, cancel := context.WithCancel(ctx)
	t.cancel = cancel
	go t.readSSE(sseCtx)

	return nil
}

func (t *SSETransport) Send(msg BridgeMessage) error {
	t.mu.RLock()
	if !t.connected {
		t.mu.RUnlock()
		return fmt.Errorf("not connected")
	}
	baseURL := t.baseURL
	headers := t.headers
	t.mu.RUnlock()

	data, err := json.Marshal(msg)
	if err != nil {
		return err
	}

	req, err := http.NewRequest("POST", baseURL+"/api/messages", jsonReader(data))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	for k, v := range headers {
		req.Header[k] = v
	}

	resp, err := t.client.Do(req)
	if err != nil {
		return err
	}
	resp.Body.Close()
	return nil
}

func (t *SSETransport) Receive() <-chan BridgeMessage {
	return t.msgChan
}

func (t *SSETransport) Close() error {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.connected = false
	if t.cancel != nil {
		t.cancel()
	}
	return nil
}

func (t *SSETransport) IsConnected() bool {
	t.mu.RLock()
	defer t.mu.RUnlock()
	return t.connected
}

func (t *SSETransport) readSSE(ctx context.Context) {
	t.mu.RLock()
	url := t.baseURL + "/api/events"
	headers := t.headers
	t.mu.RUnlock()

	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return
	}
	req.Header.Set("Accept", "text/event-stream")
	for k, v := range headers {
		req.Header[k] = v
	}

	resp, err := t.client.Do(req)
	if err != nil {
		return
	}
	defer resp.Body.Close()

	buf := make([]byte, 4096)
	var data []byte

	for {
		select {
		case <-ctx.Done():
			return
		default:
		}

		n, err := resp.Body.Read(buf)
		if n > 0 {
			data = append(data, buf[:n]...)
			// Parse SSE events from buffer
			for {
				event, rest, ok := parseSSEEvent(data)
				if !ok {
					break
				}
				data = rest
				if event != nil {
					select {
					case t.msgChan <- *event:
					default:
					}
				}
			}
		}
		if err == io.EOF {
			return
		}
		if err != nil {
			return
		}
	}
}

func parseSSEEvent(data []byte) (*BridgeMessage, []byte, bool) {
	// Look for double newline (event boundary)
	for i := 0; i < len(data)-1; i++ {
		if data[i] == '\n' && data[i+1] == '\n' {
			eventData := data[:i]
			rest := data[i+2:]

			// Parse "data: {...}" lines
			var msg BridgeMessage
			for _, line := range splitLines(eventData) {
				if len(line) > 6 && string(line[:6]) == "data: " {
					json.Unmarshal(line[6:], &msg)
				}
			}
			if msg.Type != "" {
				return &msg, rest, true
			}
			return nil, rest, true
		}
	}
	return nil, data, false
}

func splitLines(data []byte) [][]byte {
	var lines [][]byte
	start := 0
	for i, b := range data {
		if b == '\n' {
			lines = append(lines, data[start:i])
			start = i + 1
		}
	}
	if start < len(data) {
		lines = append(lines, data[start:])
	}
	return lines
}

func jsonReader(data []byte) io.Reader {
	return &jsonBytesReader{data: data}
}

type jsonBytesReader struct {
	data []byte
	pos  int
}

func (r *jsonBytesReader) Read(p []byte) (int, error) {
	if r.pos >= len(r.data) {
		return 0, io.EOF
	}
	n := copy(p, r.data[r.pos:])
	r.pos += n
	return n, nil
}
