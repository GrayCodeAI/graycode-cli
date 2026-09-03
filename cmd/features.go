package cmd

import (
	"fmt"
	"sort"
	"strings"

	"github.com/GrayCodeAI/graycode-cli/internal/feature"
	"github.com/spf13/cobra"
)

var featuresCmd = &cobra.Command{
	Use:   "features",
	Short: "List and manage feature flags",
	Long: `features lists all registered feature flags, their current values,
and how to override them via environment variables.

Feature flags allow runtime configuration of experimental or gated
capabilities without code changes or restarts (some changes may require
a daemon restart).

Override a flag via environment variable:
    GRAYCODE_FEATURE_<FLAG_NAME>=1 graycode daemon start

Show a specific flag:
    graycode features get <flag-name>`,
	RunE: func(cmd *cobra.Command, args []string) error {
		if len(args) > 0 && args[0] == "get" {
			if len(args) < 2 {
				return fmt.Errorf("usage: graycode features get <flag-name>")
			}
			f, ok := feature.Info(args[1])
			if !ok {
				return fmt.Errorf("unknown feature flag: %s", args[1])
			}
			fmt.Printf("Name:        %s\n", f.Name())
			fmt.Printf("Default:     %v\n", f.DefaultValue())
			fmt.Printf("Current:     %v\n", feature.EnabledByName(args[1]))
			fmt.Printf("Description: %s\n", f.Description())
			envVar := "GRAYCODE_FEATURE_" + strings.ReplaceAll(strings.ToUpper(args[1]), "-", "_")
			fmt.Printf("Env var:     %s\n", envVar)
			return nil
		}

		flags := feature.List()
		names := make([]string, 0, len(flags))
		for name := range flags {
			names = append(names, name)
		}
		sort.Strings(names)

		fmt.Println("Feature Flags:")
		fmt.Println()
		for _, name := range names {
			f, _ := feature.Info(name)
			val := flags[name]
			status := "DISABLED"
			if val {
				status = "ENABLED"
			}
			fmt.Printf("  %s = %v  [%s]\n", name, val, status)
			if f != nil {
				fmt.Printf("    default: %v\n", f.DefaultValue())
				fmt.Printf("    description: %s\n", f.Description())
				envVar := "GRAYCODE_FEATURE_" + strings.ReplaceAll(strings.ToUpper(name), "-", "_")
				fmt.Printf("    env: %s\n", envVar)
			}
			fmt.Println()
		}
		return nil
	},
}
