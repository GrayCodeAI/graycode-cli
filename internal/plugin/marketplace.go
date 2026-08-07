package plugin

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/GrayCodeAI/hawk/internal/flags"
	"github.com/GrayCodeAI/hawk/internal/fsutil"
	"github.com/GrayCodeAI/hawk/internal/storage"
)

// MarketplaceEntry is one installable plugin package in a marketplace index.
type MarketplaceEntry struct {
	Name        string   `json:"name"`
	Description string   `json:"description"`
	Repo        string   `json:"repo"` // owner/name or full git URL
	Version     string   `json:"version,omitempty"`
	Tags        []string `json:"tags,omitempty"`
	Homepage    string   `json:"homepage,omitempty"`
}

// MarketplaceIndex is the JSON schema for a marketplace source.
type MarketplaceIndex struct {
	Version   int                `json:"version"`
	UpdatedAt string             `json:"updated_at,omitempty"`
	Plugins   []MarketplaceEntry `json:"plugins"`
}

// MarketplaceSource is a named index URL.
type MarketplaceSource struct {
	Name string `json:"name"`
	URL  string `json:"url"`
}

// DefaultMarketplaceSources returns built-in sources.
// Official GrayCodeAI plugin index (may 404 until published — callers handle).
func DefaultMarketplaceSources() []MarketplaceSource {
	return []MarketplaceSource{
		{
			Name: "official",
			URL:  "https://raw.githubusercontent.com/GrayCodeAI/hawk-community-skills/main/plugins-registry.json",
		},
	}
}

// MarketplaceClient fetches plugin marketplace indexes and installs packages.
type MarketplaceClient struct {
	Sources  []MarketplaceSource
	CacheDir string
	client   *http.Client
}

// NewMarketplaceClient creates a client with default sources plus user-configured ones.
func NewMarketplaceClient() *MarketplaceClient {
	sources := DefaultMarketplaceSources()
	sources = append(sources, loadUserMarketplaceSources()...)
	return &MarketplaceClient{
		Sources:  sources,
		CacheDir: filepath.Join(storage.CacheDir(), "marketplace"),
		client:   &http.Client{Timeout: 20 * time.Second},
	}
}

func loadUserMarketplaceSources() []MarketplaceSource {
	path := filepath.Join(storage.ConfigDir(), "marketplace-sources.json")
	data, err := os.ReadFile(path) // #nosec G304 -- fixed config path
	if err != nil {
		return nil
	}
	var srcs []MarketplaceSource
	if json.Unmarshal(data, &srcs) != nil {
		return nil
	}
	return srcs
}

// SaveUserSources writes extra marketplace sources to config.
func SaveUserSources(srcs []MarketplaceSource) error {
	path := filepath.Join(storage.ConfigDir(), "marketplace-sources.json")
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	data, err := json.MarshalIndent(srcs, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0o600)
}

// AddSource appends a marketplace source and persists it.
func AddSource(name, url string) error {
	existing := loadUserMarketplaceSources()
	for _, s := range existing {
		if s.Name == name || s.URL == url {
			return fmt.Errorf("source already registered: %s", name)
		}
	}
	existing = append(existing, MarketplaceSource{Name: name, URL: url})
	return SaveUserSources(existing)
}

// FetchAll downloads indexes from all sources and merges entries (first wins).
func (mc *MarketplaceClient) FetchAll() ([]MarketplaceEntry, error) {
	seen := map[string]bool{}
	var all []MarketplaceEntry
	var lastErr error
	for _, src := range mc.Sources {
		idx, err := mc.fetchOne(src)
		if err != nil {
			lastErr = err
			continue
		}
		for _, e := range idx.Plugins {
			key := strings.ToLower(e.Name)
			if seen[key] {
				continue
			}
			seen[key] = true
			all = append(all, e)
		}
	}
	if len(all) == 0 && lastErr != nil {
		return nil, lastErr
	}
	return all, nil
}

func (mc *MarketplaceClient) fetchOne(src MarketplaceSource) (*MarketplaceIndex, error) {
	_ = os.MkdirAll(mc.CacheDir, 0o750)
	cachePath := filepath.Join(mc.CacheDir, sanitizeName(src.Name)+".json")

	if info, err := os.Stat(cachePath); err == nil && time.Since(info.ModTime()) < time.Hour {
		if data, err := fsutil.ReadPinnedFile(cachePath); err == nil {
			var idx MarketplaceIndex
			if json.Unmarshal(data, &idx) == nil {
				return &idx, nil
			}
		}
	}

	req, err := http.NewRequestWithContext(context.Background(), http.MethodGet, src.URL, nil)
	if err != nil {
		return nil, err
	}
	resp, err := mc.client.Do(req)
	if err != nil {
		return loadCachedMarketplace(cachePath)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		return loadCachedMarketplace(cachePath)
	}
	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return loadCachedMarketplace(cachePath)
	}
	_ = os.WriteFile(cachePath, data, 0o600)
	var idx MarketplaceIndex
	if err := json.Unmarshal(data, &idx); err != nil {
		return nil, err
	}
	return &idx, nil
}

func loadCachedMarketplace(path string) (*MarketplaceIndex, error) {
	data, err := os.ReadFile(path) // #nosec G304
	if err != nil {
		return nil, err
	}
	var idx MarketplaceIndex
	if err := json.Unmarshal(data, &idx); err != nil {
		return nil, err
	}
	return &idx, nil
}

// Find looks up a plugin by name across all sources.
func (mc *MarketplaceClient) Find(name string) (*MarketplaceEntry, error) {
	all, err := mc.FetchAll()
	if err != nil {
		return nil, err
	}
	want := strings.ToLower(strings.TrimSpace(name))
	for i := range all {
		if strings.ToLower(all[i].Name) == want {
			return &all[i], nil
		}
	}
	return nil, fmt.Errorf("plugin %q not found in marketplace", name)
}

// Install installs a marketplace entry into the user plugins directory.
// Requires flags.Marketplace() to be enabled.
func (mc *MarketplaceClient) Install(entry MarketplaceEntry) (string, error) {
	if !flags.Marketplace() {
		return "", fmt.Errorf("marketplace installs disabled — set HAWK_Y0_MARKETPLACE=1 to enable")
	}
	if entry.Repo == "" {
		return "", fmt.Errorf("marketplace entry %q has no repo", entry.Name)
	}
	destRoot := filepath.Join(storage.StateDir(), "plugins")
	if err := os.MkdirAll(destRoot, 0o750); err != nil {
		return "", err
	}
	name := entry.Name
	if name == "" {
		name = filepath.Base(entry.Repo)
	}
	pluginDir := filepath.Join(destRoot, sanitizeName(name))
	if _, err := os.Stat(pluginDir); err == nil {
		return "", fmt.Errorf("plugin directory already exists: %s", pluginDir)
	}

	url := entry.Repo
	if strings.HasPrefix(url, "git@") {
		// scp-style URLs (git@host:user/repo.git) bypass the HTTPS transport
		// and cannot be pinned or verified; refuse them.
		return "", fmt.Errorf("marketplace entry %q uses an unsupported scp-style repo URL %q; use an https URL", entry.Name, entry.Repo)
	}
	if !strings.HasPrefix(url, "http://") && !strings.HasPrefix(url, "https://") {
		url = "https://github.com/" + strings.TrimSuffix(entry.Repo, ".git") + ".git"
	}

	cmd := exec.CommandContext(context.Background(), "git", "clone", "--depth", "1", "--single-branch", "--", url, pluginDir) // #nosec G204
	out, err := cmd.CombinedOutput()
	if err != nil {
		_ = os.RemoveAll(pluginDir)
		return "", fmt.Errorf("git clone: %w\n%s", err, string(out))
	}

	// Security scan: a plugin without a plugin.json manifest cannot be
	// verified, so fail closed rather than install unverifiable code.
	if _, err := os.Stat(filepath.Join(pluginDir, "plugin.json")); err != nil {
		_ = os.RemoveAll(pluginDir)
		return "", fmt.Errorf("plugin %q has no plugin.json manifest; refusing to install unverifiable plugin", entry.Name)
	}
	if issues := criticalPluginIssues(ScanPlugin(pluginDir)); len(issues) > 0 {
		_ = os.RemoveAll(pluginDir)
		return "", fmt.Errorf("plugin security scan failed: %s", strings.Join(issues, "; "))
	}
	return pluginDir, nil
}

func sanitizeName(s string) string {
	s = strings.TrimSpace(s)
	s = strings.ReplaceAll(s, "/", "-")
	s = strings.ReplaceAll(s, "\\", "-")
	if s == "" {
		return "plugin"
	}
	return s
}
