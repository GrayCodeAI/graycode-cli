package tool

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"time"
)

// DownloadTool downloads a file from a URL to a local path.
type DownloadTool struct{}

func (DownloadTool) Name() string      { return "Download" }
func (DownloadTool) RiskLevel() string  { return "medium" }
func (DownloadTool) Aliases() []string  { return []string{"download"} }
func (DownloadTool) Description() string {
	return "Download a file from a URL and save it to a local path."
}
func (DownloadTool) Parameters() map[string]interface{} {
	return map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"url":         map[string]interface{}{"type": "string", "description": "URL to download from"},
			"destination": map[string]interface{}{"type": "string", "description": "Local file path to save to"},
		},
	}
}

const maxDownloadSize = 50 * 1024 * 1024 // 50MB

func (DownloadTool) Execute(ctx context.Context, input json.RawMessage) (string, error) {
	var p struct {
		URL         string `json:"url"`
		Destination string `json:"destination"`
	}
	if err := json.Unmarshal(input, &p); err != nil {
		return "", err
	}
	if p.URL == "" || p.Destination == "" {
		return "", fmt.Errorf("url and destination are required")
	}
	if err := validatePathAllowed(ctx, p.Destination); err != nil {
		return "", err
	}

	client := &http.Client{Timeout: 2 * time.Minute}
	resp, err := client.Get(p.URL)
	if err != nil {
		return "", fmt.Errorf("download failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("HTTP %d: %s", resp.StatusCode, resp.Status)
	}

	os.MkdirAll(filepath.Dir(p.Destination), 0o755)
	f, err := os.Create(p.Destination)
	if err != nil {
		return "", fmt.Errorf("create file: %w", err)
	}
	defer f.Close()

	n, err := io.Copy(f, io.LimitReader(resp.Body, maxDownloadSize))
	if err != nil {
		return "", fmt.Errorf("write file: %w", err)
	}

	ct := resp.Header.Get("Content-Type")
	return fmt.Sprintf("Downloaded %d bytes to %s (type: %s)", n, p.Destination, ct), nil
}
