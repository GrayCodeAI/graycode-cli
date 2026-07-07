package cmd

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/GrayCodeAI/hawk/internal/storage"
	"github.com/spf13/cobra"
)

// ─────────────────────────────────────────────────────────────────────────────
// Background Sessions — run hawk sessions in the background and manage them.
// ─────────────────────────────────────────────────────────────────────────────

func bgSessionsDir() string {
	return filepath.Join(storage.StateDir(), "bg-sessions")
}

// BGSessionInfo tracks a running background session.
type BGSessionInfo struct {
	ID        string    `json:"id"`
	Prompt    string    `json:"prompt"`
	PID       int       `json:"pid"`
	StartedAt time.Time `json:"started_at"`
	CWD       string    `json:"cwd"`
	LogFile   string    `json:"log_file"`
	Status    string    `json:"status"` // running, completed, failed
}

// SaveBGSession persists a background session info.
func SaveBGSession(info *BGSessionInfo) error {
	dir := bgSessionsDir()
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	data, err := json.MarshalIndent(info, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(dir, info.ID+".json"), data, 0o644)
}

// LoadBGSession reads a background session info.
func LoadBGSession(id string) (*BGSessionInfo, error) {
	data, err := os.ReadFile(filepath.Join(bgSessionsDir(), id+".json"))  // #nosec G304 -- path built from internal bg-sessions directory and session id
	if err != nil {
		return nil, err
	}
	var info BGSessionInfo
	if err := json.Unmarshal(data, &info); err != nil {
		return nil, err
	}
	return &info, nil
}

// ListBGSessions returns all background sessions.
func ListBGSessions() ([]*BGSessionInfo, error) {
	dir := bgSessionsDir()
	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}

	var sessions []*BGSessionInfo
	for _, e := range entries {
		if e.IsDir() || filepath.Ext(e.Name()) != ".json" {
			continue
		}
		id := e.Name()[:len(e.Name())-5]
		info, err := LoadBGSession(id)
		if err != nil {
			continue
		}
		// Check if process is still running
		info.Status = checkProcessStatus(info.PID)
		sessions = append(sessions, info)
	}
	return sessions, nil
}

func checkProcessStatus(pid int) string {
	// Check if process is running
	proc, err := os.FindProcess(pid)
	if err != nil {
		return "failed"
	}
	// On Unix, FindProcess always succeeds; check with signal 0
	err = proc.Signal(nil)
	if err != nil {
		return "completed"
	}
	return "running"
}

// KillBGSession terminates a background session.
func KillBGSession(id string) error {
	info, err := LoadBGSession(id)
	if err != nil {
		return fmt.Errorf("session %s not found", id)
	}
	proc, err := os.FindProcess(info.PID)
	if err != nil {
		return fmt.Errorf("process not found: %w", err)
	}
	if err := proc.Kill(); err != nil {
		return fmt.Errorf("failed to kill process: %w", err)
	}
	info.Status = "killed"
	return SaveBGSession(info)
}

// StartBGSession launches hawk in background mode.
func StartBGSession(prompt string, args []string) (*BGSessionInfo, error) {
	id := genID()
	cwd, _ := os.Getwd()
	logFile := filepath.Join(bgSessionsDir(), id+".log")

	// Build command: hawk --print <prompt> with all inherited flags
	cmdArgs := append([]string{"--print", "--session-id", id, prompt}, args...)
	cmd := exec.CommandContext(context.Background(), "hawk", cmdArgs...)  // #nosec G204 -- fixed command 'hawk' relaunching self with internal flags
	cmd.Dir = cwd

	logF, err := os.Create(logFile)  // #nosec G304 -- logFile built from internal bg-sessions directory and generated id
	if err != nil {
		return nil, fmt.Errorf("create log file: %w", err)
	}
	cmd.Stdout = logF
	cmd.Stderr = logF

	if err := cmd.Start(); err != nil {
		_ = logF.Close()
		return nil, fmt.Errorf("start background session: %w", err)
	}

	info := &BGSessionInfo{
		ID:        id,
		Prompt:    prompt,
		PID:       cmd.Process.Pid,
		StartedAt: time.Now(),
		CWD:       cwd,
		LogFile:   logFile,
		Status:    "running",
	}

	if err := SaveBGSession(info); err != nil {
		return nil, fmt.Errorf("save session info: %w", err)
	}

	return info, nil
}

// FormatBGSessions formats background sessions for display.
func FormatBGSessions(sessions []*BGSessionInfo) string {
	if len(sessions) == 0 {
		return "No background sessions."
	}

	var b strings.Builder
	b.WriteString(fmt.Sprintf("Background sessions (%d):\n", len(sessions)))
	b.WriteString(strings.Repeat("─", 60) + "\n")

	for _, s := range sessions {
		shortID := s.ID
		if len(shortID) > 8 {
			shortID = shortID[:8]
		}
		preview := s.Prompt
		if len(preview) > 50 {
			preview = preview[:50] + "..."
		}
		age := time.Since(s.StartedAt).Round(time.Minute)
		b.WriteString(fmt.Sprintf("  [%s] %s — %s\n", shortID, s.Status, preview))
		b.WriteString(fmt.Sprintf("    PID: %d · started %s ago · %s\n\n", s.PID, age, s.CWD))
	}

	return b.String()
}

var bgCmd = &cobra.Command{
	Use:   "bg [prompt]",
	Short: "Run a session in the background",
	Long: `Start hawk in the background and continue working in your terminal.

Examples:
  hawk bg "Refactor the auth module"
  hawk bg "Run tests and fix failures"`,
	RunE: func(cmd *cobra.Command, args []string) error {
		if len(args) == 0 {
			return fmt.Errorf("prompt required")
		}
		prompt := strings.Join(args, " ")

		info, err := StartBGSession(prompt, collectDaemonArgs())
		if err != nil {
			return err
		}

		cmd.Printf("Background session started: %s (PID %d)\n", info.ID, info.PID)
		cmd.Printf("View logs: tail -f %s\n", info.LogFile)
		cmd.Printf("Attach: hawk attach %s\n", info.ID[:8])
		return nil
	},
}

var attachCmd = &cobra.Command{
	Use:   "attach <session-id>",
	Short: "Attach to a running background session",
	RunE: func(cmd *cobra.Command, args []string) error {
		if len(args) == 0 {
			return fmt.Errorf("session ID required")
		}
		id := args[0]

		// Try loading full ID from partial
		sessions, err := ListBGSessions()
		if err != nil {
			return err
		}

		var target *BGSessionInfo
		for _, s := range sessions {
			if s.ID == id || (len(id) <= 8 && len(s.ID) > 8 && s.ID[:len(id)] == id) {
				target = s
				break
			}
		}

		if target == nil {
			return fmt.Errorf("session %s not found", id)
		}

		if target.Status != "running" {
			cmd.Printf("Session %s is %s\n", target.ID, target.Status)
			cmd.Println("Recent log output:")
			return tailLog(cmd, target.LogFile, 20)
		}

		cmd.Printf("Attaching to session %s (PID %d)\n", target.ID, target.PID)
		cmd.Println("Recent output:")
		return tailLog(cmd, target.LogFile, 30)
	},
}

var sessionsLsCmd = &cobra.Command{
	Use:   "ls",
	Short: "List background sessions",
	RunE: func(cmd *cobra.Command, args []string) error {
		sessions, err := ListBGSessions()
		if err != nil {
			return err
		}
		cmd.Println(FormatBGSessions(sessions))
		return nil
	},
}

var sessionsKillCmd = &cobra.Command{
	Use:   "kill <session-id>",
	Short: "Kill a running background session",
	RunE: func(cmd *cobra.Command, args []string) error {
		if len(args) == 0 {
			return fmt.Errorf("session ID required")
		}
		if err := KillBGSession(args[0]); err != nil {
			return err
		}
		cmd.Println("Session killed:", args[0])
		return nil
	},
}

func init() {
	sessionsCmd.AddCommand(sessionsLsCmd, sessionsKillCmd)
	rootCmd.AddCommand(bgCmd)
	rootCmd.AddCommand(attachCmd)
}

func tailLog(cmd *cobra.Command, path string, lines int) error {
	data, err := os.ReadFile(path)  // #nosec G304 -- path built from internal bg-sessions directory
	if err != nil {
		return fmt.Errorf("read log: %w", err)
	}
	allLines := strings.Split(string(data), "\n")
	start := 0
	if len(allLines) > lines {
		start = len(allLines) - lines
	}
	for _, line := range allLines[start:] {
		if line != "" {
			cmd.Println(line)
		}
	}
	return nil
}

func collectDaemonArgs() []string {
	var args []string
	if model != "" {
		args = append(args, "--model", model)
	}
	if provider != "" {
		args = append(args, "--provider", provider)
	}
	if sandboxFlag != "" {
		args = append(args, "--sandbox", sandboxFlag)
	}
	if dangerouslySkipPermissions {
		args = append(args, "--dangerously-skip-permissions")
	}
	return args
}
