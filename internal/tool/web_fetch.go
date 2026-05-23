package tool

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"regexp"
	"strings"
	"time"
)

type WebFetchTool struct{}

func (WebFetchTool) Name() string      { return "WebFetch" }
func (WebFetchTool) Aliases() []string { return []string{"web_fetch"} }
func (WebFetchTool) Description() string {
	return "Fetch a URL and return its content as text. HTML is converted to plain text."
}

func (WebFetchTool) Parameters() map[string]interface{} {
	return map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"url": map[string]interface{}{"type": "string", "description": "URL to fetch"},
		},
		"required": []string{"url"},
	}
}

var (
	htmlTagRe    = regexp.MustCompile(`<[^>]*>`)
	multiSpaceRe = regexp.MustCompile(`\s{3,}`)
)

func (WebFetchTool) Execute(ctx context.Context, input json.RawMessage) (string, error) {
	var p struct {
		URL string `json:"url"`
	}
	if err := json.Unmarshal(input, &p); err != nil {
		return "", err
	}
	if p.URL == "" {
		return "", fmt.Errorf("url is required")
	}
	pinnedURL, err := validateURLPublic(ctx, p.URL)
	if err != nil {
		return "", err
	}

	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, "GET", pinnedURL, nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("User-Agent", "hawk/0.2.0")

	client := ssrfSafeClient(ctx, 30*time.Second)
	resp, err := client.Do(req)
	if err != nil {
		return "", err
	}
	defer func() { _ = resp.Body.Close() }()

	body, err := io.ReadAll(io.LimitReader(resp.Body, 500_000))
	if err != nil {
		return "", err
	}

	text := string(body)
	// Strip HTML tags for a rough text extraction
	if strings.Contains(resp.Header.Get("Content-Type"), "html") {
		text = htmlTagRe.ReplaceAllString(text, " ")
		text = multiSpaceRe.ReplaceAllString(text, "\n")
		text = strings.TrimSpace(text)
	}

	if len(text) > 50000 {
		text = text[:50000] + "\n... (truncated)"
	}
	return fmt.Sprintf("[%d] %s\n\n%s", resp.StatusCode, p.URL, text), nil
}
