package update

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"runtime"
	"strings"
)

const updateURL = "https://api.github.com/repos/GrayCodeAI/hawk/releases/latest"

// ReleaseInfo represents a GitHub release.
type ReleaseInfo struct {
	TagName string `json:"tag_name"`
	Name    string `json:"name"`
	Body    string `json:"body"`
	URL     string `json:"html_url"`
}

// Check checks for available updates.
func Check(currentVersion string) (*ReleaseInfo, error) {
	req, err := http.NewRequest("GET", updateURL, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "application/vnd.github.v3+json")
	req.Header.Set("User-Agent", "hawk-cli")

	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("update check failed: %s", resp.Status)
	}

	body, err := io.ReadAll(resp.Body)
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

// isNewer checks if version a is newer than version b.
func isNewer(a, b string) bool {
	// Simple semver comparison
	a = strings.TrimPrefix(a, "v")
	b = strings.TrimPrefix(b, "v")
	return a != b && a > b
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
