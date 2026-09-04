package cmd

import (
	"encoding/json"
	"fmt"
	"os"
	"strconv"
	"strings"

	"github.com/GrayCodeAI/graycode-cli/internal/permissions"
	"github.com/GrayCodeAI/graycode-cli/internal/permissions/stableid"
	"github.com/spf13/cobra"
)

var (
	permissionsJSON  bool
	permissionsScope string
)

type permissionRuleOutput struct {
	ID         uint64 `json:"id"`
	Kind       string `json:"kind"`
	Identity   string `json:"identity"`
	Decision   string `json:"decision"`
	Generation uint64 `json:"generation"`
}

var permissionsCmd = &cobra.Command{
	Use:   "permissions",
	Short: "List and manage exact permission rules",
}

var permissionsListCmd = &cobra.Command{
	Use:   "list",
	Short: "List persisted exact permission rules",
	RunE: func(cmd *cobra.Command, _ []string) error {
		store := currentStableRuleStore()
		if err := store.Load(); err != nil {
			return err
		}
		rules := store.List()
		out := make([]permissionRuleOutput, 0, len(rules))
		for _, rule := range rules {
			out = append(out, permissionRuleOutput{
				ID: rule.ID, Kind: rule.Key.Kind.String(), Identity: rule.DisplayIdentity,
				Decision: rule.Decision.String(), Generation: rule.Generation,
			})
		}
		if permissionsJSON {
			data, err := json.MarshalIndent(out, "", "  ")
			if err != nil {
				return err
			}
			_, err = cmd.OutOrStdout().Write(append(data, '\n'))
			return err
		}
		if len(out) == 0 {
			cmd.Println("No persisted permission rules.")
			return nil
		}
		for _, rule := range out {
			cmd.Printf("%d\t%s\t%s\t%s\t%d\n", rule.ID, rule.Decision, rule.Kind, rule.Identity, rule.Generation)
		}
		return nil
	},
}

var permissionsAddCmd = &cobra.Command{
	Use:   "add <allow|deny> <command|file|tool> <identity>",
	Short: "Persist an exact permission rule",
	Args:  cobra.ExactArgs(3),
	RunE: func(cmd *cobra.Command, args []string) error {
		decision, err := parsePermissionDecision(args[0])
		if err != nil {
			return err
		}
		kind, err := parsePermissionKind(args[1])
		if err != nil {
			return err
		}
		identity := strings.TrimSpace(args[2])
		if identity == "" {
			return fmt.Errorf("permission identity cannot be empty")
		}
		store := currentStableRuleStore()
		if err := store.Load(); err != nil {
			return err
		}
		id, ok := store.Remember(kind, identity, identity, decision)
		if !ok {
			return fmt.Errorf("could not add permission rule")
		}
		if err := store.Save(); err != nil {
			return err
		}
		cmd.Printf("Permission rule %d saved.\n", id)
		return nil
	},
}

var permissionsRevokeCmd = &cobra.Command{
	Use:   "revoke <rule-id>",
	Short: "Revoke a persisted exact permission rule",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		id, err := strconv.ParseUint(args[0], 10, 64)
		if err != nil || id == 0 {
			return fmt.Errorf("invalid permission rule ID %q", args[0])
		}
		store := currentStableRuleStore()
		if err := store.Load(); err != nil {
			return err
		}
		if !store.Revoke(id) {
			return fmt.Errorf("permission rule %d not found", id)
		}
		if err := store.Save(); err != nil {
			return err
		}
		cmd.Printf("Permission rule %d revoked.\n", id)
		return nil
	},
}

var permissionsResetCmd = &cobra.Command{
	Use:   "reset",
	Short: "Remove all persisted exact permission rules",
	RunE: func(cmd *cobra.Command, _ []string) error {
		if permissionsScope != "" && permissionsScope != "project" {
			return fmt.Errorf("unsupported permission scope %q", permissionsScope)
		}
		store := currentStableRuleStore()
		if err := store.Load(); err != nil {
			return err
		}
		if !store.Reset() {
			cmd.Println("No persisted permission rules.")
			return nil
		}
		if err := store.Save(); err != nil {
			return err
		}
		cmd.Println("Persisted permission rules reset.")
		return nil
	},
}

func currentStableRuleStore() *permissions.StableRuleStore {
	projectDir, err := os.Getwd()
	if err != nil {
		projectDir = "."
	}
	return permissions.NewStableRuleStore(permissions.DefaultStableRulesPath(projectDir))
}

func parsePermissionDecision(raw string) (stableid.Decision, error) {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case "allow":
		return stableid.Allow, nil
	case "deny":
		return stableid.Deny, nil
	default:
		return stableid.Deny, fmt.Errorf("permission decision must be allow or deny")
	}
}

func parsePermissionKind(raw string) (stableid.Kind, error) {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case "command", "bash":
		return stableid.KindCommand, nil
	case "file", "file_mutation", "edit", "write":
		return stableid.KindFileMutation, nil
	case "tool", "structured_tool":
		return stableid.KindStructuredTool, nil
	default:
		return stableid.KindCommand, fmt.Errorf("permission kind must be command, file, or tool")
	}
}

func init() {
	permissionsListCmd.Flags().BoolVar(&permissionsJSON, "json", false, "output rules as JSON")
	permissionsResetCmd.Flags().StringVar(&permissionsScope, "scope", "project", "rule scope to reset")
	permissionsCmd.AddCommand(permissionsListCmd, permissionsAddCmd, permissionsRevokeCmd, permissionsResetCmd)
	rootCmd.AddCommand(permissionsCmd)
}
