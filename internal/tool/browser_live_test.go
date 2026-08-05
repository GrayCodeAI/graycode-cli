package tool

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// TestBrowserLive is an opt-in end-to-end check against a real
// Chrome/Chromium install. CI runs without a display/browser, so the live
// path is exercised in developer environments only.
// TODO: https://github.com/GrayCodeAI/hawk/issues/273 track live-browser CI coverage.
func TestBrowserLive(t *testing.T) {
	if os.Getenv("HAWK_LIVE_BROWSER") == "" {
		t.Skip("requires HAWK_LIVE_BROWSER=1 and a Chrome/Chromium binary")
	}
	t.Cleanup(releaseBrowser)

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	d := t.TempDir()
	shot := filepath.Join(d, "shot.png")

	out, err := BrowserTool{}.Execute(ctx, mustBrowserInput(t, map[string]interface{}{
		"action": "navigate",
		"url":    "https://example.com",
	}))
	if err != nil {
		t.Fatalf("navigate: %v", err)
	}
	if !strings.Contains(out, "example.com") {
		t.Fatalf("unexpected navigate output: %q", out)
	}

	out, err = ScreenshotTool{}.Execute(ctx, mustBrowserInput(t, map[string]interface{}{
		"url":     "https://example.com",
		"path":    shot,
		"wait_ms": 500,
	}))
	if err != nil {
		t.Fatalf("screenshot: %v", err)
	}
	info, err := os.Stat(shot)
	if err != nil {
		t.Fatalf("screenshot file: %v", err)
	}
	if info.Size() == 0 {
		t.Fatalf("screenshot is empty")
	}
	if !strings.HasPrefix(out, "Screenshot saved to") {
		t.Fatalf("unexpected screenshot output: %q", out)
	}

	out, err = BrowserTool{}.Execute(ctx, mustBrowserInput(t, map[string]interface{}{
		"action": "title",
		"url":    "https://example.com",
	}))
	if err != nil {
		t.Fatalf("title: %v", err)
	}
	if !strings.Contains(out, "Example Domain") {
		t.Fatalf("unexpected title: %q", out)
	}
}

func mustBrowserInput(t *testing.T, m map[string]interface{}) json.RawMessage {
	t.Helper()
	b, err := json.Marshal(m)
	if err != nil {
		t.Fatal(err)
	}
	return b
}
