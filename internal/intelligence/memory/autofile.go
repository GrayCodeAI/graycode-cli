package memory

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"
)

// AutoFile manages a user-visible MEMORY.md that auto-discovers project conventions.
// It extracts durable patterns from conversation and writes them to a markdown file
// that loads at the start of every session.
type AutoFile struct {
	mu       sync.Mutex
	repoDir  string
	entries  map[string]AutoFileEntry
	maxLines int
}

// AutoFileEntry is a single discovered pattern for the auto-written MEMORY.md.
type AutoFileEntry struct {
	Category  string    `json:"category"`
	Pattern   string    `json:"pattern"`
	Detail    string    `json:"detail"`
	Source    string    `json:"source"` // "user", "agent", "auto"
	CreatedAt time.Time `json:"created_at"`
	Hits      int       `json:"hits"` // how many times this was confirmed
}

// NewAutoFile creates an AutoFile for the given repo directory.
func NewAutoFile(repoDir string) *AutoFile {
	if repoDir == "" {
		repoDir = "."
	}
	af := &AutoFile{
		repoDir:  repoDir,
		entries:  make(map[string]AutoFileEntry),
		maxLines: 200,
	}
	af.load()
	return af
}

// MemoryPath returns the path to the memory file.
func (af *AutoFile) MemoryPath() string {
	return filepath.Join(af.repoDir, ".hawk", "MEMORY.md")
}

// load reads existing memory entries from MEMORY.md.
func (af *AutoFile) load() {
	data, err := os.ReadFile(af.MemoryPath())
	if err != nil {
		return
	}
	content := string(data)
	lines := strings.Split(content, "\n")
	var current AutoFileEntry
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "## ") {
			current.Category = strings.TrimPrefix(line, "## ")
		} else if strings.HasPrefix(line, "- **") {
			if parts := strings.SplitN(line, "**", 3); len(parts) >= 3 {
				current.Pattern = strings.TrimSpace(parts[1])
				current.Detail = strings.TrimSpace(strings.TrimPrefix(parts[2], ":"))
				current.CreatedAt = time.Now()
				current.Hits = 1
				key := current.Category + ":" + current.Pattern
				af.entries[key] = current
			}
		}
	}
}

// Observe processes a user message or agent response and extracts potential memory entries.
func (af *AutoFile) Observe(ctx context.Context, role, content string) {
	af.mu.Lock()
	defer af.mu.Unlock()

	lower := strings.ToLower(content)

	// Detect build commands.
	if strings.Contains(lower, "run ") || strings.Contains(lower, "build ") || strings.Contains(lower, "install ") {
		if entry := extractBuildCommand(content); entry != nil {
			key := entry.Category + ":" + entry.Pattern
			if existing, ok := af.entries[key]; ok {
				existing.Hits++
				af.entries[key] = existing
			} else {
				entry.CreatedAt = time.Now()
				entry.Source = role
				af.entries[key] = *entry
			}
		}
	}

	// Detect test commands.
	if strings.Contains(lower, "test ") || strings.Contains(lower, "run test") || strings.Contains(lower, "pytest") || strings.Contains(lower, "go test") {
		if entry := extractTestCommand(content); entry != nil {
			key := entry.Category + ":" + entry.Pattern
			if existing, ok := af.entries[key]; ok {
				existing.Hits++
				af.entries[key] = existing
			} else {
				entry.CreatedAt = time.Now()
				entry.Source = role
				af.entries[key] = *entry
			}
		}
	}

	// Detect lint commands.
	if strings.Contains(lower, "lint ") || strings.Contains(lower, "golangci") || strings.Contains(lower, "eslint") || strings.Contains(lower, "ruff") {
		if entry := extractLintCommand(content); entry != nil {
			key := entry.Category + ":" + entry.Pattern
			if existing, ok := af.entries[key]; ok {
				existing.Hits++
				af.entries[key] = existing
			} else {
				entry.CreatedAt = time.Now()
				entry.Source = role
				af.entries[key] = *entry
			}
		}
	}
}

// Save writes the memory file to disk.
func (af *AutoFile) Save() error {
	af.mu.Lock()
	defer af.mu.Unlock()

	if len(af.entries) == 0 {
		return nil
	}

	dir := filepath.Dir(af.MemoryPath())
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}

	// Group by category.
	categories := make(map[string][]AutoFileEntry)
	for _, entry := range af.entries {
		categories[entry.Category] = append(categories[entry.Category], entry)
	}

	// Sort categories.
	var catNames []string
	for c := range categories {
		catNames = append(catNames, c)
	}
	sort.Strings(catNames)

	var b strings.Builder
	b.WriteString(fmt.Sprintf("# Project Memory\n\nAuto-discovered patterns from %s sessions.\n\n", time.Now().Format("2006-01-02")))

	remaining := af.maxLines
	for _, cat := range catNames {
		if remaining <= 0 {
			break
		}
		entries := categories[cat]
		sort.Slice(entries, func(i, j int) bool {
			return entries[i].Hits > entries[j].Hits
		})

		if remaining > 2 {
			b.WriteString(fmt.Sprintf("## %s\n", cat))
			remaining--
		}
		for _, e := range entries {
			if remaining <= 0 {
				break
			}
			b.WriteString(fmt.Sprintf("- **%s**: %s\n", e.Pattern, e.Detail))
			remaining--
		}
	}

	return os.WriteFile(af.MemoryPath(), []byte(b.String()), 0o644)
}

func extractBuildCommand(content string) *AutoFileEntry {
	// Look for patterns like "npm run build", "go build", "cargo build", etc.
	lower := strings.ToLower(content)
	switch {
	case strings.Contains(lower, "npm run"):
		return &AutoFileEntry{Category: "Build", Pattern: "npm run build", Detail: "Use `npm run build` to compile the project"}
	case strings.Contains(lower, "pnpm run"):
		return &AutoFileEntry{Category: "Build", Pattern: "pnpm run build", Detail: "Use `pnpm run build` to compile the project"}
	case strings.Contains(lower, "go build"):
		return &AutoFileEntry{Category: "Build", Pattern: "go build", Detail: "Use `go build ./...` to compile the project"}
	case strings.Contains(lower, "cargo build"):
		return &AutoFileEntry{Category: "Build", Pattern: "cargo build", Detail: "Use `cargo build` to compile the project"}
	case strings.Contains(lower, "make"):
		return &AutoFileEntry{Category: "Build", Pattern: "make", Detail: "The project uses Make for builds"}
	case strings.Contains(lower, "pip install") || strings.Contains(lower, "pip3 install"):
		return &AutoFileEntry{Category: "Build", Pattern: "pip install", Detail: "Use `pip install -r requirements.txt` to install dependencies"}
	case strings.Contains(lower, "bundle install"):
		return &AutoFileEntry{Category: "Build", Pattern: "bundle install", Detail: "Use `bundle install` to install Ruby dependencies"}
	default:
		return nil
	}
}

func extractTestCommand(content string) *AutoFileEntry {
	lower := strings.ToLower(content)
	switch {
	case strings.Contains(lower, "go test"):
		return &AutoFileEntry{Category: "Test", Pattern: "go test", Detail: "Use `go test ./...` to run all tests"}
	case strings.Contains(lower, "pytest"):
		return &AutoFileEntry{Category: "Test", Pattern: "pytest", Detail: "Use `pytest` to run the test suite"}
	case strings.Contains(lower, "npm test") || strings.Contains(lower, "npm run test"):
		return &AutoFileEntry{Category: "Test", Pattern: "npm test", Detail: "Use `npm test` to run the test suite"}
	case strings.Contains(lower, "pnpm test") || strings.Contains(lower, "pnpm run test"):
		return &AutoFileEntry{Category: "Test", Pattern: "pnpm test", Detail: "Use `pnpm test` to run the test suite"}
	case strings.Contains(lower, "cargo test"):
		return &AutoFileEntry{Category: "Test", Pattern: "cargo test", Detail: "Use `cargo test` to run all tests"}
	case strings.Contains(lower, "rails test") || strings.Contains(lower, "rspec"):
		return &AutoFileEntry{Category: "Test", Pattern: "rails test", Detail: "Use `rails test` or `rspec` to run tests"}
	default:
		return nil
	}
}

func extractLintCommand(content string) *AutoFileEntry {
	lower := strings.ToLower(content)
	switch {
	case strings.Contains(lower, "golangci"):
		return &AutoFileEntry{Category: "Lint", Pattern: "golangci-lint", Detail: "Use `golangci-lint run ./...` to lint the project"}
	case strings.Contains(lower, "eslint"):
		return &AutoFileEntry{Category: "Lint", Pattern: "eslint", Detail: "Use `npx eslint .` to lint JavaScript/TypeScript"}
	case strings.Contains(lower, "ruff"):
		return &AutoFileEntry{Category: "Lint", Pattern: "ruff", Detail: "Use `ruff check .` to lint Python"}
	case strings.Contains(lower, "gofumpt") || strings.Contains(lower, "go fmt"):
		return &AutoFileEntry{Category: "Lint", Pattern: "gofumpt", Detail: "Use `gofumpt -w .` to format Go code"}
	case strings.Contains(lower, "prettier"):
		return &AutoFileEntry{Category: "Lint", Pattern: "prettier", Detail: "Use `npx prettier --write .` to format code"}
	default:
		return nil
	}
}
