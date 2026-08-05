package tool

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/chromedp/chromedp"
)

// ScreenshotTool captures a full-page PNG of a URL using headless Chrome.
// It is the visual-verification counterpart to BrowserTool's screenshot
// action: a single-shot, one-URL call that returns the saved file path.
type ScreenshotTool struct{}

func (ScreenshotTool) Name() string      { return "Screenshot" }
func (ScreenshotTool) Aliases() []string { return []string{"screenshot"} }
func (ScreenshotTool) RiskLevel() string { return "high" }
func (ScreenshotTool) Description() string {
	return "Capture a full-page screenshot of a URL with headless Chrome and save it as a PNG file."
}

func (ScreenshotTool) Parameters() map[string]interface{} {
	return map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"url":      map[string]interface{}{"type": "string", "description": "URL (http/https) to capture"},
			"path":     map[string]interface{}{"type": "string", "description": "Destination PNG path; defaults to a temp file"},
			"width":    map[string]interface{}{"type": "number", "description": "Viewport width in pixels (default 1280)"},
			"height":   map[string]interface{}{"type": "number", "description": "Viewport height in pixels (default 800)"},
			"wait_ms":  map[string]interface{}{"type": "number", "description": "Milliseconds to wait for the page to render (default 1500)"},
			"selector": map[string]interface{}{"type": "string", "description": "Optional CSS selector to wait for before capturing"},
		},
		"required": []string{"url"},
	}
}

func (ScreenshotTool) Execute(ctx context.Context, input json.RawMessage) (string, error) {
	var p struct {
		URL      string `json:"url"`
		Path     string `json:"path"`
		Width    int    `json:"width"`
		Height   int    `json:"height"`
		WaitMS   int    `json:"wait_ms"`
		Selector string `json:"selector"`
	}
	if err := json.Unmarshal(input, &p); err != nil {
		return "", err
	}
	if err := validateBrowserURL(p.URL); err != nil {
		return "", err
	}

	dest := p.Path
	if dest == "" {
		dest = filepath.Join(os.TempDir(), fmt.Sprintf("hawk-screenshot-%d.png", time.Now().UnixNano()))
	}
	if err := validatePathAllowed(ctx, dest); err != nil {
		return "", err
	}

	wait := time.Duration(p.WaitMS) * time.Millisecond
	if wait <= 0 {
		wait = 1500 * time.Millisecond
	}
	width := p.Width
	if width <= 0 {
		width = 1280
	}
	height := p.Height
	if height <= 0 {
		height = 800
	}

	bctx, err := acquireBrowser()
	if err != nil {
		return "", browserErr(err)
	}

	actions := []chromedp.Action{
		chromedp.EmulateViewport(int64(width), int64(height)),
		chromedp.Navigate(p.URL),
		chromedp.Sleep(wait),
	}
	if p.Selector != "" {
		sel := p.Selector
		actions = append(actions, chromedp.ActionFunc(func(c context.Context) error {
			return waitForSelector(c, sel, 10*time.Second)
		}))
	}
	var buf []byte
	actions = append(actions, chromedp.FullScreenshot(&buf, 100))
	if err := chromedp.Run(bctx, actions...); err != nil {
		return "", browserErr(err)
	}
	if err := os.WriteFile(dest, buf, 0o644); err != nil {
		return "", fmt.Errorf("write screenshot: %w", err)
	}
	return fmt.Sprintf("Screenshot saved to %s (%d bytes)", dest, len(buf)), nil
}
