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
}

// TelegramUpdate represents an incoming Telegram message.
type TelegramUpdate struct {
	UpdateID int `json:"update_id"`
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
	defer resp.Body.Close()

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
	// Forward to hawk daemon
	response, err := tg.forwardToHawk(ctx, msg.Text)
	if err != nil {
		response = fmt.Sprintf("Error: %v", err)
	}

	// Format for Telegram (truncate if too long)
	if len(response) > 4000 {
		response = response[:4000] + "\n\n... (truncated)"
	}

	_ = tg.sendMessage(ctx, msg.Chat.ID, response)
}

func (tg *TelegramGateway) forwardToHawk(ctx context.Context, prompt string) (string, error) {
	payload := fmt.Sprintf(`{"prompt":%q}`, prompt)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, tg.DaemonAddr+"/v1/chat", strings.NewReader(payload))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := tg.client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
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
	resp.Body.Close()
	return nil
}
