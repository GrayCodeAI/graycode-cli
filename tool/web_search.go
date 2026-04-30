package tool

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

type WebSearchTool struct{}

func (WebSearchTool) Name() string      { return "WebSearch" }
func (WebSearchTool) Aliases() []string { return []string{"web_search"} }
func (WebSearchTool) Description() string {
	return "Search the web and return results. Uses DuckDuckGo."
}
func (WebSearchTool) Parameters() map[string]interface{} {
	return map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"query": map[string]interface{}{"type": "string", "description": "Search query"},
		},
		"required": []string{"query"},
	}
}

func (WebSearchTool) Execute(ctx context.Context, input json.RawMessage) (string, error) {
	var p struct {
		Query string `json:"query"`
	}
	if err := json.Unmarshal(input, &p); err != nil {
		return "", err
	}
	if p.Query == "" {
		return "", fmt.Errorf("query is required")
	}

	ctx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()

	u := "https://html.duckduckgo.com/html/?q=" + url.QueryEscape(p.Query)
	req, _ := http.NewRequestWithContext(ctx, "GET", u, nil)
	req.Header.Set("User-Agent", "hawk/0.0.1")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(io.LimitReader(resp.Body, 200_000))
	text := htmlTagRe.ReplaceAllString(string(body), " ")
	text = multiSpaceRe.ReplaceAllString(text, "\n")

	// Keep first 50 non-empty lines
	var lines []string
	for _, l := range strings.Split(text, "\n") {
		l = strings.TrimSpace(l)
		if l != "" {
			lines = append(lines, l)
			if len(lines) >= 50 {
				break
			}
		}
	}
	return fmt.Sprintf("Search results for: %s\n\n%s", p.Query, strings.Join(lines, "\n")), nil
}
