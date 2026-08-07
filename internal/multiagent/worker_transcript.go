package mission

import (
	"bufio"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/GrayCodeAI/hawk/internal/types"
)

// TranscriptPath returns the path to a feature's worker transcript file.
func TranscriptPath(missionDir, featureID string) string {
	return filepath.Join(missionDir, "workers", sanitize(featureID)+".jsonl")
}

// IsTranscriptComplete checks if a transcript file exists and has a completion
// marker. Returns (exists, complete, err).
func IsTranscriptComplete(path string) (bool, bool, error) {
	f, err := os.Open(path)
	if err != nil {
		if os.IsNotExist(err) {
			return false, false, nil
		}
		return false, false, err
	}
	defer func() { _ = f.Close() }()

	scanner := bufio.NewScanner(f)
	// 1MB max line size for long tool outputs.
	buf := make([]byte, 1024*1024)
	scanner.Buffer(buf, 1024*1024)

	var found bool
	for scanner.Scan() {
		found = true
		var probe struct {
			Role    string   `json:"role"`
			Handoff *Handoff `json:"handoff,omitempty"`
		}
		if err := json.Unmarshal(scanner.Bytes(), &probe); err != nil {
			continue
		}
		if probe.Role == "__complete" && probe.Handoff != nil {
			return true, true, nil
		}
	}
	if err := scanner.Err(); err != nil {
		return found, false, err
	}
	return found, false, nil
}

// LoadTranscript reads a transcript file back into EyrieMessages. The final
// __complete record (if any) is returned separately as handoff+true.
func LoadTranscript(path string) ([]types.EyrieMessage, *Handoff, bool, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, nil, false, err
	}
	defer func() { _ = f.Close() }()

	var messages []types.EyrieMessage
	var handoff *Handoff
	var complete bool

	scanner := bufio.NewScanner(f)
	buf := make([]byte, 1024*1024)
	scanner.Buffer(buf, 1024*1024)

	for scanner.Scan() {
		line := scanner.Bytes()

		// Check for completion marker.
		var probe struct {
			Role    string   `json:"role"`
			Handoff *Handoff `json:"handoff,omitempty"`
		}
		if err := json.Unmarshal(line, &probe); err == nil && probe.Role == "__complete" {
			handoff = probe.Handoff
			complete = true
			continue
		}

		var msg types.EyrieMessage
		if err := json.Unmarshal(line, &msg); err != nil {
			// Skip malformed lines rather than failing the whole load.
			continue
		}
		messages = append(messages, msg)
	}
	if err := scanner.Err(); err != nil {
		return messages, handoff, complete, err
	}
	return messages, handoff, complete, nil
}

// PersistWriter is an append-only JSONL writer for worker transcripts.
type PersistWriter struct {
	mu   sync.Mutex
	file *os.File
	path string
}

// NewPersistWriter creates (or appends to) a transcript file. The parent
// directory is created if needed.
func NewPersistWriter(path string) (*PersistWriter, error) {
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return nil, fmt.Errorf("create transcript dir: %w", err)
	}
	f, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o600)
	if err != nil {
		return nil, fmt.Errorf("open transcript: %w", err)
	}
	return &PersistWriter{file: f, path: path}, nil
}

// Path returns the file path.
func (w *PersistWriter) Path() string {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.path
}

// Write appends one message to the transcript.
func (w *PersistWriter) Write(msg types.EyrieMessage) error {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.writeLocked(msg)
}

func (w *PersistWriter) writeLocked(msg types.EyrieMessage) error {
	data, err := json.Marshal(msg)
	if err != nil {
		return fmt.Errorf("marshal transcript message: %w", err)
	}
	if _, err := w.file.Write(data); err != nil {
		return fmt.Errorf("write transcript: %w", err)
	}
	if _, err := w.file.WriteString("\n"); err != nil {
		return fmt.Errorf("write transcript newline: %w", err)
	}
	return nil
}

// MarkComplete appends the completion marker with the handoff result.
func (w *PersistWriter) MarkComplete(handoff *Handoff) error {
	w.mu.Lock()
	defer w.mu.Unlock()

	rec := struct {
		Role    string    `json:"role"`
		Handoff *Handoff  `json:"handoff,omitempty"`
		At      time.Time `json:"at"`
	}{
		Role:    "__complete",
		Handoff: handoff,
		At:      time.Now(),
	}
	data, err := json.Marshal(rec)
	if err != nil {
		return fmt.Errorf("marshal completion marker: %w", err)
	}
	if _, err := w.file.Write(data); err != nil {
		return fmt.Errorf("write completion marker: %w", err)
	}
	if _, err := w.file.WriteString("\n"); err != nil {
		return fmt.Errorf("write completion newline: %w", err)
	}
	return nil
}

// Close closes the underlying file.
func (w *PersistWriter) Close() error {
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.file != nil {
		err := w.file.Close()
		w.file = nil
		return err
	}
	return nil
}

// sanitize makes a feature ID safe for use in a filename.
func sanitize(id string) string {
	// Replace path-unsafe characters while keeping the ID readable.
	var b []byte
	for i := 0; i < len(id); i++ {
		c := id[i]
		switch {
		case c >= 'a' && c <= 'z', c >= 'A' && c <= 'Z', c >= '0' && c <= '9', c == '-', c == '_':
			b = append(b, c)
		default:
			b = append(b, '_')
		}
	}
	if len(b) == 0 {
		return "feature"
	}
	return string(b)
}

// ErrTranscriptIncomplete is returned when a transcript exists but has no
// completion marker (worker was interrupted).
var ErrTranscriptIncomplete = errors.New("worker transcript is incomplete")
