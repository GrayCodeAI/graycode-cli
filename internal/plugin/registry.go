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
	"sort"
	"strings"
	"time"

	"github.com/GrayCodeAI/graycode-cli/internal/installtxn"
	"github.com/GrayCodeAI/graycode-cli/internal/storage"
)

const defaultIndexURL = "https://raw.githubusercontent.com/GrayCodeAI/starling/main/registry.json"

// SkillInvocationPolicy controls which callers may invoke a skill.
type SkillInvocationPolicy struct {
	ModelInvocable *bool `json:"model_invocable,omitempty"`
	UserInvocable  *bool `json:"user_invocable,omitempty"`
}

// IsModelInvocable returns true unless ModelInvocable is explicitly false.
func (p SkillInvocationPolicy) IsModelInvocable() bool {
	if p.ModelInvocable == nil {
		return true
	}
	return *p.ModelInvocable
}

// IsUserInvocable returns true unless UserInvocable is explicitly false.
func (p SkillInvocationPolicy) IsUserInvocable() bool {
	if p.UserInvocable == nil {
		return true
	}
	return *p.UserInvocable
}

// NewInvocationPolicy returns an explicit invocation policy.
func NewInvocationPolicy(modelInvocable, userInvocable bool) SkillInvocationPolicy {
	return SkillInvocationPolicy{
		ModelInvocable: &modelInvocable,
		UserInvocable:  &userInvocable,
	}
}

// SkillEntry is a single skill in the registry index.
type SkillEntry struct {
	Name         string                `json:"name"`
	Description  string                `json:"description"`
	Author       string                `json:"author,omitempty"`
	Repo         string                `json:"repo,omitempty"`
	Path         string                `json:"path,omitempty"`
	Category     string                `json:"category,omitempty"`
	Tags         []string              `json:"tags,omitempty"`
	Version      string                `json:"version,omitempty"`
	License      string                `json:"license,omitempty"`
	Agents       []string              `json:"agents,omitempty"`
	Installs     int                   `json:"installs,omitempty"`
	UpdatedAt    string                `json:"updated_at,omitempty"`
	Invocation   SkillInvocationPolicy `json:"invocation"`
	Provider     string                `json:"provider,omitempty"`
	Content      string                `json:"content,omitempty"`
	ResourceBase string                `json:"resource_base,omitempty"`
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
	return &RegistryClient{
		IndexURL: defaultIndexURL,
		CacheDir: filepath.Join(storage.CacheDir(), "skills"),
		client:   &http.Client{Timeout: 15 * time.Second},
	}
}

// FetchIndex downloads the registry index, using a local cache when fresh.
func (rc *RegistryClient) FetchIndex() (*SkillIndex, error) {
	_ = os.MkdirAll(rc.CacheDir, 0o750)
	cachePath := filepath.Join(rc.CacheDir, "skills-index.json")

	// Use cache if less than 1 hour old.
	if info, err := os.Stat(cachePath); err == nil {
		if time.Since(info.ModTime()) < time.Hour {
			data, err := os.ReadFile(cachePath) // #nosec G304 -- cachePath is derived from rc.CacheDir, a fixed internal cache location, not user input
			if err == nil {
				var idx SkillIndex
				if json.Unmarshal(data, &idx) == nil {
					return &idx, nil
				}
			}
		}
	}

	resp, err := rc.client.Do(func() *http.Request {
		r, _ := http.NewRequestWithContext(context.Background(), http.MethodGet, rc.IndexURL, nil)
		return r
	}())
	if err != nil {
		// Fall back to stale cache on network error.
		return rc.loadCachedIndex(cachePath)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		return rc.loadCachedIndex(cachePath)
	}

	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return rc.loadCachedIndex(cachePath)
	}

	// Write cache.
	_ = os.WriteFile(cachePath, data, 0o600)

	var idx SkillIndex
	if err := json.Unmarshal(data, &idx); err != nil {
		return nil, fmt.Errorf("invalid index: %w", err)
	}
	return &idx, nil
}

func (rc *RegistryClient) loadCachedIndex(path string) (*SkillIndex, error) {
	data, err := os.ReadFile(path) // #nosec G304 -- path is always cachePath, derived from the fixed internal cache directory, not user input
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
	var destBase string
	switch scope {
	case "user":
		destBase = filepath.Join(storage.StateDir(), "skills")
	default: // "project"
		destBase = filepath.Join(storage.ProjectStateDir("."), "skills")
	}
	_ = os.MkdirAll(destBase, 0o750)

	// Clone into a temp dir, then copy the skill(s).
	tmpDir, err := os.MkdirTemp("", "graycode-skill-*")
	if err != nil {
		return "", fmt.Errorf("create temp dir: %w", err)
	}
	defer func() { _ = os.RemoveAll(tmpDir) }()

	url := "https://github.com/" + repo + ".git"
	cmd := exec.CommandContext(context.Background(), "git", "clone", "--depth", "1", "--single-branch", url, tmpDir) // #nosec G204 -- url is built from a caller-supplied repo slug prefixed with a fixed GitHub URL, consistent with other install paths in this package
	if out, cloneErr := cmd.CombinedOutput(); cloneErr != nil {
		return "", fmt.Errorf("git clone failed: %w\n%s", cloneErr, string(out))
	}

	// Record the cloned HEAD so the lockfile can pin what was installed.
	headOut, headErr := exec.CommandContext(context.Background(), "git", "-C", tmpDir, "rev-parse", "HEAD").Output() // #nosec G204 -- tmpDir is a freshly created temp directory owned by this function, not external input
	commitSha := ""
	if headErr == nil {
		commitSha = strings.TrimSpace(string(headOut))
	}

	// Discover skills in the cloned repo.
	skillsRoot := tmpDir
	// Check for skills/ subdirectory (agentskills.io convention).
	if info, statErr := os.Stat(filepath.Join(tmpDir, "skills")); statErr == nil && info.IsDir() {
		skillsRoot = filepath.Join(tmpDir, "skills")
	}

	installed := []string{}
	blocked := []string{}
	sanitized := 0
	var warnings strings.Builder
	lock, lockErr := LoadSkillsLock(scope)
	if lockErr != nil {
		return "", fmt.Errorf("load skills lock: %w", lockErr)
	}
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
		_ = os.MkdirAll(destDir, 0o750)

		data, err := os.ReadFile(srcSkill) // #nosec G304 -- srcSkill is derived from skillsRoot, a path inside the freshly cloned tmpDir, and the enumerated entry name, not raw external input
		if err != nil {
			continue
		}

		content := injectSourceMetadata(string(data), repo)

		// Unicode audit: sanitize dangerous characters before any further use.
		findings := auditContent(srcSkill, string(data))
		hasCritical := false
		for _, f := range findings {
			if f.Severity == SeverityCritical {
				hasCritical = true
				break
			}
		}
		if hasCritical {
			content = StripDangerousChars(content)
		}

		// Threat scan: refuse skills whose content looks malicious.
		scan := ScanThreats(content)
		if scan.Blocked {
			blocked = append(blocked, name)
			continue
		}

		if err := installtxn.WriteFileAtomically(filepath.Join(destDir, "SKILL.md"), []byte(content), 0o600); err != nil {
			continue
		}

		installed = append(installed, name)
		lock.Set(name, SkillsLockEntry{
			Source:       repo,
			SourceType:   "github",
			SkillPath:    strings.TrimPrefix(strings.TrimPrefix(srcSkill, tmpDir), "/"),
			Commit:       commitSha,
			ComputedHash: HashSkillContent([]byte(content)),
		})
		if hasCritical {
			sanitized++
		}
		warnings.WriteString(FormatThreatScan(name, scan))
	}

	if len(installed) > 0 {
		if err := lock.Save(scope); err != nil {
			// Lockfile is advisory; a failed save must not roll back the install.
			warnings.WriteString(fmt.Sprintf("warning: skills-lock.json update failed: %v\n", err))
		}
	}

	if len(installed) == 0 {
		if len(blocked) > 0 {
			return "", fmt.Errorf("refused to install skill(s) from %s: security score below threshold: %s",
				repo, strings.Join(blocked, ", "))
		}
		if skillName != "" {
			return "", fmt.Errorf("skill %q not found in %s", skillName, repo)
		}
		return "", fmt.Errorf("no skills found in %s", repo)
	}

	msg := fmt.Sprintf("Installed %d skill(s): %s", len(installed), strings.Join(installed, ", "))
	if sanitized > 0 {
		msg += fmt.Sprintf(" (%d sanitized)", sanitized)
	}
	if warnings.Len() > 0 {
		msg += "\nSecurity warnings:\n" + strings.TrimRight(warnings.String(), "\n")
	}
	if len(blocked) > 0 {
		msg += "\nRefused (security): " + strings.Join(blocked, ", ")
	}
	return msg, nil
}

// Remove uninstalls a skill by name from Graycode user state.
func Remove(name string) error {
	dirs := []string{
		filepath.Join(storage.StateDir(), "skills", name),
	}
	removed := false
	for _, d := range dirs {
		if _, err := os.Stat(d); err == nil {
			_ = os.RemoveAll(d)
			removed = true
		}
	}

	// Drop the skill from every scope's lockfile where it is pinned.
	for _, scope := range []string{"user", "project"} {
		if lock, err := LoadSkillsLock(scope); err == nil && lock.Delete(name) {
			_ = lock.Save(scope)
		}
	}
	if !removed {
		return fmt.Errorf("skill %q not found", name)
	}
	return nil
}

// InstalledSkillInfo returns source metadata for an installed skill.
func InstalledSkillInfo(name string) (SmartSkill, string, bool) {
	dirs := []string{
		filepath.Join(storage.StateDir(), "skills"),
	}
	for _, dir := range dirs {
		skillFile := filepath.Join(dir, name, "SKILL.md")
		data, err := os.ReadFile(skillFile) // #nosec G304 -- skillFile is built from the fixed skills state dir and a locally known skill name, not raw external input
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
	_, _ = fmt.Fprintf(&b, "  %s", e.Name)
	if e.Version != "" {
		_, _ = fmt.Fprintf(&b, " v%s", e.Version)
	}
	if e.Author != "" {
		_, _ = fmt.Fprintf(&b, " by %s", e.Author)
	}
	if e.Installs > 0 {
		_, _ = fmt.Fprintf(&b, " (%d installs)", e.Installs)
	}
	b.WriteString("\n")
	if e.Description != "" {
		_, _ = fmt.Fprintf(&b, "    %s\n", e.Description)
	}
	if e.Repo != "" {
		_, _ = fmt.Fprintf(&b, "    repo: %s\n", e.Repo)
	}
	return b.String()
}

// FormatSkillInfo formats detailed skill info for display.
func FormatSkillInfo(s SmartSkill, path string) string {
	var b strings.Builder
	_, _ = fmt.Fprintf(&b, "Skill: %s\n", s.Name)
	if s.Version != "" {
		_, _ = fmt.Fprintf(&b, "Version: %s\n", s.Version)
	}
	if s.Author != "" {
		_, _ = fmt.Fprintf(&b, "Author: %s\n", s.Author)
	}
	if s.License != "" {
		_, _ = fmt.Fprintf(&b, "License: %s\n", s.License)
	}
	if s.Category != "" {
		_, _ = fmt.Fprintf(&b, "Category: %s\n", s.Category)
	}
	if s.Description != "" {
		_, _ = fmt.Fprintf(&b, "Description: %s\n", s.Description)
	}
	if len(s.Tags) > 0 {
		_, _ = fmt.Fprintf(&b, "Tags: %s\n", strings.Join(s.Tags, ", "))
	}
	if len(s.Agents) > 0 {
		_, _ = fmt.Fprintf(&b, "Agents: %s\n", strings.Join(s.Agents, ", "))
	}
	if s.AllowedTools != "" {
		_, _ = fmt.Fprintf(&b, "Tools: %s\n", s.AllowedTools)
	}
	if s.Source.Repo != "" {
		_, _ = fmt.Fprintf(&b, "Source: %s", s.Source.Repo)
		if s.Source.Ref != "" {
			_, _ = fmt.Fprintf(&b, " @ %s", s.Source.Ref)
		}
		b.WriteString("\n")
	}
	if path != "" {
		_, _ = fmt.Fprintf(&b, "Path: %s\n", path)
	}
	return b.String()
}
