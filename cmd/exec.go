package cmd

import (
	"context"
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
)

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

Autonomy Levels:
  supervised (default)  Ask for permission on every tool call
  basic                 Auto-allow read-only tools
  semi                  Auto-allow reads and writes, ask for Bash
  full                  Auto-allow everything except destructive commands
  yolo                  Never ask for permission

Examples:
  hawk exec "analyze this codebase"
  hawk exec --auto full "fix the tests and commit"
  hawk exec --output-format json "what files are in src/"
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
}

func runExec(_ *cobra.Command, args []string) error {
	start := time.Now()

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
			branch = fmt.Sprintf("hawk-exec/%d", start.UnixMilli())
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

	// Override global flags so shared helpers pick up exec-specific values
	if execModel != "" {
		model = execModel
	}
	if execMaxTurns > 0 {
		maxTurns = execMaxTurns
	}

	// Load settings
	settings := hawkconfig.LoadSettings()

	// Build system prompt
	systemPrompt, err := buildSystemPrompt()
	if err != nil {
		return err
	}

	// If --agent is specified, prepend the agent persona
	if execAgent != "" {
		agentDef, err := agents.Get(execAgent)
		if err != nil {
			return fmt.Errorf("agent %q: %w", execAgent, err)
		}
		systemPrompt = agentDef.Prompt + "\n\n" + systemPrompt
		if agentDef.Model != "" {
			model = agentDef.Model
		}
	}

	// Create tool registry
	registry, err := defaultRegistry(settings)
	if err != nil {
		return err
	}

	// Create engine session
	effectiveModel, effectiveProvider := effectiveModelAndProvider(settings)
	sess := engine.NewSession(effectiveProvider, effectiveModel, systemPrompt, registry)
	sess.SetLogger(logger.New(io.Discard, logger.Error))

	if err := configureSession(sess, settings); err != nil {
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

	for ev := range events {
		switch ev.Type {
		case "content":
			response.WriteString(ev.Content)
			if execOutputFormat == "text" {
				fmt.Print(ev.Content)
			}
		case "usage":
			if ev.Usage != nil {
				totalIn += ev.Usage.PromptTokens
				totalOut += ev.Usage.CompletionTokens
				turns++
			}
		case "error":
			if execOutputFormat == "text" {
				_, _ = fmt.Fprintf(os.Stderr, "\nerror: %s\n", ev.Content)
			}
		case "done":
			// loop will exit when channel closes
		}
	}

	// Persist session for resume/search
	sessionID := fmt.Sprintf("exec-%d", start.UnixMilli())
	persistExecSession(sessionID, effectiveModel, effectiveProvider, prompt, response.String())

	if execOutputFormat == "text" {
		if !strings.HasSuffix(response.String(), "\n") {
			fmt.Println()
		}
		return nil
	}

	// JSON output
	result := ExecResult{
		SessionID:  sessionID,
		Response:   response.String(),
		ExitCode:   0,
		TokensIn:   totalIn,
		TokensOut:  totalOut,
		TurnsTaken: turns,
		Duration:   time.Since(start).Round(time.Millisecond).String(),
		Model:      effectiveModel,
		Worktree:   wtPath,
		Branch:     wtBranch,
	}

	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")
	return enc.Encode(result)
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
	cmd := exec.Command("mktemp", "-d")
	out, err := cmd.Output()
	if err != nil {
		return "", err
	}
	wtPath := strings.TrimSpace(string(out))

	gitCmd := exec.Command("git", "worktree", "add", "-b", branch, wtPath, baseBranch)
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
	cmd := exec.Command("git", "worktree", "remove", "--force", wtPath)
	cmd.Dir = repoDir
	_ = cmd.Run()
}
