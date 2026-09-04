package cmd

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"time"

	graycodeKestrel "github.com/GrayCodeAI/graycode-cli/internal/bridge/kestrel"
	graycodeconfig "github.com/GrayCodeAI/graycode-cli/internal/config"
	reviewcontracts "github.com/GrayCodeAI/graycode-cli/internal/contracts/review"
	"github.com/GrayCodeAI/graycode-cli/internal/engine"
	"github.com/GrayCodeAI/graycode-cli/internal/ui/icons"
	kestrelLib "github.com/GrayCodeAI/kestrel"
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
		out, err := exec.CommandContext(context.Background(), "git", "rev-parse", sha).Output() // #nosec G204 -- fixed command 'git' with args, not user-controlled binary
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
	existing, getErr := store.GetBySHA(sha)
	if getErr != nil {
		return silentErr(getErr, "load existing review")
	}
	if existing != nil && existing.Status != ReviewStatusFailed {
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
	if err := store.SetStatus(id, ReviewStatusRunning); err != nil {
		return silentErr(err, "mark review running")
	}

	// Get commit diff.
	diff, err := getCommitDiff(sha)
	if err != nil {
		if statusErr := store.SetStatus(id, ReviewStatusFailed); statusErr != nil {
			return silentErr(statusErr, "mark review failed")
		}
		return silentErr(err, "get commit diff")
	}
	if strings.TrimSpace(diff) == "" {
		if statusErr := store.SetStatus(id, ReviewStatusPassed); statusErr != nil {
			return silentErr(statusErr, "mark review passed")
		}
		if !reviewRunBackground {
			fmt.Println("Empty diff — nothing to review.")
		}
		return nil
	}

	// Build the Kestrel bridge through Graycode's Eyrie engine boundary.
	ctx := context.Background()
	selection := graycodeconfig.EffectiveSelection(ctx, graycodeconfig.SelectionOptions{
		ProviderOverride: strings.TrimSpace(provider),
		ModelOverride:    strings.TrimSpace(reviewRunModel),
	})
	chatProvider, providerID, err := engine.BuildChatProvider(ctx, selection, strings.TrimSpace(provider))
	if err != nil {
		if statusErr := store.SetStatus(id, ReviewStatusFailed); statusErr != nil {
			return silentErr(statusErr, "mark review failed")
		}
		return silentErr(fmt.Errorf("resolve engine transport: %w", err), "init bridge")
	}

	var opts []kestrelLib.Option
	if reviewRunModel != "" {
		opts = append(opts, kestrelLib.WithModel(reviewRunModel))
	}
	if reviewRunConcerns != "" {
		concerns := strings.Split(reviewRunConcerns, ",")
		for i := range concerns {
			concerns[i] = strings.TrimSpace(concerns[i])
		}
		opts = append(opts, kestrelLib.WithConcerns(concerns...))
	}

	bridge := graycodeKestrel.NewBridge(chatProvider, providerID, opts...)
	if !bridge.Ready() {
		if statusErr := store.SetStatus(id, ReviewStatusFailed); statusErr != nil {
			return silentErr(statusErr, "mark review failed")
		}
		return silentErr(fmt.Errorf("kestrel bridge not ready"), "init bridge")
	}

	if reviewRunTimeout > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, reviewRunTimeout)
		defer cancel()
	}

	// Run review.
	result, err := bridge.ReviewContracts(ctx, diff)
	if err != nil {
		if statusErr := store.SetStatus(id, ReviewStatusFailed); statusErr != nil {
			return silentErr(statusErr, "mark review failed")
		}
		return silentErr(err, "kestrel review")
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
	out, err := exec.CommandContext(context.Background(), "git", "diff-tree", "-p", sha).Output() // #nosec G204 -- fixed git executable
	if err != nil {
		// Fallback: diff against parent.
		out, err = exec.CommandContext(context.Background(), "git", "diff", sha+"^", sha).Output() // #nosec G204 -- fixed command 'git' with args, not user-controlled binary
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
