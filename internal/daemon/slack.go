package daemon

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"
)

// slackAPIBase is the Slack Web API root. Package var so tests can redirect it.
var slackAPIBase = "https://slack.com/api"

// defaultSlackPath is the route registered for the Events API webhook.
const defaultSlackPath = "/v1/slack/events"

// slackMaxSkew bounds the allowed clock skew for the request timestamp to defend
// against replay attacks.
const slackMaxSkew = 5 * time.Minute

// SlackGateway implements the Slack Events API bridge. Unlike Telegram/Discord it
// is webhook-driven: Start registers an HTTP handler on the daemon mux and returns
// after blocking on ctx (so the gatewayManager lifecycle is uniform). Inbound
// app_mention events are verified against the signing secret, forwarded to the
// daemon, and answered as threaded replies.
type SlackGateway struct {
	cfg        SlackConfig
	daemonAddr string
	apiKey     string
	client     *http.Client
	auth       *authorizer
	server     *Server
	path       string
	dispatch   *asyncDispatcher

	// mu guards runCtx, which is set to the Start context so webhook-triggered
	// handlers run under (and are cancelled by) the gateway lifecycle rather than
	// a detached context.Background().
	mu     sync.Mutex
	runCtx context.Context

	// now is overridable for deterministic signature-skew tests.
	now func() time.Time
}

// newSlackGateway builds a Slack gateway and registers its webhook route.
func newSlackGateway(cfg SlackConfig, daemonAddr, apiKey string, s *Server) *SlackGateway {
	path := cfg.Path
	if path == "" {
		path = defaultSlackPath
	}
	g := &SlackGateway{
		cfg:        cfg,
		daemonAddr: daemonAddr,
		apiKey:     apiKey,
		client:     &http.Client{Timeout: 30 * time.Second},
		auth:       newAuthorizer(cfg.PairingCode, cfg.AllowList),
		server:     s,
		path:       path,
		dispatch:   newAsyncDispatcher(8),
		runCtx:     context.Background(),
		now:        time.Now,
	}
	if s != nil {
		s.mux.HandleFunc("POST "+path, g.handleEvents)
	}
	return g
}

// Name implements Gateway.
func (g *SlackGateway) Name() string { return "slack" }

// setDaemonURL implements daemonURLSetter. Slack forwards to the daemon via
// forwardToHawk, so it needs the resolved address even though it replies via the
// Slack Web API.
func (g *SlackGateway) setDaemonURL(url string) { g.daemonAddr = url }

// Start implements Gateway. The webhook is already registered at construction; we
// record the lifecycle context (so handlers are cancelled on shutdown), block
// until the daemon stops the gateway, then drain in-flight handlers.
func (g *SlackGateway) Start(ctx context.Context) error {
	g.mu.Lock()
	g.runCtx = ctx
	g.mu.Unlock()
	<-ctx.Done()
	g.dispatch.wait()
	return ctx.Err()
}

// Stop implements Gateway.
func (g *SlackGateway) Stop() error { return nil }

// handlerContext returns the current lifecycle context for webhook-triggered work.
func (g *SlackGateway) handlerContext() context.Context {
	g.mu.Lock()
	defer g.mu.Unlock()
	return g.runCtx
}

// slackEnvelope is the outer Events API payload.
type slackEnvelope struct {
	Type      string          `json:"type"`
	Challenge string          `json:"challenge,omitempty"`
	Event     slackEventInner `json:"event,omitempty"`
}

type slackEventInner struct {
	Type     string `json:"type"`
	User     string `json:"user"`
	Text     string `json:"text"`
	Channel  string `json:"channel"`
	TS       string `json:"ts"`
	ThreadTS string `json:"thread_ts,omitempty"`
	BotID    string `json:"bot_id,omitempty"`
}

func (g *SlackGateway) handleEvents(w http.ResponseWriter, r *http.Request) {
	body, err := io.ReadAll(io.LimitReader(r.Body, maxRequestBodyBytes))
	if err != nil {
		http.Error(w, "read error", http.StatusBadRequest)
		return
	}

	if !g.verifySignature(r.Header, body) {
		http.Error(w, "invalid signature", http.StatusUnauthorized)
		return
	}

	var env slackEnvelope
	if err := json.Unmarshal(body, &env); err != nil {
		http.Error(w, "invalid JSON", http.StatusBadRequest)
		return
	}

	// URL verification handshake.
	if env.Type == "url_verification" {
		writeJSON(w, http.StatusOK, map[string]string{"challenge": env.Challenge})
		return
	}

	// Acknowledge immediately; process asynchronously so Slack does not retry.
	w.WriteHeader(http.StatusOK)

	if env.Type == "event_callback" && env.Event.Type == "app_mention" && env.Event.BotID == "" {
		ctx := g.handlerContext()
		ev := env.Event
		g.dispatch.run(ctx, func() { g.handleMention(ctx, ev) })
	}
}

func (g *SlackGateway) handleMention(ctx context.Context, ev slackEventInner) {
	text := stripSlackMention(ev.Text)
	threadTS := ev.ThreadTS
	if threadTS == "" {
		threadTS = ev.TS // reply in a thread rooted at the mention
	}

	if isPair, ok := g.auth.tryPair(ev.User, text); isPair {
		if ok {
			g.reply(ctx, ev.Channel, threadTS, "Paired. You can now chat with hawk.")
		} else {
			g.reply(ctx, ev.Channel, threadTS, "Pairing failed: invalid code.")
		}
		return
	}
	if !g.auth.allowed(ev.User) {
		g.reply(ctx, ev.Channel, threadTS, "Unauthorized. Send /pair <code> to authorize.")
		return
	}

	reply, err := forwardToHawk(ctx, g.client, g.daemonAddr, g.apiKey, text)
	if err != nil {
		reply = fmt.Sprintf("Error: %v", err)
	}
	g.reply(ctx, ev.Channel, threadTS, reply)
}

// reply posts a threaded message and logs (rather than swallows) any failure.
func (g *SlackGateway) reply(ctx context.Context, channel, threadTS, text string) {
	if err := g.postMessage(ctx, channel, threadTS, text); err != nil {
		slog.Error("slack postMessage failed", "channel", channel, "error", err)
	}
}

// stripSlackMention removes a leading <@U...> mention from the message text.
func stripSlackMention(text string) string {
	text = strings.TrimSpace(text)
	if strings.HasPrefix(text, "<@") {
		if i := strings.Index(text, ">"); i >= 0 {
			return strings.TrimSpace(text[i+1:])
		}
	}
	return text
}

// verifySignature implements Slack's v0 HMAC-SHA256 request signing scheme:
// base = "v0:{timestamp}:{body}", sig = "v0=" + hex(HMAC_SHA256(secret, base)).
func (g *SlackGateway) verifySignature(h http.Header, body []byte) bool {
	if g.cfg.SigningSecret == "" {
		return false // fail closed
	}
	tsStr := h.Get("X-Slack-Request-Timestamp")
	sig := h.Get("X-Slack-Signature")
	if tsStr == "" || sig == "" {
		return false
	}
	ts, err := strconv.ParseInt(tsStr, 10, 64)
	if err != nil {
		return false
	}
	// Reject stale/future timestamps (replay protection).
	now := g.now().Unix()
	if diff := now - ts; diff > int64(slackMaxSkew/time.Second) || diff < -int64(slackMaxSkew/time.Second) {
		return false
	}

	mac := hmac.New(sha256.New, []byte(g.cfg.SigningSecret))
	var base bytes.Buffer
	base.WriteString("v0:")
	base.WriteString(tsStr)
	base.WriteByte(':')
	base.Write(body)
	mac.Write(base.Bytes())
	expected := "v0=" + hex.EncodeToString(mac.Sum(nil))
	return subtle.ConstantTimeCompare([]byte(expected), []byte(sig)) == 1
}

// postMessage posts a threaded reply via Slack chat.postMessage.
func (g *SlackGateway) postMessage(ctx context.Context, channel, threadTS, text string) error {
	if g.cfg.BotToken == "" || channel == "" {
		return nil
	}
	payload, _ := json.Marshal(map[string]string{
		"channel":   channel,
		"text":      text,
		"thread_ts": threadTS,
	})
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, slackAPIBase+"/chat.postMessage", bytes.NewReader(payload))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json; charset=utf-8")
	req.Header.Set("Authorization", "Bearer "+g.cfg.BotToken)
	resp, err := g.client.Do(req)
	if err != nil {
		return err
	}
	defer func() { _ = resp.Body.Close() }()
	// Slack returns HTTP 200 with {"ok":false,"error":"..."} on logical failures
	// (bad token, not_in_channel, ...), so the status code alone is insufficient.
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
	var result struct {
		OK    bool   `json:"ok"`
		Error string `json:"error"`
	}
	if err := json.Unmarshal(body, &result); err != nil {
		if resp.StatusCode/100 != 2 {
			return fmt.Errorf("slack chat.postMessage: HTTP %d", resp.StatusCode)
		}
		return nil
	}
	if !result.OK {
		return fmt.Errorf("slack chat.postMessage: %s", result.Error)
	}
	return nil
}
