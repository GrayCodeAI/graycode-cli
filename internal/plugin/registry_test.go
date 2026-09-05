package plugin

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"github.com/GrayCodeAI/graycode-cli/internal/testutil"
)

func testIndex() SkillIndex {
	return SkillIndex{
		Version:   1,
		UpdatedAt: "2026-05-01T00:00:00Z",
		Skills: []SkillEntry{
			{Name: "api-review", Description: "Reviews API endpoints", Author: "graycode", Repo: "GrayCodeAI/starling", Category: "engineering", Tags: []string{"api", "review"}, Version: "1.0.0", Installs: 342},
			{Name: "security-scan", Description: "Scans for security vulnerabilities", Author: "graycode", Repo: "GrayCodeAI/starling", Category: "security", Tags: []string{"security", "scan"}, Version: "2.1.0", Installs: 891},
			{Name: "changelog", Description: "Generates changelogs from git commits", Author: "community", Repo: "community/skills", Category: "workflow", Tags: []string{"changelog", "git"}, Version: "1.2.0", Installs: 156},
		},
	}
}

func serveIndex(t *testing.T) (*httptest.Server, *RegistryClient) {
	t.Helper()
	idx := testIndex()
	data, _ := json.Marshal(idx)
	srv := testutil.NewLoopbackHTTPServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write(data)
	}))
	rc := &RegistryClient{
		IndexURL: srv.URL,
		CacheDir: t.TempDir(),
		client:   srv.Client(),
	}
	return srv, rc
}

func TestFetchIndex(t *testing.T) {
	srv, rc := serveIndex(t)
	defer srv.Close()

	idx, err := rc.FetchIndex()
	if err != nil {
		t.Fatalf("FetchIndex: %v", err)
	}
	if len(idx.Skills) != 3 {
		t.Fatalf("expected 3 skills, got %d", len(idx.Skills))
	}
}

func TestFetchIndexCache(t *testing.T) {
	srv, rc := serveIndex(t)

	// First fetch populates cache.
	_, err := rc.FetchIndex()
	if err != nil {
		t.Fatalf("first fetch: %v", err)
	}

	// Close server — second fetch should use cache.
	srv.Close()
	idx, err := rc.FetchIndex()
	if err != nil {
		t.Fatalf("cached fetch: %v", err)
	}
	if len(idx.Skills) != 3 {
		t.Fatalf("expected 3 skills from cache, got %d", len(idx.Skills))
	}
}

func TestSearch(t *testing.T) {
	srv, rc := serveIndex(t)
	defer srv.Close()

	// Search by name.
	results, err := rc.Search("api", "")
	if err != nil {
		t.Fatalf("Search: %v", err)
	}
	if len(results) != 1 {
		t.Fatalf("expected 1 result for 'api', got %d", len(results))
	}
	if results[0].Name != "api-review" {
		t.Errorf("expected api-review, got %s", results[0].Name)
	}

	// Search by tag.
	results, err = rc.Search("security", "")
	if err != nil {
		t.Fatalf("Search: %v", err)
	}
	if len(results) != 1 {
		t.Fatalf("expected 1 result for 'security', got %d", len(results))
	}

	// Search with category filter.
	results, err = rc.Search("", "engineering")
	if err != nil {
		t.Fatalf("Search: %v", err)
	}
	if len(results) != 1 {
		t.Fatalf("expected 1 result for category 'engineering', got %d", len(results))
	}

	// Empty query returns all.
	results, err = rc.Search("", "")
	if err != nil {
		t.Fatalf("Search: %v", err)
	}
	if len(results) != 3 {
		t.Fatalf("expected 3 results for empty query, got %d", len(results))
	}
}

func TestTrending(t *testing.T) {
	srv, rc := serveIndex(t)
	defer srv.Close()

	results, err := rc.Trending(2)
	if err != nil {
		t.Fatalf("Trending: %v", err)
	}
	if len(results) != 2 {
		t.Fatalf("expected 2 results, got %d", len(results))
	}
	if results[0].Name != "security-scan" {
		t.Errorf("expected security-scan first (891 installs), got %s", results[0].Name)
	}
	if results[1].Name != "api-review" {
		t.Errorf("expected api-review second (342 installs), got %s", results[1].Name)
	}
}

func TestInfo(t *testing.T) {
	srv, rc := serveIndex(t)
	defer srv.Close()

	entry, err := rc.Info("changelog")
	if err != nil {
		t.Fatalf("Info: %v", err)
	}
	if entry.Author != "community" {
		t.Errorf("expected author 'community', got %q", entry.Author)
	}

	_, err = rc.Info("nonexistent")
	if err == nil {
		t.Error("expected error for nonexistent skill")
	}
}

func TestParseSmartSkillExtendedFields(t *testing.T) {
	content := `---
name: api-review
description: Reviews API endpoints
version: "1.2.0"
author: graycode
license: MIT
category: engineering
tags: ["api", "review", "rest"]
agents: ["graycode", "claude-code"]
source-repo: GrayCodeAI/starling
source-ref: v1.2.0
source-installed-at: 2026-05-01T00:00:00Z
---
Review all API endpoints.
`
	skill := parseSmartSkill(content)
	if skill.Version != `"1.2.0"` {
		t.Errorf("version: got %q", skill.Version)
	}
	if skill.Author != "graycode" {
		t.Errorf("author: got %q", skill.Author)
	}
	if skill.License != "MIT" {
		t.Errorf("license: got %q", skill.License)
	}
	if skill.Category != "engineering" {
		t.Errorf("category: got %q", skill.Category)
	}
	if len(skill.Tags) != 3 {
		t.Errorf("expected 3 tags, got %d", len(skill.Tags))
	}
	if len(skill.Agents) != 2 {
		t.Errorf("expected 2 agents, got %d", len(skill.Agents))
	}
	if skill.Source.Repo != "GrayCodeAI/starling" {
		t.Errorf("source repo: got %q", skill.Source.Repo)
	}
	if skill.Source.Ref != "v1.2.0" {
		t.Errorf("source ref: got %q", skill.Source.Ref)
	}
	if !strings.Contains(skill.Content, "Review all API endpoints") {
		t.Error("expected body content")
	}
}

func TestInjectSourceMetadata(t *testing.T) {
	// With existing frontmatter.
	content := "---\nname: test\ndescription: A test skill\n---\nBody content."
	result := injectSourceMetadata(content, "owner/repo")
	if !strings.Contains(result, "source-repo: owner/repo") {
		t.Error("expected source-repo in output")
	}
	if !strings.Contains(result, "source-installed-at:") {
		t.Error("expected source-installed-at in output")
	}
	if !strings.Contains(result, "name: test") {
		t.Error("expected original frontmatter preserved")
	}
	if !strings.Contains(result, "Body content.") {
		t.Error("expected body preserved")
	}

	// Without frontmatter.
	content = "Just some instructions."
	result = injectSourceMetadata(content, "owner/repo")
	if !strings.Contains(result, "---\nsource-repo: owner/repo") {
		t.Error("expected frontmatter wrapper")
	}
	if !strings.Contains(result, "Just some instructions.") {
		t.Error("expected body preserved")
	}
}

func TestRemove(t *testing.T) {
	dir := t.TempDir()
	skillDir := filepath.Join(dir, ".graycode", "skills", "test-skill")
	os.MkdirAll(skillDir, 0o755)
	os.WriteFile(filepath.Join(skillDir, "SKILL.md"), []byte("test"), 0o644)

	// Remove should fail for nonexistent skill (since we can't override home dir easily).
	err := Remove("definitely-not-installed-xyz")
	if err == nil {
		t.Error("expected error for nonexistent skill")
	}
}

func TestFormatSkillEntry(t *testing.T) {
	e := SkillEntry{
		Name:        "api-review",
		Version:     "1.0.0",
		Author:      "graycode",
		Description: "Reviews API endpoints",
		Repo:        "GrayCodeAI/starling",
		Installs:    342,
	}
	out := FormatSkillEntry(e)
	if !strings.Contains(out, "api-review") {
		t.Error("expected name")
	}
	if !strings.Contains(out, "v1.0.0") {
		t.Error("expected version")
	}
	if !strings.Contains(out, "graycode") {
		t.Error("expected author")
	}
	if !strings.Contains(out, "342 installs") {
		t.Error("expected install count")
	}
}

func TestFormatSkillInfo(t *testing.T) {
	s := SmartSkill{
		Name:     "api-review",
		Version:  "1.0.0",
		Author:   "graycode",
		License:  "MIT",
		Category: "engineering",
		Tags:     []string{"api", "review"},
		Source:   SkillSource{Repo: "GrayCodeAI/starling", Ref: "v1.0.0"},
	}
	out := FormatSkillInfo(s, "/path/to/skill")
	if !strings.Contains(out, "Skill: api-review") {
		t.Error("expected skill name")
	}
	if !strings.Contains(out, "MIT") {
		t.Error("expected license")
	}
	if !strings.Contains(out, "GrayCodeAI/starling") {
		t.Error("expected source repo")
	}
}

func TestDefaultIndexURLIsPublished(t *testing.T) {
	const want = "https://github.com/GrayCodeAI/graycode-skills/releases/download/registry-latest/registry.json"
	if defaultIndexURL != want {
		t.Fatalf("defaultIndexURL = %q, want %q", defaultIndexURL, want)
	}
	if strings.Contains(defaultIndexURL, "starling") {
		t.Errorf("defaultIndexURL still references the renamed starling repo")
	}
}

func TestFetchIndexParsesGeneratedShape(t *testing.T) {
	// Byte-for-byte the shape graycode-skills/tools/update_registry.py emits.
	const generated = `{
  "version": 1,
  "skills": [
    {
      "name": "ab-test-setup",
      "description": "Plan and design an A/B test",
      "category": "testing",
      "tags": ["testing"],
      "path": "categories/testing/ab-test-setup",
      "repo": "GrayCodeAI/graycode-skills",
      "file_count": 1,
      "has_scripts": false
    }
  ]
}
`
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(generated))
	}))
	defer srv.Close()

	rc := &RegistryClient{IndexURL: srv.URL, CacheDir: t.TempDir(), client: srv.Client()}
	idx, err := rc.FetchIndex()
	if err != nil {
		t.Fatalf("FetchIndex: %v", err)
	}
	if len(idx.Skills) != 1 {
		t.Fatalf("skills = %d, want 1", len(idx.Skills))
	}
	if idx.Skills[0].Repo != "GrayCodeAI/graycode-skills" {
		t.Errorf("Repo = %q, want the slug the installer clones from", idx.Skills[0].Repo)
	}
}

func TestDiscoverSkillDirs(t *testing.T) {
	tests := []struct {
		name   string
		layout []string
		want   []string
	}{
		{name: "flat layout", layout: []string{"go-review/SKILL.md"}, want: []string{"go-review"}},
		{name: "agentskills.io skills/ layout", layout: []string{"skills/go-review/SKILL.md"}, want: []string{"go-review"}},
		{
			name:   "graycode-skills categories layout",
			layout: []string{"categories/go/go-review/SKILL.md", "categories/python/pandas/SKILL.md"},
			want:   []string{"go-review", "pandas"},
		},
		{
			name:   "ignores vendored and dot directories",
			layout: []string{"go-review/SKILL.md", ".git/hooks/SKILL.md", "node_modules/pkg/SKILL.md"},
			want:   []string{"go-review"},
		},
		{
			name:   "ignores a top-level SKILL.md documenting the repo",
			layout: []string{"SKILL.md", "categories/go/go-review/SKILL.md"},
			want:   []string{"go-review"},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			root := t.TempDir()
			for _, rel := range tc.layout {
				full := filepath.Join(root, rel)
				if err := os.MkdirAll(filepath.Dir(full), 0o750); err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(full, []byte("---\nname: x\n---\n"), 0o600); err != nil {
					t.Fatal(err)
				}
			}

			got, err := discoverSkillDirs(root)
			if err != nil {
				t.Fatalf("discoverSkillDirs: %v", err)
			}
			if len(got) != len(tc.want) {
				t.Fatalf("found %d skills %v, want %d %v", len(got), keysOf(got), len(tc.want), tc.want)
			}
			for _, name := range tc.want {
				dir, ok := got[name]
				if !ok {
					t.Errorf("missing skill %q; got %v", name, keysOf(got))
					continue
				}
				if _, err := os.Stat(filepath.Join(dir, "SKILL.md")); err != nil {
					t.Errorf("skill %q maps to %q which has no SKILL.md", name, dir)
				}
			}
		})
	}
}

func keysOf(m map[string]string) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}
