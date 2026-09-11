package cmd

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/GrayCodeAI/hawk/internal/ui/icons"
)

var (
	refineMaxIter int
	refineModel   string
	refineTimeout time.Duration
)

var reviewRefineCmd = &cobra.Command{
	Use:   "refine [id...]",
	Short: "Iterative fix loop: fix, re-review, repeat until passing",
	Long: `Runs in a loop: fix findings → re-review the new commit → fix again,
until all reviews pass or --max-iterations is reached.

Uses an isolated worktree by default to avoid disrupting your working tree.`,
	RunE: runReviewRefine,
}

func init() {
	reviewRefineCmd.Flags().IntVar(&refineMaxIter, "max-iterations", 3, "Maximum fix iterations")
	reviewRefineCmd.Flags().StringVarP(&refineModel, "model", "m", "", "Model for fix agent")
	reviewRefineCmd.Flags().DurationVar(&refineTimeout, "timeout", 5*time.Minute, "Timeout per review cycle")
	reviewCmd.AddCommand(reviewRefineCmd)
}

func runReviewRefine(_ *cobra.Command, args []string) error {
	projectDir, _ := os.Getwd()
	store, err := OpenReviewStore(projectDir)
	if err != nil {
		return fmt.Errorf("open store: %w", err)
	}
	defer func() { _ = store.Close() }()

	// Collect target reviews.
	var reviews []*ReviewRecord
	if len(args) > 0 {
		for _, ref := range args {
			r, resolveErr := resolveReview(store, ref)
			if resolveErr != nil {
				return resolveErr
			}
			reviews = append(reviews, r)
		}
	} else {
		reviews, err = store.ListOpen()
		if err != nil {
			return err
		}
	}

	if len(reviews) == 0 {
		fmt.Println(auditTint("No open reviews to refine.", textMuted))
		return nil
	}

	fmt.Printf("%s\n\n", auditTint(fmt.Sprintf("Refining %d review(s), max %d iterations...", len(reviews), refineMaxIter), textPrimary))

	for iter := 1; iter <= refineMaxIter; iter++ {
		fmt.Printf("%s\n", auditTint(fmt.Sprintf("── Iteration %d/%d ──", iter, refineMaxIter), hawkColor))

		// Fix all open reviews.
		for _, r := range reviews {
			if r.Status != ReviewStatusOpen {
				continue
			}
			if err := fixReviewRefine(store, r); err != nil {
				fmt.Printf("  %s %s\n", auditTint(icons.CloseThick(), errorCoral), auditTint(fmt.Sprintf("#%d fix failed: %v", r.ID, err), errorCoral))
			} else {
				fmt.Printf("  %s %s\n", auditTint(icons.CheckBold(), doneGreen), auditTint(fmt.Sprintf("#%d fix applied", r.ID), doneGreen))
			}
		}

		// Wait briefly for hook to fire, then re-review the latest commit.
		latestSHA := getLatestCommitSHA()
		if latestSHA == "" {
			fmt.Println(auditTint("  Could not determine latest commit.", textMuted))
			break
		}

		fmt.Printf("%s\n", auditTint("  Reviewing "+latestSHA[:8]+"...", textPrimary))
		if err := runReviewOnSHA(store, latestSHA); err != nil {
			fmt.Printf("  %s %s\n", auditTint(icons.CloseThick(), errorCoral), auditTint("Review failed: "+err.Error(), errorCoral))
			break
		}

		// Check if the new review passed.
		newReview, getErr := store.GetBySHA(latestSHA)
		if getErr != nil {
			return fmt.Errorf("load review for %s: %w", latestSHA[:8], getErr)
		}
		if newReview != nil && newReview.Status == ReviewStatusPassed {
			fmt.Printf("\n%s %s\n", auditTint(icons.CheckBold(), doneGreen), auditTint(fmt.Sprintf("All clean after %d iteration(s)!", iter), textPrimary))
			return nil
		}

		// Update reviews list for next iteration.
		if newReview != nil && newReview.Status == ReviewStatusOpen {
			reviews = []*ReviewRecord{newReview}
		} else {
			var listErr error
			reviews, listErr = store.ListOpen()
			if listErr != nil {
				return fmt.Errorf("list open reviews: %w", listErr)
			}
			if len(reviews) == 0 {
				fmt.Printf("\n%s %s\n", auditTint(icons.CheckBold(), doneGreen), auditTint(fmt.Sprintf("All reviews resolved after %d iteration(s)!", iter), textPrimary))
				return nil
			}
		}
	}

	// Report remaining issues.
	remaining, listErr := store.ListOpen()
	if listErr != nil {
		return fmt.Errorf("list open reviews: %w", listErr)
	}
	if len(remaining) > 0 {
		fmt.Printf("\n%s %s\n", auditTint(icons.Alert(), warnAmber), auditTint(fmt.Sprintf("%d review(s) still open after %d iterations.", len(remaining), refineMaxIter), textPrimary))
		fmt.Println(auditTint("  Run 'hawk review show' to inspect, or increase --max-iterations.", textMuted))
	}
	return nil
}

func fixReviewRefine(store *ReviewStore, r *ReviewRecord) error {
	prompt := buildFixPrompt(r)

	hawkBin, err := os.Executable()
	if err != nil {
		hawkBin = "hawk"
	}

	execArgs := []string{"exec", "--auto", "full"}
	if refineModel != "" {
		execArgs = append(execArgs, "--model", refineModel)
	}
	execArgs = append(execArgs, prompt)

	cmd := exec.CommandContext(context.Background(), hawkBin, execArgs...) // #nosec G204 -- hawkBin resolved via os.Executable() or literal 'hawk'; args are internal flags
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	if err := cmd.Run(); err != nil {
		return err
	}
	return store.SetStatus(r.ID, ReviewStatusFixed)
}

func runReviewOnSHA(store *ReviewStore, sha string) error {
	hawkBin, err := os.Executable()
	if err != nil {
		hawkBin = "hawk"
	}

	args := []string{"review", "run", sha}
	if refineTimeout > 0 {
		args = append(args, "--timeout", refineTimeout.String())
	}
	if refineModel != "" {
		args = append(args, "--model", refineModel)
	}

	cmd := exec.CommandContext(context.Background(), hawkBin, args...) // #nosec G204 -- hawkBin resolved via os.Executable() or literal 'hawk'; args are internal flags
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

func getLatestCommitSHA() string {
	out, err := exec.CommandContext(context.Background(), "git", "rev-parse", "HEAD").Output()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(out))
}
