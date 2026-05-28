package cmd

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strings"
	"time"

	hawkconfig "github.com/GrayCodeAI/hawk/internal/config"
	"github.com/GrayCodeAI/hawk/internal/engine"
	"github.com/GrayCodeAI/hawk/internal/multiagent/agents"
	"github.com/GrayCodeAI/hawk/internal/observability/logger"
	"github.com/GrayCodeAI/hawk/internal/session"
	"github.com/spf13/cobra"
)

var (
	execOutputFormat string
	execAutoLevel    string
	execModel        string
	execMaxTurns     int
	execCWD          string
	execAgent        string
	execSessionID    string
	execTag          string
	execWorktree     bool
	execWorktreeName string
	execEphemeral    bool
	execJSON         bool
)

// ExitCodeError wraps a non-zero exit code so it can be returned from RunE
// instead of calling os.Exit directly. This allows deferred cleanup (worktree
// removal, global state restoration) to run before the process exits.
type ExitCodeError struct {
	Code int
	Err  error
}

func (e *ExitCodeError) Error() string {
	if e.Err != nil {
		return e.Err.Error()
	}
	return fmt.Sprintf("exit code %d", e.Code)
}

func (e *ExitCodeError) Unwrap() error { return e.Err }

// ExecResult is the structured output for --output-format json.
type ExecResult struct {
	SessionID  string `json:"session_id"`
	Response   string `json:"response"`
	ExitCode   int    `json:"exit_code"`
	TokensIn   int    `json:"tokens_in,omitempty"`
	TokensOut  int    `json:"tokens_out,omitempty"`
	TurnsTaken int    `json:"turns_taken"`
	Duration   string `json:"duration"`
	Model      string `json:"model,omitempty"`
	Worktree   string `json:"worktree,omitempty"`
	Branch     string `json:"branch,omitempty"`
}

var execCmd = &cobra.Command{
	Use:   "exec [prompt]",
	Short: "Execute a single command non-interactively",
	Long: `Execute a prompt in non-interactive mode for scripting and CI/CD.

Supports piping from stdin, JSON output format, and autonomy levels.

Use --ephemeral to skip session persistence (ideal for CI runs).
Use --json for JSON output (alias for --output-format json).

Autonomy Levels:
  supervised (default)  Ask for permission on every tool call
  basic                 Auto-allow read-only tools
  semi                  Auto-allow reads and writes, ask for Bash
  full                  Auto-allow everything except destructive commands
  yolo                  Never ask for permission

Examples:
  hawk exec "analyze this codebase"
  hawk exec --auto full "fix the tests and commit"
  hawk exec --json "what files are in src/"
  hawk exec --ephemeral --json "run tests and report" > result.json
  echo "explain main.go" | hawk exec -
  hawk exec --agent reviewer "review the latest commit"
  hawk exec --model claude-sonnet-4-6 "quick fix: typo in README"`,
	Args: cobra.MaximumNArgs(1),
	RunE: runExec,
}

func init() {
	execCmd.Flags().StringVarP(&execOutputFormat, "output-format", "o", "text", "Output format: text or json")
	execCmd.Flags().StringVar(&execAutoLevel, "auto", "", "Autonomy level: supervised|basic|semi|full|yolo")
	execCmd.Flags().StringVarP(&execModel, "model", "m", "", "Model ID to use")
	execCmd.Flags().IntVar(&execMaxTurns, "max-turns", 0, "Maximum agentic turns (0 = unlimited)")
	execCmd.Flags().StringVar(&execCWD, "cwd", "", "Working directory")
	execCmd.Flags().StringVar(&execAgent, "agent", "", "Agent persona to use (from ~/.hawk/agents/)")
	execCmd.Flags().StringVarP(&execSessionID, "session-id", "s", "", "Continue an existing session")
	execCmd.Flags().StringVar(&execTag, "tag", "", "Session tag for categorization")
	execCmd.Flags().BoolVarP(&execWorktree, "worktree", "w", false, "Run in an isolated git worktree")
	execCmd.Flags().StringVar(&execWorktreeName, "worktree-name", "", "Branch name for worktree (auto-generated if empty)")
	execCmd.Flags().BoolVar(&execEphemeral, "ephemeral", false, "Skip session persistence (CI mode)")
	execCmd.Flags().BoolVarP(&execJSON, "json", "j", false, "JSON output (alias for --output-format json)")
}

func runExec(_ *cobra.Command, args []string) error {
	start := time.Now()

	if execJSON {
		execOutputFormat = "json"
	}

	prompt, err := resolveExecPrompt(args)
	if err != nil {
		return err
	}

	if execCWD != "" {
		if err := os.Chdir(execCWD); err != nil {
			return fmt.Errorf("chdir %s: %w", execCWD, err)
		}
	}

	// Worktree isolation: create a temporary worktree and chdir into it
	var wtPath, wtBranch string
	if execWorktree {
		cwd, _ := os.Getwd()
		base := getCurrentBranch(cwd)
		branch := execWorktreeName
		if branch == "" {
			branch = fmt.Sprintf("hawk-exec/%d-%s", start.UnixMilli(), randomHex(4))
		}
		var wtErr error
		wtPath, wtErr = createExecWorktree(cwd, base, branch)
		if wtErr != nil {
			return fmt.Errorf("worktree: %w", wtErr)
		}
		wtBranch = branch
		defer cleanupExecWorktree(cwd, wtPath)
		if err := os.Chdir(wtPath); err != nil {
			return fmt.Errorf("chdir worktree: %w", err)
		}
	}

	// Load settings
	settings := hawkconfig.LoadSettings()

	// Build system prompt
	systemPrompt, err := buildSystemPrompt()
	if err != nil {
		return err
	}

	// If --agent is specified, prepend the agent persona
	var agentModel string
	if execAgent != "" {
		agentDef, err := agents.Get(execAgent)
		if err != nil {
			return fmt.Errorf("agent %q: %w", execAgent, err)
		}
		systemPrompt = agentDef.Prompt + "\n\n" + systemPrompt
		agentModel = agentDef.Model
	}

	// Create tool registry
	registry, err := defaultRegistry(settings)
	if err != nil {
		return err
	}

	// Resolve effective model/provider without mutating globals.
	// Priority: agent model > exec model > settings > global > auto-detect.
	effectiveModel, effectiveProvider := effectiveModelAndProvider(settings)
	if execModel != "" {
		effectiveModel = execModel
	}
	if agentModel != "" {
		effectiveModel = agentModel
	}

	// Create engine session
	sess := newHawkSession(settings, effectiveProvider, effectiveModel, systemPrompt, registry)
	sess.SetLogger(logger.New(io.Discard, logger.Error))

	if err := configureSession(sess, settings, execMaxTurns); err != nil {
		return err
	}

	// Apply autonomy level
	if execAutoLevel != "" {
		sess.Autonomy = engine.ParseAutonomyLevel(execAutoLevel)
	}

	// In exec mode, auto-approve based on autonomy level (no TUI to ask)
	sess.PermissionFn = func(req engine.PermissionRequest) {
		cfg := engine.PresetConfig(sess.Autonomy)
		allowed := !cfg.NeedsPermission(req.ToolName, false)
		if req.Response != nil {
			req.Response <- allowed
		}
	}

	// Resume existing session if --session-id provided
	if execSessionID != "" {
		saved, err := session.Load(execSessionID)
		if err != nil {
			return fmt.Errorf("resume session %s: %w", execSessionID, err)
		}
		sess.LoadMessages(toEyrieMessages(saved.Messages))
	}

	// Add the user prompt
	sess.AddUser(prompt)

	// Run the agent loop
	ctx := context.Background()
	if timeout > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, timeout)
		defer cancel()
	}

	events, err := sess.Stream(ctx)
	if err != nil {
		return fmt.Errorf("stream: %w", err)
	}

	// Collect response
	var response strings.Builder
	var totalIn, totalOut, turns int
	var execErr string
	jsonEnc := json.NewEncoder(os.Stdout)

	for ev := range events {
		switch ev.Type {
		case "content":
			response.WriteString(ev.Content)
			if execOutputFormat == "text" {
				fmt.Print(ev.Content)
			}
			if execOutputFormat == "json" {
				_ = jsonEnc.Encode(map[string]interface{}{
					"type":    "content",
					"content": ev.Content,
				})
			}
		case "usage":
			if ev.Usage != nil {
				totalIn += ev.Usage.PromptTokens
				totalOut += ev.Usage.CompletionTokens
				turns++
			}
		case "error":
			execErr = ev.Content
			if execOutputFormat == "text" {
				_, _ = fmt.Fprintf(os.Stderr, "\nerror: %s\n", ev.Content)
			}
			if execOutputFormat == "json" {
				_ = jsonEnc.Encode(map[string]interface{}{
					"type":    "error",
					"content": ev.Content,
				})
			}
		case "tool_use":
			if execOutputFormat == "json" {
				_ = jsonEnc.Encode(map[string]interface{}{
					"type": "tool_use",
					"tool": ev.ToolName,
				})
			}
		case "tool_result":
			if execOutputFormat == "json" {
				_ = jsonEnc.Encode(map[string]interface{}{
					"type":   "tool_result",
					"tool":   ev.ToolName,
					"result": ev.Content,
				})
			}
		case "done":
			if execOutputFormat == "json" {
				_ = jsonEnc.Encode(map[string]interface{}{
					"type": "done",
				})
			}
		}
	}

	// Persist session for resume/search (skip in ephemeral/CI mode)
	exitCode := 0
	if execErr != "" {
		exitCode = 1
	}
	if !execEphemeral {
		sessionID := fmt.Sprintf("exec-%d-%s", start.UnixMilli(), randomHex(4))
		persistExecSession(sessionID, effectiveModel, effectiveProvider, prompt, response.String())
	}

	if execOutputFormat == "text" {
		if !strings.HasSuffix(response.String(), "\n") {
			fmt.Println()
		}
		if exitCode != 0 {
			return fmt.Errorf("exec failed: %s", execErr)
		}
		return nil
	}

	if execOutputFormat == "json" {
		result := ExecResult{
			SessionID:  fmt.Sprintf("exec-%d-%s", start.UnixMilli(), randomHex(4)),
			Response:   response.String(),
			ExitCode:   exitCode,
			TokensIn:   totalIn,
			TokensOut:  totalOut,
			TurnsTaken: turns,
			Duration:   time.Since(start).Round(time.Millisecond).String(),
			Model:      effectiveModel,
			Worktree:   wtPath,
			Branch:     wtBranch,
		}
		if execOutputFormat == "json" && execEphemeral {
			_ = jsonEnc.Encode(map[string]interface{}{
				"type":   "result",
				"result": result,
			})
		}
		if !execEphemeral {
			enc := json.NewEncoder(os.Stdout)
			enc.SetIndent("", "  ")
			return enc.Encode(result)
		}
	}

	if exitCode != 0 {
		return &ExitCodeError{Code: exitCode}
	}
	return nil
}

func resolveExecPrompt(args []string) (string, error) {
	if len(args) == 0 {
		return "", fmt.Errorf("prompt is required (provide as argument or pipe via stdin with '-')")
	}
	if args[0] == "-" {
		data, err := io.ReadAll(os.Stdin)
		if err != nil {
			return "", fmt.Errorf("reading stdin: %w", err)
		}
		return strings.TrimSpace(string(data)), nil
	}
	return args[0], nil
}

func persistExecSession(id, model, provider, userMsg, assistantMsg string) {
	s := &session.Session{
		ID:       id,
		Model:    model,
		Provider: provider,
		Messages: []session.Message{
			{Role: "user", Content: userMsg},
			{Role: "assistant", Content: assistantMsg},
		},
	}
	_ = session.Save(s)
}

func createExecWorktree(repoDir, baseBranch, branch string) (string, error) {
	cmd := exec.CommandContext(context.Background(), "mktemp", "-d")
	out, err := cmd.Output()
	if err != nil {
		return "", err
	}
	wtPath := strings.TrimSpace(string(out))

	gitCmd := exec.CommandContext(context.Background(), "git", "worktree", "add", "-b", branch, wtPath, baseBranch)
	gitCmd.Dir = repoDir
	if errOut, err := gitCmd.CombinedOutput(); err != nil {
		return "", fmt.Errorf("%s: %w", strings.TrimSpace(string(errOut)), err)
	}
	return wtPath, nil
}

func cleanupExecWorktree(repoDir, wtPath string) {
	if wtPath == "" {
		return
	}
	cmd := exec.CommandContext(context.Background(), "git", "worktree", "remove", "--force", wtPath)
	cmd.Dir = repoDir
	_ = cmd.Run()
}

// randomHex returns a hex-encoded string of n random bytes.
func randomHex(n int) string {
	b := make([]byte, n)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}
