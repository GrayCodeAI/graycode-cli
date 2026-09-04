package cmd

import (
	"fmt"
	"os"
	"os/exec"
	"strings"

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

		// 1. Security event log chain integrity.
		dir := securitylog.DefaultDir()
		count, err := securitylog.Verify(dir)
		if err != nil {
			ok = false
			cmd.Printf("[FAIL] security event log: %v\n", err)
		} else {
			cmd.Printf("[OK]   security event log: %d entries verified (%s)\n", count, dir)
		}

		// 2. Managed governance policy validity (only when installed).
		policyPath := governance.ManagedPolicyPath()
		if _, statErr := os.Stat(policyPath); statErr != nil {
			cmd.Printf("[SKIP] governance policy: not installed (%s)\n", policyPath)
		} else if _, err := governance.LoadLayer("policy", policyPath); err != nil {
			ok = false
			cmd.Printf("[FAIL] governance policy: %v\n", err)
		} else {
			cmd.Printf("[OK]   governance policy: valid (%s)\n", policyPath)
		}

		// 3. Project test/verify checks discovered from the workspace.
		for _, c := range runWorkspaceChecks() {
			if c.Err != nil {
				ok = false
				cmd.Printf("[FAIL] %s: %v\n", c.Name, c.Err)
				continue
			}
			cmd.Printf("[OK]   %s: %s\n", c.Name, c.Detail)
		}

		if !ok {
			return fmt.Errorf("verification failed — see messages above")
		}
		cmd.Println("verification passed")
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
		run := exec.Command(c.Command[0], c.Command[1:]...) // #nosec G204 -- discovered from project manifests
		var stdout, stderr strings.Builder
		run.Stdout = &stdout
		run.Stderr = &stderr
		runErr := run.Run()
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
