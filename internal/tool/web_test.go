package tool

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"testing"

	"github.com/GrayCodeAI/hawk/internal/testutil"
)

func TestWebSearchTool_Name(t *testing.T) {
	var ws WebSearchTool
	if ws.Name() != "WebSearch" {
		t.Fatalf("expected WebSearch, got %q", ws.Name())
	}
}

func TestWebSearchTool_Parameters(t *testing.T) {
	var ws WebSearchTool
	p := ws.Parameters()
	props := p["properties"].(map[string]interface{})
	if _, ok := props["query"]; !ok {
		t.Fatal("expected query field in parameters")
	}
}

func TestWebSearchTool_EmptyQuery(t *testing.T) {
	var ws WebSearchTool
	input, _ := json.Marshal(map[string]string{"query": ""})
	_, err := ws.Execute(context.Background(), input)
	if err == nil || !strings.Contains(err.Error(), "is required") {
		t.Fatalf("expected a 'required' error, got %v", err)
	}
}

func TestWebSearchTool_RejectsTooManyQueries(t *testing.T) {
	queries := make([]string, maxWebSearchQueries+1)
	for i := range queries {
		queries[i] = "query"
	}
	input, _ := json.Marshal(map[string]any{"queries": queries})
	_, err := (WebSearchTool{}).Execute(context.Background(), input)
	if err == nil || !strings.Contains(err.Error(), "at most") {
		t.Fatalf("expected query limit error, got %v", err)
	}
}

func TestWebSearchTool_RejectsLongQuery(t *testing.T) {
	input, _ := json.Marshal(map[string]string{"query": strings.Repeat("x", maxWebSearchQueryLength+1)})
	_, err := (WebSearchTool{}).Execute(context.Background(), input)
	if err == nil || !strings.Contains(err.Error(), "exceeds") {
		t.Fatalf("expected query length error, got %v", err)
	}
}

func TestWebFetchTool_Name(t *testing.T) {
	var wf WebFetchTool
	if wf.Name() != "WebFetch" {
		t.Fatalf("expected WebFetch, got %q", wf.Name())
	}
}

func TestWebFetchTool_Parameters(t *testing.T) {
	var wf WebFetchTool
	p := wf.Parameters()
	props := p["properties"].(map[string]interface{})
	if _, ok := props["url"]; !ok {
		t.Fatal("expected url field in parameters")
	}
}

func TestWebFetchTool_EmptyURL(t *testing.T) {
	var wf WebFetchTool
	input, _ := json.Marshal(map[string]string{"url": ""})
	_, err := wf.Execute(context.Background(), input)
	if err == nil || !strings.Contains(err.Error(), "url is required") {
		t.Fatalf("expected 'url is required' error, got %v", err)
	}
}

func TestWebFetchTool_HTMLStripping(t *testing.T) {
	srv := testutil.NewLoopbackHTTPServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		w.Write([]byte("<html><body><h1>Hello</h1><p>World</p></body></html>"))
	}))
	defer srv.Close()

	var wf WebFetchTool
	input, _ := json.Marshal(map[string]string{"url": srv.URL})
	result, err := wf.Execute(WithSSRFSkip(context.Background()), input)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if strings.Contains(result, "<h1>") || strings.Contains(result, "<p>") {
		t.Fatal("HTML tags were not stripped")
	}
	if !strings.Contains(result, "Hello") || !strings.Contains(result, "World") {
		t.Fatal("expected text content to be preserved")
	}
}

func TestWebFetchTool_Truncation(t *testing.T) {
	srv := testutil.NewLoopbackHTTPServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/plain")
		w.Write([]byte(strings.Repeat("a", 60000)))
	}))
	defer srv.Close()

	var wf WebFetchTool
	input, _ := json.Marshal(map[string]string{"url": srv.URL})
	result, err := wf.Execute(WithSSRFSkip(context.Background()), input)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(result, "truncated") {
		t.Fatal("expected truncation marker")
	}
}
