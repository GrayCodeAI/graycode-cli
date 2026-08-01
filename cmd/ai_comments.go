package cmd

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"github.com/GrayCodeAI/hawk/internal/fsutil"
)

// AIDirective represents a found AI comment directive in a source file.
type AIDirective struct {
	Path        string
	Line        int
	Instruction string
	Mode        string // "!" (do) or "?" (ask)
}

// aiCommentPatterns matches AI directives in various comment styles.
// Supported: // AI!, # AI!, /* AI! */, -- AI!, and the ? variants.
var aiCommentRe = regexp.MustCompile(
	`(?://|#|/\*|--)\s*AI([!?])\s*(.+?)(?:\s*\*/)?$`,
)

// aiSupportedExts are file extensions scanned for AI comments.
var aiSupportedExts = map[string]bool{
	".go": true, ".py": true, ".js": true, ".ts": true,
	".rs": true, ".java": true,
}

// scanForAIComments walks dir looking for AI directives in source files,
// skipping directories matching ignore patterns.
func scanForAIComments(dir string, ignore []string) []AIDirective {
	ignoreSet := make(map[string]bool, len(ignore))
	for _, p := range ignore {
		ignoreSet[p] = true
	}

	var directives []AIDirective

	_ = filepath.WalkDir(dir, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			if ignoreSet[filepath.Base(path)] {
				return filepath.SkipDir
			}
			return nil
		}
		ext := filepath.Ext(path)
		if !aiSupportedExts[ext] {
			return nil
		}
		data, err := fsutil.ReadPinnedFile(path)
		if err != nil {
			return nil
		}
		lines := strings.Split(string(data), "\n")
		for i, line := range lines {
			m := aiCommentRe.FindStringSubmatch(line)
			if m == nil {
				continue
			}
			mode := m[1]
			instruction := strings.TrimSpace(m[2])
			relPath, relErr := filepath.Rel(dir, path)
			if relErr != nil {
				relPath = path
			}
			directives = append(directives, AIDirective{
				Path:        relPath,
				Line:        i + 1,
				Instruction: instruction,
				Mode:        mode,
			})
		}
		return nil
	})

	return directives
}

// aiDispatchFn is the LLM/agent execution path used to act on a single AI
// directive. It defaults to runAIDirectivePrint (which drives the same chat/agent
// stream as `runPrint`), but is a package-level variable so tests can substitute
// a mock without standing up a real model.
var aiDispatchFn = runAIDirectivePrint

// runAIDirectivePrint dispatches a single AI directive to the existing print
// execution path. The prompt is built from the directive so the agent edits the
// referenced file in place.
func runAIDirectivePrint(d AIDirective) error {
	return runPrint(formatDirectivePrompt(d))
}

// formatDirectivePrompt builds a targeted prompt for a single AI directive.
// AI! directives instruct the agent to act now; AI? directives ask it to answer
// the embedded question.
func formatDirectivePrompt(d AIDirective) string {
	var b strings.Builder
	if d.Mode == "?" {
		b.WriteString(fmt.Sprintf(
			"An AI question comment was found at %s:%d.\n\nQuestion: %s\n\n",
			d.Path, d.Line, d.Instruction,
		))
		b.WriteString("Answer the question. If a code change is warranted, make it. ")
	} else {
		b.WriteString(fmt.Sprintf(
			"An AI instruction comment was found at %s:%d.\n\nInstruction: %s\n\n",
			d.Path, d.Line, d.Instruction,
		))
		b.WriteString("Implement this change now, editing the file in place. ")
	}
	b.WriteString(fmt.Sprintf(
		"The directive lives in the file %s; after you finish, the AI comment token will be removed automatically.\n",
		d.Path,
	))
	return b.String()
}

// processAIDirectives scans dir for AI!/AI? directives, dispatches each one to
// the configured execution path, and strips the AI comment token from the file
// after a successful dispatch. It returns the number of directives that were
// successfully processed (token stripped). Errors from individual directives are
// reported to stderr and do not abort the remaining directives.
//
// Directives are processed from the bottom of each file upward so that removing a
// line never shifts the line numbers of directives not yet handled in the same
// file.
func processAIDirectives(dir string, ignore []string) int {
	directives := scanForAIComments(dir, ignore)
	if len(directives) == 0 {
		return 0
	}

	// Sort by file then descending line so earlier-line removals don't invalidate
	// the line numbers of later directives in the same file.
	sort.SliceStable(directives, func(i, j int) bool {
		if directives[i].Path != directives[j].Path {
			return directives[i].Path < directives[j].Path
		}
		return directives[i].Line > directives[j].Line
	})

	processed := 0
	for _, d := range directives {
		if err := aiDispatchFn(d); err != nil {
			fmt.Fprintf(os.Stderr, "AI directive %s:%d failed: %v\n", d.Path, d.Line, err)
			continue
		}
		// Resolve back to an absolute path for removal; scan returns paths
		// relative to dir.
		full := d.Path
		if !filepath.IsAbs(full) {
			full = filepath.Join(dir, d.Path)
		}
		if err := removeAIComment(full, d.Line); err != nil {
			fmt.Fprintf(os.Stderr, "AI directive %s:%d: failed to strip token: %v\n", d.Path, d.Line, err)
			continue
		}
		processed++
	}
	return processed
}

// formatDirectivesAsPrompt formats found directives into a prompt string.
func formatDirectivesAsPrompt(directives []AIDirective) string {
	if len(directives) == 0 {
		return ""
	}
	var b strings.Builder
	b.WriteString("The following AI directives were found in your files:\n")
	for _, d := range directives {
		prefix := "DO"
		if d.Mode == "?" {
			prefix = "ASK"
		}
		b.WriteString(fmt.Sprintf("- %s:%d: [%s] %s\n", d.Path, d.Line, prefix, d.Instruction))
	}
	return b.String()
}

// removeAIComment removes the AI comment at the given line from the file.
// Line numbers are 1-based.
func removeAIComment(path string, line int) error {
	data, err := os.ReadFile(path) // #nosec G304 -- path comes from internal AI-comment scan results
	if err != nil {
		return err
	}
	lines := strings.Split(string(data), "\n")
	if line < 1 || line > len(lines) {
		return fmt.Errorf("line %d out of range (file has %d lines)", line, len(lines))
	}

	// Check if the entire line is just the AI comment (possibly with whitespace)
	trimmed := strings.TrimSpace(lines[line-1])
	if aiCommentRe.MatchString(trimmed) && !strings.Contains(trimmed, ";") {
		// Remove the entire line
		lines = append(lines[:line-1], lines[line:]...)
	} else {
		// Remove just the AI comment portion from the line
		lines[line-1] = aiCommentRe.ReplaceAllString(lines[line-1], "")
		lines[line-1] = strings.TrimRight(lines[line-1], " \t")
	}

	// #nosec G306 -- rewrites an existing project source file in place, matching typical source file permissions
	return fsutil.WritePinnedFile(path, []byte(strings.Join(lines, "\n")), 0o644)
}
