package cmd

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"runtime"
	"strings"
	"time"

	"github.com/GrayCodeAI/hawk/internal/config"
	"github.com/GrayCodeAI/hawk/internal/permissions"
	"github.com/GrayCodeAI/hawk/internal/swift"
	"github.com/spf13/cobra"
)

// swiftReportOut forces writing to an explicit path instead of the temp dir.
var swiftReportOut string

// swiftReportLog optionally appends a tail of a log file to the report.
var swiftReportLog string

// swiftReportNoCopy disables the clipboard-copy attempt on macOS.
var swiftReportNoCopy bool

var swiftReportCmd = &cobra.Command{
	Use:   "swift-report",
	Short: "Write a private diagnostic swift report (fx /swift parity)",
	Long: `Snapshot current session context, permissions, and recent activity into a
single private, redactable markdown document — ported from fx's /swift
slash command.

The report is written to a private (0600) file in the temp directory with a
uniquely named file, then (on macOS) an attempt is made to copy it to the
clipboard. Obvious secrets are masked in the output; review and redact before
sharing.`,
	RunE: runSwiftReport,
}

func init() {
	swiftReportCmd.Flags().StringVar(&swiftReportOut, "out", "", "write report to this exact path instead of the temp dir")
	swiftReportCmd.Flags().StringVar(&swiftReportLog, "log", "", "append the tail of this log file to the report")
	swiftReportCmd.Flags().BoolVar(&swiftReportNoCopy, "no-copy", false, "do not attempt a clipboard copy")
	rootCmd.AddCommand(swiftReportCmd)
}

func runSwiftReport(cmd *cobra.Command, _ []string) error {
	cwd, err := os.Getwd()
	if err != nil {
		return fmt.Errorf("get working directory: %w", err)
	}

	s := swift.Snapshot{
		Timestamp: time.Now(),
		Version:   DisplayVersion(),
		Build:     buildDateOrDev(),
		Platform:  runtime.GOOS + "/" + runtime.GOARCH,
		Model:     config.ActiveModel(context.Background()),
		Workspace: cwd,
		SessionID: os.Getenv("HAWK_SESSION_ID"),
		PID:       os.Getpid(),
		Terminal:  terminalSize(),
		Env:       selectedEnv(),
	}

	s.StableRules = stableRules(cwd)

	if swiftReportLog != "" {
		s.LogTail = readLogTail(swiftReportLog)
	}

	var path string
	if swiftReportOut != "" {
		if err := swift.WriteReportToPath(swiftReportOut, &s); err != nil {
			return err
		}
		path = swiftReportOut
	} else {
		path, err = swift.WriteReportFile(&s)
		if err != nil {
			return err
		}
	}

	// Mirror fx: attempt clipboard copy; on failure print a review-and-redact
	// notice pointing at the saved path.
	if !swiftReportNoCopy && swift.TryClipboard(swift.Build(&s)) {
		cmd.Println("Swift report copied to clipboard. Saved at " + path + " (review and redact before sharing).")
	} else {
		cmd.Println("Swift saved at " + path + ". Review and redact it before sharing.")
	}
	return nil
}

// stableRules loads the project's persisted exact permission rules.
func stableRules(projectDir string) []swift.StableRule {
	store := permissions.NewStableRuleStore(permissions.DefaultStableRulesPath(projectDir))
	if err := store.Load(); err != nil {
		return nil
	}
	rules := store.List()
	if len(rules) == 0 {
		return nil
	}
	out := make([]swift.StableRule, 0, len(rules))
	for _, r := range rules {
		out = append(out, swift.StableRule{
			ID:       r.ID,
			Kind:     r.Key.Kind.String(),
			Identity: r.Key.Canonical,
			Decision: r.Decision.String(),
		})
	}
	return out
}

func buildDateOrDev() string {
	d := strings.TrimSpace(buildDate)
	if d == "" || d == "unknown" {
		return "dev"
	}
	return d
}

func terminalSize() string {
	cols := os.Getenv("COLUMNS")
	rows := os.Getenv("LINES")
	if cols == "" || rows == "" {
		return ""
	}
	return cols + "x" + rows
}

// selectedEnv returns a small set of non-sensitive environment variables that
// are useful for diagnosis. Secrets are never selected.
func selectedEnv() []string {
	var out []string
	for _, kv := range os.Environ() {
		key := kv
		if i := strings.IndexByte(kv, '='); i >= 0 {
			key = kv[:i]
		}
		if isEnvSecretKey(key) {
			continue
		}
		out = append(out, kv)
		if len(out) >= 40 {
			break
		}
	}
	return out
}

// isEnvSecretKey reports whether an env var name is likely to carry a secret.
func isEnvSecretKey(key string) bool {
	lower := strings.ToLower(key)
	for _, needle := range []string{"token", "secret", "password", "passwd", "api_key", "apikey", "key", "auth", "credential", "pem"} {
		if strings.Contains(lower, needle) {
			return true
		}
	}
	return false
}

// readLogTail reads up to swift.MaxLogTailBytes trailing bytes and up to
// swift.MaxLogTailLines non-empty lines from a log file.
func readLogTail(path string) []swift.LogEntry {
	info, err := os.Stat(path)
	if err != nil {
		return nil
	}
	total := info.Size()
	if total <= 0 {
		return nil
	}
	readSize := int64(swift.MaxLogTailBytes)
	offset := total - readSize
	if offset < 0 {
		offset = 0
		readSize = total
	}
	data := make([]byte, readSize)
	f, err := os.Open(path)
	if err != nil {
		return nil
	}
	n, rerr := f.ReadAt(data, offset)
	_ = f.Close()
	if rerr != nil && !errors.Is(rerr, io.EOF) {
		return nil
	}
	text := string(data[:n])
	var lines []string
	for _, line := range strings.Split(text, "\n") {
		line = strings.TrimRight(line, " \t\r")
		if line == "" {
			continue
		}
		lines = append(lines, line)
	}
	if len(lines) > swift.MaxLogTailLines {
		lines = lines[len(lines)-swift.MaxLogTailLines:]
	}
	out := make([]swift.LogEntry, 0, len(lines))
	for _, l := range lines {
		out = append(out, swift.LogEntry{Line: l, Sensitive: true})
	}
	return out
}
