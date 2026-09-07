package cmd

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"time"

	"github.com/GrayCodeAI/graycode-cli/internal/governance"
	"github.com/GrayCodeAI/graycode-cli/internal/securitylog"
	"github.com/GrayCodeAI/graycode-cli/internal/testrunner"
	"github.com/spf13/cobra"
)

// verifyCmd runs local self-verification: integrity of the security event log
// and validity of the managed governance policy, if one is installed.
var verifyCmd = &cobra.Command{
	Use:   "verify",
	Short: "Run local self-verification (security log, governance policy)",
	Long: `Run graycode's self-verification checks without a model:
  1. The tamper-evident security event log hash chain is intact.
  2. The managed governance policy (if installed) parses and validates.

Exits non-zero on the first failed check.`,
	RunE: func(cmd *cobra.Command, args []string) error {
		ok := true
		// Themed markers (padded to a fixed width so colorized output keeps
		// its column alignment; plain when piped via ShouldColor).
		okMark := auditTint("[OK]   ", doneGreen)
		failMark := auditTint("[FAIL] ", errorCoral)
		skipMark := auditTint("[SKIP] ", textMuted)

		// 1. Security event log chain integrity.
		dir := securitylog.DefaultDir()
		count, err := securitylog.Verify(dir)
		if err != nil {
			ok = false
			cmd.Printf("%ssecurity event log: %v\n", failMark, err)
		} else {
			cmd.Printf("%ssecurity event log: %d entries verified (%s)\n", okMark, count, dir)
		}

		// 2. Managed governance policy validity (only when installed).
		policyPath := governance.ManagedPolicyPath()
		if _, statErr := os.Stat(policyPath); statErr != nil {
			cmd.Printf("%sgovernance policy: not installed (%s)\n", skipMark, policyPath)
		} else if _, err := governance.LoadLayer("policy", policyPath); err != nil {
			ok = false
			cmd.Printf("%sgovernance policy: %v\n", failMark, err)
		} else {
			cmd.Printf("%sgovernance policy: valid (%s)\n", okMark, policyPath)
		}

		// 3. Project test/verify checks discovered from the workspace.
		// Running them can be slow (actual test/verify commands), so show a
		// TTY-only animated indicator while they execute; the structured
		// [OK]/[FAIL] results are printed only after the animation clears.
		checks, detectErr := testrunner.Detect(".")
		var prog *CLIProgress
		if detectErr == nil && len(checks) > 0 && stdoutIsTerminal() {
			prog = NewCLIProgress("Verify", []string{fmt.Sprintf("Running %d project checks", len(checks))})
			defer prog.Abort()
			prog.StartStep(0)
		}
		results := runWorkspaceChecks()
		if prog != nil {
			prog.CompleteStep(0)
			prog.Done()
		}
		for _, c := range results {
			if c.Err != nil {
				ok = false
				cmd.Printf("%s%s: %v\n", failMark, c.Name, c.Err)
				continue
			}
			cmd.Printf("%s%s: %s\n", okMark, c.Name, c.Detail)
		}

		if !ok {
			return fmt.Errorf("verification failed — see messages above")
		}
		cmd.Println(auditTint("verification passed", doneGreen))
		return nil
	},
}

// workspaceCheckResult is one discovered test/verify check's outcome.
type workspaceCheckResult struct {
	Name   string
	Detail string
	Err    error
}

// runWorkspaceChecks detects project test/verify commands via testrunner and
// runs them, parsing runner output into a structured summary. It is purely
// additive: a project without a detected runner yields no checks.
func runWorkspaceChecks() []workspaceCheckResult {
	checks, err := testrunner.Detect(".")
	if err != nil {
		return []workspaceCheckResult{{Name: "project checks", Err: fmt.Errorf("detect: %w", err)}}
	}
	var results []workspaceCheckResult
	for _, c := range checks {
		// Guard against a hanging project test/verify command: cap each check
		// at 10 minutes and report a timed-out check as a failure with a clear
		// message instead of blocking verify forever.
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
		run := exec.CommandContext(ctx, c.Command[0], c.Command[1:]...) // #nosec G204 -- discovered from project manifests
		var stdout, stderr strings.Builder
		run.Stdout = &stdout
		run.Stderr = &stderr
		runErr := run.Run()
		cancel()
		if ctx.Err() == context.DeadlineExceeded {
			results = append(results, workspaceCheckResult{Name: c.Name, Err: fmt.Errorf("timed out after 10m: %s", strings.TrimSpace(stderr.String()))})
			continue
		}
		summary := testrunner.ParseSummary(c, stdout.String(), stderr.String())
		if runErr != nil && summary == nil {
			results = append(results, workspaceCheckResult{Name: c.Name, Err: fmt.Errorf("%w: %s", runErr, strings.TrimSpace(stderr.String()))})
			continue
		}
		if summary != nil {
			detail := fmt.Sprintf("%d/%d passed (%d failed, %d skipped)", summary.Passed, summary.Total, summary.Failed, summary.Skipped)
			for _, f := range summary.Failures {
				if f.File != "" {
					detail += fmt.Sprintf("\n    %s: %s (%s)", f.Name, f.Message, f.File)
				} else {
					detail += fmt.Sprintf("\n    %s", f.Name)
				}
			}
			if runErr != nil {
				results = append(results, workspaceCheckResult{Name: c.Name, Detail: detail, Err: fmt.Errorf("%w", runErr)})
			} else {
				results = append(results, workspaceCheckResult{Name: c.Name, Detail: detail})
			}
			continue
		}
		if runErr != nil {
			results = append(results, workspaceCheckResult{Name: c.Name, Err: fmt.Errorf("%w", runErr)})
		} else {
			results = append(results, workspaceCheckResult{Name: c.Name, Detail: "ok"})
		}
	}
	return results
}

func init() {
	rootCmd.AddCommand(verifyCmd)
}
