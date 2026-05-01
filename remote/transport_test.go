package remote

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestHTTPTransport_Connect(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	transport := NewHTTPTransport()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	err := transport.Connect(ctx, server.URL, nil)
	if err != nil {
		t.Fatalf("Connect error: %v", err)
	}
	if !transport.IsConnected() {
		t.Error("should be connected")
	}

	transport.Close()
	if transport.IsConnected() {
		t.Error("should be disconnected after close")
	}
}

func TestHTTPTransport_Send(t *testing.T) {
	var received BridgeMessage
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/messages" {
			json.NewDecoder(r.Body).Decode(&received)
			w.WriteHeader(http.StatusAccepted)
		}
	}))
	defer server.Close()

	transport := NewHTTPTransport()
	ctx := context.Background()
	transport.Connect(ctx, server.URL, nil)
	defer transport.Close()

	msg := BridgeMessage{
		Type:      BridgeMsgUserInput,
		SessionID: "sess-1",
		Timestamp: time.Now(),
	}
	err := transport.Send(msg)
	if err != nil {
		t.Fatalf("Send error: %v", err)
	}
	if received.Type != BridgeMsgUserInput {
		t.Errorf("expected user_input, got %s", received.Type)
	}
}

func TestHTTPTransport_NotConnected(t *testing.T) {
	transport := NewHTTPTransport()
	err := transport.Send(BridgeMessage{Type: "test"})
	if err == nil {
		t.Error("expected error when not connected")
	}
}

func TestSSETransport_Connect(t *testing.T) {
	transport := NewSSETransport()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/events" {
			w.Header().Set("Content-Type", "text/event-stream")
			w.WriteHeader(http.StatusOK)
			flusher, ok := w.(http.Flusher)
			if ok {
				flusher.Flush()
			}
			// Keep connection open briefly
			time.Sleep(100 * time.Millisecond)
		} else if r.URL.Path == "/api/messages" {
			w.WriteHeader(http.StatusAccepted)
		}
	}))
	defer server.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	err := transport.Connect(ctx, server.URL, nil)
	if err != nil {
		t.Fatalf("Connect error: %v", err)
	}
	if !transport.IsConnected() {
		t.Error("should be connected")
	}

	transport.Close()
}

func TestBridgeProtocol_MessageRouting(t *testing.T) {
	msgChan := make(chan BridgeMessage, 10)
	transport := &mockTransport{
		receiveChan: msgChan,
		connected:   true,
	}

	protocol := NewBridgeProtocol(transport)

	var handledMsg BridgeMessage
	protocol.OnMessage(BridgeMsgUserInput, func(msg BridgeMessage) {
		handledMsg = msg
	})

	// Send a message and start listener
	go protocol.Listen()

	msgChan <- BridgeMessage{Type: BridgeMsgUserInput, SessionID: "test-session"}
	time.Sleep(50 * time.Millisecond)
	close(msgChan)

	if handledMsg.Type != BridgeMsgUserInput {
		t.Errorf("expected handler to be called, got type %q", handledMsg.Type)
	}
}

func TestBridgeClientWithTransport(t *testing.T) {
	transport := &mockTransport{
		receiveChan: make(chan BridgeMessage, 10),
		connected:   true,
	}

	client := NewBridgeClientWithTransport("http://localhost:8080", "token123", transport)
	if client == nil {
		t.Error("expected non-nil client")
	}
	if client.serverURL != "http://localhost:8080" {
		t.Errorf("expected URL to be set, got %s", client.serverURL)
	}
}

func TestParseSSEEvent(t *testing.T) {
	data := []byte("data: {\"type\":\"user_input\",\"sessionId\":\"s1\"}\n\n")

	msg, rest, ok := parseSSEEvent(data)
	if !ok {
		t.Fatal("expected ok=true")
	}
	if msg == nil {
		t.Fatal("expected non-nil message")
	}
	if msg.Type != "user_input" {
		t.Errorf("expected user_input, got %s", msg.Type)
	}
	if len(rest) != 0 {
		t.Errorf("expected empty rest, got %d bytes", len(rest))
	}
}

func TestParseSSEEvent_Incomplete(t *testing.T) {
	data := []byte("data: {\"type\":\"partial\"}")

	_, _, ok := parseSSEEvent(data)
	if ok {
		t.Error("incomplete event should return ok=false")
	}
}

// mockTransport implements Transport for testing.
type mockTransport struct {
	receiveChan chan BridgeMessage
	sentMsgs    []BridgeMessage
	connected   bool
}

func (m *mockTransport) Connect(ctx context.Context, url string, headers http.Header) error {
	m.connected = true
	return nil
}

func (m *mockTransport) Send(msg BridgeMessage) error {
	m.sentMsgs = append(m.sentMsgs, msg)
	return nil
}

func (m *mockTransport) Receive() <-chan BridgeMessage {
	return m.receiveChan
}

func (m *mockTransport) Close() error {
	m.connected = false
	return nil
}

func (m *mockTransport) IsConnected() bool {
	return m.connected
}
