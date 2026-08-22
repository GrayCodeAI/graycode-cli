package config

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// Context-file reference budgets, mirroring goose `hints/import_files.rs`.
const (
	// maxRefDepth bounds recursive inlining depth.
	maxRefDepth = 3
	// maxRefOps bounds the total number of reference operations per root file.
	maxRefOps = 64
	// maxRefBytes bounds the total expanded output per root file.
	maxRefBytes = 1 << 20 // 1 MiB
	// maxRefFileSize bounds a single referenced file's parse size (ReDoS guard).
	maxRefFileSize = 128 << 10 // 128 KiB
)

// gitRoot finds the repository root for start by walking up for a `.git`
// entry (directory or file). Returns "" when no repo is found.
func gitRoot(start string) string {
	dir := start
	if abs, err := filepath.Abs(dir); err == nil {
		dir = abs
	}
	for {
		if _, err := os.Stat(filepath.Join(dir, ".git")); err == nil {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return ""
		}
		dir = parent
	}
}

// expandContextReferences inlines `@path` reference lines found in a context
// file (e.g. AGENTS.md) with recursive resolution bounded by the git root and
// strict size/depth budgets. Reference lines are a path prefixed by `@` on its
// own line (trimmed). Referenced file content is inserted in place of the
// reference line. Any violation (out-of-root path, budget, size, parse failure)
// skips that reference gracefully without failing the whole load.
func expandContextReferences(content, baseDir, root string) string {
	state := &refState{ops: 0}
	out := expandRecursive(content, baseDir, root, 0, state)
	if state.bytes > maxRefBytes {
		// The caller already truncated to maxAgentsMDSize; keep the budget
		// enforcement explicit so the referenced content never dominates.
		if len(out) > maxRefBytes {
			out = out[:maxRefBytes]
		}
	}
	return out
}

// refState tracks the cumulative budget across recursive reference expansion.
type refState struct {
	ops   int
	bytes int
}

func expandRecursive(content, baseDir, root string, depth int, state *refState) string {
	if depth > maxRefDepth {
		return content
	}
	lines := strings.Split(content, "\n")
	var out []string
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		ref, ok := parseReferenceLine(trimmed)
		if !ok {
			out = append(out, line)
			continue
		}
		if state.ops >= maxRefOps {
			out = append(out, "# [context] reference skipped: operation budget exhausted")
			continue
		}
		resolved, err := resolveReference(baseDir, root, ref)
		if err != nil {
			out = append(out, "# [context] reference skipped: "+err.Error())
			continue
		}
		data, err := os.ReadFile(resolved) // #nosec G304 -- resolved is constrained to the git root by resolveReference
		if err != nil {
			out = append(out, "# [context] reference skipped: cannot read "+ref)
			continue
		}
		if len(data) > maxRefFileSize {
			out = append(out, "# [context] reference skipped: file too large "+ref)
			continue
		}
		if state.bytes+len(data) > maxRefBytes {
			out = append(out, "# [context] reference skipped: byte budget exhausted")
			continue
		}
		state.ops++
		state.bytes += len(data)
		refContent := string(data)
		refContent = expandRecursive(refContent, filepath.Dir(resolved), root, depth+1, state)
		out = append(out, refContent)
	}
	return strings.Join(out, "\n")
}

// parseReferenceLine returns (path, true) when the line is a reference
// (`@` + a non-empty path without spaces), else (_, false).
func parseReferenceLine(trimmed string) (string, bool) {
	if !strings.HasPrefix(trimmed, "@") {
		return "", false
	}
	ref := strings.TrimSpace(strings.TrimPrefix(trimmed, "@"))
	if ref == "" || strings.ContainsAny(ref, " \t") {
		return "", false
	}
	return ref, true
}

// resolveReference resolves a reference path relative to baseDir and ensures it
// stays within root (the git root boundary).
func resolveReference(baseDir, root, ref string) (string, error) {
	if baseDir == "" {
		return "", fmt.Errorf("no base directory")
	}
	abs, err := filepath.Abs(filepath.Join(baseDir, ref))
	if err != nil {
		return "", fmt.Errorf("bad reference %q: %w", ref, err)
	}
	if root == "" {
		// No repo boundary; refuse absolute/escaping references for safety.
		if strings.HasPrefix(ref, "/") {
			return "", fmt.Errorf("absolute reference %q refused (no repo boundary)", ref)
		}
		return abs, nil
	}
	rootAbs, err := filepath.Abs(root)
	if err != nil {
		return "", fmt.Errorf("bad root %q: %w", root, err)
	}
	if !within(rootAbs, abs) {
		return "", fmt.Errorf("reference %q escapes the git root", ref)
	}
	return abs, nil
}

func within(root, path string) bool {
	rel, err := filepath.Rel(root, path)
	if err != nil {
		return false
	}
	return rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator))
}
