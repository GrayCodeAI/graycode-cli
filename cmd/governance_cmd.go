package cmd

import (
	"fmt"
	"os"
	"sort"
	"strings"

	"github.com/GrayCodeAI/graycode-cli/internal/governance"
	"github.com/spf13/cobra"
)

var governancePath string

// governanceCmd exposes the POLICY ∩ PROFILE permission ceiling.
var governanceCmd = &cobra.Command{
	Use:   "governance",
	Short: "Inspect and validate the governance policy ceiling",
	Long: `Governance is the administrator-set POLICY ceiling layered under the
per-session PROFILE (tightest-wins). Tools are permitted only when both
layers allow them.

  graycode governance                    Show the managed policy status
  graycode governance show               Print the effective capability rows
  graycode governance validate <file>    Validate a policy or profile document
  graycode governance explain <tool>     Evaluate a tool against the policy`,
	RunE: func(cmd *cobra.Command, args []string) error {
		return runGovernanceStatus(cmd)
	},
}

var governanceShowCmd = &cobra.Command{
	Use:   "show",
	Short: "Print the effective policy capability rows",
	RunE: func(cmd *cobra.Command, args []string) error {
		layer, err := governanceLayerForCLI()
		if err != nil {
			return err
		}
		path := governancePath
		if path == "" {
			path = governance.ManagedPolicyPath()
		}
		cmd.Printf("Governance layer %q (%s)\n", layer.Name, path)
		cmd.Printf("Fail-closed: %t\n", layer.FailClosed)
		if len(layer.DeniedTools) > 0 {
			cmd.Printf("Denied tools: %s\n", sortedKeys(layer.DeniedTools))
		}
		if len(layer.DeniedBash) > 0 {
			cmd.Printf("Denied bash patterns: %s\n", strings.Join(layer.DeniedBash, ", "))
		}
		if len(layer.SensitivePaths) > 0 {
			cmd.Printf("Sensitive paths: %s\n", strings.Join(layer.SensitivePaths, ", "))
		}
		if len(layer.Capabilities) == 0 {
			cmd.Println("No capability rows.")
			return nil
		}
		cmd.Println("\nCapabilities:")
		for _, cap := range layer.Capabilities {
			pattern := cap.Pattern
			if pattern == "" {
				pattern = "*"
			}
			reason := ""
			if cap.Reason != "" {
				reason = " (" + cap.Reason + ")"
			}
			cmd.Printf("  %-8s %-20s %-12s %s\n", cap.Action, cap.Scope, pattern, reason)
		}
		return nil
	},
}

var governanceValidateCmd = &cobra.Command{
	Use:   "validate <file>",
	Short: "Validate a governance policy or profile document",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		layer, err := governance.LoadLayer("policy", args[0])
		if err != nil {
			return err
		}
		cmd.Printf("valid: %d capability row(s), fail_closed=%t (%s)\n",
			len(layer.Capabilities), layer.FailClosed, args[0])
		return nil
	},
}

var governanceExplainCmd = &cobra.Command{
	Use:   "explain <tool> [summary]",
	Short: "Evaluate a tool call against the policy and show the decision",
	Args:  cobra.MinimumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		toolName := args[0]
		summary := strings.Join(args[1:], " ")

		layer, err := governanceLayerForCLI()
		if err != nil {
			return err
		}
		eng := governance.New()
		eng.SetPolicy(layer)

		dec := eng.Evaluate(toolName, summary)
		scopes := governance.ScopesForTool(toolName)
		scoped := "(ungoverned scope)"
		if len(scopes) > 0 {
			scoped = strings.Join(scopeNames(scopes), ", ")
		}
		verdict := "DENY"
		if dec.Allowed {
			verdict = "ALLOW"
		}
		cmd.Printf("tool:       %s\n", toolName)
		cmd.Printf("scopes:     %s\n", scoped)
		if summary != "" {
			cmd.Printf("summary:    %s\n", summary)
		}
		cmd.Printf("decision:   %s\n", verdict)
		cmd.Printf("source:     %s\n", dec.Source)
		if dec.Scope != "" {
			cmd.Printf("scope hit:  %s\n", dec.Scope)
		}
		if dec.Rule != "" {
			cmd.Printf("rule:       %s\n", dec.Rule)
		}
		if dec.Reason != "" {
			cmd.Printf("reason:     %s\n", dec.Reason)
		}
		return nil
	},
}

func init() {
	governanceShowCmd.Flags().StringVar(&governancePath, "path", "", "policy file to inspect (default: managed policy path)")
	governanceExplainCmd.Flags().StringVar(&governancePath, "path", "", "policy file to evaluate against (default: managed policy path)")
	governanceCmd.AddCommand(governanceShowCmd)
	governanceCmd.AddCommand(governanceValidateCmd)
	governanceCmd.AddCommand(governanceExplainCmd)
	rootCmd.AddCommand(governanceCmd)
}

func runGovernanceStatus(cmd *cobra.Command) error {
	path := governance.ManagedPolicyPath()
	cmd.Printf("Managed policy path: %s\n", path)
	if _, err := os.Stat(path); err != nil {
		cmd.Println("Status: not installed (governance is fail-open; no ceiling enforced)")
		return nil
	}
	layer, err := governance.LoadLayer("policy", path)
	if err != nil {
		return fmt.Errorf("managed policy is invalid: %w", err)
	}
	cmd.Printf("Status: installed — fail_closed=%t, %d capability row(s), %d denied tool(s)\n",
		layer.FailClosed, len(layer.Capabilities), len(layer.DeniedTools))
	return nil
}

func governanceLayerForCLI() (*governance.Layer, error) {
	path := governancePath
	if path == "" {
		path = governance.ManagedPolicyPath()
	}
	if _, err := os.Stat(path); err != nil {
		if os.IsNotExist(err) {
			return nil, fmt.Errorf("no governance policy at %s; use --path to point at a policy file", path)
		}
		return nil, err
	}
	return governance.LoadLayer("policy", path)
}

func sortedKeys(m map[string]struct{}) string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return strings.Join(keys, ", ")
}

func scopeNames(scopes []governance.ScopeName) []string {
	names := make([]string, len(scopes))
	for i, s := range scopes {
		names[i] = string(s)
	}
	return names
}
