package tool

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/chromedp/chromedp"
)

// Shared headless browser. The exec allocator is created lazily on first use
// and reused across calls so repeated navigations do not pay a Chrome startup
// cost. All access must go through acquireBrowser/releaseBrowser so the mutex
// guards the allocator lifecycle.
var (
	browserMu        sync.Mutex
	browserAllocator *execBrowser
)

// execBrowser wraps the chromedp exec allocator context and its cancel func.
type execBrowser struct {
	ctx         context.Context
	cancel      context.CancelFunc // cancels the browser context
	cancelAlloc context.CancelFunc // cancels the exec allocator (kills Chrome)
}

// acquireBrowser returns the chromedp context built on the shared allocator,
// creating the allocator on first use.
func acquireBrowser() (context.Context, error) {
	browserMu.Lock()
	defer browserMu.Unlock()
	if browserAllocator == nil {
		abctx, cancelAlloc := chromedp.NewExecAllocator(
			context.Background(),
			chromedp.Headless,
			chromedp.NoSandbox,
			chromedp.NoFirstRun,
			chromedp.NoDefaultBrowserCheck,
			chromedp.DisableGPU,
			chromedp.WindowSize(1280, 800),
		)
		ebctx, cancelBrowser := chromedp.NewContext(abctx)
		browserAllocator = &execBrowser{
			ctx:         ebctx,
			cancel:      cancelBrowser,
			cancelAlloc: cancelAlloc,
		}
	}
	return browserAllocator.ctx, nil
}

// releaseBrowser tears down the shared browser so its Chrome process exits.
// Safe to call multiple times; no-op when nothing is running.
func releaseBrowser() {
	browserMu.Lock()
	defer browserMu.Unlock()
	if browserAllocator != nil {
		browserAllocator.cancel()
		browserAllocator.cancelAlloc()
		browserAllocator = nil
	}
}

// missingChromeHint is returned when a headless browser binary cannot be found.
const missingChromeHint = "no Chrome/Chromium executable found; install Chrome or set CHROME_PATH"

// browserErr wraps raw chromedp failures with the missing-browser hint when
// the failure happens during allocator startup.
func browserErr(err error) error {
	if err == nil {
		return nil
	}
	if strings.Contains(err.Error(), "exec:") || strings.Contains(err.Error(), "executable file not found") {
		return fmt.Errorf("%s: %w", missingChromeHint, err)
	}
	return err
}

// validateBrowserURL allows http/https (including localhost and LAN dev
// servers, which are common screenshot targets) but blocks schemes that would
// reach into local files or privileged endpoints.
func validateBrowserURL(raw string) error {
	if raw == "" {
		return fmt.Errorf("url is required")
	}
	u, err := url.Parse(raw)
	if err != nil {
		return fmt.Errorf("invalid url: %w", err)
	}
	switch strings.ToLower(u.Scheme) {
	case "http", "https":
	default:
		return fmt.Errorf("only http and https urls are supported, got %q", u.Scheme)
	}
	if u.Hostname() == "" {
		return fmt.Errorf("url must include a host")
	}
	return nil
}

// BrowserTool drives a headless Chrome browser through the Chrome DevTools
// Protocol. It covers browser automation (navigate, click, type) and content
// extraction (title, location, text/html), and can persist full-page
// screenshots for visual verification.
type BrowserTool struct{}

func (BrowserTool) Name() string      { return "Browser" }
func (BrowserTool) Aliases() []string { return []string{"browser"} }
func (BrowserTool) RiskLevel() string { return "high" }
func (BrowserTool) Description() string {
	return "Control a headless Chrome browser: navigate to URLs, click and type into elements, extract page text/HTML/title, and take screenshots."
}

func (BrowserTool) Parameters() map[string]interface{} {
	return map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"action": map[string]interface{}{
				"type":        "string",
				"enum":        []string{"navigate", "content", "screenshot", "click", "type", "title", "location", "close"},
				"description": "Action to perform. navigate: go to a URL (optionally waiting for a selector). content: extract page text or HTML (optionally scoped to a selector). screenshot: save a full-page PNG. click/type: interact with an element. title/location: read page metadata. close: shut down the shared browser.",
			},
			"url":      map[string]interface{}{"type": "string", "description": "Target URL (http/https) for navigate/screenshot"},
			"selector": map[string]interface{}{"type": "string", "description": "CSS selector for content/click/type and optional navigate wait"},
			"text":     map[string]interface{}{"type": "string", "description": "Text to type (type action)"},
			"clear":    map[string]interface{}{"type": "boolean", "description": "Clear the field before typing"},
			"path":     map[string]interface{}{"type": "string", "description": "File path to save a screenshot to"},
			"wait_ms":  map[string]interface{}{"type": "number", "description": "Milliseconds to wait after navigation (default 800)"},
			"html":     map[string]interface{}{"type": "boolean", "description": "Return outer HTML instead of inner text (content action)"},
			"max_chars": map[string]interface{}{
				"type": "number", "description": "Truncate extracted content to this many characters (default 20000)",
			},
		},
		"required": []string{"action"},
	}
}

// browserParams mirrors the declared JSON schema.
type browserParams struct {
	Action   string `json:"action"`
	URL      string `json:"url"`
	Selector string `json:"selector"`
	Text     string `json:"text"`
	Clear    bool   `json:"clear"`
	Path     string `json:"path"`
	WaitMS   int    `json:"wait_ms"`
	HTML     bool   `json:"html"`
	MaxChars int    `json:"max_chars"`
}

func (BrowserTool) Execute(ctx context.Context, input json.RawMessage) (string, error) {
	var p browserParams
	if err := json.Unmarshal(input, &p); err != nil {
		return "", err
	}
	p.Action = strings.ToLower(strings.TrimSpace(p.Action))
	if p.Action == "" {
		return "", fmt.Errorf("action is required")
	}

	wait := time.Duration(p.WaitMS) * time.Millisecond
	if wait <= 0 {
		wait = 800 * time.Millisecond
	}
	maxChars := p.MaxChars
	if maxChars <= 0 {
		maxChars = 20000
	}

	if p.Action == "close" {
		releaseBrowser()
		return "Browser closed.", nil
	}

	if err := validateBrowserURL(p.URL); err != nil {
		return "", err
	}

	bctx, err := acquireBrowser()
	if err != nil {
		return "", browserErr(err)
	}

	var out string

	switch p.Action {
	case "navigate":
		actions := []chromedp.Action{
			chromedp.Navigate(p.URL),
			chromedp.Sleep(wait),
		}
		if p.Selector != "" {
			sel := p.Selector
			actions = append(actions, chromedp.ActionFunc(func(c context.Context) error {
				return waitForSelector(c, sel, 10*time.Second)
			}))
		}
		var loc string
		actions = append(actions, chromedp.Location(&loc))
		if err := chromedp.Run(bctx, actions...); err != nil {
			return "", browserErr(err)
		}
		out = fmt.Sprintf("Navigated to %s", loc)

	case "title":
		var title string
		if err := chromedp.Run(bctx, chromedp.Navigate(p.URL), chromedp.Sleep(wait), chromedp.Title(&title)); err != nil {
			return "", browserErr(err)
		}
		out = title

	case "content":
		var text string
		var contentAction chromedp.Action
		if p.Selector != "" {
			sel := p.Selector
			if p.HTML {
				contentAction = chromedp.OuterHTML(sel, &text, chromedp.ByQuery)
			} else {
				contentAction = chromedp.Text(sel, &text, chromedp.ByQuery)
			}
		} else if p.HTML {
			contentAction = chromedp.OuterHTML("html", &text, chromedp.ByQuery)
		} else {
			contentAction = chromedp.Evaluate(`document.body.innerText`, &text)
		}
		if err := chromedp.Run(bctx, chromedp.Navigate(p.URL), chromedp.Sleep(wait), contentAction); err != nil {
			return "", browserErr(err)
		}
		out = truncateChars(text, maxChars)

	case "screenshot":
		dest := p.Path
		if dest == "" {
			dest = filepath.Join(os.TempDir(), fmt.Sprintf("hawk-browser-%d.png", time.Now().UnixNano()))
		}
		if err := validatePathAllowed(ctx, dest); err != nil {
			return "", err
		}
		var buf []byte
		if err := chromedp.Run(
			bctx,
			chromedp.Navigate(p.URL),
			chromedp.Sleep(wait),
			chromedp.FullScreenshot(&buf, 100),
		); err != nil {
			return "", browserErr(err)
		}
		if err := os.WriteFile(dest, buf, 0o644); err != nil {
			return "", fmt.Errorf("write screenshot: %w", err)
		}
		out = fmt.Sprintf("Screenshot saved to %s (%d bytes)", dest, len(buf))

	case "click":
		if p.Selector == "" {
			return "", fmt.Errorf("selector is required for click")
		}
		if err := chromedp.Run(bctx, chromedp.Navigate(p.URL), chromedp.Sleep(wait), chromedp.Click(p.Selector, chromedp.ByQuery)); err != nil {
			return "", browserErr(err)
		}
		out = fmt.Sprintf("Clicked %s", p.Selector)

	case "type":
		if p.Selector == "" {
			return "", fmt.Errorf("selector is required for type")
		}
		sel := p.Selector
		actions := []chromedp.Action{chromedp.Navigate(p.URL), chromedp.Sleep(wait)}
		if p.Clear {
			actions = append(actions, chromedp.Clear(sel, chromedp.ByQuery))
		}
		actions = append(actions, chromedp.SendKeys(sel, p.Text, chromedp.ByQuery))
		if err := chromedp.Run(bctx, actions...); err != nil {
			return "", browserErr(err)
		}
		out = fmt.Sprintf("Typed %d characters into %s", len(p.Text), sel)

	case "location":
		var loc string
		if err := chromedp.Run(bctx, chromedp.Location(&loc)); err != nil {
			return "", browserErr(err)
		}
		out = loc

	default:
		return "", fmt.Errorf("unknown browser action %q", p.Action)
	}

	return out, nil
}

// truncateChars trims s to at most max runes with an ellipsis suffix.
func truncateChars(s string, max int) string {
	s = strings.TrimSpace(s)
	r := []rune(s)
	if len(r) <= max {
		return s
	}
	return string(r[:max]) + "…"
}

// waitForSelector polls document.querySelector via the already-running page
// until the selector matches or the timeout elapses.
func waitForSelector(ctx context.Context, selector string, timeout time.Duration) error {
	deadline := time.Now().Add(timeout)
	for {
		var ok bool
		expr := fmt.Sprintf("!!document.querySelector(%q)", selector)
		if err := chromedp.Run(ctx, chromedp.Evaluate(expr, &ok)); err != nil {
			return fmt.Errorf("wait for selector %q: %w", selector, err)
		}
		if ok {
			return nil
		}
		if time.Now().After(deadline) {
			return fmt.Errorf("timeout waiting for selector %q", selector)
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(100 * time.Millisecond):
		}
	}
}
