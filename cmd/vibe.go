package cmd

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/GrayCodeAI/hawk/engine"
)

// VibeConfig controls vibe coding behavior.
type VibeConfig struct {
	Enabled       bool
	AutoApply     bool   // apply file changes without asking
	AutoRun       bool   // run build/test after each change
	RunCommand    string // command to run (default: auto-detect)
	ShowDiffs     bool   // show diffs briefly (default: false in full vibe, true in semi-vibe)
	MaxIterations int    // max auto-fix iterations (default: 10)
}

// DefaultVibeConfig returns the default vibe coding configuration.
func DefaultVibeConfig() VibeConfig {
	return VibeConfig{
		Enabled:       true,
		AutoApply:     true,
		AutoRun:       true,
		ShowDiffs:     false,
		MaxIterations: 10,
	}
}

// DetectRunCommand auto-detects the project's test/build command by looking
// for well-known build files in the given directory.
func DetectRunCommand(dir string) string {
	checks := []struct {
		file    string
		command string
	}{
		{"go.mod", "go test ./..."},
		{"package.json", "npm test"},
		{"Cargo.toml", "cargo test"},
		{"pytest.ini", "pytest"},
		{"setup.py", "pytest"},
		{"pyproject.toml", "pytest"},
		{"Makefile", "make test"},
	}
	for _, c := range checks {
		if _, err := os.Stat(filepath.Join(dir, c.file)); err == nil {
			return c.command
		}
	}
	return ""
}

// VibeLoop runs the vibe coding loop: edit -> run -> check -> fix -> repeat.
//
// 1. Send prompt to LLM
// 2. Auto-apply all file changes (no permission prompt)
// 3. Run RunCommand
// 4. If passes: done, print success
// 5. If fails: send error output back to LLM, ask it to fix
// 6. Repeat until passes or MaxIterations reached
func VibeLoop(ctx context.Context, sess *engine.Session, prompt string, config VibeConfig) error {
	if config.MaxIterations <= 0 {
		config.MaxIterations = 10
	}

	// Auto-detect run command if not specified
	if config.RunCommand == "" {
		cwd, err := os.Getwd()
		if err != nil {
			return fmt.Errorf("vibe: get working directory: %w", err)
		}
		config.RunCommand = DetectRunCommand(cwd)
	}

	// Configure session for full autonomy
	sess.Mode = engine.PermissionModeBypassPermissions

	currentPrompt := prompt

	for i := 0; i < config.MaxIterations; i++ {
		// Step 1: Send prompt to LLM
		sess.AddUser(currentPrompt)
		ch, err := sess.Stream(ctx)
		if err != nil {
			return fmt.Errorf("vibe iteration %d: stream error: %w", i+1, err)
		}

		// Drain the stream and collect the assistant response
		var assistantText strings.Builder
		for ev := range ch {
			switch ev.Type {
			case "content":
				assistantText.WriteString(ev.Content)
			case "error":
				return fmt.Errorf("vibe iteration %d: LLM error: %s", i+1, ev.Content)
			}
		}

		// Step 2: Changes are auto-applied (session is in bypass mode)

		// Step 3: Run the test/build command if configured
		if !config.AutoRun || config.RunCommand == "" {
			fmt.Printf("[vibe] iteration %d complete (no run command configured)\n", i+1)
			return nil
		}

		output, runErr := runVibeCommand(ctx, config.RunCommand)

		// Step 4: If passes, we're done
		if runErr == nil {
			fmt.Printf("[vibe] iteration %d: all good\n", i+1)
			return nil
		}

		// Step 5: If fails, send error back to LLM for fixing
		fmt.Printf("[vibe] iteration %d: command failed, asking LLM to fix...\n", i+1)
		currentPrompt = fmt.Sprintf(
			"The command `%s` failed with the following output:\n\n```\n%s\n```\n\nPlease fix the issues and try again.",
			config.RunCommand, output,
		)
	}

	return fmt.Errorf("vibe: max iterations (%d) reached without passing", config.MaxIterations)
}

// runVibeCommand executes a shell command and returns its combined output and error.
func runVibeCommand(ctx context.Context, command string) (string, error) {
	ctx, cancel := context.WithTimeout(ctx, 5*time.Minute)
	defer cancel()

	cmd := exec.CommandContext(ctx, "sh", "-c", command)
	out, err := cmd.CombinedOutput()
	return strings.TrimSpace(string(out)), err
}
