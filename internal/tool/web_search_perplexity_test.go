package tool

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

// pplxTestKey is a placeholder credential for exercising provider request paths.
const pplxTestKey = "pplx-test"

func TestPerplexitySearchClientAvailable(t *testing.T) {
	t.Setenv("PERPLEXITY_API_KEY", "")
	if newPerplexitySearchClient().available() {
		t.Fatal("expected unavailable without PERPLEXITY_API_KEY")
	}
	t.Setenv("PERPLEXITY_API_KEY", pplxTestKey)
	if !newPerplexitySearchClient().available() {
		t.Fatal("expected available with PERPLEXITY_API_KEY")
	}
}

func TestPerplexitySearchClientSearch(t *testing.T) {
	var gotBody perplexityChatRequest
	var gotAuth string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotAuth = r.Header.Get("authorization")
		if r.URL.Path != "/chat/completions" {
			t.Errorf("path = %q, want /chat/completions", r.URL.Path)
		}
		if err := json.NewDecoder(r.Body).Decode(&gotBody); err != nil {
			t.Fatal(err)
		}
		w.Header().Set("content-type", "application/json")
		_, _ = w.Write([]byte(`{
			"choices": [{"message": {"content": "answer"}}],
			"search_results": [
				{"title": "P One", "url": "https://pplx.example/1", "content": "snippet one"}
			]
		}`))
	}))
	defer srv.Close()

	c := newPerplexitySearchClient()
	c.apiKey = pplxTestKey
	c.baseURL = srv.URL

	results, err := c.search(context.Background(), "go", 5)
	if err != nil {
		t.Fatalf("search() error: %v", err)
	}
	if len(results) != 1 || results[0].URL != "https://pplx.example/1" || results[0].Description != "snippet one" {
		t.Errorf("results = %+v", results)
	}
	if gotAuth != "Bearer pplx-test" {
		t.Errorf("authorization = %q", gotAuth)
	}
	if gotBody.Model != perplexitySearchModel || gotBody.Messages[0].Content != "go" {
		t.Errorf("body = %+v", gotBody)
	}
}

func TestPerplexitySearchClientCitationFallback(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("content-type", "application/json")
		_, _ = w.Write([]byte(`{
			"choices": [{"message": {"content": "answer"}}],
			"citations": ["https://pplx.example/1", "https://pplx.example/2"]
		}`))
	}))
	defer srv.Close()

	c := newPerplexitySearchClient()
	c.apiKey = pplxTestKey
	c.baseURL = srv.URL

	results, err := c.search(context.Background(), "go", 5)
	if err != nil {
		t.Fatalf("search() error: %v", err)
	}
	if len(results) != 2 {
		t.Fatalf("len(results) = %d, want 2", len(results))
	}
	if results[0].URL != "https://pplx.example/1" {
		t.Errorf("result[0] = %+v", results[0])
	}
}

func TestPerplexitySearchClientNoResultsErrors(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("content-type", "application/json")
		_, _ = w.Write([]byte(`{"choices": [{"message": {"content": ""}}]}`))
	}))
	defer srv.Close()

	c := newPerplexitySearchClient()
	c.apiKey = pplxTestKey
	c.baseURL = srv.URL

	if _, err := c.search(context.Background(), "none", 5); err == nil {
		t.Fatal("expected error when no search_results or citations")
	}
}
