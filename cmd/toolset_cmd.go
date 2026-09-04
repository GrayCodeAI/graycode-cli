package cmd

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/GrayCodeAI/graycode-cli/internal/toolset"
	"github.com/spf13/cobra"
)

// toolsetCmd exposes named, composable tool groups (adopted from Hermes
// Agent's toolset system) as a CLI surface: list the available groups or
// resolve one to its concrete, de-duplicated, sorted tool list.
var toolsetCmd = &cobra.Command{
	Use:   "toolset [name]",
	Short: "List or resolve composable tool groups",
	Long: `Named, composable tool groups for scoping an agent's tool surface.

  graycode toolset            List available toolsets
  graycode toolset research   Resolve 'research' to its concrete tool list

Toolsets compose from other toolsets; resolving expands Requires
transitively (cycle-safe) and de-duplicates.`,
	RunE: func(cmd *cobra.Command, args []string) error {
		reg, err := toolset.NewRegistry(toolset.Defaults())
		if err != nil {
			return err
		}
		if len(args) == 0 {
			fmt.Println("Available toolsets: " + strings.Join(reg.Names(), ", "))
			return nil
		}
		name := args[0]
		tools, err := reg.Resolve(name)
		if err != nil {
			return err
		}
		payload := map[string]interface{}{"toolset": name, "tools": tools, "count": len(tools)}
		out, _ := json.MarshalIndent(payload, "", "  ")
		fmt.Println(string(out))
		return nil
	},
}

func init() {
	rootCmd.AddCommand(toolsetCmd)
}
