package mcp

import (
	"bufio"
	"context"
	"crypto/rand"
	"crypto/sha1" // #nosec G505 -- SHA-1 required by RFC 6455 Sec-WebSocket-Accept
	"crypto/tls"
	"encoding/base64"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"
)

// Transport coverage for MCP servers:
//
//   - stdio        : Connect      (mcp.go)   — child process over stdin/stdout
//   - http         : ConnectHTTP  (http.go)  — streamable HTTP, JSON-RPC per request
//   - sse          : ConnectSSE   (http.go)  — HTTP POST, Server-Sent Events response
//   - websocket    : ConnectWS    (ws.go)    — full-duplex JSON-RPC over a single
//                                              WebSocket connection (this file)
//
// The WebSocket transport speaks the same JSON-RPC 2.0 framing as the other
// transports (one JSON-RPC message per WebSocket text frame). It implements
// the minimal RFC 6455 client handshake and frame codec over net.Dial so it
// requires no third-party dependency.

// wsGUID is the RFC 6455 magic value used to compute the Sec-WebSocket-Accept
// response from the client's Sec-WebSocket-Key.
const wsGUID = "258EAFA5-E914-47DA-95CA-C5AB0DC85B11"

// WebSocket opcodes (RFC 6455 §5.2).
const (
	wsOpContinuation = 0x0
	wsOpText         = 0x1
	wsOpBinary       = 0x2
	wsOpClose        = 0x8
	wsOpPing         = 0x9
	wsOpPong         = 0xA
)

// WSServer represents an MCP server connected via the WebSocket transport.
// It maintains a single persistent connection over which JSON-RPC requests and
// responses are multiplexed by request ID, mirroring the stdio transport.
type WSServer struct {
	Name    string
	URL     string
	Headers map[string]string

	conn   net.Conn
	rw     *bufio.ReadWriter
	writeM sync.Mutex // serializes frame writes

	mu      sync.Mutex
	nextID  int
	pending map[int]chan json.RawMessage
	pendMu  sync.Mutex

	closeOnce sync.Once
	closed    chan struct{}
}

// ConnectWS connects to an MCP server via the WebSocket transport and performs
// the JSON-RPC `initialize` handshake. The url scheme may be ws:// or wss://;
// http:// and https:// are accepted as aliases.
func ConnectWS(ctx context.Context, name, rawURL string, headers map[string]string) (*WSServer, error) {
	conn, rw, err := wsDial(ctx, rawURL, headers)
	if err != nil {
		return nil, fmt.Errorf("mcp ws dial: %w", err)
	}

	s := &WSServer{
		Name:    name,
		URL:     rawURL,
		Headers: headers,
		conn:    conn,
		rw:      rw,
		pending: make(map[int]chan json.RawMessage),
		closed:  make(chan struct{}),
	}

	go s.readLoop()

	if _, err := s.Call(ctx, "initialize", map[string]interface{}{
		"protocolVersion": "2025-03-26",
		"capabilities":    map[string]interface{}{},
		"clientInfo":      map[string]interface{}{"name": "graycode", "version": clientVersion},
	}); err != nil {
		_ = s.Close()
		return nil, fmt.Errorf("mcp ws init: %w", err)
	}

	// Best-effort initialized notification, matching the stdio transport.
	s.notify("notifications/initialized", nil)

	return s, nil
}

// wsDial performs the RFC 6455 opening handshake and returns the raw
// connection plus a buffered reader/writer positioned just after the
// switching-protocols response.
func wsDial(ctx context.Context, rawURL string, headers map[string]string) (net.Conn, *bufio.ReadWriter, error) {
	u, err := url.Parse(rawURL)
	if err != nil {
		return nil, nil, err
	}

	secure := false
	switch strings.ToLower(u.Scheme) {
	case "wss", "https":
		secure = true
	case "ws", "http":
		secure = false
	default:
		return nil, nil, fmt.Errorf("unsupported scheme %q (want ws/wss)", u.Scheme)
	}

	host := u.Host
	if u.Port() == "" {
		if secure {
			host = net.JoinHostPort(u.Hostname(), "443")
		} else {
			host = net.JoinHostPort(u.Hostname(), "80")
		}
	}

	dialer := &net.Dialer{}
	conn, err := dialer.DialContext(ctx, "tcp", host)
	if err != nil {
		return nil, nil, err
	}
	if secure {
		// TLS is wired here for wss:// completeness; the no-dep test server
		// uses ws://. Pin the minimum version explicitly (matches the stdlib
		// client default, but guards against future config drift).
		tlsConn := tls.Client(conn, &tls.Config{ // #nosec G402 -- MinVersion already set appropriately
			ServerName: u.Hostname(),
			MinVersion: tls.VersionTLS12,
		})
		if err = tlsConn.HandshakeContext(ctx); err != nil {
			_ = conn.Close()
			return nil, nil, err
		}
		conn = tlsConn
	}

	if deadline, ok := ctx.Deadline(); ok {
		_ = conn.SetDeadline(deadline)
	}

	key, err := wsNonce()
	if err != nil {
		_ = conn.Close()
		return nil, nil, err
	}

	reqPath := u.RequestURI()
	if reqPath == "" {
		reqPath = "/"
	}

	var b strings.Builder
	fmt.Fprintf(&b, "GET %s HTTP/1.1\r\n", reqPath)
	fmt.Fprintf(&b, "Host: %s\r\n", u.Host)
	b.WriteString("Upgrade: websocket\r\n")
	b.WriteString("Connection: Upgrade\r\n")
	fmt.Fprintf(&b, "Sec-WebSocket-Key: %s\r\n", key)
	b.WriteString("Sec-WebSocket-Version: 13\r\n")
	for k, v := range headers {
		fmt.Fprintf(&b, "%s: %s\r\n", k, v)
	}
	b.WriteString("\r\n")

	if _, writeErr := io.WriteString(conn, b.String()); writeErr != nil {
		_ = conn.Close()
		return nil, nil, writeErr
	}

	rw := bufio.NewReadWriter(bufio.NewReader(conn), bufio.NewWriter(conn))
	resp, err := http.ReadResponse(rw.Reader, &http.Request{Method: "GET"})
	if err != nil {
		_ = conn.Close()
		return nil, nil, err
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusSwitchingProtocols {
		_ = conn.Close()
		return nil, nil, fmt.Errorf("handshake failed: HTTP %d", resp.StatusCode)
	}
	if !strings.EqualFold(resp.Header.Get("Upgrade"), "websocket") {
		_ = conn.Close()
		return nil, nil, fmt.Errorf("handshake failed: missing Upgrade: websocket")
	}
	if got, want := resp.Header.Get("Sec-WebSocket-Accept"), wsAcceptKey(key); got != want {
		_ = conn.Close()
		return nil, nil, fmt.Errorf("handshake failed: bad Sec-WebSocket-Accept")
	}

	// Clear the dial deadline; per-call timeouts are handled in Call.
	_ = conn.SetDeadline(time.Time{})
	return conn, rw, nil
}

// wsNonce returns a base64-encoded 16-byte random nonce for Sec-WebSocket-Key.
func wsNonce() (string, error) {
	buf := make([]byte, 16)
	if _, err := rand.Read(buf); err != nil {
		return "", err
	}
	return base64.StdEncoding.EncodeToString(buf), nil
}

// wsAcceptKey computes the expected Sec-WebSocket-Accept value for a key.
func wsAcceptKey(key string) string {
	h := sha1.Sum([]byte(key + wsGUID)) // #nosec G401 -- SHA-1 required by RFC 6455 Sec-WebSocket-Accept
	return base64.StdEncoding.EncodeToString(h[:])
}

// readLoop reads WebSocket frames, reassembles text messages, and dispatches
// JSON-RPC responses to pending request channels. It exits when the connection
// closes or a protocol/read error occurs.
func (s *WSServer) readLoop() {
	defer s.failAllPending()
	for {
		opcode, payload, err := s.readMessage()
		if err != nil {
			return
		}
		switch opcode {
		case wsOpClose:
			return
		case wsOpPing:
			_ = s.writeFrame(wsOpPong, payload)
			continue
		case wsOpPong:
			continue
		case wsOpText, wsOpBinary:
			var msg jsonrpcResponse
			if err := json.Unmarshal(payload, &msg); err != nil {
				continue
			}
			if msg.ID == 0 {
				continue // notification
			}
			s.pendMu.Lock()
			ch, ok := s.pending[msg.ID]
			if ok {
				delete(s.pending, msg.ID)
			}
			s.pendMu.Unlock()
			if !ok {
				continue
			}
			if msg.Error != nil {
				ch <- nil
			} else {
				ch <- msg.Result
			}
			close(ch)
		}
	}
}

func (s *WSServer) failAllPending() {
	s.pendMu.Lock()
	for id, ch := range s.pending {
		close(ch)
		delete(s.pending, id)
	}
	s.pendMu.Unlock()
}

// readMessage reads a (possibly fragmented) WebSocket message and returns its
// opcode and reassembled payload. Control frames are returned individually.
func (s *WSServer) readMessage() (opcode int, payload []byte, err error) {
	var msgOpcode int
	var buf []byte
	for {
		fin, op, frame, err := s.readFrame()
		if err != nil {
			return 0, nil, err
		}
		// Control frames (>= 0x8) are never fragmented.
		if op >= wsOpClose {
			return op, frame, nil
		}
		if op != wsOpContinuation {
			msgOpcode = op
		}
		buf = append(buf, frame...)
		if fin {
			return msgOpcode, buf, nil
		}
	}
}

// readFrame reads a single WebSocket frame. Server-to-client frames are never
// masked, per RFC 6455 §5.1.
func (s *WSServer) readFrame() (fin bool, opcode int, payload []byte, err error) {
	var hdr [2]byte
	if _, err = io.ReadFull(s.rw, hdr[:]); err != nil {
		return
	}
	fin = hdr[0]&0x80 != 0
	opcode = int(hdr[0] & 0x0F)
	masked := hdr[1]&0x80 != 0
	length := uint64(hdr[1] & 0x7F)

	switch length {
	case 126:
		var ext [2]byte
		if _, err = io.ReadFull(s.rw, ext[:]); err != nil {
			return
		}
		length = uint64(binary.BigEndian.Uint16(ext[:]))
	case 127:
		var ext [8]byte
		if _, err = io.ReadFull(s.rw, ext[:]); err != nil {
			return
		}
		length = binary.BigEndian.Uint64(ext[:])
	}

	var mask [4]byte
	if masked {
		if _, err = io.ReadFull(s.rw, mask[:]); err != nil {
			return
		}
	}

	payload = make([]byte, length)
	if _, err = io.ReadFull(s.rw, payload); err != nil {
		return
	}
	if masked {
		for i := range payload {
			payload[i] ^= mask[i%4]
		}
	}
	return
}

// writeFrame writes a single masked client frame (clients MUST mask, §5.3).
func (s *WSServer) writeFrame(opcode int, payload []byte) error {
	s.writeM.Lock()
	defer s.writeM.Unlock()
	if opcode < 0 || opcode > 0x0f {
		return fmt.Errorf("invalid websocket opcode %d", opcode)
	}

	var hdr []byte
	b0 := 0x80 | byte(opcode) // FIN + opcode
	hdr = append(hdr, b0)

	n := len(payload)
	switch {
	case n <= 125:
		hdr = append(hdr, 0x80|byte(n)) // mask bit + length
	case n <= 0xFFFF:
		hdr = append(hdr, 0x80|126)
		var ext [2]byte
		binary.BigEndian.PutUint16(ext[:], uint16(n))
		hdr = append(hdr, ext[:]...)
	default:
		hdr = append(hdr, 0x80|127)
		var ext [8]byte
		binary.BigEndian.PutUint64(ext[:], uint64(n))
		hdr = append(hdr, ext[:]...)
	}

	var mask [4]byte
	if _, err := rand.Read(mask[:]); err != nil {
		return err
	}
	hdr = append(hdr, mask[:]...)

	masked := make([]byte, n)
	for i := range payload {
		masked[i] = payload[i] ^ mask[i%4]
	}

	if _, err := s.rw.Write(hdr); err != nil {
		return err
	}
	if _, err := s.rw.Write(masked); err != nil {
		return err
	}
	return s.rw.Flush()
}

// Call sends a JSON-RPC request over the WebSocket and waits for the response.
func (s *WSServer) Call(ctx context.Context, method string, params interface{}) (json.RawMessage, error) {
	select {
	case <-s.closed:
		return nil, fmt.Errorf("mcp ws: connection closed")
	default:
	}

	s.mu.Lock()
	s.nextID++
	id := s.nextID
	s.mu.Unlock()

	ch := make(chan json.RawMessage, 1)
	s.pendMu.Lock()
	s.pending[id] = ch
	s.pendMu.Unlock()

	req := jsonrpcRequest{JSONRPC: "2.0", ID: id, Method: method, Params: params}
	data, _ := json.Marshal(req)
	if err := s.writeFrame(wsOpText, data); err != nil {
		s.pendMu.Lock()
		delete(s.pending, id)
		s.pendMu.Unlock()
		return nil, fmt.Errorf("mcp ws write: %w", err)
	}

	timeout := defaultCallTimeout
	if deadline, ok := ctx.Deadline(); ok {
		timeout = time.Until(deadline)
	}

	select {
	case result, ok := <-ch:
		if !ok {
			return nil, fmt.Errorf("mcp ws: connection closed")
		}
		if result == nil {
			return nil, fmt.Errorf("mcp ws: server returned error")
		}
		return result, nil
	case <-time.After(timeout):
		s.pendMu.Lock()
		delete(s.pending, id)
		s.pendMu.Unlock()
		return nil, fmt.Errorf("mcp ws: call %s timed out after %s", method, timeout)
	case <-ctx.Done():
		s.pendMu.Lock()
		delete(s.pending, id)
		s.pendMu.Unlock()
		return nil, ctx.Err()
	case <-s.closed:
		return nil, fmt.Errorf("mcp ws: connection closed")
	}
}

func (s *WSServer) notify(method string, params interface{}) {
	req := jsonrpcRequest{JSONRPC: "2.0", Method: method, Params: params}
	data, _ := json.Marshal(req)
	_ = s.writeFrame(wsOpText, data)
}

// ListTools returns tools from the WebSocket MCP server.
func (s *WSServer) ListTools(ctx context.Context) ([]Tool, error) {
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

// CallTool invokes a tool on the WebSocket MCP server.
func (s *WSServer) CallTool(ctx context.Context, name string, args map[string]interface{}) (string, error) {
	result, err := s.Call(ctx, "tools/call", map[string]interface{}{
		"name":      name,
		"arguments": args,
	})
	if err != nil {
		return "", err
	}
	// Reuse the shared decoder so the WebSocket transport honors the spec's
	// isError flag identically to the stdio transport: a remote tool failure
	// must surface as a Go error, not a successful result.
	return parseToolCallResult(result)
}

// Close sends a WebSocket close frame and tears down the connection.
func (s *WSServer) Close() error {
	var err error
	s.closeOnce.Do(func() {
		close(s.closed)
		_ = s.writeFrame(wsOpClose, nil)
		err = s.conn.Close()
	})
	return err
}
