package search

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestJinaReader_DisabledByDefault(t *testing.T) {
	t.Parallel()
	r := NewJinaReader()
	if r.Enabled {
		t.Fatal("NewJinaReader should default to disabled")
	}
	if r.Available() {
		t.Fatal("Available() should be false when disabled")
	}
	if _, err := r.FetchMarkdown(context.Background(), "https://example.com"); err == nil {
		t.Fatal("expected error when disabled")
	}
}

func TestJinaReader_FetchMarkdown(t *testing.T) {
	t.Parallel()

	var gotPath string
	var gotFormat, gotEngine string
	var gotAuth string
	var gotBody jinaRequest

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		gotFormat = r.Header.Get("X-Return-Format")
		gotEngine = r.Header.Get("X-Engine")
		gotAuth = r.Header.Get("Authorization")
		b, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(b, &gotBody)
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("Title: Docs\n\nURL Source: https://example.com\n\nMarkdown Content:\n# Hello\n\nbody text"))
	}))
	defer srv.Close()

	testKey := "test-key"
	r := &JinaReader{Enabled: true, BaseURL: srv.URL, APIKey: testKey}

	out, err := r.FetchMarkdown(context.Background(), "https://example.com/page")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if gotPath != "/" {
		t.Errorf("request path = %q, want /", gotPath)
	}
	if gotFormat != "markdown" {
		t.Errorf("X-Return-Format = %q, want markdown", gotFormat)
	}
	if gotEngine != "direct" {
		t.Errorf("X-Engine = %q, want direct", gotEngine)
	}
	if gotAuth != "Bearer test-key" {
		t.Errorf("Authorization = %q", gotAuth)
	}
	if gotBody.URL != "https://example.com/page" {
		t.Errorf("body url = %q", gotBody.URL)
	}
	want := "# Hello\n\nbody text"
	if strings.TrimSpace(out) != want {
		t.Errorf("output = %q, want %q", out, want)
	}
}

func TestJinaReader_NoPreamble(t *testing.T) {
	t.Parallel()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("# Just markdown\n\nclean output"))
	}))
	defer srv.Close()

	r := NewJinaReader()
	r.Enabled = true
	r.BaseURL = srv.URL

	out, err := r.FetchMarkdown(context.Background(), "https://example.com")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if out != "# Just markdown\n\nclean output" {
		t.Errorf("output = %q", out)
	}
}

func TestJinaReader_Non200(t *testing.T) {
	t.Parallel()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "rate limited", http.StatusTooManyRequests)
	}))
	defer srv.Close()

	r := NewJinaReader()
	r.Enabled = true
	r.BaseURL = srv.URL

	if _, err := r.FetchMarkdown(context.Background(), "https://example.com"); err == nil {
		t.Fatal("expected error for non-200 status")
	}
}

func TestJinaReader_InputValidation(t *testing.T) {
	t.Parallel()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
	defer srv.Close()

	r := NewJinaReader()
	r.Enabled = true
	r.BaseURL = srv.URL

	for _, tc := range []struct {
		name, url string
	}{
		{"empty", ""},
		{"whitespace", "   "},
		{"not-http", "ftp://example.com"},
	} {
		if _, err := r.FetchMarkdown(context.Background(), tc.url); err == nil {
			t.Errorf("%s: expected error", tc.name)
		}
	}
}

func TestStripJinaPreamble(t *testing.T) {
	t.Parallel()
	cases := []struct{ name, in, want string }{
		{
			name: "full preamble",
			in:   "Title: T\n\nURL Source: https://x\n\nMarkdown Content:\n# body",
			want: "# body",
		},
		{
			name: "no preamble",
			in:   "# body",
			want: "# body",
		},
		{
			name: "partial preamble stops at first non-meta",
			in:   "Title: T\n# not meta\nURL Source: x",
			want: "# not meta\nURL Source: x",
		},
	}
	for _, tc := range cases {
		if got := stripJinaPreamble(tc.in); got != tc.want {
			t.Errorf("%s: stripJinaPreamble = %q, want %q", tc.name, got, tc.want)
		}
	}
}
