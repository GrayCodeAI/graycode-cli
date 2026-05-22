package workflow

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"
)

// WorkspaceState maintains awareness of the current project state including
// which files are open, modified, and staged. It provides workspace context
// to the agent for informed decision-making.
type WorkspaceState struct {
	OpenFiles     map[string]*FileState
	ModifiedFiles map[string]time.Time
	StagedFiles   []string
	ProjectDir    string
	LastScan      time.Time
	mu            sync.RWMutex

	// Internal tracking for change detection
	fileHashes map[string]string
	scanState  map[string]*FileState
}

// FileState represents the tracked state of a single file in the workspace.
type FileState struct {
	Path        string
	Size        int64
	ModTime     time.Time
	Language    string
	IsTest      bool
	IsGenerated bool
	Hash        string
}

// NewWorkspaceState creates a new WorkspaceState for the given project directory.
func NewWorkspaceState(projectDir string) *WorkspaceState {
	return &WorkspaceState{
		OpenFiles:     make(map[string]*FileState),
		ModifiedFiles: make(map[string]time.Time),
		StagedFiles:   nil,
		ProjectDir:    projectDir,
		fileHashes:    make(map[string]string),
		scanState:     make(map[string]*FileState),
	}
}

// Scan walks the project directory and updates file states, detecting changes
// since the last scan.
func (ws *WorkspaceState) Scan() error {
	ws.mu.Lock()
	defer ws.mu.Unlock()

	newScanState := make(map[string]*FileState)

	err := filepath.Walk(ws.ProjectDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return nil // skip inaccessible files
		}

		// Skip hidden directories and common non-source directories
		if info.IsDir() {
			base := filepath.Base(path)
			if strings.HasPrefix(base, ".") || base == "node_modules" || base == "vendor" || base == "__pycache__" {
				return filepath.SkipDir
			}
			return nil
		}

		// Skip binary and non-source files
		if !isSourceFile(path) {
			return nil
		}

		relPath, _ := filepath.Rel(ws.ProjectDir, path)
		hash, _ := hashFile(path)

		fs := &FileState{
			Path:        relPath,
			Size:        info.Size(),
			ModTime:     info.ModTime(),
			Language:    wsDetectLanguage(path),
			IsTest:      wsIsTestFile(path),
			IsGenerated: isGeneratedFile(path),
			Hash:        hash,
		}

		newScanState[relPath] = fs
		return nil
	})
	if err != nil {
		return fmt.Errorf("workspace scan failed: %w", err)
	}

	ws.scanState = newScanState
	ws.LastScan = time.Now()
	return nil
}

// MarkOpened tracks that the agent has read this file.
func (ws *WorkspaceState) MarkOpened(path string) {
	ws.mu.Lock()
	defer ws.mu.Unlock()

	relPath := ws.toRelPath(path)

	info, err := os.Stat(ws.toAbsPath(relPath))
	if err != nil {
		// File may not exist, store minimal state
		ws.OpenFiles[relPath] = &FileState{Path: relPath}
		return
	}

	hash, _ := hashFile(ws.toAbsPath(relPath))

	ws.OpenFiles[relPath] = &FileState{
		Path:        relPath,
		Size:        info.Size(),
		ModTime:     info.ModTime(),
		Language:    wsDetectLanguage(relPath),
		IsTest:      wsIsTestFile(relPath),
		IsGenerated: isGeneratedFile(relPath),
		Hash:        hash,
	}

	// Track the hash at time of opening for change detection
	ws.fileHashes[relPath] = hash
}

// MarkModified tracks that the agent has modified this file.
func (ws *WorkspaceState) MarkModified(path string) {
	ws.mu.Lock()
	defer ws.mu.Unlock()

	relPath := ws.toRelPath(path)
	ws.ModifiedFiles[relPath] = time.Now()

	// Update the hash to reflect the modification
	hash, _ := hashFile(ws.toAbsPath(relPath))
	ws.fileHashes[relPath] = hash
}

// MarkStaged tracks that a file is staged for commit.
func (ws *WorkspaceState) MarkStaged(path string) {
	ws.mu.Lock()
	defer ws.mu.Unlock()

	relPath := ws.toRelPath(path)

	// Avoid duplicates
	for _, f := range ws.StagedFiles {
		if f == relPath {
			return
		}
	}
	ws.StagedFiles = append(ws.StagedFiles, relPath)
}

// GetModified returns all files modified in this session.
func (ws *WorkspaceState) GetModified() []string {
	ws.mu.RLock()
	defer ws.mu.RUnlock()

	result := make([]string, 0, len(ws.ModifiedFiles))
	for path := range ws.ModifiedFiles {
		result = append(result, path)
	}
	sort.Strings(result)
	return result
}

// GetOpened returns all files the agent has read.
func (ws *WorkspaceState) GetOpened() []string {
	ws.mu.RLock()
	defer ws.mu.RUnlock()

	result := make([]string, 0, len(ws.OpenFiles))
	for path := range ws.OpenFiles {
		result = append(result, path)
	}
	sort.Strings(result)
	return result
}

// HasChanged returns true if the file has changed on disk since it was last read.
func (ws *WorkspaceState) HasChanged(path string) bool {
	ws.mu.RLock()
	defer ws.mu.RUnlock()

	relPath := ws.toRelPath(path)

	oldHash, tracked := ws.fileHashes[relPath]
	if !tracked {
		return false
	}

	currentHash, err := hashFile(ws.toAbsPath(relPath))
	if err != nil {
		return true // if we can't read it, assume changed
	}

	return currentHash != oldHash
}

// DetectExternalChanges returns files that changed outside of hawk's modifications.
func (ws *WorkspaceState) DetectExternalChanges() []string {
	ws.mu.RLock()
	defer ws.mu.RUnlock()

	var changed []string

	for path, oldHash := range ws.fileHashes {
		// Skip files we modified ourselves
		if _, modified := ws.ModifiedFiles[path]; modified {
			continue
		}

		currentHash, err := hashFile(ws.toAbsPath(path))
		if err != nil {
			// File deleted or inaccessible — that's an external change
			changed = append(changed, path)
			continue
		}

		if currentHash != oldHash {
			changed = append(changed, path)
		}
	}

	sort.Strings(changed)
	return changed
}

// Summary returns a human-readable summary of the workspace state.
func (ws *WorkspaceState) Summary() string {
	ws.mu.RLock()
	defer ws.mu.RUnlock()

	var b strings.Builder

	b.WriteString("Workspace State:\n")
	b.WriteString("─────────────────\n")

	// Project line with detected language
	lang := ws.detectProjectLanguage()
	if lang != "" {
		_, _ = fmt.Fprintf(&b, "Project: %s (%s)\n", ws.ProjectDir, lang)
	} else {
		_, _ = fmt.Fprintf(&b, "Project: %s\n", ws.ProjectDir)
	}

	// Modified files
	if len(ws.ModifiedFiles) > 0 {
		names := make([]string, 0, len(ws.ModifiedFiles))
		for path := range ws.ModifiedFiles {
			names = append(names, path)
		}
		sort.Strings(names)
		_, _ = fmt.Fprintf(&b, "Modified: %d files (%s)\n", len(names), strings.Join(names, ", "))
	} else {
		b.WriteString("Modified: 0 files\n")
	}

	// Opened files
	_, _ = fmt.Fprintf(&b, "Opened: %d files\n", len(ws.OpenFiles))

	// Staged files
	_, _ = fmt.Fprintf(&b, "Staged: %d files\n", len(ws.StagedFiles))

	// External changes
	externalChanges := ws.detectExternalChangesLocked()
	if len(externalChanges) > 0 {
		descriptions := make([]string, 0, len(externalChanges))
		for _, path := range externalChanges {
			descriptions = append(descriptions, fmt.Sprintf("%s updated", filepath.Base(path)))
		}
		_, _ = fmt.Fprintf(&b, "External changes: %d file (%s)\n", len(externalChanges), strings.Join(descriptions, ", "))
	} else {
		b.WriteString("External changes: none\n")
	}

	// Last scan
	if !ws.LastScan.IsZero() {
		ago := time.Since(ws.LastScan).Round(time.Second)
		_, _ = fmt.Fprintf(&b, "Last scan: %s ago\n", ago)
	} else {
		b.WriteString("Last scan: never\n")
	}

	return b.String()
}

// BuildContextForAgent formats workspace awareness for inclusion in a system prompt.
func (ws *WorkspaceState) BuildContextForAgent() string {
	ws.mu.RLock()
	defer ws.mu.RUnlock()

	var b strings.Builder

	b.WriteString("<workspace_state>\n")
	_, _ = fmt.Fprintf(&b, "project_dir: %s\n", ws.ProjectDir)

	// Recently modified
	if len(ws.ModifiedFiles) > 0 {
		b.WriteString("modified_files:\n")
		names := make([]string, 0, len(ws.ModifiedFiles))
		for path := range ws.ModifiedFiles {
			names = append(names, path)
		}
		sort.Strings(names)
		for _, name := range names {
			_, _ = fmt.Fprintf(&b, "  - %s (at %s)\n", name, ws.ModifiedFiles[name].Format(time.RFC3339))
		}
	}

	// Open files with context
	if len(ws.OpenFiles) > 0 {
		b.WriteString("open_files:\n")
		names := make([]string, 0, len(ws.OpenFiles))
		for path := range ws.OpenFiles {
			names = append(names, path)
		}
		sort.Strings(names)
		for _, name := range names {
			fs := ws.OpenFiles[name]
			_, _ = fmt.Fprintf(&b, "  - %s [%s", name, fs.Language)
			if fs.IsTest {
				b.WriteString(", test")
			}
			if fs.IsGenerated {
				b.WriteString(", generated")
			}
			b.WriteString("]\n")
		}
	}

	// Staged files
	if len(ws.StagedFiles) > 0 {
		b.WriteString("staged_files:\n")
		for _, path := range ws.StagedFiles {
			_, _ = fmt.Fprintf(&b, "  - %s\n", path)
		}
	}

	// External changes warning
	externalChanges := ws.detectExternalChangesLocked()
	if len(externalChanges) > 0 {
		b.WriteString("external_changes:\n")
		for _, path := range externalChanges {
			_, _ = fmt.Fprintf(&b, "  - %s (changed outside hawk)\n", path)
		}
	}

	b.WriteString("</workspace_state>")
	return b.String()
}

// Reset clears all tracking state.
func (ws *WorkspaceState) Reset() {
	ws.mu.Lock()
	defer ws.mu.Unlock()

	ws.OpenFiles = make(map[string]*FileState)
	ws.ModifiedFiles = make(map[string]time.Time)
	ws.StagedFiles = nil
	ws.fileHashes = make(map[string]string)
	ws.scanState = make(map[string]*FileState)
	ws.LastScan = time.Time{}
}

// detectExternalChangesLocked detects external changes without acquiring the lock.
// Caller must hold at least a read lock.
func (ws *WorkspaceState) detectExternalChangesLocked() []string {
	var changed []string

	for path, oldHash := range ws.fileHashes {
		if _, modified := ws.ModifiedFiles[path]; modified {
			continue
		}

		currentHash, err := hashFile(ws.toAbsPath(path))
		if err != nil {
			changed = append(changed, path)
			continue
		}

		if currentHash != oldHash {
			changed = append(changed, path)
		}
	}

	sort.Strings(changed)
	return changed
}

// detectProjectLanguage determines the primary language of the project.
func (ws *WorkspaceState) detectProjectLanguage() string {
	langCount := make(map[string]int)

	for _, fs := range ws.scanState {
		if fs.Language != "" && !fs.IsGenerated {
			langCount[fs.Language]++
		}
	}

	// Also consider open files if no scan has been done
	if len(langCount) == 0 {
		for _, fs := range ws.OpenFiles {
			if fs.Language != "" {
				langCount[fs.Language]++
			}
		}
	}

	if len(langCount) == 0 {
		return ""
	}

	// Find the most common language
	var topLang string
	var topCount int
	for lang, count := range langCount {
		if count > topCount {
			topLang = lang
			topCount = count
		}
	}
	return topLang
}

// toRelPath converts a path to a relative path from the project directory.
func (ws *WorkspaceState) toRelPath(path string) string {
	if filepath.IsAbs(path) {
		rel, err := filepath.Rel(ws.ProjectDir, path)
		if err == nil {
			return rel
		}
	}
	return path
}

// toAbsPath converts a relative path to an absolute path.
func (ws *WorkspaceState) toAbsPath(path string) string {
	if filepath.IsAbs(path) {
		return path
	}
	return filepath.Join(ws.ProjectDir, path)
}

// hashFile computes the SHA-256 hash of a file's contents.
func hashFile(path string) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer func() { _ = f.Close() }()

	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return "", err
	}

	return hex.EncodeToString(h.Sum(nil)), nil
}

// wsDetectLanguage determines the programming language of a file based on its extension.
func wsDetectLanguage(path string) string {
	ext := strings.ToLower(filepath.Ext(path))
	switch ext {
	case ".go":
		return "Go"
	case ".py":
		return "Python"
	case ".js":
		return "JavaScript"
	case ".ts":
		return "TypeScript"
	case ".tsx":
		return "TypeScript"
	case ".jsx":
		return "JavaScript"
	case ".rs":
		return "Rust"
	case ".java":
		return "Java"
	case ".rb":
		return "Ruby"
	case ".c", ".h":
		return "C"
	case ".cpp", ".cc", ".cxx", ".hpp":
		return "C++"
	case ".cs":
		return "C#"
	case ".swift":
		return "Swift"
	case ".kt":
		return "Kotlin"
	case ".php":
		return "PHP"
	case ".sh", ".bash":
		return "Shell"
	case ".yaml", ".yml":
		return "YAML"
	case ".json":
		return "JSON"
	case ".toml":
		return "TOML"
	case ".md":
		return "Markdown"
	case ".sql":
		return "SQL"
	case ".html", ".htm":
		return "HTML"
	case ".css":
		return "CSS"
	case ".scss":
		return "SCSS"
	case ".lua":
		return "Lua"
	case ".zig":
		return "Zig"
	default:
		return ""
	}
}

// wsIsTestFile determines if a file is a test file based on naming conventions.
func wsIsTestFile(path string) bool {
	base := filepath.Base(path)
	ext := filepath.Ext(path)
	name := strings.TrimSuffix(base, ext)

	// Go: *_test.go
	if strings.HasSuffix(base, "_test.go") {
		return true
	}
	// Python: test_* or *_test.py
	if strings.HasPrefix(name, "test_") || strings.HasSuffix(name, "_test") {
		return true
	}
	// JS/TS: *.test.* or *.spec.*
	if strings.Contains(base, ".test.") || strings.Contains(base, ".spec.") {
		return true
	}
	// Java: *Test.java
	if ext == ".java" && strings.HasSuffix(name, "Test") {
		return true
	}
	// Rust: in tests/ or test/ directory
	if strings.Contains(path, "/tests/") || strings.Contains(path, "/test/") ||
		strings.HasPrefix(path, "tests/") || strings.HasPrefix(path, "test/") {
		return true
	}
	return false
}

// isGeneratedFile detects if a file is auto-generated based on common patterns.
func isGeneratedFile(path string) bool {
	base := filepath.Base(path)

	// Common generated file names
	generatedNames := []string{
		"generated", "gen_", "_gen.", ".gen.",
		"mock_", "_mock.", ".mock.",
		".pb.go", ".pb.cc", ".pb.h",
		"_generated.", "_string.", "_enumer.",
	}

	lower := strings.ToLower(base)
	for _, pattern := range generatedNames {
		if strings.Contains(lower, pattern) {
			return true
		}
	}

	// Lock files
	if base == "go.sum" || base == "package-lock.json" || base == "yarn.lock" || base == "Cargo.lock" || base == "Gemfile.lock" {
		return true
	}

	return false
}

// isSourceFile checks if a file is likely a source/text file worth tracking.
func isSourceFile(path string) bool {
	ext := strings.ToLower(filepath.Ext(path))
	sourceExts := map[string]bool{
		".go": true, ".py": true, ".js": true, ".ts": true, ".tsx": true, ".jsx": true,
		".rs": true, ".java": true, ".rb": true, ".c": true, ".h": true,
		".cpp": true, ".cc": true, ".cxx": true, ".hpp": true, ".cs": true,
		".swift": true, ".kt": true, ".php": true, ".sh": true, ".bash": true,
		".yaml": true, ".yml": true, ".json": true, ".toml": true,
		".md": true, ".txt": true, ".sql": true, ".html": true, ".htm": true,
		".css": true, ".scss": true, ".lua": true, ".zig": true,
		".proto": true, ".graphql": true, ".gql": true,
		".xml": true, ".svg": true, ".env": true,
		".mod": true, ".sum": true, ".lock": true,
		".cfg": true, ".ini": true, ".conf": true,
		".makefile": true, ".dockerfile": true,
	}

	if sourceExts[ext] {
		return true
	}

	// Check for extensionless known files
	base := filepath.Base(path)
	knownFiles := map[string]bool{
		"Makefile": true, "Dockerfile": true, "Rakefile": true,
		"Gemfile": true, "Vagrantfile": true, ".gitignore": true,
		".dockerignore": true, "LICENSE": true,
	}

	return knownFiles[base]
}
