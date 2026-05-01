package remote

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"sync"
	"time"

	"nhooyr.io/websocket"
)

// WSTransport implements Transport over a WebSocket connection.
type WSTransport struct {
	mu         sync.RWMutex
	conn       *websocket.Conn
	msgChan    chan BridgeMessage
	connected  bool
	cancel     context.CancelFunc
	pingTicker *time.Ticker
}

// NewWSTransport creates a new WebSocket transport.
func NewWSTransport() *WSTransport {
	return &WSTransport{
		msgChan: make(chan BridgeMessage, 100),
	}
}

func (t *WSTransport) Connect(ctx context.Context, url string, headers http.Header) error {
	opts := &websocket.DialOptions{
		HTTPHeader: headers,
	}

	conn, _, err := websocket.Dial(ctx, url, opts)
	if err != nil {
		return fmt.Errorf("ws dial: %w", err)
	}

	t.mu.Lock()
	t.conn = conn
	t.connected = true
	t.mu.Unlock()

	readCtx, cancel := context.WithCancel(ctx)
	t.cancel = cancel

	// Start read loop
	go t.readLoop(readCtx)

	// Start keepalive pings (30s interval)
	t.pingTicker = time.NewTicker(30 * time.Second)
	go t.pingLoop(readCtx)

	return nil
}

func (t *WSTransport) Send(msg BridgeMessage) error {
	t.mu.RLock()
	if !t.connected || t.conn == nil {
		t.mu.RUnlock()
		return fmt.Errorf("not connected")
	}
	conn := t.conn
	t.mu.RUnlock()

	data, err := json.Marshal(msg)
	if err != nil {
		return err
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	return conn.Write(ctx, websocket.MessageText, data)
}

func (t *WSTransport) Receive() <-chan BridgeMessage {
	return t.msgChan
}

func (t *WSTransport) Close() error {
	t.mu.Lock()
	defer t.mu.Unlock()

	t.connected = false
	if t.cancel != nil {
		t.cancel()
	}
	if t.pingTicker != nil {
		t.pingTicker.Stop()
	}
	if t.conn != nil {
		return t.conn.Close(websocket.StatusNormalClosure, "closing")
	}
	return nil
}

func (t *WSTransport) IsConnected() bool {
	t.mu.RLock()
	defer t.mu.RUnlock()
	return t.connected
}

func (t *WSTransport) readLoop(ctx context.Context) {
	defer func() {
		t.mu.Lock()
		t.connected = false
		t.mu.Unlock()
	}()

	for {
		select {
		case <-ctx.Done():
			return
		default:
		}

		_, data, err := t.conn.Read(ctx)
		if err != nil {
			if ctx.Err() != nil {
				return
			}
			// Connection lost — attempt reconnect or close
			return
		}

		var msg BridgeMessage
		if err := json.Unmarshal(data, &msg); err != nil {
			continue
		}

		select {
		case t.msgChan <- msg:
		default:
			// Drop if channel full
		}
	}
}

func (t *WSTransport) pingLoop(ctx context.Context) {
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.pingTicker.C:
			t.mu.RLock()
			conn := t.conn
			connected := t.connected
			t.mu.RUnlock()

			if !connected || conn == nil {
				return
			}

			pingCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
			err := conn.Ping(pingCtx)
			cancel()
			if err != nil {
				t.mu.Lock()
				t.connected = false
				t.mu.Unlock()
				return
			}
		}
	}
}
