package mcp

import (
	"bufio"
	"context"
	"crypto/sha1"
	"encoding/base64"
	"encoding/binary"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

// wsTestServer is a minimal RFC 6455 server used to exercise the ConnectWS
// client. It echoes the MCP handshake: it responds to `initialize` and
// `tools/list` with canned JSON-RPC results and replies to any other method
// with an empty result, so we can verify the full request/response loop.
func wsTestServer(t *testing.T) *httptest.Server {
	t.Helper()
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.EqualFold(r.Header.Get("Upgrade"), "websocket") {
			http.Error(w, "expected websocket upgrade", http.StatusBadRequest)
			return
		}
		key := r.Header.Get("Sec-WebSocket-Key")
		accept := wsAcceptKey(key)

		hj, ok := w.(http.Hijacker)
		if !ok {
			t.Errorf("response writer does not support hijacking")
			return
		}
		conn, brw, err := hj.Hijack()
		if err != nil {
			t.Errorf("hijack: %v", err)
			return
		}
		defer conn.Close()

		resp := "HTTP/1.1 101 Switching Protocols\r\n" +
			"Upgrade: websocket\r\n" +
			"Connection: Upgrade\r\n" +
			"Sec-WebSocket-Accept: " + accept + "\r\n\r\n"
		if _, err := io.WriteString(brw, resp); err != nil {
			return
		}
		_ = brw.Flush()

		for {
			op, payload, err := srvReadMessage(brw.Reader)
			if err != nil {
				return
			}
			if op == wsOpClose {
				return
			}
			if op != wsOpText {
				continue
			}
			var req struct {
				ID     int    `json:"id"`
				Method string `json:"method"`
			}
			if err := json.Unmarshal(payload, &req); err != nil {
				continue
			}
			if req.ID == 0 {
				continue // notification, no reply
			}
			var result string
			switch req.Method {
			case "initialize":
				result = `{"protocolVersion":"2025-03-26","serverInfo":{"name":"test","version":"1"}}`
			case "tools/list":
				result = `{"tools":[{"name":"echo","description":"echoes"}]}`
			default:
				result = `{}`
			}
			out := []byte(`{"jsonrpc":"2.0","id":` + itoa(req.ID) + `,"result":` + result + `}`)
			if err := srvWriteFrame(brw, wsOpText, out); err != nil {
				return
			}
		}
	})
	return httptest.NewServer(handler)
}

func TestConnectWS_HandshakeAndExchange(t *testing.T) {
	ts := wsTestServer(t)
	defer ts.Close()

	wsURL := "ws://" + strings.TrimPrefix(ts.URL, "http://")

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	srv, err := ConnectWS(ctx, "test", wsURL, nil)
	if err != nil {
		t.Fatalf("ConnectWS: %v", err)
	}
	defer srv.Close()

	tools, err := srv.ListTools(ctx)
	if err != nil {
		t.Fatalf("ListTools: %v", err)
	}
	if len(tools) != 1 || tools[0].Name != "echo" {
		t.Fatalf("unexpected tools: %+v", tools)
	}
}

func TestConnectWS_BadScheme(t *testing.T) {
	_, err := ConnectWS(context.Background(), "x", "ftp://example.com", nil)
	if err == nil {
		t.Fatal("expected error for unsupported scheme")
	}
}

func TestWSAcceptKey(t *testing.T) {
	// Known vector from RFC 6455 §1.3.
	key := "dGhlIHNhbXBsZSBub25jZQ=="
	want := "s3pPLMBiTxaQ9kYGzzhZRbK+xOo="
	if got := wsAcceptKey(key); got != want {
		t.Errorf("wsAcceptKey = %q, want %q", got, want)
	}
	// Sanity: matches a manual sha1 computation.
	h := sha1.Sum([]byte(key + wsGUID))
	if base64.StdEncoding.EncodeToString(h[:]) != want {
		t.Error("manual sha1 mismatch")
	}
}

// --- server-side frame helpers (client-masked in, server-unmasked out) ---

func srvReadMessage(r *bufio.Reader) (int, []byte, error) {
	var msgOp int
	var buf []byte
	for {
		fin, op, frame, err := srvReadFrame(r)
		if err != nil {
			return 0, nil, err
		}
		if op >= wsOpClose {
			return op, frame, nil
		}
		if op != wsOpContinuation {
			msgOp = op
		}
		buf = append(buf, frame...)
		if fin {
			return msgOp, buf, nil
		}
	}
}

func srvReadFrame(r *bufio.Reader) (fin bool, opcode int, payload []byte, err error) {
	var hdr [2]byte
	if _, err = io.ReadFull(r, hdr[:]); err != nil {
		return
	}
	fin = hdr[0]&0x80 != 0
	opcode = int(hdr[0] & 0x0F)
	masked := hdr[1]&0x80 != 0
	length := uint64(hdr[1] & 0x7F)
	switch length {
	case 126:
		var ext [2]byte
		if _, err = io.ReadFull(r, ext[:]); err != nil {
			return
		}
		length = uint64(binary.BigEndian.Uint16(ext[:]))
	case 127:
		var ext [8]byte
		if _, err = io.ReadFull(r, ext[:]); err != nil {
			return
		}
		length = binary.BigEndian.Uint64(ext[:])
	}
	var mask [4]byte
	if masked {
		if _, err = io.ReadFull(r, mask[:]); err != nil {
			return
		}
	}
	payload = make([]byte, length)
	if _, err = io.ReadFull(r, payload); err != nil {
		return
	}
	if masked {
		for i := range payload {
			payload[i] ^= mask[i%4]
		}
	}
	return
}

func srvWriteFrame(bw *bufio.ReadWriter, opcode int, payload []byte) error {
	var hdr []byte
	hdr = append(hdr, byte(0x80|opcode))
	n := len(payload)
	switch {
	case n <= 125:
		hdr = append(hdr, byte(n)) // server frames are unmasked
	case n <= 0xFFFF:
		hdr = append(hdr, 126)
		var ext [2]byte
		binary.BigEndian.PutUint16(ext[:], uint16(n))
		hdr = append(hdr, ext[:]...)
	default:
		hdr = append(hdr, 127)
		var ext [8]byte
		binary.BigEndian.PutUint64(ext[:], uint64(n))
		hdr = append(hdr, ext[:]...)
	}
	if _, err := bw.Write(hdr); err != nil {
		return err
	}
	if _, err := bw.Write(payload); err != nil {
		return err
	}
	return bw.Flush()
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	var b [20]byte
	i := len(b)
	neg := n < 0
	if neg {
		n = -n
	}
	for n > 0 {
		i--
		b[i] = byte('0' + n%10)
		n /= 10
	}
	if neg {
		i--
		b[i] = '-'
	}
	return string(b[i:])
}
