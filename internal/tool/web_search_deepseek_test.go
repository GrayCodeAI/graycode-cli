package tool

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

// testKey is a placeholder credential for exercising provider request paths.
const testKey = "sk-test"

func TestDeepseekSearchClientAvailable(t *testing.T) {
	t.Setenv("DEEPSEEK_API_KEY", "")
	if newDeepseekSearchClient().available() {
		t.Fatal("expected unavailable without DEEPSEEK_API_KEY")
	}
	t.Setenv("DEEPSEEK_API_KEY", testKey)
	if !newDeepseekSearchClient().available() {
		t.Fatal("expected available with DEEPSEEK_API_KEY")
	}
}

func TestDeepseekSearchClientSearch(t *testing.T) {
	var gotBody deepseekMessagesRequest
	var gotHeaders http.Header
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotHeaders = r.Header.Clone()
		if r.URL.Path != "/messages" {
			t.Errorf("path = %q, want /messages", r.URL.Path)
		}
		if err := json.NewDecoder(r.Body).Decode(&gotBody); err != nil {
			t.Fatal(err)
		}
		w.Header().Set("content-type", "application/json")
		_, _ = w.Write([]byte(`{
			"content": [
				{"type": "web_search_tool_result", "search_results": [
					{"url": "https://example.com/a", "title": "A", "text": "desc a"}
				]},
				{"type": "text", "text": "answer", "citations": [
					{"url": "https://example.com/b", "title": "B", "cited_text": "desc b"}
				]}
			]
		}`))
	}))
	defer srv.Close()

	c := newDeepseekSearchClient()
	c.apiKey = testKey
	c.baseURL = srv.URL

	results, err := c.search(context.Background(), "golang", 5)
	if err != nil {
		t.Fatalf("search() error: %v", err)
	}
	if len(results) != 2 {
		t.Fatalf("len(results) = %d, want 2", len(results))
	}
	if results[0].URL != "https://example.com/a" || results[0].Title != "A" || results[0].Description != "desc a" {
		t.Errorf("result[0] = %+v", results[0])
	}
	if results[1].URL != "https://example.com/b" {
		t.Errorf("result[1] = %+v", results[1])
	}

	// Request shape: model, native web_search tool, both auth headers.
	if gotBody.Model != deepseekSearchModel {
		t.Errorf("model = %q, want %q", gotBody.Model, deepseekSearchModel)
	}
	if len(gotBody.Tools) != 1 || gotBody.Tools[0].Type != "web_search_20250305" {
		t.Errorf("tools = %+v, want web_search_20250305", gotBody.Tools)
	}
	if gotHeaders.Get("x-api-key") != "sk-test" {
		t.Errorf("x-api-key header = %q", gotHeaders.Get("x-api-key"))
	}
	if gotHeaders.Get("authorization") != "Bearer sk-test" {
		t.Errorf("authorization header = %q", gotHeaders.Get("authorization"))
	}
}

func TestDeepseekSearchClientDeduplicatesByURL(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("content-type", "application/json")
		_, _ = w.Write([]byte(`{
			"content": [
				{"type": "web_search_tool_result", "search_results": [
					{"url": "https://example.com/x", "title": "X1", "text": "one"}
				]},
				{"type": "text", "text": "answer", "citations": [
					{"url": "https://example.com/x", "title": "X2", "cited_text": "two"}
				]}
			]
		}`))
	}))
	defer srv.Close()

	c := newDeepseekSearchClient()
	c.apiKey = testKey
	c.baseURL = srv.URL

	results, err := c.search(context.Background(), "dup", 5)
	if err != nil {
		t.Fatalf("search() error: %v", err)
	}
	if len(results) != 1 {
		t.Fatalf("len(results) = %d, want 1 (dedup)", len(results))
	}
}

func TestDeepseekSearchClientNoToolResultErrors(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("content-type", "application/json")
		_, _ = w.Write([]byte(`{"content": [{"type": "text", "text": "no search triggered"}]}`))
	}))
	defer srv.Close()

	c := newDeepseekSearchClient()
	c.apiKey = testKey
	c.baseURL = srv.URL

	if _, err := c.search(context.Background(), "none", 5); err == nil {
		t.Fatal("expected error when no web_search_tool_result blocks are returned")
	}
}
