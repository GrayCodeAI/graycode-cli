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

// exaSearchClient is an Exa-backed web search provider (DSH
// `web/web-search-exa` parity, dsh-v0.1.0-rc.7). Empty EXA_API_KEY makes the
// provider unavailable so the cascade falls through.
type exaSearchClient struct {
	apiKey  string
	baseURL string
	http    *http.Client
}

func newExaSearchClient() *exaSearchClient {
	return &exaSearchClient{
		apiKey:  env.Getenv("EXA_API_KEY"),
		baseURL: exaSearchBaseURL,
		http:    &http.Client{Timeout: 30 * time.Second},
	}
}

func (c *exaSearchClient) available() bool { return c.apiKey != "" }

const exaSearchBaseURL = "https://api.exa.ai"

// exaSearchRequest is the Exa /search request body. `auto` type mirrors DSH's
// EXA_DEFAULT_SEARCH_TYPE.
type exaSearchRequest struct {
	Query      string      `json:"query"`
	Type       string      `json:"type"`
	NumResults int         `json:"numResults"`
	Contents   exaContents `json:"contents"`
}

type exaContents struct {
	Highlights exaHighlights `json:"highlights"`
}

type exaHighlights struct {
	HighlightsPerURL int `json:"highlightsPerUrl"`
}

type exaSearchResponse struct {
	Results []exaResult `json:"results"`
}

type exaResult struct {
	Title      string   `json:"title"`
	URL        string   `json:"url"`
	Text       string   `json:"text"`
	Highlights []string `json:"highlights"`
}

// search runs one query through the Exa API and returns structured results.
func (c *exaSearchClient) search(ctx context.Context, query string, count int) ([]searchResult, error) {
	if !c.available() {
		return nil, fmt.Errorf("exa search: API key not configured")
	}

	body := exaSearchRequest{
		Query:      query,
		Type:       "auto",
		NumResults: count,
		Contents: exaContents{Highlights: exaHighlights{
			HighlightsPerURL: 1,
		}},
	}
	payload, err := json.Marshal(body)
	if err != nil {
		return nil, fmt.Errorf("exa search: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+"/search", bytes.NewReader(payload))
	if err != nil {
		return nil, fmt.Errorf("exa search: %w", err)
	}
	req.Header.Set("x-api-key", c.apiKey)
	req.Header.Set("content-type", "application/json")
	req.Header.Set("accept", "application/json")
	req.Header.Set("user-agent", "graycode/0.1.0")

	resp, err := c.http.Do(req)
	if err != nil {
		return nil, fmt.Errorf("exa search: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 1024))
		return nil, fmt.Errorf("exa search: API returned status %d: %s", resp.StatusCode, string(body))
	}

	raw, err := io.ReadAll(io.LimitReader(resp.Body, 1_000_000))
	if err != nil {
		return nil, fmt.Errorf("exa search: reading response: %w", err)
	}

	var parsed exaSearchResponse
	if err := json.Unmarshal(raw, &parsed); err != nil {
		return nil, fmt.Errorf("exa search: parsing response: %w", err)
	}

	var results []searchResult
	for _, r := range parsed.Results {
		if len(results) >= count {
			break
		}
		snippet := ""
		for _, h := range r.Highlights {
			if h != "" {
				snippet = h
				break
			}
		}
		if snippet == "" {
			snippet = r.Text
		}
		results = append(results, searchResult{Title: r.Title, URL: r.URL, Description: snippet})
	}
	if len(results) == 0 {
		return nil, fmt.Errorf("exa search: no results returned")
	}
	return results, nil
}
