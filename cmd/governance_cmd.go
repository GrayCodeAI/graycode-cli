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
		cmd.Printf("%s %s\n", auditTint("Governance layer", textMuted), auditTint(fmt.Sprintf("%q (%s)", layer.Name, path), textPrimary))
		cmd.Printf("%s %t\n", auditTint("Fail-closed:", textMuted), layer.FailClosed)
		if len(layer.DeniedTools) > 0 {
			cmd.Printf("%s %s\n", auditTint("Denied tools:", textMuted), auditTint(sortedKeys(layer.DeniedTools), textPrimary))
		}
		if len(layer.DeniedBash) > 0 {
			cmd.Printf("%s %s\n", auditTint("Denied bash patterns:", textMuted), auditTint(strings.Join(layer.DeniedBash, ", "), textPrimary))
		}
		if len(layer.SensitivePaths) > 0 {
			cmd.Printf("%s %s\n", auditTint("Sensitive paths:", textMuted), auditTint(strings.Join(layer.SensitivePaths, ", "), textPrimary))
		}
		if len(layer.Capabilities) == 0 {
			cmd.Println(auditTint("No capability rows.", textMuted))
			return nil
		}
		cmd.Println(auditTint("\nCapabilities:", infoSky))
		for _, cap := range layer.Capabilities {
			pattern := cap.Pattern
			if pattern == "" {
				pattern = "*"
			}
			reason := ""
			if cap.Reason != "" {
				reason = " (" + cap.Reason + ")"
			}
			actionColor := doneGreen
			if cap.Action == governance.ActionDeny {
				actionColor = errorCoral
			}
			cmd.Printf("  %s %-20s %-12s %s\n",
				auditTint(fmt.Sprintf("%-8s", cap.Action), actionColor),
				auditTint(string(cap.Scope), textPrimary),
				auditTint(pattern, textMuted),
				auditTint(reason, textMuted))
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
		cmd.Printf("%s %d capability row(s), fail_closed=%t (%s)\n",
			auditTint("valid:", doneGreen),
			len(layer.Capabilities), layer.FailClosed, auditTint(args[0], textMuted))
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
		verdictColor := errorCoral
		if dec.Allowed {
			verdict = "ALLOW"
			verdictColor = doneGreen
		}
		cmd.Printf("%s %s\n", auditTint("tool:", textMuted), auditTint(toolName, textPrimary))
		cmd.Printf("%s %s\n", auditTint("scopes:", textMuted), auditTint(scoped, textPrimary))
		if summary != "" {
			cmd.Printf("%s %s\n", auditTint("summary:", textMuted), auditTint(summary, textPrimary))
		}
		cmd.Printf("%s %s\n", auditTint("decision:", textMuted), auditTint(verdict, verdictColor))
		cmd.Printf("%s %s\n", auditTint("source:", textMuted), auditTint(dec.Source, textPrimary))
		if dec.Scope != "" {
			cmd.Printf("%s %s\n", auditTint("scope hit:", textMuted), auditTint(string(dec.Scope), textPrimary))
		}
		if dec.Rule != "" {
			cmd.Printf("%s %s\n", auditTint("rule:", textMuted), auditTint(dec.Rule, textPrimary))
		}
		if dec.Reason != "" {
			cmd.Printf("%s %s\n", auditTint("reason:", textMuted), auditTint(dec.Reason, textPrimary))
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
