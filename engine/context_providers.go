package engine

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"sync"
	"time"
)

// ContextProvider is a pluggable source of context that can be injected into the agent's system prompt.
type ContextProvider interface {
	Name() string
	Description() string
	Gather(ctx context.Context, query string) ([]ContextItem, error)
	TokenBudget() int
}

// ContextItem represents a single piece of context gathered from a provider.
type ContextItem struct {
	Source     string
	Title      string
	Content    string
	Relevance  float64
	TokenCount int
}

// ContextManager manages multiple context providers and orchestrates context gathering.
type ContextManager struct {
	Providers   []ContextProvider
	TotalBudget int
	mu          sync.RWMutex
}

// NewContextManager creates a new ContextManager with the given total token budget.
func NewContextManager(totalBudget int) *ContextManager {
	return &ContextManager{
		Providers:   make([]ContextProvider, 0),
		TotalBudget: totalBudget,
	}
}

// Register adds a context provider to the manager.
func (cm *ContextManager) Register(provider ContextProvider) {
	cm.mu.Lock()
	defer cm.mu.Unlock()
	cm.Providers = append(cm.Providers, provider)
}

// GatherAll calls all providers in parallel, collects items, sorts by relevance,
// and truncates to fit within TotalBudget.
func (cm *ContextManager) GatherAll(ctx context.Context, query string) ([]ContextItem, error) {
	cm.mu.RLock()
	providers := make([]ContextProvider, len(cm.Providers))
	copy(providers, cm.Providers)
	cm.mu.RUnlock()

	if len(providers) == 0 {
		return nil, nil
	}

	type result struct {
		items []ContextItem
		err   error
	}

	results := make([]result, len(providers))
	var wg sync.WaitGroup

	for i, p := range providers {
		wg.Add(1)
		go func(idx int, provider ContextProvider) {
			defer wg.Done()
			items, err := provider.Gather(ctx, query)
			results[idx] = result{items: items, err: err}
		}(i, p)
	}

	wg.Wait()

	var allItems []ContextItem
	for _, r := range results {
		if r.err != nil {
			// Skip providers that error; others still contribute
			continue
		}
		allItems = append(allItems, r.items...)
	}

	sort.Slice(allItems, func(i, j int) bool {
		return allItems[i].Relevance > allItems[j].Relevance
	})

	allItems = PrioritizeItems(allItems, cm.TotalBudget)

	return allItems, nil
}

// FormatContextItems formats context items into a system prompt section.
func FormatContextItems(items []ContextItem) string {
	if len(items) == 0 {
		return ""
	}

	var sb strings.Builder
	sb.WriteString("## Relevant Context\n")

	for _, item := range items {
		sb.WriteString(fmt.Sprintf("\n### [Source: %s] %s\n", item.Source, item.Title))
		sb.WriteString(item.Content)
		sb.WriteString("\n")
	}

	return sb.String()
}

// PrioritizeItems performs greedy selection sorted by relevance, picking items until
// budget is exhausted while ensuring source diversity.
func PrioritizeItems(items []ContextItem, budget int) []ContextItem {
	if len(items) == 0 || budget <= 0 {
		return nil
	}

	// Sort by relevance descending
	sorted := make([]ContextItem, len(items))
	copy(sorted, items)
	sort.Slice(sorted, func(i, j int) bool {
		return sorted[i].Relevance > sorted[j].Relevance
	})

	// Count unique sources
	sourceSet := make(map[string]struct{})
	for _, item := range sorted {
		sourceSet[item.Source] = struct{}{}
	}
	numSources := len(sourceSet)

	// Max items from a single source: allow at most ceil(total_selected / numSources) + 1
	// to ensure diversity. We enforce this by limiting per-source count.
	maxPerSource := budget // fallback: no limit if only one source
	if numSources > 1 {
		// Allow each source at most half the items
		maxPerSource = (len(sorted) / numSources) + 2
	}

	var selected []ContextItem
	usedBudget := 0
	sourceCounts := make(map[string]int)

	for _, item := range sorted {
		if usedBudget >= budget {
			break
		}
		tokenCost := item.TokenCount
		if tokenCost == 0 {
			// Estimate token count from content length (rough: 1 token ~= 4 chars)
			tokenCost = len(item.Content) / 4
			if tokenCost == 0 {
				tokenCost = 1
			}
		}
		if usedBudget+tokenCost > budget {
			continue
		}
		if numSources > 1 && sourceCounts[item.Source] >= maxPerSource {
			continue
		}
		selected = append(selected, item)
		usedBudget += tokenCost
		sourceCounts[item.Source]++
	}

	return selected
}

// ---------- Built-in Providers ----------

// GitContextProvider provides recent git history as context.
type GitContextProvider struct {
	RepoDir    string
	MaxCommits int
}

func (g *GitContextProvider) Name() string        { return "git" }
func (g *GitContextProvider) Description() string { return "Recent git history and status" }
func (g *GitContextProvider) TokenBudget() int    { return 500 }

func (g *GitContextProvider) Gather(ctx context.Context, query string) ([]ContextItem, error) {
	var items []ContextItem

	maxCommits := g.MaxCommits
	if maxCommits <= 0 {
		maxCommits = 10
	}

	// Current branch
	branchCmd := exec.CommandContext(ctx, "git", "-C", g.RepoDir, "rev-parse", "--abbrev-ref", "HEAD")
	branchOut, err := branchCmd.Output()
	if err == nil {
		branch := strings.TrimSpace(string(branchOut))
		items = append(items, ContextItem{
			Source:     "git",
			Title:      "Current Branch",
			Content:    branch,
			Relevance:  0.8,
			TokenCount: 10,
		})
	}

	// Recent commits
	logCmd := exec.CommandContext(
		ctx, "git", "-C", g.RepoDir, "log",
		fmt.Sprintf("--max-count=%d", maxCommits),
		"--oneline",
	)
	logOut, err := logCmd.Output()
	if err == nil && len(strings.TrimSpace(string(logOut))) > 0 {
		items = append(items, ContextItem{
			Source:     "git",
			Title:      "Recent Commits",
			Content:    strings.TrimSpace(string(logOut)),
			Relevance:  0.7,
			TokenCount: len(string(logOut)) / 4,
		})
	}

	// Uncommitted changes
	statusCmd := exec.CommandContext(ctx, "git", "-C", g.RepoDir, "status", "--short")
	statusOut, err := statusCmd.Output()
	if err == nil && len(strings.TrimSpace(string(statusOut))) > 0 {
		items = append(items, ContextItem{
			Source:     "git",
			Title:      "Uncommitted Changes",
			Content:    strings.TrimSpace(string(statusOut)),
			Relevance:  0.9,
			TokenCount: len(string(statusOut)) / 4,
		})
	}

	return items, nil
}

// FileContextProvider provides recently modified files as context.
type FileContextProvider struct {
	RepoDir  string
	MaxFiles int
}

func (f *FileContextProvider) Name() string        { return "files" }
func (f *FileContextProvider) Description() string { return "Recently modified files" }
func (f *FileContextProvider) TokenBudget() int    { return 300 }

func (f *FileContextProvider) Gather(ctx context.Context, query string) ([]ContextItem, error) {
	maxFiles := f.MaxFiles
	if maxFiles <= 0 {
		maxFiles = 10
	}

	type fileInfo struct {
		path    string
		modTime time.Time
	}

	var files []fileInfo

	err := filepath.Walk(f.RepoDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return nil // skip errors
		}
		// Skip hidden directories and common non-source dirs
		name := info.Name()
		if info.IsDir() {
			if strings.HasPrefix(name, ".") || name == "node_modules" || name == "vendor" {
				return filepath.SkipDir
			}
			return nil
		}
		// Only include common source files
		ext := filepath.Ext(name)
		switch ext {
		case ".go", ".js", ".ts", ".py", ".rs", ".java", ".c", ".cpp", ".h", ".rb", ".jsx", ".tsx":
			relPath, _ := filepath.Rel(f.RepoDir, path)
			files = append(files, fileInfo{path: relPath, modTime: info.ModTime()})
		}
		return nil
	})
	if err != nil {
		return nil, err
	}

	// Sort by modification time descending
	sort.Slice(files, func(i, j int) bool {
		return files[i].modTime.After(files[j].modTime)
	})

	if len(files) > maxFiles {
		files = files[:maxFiles]
	}

	if len(files) == 0 {
		return nil, nil
	}

	var sb strings.Builder
	for _, f := range files {
		sb.WriteString(fmt.Sprintf("%s (modified %s)\n", f.path, f.modTime.Format("2006-01-02 15:04")))
	}

	items := []ContextItem{
		{
			Source:     "files",
			Title:      "Recently Modified Files",
			Content:    strings.TrimSpace(sb.String()),
			Relevance:  0.6,
			TokenCount: sb.Len() / 4,
		},
	}

	return items, nil
}

// ErrorContextProvider provides recent build/test errors as context.
type ErrorContextProvider struct {
	LogDir string
}

func (e *ErrorContextProvider) Name() string        { return "errors" }
func (e *ErrorContextProvider) Description() string { return "Recent build/test errors" }
func (e *ErrorContextProvider) TokenBudget() int    { return 400 }

func (e *ErrorContextProvider) Gather(ctx context.Context, query string) ([]ContextItem, error) {
	if e.LogDir == "" {
		return nil, nil
	}

	entries, err := os.ReadDir(e.LogDir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}

	type logFile struct {
		name    string
		modTime time.Time
	}

	var logs []logFile
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		info, err := entry.Info()
		if err != nil {
			continue
		}
		// Only include log/error files
		name := entry.Name()
		if strings.HasSuffix(name, ".log") || strings.HasSuffix(name, ".err") || strings.Contains(name, "error") {
			logs = append(logs, logFile{name: name, modTime: info.ModTime()})
		}
	}

	// Sort by modification time descending
	sort.Slice(logs, func(i, j int) bool {
		return logs[i].modTime.After(logs[j].modTime)
	})

	// Read the most recent error logs (up to 3)
	maxLogs := 3
	if len(logs) < maxLogs {
		maxLogs = len(logs)
	}

	var items []ContextItem
	for i := 0; i < maxLogs; i++ {
		path := filepath.Join(e.LogDir, logs[i].name)
		data, err := os.ReadFile(path)
		if err != nil {
			continue
		}
		content := string(data)
		// Truncate long logs
		if len(content) > 1000 {
			content = content[len(content)-1000:]
		}
		items = append(items, ContextItem{
			Source:     "errors",
			Title:      fmt.Sprintf("Error Log: %s", logs[i].name),
			Content:    strings.TrimSpace(content),
			Relevance:  0.85,
			TokenCount: len(content) / 4,
		})
	}

	return items, nil
}

// DependencyContextProvider provides dependency information from project files.
type DependencyContextProvider struct {
	ProjectDir string
}

func (d *DependencyContextProvider) Name() string        { return "dependencies" }
func (d *DependencyContextProvider) Description() string { return "Project dependency information" }
func (d *DependencyContextProvider) TokenBudget() int    { return 300 }

func (d *DependencyContextProvider) Gather(ctx context.Context, query string) ([]ContextItem, error) {
	var items []ContextItem

	// Try go.mod
	goModPath := filepath.Join(d.ProjectDir, "go.mod")
	if data, err := os.ReadFile(goModPath); err == nil {
		content := string(data)
		items = append(items, ContextItem{
			Source:     "dependencies",
			Title:      "Go Dependencies (go.mod)",
			Content:    strings.TrimSpace(content),
			Relevance:  0.5,
			TokenCount: len(content) / 4,
		})
	}

	// Try package.json
	pkgPath := filepath.Join(d.ProjectDir, "package.json")
	if data, err := os.ReadFile(pkgPath); err == nil {
		content := string(data)
		items = append(items, ContextItem{
			Source:     "dependencies",
			Title:      "Node Dependencies (package.json)",
			Content:    strings.TrimSpace(content),
			Relevance:  0.5,
			TokenCount: len(content) / 4,
		})
	}

	// Try requirements.txt
	reqPath := filepath.Join(d.ProjectDir, "requirements.txt")
	if data, err := os.ReadFile(reqPath); err == nil {
		content := string(data)
		items = append(items, ContextItem{
			Source:     "dependencies",
			Title:      "Python Dependencies (requirements.txt)",
			Content:    strings.TrimSpace(content),
			Relevance:  0.5,
			TokenCount: len(content) / 4,
		})
	}

	return items, nil
}

// EnvironmentContextProvider provides system and environment information.
type EnvironmentContextProvider struct{}

func (e *EnvironmentContextProvider) Name() string        { return "environment" }
func (e *EnvironmentContextProvider) Description() string { return "System and environment info" }
func (e *EnvironmentContextProvider) TokenBudget() int    { return 200 }

func (e *EnvironmentContextProvider) Gather(ctx context.Context, query string) ([]ContextItem, error) {
	var sb strings.Builder

	// OS info
	sb.WriteString(fmt.Sprintf("OS: %s/%s\n", runtime.GOOS, runtime.GOARCH))

	// Go version
	goCmd := exec.CommandContext(ctx, "go", "version")
	if out, err := goCmd.Output(); err == nil {
		sb.WriteString(fmt.Sprintf("Go: %s\n", strings.TrimSpace(string(out))))
	}

	// Node version
	nodeCmd := exec.CommandContext(ctx, "node", "--version")
	if out, err := nodeCmd.Output(); err == nil {
		sb.WriteString(fmt.Sprintf("Node: %s\n", strings.TrimSpace(string(out))))
	}

	// Git branch
	gitCmd := exec.CommandContext(ctx, "git", "rev-parse", "--abbrev-ref", "HEAD")
	if out, err := gitCmd.Output(); err == nil {
		sb.WriteString(fmt.Sprintf("Git Branch: %s\n", strings.TrimSpace(string(out))))
	}

	// Working directory
	if wd, err := os.Getwd(); err == nil {
		sb.WriteString(fmt.Sprintf("Working Dir: %s\n", wd))
	}

	content := sb.String()
	items := []ContextItem{
		{
			Source:     "environment",
			Title:      "Environment Info",
			Content:    strings.TrimSpace(content),
			Relevance:  0.4,
			TokenCount: len(content) / 4,
		},
	}

	return items, nil
}
