package tool

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"time"
)

// braveClient is a Brave Search API client.
// Endpoint: https://api.search.brave.com/res/v1/web/search
// Headers: X-Subscription-Token: <api_key>, Accept: application/json
// Query params: q=<query>, count=<numResults>
type braveClient struct {
	apiKey string
	http   *http.Client
}

// newBraveClient creates a new Brave Search client, reading the API key from
// the BRAVE_SEARCH_API_KEY environment variable.
func newBraveClient() *braveClient {
	return &braveClient{
		apiKey: os.Getenv("BRAVE_SEARCH_API_KEY"),
		http: &http.Client{
			Timeout: 15 * time.Second,
		},
	}
}

// available returns true if the Brave Search API key is configured.
func (c *braveClient) available() bool {
	return c.apiKey != ""
}

// braveResponse represents the top-level Brave Search API response.
type braveResponse struct {
	Web struct {
		Results []braveResult `json:"results"`
	} `json:"web"`
}

// braveResult represents a single result from the Brave Search API.
type braveResult struct {
	Title       string `json:"title"`
	URL         string `json:"url"`
	Description string `json:"description"`
}

// search queries the Brave Search API and returns structured results.
func (c *braveClient) search(ctx context.Context, query string, count int) ([]searchResult, error) {
	if !c.available() {
		return nil, fmt.Errorf("brave search: API key not configured")
	}

	endpoint := "https://api.search.brave.com/res/v1/web/search"
	params := url.Values{}
	params.Set("q", query)
	params.Set("count", fmt.Sprintf("%d", count))

	reqURL := endpoint + "?" + params.Encode()
	req, err := http.NewRequestWithContext(ctx, "GET", reqURL, nil)
	if err != nil {
		return nil, fmt.Errorf("brave search: %w", err)
	}

	req.Header.Set("X-Subscription-Token", c.apiKey)
	req.Header.Set("Accept", "application/json")
	req.Header.Set("User-Agent", "hawk/0.1.0")

	resp, err := c.http.Do(req)
	if err != nil {
		return nil, fmt.Errorf("brave search: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 1024))
		return nil, fmt.Errorf("brave search: API returned status %d: %s", resp.StatusCode, string(body))
	}

	body, err := io.ReadAll(io.LimitReader(resp.Body, 500_000))
	if err != nil {
		return nil, fmt.Errorf("brave search: reading response: %w", err)
	}

	var braveResp braveResponse
	if err := json.Unmarshal(body, &braveResp); err != nil {
		return nil, fmt.Errorf("brave search: parsing response: %w", err)
	}

	var results []searchResult
	for _, r := range braveResp.Web.Results {
		if len(results) >= count {
			break
		}
		results = append(results, searchResult(r))
	}

	return results, nil
}
