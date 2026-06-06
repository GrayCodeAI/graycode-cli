package daemon

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"
	"time"
)

// Compile-time assertions that all three gateways satisfy Gateway.
var (
	_ Gateway = (*TelegramGateway)(nil)
	_ Gateway = (*DiscordGateway)(nil)
	_ Gateway = (*SlackGateway)(nil)
)

func TestGatewayManager_ConfigGating(t *testing.T) {
	tests := []struct {
		name     string
		cfg      GatewaysConfig
		wantSet  map[string]bool // gateway name -> expected present
		wantSize int
	}{
		{
			name:     "all disabled",
			cfg:      GatewaysConfig{},
			wantSize: 0,
		},
		{
			name: "telegram enabled but no token is skipped",
			cfg: GatewaysConfig{
				Telegram: TelegramConfig{Enabled: true},
			},
			wantSize: 0,
		},
		{
			name: "telegram enabled with token",
			cfg: GatewaysConfig{
				Telegram: TelegramConfig{Enabled: true, Token: "t"},
			},
			wantSet:  map[string]bool{"telegram": true},
			wantSize: 1,
		},
		{
			name: "discord disabled even with token",
			cfg: GatewaysConfig{
				Discord: DiscordConfig{Enabled: false, Token: "d"},
			},
			wantSize: 0,
		},
		{
			name: "all three enabled and credentialed",
			cfg: GatewaysConfig{
				Telegram: TelegramConfig{Enabled: true, Token: "t"},
				Discord:  DiscordConfig{Enabled: true, Token: "d"},
				Slack:    SlackConfig{Enabled: true, SigningSecret: "s"},
			},
			wantSet:  map[string]bool{"telegram": true, "discord": true, "slack": true},
			wantSize: 3,
		},
		{
			name: "slack enabled without signing secret is skipped",
			cfg: GatewaysConfig{
				Slack: SlackConfig{Enabled: true, BotToken: "xoxb"},
			},
			wantSize: 0,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			// A real Server is required so Slack can register its route.
			srv := New(Config{Port: 0}, nil)
			m := newGatewayManager(tc.cfg, "http://127.0.0.1:1", "", srv)
			got := m.Gateways()
			if len(got) != tc.wantSize {
				t.Fatalf("got %d gateways, want %d", len(got), tc.wantSize)
			}
			present := map[string]bool{}
			for _, g := range got {
				present[g.Name()] = true
			}
			for name, want := range tc.wantSet {
				if present[name] != want {
					t.Errorf("gateway %q present=%v want=%v", name, present[name], want)
				}
			}
		})
	}
}

func TestGatewayManager_StartStopNoGateways(t *testing.T) {
	m := newGatewayManager(GatewaysConfig{}, "http://127.0.0.1:1", "", nil)
	// Should be a no-op and never block.
	m.Start(context.Background())
	m.Stop()
}

func TestAuthorizer_PairingAndAllowlist(t *testing.T) {
	tests := []struct {
		name        string
		pairingCode string
		seed        []string
		sender      string
		text        string
		wantIsPair  bool
		wantPairOK  bool
		wantAllowed bool // allowed() result AFTER tryPair
	}{
		{
			name:        "seeded allowlist is authorized",
			seed:        []string{"alice"},
			sender:      "alice",
			text:        "hello",
			wantAllowed: true,
		},
		{
			name:        "unknown sender rejected",
			pairingCode: "secret",
			sender:      "mallory",
			text:        "hello",
			wantAllowed: false,
		},
		{
			name:        "correct pairing code authorizes sender",
			pairingCode: "secret",
			sender:      "bob",
			text:        "/pair secret",
			wantIsPair:  true,
			wantPairOK:  true,
			wantAllowed: true,
		},
		{
			name:        "wrong pairing code rejected",
			pairingCode: "secret",
			sender:      "eve",
			text:        "/pair wrong",
			wantIsPair:  true,
			wantPairOK:  false,
			wantAllowed: false,
		},
		{
			name:        "pair command with empty configured code fails",
			pairingCode: "",
			sender:      "carol",
			text:        "/pair anything",
			wantIsPair:  true,
			wantPairOK:  false,
			wantAllowed: false,
		},
		{
			name:        "non-pair message is not a pair attempt",
			pairingCode: "secret",
			sender:      "dan",
			text:        "what is the weather",
			wantIsPair:  false,
			wantAllowed: false,
		},
		{
			name:        "empty sender never allowed",
			seed:        []string{""},
			sender:      "",
			text:        "hi",
			wantAllowed: false,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			a := newAuthorizer(tc.pairingCode, tc.seed)
			isPair, pairOK := a.tryPair(tc.sender, tc.text)
			if isPair != tc.wantIsPair {
				t.Errorf("isPair=%v want=%v", isPair, tc.wantIsPair)
			}
			if pairOK != tc.wantPairOK {
				t.Errorf("pairOK=%v want=%v", pairOK, tc.wantPairOK)
			}
			if got := a.allowed(tc.sender); got != tc.wantAllowed {
				t.Errorf("allowed=%v want=%v", got, tc.wantAllowed)
			}
		})
	}
}

func TestForwardToHawk(t *testing.T) {
	var gotAuth, gotPrompt string
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotAuth = r.Header.Get("Authorization")
		var body struct {
			Prompt string `json:"prompt"`
		}
		_ = decodeForTest(r, &body)
		gotPrompt = body.Prompt
		writeJSON(w, http.StatusOK, ChatResponse{Response: "pong"})
	}))
	defer ts.Close()

	reply, err := forwardToHawk(context.Background(), ts.Client(), ts.URL, "key123", "ping")
	if err != nil {
		t.Fatalf("forwardToHawk: %v", err)
	}
	if reply != "pong" {
		t.Errorf("reply=%q want pong", reply)
	}
	if gotAuth != "Bearer key123" {
		t.Errorf("auth header=%q want Bearer key123", gotAuth)
	}
	if gotPrompt != "ping" {
		t.Errorf("prompt=%q want ping", gotPrompt)
	}
}

// --- Slack signature verification ---

func slackSignature(secret, ts string, body []byte) string {
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write([]byte("v0:" + ts + ":" + string(body)))
	return "v0=" + hex.EncodeToString(mac.Sum(nil))
}

func TestSlackGateway_VerifySignature(t *testing.T) {
	const secret = "8f742231b10e8888abcd99yyyzzz85a5"
	body := []byte(`{"type":"event_callback"}`)
	fixedNow := time.Unix(1_600_000_000, 0)

	tests := []struct {
		name      string
		secret    string
		ts        string
		sig       string
		now       time.Time
		wantValid bool
	}{
		{
			name:      "valid signature",
			secret:    secret,
			ts:        strconv.FormatInt(fixedNow.Unix(), 10),
			sig:       slackSignature(secret, strconv.FormatInt(fixedNow.Unix(), 10), body),
			now:       fixedNow,
			wantValid: true,
		},
		{
			name:      "wrong secret fails",
			secret:    secret,
			ts:        strconv.FormatInt(fixedNow.Unix(), 10),
			sig:       slackSignature("other-secret", strconv.FormatInt(fixedNow.Unix(), 10), body),
			now:       fixedNow,
			wantValid: false,
		},
		{
			name:      "stale timestamp rejected",
			secret:    secret,
			ts:        strconv.FormatInt(fixedNow.Add(-10*time.Minute).Unix(), 10),
			sig:       slackSignature(secret, strconv.FormatInt(fixedNow.Add(-10*time.Minute).Unix(), 10), body),
			now:       fixedNow,
			wantValid: false,
		},
		{
			name:      "missing signature header",
			secret:    secret,
			ts:        strconv.FormatInt(fixedNow.Unix(), 10),
			sig:       "",
			now:       fixedNow,
			wantValid: false,
		},
		{
			name:      "empty signing secret fails closed",
			secret:    "",
			ts:        strconv.FormatInt(fixedNow.Unix(), 10),
			sig:       slackSignature(secret, strconv.FormatInt(fixedNow.Unix(), 10), body),
			now:       fixedNow,
			wantValid: false,
		},
		{
			name:      "non-numeric timestamp",
			secret:    secret,
			ts:        "not-a-number",
			sig:       slackSignature(secret, "not-a-number", body),
			now:       fixedNow,
			wantValid: false,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			g := newSlackGateway(SlackConfig{SigningSecret: tc.secret}, "http://x", "", nil)
			g.now = func() time.Time { return tc.now }
			h := http.Header{}
			if tc.ts != "" {
				h.Set("X-Slack-Request-Timestamp", tc.ts)
			}
			if tc.sig != "" {
				h.Set("X-Slack-Signature", tc.sig)
			}
			if got := g.verifySignature(h, body); got != tc.wantValid {
				t.Errorf("verifySignature=%v want=%v", got, tc.wantValid)
			}
		})
	}
}

func TestSlackGateway_URLVerificationHandshake(t *testing.T) {
	const secret = "test-secret"
	srv := New(Config{Port: 0}, nil)
	g := newSlackGateway(SlackConfig{SigningSecret: secret, Path: "/v1/slack/events"}, "http://x", "", srv)

	body := []byte(`{"type":"url_verification","challenge":"abc123"}`)
	ts := strconv.FormatInt(time.Now().Unix(), 10)
	req := httptest.NewRequest(http.MethodPost, "/v1/slack/events", bytesReader(body))
	req.Header.Set("X-Slack-Request-Timestamp", ts)
	req.Header.Set("X-Slack-Signature", slackSignature(secret, ts, body))

	rec := httptest.NewRecorder()
	g.handleEvents(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status=%d want 200", rec.Code)
	}
	var resp map[string]string
	_ = decodeJSON(rec.Body.Bytes(), &resp)
	if resp["challenge"] != "abc123" {
		t.Errorf("challenge=%q want abc123", resp["challenge"])
	}
}

func TestSlackGateway_RejectsBadSignature(t *testing.T) {
	g := newSlackGateway(SlackConfig{SigningSecret: "secret"}, "http://x", "", nil)
	body := []byte(`{"type":"event_callback"}`)
	req := httptest.NewRequest(http.MethodPost, "/v1/slack/events", bytesReader(body))
	req.Header.Set("X-Slack-Request-Timestamp", strconv.FormatInt(time.Now().Unix(), 10))
	req.Header.Set("X-Slack-Signature", "v0=deadbeef")

	rec := httptest.NewRecorder()
	g.handleEvents(rec, req)
	if rec.Code != http.StatusUnauthorized {
		t.Errorf("status=%d want 401", rec.Code)
	}
}

func TestDiscordGateway_HandleMessage_FlowsThroughAllowlist(t *testing.T) {
	var posted []string
	// Mock Discord REST for postMessage.
	mock := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body struct {
			Content string `json:"content"`
		}
		_ = decodeForTest(r, &body)
		posted = append(posted, body.Content)
		w.WriteHeader(http.StatusOK)
	}))
	defer mock.Close()
	old := discordAPIBase
	discordAPIBase = mock.URL
	defer func() { discordAPIBase = old }()

	g := newDiscordGateway(DiscordConfig{Token: "bot", AppID: "BOT", PairingCode: "code"}, "http://x", "")

	// Unauthorized non-pair mention -> "Unauthorized" reply.
	g.handleMessage(context.Background(), discordMessage{
		ID: "1", ChannelID: "C", Content: "<@BOT> hello", GuildID: "G",
		Author:   discordUser{ID: "u1"},
		Mentions: []discordUser{{ID: "BOT"}},
	})
	// Pair, then it should be authorized.
	g.handleMessage(context.Background(), discordMessage{
		ID: "2", ChannelID: "C", Content: "<@BOT> /pair code", GuildID: "G",
		Author:   discordUser{ID: "u1"},
		Mentions: []discordUser{{ID: "BOT"}},
	})

	if len(posted) < 2 {
		t.Fatalf("expected at least 2 posts, got %d: %v", len(posted), posted)
	}
	if posted[0] == "" || posted[1] == "" {
		t.Errorf("unexpected empty replies: %v", posted)
	}
	if !g.auth.allowed("u1") {
		t.Errorf("u1 should be allowed after pairing")
	}
}

func TestDiscordGateway_IgnoresBotsAndNonMentions(t *testing.T) {
	g := newDiscordGateway(DiscordConfig{Token: "bot", AppID: "BOT"}, "http://x", "")
	// Bot author -> ignored (no panic, no post attempt path beyond guard).
	g.handleMessage(context.Background(), discordMessage{
		ID: "1", ChannelID: "C", GuildID: "G", Author: discordUser{ID: "x", Bot: true},
	})
	// Guild message without mention -> ignored.
	g.handleMessage(context.Background(), discordMessage{
		ID: "2", ChannelID: "C", GuildID: "G", Content: "no mention", Author: discordUser{ID: "y"},
	})
	if g.auth.allowed("x") || g.auth.allowed("y") {
		t.Errorf("no sender should be allowed")
	}
}

// --- small helpers ---

func bytesReader(b []byte) io.Reader { return bytes.NewReader(b) }

func decodeForTest(r *http.Request, dst any) error {
	return json.NewDecoder(r.Body).Decode(dst)
}

func decodeJSON(b []byte, dst any) error {
	return json.Unmarshal(b, dst)
}
