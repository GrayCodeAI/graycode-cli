package plugin

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

const defaultIndexURL = "https://raw.githubusercontent.com/GrayCodeAI/hawk-skills/main/registry.json"

// SkillEntry is a single skill in the registry index.
type SkillEntry struct {
	Name        string   `json:"name"`
	Description string   `json:"description"`
	Author      string   `json:"author"`
	Repo        string   `json:"repo"`
	Path        string   `json:"path"`
	Category    string   `json:"category"`
	Tags        []string `json:"tags"`
	Version     string   `json:"version"`
	License     string   `json:"license"`
	Agents      []string `json:"agents"`
	Installs    int      `json:"installs"`
	UpdatedAt   string   `json:"updated_at"`
}

// SkillIndex is the full registry index.
type SkillIndex struct {
	Version   int          `json:"version"`
	UpdatedAt string       `json:"updated_at"`
	Skills    []SkillEntry `json:"skills"`
}

// RegistryClient fetches and queries the community skill registry.
type RegistryClient struct {
	IndexURL string
	CacheDir string
	client   *http.Client
}

// NewRegistryClient creates a registry client with sensible defaults.
func NewRegistryClient() *RegistryClient {
	home, _ := os.UserHomeDir()
	return &RegistryClient{
		IndexURL: defaultIndexURL,
		CacheDir: filepath.Join(home, ".hawk", "cache"),
		client:   &http.Client{Timeout: 15 * time.Second},
	}
}

// FetchIndex downloads the registry index, using a local cache when fresh.
func (rc *RegistryClient) FetchIndex() (*SkillIndex, error) {
	os.MkdirAll(rc.CacheDir, 0o755)
	cachePath := filepath.Join(rc.CacheDir, "skills-index.json")

	// Use cache if less than 1 hour old.
	if info, err := os.Stat(cachePath); err == nil {
		if time.Since(info.ModTime()) < time.Hour {
			data, err := os.ReadFile(cachePath)
			if err == nil {
				var idx SkillIndex
				if json.Unmarshal(data, &idx) == nil {
					return &idx, nil
				}
			}
		}
	}

	resp, err := rc.client.Get(rc.IndexURL)
	if err != nil {
		// Fall back to stale cache on network error.
		return rc.loadCachedIndex(cachePath)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return rc.loadCachedIndex(cachePath)
	}

	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return rc.loadCachedIndex(cachePath)
	}

	// Write cache.
	os.WriteFile(cachePath, data, 0o644)

	var idx SkillIndex
	if err := json.Unmarshal(data, &idx); err != nil {
		return nil, fmt.Errorf("invalid index: %w", err)
	}
	return &idx, nil
}

func (rc *RegistryClient) loadCachedIndex(path string) (*SkillIndex, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("registry unavailable and no cache found")
	}
	var idx SkillIndex
	if err := json.Unmarshal(data, &idx); err != nil {
		return nil, fmt.Errorf("invalid cached index: %w", err)
	}
	return &idx, nil
}

// Search filters skills by query string and optional category.
func (rc *RegistryClient) Search(query, category string) ([]SkillEntry, error) {
	idx, err := rc.FetchIndex()
	if err != nil {
		return nil, err
	}
	q := strings.ToLower(query)
	var results []SkillEntry
	for _, s := range idx.Skills {
		if category != "" && !strings.EqualFold(s.Category, category) {
			continue
		}
		if q == "" || matchesQuery(s, q) {
			results = append(results, s)
		}
	}
	// Sort by relevance: exact name match first, then installs.
	sort.Slice(results, func(i, j int) bool {
		iExact := strings.EqualFold(results[i].Name, query)
		jExact := strings.EqualFold(results[j].Name, query)
		if iExact != jExact {
			return iExact
		}
		return results[i].Installs > results[j].Installs
	})
	return results, nil
}

func matchesQuery(s SkillEntry, q string) bool {
	if strings.Contains(strings.ToLower(s.Name), q) {
		return true
	}
	if strings.Contains(strings.ToLower(s.Description), q) {
		return true
	}
	for _, tag := range s.Tags {
		if strings.Contains(strings.ToLower(tag), q) {
			return true
		}
	}
	return false
}

// Trending returns the most-installed skills.
func (rc *RegistryClient) Trending(limit int) ([]SkillEntry, error) {
	idx, err := rc.FetchIndex()
	if err != nil {
		return nil, err
	}
	skills := make([]SkillEntry, len(idx.Skills))
	copy(skills, idx.Skills)
	sort.Slice(skills, func(i, j int) bool {
		return skills[i].Installs > skills[j].Installs
	})
	if limit > 0 && limit < len(skills) {
		skills = skills[:limit]
	}
	return skills, nil
}

// Info returns detailed information about a specific skill.
func (rc *RegistryClient) Info(name string) (*SkillEntry, error) {
	idx, err := rc.FetchIndex()
	if err != nil {
		return nil, err
	}
	for _, s := range idx.Skills {
		if strings.EqualFold(s.Name, name) {
			return &s, nil
		}
	}
	return nil, fmt.Errorf("skill %q not found in registry", name)
}

// Install clones a specific skill from a GitHub repo into the skills directory.
// If skillName is empty, all skills in the repo are installed.
func (rc *RegistryClient) Install(repo, skillName, scope string) (string, error) {
	home, _ := os.UserHomeDir()
	var destBase string
	switch scope {
	case "user":
		destBase = filepath.Join(home, ".hawk", "skills")
	default: // "project"
		destBase = filepath.Join(".hawk", "skills")
	}
	os.MkdirAll(destBase, 0o755)

	// Clone into a temp dir, then copy the skill(s).
	tmpDir, err := os.MkdirTemp("", "hawk-skill-*")
	if err != nil {
		return "", fmt.Errorf("create temp dir: %w", err)
	}
	defer os.RemoveAll(tmpDir)

	url := "https://github.com/" + repo + ".git"
	cmd := exec.Command("git", "clone", "--depth", "1", "--single-branch", url, tmpDir)
	if out, err := cmd.CombinedOutput(); err != nil {
		return "", fmt.Errorf("git clone failed: %s\n%s", err, string(out))
	}

	// Discover skills in the cloned repo.
	skillsRoot := tmpDir
	// Check for skills/ subdirectory (agentskills.io convention).
	if info, err := os.Stat(filepath.Join(tmpDir, "skills")); err == nil && info.IsDir() {
		skillsRoot = filepath.Join(tmpDir, "skills")
	}

	installed := []string{}
	entries, err := os.ReadDir(skillsRoot)
	if err != nil {
		return "", fmt.Errorf("read skills: %w", err)
	}

	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		name := e.Name()
		if skillName != "" && !strings.EqualFold(name, skillName) {
			continue
		}
		srcSkill := filepath.Join(skillsRoot, name, "SKILL.md")
		if _, err := os.Stat(srcSkill); err != nil {
			continue
		}

		destDir := filepath.Join(destBase, name)
		os.MkdirAll(destDir, 0o755)

		data, err := os.ReadFile(srcSkill)
		if err != nil {
			continue
		}

		// Inject source tracking metadata.
		content := injectSourceMetadata(string(data), repo)

		// Audit-on-install: scan for dangerous content before writing.
		findings := auditContent(srcSkill, string(data))
		hasCritical := false
		for _, f := range findings {
			if f.Severity == SeverityCritical {
				hasCritical = true
				break
			}
		}
		if hasCritical {
			// Strip dangerous chars and warn.
			content = StripDangerousChars(content)
			installed = append(installed, name+" (sanitized)")
		} else {
			installed = append(installed, name)
		}

		if err := os.WriteFile(filepath.Join(destDir, "SKILL.md"), []byte(content), 0o644); err != nil {
			continue
		}
	}

	if len(installed) == 0 {
		if skillName != "" {
			return "", fmt.Errorf("skill %q not found in %s", skillName, repo)
		}
		return "", fmt.Errorf("no skills found in %s", repo)
	}
	return fmt.Sprintf("Installed %d skill(s): %s", len(installed), strings.Join(installed, ", ")), nil
}

// Remove uninstalls a skill by name from both project and user scope.
func Remove(name string) error {
	home, _ := os.UserHomeDir()
	dirs := []string{
		filepath.Join(".hawk", "skills", name),
		filepath.Join(home, ".hawk", "skills", name),
	}
	removed := false
	for _, d := range dirs {
		if _, err := os.Stat(d); err == nil {
			os.RemoveAll(d)
			removed = true
		}
	}
	if !removed {
		return fmt.Errorf("skill %q not found", name)
	}
	return nil
}

// InstalledSkillInfo returns source metadata for an installed skill.
func InstalledSkillInfo(name string) (SmartSkill, string, bool) {
	home, _ := os.UserHomeDir()
	dirs := []string{
		filepath.Join(".hawk", "skills"),
		filepath.Join(home, ".hawk", "skills"),
	}
	for _, dir := range dirs {
		skillFile := filepath.Join(dir, name, "SKILL.md")
		data, err := os.ReadFile(skillFile)
		if err != nil {
			continue
		}
		skill := parseSmartSkill(string(data))
		if skill.Name == "" {
			skill.Name = name
		}
		return skill, skillFile, true
	}
	return SmartSkill{}, "", false
}

// FormatSkillEntry formats a registry entry for display.
func FormatSkillEntry(e SkillEntry) string {
	var b strings.Builder
	fmt.Fprintf(&b, "  %s", e.Name)
	if e.Version != "" {
		fmt.Fprintf(&b, " v%s", e.Version)
	}
	if e.Author != "" {
		fmt.Fprintf(&b, " by %s", e.Author)
	}
	if e.Installs > 0 {
		fmt.Fprintf(&b, " (%d installs)", e.Installs)
	}
	b.WriteString("\n")
	if e.Description != "" {
		fmt.Fprintf(&b, "    %s\n", e.Description)
	}
	if e.Repo != "" {
		fmt.Fprintf(&b, "    repo: %s\n", e.Repo)
	}
	return b.String()
}

// FormatSkillInfo formats detailed skill info for display.
func FormatSkillInfo(s SmartSkill, path string) string {
	var b strings.Builder
	fmt.Fprintf(&b, "Skill: %s\n", s.Name)
	if s.Version != "" {
		fmt.Fprintf(&b, "Version: %s\n", s.Version)
	}
	if s.Author != "" {
		fmt.Fprintf(&b, "Author: %s\n", s.Author)
	}
	if s.License != "" {
		fmt.Fprintf(&b, "License: %s\n", s.License)
	}
	if s.Category != "" {
		fmt.Fprintf(&b, "Category: %s\n", s.Category)
	}
	if s.Description != "" {
		fmt.Fprintf(&b, "Description: %s\n", s.Description)
	}
	if len(s.Tags) > 0 {
		fmt.Fprintf(&b, "Tags: %s\n", strings.Join(s.Tags, ", "))
	}
	if len(s.Agents) > 0 {
		fmt.Fprintf(&b, "Agents: %s\n", strings.Join(s.Agents, ", "))
	}
	if s.AllowedTools != "" {
		fmt.Fprintf(&b, "Tools: %s\n", s.AllowedTools)
	}
	if s.Source.Repo != "" {
		fmt.Fprintf(&b, "Source: %s", s.Source.Repo)
		if s.Source.Ref != "" {
			fmt.Fprintf(&b, " @ %s", s.Source.Ref)
		}
		b.WriteString("\n")
	}
	if path != "" {
		fmt.Fprintf(&b, "Path: %s\n", path)
	}
	return b.String()
}
