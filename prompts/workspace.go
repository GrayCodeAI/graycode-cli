package prompts

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
)

// WorkspaceContext gathers git and project info for prompt injection.
type WorkspaceContext struct {
	GitBranch     string
	GitStatus     string   // short status (modified files)
	RecentCommits []string // last 5 commit onelines
	TopFiles      []string // top-level files/dirs
	Language      string   // detected primary language
}

// GatherWorkspaceContext collects workspace info from the given directory.
// It uses the filesystem for top-level files and detects language from extensions.
// It uses git commands for branch, status, and recent commits.
func GatherWorkspaceContext(dir string) *WorkspaceContext {
	ctx := &WorkspaceContext{}

	// Read top-level files/dirs
	entries, err := os.ReadDir(dir)
	if err == nil {
		for _, e := range entries {
			name := e.Name()
			if strings.HasPrefix(name, ".") {
				continue
			}
			if e.IsDir() {
				ctx.TopFiles = append(ctx.TopFiles, name+"/")
			} else {
				ctx.TopFiles = append(ctx.TopFiles, name)
			}
		}
	}

	// Detect language from file extensions
	ctx.Language = detectLanguage(dir)

	// Git branch — try filesystem first
	ctx.GitBranch = readGitBranch(dir)

	// Git status (short) — requires exec
	if out, err := gitCmd(dir, "status", "--short"); err == nil {
		lines := strings.Split(strings.TrimSpace(out), "\n")
		modifiedCount := 0
		for _, l := range lines {
			if strings.TrimSpace(l) != "" {
				modifiedCount++
			}
		}
		if modifiedCount > 0 {
			ctx.GitStatus = fmt.Sprintf("%d files modified", modifiedCount)
		} else {
			ctx.GitStatus = "clean"
		}
	}

	// Recent commits (last 5 onelines)
	if out, err := gitCmd(dir, "log", "--oneline", "-5"); err == nil {
		for _, line := range strings.Split(strings.TrimSpace(out), "\n") {
			line = strings.TrimSpace(line)
			if line != "" {
				// Strip the short hash, keep just the message
				if idx := strings.IndexByte(line, ' '); idx > 0 {
					ctx.RecentCommits = append(ctx.RecentCommits, line[idx+1:])
				} else {
					ctx.RecentCommits = append(ctx.RecentCommits, line)
				}
			}
		}
	}

	return ctx
}

// Format returns the workspace context as a prompt section.
func (w *WorkspaceContext) Format() string {
	if w == nil {
		return ""
	}

	var b strings.Builder
	b.WriteString("## Project Context\n")

	if w.GitBranch != "" {
		b.WriteString("Branch: " + w.GitBranch)
		if w.GitStatus != "" {
			b.WriteString(" (" + w.GitStatus + ")")
		}
		b.WriteString("\n")
	}

	if len(w.RecentCommits) > 0 {
		b.WriteString("Recent: " + strings.Join(w.RecentCommits, " / ") + "\n")
	}

	if len(w.TopFiles) > 0 {
		dirs := w.TopFiles
		if len(dirs) > 10 {
			dirs = dirs[:10]
		}
		langNote := ""
		if w.Language != "" {
			langNote = " (" + w.Language + " project)"
		}
		b.WriteString("Structure: " + strings.Join(dirs, " ") + langNote + "\n")
	}

	return b.String()
}

// readGitBranch reads the current branch from .git/HEAD without spawning a subprocess.
func readGitBranch(dir string) string {
	headPath := filepath.Join(dir, ".git", "HEAD")
	data, err := os.ReadFile(headPath)
	if err != nil {
		// .git might be a worktree file
		gitFile := filepath.Join(dir, ".git")
		gitData, fErr := os.ReadFile(gitFile)
		if fErr != nil {
			return ""
		}
		line := strings.TrimSpace(string(gitData))
		if strings.HasPrefix(line, "gitdir: ") {
			gitDir := strings.TrimPrefix(line, "gitdir: ")
			if !filepath.IsAbs(gitDir) {
				gitDir = filepath.Join(dir, gitDir)
			}
			headPath = filepath.Join(gitDir, "HEAD")
			data, err = os.ReadFile(headPath)
			if err != nil {
				return ""
			}
		} else {
			return ""
		}
	}
	head := strings.TrimSpace(string(data))
	if strings.HasPrefix(head, "ref: refs/heads/") {
		return strings.TrimPrefix(head, "ref: refs/heads/")
	}
	if len(head) >= 8 {
		return head[:8] + " (detached)"
	}
	return ""
}

// detectLanguage examines file extensions in the directory to guess the primary language.
func detectLanguage(dir string) string {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return ""
	}

	extCount := map[string]int{}
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		ext := strings.ToLower(filepath.Ext(e.Name()))
		if ext != "" {
			extCount[ext]++
		}
	}

	// Also check one level deep for common source directories
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		name := e.Name()
		if strings.HasPrefix(name, ".") || name == "node_modules" || name == "vendor" || name == "__pycache__" {
			continue
		}
		subEntries, err := os.ReadDir(filepath.Join(dir, name))
		if err != nil {
			continue
		}
		for _, se := range subEntries {
			if se.IsDir() {
				continue
			}
			ext := strings.ToLower(filepath.Ext(se.Name()))
			if ext != "" {
				extCount[ext]++
			}
		}
	}

	langMap := map[string]string{
		".go":   "Go",
		".py":   "Python",
		".js":   "JavaScript",
		".ts":   "TypeScript",
		".tsx":  "TypeScript",
		".jsx":  "JavaScript",
		".rs":   "Rust",
		".rb":   "Ruby",
		".java": "Java",
		".kt":   "Kotlin",
		".cs":   "C#",
		".cpp":  "C++",
		".c":    "C",
		".swift": "Swift",
		".php":  "PHP",
	}

	type langCount struct {
		lang  string
		count int
	}
	var langs []langCount
	seen := map[string]bool{}
	for ext, count := range extCount {
		lang, ok := langMap[ext]
		if !ok {
			continue
		}
		if seen[lang] {
			// Accumulate (e.g., .js and .jsx both map to JavaScript)
			for i := range langs {
				if langs[i].lang == lang {
					langs[i].count += count
					break
				}
			}
			continue
		}
		seen[lang] = true
		langs = append(langs, langCount{lang: lang, count: count})
	}

	if len(langs) == 0 {
		return ""
	}

	sort.Slice(langs, func(i, j int) bool {
		return langs[i].count > langs[j].count
	})

	return langs[0].lang
}

// gitCmd runs a git command in the given directory and returns its output.
func gitCmd(dir string, args ...string) (string, error) {
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	out, err := cmd.Output()
	return strings.TrimSpace(string(out)), err
}
