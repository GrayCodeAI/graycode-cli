package daemon

import (
	"context"
	"crypto/subtle"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"strings"
	"sync"
)

// forwardToGraycode posts a prompt to the daemon's /v1/chat endpoint and returns the
// assistant reply. daemonAddr must include a scheme (e.g. "http://127.0.0.1:4590").
// apiKey, when non-empty, is sent as a Bearer token. Shared by the Discord and
// Slack gateways; Telegram keeps its own method for backwards compatibility.
func forwardToGraycode(ctx context.Context, client *http.Client, daemonAddr, apiKey, prompt string) (string, error) {
	payload, _ := json.Marshal(map[string]string{"prompt": prompt})
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, daemonAddr+"/v1/chat", strings.NewReader(string(payload)))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/json")
	if apiKey != "" {
		req.Header.Set("Authorization", "Bearer "+apiKey)
	}
	resp, err := client.Do(req)
	if err != nil {
		return "", err
	}
	defer func() { _ = resp.Body.Close() }()
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	var chatResp struct {
		Response string `json:"response"`
	}
	if err := json.Unmarshal(body, &chatResp); err != nil {
		return string(body), nil
	}
	return chatResp.Response, nil
}

// Gateway is a bidirectional messaging bridge between an external chat platform
// (Telegram, Discord, Slack, ...) and the graycode daemon. Implementations forward
// inbound messages to the daemon's /v1/chat endpoint and relay the reply back.
type Gateway interface {
	// Name returns a short identifier for the gateway (e.g. "telegram").
	Name() string
	// Start begins processing messages. It should block until ctx is cancelled
	// (long-poll gateways) or return promptly after registering handlers
	// (webhook gateways). Implementations must respect ctx cancellation.
	Start(ctx context.Context) error
	// Stop releases any resources held by the gateway. It is safe to call Stop
	// even if Start was never called or already returned.
	Stop() error
}

// GatewaysConfig groups the per-platform messaging bridge configuration.
// All gateways are disabled by default; a gateway only starts when its Enabled
// flag is set and its required credentials are present.
type GatewaysConfig struct {
	Telegram TelegramConfig `json:"telegram,omitempty"`
	Discord  DiscordConfig  `json:"discord,omitempty"`
	Slack    SlackConfig    `json:"slack,omitempty"`
}

// TelegramConfig configures the Telegram long-poll gateway.
type TelegramConfig struct {
	Enabled bool   `json:"enabled,omitempty"`
	Token   string `json:"token,omitempty"`
	// PairingCode, when non-empty, requires senders to issue "/pair <code>"
	// once before their sender ID is added to the allowlist.
	PairingCode string `json:"pairing_code,omitempty"`
	// AllowList is an optional set of pre-authorized sender IDs (Telegram usernames
	// or numeric chat IDs as strings). When both PairingCode and AllowList are
	// empty, the gateway refuses all senders (fail closed).
	AllowList []string `json:"allow_list,omitempty"`
}

// DiscordConfig configures the Discord Gateway (WebSocket) bridge. The bot's own
// user ID is learned from the Gateway READY event, so no application/channel IDs
// need to be configured; the bot responds to DMs and to @mentions in any guild
// channel it can see.
type DiscordConfig struct {
	Enabled     bool     `json:"enabled,omitempty"`
	Token       string   `json:"token,omitempty"` // bot token
	PairingCode string   `json:"pairing_code,omitempty"`
	AllowList   []string `json:"allow_list,omitempty"`
}

// SlackConfig configures the Slack Events API gateway.
type SlackConfig struct {
	Enabled       bool   `json:"enabled,omitempty"`
	SigningSecret string `json:"signing_secret,omitempty"`
	BotToken      string `json:"bot_token,omitempty"` // xoxb- token for chat.postMessage
	// Path is the HTTP route registered on the daemon mux for the Events API
	// webhook. Defaults to "/v1/slack/events" when empty.
	Path        string   `json:"path,omitempty"`
	PairingCode string   `json:"pairing_code,omitempty"`
	AllowList   []string `json:"allow_list,omitempty"`
}

// authorizer enforces a pairing-code + allowlist policy shared by all gateways.
// It is safe for concurrent use.
type authorizer struct {
	mu          sync.RWMutex
	pairingCode string
	allow       map[string]struct{}
}

// newAuthorizer builds an authorizer from a pairing code and a seed allowlist.
func newAuthorizer(pairingCode string, allow []string) *authorizer {
	m := make(map[string]struct{}, len(allow))
	for _, id := range allow {
		if id = strings.TrimSpace(id); id != "" {
			m[id] = struct{}{}
		}
	}
	return &authorizer{pairingCode: pairingCode, allow: m}
}

// allowed reports whether the sender ID is currently authorized.
func (a *authorizer) allowed(senderID string) bool {
	if senderID == "" {
		return false
	}
	a.mu.RLock()
	defer a.mu.RUnlock()
	_, ok := a.allow[senderID]
	return ok
}

// tryPair inspects a message for a "/pair <code>" command. If the command is
// present and the code matches the configured pairing code, the sender is added
// to the allowlist and (true, true) is returned. If it is a pair attempt with a
// wrong code, (true, false) is returned. If the message is not a pair command,
// (false, false) is returned.
func (a *authorizer) tryPair(senderID, text string) (isPair, ok bool) {
	fields := strings.Fields(strings.TrimSpace(text))
	if len(fields) == 0 || fields[0] != "/pair" {
		return false, false
	}
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.pairingCode == "" {
		return true, false
	}
	var supplied string
	if len(fields) > 1 {
		supplied = fields[1]
	}
	// Constant-time compare so the pairing code is not exposed to a timing
	// oracle.
	if subtle.ConstantTimeCompare([]byte(supplied), []byte(a.pairingCode)) != 1 {
		return true, false
	}
	a.allow[senderID] = struct{}{}
	return true, true
}

// asyncDispatcher runs message handlers in bounded goroutines and tracks them so
// a gateway can drain in-flight work on shutdown. It replaces ad-hoc `go f()`
// spawns that had no concurrency limit and were not tied to the gateway lifecycle.
type asyncDispatcher struct {
	sem chan struct{}
	wg  sync.WaitGroup
}

// newAsyncDispatcher builds a dispatcher allowing at most max concurrent handlers.
func newAsyncDispatcher(max int) *asyncDispatcher {
	if max <= 0 {
		max = 8
	}
	return &asyncDispatcher{sem: make(chan struct{}, max)}
}

// run executes fn in a bounded goroutine unless ctx is already done. It blocks
// only to acquire a concurrency slot (respecting ctx cancellation), then runs fn
// asynchronously. Handlers are tracked so wait can drain them on shutdown.
func (d *asyncDispatcher) run(ctx context.Context, fn func()) {
	select {
	case <-ctx.Done():
		return
	case d.sem <- struct{}{}:
	}
	d.wg.Add(1)
	go func() {
		defer d.wg.Done()
		defer func() { <-d.sem }()
		fn()
	}()
}

// wait blocks until all dispatched handlers have finished.
func (d *asyncDispatcher) wait() { d.wg.Wait() }

// daemonURLSetter is implemented by poll/forward gateways whose forward target is
// only known once the daemon's real listening address resolves (port 0 at Start).
type daemonURLSetter interface {
	setDaemonURL(url string)
}

// gatewayManager owns the lifecycle of all enabled gateways for a daemon.
type gatewayManager struct {
	mu       sync.Mutex
	gateways []Gateway
	cancel   context.CancelFunc
	wg       sync.WaitGroup
	started  bool
}

// newGatewayManager builds the set of enabled gateways from cfg. Gateways whose
// Enabled flag is false or whose required credentials are missing are skipped,
// so a daemon with no gateway configuration produces an empty (but valid) manager.
func newGatewayManager(cfg GatewaysConfig, daemonAddr, apiKey string, s *Server) *gatewayManager {
	m := &gatewayManager{}
	if cfg.Telegram.Enabled && cfg.Telegram.Token != "" {
		m.gateways = append(m.gateways, newTelegramGatewayFromConfig(cfg.Telegram, daemonAddr, apiKey))
	}
	if cfg.Discord.Enabled && cfg.Discord.Token != "" {
		m.gateways = append(m.gateways, newDiscordGateway(cfg.Discord, daemonAddr, apiKey))
	}
	if cfg.Slack.Enabled && cfg.Slack.SigningSecret != "" {
		m.gateways = append(m.gateways, newSlackGateway(cfg.Slack, daemonAddr, apiKey, s))
	}
	return m
}

// setDaemonURL updates the forward target for poll-based gateways once the
// daemon's real listening address is known (port 0 resolves at Start time).
// Webhook gateways (Slack) ignore this as they reply via platform APIs.
func (m *gatewayManager) setDaemonURL(url string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	for _, g := range m.gateways {
		if s, ok := g.(daemonURLSetter); ok {
			s.setDaemonURL(url)
		}
	}
}

// Gateways returns the configured (enabled) gateways. Useful for tests/inspection.
func (m *gatewayManager) Gateways() []Gateway {
	m.mu.Lock()
	defer m.mu.Unlock()
	out := make([]Gateway, len(m.gateways))
	copy(out, m.gateways)
	return out
}

// Start launches every enabled gateway in its own goroutine. It returns
// immediately; gateways run until Stop is called or the parent ctx is cancelled.
func (m *gatewayManager) Start(ctx context.Context) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.started || len(m.gateways) == 0 {
		return
	}
	m.started = true
	ctx, m.cancel = context.WithCancel(ctx)
	for _, g := range m.gateways {
		g := g
		m.wg.Add(1)
		go func() {
			defer m.wg.Done()
			slog.Info("gateway starting", "gateway", g.Name())
			if err := g.Start(ctx); err != nil && ctx.Err() == nil {
				slog.Error("gateway exited", "gateway", g.Name(), "error", err)
			}
		}()
	}
}

// Stop cancels all gateways and waits for them to finish.
func (m *gatewayManager) Stop() {
	m.mu.Lock()
	if !m.started {
		m.mu.Unlock()
		return
	}
	cancel := m.cancel
	gws := append([]Gateway(nil), m.gateways...)
	m.mu.Unlock()

	if cancel != nil {
		cancel()
	}
	for _, g := range gws {
		_ = g.Stop()
	}
	m.wg.Wait()
}
