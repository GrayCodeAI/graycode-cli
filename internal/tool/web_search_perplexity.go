package tool

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/GrayCodeAI/graycode-cli/internal/env"
)

// perplexitySearchClient is a Perplexity-backed web search provider (DSH
// `web/web-search-perplexity` parity, dsh-v0.1.0-rc.7). It calls the
// Perplexity chat completions API; sources prefer structured
// `search_results[]` and fall back to URL-only `citations[]`.
type perplexitySearchClient struct {
	apiKey  string
	baseURL string
	http    *http.Client
}

func newPerplexitySearchClient() *perplexitySearchClient {
	return &perplexitySearchClient{
		apiKey:  env.Getenv("PERPLEXITY_API_KEY"),
		baseURL: perplexitySearchBaseURL,
		http:    &http.Client{Timeout: 30 * time.Second},
	}
}

func (c *perplexitySearchClient) available() bool { return c.apiKey != "" }

const (
	perplexitySearchBaseURL   = "https://api.perplexity.ai"
	perplexitySearchModel     = "sonar"
	perplexitySearchMaxTokens = 1024
)

type perplexityChatRequest struct {
	Model     string          `json:"model"`
	MaxTokens int             `json:"max_tokens"`
	Messages  []perplexityMsg `json:"messages"`
}

type perplexityMsg struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type perplexityChatResponse struct {
	Choices []struct {
		Message struct {
			Content string `json:"content"`
		} `json:"message"`
	} `json:"choices"`
	SearchResults []struct {
		Title   string `json:"title"`
		URL     string `json:"url"`
		Content string `json:"content"`
	} `json:"search_results"`
	Citations []string `json:"citations"`
}

// search runs one query through the Perplexity API and returns structured
// results from `search_results[]`, falling back to URL-only `citations[]`.
func (c *perplexitySearchClient) search(ctx context.Context, query string, count int) ([]searchResult, error) {
	if !c.available() {
		return nil, fmt.Errorf("perplexity search: API key not configured")
	}

	body := perplexityChatRequest{
		Model:     perplexitySearchModel,
		MaxTokens: perplexitySearchMaxTokens,
		Messages:  []perplexityMsg{{Role: "user", Content: query}},
	}
	payload, err := json.Marshal(body)
	if err != nil {
		return nil, fmt.Errorf("perplexity search: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+"/chat/completions", bytes.NewReader(payload))
	if err != nil {
		return nil, fmt.Errorf("perplexity search: %w", err)
	}
	req.Header.Set("authorization", "Bearer "+c.apiKey)
	req.Header.Set("content-type", "application/json")
	req.Header.Set("accept", "application/json")
	req.Header.Set("user-agent", "graycode/0.1.0")

	resp, err := c.http.Do(req)
	if err != nil {
		return nil, fmt.Errorf("perplexity search: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 1024))
		return nil, fmt.Errorf("perplexity search: API returned status %d: %s", resp.StatusCode, string(body))
	}

	raw, err := io.ReadAll(io.LimitReader(resp.Body, 1_000_000))
	if err != nil {
		return nil, fmt.Errorf("perplexity search: reading response: %w", err)
	}

	var parsed perplexityChatResponse
	if err := json.Unmarshal(raw, &parsed); err != nil {
		return nil, fmt.Errorf("perplexity search: parsing response: %w", err)
	}

	var results []searchResult
	for _, r := range parsed.SearchResults {
		if len(results) >= count {
			break
		}
		results = append(results, searchResult{Title: r.Title, URL: r.URL, Description: r.Content})
	}
	if len(results) == 0 {
		for _, cite := range parsed.Citations {
			if len(results) >= count {
				break
			}
			results = append(results, searchResult{Title: cite, URL: cite})
		}
	}
	if len(results) == 0 {
		return nil, fmt.Errorf("perplexity search: no search_results or citations returned")
	}
	return results, nil
}
