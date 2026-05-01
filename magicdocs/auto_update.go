package magicdocs

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

const magicDocMarker = "# MAGIC DOC:"
const staleDuration = 24 * time.Hour

// MagicDocFile represents a file containing auto-update markers.
type MagicDocFile struct {
	Path        string
	Title       string
	LastUpdated time.Time
}

// ScanForMagicDocs finds files containing "# MAGIC DOC:" markers in the given
// directory tree. It skips hidden directories and common non-source paths.
func ScanForMagicDocs(dir string) []MagicDocFile {
	var docs []MagicDocFile

	_ = filepath.Walk(dir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return nil
		}

		base := filepath.Base(path)

		// Skip hidden directories and common non-source paths
		if info.IsDir() {
			if strings.HasPrefix(base, ".") || base == "node_modules" || base == "vendor" || base == "__pycache__" {
				return filepath.SkipDir
			}
			return nil
		}

		// Only scan text-like files
		ext := filepath.Ext(path)
		if !isTextExt(ext) {
			return nil
		}

		doc := scanFileForMarker(path, info)
		if doc != nil {
			docs = append(docs, *doc)
		}

		return nil
	})

	return docs
}

func scanFileForMarker(path string, info os.FileInfo) *MagicDocFile {
	f, err := os.Open(path)
	if err != nil {
		return nil
	}
	defer f.Close()

	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := scanner.Text()
		if idx := strings.Index(line, magicDocMarker); idx >= 0 {
			title := strings.TrimSpace(line[idx+len(magicDocMarker):])
			return &MagicDocFile{
				Path:        path,
				Title:       title,
				LastUpdated: info.ModTime(),
			}
		}
	}

	return nil
}

// NeedsUpdate returns true if the magic doc is stale (>24h since last update).
func (m *MagicDocFile) NeedsUpdate() bool {
	return time.Since(m.LastUpdated) > staleDuration
}

// GenerateUpdatePrompt creates the prompt for the LLM to regenerate the doc.
func (m *MagicDocFile) GenerateUpdatePrompt() string {
	return fmt.Sprintf(
		"Update the magic doc in %s (title: %q). "+
			"Read the current file content, analyze the surrounding codebase for changes since the doc was last updated (%s), "+
			"and regenerate the documentation section marked with '# MAGIC DOC: %s'. "+
			"Keep the same format and marker comments. Only update the content between the markers.",
		m.Path,
		m.Title,
		m.LastUpdated.Format(time.RFC3339),
		m.Title,
	)
}

func isTextExt(ext string) bool {
	textExts := map[string]bool{
		".go": true, ".md": true, ".txt": true, ".py": true,
		".js": true, ".ts": true, ".tsx": true, ".jsx": true,
		".rs": true, ".rb": true, ".java": true, ".c": true,
		".h": true, ".cpp": true, ".yaml": true, ".yml": true,
		".toml": true, ".json": true, ".sh": true, ".bash": true,
	}
	return textExts[ext]
}
