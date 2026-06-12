package cmd

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"strconv"
	"strings"

	"github.com/spf13/cobra"
)

var (
	reviewFixAll      bool
	reviewFixWorktree bool
	reviewFixModel    string
)

var reviewFixCmd = &cobra.Command{
	Use:   "fix [id...]",
	Short: "Auto-fix review findings using hawk exec",
	Long: `Feeds review findings to hawk's engine which applies fixes and commits.
Without arguments, fixes all open reviews. Specify IDs to fix specific ones.`,
	RunE: runReviewFix,
}

func init() {
	reviewFixCmd.Flags().BoolVarP(&reviewFixAll, "all", "a", false, "Fix all open reviews")
	reviewFixCmd.Flags().BoolVarP(&reviewFixWorktree, "worktree", "w", false, "Run fixes in isolated worktree")
	reviewFixCmd.Flags().StringVarP(&reviewFixModel, "model", "m", "", "Model for fix agent")
	reviewCmd.AddCommand(reviewFixCmd)
}

func runReviewFix(_ *cobra.Command, args []string) error {
	projectDir, _ := os.Getwd()
	store, err := OpenReviewStore(projectDir)
	if err != nil {
		return fmt.Errorf("open store: %w", err)
	}
	defer func() { _ = store.Close() }()

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
		fmt.Println("No open reviews to fix.")
		return nil
	}

	for _, r := range reviews {
		if err := fixReview(store, r); err != nil {
			fmt.Printf("✗ Review #%d (%s): %v\n", r.ID, r.SHA[:8], err)
			continue
		}
		fmt.Printf("✓ Review #%d (%s) fixed\n", r.ID, r.SHA[:8])
	}
	return nil
}

func fixReview(store *ReviewStore, r *ReviewRecord) error {
	if len(r.Findings) == 0 {
		return nil
	}

	prompt := buildFixPrompt(r)

	// Invoke hawk exec with the fix prompt.
	execArgs := []string{"exec", "--auto", "full"}
	if reviewFixWorktree {
		execArgs = append(execArgs, "--worktree")
	}
	if reviewFixModel != "" {
		execArgs = append(execArgs, "--model", reviewFixModel)
	}
	execArgs = append(execArgs, prompt)

	hawkBin, err := os.Executable()
	if err != nil {
		hawkBin = "hawk"
	}

	cmd := exec.CommandContext(context.Background(), hawkBin, execArgs...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	cmd.Stdin = os.Stdin

	if err := cmd.Run(); err != nil {
		return fmt.Errorf("hawk exec: %w", err)
	}

	// Mark as fixed.
	return store.SetStatus(r.ID, ReviewStatusFixed)
}

func buildFixPrompt(r *ReviewRecord) string {
	var b strings.Builder
	b.WriteString("Fix the following code review findings from commit " + r.SHA[:8] + ":\n\n")

	for i, f := range r.Findings {
		b.WriteString(strconv.Itoa(i+1) + ". [" + f.Severity.String() + "] " + f.File + ":" + strconv.Itoa(f.Line) + "\n")
		b.WriteString("   " + f.Message + "\n")
		if f.Fix != "" {
			b.WriteString("   Suggested fix: " + f.Fix + "\n")
		}
		b.WriteString("\n")
	}

	b.WriteString("Apply all fixes, then commit with message 'fix: address review findings for " + r.SHA[:8] + "'.\n")
	b.WriteString("Do not introduce new issues. Keep changes minimal and focused.")
	return b.String()
}
