package cmd

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"strings"

	"github.com/spf13/cobra"
)

var (
	issueTitle  string
	issueBody   string
	issueAssign string
	issueLabels []string
	issueDryRun bool
	issueJSON   bool
)

var issueCmd = &cobra.Command{
	Use:   "issue [context]",
	Short: "Draft or publish a GitHub issue (fx issue parity)",
	Long: `Draft or publish a GitHub issue for the current repository, mirroring
fx's "issue" command.

A title and body are generated from the optional <context> — describe a
problem, paste a stack trace, or leave it empty. Publishing creates the issue
through the GitHub CLI ("gh"), so that must be installed and authenticated.

Use --dry-run to preview the title and body without publishing anything.`,
	Args: cobra.MaximumNArgs(1),
	RunE: runIssue,
}

func init() {
	issueCmd.Flags().StringVar(&issueTitle, "title", "", "issue title (default: generated from context)")
	issueCmd.Flags().StringVar(&issueBody, "body", "", "issue body (default: generated from context)")
	issueCmd.Flags().StringVar(&issueAssign, "assign", "", "add an assignee")
	issueCmd.Flags().StringSliceVar(&issueLabels, "label", nil, "apply a label (repeatable)")
	issueCmd.Flags().BoolVar(&issueDryRun, "dry-run", false, "preview the issue without publishing")
	issueCmd.Flags().BoolVar(&issueJSON, "json", false, "output the draft as JSON (requires --dry-run)")
	rootCmd.AddCommand(issueCmd)
}

func runIssue(cmd *cobra.Command, args []string) error {
	ctx := strings.TrimSpace(strings.Join(args, " "))

	title := issueTitle
	if title == "" {
		title = generateIssueTitle(ctx)
	}
	body := issueBody
	if body == "" {
		body = generateIssueBody(ctx)
	}

	if issueDryRun {
		out := struct {
			Title    string   `json:"title"`
			Body     string   `json:"body"`
			Assignee string   `json:"assignee,omitempty"`
			Labels   []string `json:"labels,omitempty"`
		}{Title: title, Body: body, Assignee: issueAssign, Labels: issueLabels}
		if issueJSON {
			raw, err := json.MarshalIndent(out, "", "  ")
			if err != nil {
				return fmt.Errorf("issue: marshal json: %w", err)
			}
			_, _ = cmd.OutOrStdout().Write(raw)
			_, _ = fmt.Fprintln(cmd.OutOrStdout())
			return nil
		}
		cmd.Println("Issue preview (dry run — not published)")
		cmd.Println("Title: " + title)
		cmd.Println()
		cmd.Print(body)
		if len(issueLabels) > 0 {
			cmd.Println()
			cmd.Println("Labels: " + strings.Join(issueLabels, ", "))
		}
		return nil
	}

	if err := requireGH(); err != nil {
		return err
	}

	ghArgs := []string{"issue", "create", "--title", title, "--body", body}
	if issueAssign != "" {
		ghArgs = append(ghArgs, "--assignee", issueAssign)
	}
	for _, l := range issueLabels {
		ghArgs = append(ghArgs, "--label", l)
	}

	cc := exec.CommandContext(context.Background(), "gh", ghArgs...) // #nosec G204 -- fixed command 'gh' with args; title/body are data arguments, not the executable
	cc.Stderr = os.Stderr
	out, err := cc.Output()
	if err != nil {
		return fmt.Errorf("gh issue create failed: %w", err)
	}
	cmd.Println("Issue created: " + strings.TrimSpace(string(out)))
	return nil
}

// generateIssueTitle derives a short title from the supplied context.
func generateIssueTitle(ctx string) string {
	first := ctx
	if i := strings.IndexByte(first, '\n'); i >= 0 {
		first = first[:i]
	}
	first = strings.TrimSpace(first)
	if first == "" {
		return "Untitled report"
	}
	for _, c := range []string{"#", "##", "###", ">", "-", "*"} {
		first = strings.TrimPrefix(first, c)
	}
	return strings.TrimSpace(first)
}

// generateIssueBody wraps the context in a fenced block so trace text is
// preserved verbatim.
func generateIssueBody(ctx string) string {
	if ctx == "" {
		return "No additional context was provided."
	}
	return "**Reported via hawk**\n\n```\n" + ctx + "\n```\n"
}
