package engine

import (
	"os"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"
)

// RepoRelation defines how a related repo connects to the current one.
type RepoRelation struct {
	Path     string `yaml:"path"`
	Relation string `yaml:"relation"` // "dependency", "types", "service", "shared"
}

// MultiRepoConfig is loaded from .hawk/repos.yaml.
type MultiRepoConfig struct {
	Repos []RepoRelation `yaml:"repos"`
}

// MultiRepoContext loads and manages cross-repo context.
type MultiRepoContext struct {
	Config  MultiRepoConfig
	BaseDir string
}

// LoadMultiRepoConfig reads .hawk/repos.yaml from the project root.
func LoadMultiRepoConfig(projectDir string) *MultiRepoContext {
	mrc := &MultiRepoContext{BaseDir: projectDir}
	path := filepath.Join(projectDir, ".hawk", "repos.yaml")
	data, err := os.ReadFile(path)
	if err != nil {
		return mrc
	}
	_ = yaml.Unmarshal(data, &mrc.Config)
	return mrc
}

// HasRelatedRepos reports whether any related repos are configured.
func (mrc *MultiRepoContext) HasRelatedRepos() bool {
	return len(mrc.Config.Repos) > 0
}

// LoadBoundaryContext loads interface/type definitions from related repos.
// Only loads public API surfaces (exported types, interfaces, function signatures).
func (mrc *MultiRepoContext) LoadBoundaryContext() string {
	if !mrc.HasRelatedRepos() {
		return ""
	}
	var sections []string
	for _, repo := range mrc.Config.Repos {
		repoPath := repo.Path
		if !filepath.IsAbs(repoPath) {
			repoPath = filepath.Join(mrc.BaseDir, repoPath)
		}
		if _, err := os.Stat(repoPath); err != nil {
			continue
		}
		context := extractBoundary(repoPath, repo.Relation)
		if context != "" {
			sections = append(sections, "## ["+repo.Relation+": "+filepath.Base(repoPath)+"]\n"+context)
		}
	}
	if len(sections) == 0 {
		return ""
	}
	return "# Cross-Repo Context\n\n" + strings.Join(sections, "\n\n")
}

// extractBoundary pulls relevant interface/type info from a related repo.
func extractBoundary(repoPath, relation string) string {
	var files []string

	switch relation {
	case "types", "shared":
		// Look for type definition files
		files = findFiles(repoPath, []string{"types.go", "models.go", "schema.go", "types.ts", "models.ts", "types.py"})
	case "dependency", "service":
		// Look for API/interface files
		files = findFiles(repoPath, []string{"api.go", "client.go", "interface.go", "openapi.yaml", "proto"})
	default:
		files = findFiles(repoPath, []string{"types.go", "api.go", "README.md"})
	}

	var content []string
	for _, f := range files {
		data, err := os.ReadFile(f)
		if err != nil {
			continue
		}
		text := string(data)
		// Truncate large files to just signatures
		if len(text) > 3000 {
			text = text[:3000] + "\n... (truncated)"
		}
		rel, _ := filepath.Rel(repoPath, f)
		content = append(content, "### "+rel+"\n```\n"+text+"\n```")
	}
	return strings.Join(content, "\n\n")
}

func findFiles(dir string, names []string) []string {
	var found []string
	_ = filepath.Walk(dir, func(path string, info os.FileInfo, err error) error {
		if err != nil || info.IsDir() {
			if info != nil && info.IsDir() {
				name := info.Name()
				if name == ".git" || name == "node_modules" || name == "vendor" {
					return filepath.SkipDir
				}
			}
			return nil
		}
		base := filepath.Base(path)
		for _, n := range names {
			if base == n || strings.Contains(base, n) {
				found = append(found, path)
				if len(found) >= 5 { // limit per repo
					return filepath.SkipAll
				}
			}
		}
		return nil
	})
	return found
}
