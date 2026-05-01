package session

import (
	"bufio"
	"compress/gzip"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"time"
)

// CompressOldSessions gzips session files older than maxAge.
// Returns the number of sessions compressed.
func CompressOldSessions(maxAge time.Duration) (int, error) {
	dir := sessionsDir()
	entries, err := os.ReadDir(dir)
	if err != nil {
		return 0, err
	}

	compressed := 0
	cutoff := time.Now().Add(-maxAge)

	for _, e := range entries {
		ext := filepath.Ext(e.Name())
		if ext != ".jsonl" && ext != ".json" {
			continue
		}

		info, err := e.Info()
		if err != nil {
			continue
		}
		if !info.ModTime().Before(cutoff) {
			continue
		}

		srcPath := filepath.Join(dir, e.Name())
		dstPath := srcPath + ".gz"

		// Skip if already compressed
		if _, err := os.Stat(dstPath); err == nil {
			continue
		}

		if err := compressFile(srcPath, dstPath); err != nil {
			continue
		}

		// Remove the original after successful compression
		os.Remove(srcPath)
		compressed++
	}
	return compressed, nil
}

// DecompressSession decompresses a .jsonl.gz file for loading.
func DecompressSession(id string) (*Session, error) {
	dir := sessionsDir()

	// Try .jsonl.gz first, then .json.gz
	var gzPath string
	for _, ext := range []string{".jsonl.gz", ".json.gz"} {
		candidate := filepath.Join(dir, id+ext)
		if _, err := os.Stat(candidate); err == nil {
			gzPath = candidate
			break
		}
	}
	if gzPath == "" {
		return nil, fmt.Errorf("no compressed session found for %s", id)
	}

	f, err := os.Open(gzPath)
	if err != nil {
		return nil, fmt.Errorf("open compressed session: %w", err)
	}
	defer f.Close()

	gz, err := gzip.NewReader(f)
	if err != nil {
		return nil, fmt.Errorf("gzip reader: %w", err)
	}
	defer gz.Close()

	// Determine format based on original extension
	isJSONL := filepath.Ext(gzPath[:len(gzPath)-3]) == ".jsonl"

	if isJSONL {
		return parseJSONLFromReader(id, gz)
	}
	return parseLegacyJSONFromReader(id, gz)
}

func compressFile(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()

	out, err := os.Create(dst)
	if err != nil {
		return err
	}
	defer out.Close()

	gw := gzip.NewWriter(out)
	if _, err := io.Copy(gw, in); err != nil {
		gw.Close()
		os.Remove(dst)
		return err
	}
	if err := gw.Close(); err != nil {
		os.Remove(dst)
		return err
	}
	return nil
}

func parseJSONLFromReader(id string, r io.Reader) (*Session, error) {
	var s Session
	s.ID = id
	scanner := bufio.NewScanner(r)
	scanner.Buffer(make([]byte, 1024*1024), 1024*1024)
	firstLine := true

	for scanner.Scan() {
		line := scanner.Bytes()
		if len(line) == 0 {
			continue
		}

		if firstLine {
			firstLine = false
			var meta map[string]interface{}
			if err := json.Unmarshal(line, &meta); err != nil {
				return nil, err
			}
			if v, ok := meta["model"].(string); ok {
				s.Model = v
			}
			if v, ok := meta["provider"].(string); ok {
				s.Provider = v
			}
			if v, ok := meta["cwd"].(string); ok {
				s.CWD = v
			}
			if v, ok := meta["name"].(string); ok {
				s.Name = v
			}
			if v, ok := meta["created_at"].(string); ok {
				s.CreatedAt, _ = time.Parse(time.RFC3339, v)
			}
			if v, ok := meta["updated_at"].(string); ok {
				s.UpdatedAt, _ = time.Parse(time.RFC3339, v)
			}
			continue
		}

		var msg Message
		if err := json.Unmarshal(line, &msg); err != nil {
			continue
		}
		s.Messages = append(s.Messages, msg)
	}

	if err := scanner.Err(); err != nil {
		return nil, err
	}
	return &s, nil
}

func parseLegacyJSONFromReader(id string, r io.Reader) (*Session, error) {
	data, err := io.ReadAll(r)
	if err != nil {
		return nil, err
	}
	var s Session
	if err := json.Unmarshal(data, &s); err != nil {
		return nil, err
	}
	s.ID = id
	return &s, nil
}
