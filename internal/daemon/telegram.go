// Package daemon provides a Telegram gateway for graycode.
// Allows users to interact with graycode via Telegram bot messages.
package daemon

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/GrayCodeAI/graycode-cli/internal/stt"
)

// TelegramGateway connects graycode to a Telegram bot.
type TelegramGateway struct {
	Token      string
	DaemonAddr string // graycode daemon address to forward messages to
	client     *http.Client
	offset     int

	// apiKey, when set, is sent as a Bearer token on forwarded daemon requests.
	apiKey string
	// auth enforces a pairing-code/allowlist policy. It is always non-nil; the
	// bare constructor seeds an empty (fail-closed) authorizer that refuses all
	// senders until a pairing code or allowlist is configured.
	auth     *authorizer
	dispatch *asyncDispatcher
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
	// Voice is set for Telegram voice notes; Audio for audio-file attachments.
	// When either is present and an STT transcriber is installed, the audio is
	// transcribed and the text is replaced with the transcript so the agent
	// can answer it.
	Voice *telegramAudio `json:"voice,omitempty"`
	Audio *telegramAudio `json:"audio,omitempty"`
}

// telegramAudio describes a Telegram voice/audio attachment.
type telegramAudio struct {
	FileID   string `json:"file_id"`
	Duration int    `json:"duration"`
	MimeType string `json:"mime_type"`
}

// NewTelegramGateway creates a gateway with the given bot token. The authorizer
// is seeded empty and therefore fails closed: no sender is authorized until a
// pairing code or allowlist is supplied (see newTelegramGatewayFromConfig).
func NewTelegramGateway(token, daemonAddr string) *TelegramGateway {
	return &TelegramGateway{
		Token:      token,
		DaemonAddr: daemonAddr,
		client:     &http.Client{Timeout: 30 * time.Second},
		auth:       newAuthorizer("", nil),
		dispatch:   newAsyncDispatcher(8),
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

// setDaemonURL implements daemonURLSetter.
func (tg *TelegramGateway) setDaemonURL(url string) { tg.DaemonAddr = url }

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

// Run starts the long-polling loop. Blocks until context is cancelled, then
// drains any in-flight message handlers before returning.
func (tg *TelegramGateway) Run(ctx context.Context) error {
	const (
		baseBackoff = time.Second
		maxBackoff  = 30 * time.Second
	)
	backoff := baseBackoff
	for {
		select {
		case <-ctx.Done():
			tg.dispatch.wait()
			return ctx.Err()
		default:
		}

		updates, err := tg.getUpdates(ctx)
		if err != nil {
			if ctx.Err() != nil {
				tg.dispatch.wait()
				return ctx.Err()
			}
			slog.Warn("telegram getUpdates failed; backing off", "backoff", backoff, "error", err)
			// Context-aware sleep with exponential backoff so a bad token or
			// outage does not hot-loop and Stop is honored promptly.
			select {
			case <-ctx.Done():
				tg.dispatch.wait()
				return ctx.Err()
			case <-time.After(backoff):
			}
			if backoff *= 2; backoff > maxBackoff {
				backoff = maxBackoff
			}
			continue
		}
		backoff = baseBackoff

		for _, u := range updates {
			tg.offset = u.UpdateID + 1
			if u.Message == nil || u.Message.Text == "" {
				continue
			}
			msg := u.Message
			tg.dispatch.run(ctx, func() { tg.handleMessage(ctx, msg) })
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
		OK          bool             `json:"ok"`
		Description string           `json:"description"`
		Result      []TelegramUpdate `json:"result"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, err
	}
	if !result.OK {
		return nil, fmt.Errorf("telegram getUpdates: %s", result.Description)
	}
	return result.Result, nil
}

func (tg *TelegramGateway) handleMessage(ctx context.Context, msg *TelegramMessage) {
	sender := telegramSenderID(msg)
	if isPair, ok := tg.auth.tryPair(sender, msg.Text); isPair {
		if ok {
			tg.reply(ctx, msg.Chat.ID, "Paired. You can now chat with graycode.")
		} else {
			tg.reply(ctx, msg.Chat.ID, "Pairing failed: invalid code.")
		}
		return
	}
	if !tg.auth.allowed(sender) {
		tg.reply(ctx, msg.Chat.ID, "Unauthorized. Send /pair <code> to authorize.")
		return
	}

	// Transcribe voice/audio attachments when an STT backend is installed.
	prompt := msg.Text
	if audio := msg.Voice; audio != nil {
		prompt = tg.transcribeAudio(ctx, msg.Chat.ID, audio)
		if prompt == "" {
			return // error already replied
		}
	} else if audio := msg.Audio; audio != nil {
		prompt = tg.transcribeAudio(ctx, msg.Chat.ID, audio)
		if prompt == "" {
			return
		}
	}

	// Forward to graycode daemon
	response, err := tg.forwardToGraycode(ctx, prompt)
	if err != nil {
		response = fmt.Sprintf("Error: %v", err)
	}

	// Format for Telegram (truncate if too long, at rune boundary)
	if len([]rune(response)) > 4000 {
		response = string([]rune(response)[:4000]) + "\n\n... (truncated)"
	}

	tg.reply(ctx, msg.Chat.ID, response)
}

// transcribeAudio downloads a Telegram voice/audio attachment, transcribes it
// via the installed STT engine, and returns the transcript prefixed with
// a marker. On any error it replies to the user with the problem and returns "".
func (tg *TelegramGateway) transcribeAudio(ctx context.Context, chatID int64, a *telegramAudio) string {
	if !stt.Enabled() {
		return ""
	}

	// Resolve file_id to a download URL via Telegram's bot API.
	filePath, err := tg.getAudioFilePath(ctx, a.FileID)
	if err != nil {
		tg.reply(ctx, chatID, fmt.Sprintf("Audio resolve failed: %v", err))
		return ""
	}
	downloadURL := fmt.Sprintf("https://api.telegram.org/file/bot%s/%s", tg.Token, filePath)

	localPath, err := stt.DownloadAttachment(ctx, tg.client, downloadURL, "", safeFileName(a.FileID, a.MimeType))
	if err != nil {
		tg.reply(ctx, chatID, fmt.Sprintf("Audio download failed: %v", err))
		return ""
	}
	defer func() {
		// Clean up the temp dir containing the downloaded file.
		_ = os.RemoveAll(filepath.Dir(localPath))
	}()

	text, err := stt.Transcribe(ctx, localPath, "")
	if err != nil {
		tg.reply(ctx, chatID, fmt.Sprintf("Transcription failed: %v", err))
		return ""
	}
	return "[Voice transcript] " + text
}

// getAudioFilePath resolves a Telegram file_id to a file path on the bot API
// server so the message can be downloaded via the file URL.
func (tg *TelegramGateway) getAudioFilePath(ctx context.Context, fileID string) (string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet,
		"https://api.telegram.org/bot"+tg.Token+"/getFile?file_id="+url.QueryEscape(fileID), nil)
	if err != nil {
		return "", err
	}
	resp, err := tg.client.Do(req)
	if err != nil {
		return "", err
	}
	defer func() { _ = resp.Body.Close() }()
	body, err := io.ReadAll(io.LimitReader(resp.Body, 1<<16))
	if err != nil {
		return "", err
	}
	var result struct {
		OK     bool `json:"ok"`
		Result struct {
			FilePath string `json:"file_path"`
		} `json:"result"`
	}
	if err := json.Unmarshal(body, &result); err != nil {
		return "", err
	}
	if !result.OK {
		return "", fmt.Errorf("telegram getFile returned not OK")
	}
	return result.Result.FilePath, nil
}

// safeFileName builds a human-readable filename for a downloaded Telegram audio
// file using the file_id prefix and the MIME-suggested extension.
func safeFileName(fileID, mimeType string) string {
	ext := stt.ExtensionForMedia(fileID, mimeType)
	prefix := fileID
	if len(prefix) > 16 {
		prefix = prefix[:16]
	}
	return prefix + ext
}

// reply sends text and logs (rather than swallows) any delivery failure.
func (tg *TelegramGateway) reply(ctx context.Context, chatID int64, text string) {
	if err := tg.sendMessage(ctx, chatID, text); err != nil {
		slog.Error("telegram sendMessage failed", "chat", chatID, "error", err)
	}
}

func (tg *TelegramGateway) forwardToGraycode(ctx context.Context, prompt string) (string, error) {
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
	body, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		slog.Warn("partial read in forwardToGraycode response", "error", err)
	}
	var chatResp struct {
		Response string `json:"response"`
	}
	if err := json.Unmarshal(body, &chatResp); err != nil {
		// JSON unmarshal may fail with partial body; return raw body as fallback
		return string(body), nil
	}
	return chatResp.Response, nil
}

func (tg *TelegramGateway) sendMessage(ctx context.Context, chatID int64, text string) error {
	apiURL := fmt.Sprintf("https://api.telegram.org/bot%s/sendMessage", tg.Token)
	data := url.Values{
		"chat_id":    {fmt.Sprintf("%d", chatID)},
		"text":       {text},
		"parse_mode": {""},
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
	defer func() { _ = resp.Body.Close() }()
	body, err := io.ReadAll(io.LimitReader(resp.Body, 4096))
	if err != nil {
		slog.Warn("partial read in sendMessage response", "error", err)
	}
	var result struct {
		OK          bool   `json:"ok"`
		Description string `json:"description"`
	}
	if err := json.Unmarshal(body, &result); err != nil {
		// Non-JSON body: fall back to the HTTP status for a useful error.
		if resp.StatusCode/100 != 2 {
			return fmt.Errorf("telegram sendMessage: HTTP %d", resp.StatusCode)
		}
		return nil
	}
	if !result.OK {
		return fmt.Errorf("telegram sendMessage: %s", result.Description)
	}
	return nil
}
