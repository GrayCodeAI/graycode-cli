package tool

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

// exaTestKey is a placeholder credential for exercising provider request paths.
const exaTestKey = "exa-test"

func TestExaSearchClientAvailable(t *testing.T) {
	t.Setenv("EXA_API_KEY", "")
	if newExaSearchClient().available() {
		t.Fatal("expected unavailable without EXA_API_KEY")
	}
	t.Setenv("EXA_API_KEY", exaTestKey)
	if !newExaSearchClient().available() {
		t.Fatal("expected available with EXA_API_KEY")
	}
}

func TestExaSearchClientSearch(t *testing.T) {
	var gotBody exaSearchRequest
	var gotKey string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotKey = r.Header.Get("x-api-key")
		if r.URL.Path != "/search" {
			t.Errorf("path = %q, want /search", r.URL.Path)
		}
		if err := json.NewDecoder(r.Body).Decode(&gotBody); err != nil {
			t.Fatal(err)
		}
		w.Header().Set("content-type", "application/json")
		_, _ = w.Write([]byte(`{"results": [
			{"title": "Exa One", "url": "https://exa.example/1", "highlights": ["highlight sentence"]},
			{"title": "Exa Two", "url": "https://exa.example/2", "text": "full text fallback"}
		]}`))
	}))
	defer srv.Close()

	c := newExaSearchClient()
	c.apiKey = exaTestKey
	c.baseURL = srv.URL

	results, err := c.search(context.Background(), "go", 5)
	if err != nil {
		t.Fatalf("search() error: %v", err)
	}
	if len(results) != 2 {
		t.Fatalf("len(results) = %d, want 2", len(results))
	}
	if results[0].Title != "Exa One" || results[0].Description != "highlight sentence" {
		t.Errorf("result[0] = %+v", results[0])
	}
	if results[1].Description != "full text fallback" {
		t.Errorf("result[1] description = %q, want text fallback", results[1].Description)
	}
	if gotKey != "exa-test" {
		t.Errorf("x-api-key = %q, want exa-test", gotKey)
	}
	if gotBody.Type != "auto" || gotBody.NumResults != 5 || gotBody.Contents.Highlights.HighlightsPerURL != 1 {
		t.Errorf("body = %+v", gotBody)
	}
}

func TestExaSearchClientEmptyResultsErrors(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("content-type", "application/json")
		_, _ = w.Write([]byte(`{"results": []}`))
	}))
	defer srv.Close()

	c := newExaSearchClient()
	c.apiKey = exaTestKey
	c.baseURL = srv.URL

	if _, err := c.search(context.Background(), "none", 5); err == nil {
		t.Fatal("expected error on empty results")
	}
}
