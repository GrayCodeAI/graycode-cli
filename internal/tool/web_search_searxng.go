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

	"github.com/GrayCodeAI/hawk/internal/env"
)

// searxngClient is a SearXNG API client.
// Endpoint: {SEARXNG_URL}/search?q=<query>&format=json&pageno=1
// Response JSON: { results: [{title, url, content}] }
type searxngClient struct {
	baseURL string
	http    *http.Client
}

// newSearxngClient creates a new SearXNG client, reading the instance URL from
// the SEARXNG_URL environment variable.
func newSearxngClient() *searxngClient {
	baseURL := env.Getenv("SEARXNG_URL")
	// Ensure no trailing slash
	baseURL = strings.TrimRight(baseURL, "/")
	return &searxngClient{
		baseURL: baseURL,
		http: &http.Client{
			Timeout: 15 * time.Second,
		},
	}
}

// available returns true if the SearXNG base URL is configured.
func (c *searxngClient) available() bool {
	return c.baseURL != ""
}

// searxngResponse represents the top-level SearXNG API response.
type searxngResponse struct {
	Results []searxngResult `json:"results"`
}

// searxngResult represents a single result from the SearXNG API.
type searxngResult struct {
	Title   string `json:"title"`
	URL     string `json:"url"`
	Content string `json:"content"`
}

// search queries the SearXNG instance and returns structured results.
func (c *searxngClient) search(ctx context.Context, query string, count int) ([]searchResult, error) {
	if !c.available() {
		return nil, fmt.Errorf("searxng: base URL not configured")
	}

	params := url.Values{}
	params.Set("q", query)
	params.Set("format", "json")
	params.Set("pageno", "1")

	reqURL := c.baseURL + "/search?" + params.Encode()
	req, err := http.NewRequestWithContext(ctx, "GET", reqURL, nil)
	if err != nil {
		return nil, fmt.Errorf("searxng: %w", err)
	}

	req.Header.Set("Accept", "application/json")
	req.Header.Set("User-Agent", "hawk/0.1.0")

	resp, err := c.http.Do(req)
	if err != nil {
		return nil, fmt.Errorf("searxng: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 1024))
		return nil, fmt.Errorf("searxng: API returned status %d: %s", resp.StatusCode, string(body))
	}

	body, err := io.ReadAll(io.LimitReader(resp.Body, 500_000))
	if err != nil {
		return nil, fmt.Errorf("searxng: reading response: %w", err)
	}

	var searxResp searxngResponse
	if err := json.Unmarshal(body, &searxResp); err != nil {
		return nil, fmt.Errorf("searxng: parsing response: %w", err)
	}

	var results []searchResult
	for _, r := range searxResp.Results {
		if len(results) >= count {
			break
		}
		results = append(results, searchResult{
			Title:       r.Title,
			URL:         r.URL,
			Description: r.Content,
		})
	}

	return results, nil
}
