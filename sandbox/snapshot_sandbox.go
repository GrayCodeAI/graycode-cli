package sandbox

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"
)

// SandboxState represents the full state of an execution sandbox.
type SandboxState struct {
	ID           string            `json:"id"`
	WorkDir      string            `json:"work_dir"`
	EnvVars      map[string]string `json:"env_vars"`
	Files        map[string][]byte `json:"files"`
	ProcessState string            `json:"process_state"`
	CreatedAt    time.Time         `json:"created_at"`
	PausedAt     *time.Time        `json:"paused_at,omitempty"`
	ResumedAt    *time.Time        `json:"resumed_at,omitempty"`
	Status       string            `json:"status"` // "running", "paused", "terminated"

	// initialFiles holds the file state at creation for diffing.
	initialFiles map[string][]byte
}

// SandboxManager manages sandbox lifecycle including pause/resume and snapshots.
type SandboxManager struct {
	Sandboxes    map[string]*SandboxState
	Dir          string
	MaxSandboxes int
	mu           sync.RWMutex
}

// NewSandboxManager creates a new SandboxManager that persists state to dir.
func NewSandboxManager(dir string) *SandboxManager {
	return &SandboxManager{
		Sandboxes:    make(map[string]*SandboxState),
		Dir:          dir,
		MaxSandboxes: 64,
	}
}

// Create creates a new sandbox with the given working directory and environment variables.
// It captures the initial file state of workDir.
func (m *SandboxManager) Create(workDir string, envVars map[string]string) (*SandboxState, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	if len(m.Sandboxes) >= m.MaxSandboxes {
		return nil, fmt.Errorf("maximum sandbox limit (%d) reached", m.MaxSandboxes)
	}

	id, err := generateID()
	if err != nil {
		return nil, fmt.Errorf("generate id: %w", err)
	}

	files, err := captureFiles(workDir)
	if err != nil {
		return nil, fmt.Errorf("capture files: %w", err)
	}

	if envVars == nil {
		envVars = make(map[string]string)
	}

	state := &SandboxState{
		ID:           id,
		WorkDir:      workDir,
		EnvVars:      envVars,
		Files:        files,
		ProcessState: "idle",
		CreatedAt:    time.Now(),
		Status:       "running",
		initialFiles: copyFileMap(files),
	}

	m.Sandboxes[id] = state
	return state, nil
}

// Pause captures the current file state, persists the sandbox to disk, and marks it as paused.
func (m *SandboxManager) Pause(id string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	sb, ok := m.Sandboxes[id]
	if !ok {
		return fmt.Errorf("sandbox %q not found", id)
	}
	if sb.Status != "running" {
		return fmt.Errorf("sandbox %q is not running (status: %s)", id, sb.Status)
	}

	// Capture current file state from the working directory.
	files, err := captureFiles(sb.WorkDir)
	if err != nil {
		return fmt.Errorf("capture files: %w", err)
	}
	sb.Files = files

	now := time.Now()
	sb.PausedAt = &now
	sb.Status = "paused"
	sb.ProcessState = "suspended"

	// Persist to disk.
	if err := m.saveToDisk(sb); err != nil {
		return fmt.Errorf("save to disk: %w", err)
	}

	return nil
}

// Resume loads a paused sandbox from disk, restores its file state, and marks it as running.
func (m *SandboxManager) Resume(id string) (*SandboxState, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	sb, ok := m.Sandboxes[id]
	if !ok {
		// Try loading from disk.
		loaded, err := m.loadFromDisk(id)
		if err != nil {
			return nil, fmt.Errorf("sandbox %q not found", id)
		}
		sb = loaded
		m.Sandboxes[id] = sb
	}

	if sb.Status != "paused" {
		return nil, fmt.Errorf("sandbox %q is not paused (status: %s)", id, sb.Status)
	}

	// Restore file state to the working directory.
	if err := restoreFiles(sb.WorkDir, sb.Files); err != nil {
		return nil, fmt.Errorf("restore files: %w", err)
	}

	now := time.Now()
	sb.ResumedAt = &now
	sb.Status = "running"
	sb.ProcessState = "idle"

	return sb, nil
}

// Snapshot serializes the full state of a sandbox to JSON bytes.
func (m *SandboxManager) Snapshot(id string) ([]byte, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	sb, ok := m.Sandboxes[id]
	if !ok {
		return nil, fmt.Errorf("sandbox %q not found", id)
	}

	// Re-capture current files if running.
	if sb.Status == "running" {
		files, err := captureFiles(sb.WorkDir)
		if err != nil {
			return nil, fmt.Errorf("capture files: %w", err)
		}
		sb.Files = files
	}

	data, err := json.Marshal(sb)
	if err != nil {
		return nil, fmt.Errorf("marshal: %w", err)
	}
	return data, nil
}

// Restore deserializes snapshot data and creates a new sandbox from it.
func (m *SandboxManager) Restore(data []byte) (*SandboxState, error) {
	var sb SandboxState
	if err := json.Unmarshal(data, &sb); err != nil {
		return nil, fmt.Errorf("unmarshal: %w", err)
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	if len(m.Sandboxes) >= m.MaxSandboxes {
		return nil, fmt.Errorf("maximum sandbox limit (%d) reached", m.MaxSandboxes)
	}

	// Restore file state.
	if sb.WorkDir != "" && len(sb.Files) > 0 {
		if err := restoreFiles(sb.WorkDir, sb.Files); err != nil {
			return nil, fmt.Errorf("restore files: %w", err)
		}
	}

	sb.initialFiles = copyFileMap(sb.Files)
	now := time.Now()
	sb.ResumedAt = &now
	sb.Status = "running"

	m.Sandboxes[sb.ID] = &sb
	return &sb, nil
}

// List returns all sandboxes sorted by creation time (newest first).
func (m *SandboxManager) List() []*SandboxState {
	m.mu.RLock()
	defer m.mu.RUnlock()

	result := make([]*SandboxState, 0, len(m.Sandboxes))
	for _, sb := range m.Sandboxes {
		result = append(result, sb)
	}

	sort.Slice(result, func(i, j int) bool {
		return result[i].CreatedAt.After(result[j].CreatedAt)
	})
	return result
}

// Cleanup removes sandboxes older than maxAge and their persisted state.
func (m *SandboxManager) Cleanup(maxAge time.Duration) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	cutoff := time.Now().Add(-maxAge)
	var errs []string

	for id, sb := range m.Sandboxes {
		if sb.CreatedAt.Before(cutoff) {
			// Remove persisted file.
			path := filepath.Join(m.Dir, id+".json")
			if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
				errs = append(errs, fmt.Sprintf("remove %s: %v", path, err))
			}
			delete(m.Sandboxes, id)
		}
	}

	if len(errs) > 0 {
		return fmt.Errorf("cleanup errors: %s", strings.Join(errs, "; "))
	}
	return nil
}

// DiffSandbox compares the current file state of a sandbox's working directory
// against its initial state and returns a list of changes.
func (m *SandboxManager) DiffSandbox(id string) ([]string, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	sb, ok := m.Sandboxes[id]
	if !ok {
		return nil, fmt.Errorf("sandbox %q not found", id)
	}

	currentFiles, err := captureFiles(sb.WorkDir)
	if err != nil {
		return nil, fmt.Errorf("capture files: %w", err)
	}

	var diffs []string

	// Check for modified and deleted files.
	for path, initialContent := range sb.initialFiles {
		currentContent, exists := currentFiles[path]
		if !exists {
			diffs = append(diffs, fmt.Sprintf("deleted: %s", path))
		} else if string(currentContent) != string(initialContent) {
			diffs = append(diffs, fmt.Sprintf("modified: %s", path))
		}
	}

	// Check for new files.
	for path := range currentFiles {
		if _, exists := sb.initialFiles[path]; !exists {
			diffs = append(diffs, fmt.Sprintf("added: %s", path))
		}
	}

	sort.Strings(diffs)
	return diffs, nil
}

// FormatStatus returns a human-readable summary of all sandboxes.
func (m *SandboxManager) FormatStatus() string {
	sandboxes := m.List()
	if len(sandboxes) == 0 {
		return "Sandboxes (0):\n─────────────────\nNo sandboxes active."
	}

	var b strings.Builder
	fmt.Fprintf(&b, "Sandboxes (%d):\n", len(sandboxes))
	b.WriteString("─────────────────\n")

	for i, sb := range sandboxes {
		switch sb.Status {
		case "running":
			ago := time.Since(sb.CreatedAt).Truncate(time.Second)
			fmt.Fprintf(&b, "%d. [running] %s (%s)\n", i+1, sb.ID, sb.WorkDir)
			fmt.Fprintf(&b, "   Created: %s ago, Files: %d\n", formatDuration(ago), len(sb.Files))
		case "paused":
			var pausedAgo string
			if sb.PausedAt != nil {
				pausedAgo = formatDuration(time.Since(*sb.PausedAt).Truncate(time.Second))
			} else {
				pausedAgo = "unknown"
			}
			fmt.Fprintf(&b, "%d. [paused] %s (%s)\n", i+1, sb.ID, sb.WorkDir)
			fmt.Fprintf(&b, "   Paused: %s ago, Resumable\n", pausedAgo)
		case "terminated":
			fmt.Fprintf(&b, "%d. [terminated] %s\n", i+1, sb.ID)
			b.WriteString("   Cleanup eligible\n")
		}
	}

	return strings.TrimRight(b.String(), "\n")
}

// saveToDisk writes sandbox state to a JSON file on disk.
func (m *SandboxManager) saveToDisk(sb *SandboxState) error {
	if err := os.MkdirAll(m.Dir, 0o755); err != nil {
		return err
	}
	data, err := json.Marshal(sb)
	if err != nil {
		return err
	}
	path := filepath.Join(m.Dir, sb.ID+".json")
	return os.WriteFile(path, data, 0o644)
}

// loadFromDisk reads sandbox state from a JSON file on disk.
func (m *SandboxManager) loadFromDisk(id string) (*SandboxState, error) {
	path := filepath.Join(m.Dir, id+".json")
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var sb SandboxState
	if err := json.Unmarshal(data, &sb); err != nil {
		return nil, err
	}
	sb.initialFiles = copyFileMap(sb.Files)
	return &sb, nil
}

// captureFiles walks a directory and captures all regular files as a map.
func captureFiles(dir string) (map[string][]byte, error) {
	files := make(map[string][]byte)
	if dir == "" {
		return files, nil
	}

	info, err := os.Stat(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return files, nil
		}
		return nil, err
	}
	if !info.IsDir() {
		return nil, fmt.Errorf("%q is not a directory", dir)
	}

	err = filepath.Walk(dir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return nil // skip unreadable files
		}
		if info.IsDir() {
			// Skip hidden directories.
			if strings.HasPrefix(info.Name(), ".") && path != dir {
				return filepath.SkipDir
			}
			return nil
		}
		// Skip large files (>1MB) to avoid memory issues.
		if info.Size() > 1<<20 {
			return nil
		}
		rel, err := filepath.Rel(dir, path)
		if err != nil {
			return nil
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return nil
		}
		files[rel] = data
		return nil
	})
	return files, err
}

// restoreFiles writes a file map back to a directory, creating directories as needed.
func restoreFiles(dir string, files map[string][]byte) error {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	for rel, content := range files {
		path := filepath.Join(dir, rel)
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			return err
		}
		if err := os.WriteFile(path, content, 0o644); err != nil {
			return err
		}
	}
	return nil
}

// copyFileMap creates a deep copy of a file map.
func copyFileMap(m map[string][]byte) map[string][]byte {
	if m == nil {
		return nil
	}
	c := make(map[string][]byte, len(m))
	for k, v := range m {
		buf := make([]byte, len(v))
		copy(buf, v)
		c[k] = buf
	}
	return c
}

// generateID creates a random sandbox ID with the "sb-" prefix.
func generateID() (string, error) {
	b := make([]byte, 6)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return "sb-" + hex.EncodeToString(b), nil
}

// formatDuration formats a duration into a human-friendly string.
func formatDuration(d time.Duration) string {
	if d < time.Minute {
		return fmt.Sprintf("%ds", int(d.Seconds()))
	}
	if d < time.Hour {
		return fmt.Sprintf("%dm", int(d.Minutes()))
	}
	return fmt.Sprintf("%dh", int(d.Hours()))
}
