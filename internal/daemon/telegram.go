// Package daemon provides a Telegram gateway for hawk.
// Allows users to interact with hawk via Telegram bot messages.
package daemon

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// TelegramGateway connects hawk to a Telegram bot.
type TelegramGateway struct {
	Token      string
	DaemonAddr string // hawk daemon address to forward messages to
	client     *http.Client
	offset     int

	// apiKey, when set, is sent as a Bearer token on forwarded daemon requests.
	apiKey string
	// auth, when set, enforces a pairing-code/allowlist policy. When nil, all
	// senders are permitted (legacy behaviour for the bare constructor).
	auth *authorizer
}

// TelegramUpdate represents an incoming Telegram message.
type TelegramUpdate struct {
	UpdateID int              `json:"update_id"`
	Message  *TelegramMessage `json:"message,omitempty"`
}

// TelegramMessage is a Telegram chat message.
type TelegramMessage struct {
	MessageID int    `json:"message_id"`
	Text      string `json:"text"`
	Chat      struct {
		ID int64 `json:"id"`
	} `json:"chat"`
	From struct {
		Username string `json:"username"`
	} `json:"from"`
}

// NewTelegramGateway creates a gateway with the given bot token.
func NewTelegramGateway(token, daemonAddr string) *TelegramGateway {
	return &TelegramGateway{
		Token:      token,
		DaemonAddr: daemonAddr,
		client:     &http.Client{Timeout: 30 * time.Second},
	}
}

// newTelegramGatewayFromConfig builds a gateway from settings, wiring the
// pairing-code/allowlist authorizer and the daemon API key.
func newTelegramGatewayFromConfig(cfg TelegramConfig, daemonAddr, apiKey string) *TelegramGateway {
	tg := NewTelegramGateway(cfg.Token, daemonAddr)
	tg.apiKey = apiKey
	tg.auth = newAuthorizer(cfg.PairingCode, cfg.AllowList)
	return tg
}

// Name implements Gateway.
func (tg *TelegramGateway) Name() string { return "telegram" }

// Start implements Gateway by delegating to the long-poll Run loop.
func (tg *TelegramGateway) Start(ctx context.Context) error { return tg.Run(ctx) }

// Stop implements Gateway. The long-poll loop is driven by the context passed to
// Start, so there is nothing to release here.
func (tg *TelegramGateway) Stop() error { return nil }

// senderID derives a stable allowlist key for a message: the username if present,
// otherwise the numeric chat ID.
func telegramSenderID(msg *TelegramMessage) string {
	if msg.From.Username != "" {
		return msg.From.Username
	}
	return fmt.Sprintf("%d", msg.Chat.ID)
}

// Run starts the long-polling loop. Blocks until context is cancelled.
func (tg *TelegramGateway) Run(ctx context.Context) error {
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		updates, err := tg.getUpdates(ctx)
		if err != nil {
			time.Sleep(5 * time.Second)
			continue
		}

		for _, u := range updates {
			tg.offset = u.UpdateID + 1
			if u.Message == nil || u.Message.Text == "" {
				continue
			}
			go tg.handleMessage(ctx, u.Message)
		}
	}
}

func (tg *TelegramGateway) getUpdates(ctx context.Context) ([]TelegramUpdate, error) {
	apiURL := fmt.Sprintf("https://api.telegram.org/bot%s/getUpdates?offset=%d&timeout=25", tg.Token, tg.offset)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, apiURL, nil)
	if err != nil {
		return nil, err
	}
	resp, err := tg.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer func() { _ = resp.Body.Close() }()

	var result struct {
		OK     bool             `json:"ok"`
		Result []TelegramUpdate `json:"result"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, err
	}
	return result.Result, nil
}

func (tg *TelegramGateway) handleMessage(ctx context.Context, msg *TelegramMessage) {
	if tg.auth != nil {
		sender := telegramSenderID(msg)
		if isPair, ok := tg.auth.tryPair(sender, msg.Text); isPair {
			if ok {
				_ = tg.sendMessage(ctx, msg.Chat.ID, "Paired. You can now chat with hawk.")
			} else {
				_ = tg.sendMessage(ctx, msg.Chat.ID, "Pairing failed: invalid code.")
			}
			return
		}
		if !tg.auth.allowed(sender) {
			_ = tg.sendMessage(ctx, msg.Chat.ID, "Unauthorized. Send /pair <code> to authorize.")
			return
		}
	}

	// Forward to hawk daemon
	response, err := tg.forwardToHawk(ctx, msg.Text)
	if err != nil {
		response = fmt.Sprintf("Error: %v", err)
	}

	// Format for Telegram (truncate if too long, at rune boundary)
	if len([]rune(response)) > 4000 {
		response = string([]rune(response)[:4000]) + "\n\n... (truncated)"
	}

	_ = tg.sendMessage(ctx, msg.Chat.ID, response)
}

func (tg *TelegramGateway) forwardToHawk(ctx context.Context, prompt string) (string, error) {
	// Use json.Marshal for safe JSON encoding instead of fmt.Sprintf
	// with %q, which does not handle all JSON edge cases (e.g., control
	// characters, surrogate pairs).
	payload, err := json.Marshal(map[string]string{"prompt": prompt})
	if err != nil {
		return "", fmt.Errorf("encode prompt: %w", err)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, tg.DaemonAddr+"/v1/chat", strings.NewReader(string(payload)))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/json")
	if tg.apiKey != "" {
		req.Header.Set("Authorization", "Bearer "+tg.apiKey)
	}

	resp, err := tg.client.Do(req)
	if err != nil {
		return "", err
	}
	defer func() { _ = resp.Body.Close() }()

	// Limit response body to 1 MiB to prevent memory exhaustion.
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	var chatResp struct {
		Response string `json:"response"`
	}
	if err := json.Unmarshal(body, &chatResp); err != nil {
		return string(body), nil
	}
	return chatResp.Response, nil
}

func (tg *TelegramGateway) sendMessage(ctx context.Context, chatID int64, text string) error {
	apiURL := fmt.Sprintf("https://api.telegram.org/bot%s/sendMessage", tg.Token)
	data := url.Values{
		"chat_id":    {fmt.Sprintf("%d", chatID)},
		"text":       {text},
		"parse_mode": {"Markdown"},
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, apiURL, strings.NewReader(data.Encode()))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	resp, err := tg.client.Do(req)
	if err != nil {
		return err
	}
	_ = resp.Body.Close()
	return nil
}
