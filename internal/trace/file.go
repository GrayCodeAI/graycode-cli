package trace

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

// defaultTMPDir mirrors fx's `getenv("TMPDIR") orelse "/tmp"`.
func defaultTMPDir() string {
	if d := os.Getenv("TMPDIR"); d != "" {
		return d
	}
	return "/tmp"
}

// fileNameSuffix returns `2026-08-21-120400-1a2b3c`.
func fileNameSuffix(ts time.Time, randomHex string) string {
	return ts.Format("2006-01-02-150405") + "-" + randomHex
}

// WriteReportToPath renders the snapshot and writes it to path with 0o600
// mode. The parent directory must already exist.
func WriteReportToPath(path string, s *Snapshot) error {
	if s == nil {
		s = &Snapshot{}
	}
	contents := Build(s)
	f, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0o600)
	if err != nil {
		return fmt.Errorf("create trace file: %w", err)
	}
	if _, err := f.WriteString(contents); err != nil {
		_ = f.Close()
		_ = os.Remove(path)
		return fmt.Errorf("write trace file: %w", err)
	}
	if err := f.Close(); err != nil {
		return fmt.Errorf("close trace file: %w", err)
	}
	return nil
}

// WriteReportFile renders the snapshot and writes it to a private file in the
// temp directory with an exclusive-create name, mirroring fx's
// writeTraceReportFile (0o600 mode, up to 8 collision attempts, random suffix).
// It returns the absolute path of the written file.
func WriteReportFile(s *Snapshot) (string, error) {
	if s == nil {
		s = &Snapshot{}
	}
	contents := Build(s)
	dir := strings.TrimRight(defaultTMPDir(), "/")
	now := time.Now()
	for attempt := 0; attempt < 8; attempt++ {
		hexBytes := randomHex(3) // 6 hex chars
		name := "hawk-trace-" + fileNameSuffix(now, hexBytes) + ".md"
		path := filepath.Join(dir, name)
		f, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
		if err != nil {
			if os.IsExist(err) {
				continue
			}
			return "", fmt.Errorf("create trace file: %w", err)
		}
		_, werr := f.WriteString(contents)
		cerr := f.Close()
		if werr != nil {
			_ = os.Remove(path)
			return "", fmt.Errorf("write trace file: %w", werr)
		}
		if cerr != nil {
			return "", fmt.Errorf("close trace file: %w", cerr)
		}
		return path, nil
	}
	return "", fmt.Errorf("could not allocate a unique trace file name")
}

// TryClipboard attempts to copy text to the system clipboard on macOS via
// pbcopy. It returns false when unsupported or the copy fails, matching fx's
// branch that reports "Clipboard copy failed."
func TryClipboard(text string) bool {
	if !isDarwin() {
		return false
	}
	cmd := exec.Command("pbcopy")
	cmd.Stdin = strings.NewReader(text)
	if err := cmd.Run(); err != nil {
		return false
	}
	return true
}

func isDarwin() bool {
	return os.Getenv("TRACE_FORCE_DARWIN") == "1"
}

func randomHex(nBytes int) string {
	b := make([]byte, nBytes)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}
