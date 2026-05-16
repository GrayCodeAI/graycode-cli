package memory

import (
	"fmt"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
)

// SessionDiffAnalyzer examines what changed during a session and extracts
// memories from the diff: new files → purpose, new deps → choices,
// conventions followed/violated → confidence updates.
type SessionDiffAnalyzer struct {
	bridge       *YaadBridge
	projectDir   string
	startFiles   map[string]string // path → content hash at session start
	mu           sync.Mutex
}

// DiffResult holds the analysis of what changed during a session.
type DiffResult struct {
	NewFiles      []string
	ModifiedFiles []string
	DeletedFiles  []string
	NewDeps       []string
	Commits       []string
}

// NewSessionDiffAnalyzer creates a diff analyzer for a project directory.
func NewSessionDiffAnalyzer(bridge *YaadBridge, projectDir string) *SessionDiffAnalyzer {
	return &SessionDiffAnalyzer{
		bridge:     bridge,
		projectDir: projectDir,
		startFiles: make(map[string]string),
	}
}

// SnapshotStart captures the project state at session start for later diffing.
func (sd *SessionDiffAnalyzer) SnapshotStart() {
	sd.mu.Lock()
	defer sd.mu.Unlock()

	// Use git to capture current state
	out, err := exec.Command("git", "-C", sd.projectDir, "status", "--porcelain").Output()
	if err != nil {
		return
	}
	// Store current HEAD for later comparison
	head, err := exec.Command("git", "-C", sd.projectDir, "rev-parse", "HEAD").Output()
	if err != nil {
		return
	}
	sd.startFiles["__HEAD__"] = strings.TrimSpace(string(head))
	_ = out // we'll diff against HEAD at session end
}

// AnalyzeEnd computes the diff between session start and end, extracting memories.
func (sd *SessionDiffAnalyzer) AnalyzeEnd() *DiffResult {
	sd.mu.Lock()
	defer sd.mu.Unlock()

	result := &DiffResult{}
	startHead := sd.startFiles["__HEAD__"]
	if startHead == "" {
		return result
	}

	// Get diff since session start
	diffCmd := exec.Command("git", "-C", sd.projectDir, "diff", "--name-status", startHead+"..HEAD")
	out, err := diffCmd.Output()
	if err != nil {
		// Fall back to uncommitted changes
		diffCmd = exec.Command("git", "-C", sd.projectDir, "diff", "--name-status", startHead)
		out, _ = diffCmd.Output()
	}

	for _, line := range strings.Split(string(out), "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		parts := strings.Fields(line)
		if len(parts) < 2 {
			continue
		}
		status := parts[0]
		file := parts[1]

		switch {
		case status == "A":
			result.NewFiles = append(result.NewFiles, file)
		case status == "M":
			result.ModifiedFiles = append(result.ModifiedFiles, file)
		case status == "D":
			result.DeletedFiles = append(result.DeletedFiles, file)
		}
	}

	// Get new commits since session start
	logCmd := exec.Command("git", "-C", sd.projectDir, "log", "--oneline", startHead+"..HEAD")
	logOut, err := logCmd.Output()
	if err == nil {
		for _, line := range strings.Split(string(logOut), "\n") {
			line = strings.TrimSpace(line)
			if line != "" {
				result.Commits = append(result.Commits, line)
			}
		}
	}

	// Detect new dependencies
	result.NewDeps = sd.detectNewDeps(result.ModifiedFiles, startHead)

	return result
}

// StoreMemoriesFromDiff extracts and stores memories based on the session diff.
func (sd *SessionDiffAnalyzer) StoreMemoriesFromDiff(diff *DiffResult) {
	if !sd.bridge.Ready() || diff == nil {
		return
	}

	// New files → remember their purpose
	for _, f := range diff.NewFiles {
		ext := filepath.Ext(f)
		basename := filepath.Base(f)
		dir := filepath.Dir(f)

		content := fmt.Sprintf("New file created: %s in %s", basename, dir)
		if isTestFile(f) {
			content = fmt.Sprintf("Test file: %s (tests for %s)", basename, dir)
		} else if isConfigFile(f) {
			content = fmt.Sprintf("Config file: %s (%s)", basename, ext)
		}
		_ = sd.bridge.Remember(content, "file")
	}

	// New dependencies → remember as decisions
	for _, dep := range diff.NewDeps {
	_ = sd.bridge.Remember(
			fmt.Sprintf("Dependency added: %s", dep),
			"decision",
		)
	}

	// Commits → extract decisions from commit messages
	for _, commit := range diff.Commits {
		// Skip merge commits and trivial commits
		if strings.Contains(commit, "Merge") || len(commit) < 20 {
			continue
		}
		// Remove the hash prefix
		parts := strings.SplitN(commit, " ", 2)
		if len(parts) > 1 {
	_ = sd.bridge.Remember(
				fmt.Sprintf("Decision: %s", parts[1]),
				"decision",
			)
		}
	}
}

func (sd *SessionDiffAnalyzer) detectNewDeps(modifiedFiles []string, startHead string) []string {
	var newDeps []string

	for _, f := range modifiedFiles {
		base := filepath.Base(f)
		switch base {
		case "package.json", "go.mod", "Cargo.toml", "pyproject.toml", "requirements.txt":
			// Get the diff for this specific file
			cmd := exec.Command("git", "-C", sd.projectDir, "diff", startHead, "--", f)
			out, err := cmd.Output()
			if err != nil {
				continue
			}
			// Extract added lines that look like dependencies
			for _, line := range strings.Split(string(out), "\n") {
				if !strings.HasPrefix(line, "+") || strings.HasPrefix(line, "+++") {
					continue
				}
				dep := extractDepFromLine(line, base)
				if dep != "" {
					newDeps = append(newDeps, dep)
				}
			}
		}
	}
	return newDeps
}

func extractDepFromLine(line, filename string) string {
	line = strings.TrimPrefix(line, "+")
	line = strings.TrimSpace(line)

	switch filename {
	case "package.json":
		// "package-name": "^1.0.0"
		if strings.Contains(line, `"`) && strings.Contains(line, ":") {
			parts := strings.SplitN(line, ":", 2)
			name := strings.Trim(parts[0], `" ,`)
			if name != "" && !strings.HasPrefix(name, "@types") {
				return name
			}
		}
	case "go.mod":
		// github.com/foo/bar v1.2.3
		if strings.Contains(line, "/") && !strings.HasPrefix(line, "//") {
			parts := strings.Fields(line)
			if len(parts) >= 1 {
				return parts[0]
			}
		}
	case "Cargo.toml":
		// package = "version"
		if strings.Contains(line, "=") {
			parts := strings.SplitN(line, "=", 2)
			name := strings.TrimSpace(parts[0])
			if name != "" {
				return name
			}
		}
	case "requirements.txt", "pyproject.toml":
		// package==version or package>=version
		parts := strings.FieldsFunc(line, func(r rune) bool {
			return r == '=' || r == '>' || r == '<' || r == '~'
		})
		if len(parts) >= 1 && parts[0] != "" {
			return parts[0]
		}
	}
	return ""
}

func isTestFile(path string) bool {
	lower := strings.ToLower(path)
	return strings.Contains(lower, "_test") || strings.Contains(lower, ".test.") ||
		strings.Contains(lower, ".spec.") || strings.Contains(lower, "__tests__")
}

func isConfigFile(path string) bool {
	base := strings.ToLower(filepath.Base(path))
	configs := []string{
		"config", "settings", ".env", "dockerfile", "docker-compose",
		"makefile", "tsconfig", "webpack", "vite", "eslint", "prettier",
	}
	for _, c := range configs {
		if strings.Contains(base, c) {
			return true
		}
	}
	return false
}
