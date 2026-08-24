package tool

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestSearchXBasic(t *testing.T) {
	var sawAuth string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/chat/completions" {
			t.Fatalf("path = %s", r.URL.Path)
		}
		sawAuth = r.Header.Get("Authorization")
		fmt.Fprint(w, `{"choices":[{"message":{"role":"assistant","content":"summary of X results"}}]}`)
	}))
	defer srv.Close()
	oldBase := xAISearchBaseURL
	xAISearchBaseURL = srv.URL
	defer func() { xAISearchBaseURL = oldBase }()
	t.Setenv("XAI_API_KEY", "k")

	out, err := SearchXTool{}.Execute(context.Background(), json.RawMessage(`{"query":"hawk ai"}`))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, "summary of X results") {
		t.Fatalf("out = %q", out)
	}
	if !strings.Contains(sawAuth, "Bearer k") {
		t.Fatalf("auth = %q", sawAuth)
	}
}

func TestSearchXRequiresQuery(t *testing.T) {
	t.Setenv("XAI_API_KEY", "k")
	tool := SearchXTool{}
	if _, err := tool.Execute(context.Background(), json.RawMessage(`{"query":""}`)); err == nil {
		t.Fatal("expected error for empty query")
	}
}

func TestSearchXRequiresKey(t *testing.T) {
	t.Setenv("XAI_API_KEY", "")
	t.Setenv("GROK_API_KEY", "")
	tool := SearchXTool{}
	if _, err := tool.Execute(context.Background(), json.RawMessage(`{"query":"x"}`)); err == nil || !strings.Contains(err.Error(), "XAI_API_KEY") {
		t.Fatalf("err = %v", err)
	}
}

func TestSearchXErrorStatus(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		fmt.Fprint(w, `{"error":"bad key"}`)
	}))
	defer srv.Close()
	oldBase := xAISearchBaseURL
	xAISearchBaseURL = srv.URL
	defer func() { xAISearchBaseURL = oldBase }()
	t.Setenv("XAI_API_KEY", "k")

	tool := SearchXTool{}
	if _, err := tool.Execute(context.Background(), json.RawMessage(`{"query":"x"}`)); err == nil || !strings.Contains(err.Error(), "401") {
		t.Fatalf("err = %v", err)
	}
}

func TestSearchXNoChoices(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		fmt.Fprint(w, `{"choices":[]}`)
	}))
	defer srv.Close()
	oldBase := xAISearchBaseURL
	xAISearchBaseURL = srv.URL
	defer func() { xAISearchBaseURL = oldBase }()
	t.Setenv("XAI_API_KEY", "k")

	out, err := SearchXTool{}.Execute(context.Background(), json.RawMessage(`{"query":"x"}`))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, "no results") {
		t.Fatalf("out = %q", out)
	}
}

func TestSearchXGrokKeyFallback(t *testing.T) {
	t.Setenv("XAI_API_KEY", "")
	t.Setenv("GROK_API_KEY", "grok-key")
	if got := xAISearchKey(); got != "grok-key" {
		t.Fatalf("key = %q", got)
	}
}
