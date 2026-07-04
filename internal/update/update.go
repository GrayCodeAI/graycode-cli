package update

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"runtime"
	"strings"
	"time"
)

// checkTimeout bounds the time a single update-check HTTP request can take.
// The package's public entry point runs the request through context.Background
// (so it cannot be cancelled by the caller), so without an http.Client
// Timeout a slow or hijacked response could hang the binary on launch
// forever. 10s is generous for a single GET against api.github.com.
const checkTimeout = 10 * time.Second

// maxResponseBytes caps the body read so a malicious or compromised
// api.github.com response cannot exhaust memory.
const maxResponseBytes = 1 << 20 // 1 MiB

var updateURL = "https://api.github.com/repos/GrayCodeAI/hawk/releases/latest"

func setUpdateURL(url string) {
	updateURL = url
}

// ReleaseInfo represents a GitHub release.
type ReleaseInfo struct {
	TagName string `json:"tag_name"`
	Name    string `json:"name"`
	Body    string `json:"body"`
	URL     string `json:"html_url"`
}

// Check checks for available updates.
func Check(currentVersion string) (*ReleaseInfo, error) {
	req, err := http.NewRequestWithContext(context.Background(), http.MethodGet, updateURL, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "application/vnd.github.v3+json")
	req.Header.Set("User-Agent", "hawk-cli")

	client := &http.Client{Timeout: checkTimeout}
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("update check failed: %s", resp.Status)
	}

	body, err := io.ReadAll(io.LimitReader(resp.Body, maxResponseBytes))
	if err != nil {
		return nil, err
	}

	var release ReleaseInfo
	if err := json.Unmarshal(body, &release); err != nil {
		return nil, err
	}

	if isNewer(release.TagName, currentVersion) {
		return &release, nil
	}
	return nil, nil // no update available
}

// isNewer checks if version a is newer than version b using semver ordering
// (numeric field comparison; a pre-release sorts before its release).
func isNewer(a, b string) bool {
	na, pa, okA := parseVersion(a)
	if !okA {
		return false
	}
	nb, pb, okB := parseVersion(b)
	if !okB {
		return true
	}
	for i := range na {
		if na[i] != nb[i] {
			return na[i] > nb[i]
		}
	}
	if pa == pb {
		return false
	}
	if pa == "" {
		return true // release is newer than its pre-release
	}
	if pb == "" {
		return false
	}
	return pa > pb
}

// parseVersion parses "v1.2.3-rc1" into numeric fields and a pre-release tag.
func parseVersion(v string) (nums [3]int, pre string, ok bool) {
	v = strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(v), "v"))
	if v == "" {
		return nums, "", false
	}
	v, _, _ = strings.Cut(v, "+") // drop build metadata
	v, pre, _ = strings.Cut(v, "-")
	parts := strings.Split(v, ".")
	for i := 0; i < len(parts) && i < 3; i++ {
		n := 0
		if parts[i] == "" {
			return nums, "", false
		}
		for _, c := range parts[i] {
			if c < '0' || c > '9' {
				return nums, "", false
			}
			n = n*10 + int(c-'0')
		}
		nums[i] = n
	}
	return nums, pre, true
}

// Summary returns a formatted update summary.
func Summary(currentVersion string) string {
	release, err := Check(currentVersion)
	if err != nil {
		return fmt.Sprintf("Update check failed: %v", err)
	}
	if release == nil {
		return fmt.Sprintf("hawk is up to date (%s)", currentVersion)
	}
	return fmt.Sprintf("Update available: %s -> %s\n%s\n\nRelease notes:\n%s",
		currentVersion, release.TagName, release.URL, release.Body)
}

// Platform returns the current platform identifier.
func Platform() string {
	goos := runtime.GOOS
	goarch := runtime.GOARCH
	if goos == "darwin" {
		goos = "macos"
	}
	return fmt.Sprintf("%s-%s", goos, goarch)
}
