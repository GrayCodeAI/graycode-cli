package tool

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/GrayCodeAI/hawk/internal/env"
)

// deepseekSearchClient is a DeepSeek-backed web search provider. It calls the
// Anthropic-compatible Messages API with the native `web_search_20250305`
// server tool. It reuses DEEPSEEK_API_KEY but not DEEPSEEK_BASE_URL, because
// search and chat-completions use different bases (DSH
// `web/web-search-deepseek` parity, dsh-v0.1.0-rc.7).
type deepseekSearchClient struct {
	apiKey  string
	baseURL string
	http    *http.Client
}

// newDeepseekSearchClient reads the key from the DEEPSEEK_API_KEY env var.
func newDeepseekSearchClient() *deepseekSearchClient {
	return &deepseekSearchClient{
		apiKey:  env.Getenv("DEEPSEEK_API_KEY"),
		baseURL: deepseekSearchBaseURL,
		http:    &http.Client{Timeout: 30 * time.Second},
	}
}

func (c *deepseekSearchClient) available() bool { return c.apiKey != "" }

const (
	deepseekSearchBaseURL  = "https://api.deepseek.com/anthropic/v1"
	deepseekSearchModel    = "deepseek-v4-flash"
	deepseekSearchMaxToken = 4096
	deepseekSearchMaxUses  = 5
)

// deepseekMessagesRequest is the Anthropic-compatible request body.
type deepseekMessagesRequest struct {
	Model     string                `json:"model"`
	MaxTokens int                   `json:"max_tokens"`
	Messages  []deepseekUserMessage `json:"messages"`
	Tools     []deepseekSearchTool  `json:"tools"`
}

type deepseekUserMessage struct {
	Role    string              `json:"role"`
	Content []deepseekTextBlock `json:"content"`
}

type deepseekTextBlock struct {
	Type string `json:"type"`
	Text string `json:"text"`
}

type deepseekSearchTool struct {
	Type    string `json:"type"`
	Name    string `json:"name"`
	MaxUses int    `json:"max_uses"`
}

// deepseekMessagesResponse is the Anthropic-compatible response body.
type deepseekMessagesResponse struct {
	Content []deepseekContentBlock `json:"content"`
}

type deepseekContentBlock struct {
	Type          string                 `json:"type"`
	Text          string                 `json:"text"`
	Citations     []deepseekCitation     `json:"citations,omitempty"`
	SearchResults []deepseekSearchResult `json:"search_results,omitempty"`
}

type deepseekCitation struct {
	URL       string `json:"url"`
	Title     string `json:"title"`
	CitedText string `json:"cited_text"`
}

type deepseekSearchResult struct {
	URL   string `json:"url"`
	Title string `json:"title"`
	Text  string `json:"text"`
}

// search runs one query through the DeepSeek native web search tool.
func (c *deepseekSearchClient) search(ctx context.Context, query string, count int) ([]searchResult, error) {
	if !c.available() {
		return nil, fmt.Errorf("deepseek search: API key not configured")
	}

	body := deepseekMessagesRequest{
		Model:     deepseekSearchModel,
		MaxTokens: deepseekSearchMaxToken,
		Messages: []deepseekUserMessage{{
			Role: "user",
			Content: []deepseekTextBlock{{
				Type: "text",
				Text: "Perform a web search for the query: " + query,
			}},
		}},
		Tools: []deepseekSearchTool{{
			Type:    "web_search_20250305",
			Name:    "web_search",
			MaxUses: deepseekSearchMaxUses,
		}},
	}

	payload, err := json.Marshal(body)
	if err != nil {
		return nil, fmt.Errorf("deepseek search: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+"/messages", bytes.NewReader(payload))
	if err != nil {
		return nil, fmt.Errorf("deepseek search: %w", err)
	}
	req.Header.Set("x-api-key", c.apiKey)
	req.Header.Set("authorization", "Bearer "+c.apiKey)
	req.Header.Set("anthropic-version", "2023-06-01")
	req.Header.Set("content-type", "application/json")
	req.Header.Set("accept", "application/json")
	req.Header.Set("user-agent", "hawk/0.1.0")

	resp, err := c.http.Do(req)
	if err != nil {
		return nil, fmt.Errorf("deepseek search: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 1024))
		return nil, fmt.Errorf("deepseek search: API returned status %d: %s", resp.StatusCode, string(body))
	}

	raw, err := io.ReadAll(io.LimitReader(resp.Body, 1_000_000))
	if err != nil {
		return nil, fmt.Errorf("deepseek search: reading response: %w", err)
	}

	var parsed deepseekMessagesResponse
	if err := json.Unmarshal(raw, &parsed); err != nil {
		return nil, fmt.Errorf("deepseek search: parsing response: %w", err)
	}

	var results []searchResult
	seen := make(map[string]bool)
	for _, block := range parsed.Content {
		switch block.Type {
		case "web_search_tool_result":
			for _, r := range block.SearchResults {
				if len(results) >= count {
					break
				}
				if seen[r.URL] {
					continue
				}
				seen[r.URL] = true
				results = append(results, searchResult{Title: r.Title, URL: r.URL, Description: r.Text})
			}
		case "text":
			for _, cite := range block.Citations {
				if len(results) >= count {
					break
				}
				if cite.URL == "" || seen[cite.URL] {
					continue
				}
				seen[cite.URL] = true
				title := cite.Title
				if title == "" {
					title = cite.URL
				}
				results = append(results, searchResult{Title: title, URL: cite.URL, Description: cite.CitedText})
			}
		}
		if len(results) >= count {
			break
		}
	}
	if len(results) == 0 {
		return nil, fmt.Errorf("deepseek search: no web_search_tool_result blocks returned")
	}
	return results, nil
}
