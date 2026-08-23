// Package notify delivers best-effort completion notifications to external
// channels (generic webhooks, Telegram), adopting Orca's "know when your
// agent finishes" pattern. Configuration is environment-driven; when nothing
// is configured Send is a silent no-op. Delivery errors never propagate to
// callers as fatal — notifications must not fail an agent run.
package notify

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/GrayCodeAI/hawk/internal/ui/icons"
)

var httpClient = &http.Client{Timeout: 10 * time.Second}

// Completion describes one finished agent run.
type Completion struct {
	Title      string `json:"title"`
	Body       string `json:"body,omitempty"`
	OK         bool   `json:"ok"`
	Source     string `json:"source,omitempty"` // e.g. "hawk exec"
	Branch     string `json:"branch,omitempty"`
	WebhookURL string `json:"-"`
}

// envReader indirection for tests.
var envReader = os.Getenv

// Configured reports whether any notification channel is set up.
func Configured() bool {
	return envReader("HAWK_NOTIFY_WEBHOOK_URL") != "" ||
		(envReader("HAWK_NOTIFY_TELEGRAM_TOKEN") != "" && envReader("HAWK_NOTIFY_TELEGRAM_CHAT_ID") != "")
}

// SendCompletion delivers c to every configured channel. Errors are joined;
// an empty/nil-error result means delivery was attempted or nothing was
// configured. It blocks at most ~10s per channel.
func SendCompletion(c Completion) error {
	if c.Title == "" {
		c.Title = "Agent run finished"
	}
	var errs []string
	if url := envReader("HAWK_NOTIFY_WEBHOOK_URL"); url != "" {
		if err := sendWebhook(url, c); err != nil {
			errs = append(errs, "webhook: "+err.Error())
		}
	}
	tok, chat := envReader("HAWK_NOTIFY_TELEGRAM_TOKEN"), envReader("HAWK_NOTIFY_TELEGRAM_CHAT_ID")
	if tok != "" && chat != "" {
		if err := sendTelegram(tok, chat, renderText(c)); err != nil {
			errs = append(errs, "telegram: "+err.Error())
		}
	}
	if len(errs) > 0 {
		return fmt.Errorf("notify: %s", strings.Join(errs, "; "))
	}
	return nil
}

func sendWebhook(url string, c Completion) error {
	payload := map[string]interface{}{
		"event":       "agent_completion",
		"title":       c.Title,
		"body":        c.Body,
		"ok":          c.OK,
		"source":      c.Source,
		"branch":      c.Branch,
		"finished_at": time.Now().UTC().Format(time.RFC3339),
	}
	raw, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(context.Background(), http.MethodPost, url, bytes.NewReader(raw)) // #nosec G107 -- operator-configured webhook URL
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := httpClient.Do(req)
	if err != nil {
		return err
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode >= 300 {
		return fmt.Errorf("webhook status %d", resp.StatusCode)
	}
	return nil
}

func sendTelegram(token, chatID, text string) error {
	api := fmt.Sprintf("https://api.telegram.org/bot%s/sendMessage", token)
	form := strings.NewReader(fmt.Sprintf(
		`{"chat_id":%q,"text":%q,"disable_web_page_preview":true}`, chatID, text,
	))
	req, err := http.NewRequestWithContext(context.Background(), http.MethodPost, api, form) // #nosec G107 -- fixed api host, operator env token
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := httpClient.Do(req)
	if err != nil {
		return err
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode >= 300 {
		return fmt.Errorf("telegram status %d", resp.StatusCode)
	}
	return nil
}

func renderText(c Completion) string {
	status := icons.Check()
	if !c.OK {
		status = icons.Close()
	}
	out := status + " " + c.Title
	if c.Body != "" {
		body := c.Body
		if len(body) > 800 {
			body = body[:800] + "…"
		}
		out += "\n\n" + body
	}
	if c.Branch != "" {
		out += "\n\nbranch: " + c.Branch
	}
	return out
}
