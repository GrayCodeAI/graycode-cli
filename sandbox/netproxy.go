package sandbox

import (
	"context"
	"fmt"
	"io"
	"net"
	"net/http"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

// ProxyStats tracks network proxy usage statistics.
type ProxyStats struct {
	AllowedRequests int64
	BlockedRequests int64
	TotalBytes      int64
	UniqueHosts     map[string]int
}

// ProxyLogEntry records a single proxy request event.
type ProxyLogEntry struct {
	Timestamp time.Time
	Host      string
	Method    string
	Allowed   bool
	Reason    string
}

// ProxyConfig configures the network proxy behavior.
type ProxyConfig struct {
	AllowedDomains []string
	BlockedDomains []string
	Mode           string // "allowlist", "blocklist", "open", "closed"
	LogRequests    bool
}

// NetworkProxy provides domain-level network access control for commands
// run by the agent. Inspired by Codex CLI's network-proxy approach.
type NetworkProxy struct {
	AllowedDomains []string
	BlockedDomains []string
	AllowAll       bool
	BlockAll       bool
	Port           int

	listener net.Listener
	mu       sync.RWMutex
	Stats    ProxyStats
	Log      []ProxyLogEntry

	config     ProxyConfig
	server     *http.Server
	cancelFunc context.CancelFunc
}

// NewNetworkProxy creates a new network proxy from the given configuration.
func NewNetworkProxy(config ProxyConfig) *NetworkProxy {
	np := &NetworkProxy{
		AllowedDomains: config.AllowedDomains,
		BlockedDomains: config.BlockedDomains,
		config:         config,
		Stats: ProxyStats{
			UniqueHosts: make(map[string]int),
		},
		Log: make([]ProxyLogEntry, 0),
	}

	switch config.Mode {
	case "open":
		np.AllowAll = true
	case "closed":
		np.BlockAll = true
	}

	return np
}

// Start starts the HTTP CONNECT proxy on localhost and returns the proxy address.
func (np *NetworkProxy) Start(ctx context.Context) (string, error) {
	ctx, cancel := context.WithCancel(ctx)
	np.cancelFunc = cancel

	addr := fmt.Sprintf("127.0.0.1:%d", np.Port)
	var err error
	np.listener, err = net.Listen("tcp", addr)
	if err != nil {
		cancel()
		return "", fmt.Errorf("failed to start proxy listener: %w", err)
	}

	// Update Port with the actual assigned port.
	np.Port = np.listener.Addr().(*net.TCPAddr).Port

	mux := http.NewServeMux()
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodConnect {
			np.handleConnect(w, r)
		} else {
			np.handleHTTP(w, r)
		}
	})

	np.server = &http.Server{
		Handler: mux,
	}

	go func() {
		if err := np.server.Serve(np.listener); err != nil && err != http.ErrServerClosed {
			// Server error — could log but we just return.
			_ = err
		}
	}()

	go func() {
		<-ctx.Done()
		_ = np.server.Close()
	}()

	return np.listener.Addr().String(), nil
}

// Stop stops the proxy server and cleans up resources.
func (np *NetworkProxy) Stop() error {
	if np.cancelFunc != nil {
		np.cancelFunc()
	}
	if np.server != nil {
		return np.server.Close()
	}
	return nil
}

// IsAllowed checks whether a host is permitted by the proxy rules.
func (np *NetworkProxy) IsAllowed(host string) bool {
	// Strip port if present.
	h, _, err := net.SplitHostPort(host)
	if err != nil {
		h = host
	}

	if np.AllowAll {
		return true
	}
	if np.BlockAll {
		return false
	}

	// Check blocked domains first (deny wins).
	for _, pattern := range np.BlockedDomains {
		if domainMatch(pattern, h) {
			return false
		}
	}

	// Check allowed domains.
	for _, pattern := range np.AllowedDomains {
		if domainMatch(pattern, h) {
			return true
		}
	}

	// Default depends on mode.
	switch np.config.Mode {
	case "blocklist":
		return true
	default:
		// "allowlist" and any other mode defaults to block.
		return false
	}
}

// EnvVars returns environment variables to set for child processes
// so they route traffic through this proxy.
func (np *NetworkProxy) EnvVars() map[string]string {
	addr := fmt.Sprintf("http://127.0.0.1:%d", np.Port)
	return map[string]string{
		"HTTP_PROXY":  addr,
		"HTTPS_PROXY": addr,
		"http_proxy":  addr,
		"https_proxy": addr,
		"NO_PROXY":    "localhost,127.0.0.1",
	}
}

// GetStats returns a copy of the current proxy statistics.
func (np *NetworkProxy) GetStats() ProxyStats {
	np.mu.RLock()
	defer np.mu.RUnlock()

	hosts := make(map[string]int, len(np.Stats.UniqueHosts))
	for k, v := range np.Stats.UniqueHosts {
		hosts[k] = v
	}
	return ProxyStats{
		AllowedRequests: atomic.LoadInt64(&np.Stats.AllowedRequests),
		BlockedRequests: atomic.LoadInt64(&np.Stats.BlockedRequests),
		TotalBytes:      atomic.LoadInt64(&np.Stats.TotalBytes),
		UniqueHosts:     hosts,
	}
}

// GetLog returns a copy of all proxy log entries.
func (np *NetworkProxy) GetLog() []ProxyLogEntry {
	np.mu.RLock()
	defer np.mu.RUnlock()

	entries := make([]ProxyLogEntry, len(np.Log))
	copy(entries, np.Log)
	return entries
}

// handleConnect handles HTTPS CONNECT tunneling requests.
func (np *NetworkProxy) handleConnect(w http.ResponseWriter, r *http.Request) {
	host := r.Host
	if host == "" {
		host = r.URL.Host
	}

	allowed := np.IsAllowed(host)
	np.recordRequest(host, r.Method, allowed)

	if !allowed {
		http.Error(w, "Forbidden: domain not allowed", http.StatusForbidden)
		return
	}

	// Dial the target.
	targetConn, err := net.DialTimeout("tcp", host, 10*time.Second)
	if err != nil {
		http.Error(w, fmt.Sprintf("Failed to connect to %s: %v", host, err), http.StatusBadGateway)
		return
	}

	// Hijack the client connection.
	hijacker, ok := w.(http.Hijacker)
	if !ok {
		_ = targetConn.Close()
		http.Error(w, "Hijacking not supported", http.StatusInternalServerError)
		return
	}

	clientConn, _, err := hijacker.Hijack()
	if err != nil {
		_ = targetConn.Close()
		http.Error(w, fmt.Sprintf("Hijack failed: %v", err), http.StatusInternalServerError)
		return
	}

	// Send 200 OK to client.
	_, _ = clientConn.Write([]byte("HTTP/1.1 200 Connection Established\r\n\r\n"))

	// Bridge the streams.
	go func() {
		n, _ := io.Copy(targetConn, clientConn)
		atomic.AddInt64(&np.Stats.TotalBytes, n)
		_ = targetConn.Close()
	}()
	go func() {
		n, _ := io.Copy(clientConn, targetConn)
		atomic.AddInt64(&np.Stats.TotalBytes, n)
		_ = clientConn.Close()
	}()
}

// handleHTTP handles plain HTTP proxy requests.
func (np *NetworkProxy) handleHTTP(w http.ResponseWriter, r *http.Request) {
	host := r.Host
	if host == "" {
		host = r.URL.Host
	}

	allowed := np.IsAllowed(host)
	np.recordRequest(host, r.Method, allowed)

	if !allowed {
		http.Error(w, "Forbidden: domain not allowed", http.StatusForbidden)
		return
	}

	// Forward the request.
	outReq, err := http.NewRequestWithContext(r.Context(), r.Method, r.URL.String(), r.Body)
	if err != nil {
		http.Error(w, fmt.Sprintf("Failed to create request: %v", err), http.StatusInternalServerError)
		return
	}

	// Copy headers.
	for key, values := range r.Header {
		for _, value := range values {
			outReq.Header.Add(key, value)
		}
	}

	client := &http.Client{
		Timeout: 30 * time.Second,
		// Don't follow redirects — let the caller handle them.
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			return http.ErrUseLastResponse
		},
	}

	resp, err := client.Do(outReq)
	if err != nil {
		http.Error(w, fmt.Sprintf("Failed to forward request: %v", err), http.StatusBadGateway)
		return
	}
		defer func() { _ = resp.Body.Close() }()

	// Copy response headers.
	for key, values := range resp.Header {
		for _, value := range values {
			w.Header().Add(key, value)
		}
	}
	w.WriteHeader(resp.StatusCode)

	n, _ := io.Copy(w, resp.Body)
	atomic.AddInt64(&np.Stats.TotalBytes, n)
}

// recordRequest updates stats and log for a request.
func (np *NetworkProxy) recordRequest(host, method string, allowed bool) {
	// Strip port for stats.
	h, _, err := net.SplitHostPort(host)
	if err != nil {
		h = host
	}

	if allowed {
		atomic.AddInt64(&np.Stats.AllowedRequests, 1)
	} else {
		atomic.AddInt64(&np.Stats.BlockedRequests, 1)
	}

	np.mu.Lock()
	defer np.mu.Unlock()

	if np.Stats.UniqueHosts == nil {
		np.Stats.UniqueHosts = make(map[string]int)
	}
	np.Stats.UniqueHosts[h]++

	if np.config.LogRequests {
		reason := "allowed by policy"
		if !allowed {
			reason = "blocked by policy"
		}
		np.Log = append(np.Log, ProxyLogEntry{
			Timestamp: time.Now(),
			Host:      h,
			Method:    method,
			Allowed:   allowed,
			Reason:    reason,
		})
	}
}

// domainMatch checks if a host matches a domain pattern.
// Supports exact match, wildcard prefix (*.example.com), and suffix (.example.com).
func domainMatch(pattern, host string) bool {
	// Normalize: lowercase both.
	pattern = strings.ToLower(pattern)
	host = strings.ToLower(host)

	// Exact match.
	if pattern == host {
		return true
	}

	// Wildcard prefix: *.example.com matches sub.example.com
	if strings.HasPrefix(pattern, "*.") {
		suffix := pattern[1:] // ".example.com"
		if strings.HasSuffix(host, suffix) {
			return true
		}
		// Also match the base domain itself (e.g., *.github.com matches github.com).
		base := pattern[2:] // "example.com"
		if host == base {
			return true
		}
	}

	// Suffix match: .example.com matches sub.example.com
	if strings.HasPrefix(pattern, ".") {
		if strings.HasSuffix(host, pattern) {
			return true
		}
	}

	// IP-style wildcard: 169.254.* matches 169.254.1.1
	if strings.Contains(pattern, "*") && !strings.HasPrefix(pattern, "*.") {
		// Convert glob pattern to a prefix check.
		prefix := strings.TrimSuffix(pattern, "*")
		if strings.HasPrefix(host, prefix) {
			return true
		}
	}

	return false
}

// DefaultDevelopmentConfig returns a proxy config suitable for development
// that allows common package registries and blocks internal networks.
func DefaultDevelopmentConfig() ProxyConfig {
	return ProxyConfig{
		AllowedDomains: []string{
			"*.github.com",
			"*.golang.org",
			"*.npmjs.org",
			"*.pypi.org",
			"*.crates.io",
			"*.rubygems.org",
			"registry.*.io",
			"*.docker.io",
			"*.googleapis.com",
		},
		BlockedDomains: []string{
			"*.internal",
			"169.254.*",
			"10.*",
			"192.168.*",
		},
		Mode:        "allowlist",
		LogRequests: true,
	}
}
