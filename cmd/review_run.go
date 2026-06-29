package cmd

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"time"

	"github.com/GrayCodeAI/eyrie/runtime"
	reviewcontracts "github.com/GrayCodeAI/hawk-core-contracts/review"
	hawkSight "github.com/GrayCodeAI/hawk/internal/bridge/sight"
	"github.com/GrayCodeAI/hawk/internal/types"
	"github.com/GrayCodeAI/hawk/internal/ui/icons"
	sightLib "github.com/GrayCodeAI/sight"
	"github.com/spf13/cobra"
)

var (
	reviewRunBackground bool
	reviewRunModel      string
	reviewRunConcerns   string
	reviewRunTimeout    time.Duration
)

var reviewRunCmd = &cobra.Command{
	Use:   "run <sha>",
	Short: "Review a specific commit",
	Args:  cobra.ExactArgs(1),
	RunE:  runReviewRun,
}

func init() {
	reviewRunCmd.Flags().BoolVar(&reviewRunBackground, "background", false, "Run silently (for hook use)")
	reviewRunCmd.Flags().StringVar(&reviewRunModel, "model", "", "LLM model for review")
	reviewRunCmd.Flags().StringVar(&reviewRunConcerns, "concerns", "", "Comma-separated concerns")
	reviewRunCmd.Flags().DurationVar(&reviewRunTimeout, "timeout", 3*time.Minute, "Review timeout")
	reviewCmd.AddCommand(reviewRunCmd)
}

func runReviewRun(_ *cobra.Command, args []string) error {
	sha := args[0]

	// Resolve short SHA to full.
	if len(sha) < 40 {
		out, err := exec.CommandContext(context.Background(), "git", "rev-parse", sha).Output()
		if err == nil {
			sha = strings.TrimSpace(string(out))
		}
	}

	projectDir, _ := os.Getwd()
	store, err := OpenReviewStore(projectDir)
	if err != nil {
		return silentErr(err, "open review store")
	}
	defer func() { _ = store.Close() }()

	// Check if already reviewed.
	if existing, _ := store.GetBySHA(sha); existing != nil && existing.Status != ReviewStatusFailed {
		if !reviewRunBackground {
			fmt.Printf("Commit %s already reviewed (status: %s)\n", sha[:8], existing.Status)
		}
		return nil
	}

	// Create pending record.
	id, err := store.Create(sha)
	if err != nil {
		return silentErr(err, "create review record")
	}
	_ = store.SetStatus(id, ReviewStatusRunning)

	// Get commit diff.
	diff, err := getCommitDiff(sha)
	if err != nil {
		_ = store.SetStatus(id, ReviewStatusFailed)
		return silentErr(err, "get commit diff")
	}
	if strings.TrimSpace(diff) == "" {
		_ = store.SetStatus(id, ReviewStatusPassed)
		if !reviewRunBackground {
			fmt.Println("Empty diff — nothing to review.")
		}
		return nil
	}

	// Build sight bridge from the runtime-owned transport resolution.
	transport, err := runtime.ResolveChatTransport(context.Background(), runtime.ChatTransportOpts{
		Selection: runtime.SelectionOpts{
			ProviderOverride: strings.TrimSpace(provider),
			ModelOverride:    reviewRunModel,
		},
	})
	if err != nil {
		_ = store.SetStatus(id, ReviewStatusFailed)
		return silentErr(fmt.Errorf("resolve runtime transport: %w", err), "init bridge")
	}
	if transport.Provider == nil {
		_ = store.SetStatus(id, ReviewStatusFailed)
		return silentErr(fmt.Errorf("runtime transport unavailable for provider %q", transport.Selection.Provider), "init bridge")
	}

	var opts []sightLib.Option
	if reviewRunModel != "" {
		opts = append(opts, sightLib.WithModel(reviewRunModel))
	}
	if reviewRunConcerns != "" {
		concerns := strings.Split(reviewRunConcerns, ",")
		for i := range concerns {
			concerns[i] = strings.TrimSpace(concerns[i])
		}
		opts = append(opts, sightLib.WithConcerns(concerns...))
	}

	bridge := hawkSight.NewBridge(types.WrapClientProvider(transport.Provider), transport.Selection.Provider, opts...)
	if !bridge.Ready() {
		_ = store.SetStatus(id, ReviewStatusFailed)
		return silentErr(fmt.Errorf("sight bridge not ready"), "init bridge")
	}

	ctx := context.Background()
	if reviewRunTimeout > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, reviewRunTimeout)
		defer cancel()
	}

	// Run review.
	result, err := bridge.ReviewContracts(ctx, diff)
	if err != nil {
		_ = store.SetStatus(id, ReviewStatusFailed)
		return silentErr(err, "sight review")
	}

	// Determine status based on findings.
	status := ReviewStatusPassed
	if len(result.Findings) > 0 {
		status = ReviewStatusOpen
	}

	if err := store.Update(id, status, result); err != nil {
		return silentErr(err, "store result")
	}

	if !reviewRunBackground {
		printReviewSummary(sha, result)
	}
	return nil
}

func getCommitDiff(sha string) (string, error) {
	// For the first commit, diff against empty tree.
	out, err := exec.CommandContext(context.Background(), "git", "diff-tree", "-p", sha).Output()
	if err != nil {
		// Fallback: diff against parent.
		out, err = exec.CommandContext(context.Background(), "git", "diff", sha+"^", sha).Output()
		if err != nil {
			return "", fmt.Errorf("git diff for %s: %w", sha[:8], err)
		}
	}
	return string(out), nil
}

func printReviewSummary(sha string, result *reviewcontracts.Result) {
	if len(result.Findings) == 0 {
		fmt.Printf("%s %s — no issues found (%d files reviewed)\n", icons.CheckBold(), sha[:8], result.Stats.FilesReviewed)
		return
	}
	fmt.Printf("%s %s — %d findings (max severity: %s)\n", icons.Alert(), sha[:8], len(result.Findings), result.MaxSeverity())
	for _, f := range result.Findings {
		fmt.Printf("  [%s] %s:%d — %s\n", f.Severity, f.File, f.Line, f.Message)
	}
}

// silentErr suppresses errors in background mode, prints otherwise.
func silentErr(err error, context string) error {
	if reviewRunBackground {
		return nil
	}
	return fmt.Errorf("%s: %w", context, err)
}
