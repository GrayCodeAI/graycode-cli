package plugin

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"
)

// RegistryEntry describes a plugin available in the marketplace.
type RegistryEntry struct {
	Name        string   `json:"name"`
	Version     string   `json:"version"`
	Description string   `json:"description"`
	Author      string   `json:"author"`
	Repository  string   `json:"repository"`
	Tags        []string `json:"tags,omitempty"`
	Downloads   int      `json:"downloads"`
	UpdatedAt   time.Time `json:"updated_at"`
	Verified    bool     `json:"verified"`
}

// Marketplace manages plugin discovery, search, and installation from a registry.
type Marketplace struct {
	mu          sync.RWMutex
	registryURL string
	cache       []RegistryEntry
	cacheTime   time.Time
	cacheTTL    time.Duration
	client      *http.Client
}

// NewMarketplace creates a marketplace client.
func NewMarketplace(registryURL string) *Marketplace {
	return &Marketplace{
		registryURL: registryURL,
		cacheTTL:    15 * time.Minute,
		client:      &http.Client{Timeout: 30 * time.Second},
	}
}

// Search finds plugins matching a query.
func (m *Marketplace) Search(ctx context.Context, query string) ([]RegistryEntry, error) {
	entries, err := m.fetchRegistry(ctx)
	if err != nil {
		return nil, err
	}

	if query == "" {
		return entries, nil
	}

	query = strings.ToLower(query)
	var results []RegistryEntry
	for _, e := range entries {
		if strings.Contains(strings.ToLower(e.Name), query) ||
			strings.Contains(strings.ToLower(e.Description), query) ||
			matchesTags(e.Tags, query) {
			results = append(results, e)
		}
	}
	return results, nil
}

// Featured returns the most popular plugins.
func (m *Marketplace) Featured(ctx context.Context, limit int) ([]RegistryEntry, error) {
	entries, err := m.fetchRegistry(ctx)
	if err != nil {
		return nil, err
	}

	sort.Slice(entries, func(i, j int) bool {
		return entries[i].Downloads > entries[j].Downloads
	})

	if limit > 0 && limit < len(entries) {
		entries = entries[:limit]
	}
	return entries, nil
}

// Install downloads and installs a plugin by name.
func (m *Marketplace) Install(ctx context.Context, name, version string) error {
	entries, err := m.fetchRegistry(ctx)
	if err != nil {
		return err
	}

	var entry *RegistryEntry
	for i := range entries {
		if entries[i].Name == name {
			entry = &entries[i]
			break
		}
	}
	if entry == nil {
		return fmt.Errorf("plugin %q not found in registry", name)
	}

	if version == "" {
		version = entry.Version
	}

	installDir := pluginInstallDir(name)
	if err := os.MkdirAll(installDir, 0o755); err != nil {
		return fmt.Errorf("creating plugin directory: %w", err)
	}

	// Download plugin manifest
	manifestURL := entry.Repository + "/releases/download/v" + version + "/manifest.json"
	manifest, err := m.download(ctx, manifestURL)
	if err != nil {
		return fmt.Errorf("downloading manifest: %w", err)
	}

	if err := os.WriteFile(filepath.Join(installDir, "manifest.json"), manifest, 0o644); err != nil {
		return err
	}

	// Write version lock
	lock := map[string]string{
		"name":         name,
		"version":      version,
		"installed_at": time.Now().UTC().Format(time.RFC3339),
		"repository":   entry.Repository,
	}
	lockData, _ := json.MarshalIndent(lock, "", "  ")
	return os.WriteFile(filepath.Join(installDir, ".lock.json"), lockData, 0o644)
}

// Upgrade upgrades a plugin to the latest version.
func (m *Marketplace) Upgrade(ctx context.Context, name string) error {
	lockPath := filepath.Join(pluginInstallDir(name), ".lock.json")
	data, err := os.ReadFile(lockPath)
	if err != nil {
		return fmt.Errorf("plugin %q not installed", name)
	}

	var lock map[string]string
	json.Unmarshal(data, &lock)

	entries, err := m.fetchRegistry(ctx)
	if err != nil {
		return err
	}

	for _, e := range entries {
		if e.Name == name {
			if e.Version == lock["version"] {
				return fmt.Errorf("already at latest version %s", e.Version)
			}
			return m.Install(ctx, name, e.Version)
		}
	}
	return fmt.Errorf("plugin %q not found in registry", name)
}

// InstalledVersion returns the installed version of a plugin.
func (m *Marketplace) InstalledVersion(name string) (string, bool) {
	lockPath := filepath.Join(pluginInstallDir(name), ".lock.json")
	data, err := os.ReadFile(lockPath)
	if err != nil {
		return "", false
	}
	var lock map[string]string
	json.Unmarshal(data, &lock)
	return lock["version"], true
}

func (m *Marketplace) fetchRegistry(ctx context.Context) ([]RegistryEntry, error) {
	m.mu.RLock()
	if time.Since(m.cacheTime) < m.cacheTTL && m.cache != nil {
		entries := m.cache
		m.mu.RUnlock()
		return entries, nil
	}
	m.mu.RUnlock()

	req, err := http.NewRequestWithContext(ctx, "GET", m.registryURL+"/plugins.json", nil)
	if err != nil {
		return nil, err
	}

	resp, err := m.client.Do(req)
	if err != nil {
		// Return cache if available
		m.mu.RLock()
		if m.cache != nil {
			entries := m.cache
			m.mu.RUnlock()
			return entries, nil
		}
		m.mu.RUnlock()
		return nil, fmt.Errorf("fetching registry: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("registry returned %d", resp.StatusCode)
	}

	var entries []RegistryEntry
	if err := json.NewDecoder(resp.Body).Decode(&entries); err != nil {
		return nil, fmt.Errorf("decoding registry: %w", err)
	}

	m.mu.Lock()
	m.cache = entries
	m.cacheTime = time.Now()
	m.mu.Unlock()

	return entries, nil
}

func (m *Marketplace) download(ctx context.Context, url string) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return nil, err
	}
	resp, err := m.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("download failed: %d", resp.StatusCode)
	}
	return io.ReadAll(resp.Body)
}

func pluginInstallDir(name string) string {
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".hawk", "plugins", name)
}

func matchesTags(tags []string, query string) bool {
	for _, tag := range tags {
		if strings.Contains(strings.ToLower(tag), query) {
			return true
		}
	}
	return false
}
