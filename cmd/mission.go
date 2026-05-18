package cmd

import (
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strings"
	"time"

	hawkconfig "github.com/GrayCodeAI/hawk/internal/config"
	"github.com/GrayCodeAI/hawk/internal/engine"
	mission "github.com/GrayCodeAI/hawk/internal/multiagent"
	"github.com/GrayCodeAI/hawk/internal/observability/logger"
	"github.com/spf13/cobra"
)

var (
	missionWorkers int
	missionModel   string
	missionAuto    string
	missionTimeout time.Duration
	missionDryRun  bool
)

var missionCmd = &cobra.Command{
	Use:   "mission <prompt>",
	Short: "Run a multi-agent mission (parallel feature execution)",
	Long: `Decompose a task into features and execute them in parallel git worktrees.

Each feature runs in its own worktree with a full engine session.
Results are committed on separate branches for review/merge.

Examples:
  hawk mission "Add auth, rate limiting, and logging to the API"
  hawk mission --workers 6 "Refactor the database layer into 3 services"
  hawk mission --model claude-sonnet-4-6 "Add tests for all untested packages"`,
	Args: cobra.ExactArgs(1),
	RunE: runMission,
}

func init() {
	missionCmd.Flags().IntVar(&missionWorkers, "workers", 4, "Max parallel workers")
	missionCmd.Flags().StringVarP(&missionModel, "model", "m", "", "Model for workers")
	missionCmd.Flags().StringVar(&missionAuto, "auto", "full", "Autonomy level for workers")
	missionCmd.Flags().DurationVar(&missionTimeout, "timeout", 30*time.Minute, "Mission timeout")
	missionCmd.Flags().BoolVar(&missionDryRun, "dry-run", false, "Plan only, don't execute workers")
}

func runMission(_ *cobra.Command, args []string) error {
	prompt := args[0]

	cwd, _ := os.Getwd()
	baseBranch := getCurrentBranch(cwd)

	settings := hawkconfig.LoadSettings()
	effectiveModel, effectiveProvider := effectiveModelAndProvider(settings)
	if missionModel != "" {
		effectiveModel = missionModel
	}

	autonomy := engine.ParseAutonomyLevel(missionAuto)

	cfg := mission.Config{
		MaxWorkers:    missionWorkers,
		WorkerModel:   effectiveModel,
		RepoDir:       cwd,
		BaseBranch:    baseBranch,
		AutonomyLevel: int(autonomy),
	}

	m := mission.New(prompt, cfg)

	fmt.Printf("Mission %s: planning...\n", m.ID)

	// Plan: decompose into features using LLM
	planFn := func(ctx context.Context, p string) ([]mission.Feature, error) {
		return planWithLLM(ctx, p, effectiveProvider, effectiveModel, settings)
	}

	ctx, cancel := context.WithTimeout(context.Background(), missionTimeout)
	defer cancel()

	if err := m.Plan(ctx, planFn); err != nil {
		return fmt.Errorf("planning: %w", err)
	}

	fmt.Printf("Mission %s: %d features planned\n", m.ID, len(m.Features))
	for i, f := range m.Features {
		fmt.Printf("  %d. %s\n", i+1, f.Description)
	}
	fmt.Println()

	if missionDryRun {
		fmt.Println("(dry-run: not executing workers)")
		return nil
	}

	// Build system prompt for workers
	systemPrompt, _ := buildSystemPrompt()

	// Run features in parallel
	workerFn := mission.EngineWorker(effectiveProvider, effectiveModel, systemPrompt)

	fmt.Printf("Executing with %d parallel workers...\n\n", cfg.MaxWorkers)
	if err := m.Run(ctx, workerFn); err != nil {
		return err
	}

	// Print results
	fmt.Println()
	fmt.Println(m.Summary())
	fmt.Println()
	for _, f := range m.Features {
		status := "✓"
		if f.Status == mission.FeatureFailed {
			status = "✗"
		}
		branch := f.Branch
		if f.Handoff != nil && f.Handoff.CommitID != "" {
			branch += " (" + f.Handoff.CommitID[:7] + ")"
		}
		fmt.Printf("  %s %s — %s\n", status, f.Description, branch)
	}

	return nil
}

func planWithLLM(ctx context.Context, prompt, provider, model string, settings hawkconfig.Settings) ([]mission.Feature, error) {
	planPrompt := fmt.Sprintf(
		"Decompose this task into independent features that can be implemented in parallel.\n\n"+
			"Task: %s\n\n"+
			"Return a numbered list of features. Each feature should be:\n"+
			"- Independent (can be implemented without the others)\n"+
			"- Specific (clear what to implement)\n"+
			"- Testable (has clear success criteria)\n\n"+
			"Format: one feature per line, numbered. Just the descriptions, no extra text.",
		prompt,
	)

	registry, _ := defaultRegistry(settings)
	sess := engine.NewSession(provider, model, planPrompt, registry)
	sess.SetLogger(logger.New(io.Discard, logger.Error))
	_ = configureSession(sess, settings)
	sess.MaxTurns = 1
	sess.PermissionFn = func(req engine.PermissionRequest) {
		if req.Response != nil {
			req.Response <- true
		}
	}

	sess.AddUser(planPrompt)
	events, err := sess.Stream(ctx)
	if err != nil {
		return nil, err
	}

	var response strings.Builder
	for ev := range events {
		if ev.Type == "content" {
			response.WriteString(ev.Content)
		}
	}

	return parseFeatures(response.String()), nil
}

func parseFeatures(text string) []mission.Feature {
	var features []mission.Feature
	for _, line := range strings.Split(text, "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		// Strip numbering: "1. ", "1) ", "- "
		for _, prefix := range []string{"- ", "* "} {
			line = strings.TrimPrefix(line, prefix)
		}
		if len(line) > 2 && line[0] >= '0' && line[0] <= '9' {
			idx := strings.IndexAny(line, ".)")
			if idx > 0 && idx < 4 {
				line = strings.TrimSpace(line[idx+1:])
			}
		}
		if line == "" {
			continue
		}
		features = append(features, mission.Feature{
			Description: line,
		})
	}
	return features
}

func getCurrentBranch(dir string) string {
	cmd := exec.Command("git", "rev-parse", "--abbrev-ref", "HEAD")
	cmd.Dir = dir
	out, err := cmd.Output()
	if err != nil {
		return "main"
	}
	return strings.TrimSpace(string(out))
}
