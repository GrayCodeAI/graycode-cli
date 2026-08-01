package sandbox

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/GrayCodeAI/hawk/internal/testutil"
)

func startTestProxy(t *testing.T, proxy *NetworkProxy, ctx context.Context) string {
	t.Helper()
	addr, err := proxy.Start(ctx)
	if err != nil {
		testutil.SkipIfLoopbackUnavailable(t, err)
		t.Fatalf("Start() error = %v", err)
	}
	return addr
}

func TestIsAllowed_AllowlistMode(t *testing.T) {
	proxy := NewNetworkProxy(ProxyConfig{
		AllowedDomains: []string{"github.com", "*.golang.org"},
		Mode:           "allowlist",
	})

	tests := []struct {
		host    string
		allowed bool
	}{
		{"github.com", true},
		{"pkg.golang.org", true},
		{"evil.com", false},
		{"random.org", false},
	}

	for _, tt := range tests {
		t.Run(tt.host, func(t *testing.T) {
			if got := proxy.IsAllowed(tt.host); got != tt.allowed {
				t.Errorf("IsAllowed(%q) = %v, want %v", tt.host, got, tt.allowed)
			}
		})
	}
}

func TestIsAllowed_BlocklistMode(t *testing.T) {
	proxy := NewNetworkProxy(ProxyConfig{
		BlockedDomains: []string{"evil.com", "*.malware.net"},
		Mode:           "blocklist",
	})

	tests := []struct {
		host    string
		allowed bool
	}{
		{"github.com", true},
		{"example.org", true},
		{"evil.com", false},
		{"sub.malware.net", false},
	}

	for _, tt := range tests {
		t.Run(tt.host, func(t *testing.T) {
			if got := proxy.IsAllowed(tt.host); got != tt.allowed {
				t.Errorf("IsAllowed(%q) = %v, want %v", tt.host, got, tt.allowed)
			}
		})
	}
}

func TestIsAllowed_WildcardDomainMatching(t *testing.T) {
	proxy := NewNetworkProxy(ProxyConfig{
		AllowedDomains: []string{"*.github.com", "*.example.org"},
		Mode:           "allowlist",
	})

	tests := []struct {
		host    string
		allowed bool
	}{
		{"api.github.com", true},
		{"raw.github.com", true},
		{"github.com", true}, // wildcard also matches base
		{"sub.example.org", true},
		{"notgithub.com", false},
		{"evil.com", false},
	}

	for _, tt := range tests {
		t.Run(tt.host, func(t *testing.T) {
			if got := proxy.IsAllowed(tt.host); got != tt.allowed {
				t.Errorf("IsAllowed(%q) = %v, want %v", tt.host, got, tt.allowed)
			}
		})
	}
}

func TestIsAllowed_ExactDomainMatching(t *testing.T) {
	proxy := NewNetworkProxy(ProxyConfig{
		AllowedDomains: []string{"github.com", "golang.org"},
		Mode:           "allowlist",
	})

	tests := []struct {
		host    string
		allowed bool
	}{
		{"github.com", true},
		{"golang.org", true},
		{"api.github.com", false}, // exact match only
		{"sub.golang.org", false}, // exact match only
		{"notgithub.com", false},
	}

	for _, tt := range tests {
		t.Run(tt.host, func(t *testing.T) {
			if got := proxy.IsAllowed(tt.host); got != tt.allowed {
				t.Errorf("IsAllowed(%q) = %v, want %v", tt.host, got, tt.allowed)
			}
		})
	}
}

func TestIsAllowed_AllowAllOverride(t *testing.T) {
	proxy := NewNetworkProxy(ProxyConfig{
		BlockedDomains: []string{"evil.com"},
		Mode:           "open",
	})

	// AllowAll should bypass all checks.
	if !proxy.AllowAll {
		t.Fatal("expected AllowAll to be true for 'open' mode")
	}

	tests := []struct {
		host    string
		allowed bool
	}{
		{"evil.com", true},
		{"anything.com", true},
		{"192.168.1.1", true},
	}

	for _, tt := range tests {
		t.Run(tt.host, func(t *testing.T) {
			if got := proxy.IsAllowed(tt.host); got != tt.allowed {
				t.Errorf("IsAllowed(%q) = %v, want %v", tt.host, got, tt.allowed)
			}
		})
	}
}

func TestIsAllowed_BlockAllOverride(t *testing.T) {
	proxy := NewNetworkProxy(ProxyConfig{
		AllowedDomains: []string{"github.com"},
		Mode:           "closed",
	})

	// BlockAll should block everything.
	if !proxy.BlockAll {
		t.Fatal("expected BlockAll to be true for 'closed' mode")
	}

	tests := []struct {
		host    string
		allowed bool
	}{
		{"github.com", false},
		{"anything.com", false},
		{"localhost", false},
	}

	for _, tt := range tests {
		t.Run(tt.host, func(t *testing.T) {
			if got := proxy.IsAllowed(tt.host); got != tt.allowed {
				t.Errorf("IsAllowed(%q) = %v, want %v", tt.host, got, tt.allowed)
			}
		})
	}
}

func TestIsAllowed_BlockedTakesPrecedenceOverAllowed(t *testing.T) {
	proxy := NewNetworkProxy(ProxyConfig{
		AllowedDomains: []string{"*.github.com"},
		BlockedDomains: []string{"evil.github.com"},
		Mode:           "allowlist",
	})

	tests := []struct {
		host    string
		allowed bool
	}{
		{"api.github.com", true},
		{"evil.github.com", false}, // blocked takes precedence
	}

	for _, tt := range tests {
		t.Run(tt.host, func(t *testing.T) {
			if got := proxy.IsAllowed(tt.host); got != tt.allowed {
				t.Errorf("IsAllowed(%q) = %v, want %v", tt.host, got, tt.allowed)
			}
		})
	}
}

func TestIsAllowed_HostWithPort(t *testing.T) {
	proxy := NewNetworkProxy(ProxyConfig{
		AllowedDomains: []string{"github.com"},
		Mode:           "allowlist",
	})

	if !proxy.IsAllowed("github.com:443") {
		t.Error("IsAllowed should strip port before matching")
	}
	if proxy.IsAllowed("evil.com:443") {
		t.Error("IsAllowed should still block even with port")
	}
}

func TestStatsTracking(t *testing.T) {
	proxy := NewNetworkProxy(ProxyConfig{
		AllowedDomains: []string{"github.com"},
		BlockedDomains: []string{"evil.com"},
		Mode:           "allowlist",
		LogRequests:    true,
	})

	// Simulate requests by calling recordRequest directly.
	proxy.recordRequest("github.com", "GET", true)
	proxy.recordRequest("github.com", "POST", true)
	proxy.recordRequest("evil.com", "GET", false)

	stats := proxy.GetStats()

	if stats.AllowedRequests != 2 {
		t.Errorf("AllowedRequests = %d, want 2", stats.AllowedRequests)
	}
	if stats.BlockedRequests != 1 {
		t.Errorf("BlockedRequests = %d, want 1", stats.BlockedRequests)
	}
	if stats.UniqueHosts["github.com"] != 2 {
		t.Errorf("UniqueHosts[github.com] = %d, want 2", stats.UniqueHosts["github.com"])
	}
	if stats.UniqueHosts["evil.com"] != 1 {
		t.Errorf("UniqueHosts[evil.com] = %d, want 1", stats.UniqueHosts["evil.com"])
	}
}

func TestLogRecording(t *testing.T) {
	proxy := NewNetworkProxy(ProxyConfig{
		AllowedDomains: []string{"github.com"},
		Mode:           "allowlist",
		LogRequests:    true,
	})

	proxy.recordRequest("github.com", "GET", true)
	proxy.recordRequest("evil.com", "POST", false)

	log := proxy.GetLog()

	if len(log) != 2 {
		t.Fatalf("GetLog() returned %d entries, want 2", len(log))
	}

	if log[0].Host != "github.com" || log[0].Method != "GET" || !log[0].Allowed {
		t.Errorf("log[0] = %+v, want github.com GET allowed", log[0])
	}
	if log[1].Host != "evil.com" || log[1].Method != "POST" || log[1].Allowed {
		t.Errorf("log[1] = %+v, want evil.com POST blocked", log[1])
	}
	if log[0].Timestamp.IsZero() {
		t.Error("log entry timestamp should not be zero")
	}
}

func TestLogNotRecordedWhenDisabled(t *testing.T) {
	proxy := NewNetworkProxy(ProxyConfig{
		AllowedDomains: []string{"github.com"},
		Mode:           "allowlist",
		LogRequests:    false,
	})

	proxy.recordRequest("github.com", "GET", true)

	log := proxy.GetLog()
	if len(log) != 0 {
		t.Errorf("GetLog() returned %d entries, want 0 when logging disabled", len(log))
	}
}

func TestEnvVars(t *testing.T) {
	proxy := NewNetworkProxy(ProxyConfig{
		Mode: "allowlist",
	})
	proxy.Port = 12345

	env := proxy.EnvVars()

	expected := "http://" + testutil.LoopbackHost + ":12345"
	if env["HTTP_PROXY"] != expected {
		t.Errorf("HTTP_PROXY = %q, want %q", env["HTTP_PROXY"], expected)
	}
	if env["HTTPS_PROXY"] != expected {
		t.Errorf("HTTPS_PROXY = %q, want %q", env["HTTPS_PROXY"], expected)
	}
	if env["http_proxy"] != expected {
		t.Errorf("http_proxy = %q, want %q", env["http_proxy"], expected)
	}
	if env["https_proxy"] != expected {
		t.Errorf("https_proxy = %q, want %q", env["https_proxy"], expected)
	}
	if env["NO_PROXY"] != testutil.LoopbackNoProxy {
		t.Errorf("NO_PROXY = %q, want %q", env["NO_PROXY"], testutil.LoopbackNoProxy)
	}
}

func TestDefaultDevelopmentConfig(t *testing.T) {
	config := DefaultDevelopmentConfig()

	proxy := NewNetworkProxy(config)

	// Should allow common registries.
	allowedHosts := []string{
		"api.github.com",
		"pkg.golang.org",
		"registry.npmjs.org",
		"pypi.org",
		"crates.io",
		"rubygems.org",
		"registry.docker.io",
		"storage.googleapis.com",
	}

	for _, host := range allowedHosts {
		if !proxy.IsAllowed(host) {
			t.Errorf("DefaultDevelopmentConfig should allow %q", host)
		}
	}

	// Should block internal networks.
	blockedHosts := []string{
		"service.internal",
		"169.254.169.254",
		"10.0.0.1",
		"192.168.1.1",
	}

	for _, host := range blockedHosts {
		if proxy.IsAllowed(host) {
			t.Errorf("DefaultDevelopmentConfig should block %q", host)
		}
	}
}

func TestDefaultDevelopmentConfig_ModeIsAllowlist(t *testing.T) {
	config := DefaultDevelopmentConfig()
	if config.Mode != "allowlist" {
		t.Errorf("DefaultDevelopmentConfig Mode = %q, want %q", config.Mode, "allowlist")
	}
}

func TestDefaultDevelopmentConfig_BlocksPrivateDestinations(t *testing.T) {
	proxy := NewNetworkProxy(DefaultDevelopmentConfig())
	for _, host := range []string{"localhost", "127.0.0.1", "[::1]", "10.0.0.1", "100.64.0.1"} {
		if proxy.IsAllowed(host) {
			t.Errorf("DefaultDevelopmentConfig should block private destination %q", host)
		}
	}
}

func TestPrivateIPClassification(t *testing.T) {
	for _, tc := range []struct {
		ip      string
		private bool
	}{
		{"127.0.0.1", true},
		{"169.254.169.254", true},
		{"10.0.0.1", true},
		{"100.64.0.1", true},
		{"8.8.8.8", false},
	} {
		if got := isPrivateIP(net.ParseIP(tc.ip)); got != tc.private {
			t.Errorf("isPrivateIP(%q) = %v, want %v", tc.ip, got, tc.private)
		}
	}
}

func TestStart_AssignsPort(t *testing.T) {
	proxy := NewNetworkProxy(ProxyConfig{
		AllowedDomains: []string{"*"},
		Mode:           "allowlist",
	})

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	addr := startTestProxy(t, proxy, ctx)
	defer proxy.Stop()

	if addr == "" {
		t.Fatal("Start() returned empty address")
	}

	// Check that a port was assigned.
	_, portStr, err := net.SplitHostPort(addr)
	if err != nil {
		t.Fatalf("SplitHostPort(%q) error = %v", addr, err)
	}
	if portStr == "0" {
		t.Error("expected an actual port, got 0")
	}
	if proxy.Port == 0 {
		t.Error("proxy.Port should be non-zero after Start")
	}
}

func TestStop_Clean(t *testing.T) {
	proxy := NewNetworkProxy(ProxyConfig{
		Mode: "open",
	})

	ctx := context.Background()
	addr := startTestProxy(t, proxy, ctx)

	// Verify the proxy is listening.
	conn, err := net.DialTimeout("tcp", addr, time.Second)
	if err != nil {
		t.Fatalf("could not connect to proxy at %s: %v", addr, err)
	}
	conn.Close()

	// Stop the proxy.
	if stopErr := proxy.Stop(); stopErr != nil {
		t.Fatalf("Stop() error = %v", stopErr)
	}

	// Give it a moment to close.
	time.Sleep(50 * time.Millisecond)

	// Verify the proxy is no longer listening.
	conn, err = net.DialTimeout("tcp", addr, 200*time.Millisecond)
	if err == nil {
		conn.Close()
		t.Error("expected connection to fail after Stop, but it succeeded")
	}
}

func TestDomainMatch(t *testing.T) {
	tests := []struct {
		pattern string
		host    string
		match   bool
	}{
		// Exact match.
		{"github.com", "github.com", true},
		{"github.com", "api.github.com", false},
		{"github.com", "notgithub.com", false},

		// Wildcard prefix.
		{"*.github.com", "api.github.com", true},
		{"*.github.com", "raw.github.com", true},
		{"*.github.com", "github.com", true}, // base domain matches wildcard
		{"*.github.com", "notgithub.com", false},

		// Suffix match.
		{".github.com", "api.github.com", true},
		{".github.com", "github.com", false},

		// IP wildcard.
		{"169.254.*", "169.254.169.254", true},
		{"169.254.*", "169.254.1.1", true},
		{"169.254.*", "10.0.0.1", false},
		{"10.*", "10.0.0.1", true},
		{"10.*", "110.0.0.1", false},
		{"192.168.*", "192.168.1.1", true},

		// Case insensitive.
		{"GitHub.Com", "github.com", true},
		{"*.GITHUB.COM", "api.github.com", true},
	}

	for _, tt := range tests {
		name := fmt.Sprintf("%s_%s", tt.pattern, tt.host)
		t.Run(name, func(t *testing.T) {
			if got := domainMatch(tt.pattern, tt.host); got != tt.match {
				t.Errorf("domainMatch(%q, %q) = %v, want %v", tt.pattern, tt.host, got, tt.match)
			}
		})
	}
}

func TestProxyBlocksRequest(t *testing.T) {
	proxy := NewNetworkProxy(ProxyConfig{
		AllowedDomains: []string{"allowed.example.com"},
		Mode:           "allowlist",
		LogRequests:    true,
	})

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	addr := startTestProxy(t, proxy, ctx)
	defer proxy.Stop()

	// Make a request to a blocked domain through the proxy.
	client := &http.Client{
		Transport: &http.Transport{
			Proxy: http.ProxyURL(mustParseURL("http://" + addr)),
		},
		Timeout: 5 * time.Second,
	}

	resp, err := client.Get("http://blocked.example.com/test")
	if err != nil {
		// Connection error could mean proxy rejected it before we got a response.
		// Check if it's a 403 or connection refused.
		if !strings.Contains(err.Error(), "Forbidden") && !strings.Contains(err.Error(), "403") {
			// If the proxy sent a 403, some clients might not propagate it cleanly.
			// Let's check stats instead.
			stats := proxy.GetStats()
			if stats.BlockedRequests < 1 {
				t.Fatalf("unexpected error and no blocked requests recorded: %v", err)
			}
			return
		}
		return
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusForbidden {
		t.Errorf("expected 403 Forbidden, got %d", resp.StatusCode)
	}
}

func TestProxyAllowsRequest(t *testing.T) {
	// Create a simple test HTTP server.
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("hello from target"))
	})
	targetServer := &http.Server{Handler: handler}
	targetLn, err := net.Listen("tcp", testutil.LoopbackDynamicAddr)
	if err != nil {
		testutil.SkipIfLoopbackUnavailable(t, err)
		t.Fatalf("failed to create target listener: %v", err)
	}
	go targetServer.Serve(targetLn)
	defer targetServer.Close()

	targetAddr := targetLn.Addr().String()
	targetHost, _, _ := net.SplitHostPort(targetAddr)

	proxy := NewNetworkProxy(ProxyConfig{
		AllowedDomains: []string{targetHost, testutil.LoopbackHost},
		Mode:           "allowlist",
		LogRequests:    true,
	})

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	addr := startTestProxy(t, proxy, ctx)
	defer proxy.Stop()

	// Make a request to the allowed target through the proxy.
	client := &http.Client{
		Transport: &http.Transport{
			Proxy: http.ProxyURL(mustParseURL("http://" + addr)),
		},
		Timeout: 5 * time.Second,
	}

	resp, err := client.Get(fmt.Sprintf("http://%s/test", targetAddr))
	if err != nil {
		t.Fatalf("request through proxy failed: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Errorf("expected 200 OK, got %d", resp.StatusCode)
	}

	stats := proxy.GetStats()
	if stats.AllowedRequests < 1 {
		t.Error("expected at least 1 allowed request in stats")
	}
}

func mustParseURL(rawURL string) *url.URL {
	u, err := url.Parse(rawURL)
	if err != nil {
		panic(err)
	}
	return u
}
