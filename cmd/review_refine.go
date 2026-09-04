package cmd

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/GrayCodeAI/graycode-cli/internal/ui/icons"
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
		fmt.Println("No open reviews to refine.")
		return nil
	}

	fmt.Printf("Refining %d review(s), max %d iterations...\n\n", len(reviews), refineMaxIter)

	for iter := 1; iter <= refineMaxIter; iter++ {
		fmt.Printf("── Iteration %d/%d ──\n", iter, refineMaxIter)

		// Fix all open reviews.
		for _, r := range reviews {
			if r.Status != ReviewStatusOpen {
				continue
			}
			if err := fixReviewRefine(store, r); err != nil {
				fmt.Printf("  %s #%d fix failed: %v\n", icons.CloseThick(), r.ID, err)
			} else {
				fmt.Printf("  %s #%d fix applied\n", icons.CheckBold(), r.ID)
			}
		}

		// Wait briefly for hook to fire, then re-review the latest commit.
		latestSHA := getLatestCommitSHA()
		if latestSHA == "" {
			fmt.Println("  Could not determine latest commit.")
			break
		}

		fmt.Printf("  Reviewing %s...\n", latestSHA[:8])
		if err := runReviewOnSHA(store, latestSHA); err != nil {
			fmt.Printf("  %s Review failed: %v\n", icons.CloseThick(), err)
			break
		}

		// Check if the new review passed.
		newReview, getErr := store.GetBySHA(latestSHA)
		if getErr != nil {
			return fmt.Errorf("load review for %s: %w", latestSHA[:8], getErr)
		}
		if newReview != nil && newReview.Status == ReviewStatusPassed {
			fmt.Printf("\n%s All clean after %d iteration(s)!\n", icons.CheckBold(), iter)
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
				fmt.Printf("\n%s All reviews resolved after %d iteration(s)!\n", icons.CheckBold(), iter)
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
		fmt.Printf("\n%s %d review(s) still open after %d iterations.\n", icons.Alert(), len(remaining), refineMaxIter)
		fmt.Println("  Run 'graycode review show' to inspect, or increase --max-iterations.")
	}
	return nil
}

func fixReviewRefine(store *ReviewStore, r *ReviewRecord) error {
	prompt := buildFixPrompt(r)

	graycodeBin, err := os.Executable()
	if err != nil {
		graycodeBin = "graycode"
	}

	execArgs := []string{"exec", "--auto", "full"}
	if refineModel != "" {
		execArgs = append(execArgs, "--model", refineModel)
	}
	execArgs = append(execArgs, prompt)

	cmd := exec.CommandContext(context.Background(), graycodeBin, execArgs...) // #nosec G204 -- graycodeBin resolved via os.Executable() or literal 'graycode'; args are internal flags
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	if err := cmd.Run(); err != nil {
		return err
	}
	return store.SetStatus(r.ID, ReviewStatusFixed)
}

func runReviewOnSHA(store *ReviewStore, sha string) error {
	graycodeBin, err := os.Executable()
	if err != nil {
		graycodeBin = "graycode"
	}

	args := []string{"review", "run", sha}
	if refineTimeout > 0 {
		args = append(args, "--timeout", refineTimeout.String())
	}
	if refineModel != "" {
		args = append(args, "--model", refineModel)
	}

	cmd := exec.CommandContext(context.Background(), graycodeBin, args...) // #nosec G204 -- graycodeBin resolved via os.Executable() or literal 'graycode'; args are internal flags
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
