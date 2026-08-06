package cmd

import (
	"fmt"
	"os"

	"github.com/GrayCodeAI/hawk/internal/governance"
	"github.com/GrayCodeAI/hawk/internal/securitylog"
	"github.com/spf13/cobra"
)

// verifyCmd runs local self-verification: integrity of the security event log
// and validity of the managed governance policy, if one is installed.
var verifyCmd = &cobra.Command{
	Use:   "verify",
	Short: "Run local self-verification (security log, governance policy)",
	Long: `Run hawk's self-verification checks without a model:
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

		if !ok {
			return fmt.Errorf("verification failed — see messages above")
		}
		cmd.Println("verification passed")
		return nil
	},
}

func init() {
	rootCmd.AddCommand(verifyCmd)
}
