//go:build egressproxy

package sandbox

import (
	"bufio"
	"crypto/tls"
	"fmt"
	"io"
	"net"
	"net/http"
	"strings"
	"testing"
)

// TestEgressProxy_ConnectorEgress verifies the proxy's CONNECT tunneling and
// policy enforcement end-to-end against the real proxy running in this process
// (the same way production uses it: the proxy binds the host and dials out from
// the host). We exercise the actual dial path rather than going through a
// container, which keeps the test focused on proxy logic and off host<->container
// networking quirks.
func TestEgressProxy_ConnectorEgress(t *testing.T) {
	np := NewNetworkProxy(ProxyConfig{
		Host:           AllInterfaces,
		Mode:           "allowlist",
		LogRequests:    true,
		AllowedDomains: []string{"github.com", "*.github.com"},
	})
	addr, err := np.Start(t.Context())
	if err != nil {
		t.Fatalf("proxy start: %v", err)
	}
	defer np.Stop()
	t.Logf("proxy listening on %s", addr)

	// Helper: open a TCP connection to the proxy, send a CONNECT, and return the
	// parsed HTTP response.
	connect := func(target string) (*http.Response, net.Conn) {
		conn, err := net.Dial("tcp4", addr)
		if err != nil {
			t.Fatalf("dial proxy: %v", err)
		}
		req := fmt.Sprintf("CONNECT %s HTTP/1.1\r\nHost: %s\r\n\r\n", target, target)
		if _, err := conn.Write([]byte(req)); err != nil {
			conn.Close()
			t.Fatalf("write CONNECT: %v", err)
		}
		resp, err := http.ReadResponse(bufio.NewReader(conn), nil)
		if err != nil {
			conn.Close()
			t.Fatalf("read CONNECT response: %v", err)
		}
		return resp, conn
	}

	// Blocked domain: expect 403 and a closed tunnel.
	t.Run("blocked_domain", func(t *testing.T) {
		resp, conn := connect("evil.example:443")
		defer conn.Close()
		if resp.StatusCode != http.StatusForbidden {
			t.Errorf("evil.example: status = %d, want 403", resp.StatusCode)
		}
	})

	// Allowed domain: expect 200 Connection Established, then a working TLS
	// tunnel to the real target.
	t.Run("allowed_domain", func(t *testing.T) {
		resp, conn := connect("github.com:443")
		defer conn.Close()
		if resp.StatusCode != http.StatusOK {
			t.Errorf("github.com: status = %d, want 200", resp.StatusCode)
			return
		}
		// Tunnel TLS over the proxied connection and make a real HTTPS request.
		tlsConn := tls.Client(conn, &tls.Config{ServerName: "github.com"})
		if err := tlsConn.Handshake(); err != nil {
			t.Fatalf("tls handshake through proxy: %v", err)
		}
		defer tlsConn.Close()

		req, _ := http.NewRequest(http.MethodGet, "https://github.com/", nil)
		if err := req.Write(tlsConn); err != nil {
			t.Fatalf("write request through tunnel: %v", err)
		}
		serverResp, err := http.ReadResponse(bufio.NewReader(tlsConn), req)
		if err != nil {
			t.Fatalf("read response through tunnel: %v", err)
		}
		defer serverResp.Body.Close()
		body, _ := io.ReadAll(io.LimitReader(serverResp.Body, 256))
		t.Logf("github.com tunnel: status=%d body=%q", serverResp.StatusCode, strings.TrimSpace(string(body)))
		if serverResp.StatusCode != http.StatusOK {
			t.Errorf("github.com: server status = %d, want 200", serverResp.StatusCode)
		}
	})

	// Plain-HTTP forwarding path (handleHTTP) in open mode.
	t.Run("http_forward", func(t *testing.T) {
		np2 := NewNetworkProxy(ProxyConfig{Host: AllInterfaces, Mode: "open"})
		addr2, err := np2.Start(t.Context())
		if err != nil {
			t.Fatalf("proxy2 start: %v", err)
		}
		defer np2.Stop()

		conn, err := net.Dial("tcp4", addr2)
		if err != nil {
			t.Fatalf("dial proxy2: %v", err)
		}
		defer conn.Close()
		req := "GET http://example.com/ HTTP/1.1\r\nHost: example.com\r\n\r\n"
		if _, err := conn.Write([]byte(req)); err != nil {
			t.Fatalf("write GET: %v", err)
		}
		resp, err := http.ReadResponse(bufio.NewReader(conn), nil)
		if err != nil {
			t.Fatalf("read GET response: %v", err)
		}
		defer resp.Body.Close()
		t.Logf("example.com http: status=%d", resp.StatusCode)
		if resp.StatusCode != http.StatusOK {
			t.Errorf("example.com: status = %d, want 200", resp.StatusCode)
		}
	})

	// Policy accounting: one allowed (github.com) + one blocked (evil.example).
	if got := np.GetStats(); got.AllowedRequests != 1 || got.BlockedRequests != 1 {
		t.Errorf("stats = %+v, want allowed=1 blocked=1", got)
	}
}
