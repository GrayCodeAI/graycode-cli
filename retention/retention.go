package retention

import (
	"os"
	"path/filepath"
	"time"
)

type Policy struct {
	MaxAge    time.Duration `json:"max_age"`
	MaxSizeMB int64         `json:"max_size_mb"`
}

func DefaultPolicy() Policy {
	return Policy{
		MaxAge:    30 * 24 * time.Hour,
		MaxSizeMB: 500,
	}
}

type CleanupResult struct {
	FilesRemoved int
	BytesFreed   int64
	Errors       []error
}

// CleanDirectory removes files older than policy.MaxAge from dir.
func CleanDirectory(dir string, policy Policy) CleanupResult {
	result := CleanupResult{}
	cutoff := time.Now().Add(-policy.MaxAge)

	entries, err := os.ReadDir(dir)
	if err != nil {
		result.Errors = append(result.Errors, err)
		return result
	}

	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		info, err := entry.Info()
		if err != nil {
			result.Errors = append(result.Errors, err)
			continue
		}
		if info.ModTime().Before(cutoff) {
			path := filepath.Join(dir, entry.Name())
			if err := os.Remove(path); err != nil {
				result.Errors = append(result.Errors, err)
			} else {
				result.FilesRemoved++
				result.BytesFreed += info.Size()
			}
		}
	}
	return result
}

// EnforceSize removes oldest files until total size is under policy.MaxSizeMB.
func EnforceSize(dir string, policy Policy) CleanupResult {
	result := CleanupResult{}
	if policy.MaxSizeMB <= 0 {
		return result
	}

	maxBytes := policy.MaxSizeMB * 1024 * 1024

	entries, err := os.ReadDir(dir)
	if err != nil {
		result.Errors = append(result.Errors, err)
		return result
	}

	type fileEntry struct {
		name    string
		size    int64
		modTime time.Time
	}
	var files []fileEntry
	var totalSize int64

	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		info, err := entry.Info()
		if err != nil {
			continue
		}
		files = append(files, fileEntry{name: entry.Name(), size: info.Size(), modTime: info.ModTime()})
		totalSize += info.Size()
	}

	if totalSize <= maxBytes {
		return result
	}

	// Sort oldest first
	for i := 0; i < len(files)-1; i++ {
		for j := i + 1; j < len(files); j++ {
			if files[j].modTime.Before(files[i].modTime) {
				files[i], files[j] = files[j], files[i]
			}
		}
	}

	for _, f := range files {
		if totalSize <= maxBytes {
			break
		}
		path := filepath.Join(dir, f.name)
		if err := os.Remove(path); err != nil {
			result.Errors = append(result.Errors, err)
			continue
		}
		result.FilesRemoved++
		result.BytesFreed += f.size
		totalSize -= f.size
	}
	return result
}
