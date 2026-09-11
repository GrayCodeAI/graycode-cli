package update

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/GrayCodeAI/hawk/internal/testutil"
)

func TestIsNewer(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name  string
		a     string
		b     string
		newer bool
	}{
		{"patch bump", "1.0.1", "1.0.0", true},
		{"same version", "1.0.0", "1.0.0", false},
		{"minor bump", "1.1.0", "1.0.0", true},
		{"major bump", "2.0.0", "1.0.0", true},
		{"older version", "1.0.0", "1.0.1", false},
		{"with v prefix", "v1.0.1", "v1.0.0", true},
		{"mixed v prefix", "v1.0.1", "1.0.0", true},
		{"empty a", "", "1.0.0", false},
		{"empty b", "1.0.0", "", true},
		{"both empty", "", "", false},
		{"pre-release older than release", "1.0.0-alpha", "1.0.0", false},
		{"release newer than its pre-release", "1.0.0", "1.0.0-alpha", true},
		{"pre-release ordering", "1.0.0-beta", "1.0.0-alpha", true},
		{"dev version", "0.4.0", "0.3.9", true},
		{"double-digit minor", "0.10.0", "0.9.0", true},
		{"double-digit minor reversed", "0.9.0", "0.10.0", false},
		{"double-digit patch", "1.0.10", "1.0.9", true},
		{"two-part version", "1.2", "1.1.9", true},
		{"build metadata ignored", "1.0.0+build5", "1.0.0", false},
		{"garbage a", "not-a-version", "1.0.0", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			result := isNewer(tt.a, tt.b)
			if result != tt.newer {
				t.Errorf("isNewer(%q, %q) = %v, want %v", tt.a, tt.b, result, tt.newer)
			}
		})
	}
}

func TestPlatform(t *testing.T) {
	t.Parallel()
	p := Platform()
	if p == "" {
		t.Fatal("expected non-empty platform")
	}
	parts := strings.Split(p, "-")
	if len(parts) != 2 {
		t.Errorf("Platform() = %q, want format 'os-arch'", p)
	}
	if parts[1] != "amd64" && parts[1] != "arm64" {
		t.Errorf("unexpected arch %q in Platform()", parts[1])
	}
}

func TestCheck(t *testing.T) {
	t.Run("newer version available", func(t *testing.T) {
		release := ReleaseInfo{
			TagName: "v1.0.0",
			Name:    "Release 1.0.0",
			Body:    "Bug fixes and improvements",
			URL:     "https://github.com/GrayCodeAI/hawk/releases/tag/v1.0.0",
		}
		server := testutil.NewLoopbackHTTPServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(release)
		}))
		defer server.Close()

		origURL := updateURL
		setUpdateURL(server.URL)
		defer setUpdateURL(origURL)

		result, err := Check("0.9.0")
		if err != nil {
			t.Fatalf("Check() error = %v", err)
		}
		if result == nil {
			t.Fatal("Check() returned nil, expected release info")
		}
		if result.TagName != "v1.0.0" {
			t.Errorf("TagName = %q, want %q", result.TagName, "v1.0.0")
		}
	})

	t.Run("no update available", func(t *testing.T) {
		release := ReleaseInfo{
			TagName: "v0.1.0",
			Name:    "Current Release",
		}
		server := testutil.NewLoopbackHTTPServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(release)
		}))
		defer server.Close()

		origURL := updateURL
		setUpdateURL(server.URL)
		defer setUpdateURL(origURL)

		result, err := Check("0.1.0")
		if err != nil {
			t.Fatalf("Check() error = %v", err)
		}
		if result != nil {
			t.Errorf("Check() = %v, want nil (no update)", result)
		}
	})

	t.Run("server error", func(t *testing.T) {
		server := testutil.NewLoopbackHTTPServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusInternalServerError)
		}))
		defer server.Close()

		origURL := updateURL
		setUpdateURL(server.URL)
		defer setUpdateURL(origURL)

		_, err := Check("0.1.0")
		if err == nil {
			t.Error("Check() should return error on server failure")
		}
	})

	t.Run("invalid JSON response", func(t *testing.T) {
		server := testutil.NewLoopbackHTTPServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte("not json"))
		}))
		defer server.Close()

		origURL := updateURL
		setUpdateURL(server.URL)
		defer setUpdateURL(origURL)

		_, err := Check("0.1.0")
		if err == nil {
			t.Error("Check() should return error on invalid JSON")
		}
	})

	t.Run("unreachable server", func(t *testing.T) {
		origURL := updateURL
		setUpdateURL("http://" + testutil.LoopbackHost + ":1")
		defer setUpdateURL(origURL)

		_, err := Check("0.1.0")
		if err == nil {
			t.Error("Check() should return error for unreachable server")
		}
	})

	t.Run("slow server is bounded by client timeout", func(t *testing.T) {
		// Server that takes longer than checkTimeout to start writing
		// the response. The client.Timeout should abort the request
		// well before the server would have responded.
		server := testutil.NewLoopbackHTTPServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			time.Sleep(15 * time.Second)
		}))
		// server.Close() blocks on the still-active connection; kick
		// the close off in a goroutine and bound the wait so the test
		// finishes promptly once the assertion has run.
		t.Cleanup(func() {
			done := make(chan struct{})
			go func() { server.Close(); close(done) }()
			select {
			case <-done:
			case <-time.After(2 * time.Second):
				// best-effort: the test has already passed; the
				// runner's process cleanup will finish the job
			}
		})

		origURL := updateURL
		setUpdateURL(server.URL)
		t.Cleanup(func() { setUpdateURL(origURL) })

		start := time.Now()
		_, err := Check("0.1.0")
		elapsed := time.Since(start)
		if err == nil {
			t.Fatal("Check() should error when the server stalls")
		}
		// checkTimeout is 10s; allow a couple seconds of slack for
		// goroutine scheduling on slow CI but require the timeout
		// to have fired (the server sleeps 15s).
		if elapsed > 12*time.Second {
			t.Errorf("Check() took %v; expected to be bounded by checkTimeout (10s)", elapsed)
		}
	})

	t.Run("oversize response is bounded", func(t *testing.T) {
		// Server returns > 1 MiB; the LimitReader in Check caps the
		// body read so JSON unmarshal of a normal release still
		// succeeds (release fields are populated from the prefix).
		server := testutil.NewLoopbackHTTPServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"tag_name":"v9.9.9","name":"x","body":"`))
			pad := strings.Repeat("a", 2<<20)
			_, _ = w.Write([]byte(pad))
			_, _ = w.Write([]byte(`","html_url":"https://example"}`))
		}))
		defer server.Close()

		origURL := updateURL
		setUpdateURL(server.URL)
		defer setUpdateURL(origURL)

		// We don't care whether Check parses the truncated body — only
		// that it returns *without* exhausting memory. The 1 MiB cap
		// guarantees allocation stays bounded.
		_, _ = Check("0.1.0")
	})
}

func TestSummary(t *testing.T) {
	t.Run("update available", func(t *testing.T) {
		release := ReleaseInfo{
			TagName: "v2.0.0",
			Name:    "Major Release",
			Body:    "Breaking changes",
			URL:     "https://github.com/GrayCodeAI/hawk/releases/tag/v2.0.0",
		}
		server := testutil.NewLoopbackHTTPServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(release)
		}))
		defer server.Close()

		origURL := updateURL
		setUpdateURL(server.URL)
		defer setUpdateURL(origURL)

		result := Summary("1.0.0")
		if !strings.Contains(result, "Update available") {
			t.Errorf("Summary() = %q, want to contain 'Update available'", result)
		}
		if !strings.Contains(result, "v2.0.0") {
			t.Errorf("Summary() = %q, want to contain version", result)
		}
	})

	t.Run("up to date", func(t *testing.T) {
		release := ReleaseInfo{TagName: "v0.1.0"}
		server := testutil.NewLoopbackHTTPServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(release)
		}))
		defer server.Close()

		origURL := updateURL
		setUpdateURL(server.URL)
		defer setUpdateURL(origURL)

		result := Summary("0.1.0")
		if !strings.Contains(result, "up to date") {
			t.Errorf("Summary() = %q, want to contain 'up to date'", result)
		}
	})
}

func TestReleaseInfo(t *testing.T) {
	t.Parallel()

	t.Run("json marshaling", func(t *testing.T) {
		t.Parallel()
		info := ReleaseInfo{
			TagName: "v1.0.0",
			Name:    "First Release",
			Body:    "Initial release",
			URL:     "https://example.com",
		}
		data, err := json.Marshal(info)
		if err != nil {
			t.Fatalf("Marshal error: %v", err)
		}

		var decoded ReleaseInfo
		if err := json.Unmarshal(data, &decoded); err != nil {
			t.Fatalf("Unmarshal error: %v", err)
		}
		if decoded.TagName != info.TagName {
			t.Errorf("TagName = %q, want %q", decoded.TagName, info.TagName)
		}
	})
}
