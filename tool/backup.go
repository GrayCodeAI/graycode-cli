package tool

import (
	"fmt"
	"os"
	"path/filepath"
	"time"
)

// BackupFile creates a backup of a file before modification.
// Returns the backup path, or empty string if backup wasn't needed.
func BackupFile(path string) (string, error) {
	info, err := os.Stat(path)
	if err != nil {
		return "", nil // file doesn't exist, nothing to backup
	}
	if info.IsDir() {
		return "", nil
	}
	if info.Size() > 10*1024*1024 {
		return "", nil // don't backup files >10MB
	}

	data, err := os.ReadFile(path)
	if err != nil {
		return "", nil
	}

	backupDir := backupDirFor(path)
	if err := os.MkdirAll(backupDir, 0o755); err != nil {
		return "", err
	}

	ts := time.Now().Format("20060102-150405")
	backupName := filepath.Base(path) + "." + ts + ".bak"
	backupPath := filepath.Join(backupDir, backupName)

	if err := os.WriteFile(backupPath, data, info.Mode()); err != nil {
		return "", err
	}

	// Keep only last 5 backups per file
	cleanOldBackups(backupDir, filepath.Base(path), 5)

	return backupPath, nil
}

// RestoreFromBackup restores the most recent backup of a file.
func RestoreFromBackup(path string) error {
	backupDir := backupDirFor(path)
	entries, err := os.ReadDir(backupDir)
	if err != nil {
		return fmt.Errorf("no backups found for %s", path)
	}

	baseName := filepath.Base(path)
	var latest string
	var latestTime time.Time

	for _, e := range entries {
		name := e.Name()
		if len(name) > len(baseName)+1 && name[:len(baseName)] == baseName {
			info, err := e.Info()
			if err != nil {
				continue
			}
			if info.ModTime().After(latestTime) {
				latestTime = info.ModTime()
				latest = filepath.Join(backupDir, name)
			}
		}
	}

	if latest == "" {
		return fmt.Errorf("no backups found for %s", path)
	}

	data, err := os.ReadFile(latest)
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0o644)
}

// ListBackups returns all backups for a file.
func ListBackups(path string) []string {
	backupDir := backupDirFor(path)
	entries, err := os.ReadDir(backupDir)
	if err != nil {
		return nil
	}

	baseName := filepath.Base(path)
	var backups []string
	for _, e := range entries {
		if len(e.Name()) > len(baseName) && e.Name()[:len(baseName)] == baseName {
			backups = append(backups, filepath.Join(backupDir, e.Name()))
		}
	}
	return backups
}

func backupDirFor(path string) string {
	home, _ := os.UserHomeDir()
	absPath, err := filepath.Abs(path)
	if err != nil {
		absPath = path
	}
	// Hash the directory to create a unique backup subdir
	dir := filepath.Dir(absPath)
	hash := simpleHash(dir)
	return filepath.Join(home, ".hawk", "backups", hash)
}

func simpleHash(s string) string {
	h := uint32(0)
	for _, c := range s {
		h = h*31 + uint32(c)
	}
	return fmt.Sprintf("%08x", h)
}

func cleanOldBackups(dir, baseName string, keep int) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return
	}

	var matching []os.DirEntry
	for _, e := range entries {
		if len(e.Name()) > len(baseName) && e.Name()[:len(baseName)] == baseName {
			matching = append(matching, e)
		}
	}

	if len(matching) <= keep {
		return
	}

	// Remove oldest (keep most recent N)
	type fileTime struct {
		name string
		mod  time.Time
	}
	var files []fileTime
	for _, e := range matching {
		info, err := e.Info()
		if err != nil {
			continue
		}
		files = append(files, fileTime{e.Name(), info.ModTime()})
	}

	// Sort by time (oldest first)
	for i := 0; i < len(files)-1; i++ {
		for j := i + 1; j < len(files); j++ {
			if files[j].mod.Before(files[i].mod) {
				files[i], files[j] = files[j], files[i]
			}
		}
	}

	// Remove all but the last N
	for i := 0; i < len(files)-keep; i++ {
		os.Remove(filepath.Join(dir, files[i].name))
	}
}
