package cmd

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"text/tabwriter"
	"time"

	"github.com/GrayCodeAI/hawk/internal/plugin"
	"github.com/spf13/cobra"
)

var dynamicManager *plugin.DynamicPluginManager

func getDynamicManager() *plugin.DynamicPluginManager {
	if dynamicManager == nil {
		dynamicManager = plugin.NewDynamicPluginManager(nil, nil, nil)
		_ = dynamicManager.DiscoverAll()
	}
	return dynamicManager
}

var pluginActivateCmd = &cobra.Command{
	Use:   "activate <name>",
	Short: "Activate a discovered plugin",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		dm := getDynamicManager()
		name := args[0]
		if err := dm.Activate(name); err != nil {
			return fmt.Errorf("activate plugin %q: %w", name, err)
		}
		cmd.Printf("Plugin %q activated.\n", name)
		return nil
	},
}

var pluginDeactivateCmd = &cobra.Command{
	Use:   "deactivate <name>",
	Short: "Deactivate an active plugin",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		dm := getDynamicManager()
		name := args[0]
		if err := dm.Deactivate(name); err != nil {
			return fmt.Errorf("deactivate plugin %q: %w", name, err)
		}
		cmd.Printf("Plugin %q deactivated.\n", name)
		return nil
	},
}

var pluginReloadCmd = &cobra.Command{
	Use:   "reload <name>",
	Short: "Reload a plugin (deactivate then activate)",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		dm := getDynamicManager()
		name := args[0]
		if err := dm.Reload(name); err != nil {
			return fmt.Errorf("reload plugin %q: %w", name, err)
		}
		cmd.Printf("Plugin %q reloaded.\n", name)
		return nil
	},
}

var pluginStatusCmd = &cobra.Command{
	Use:   "status",
	Short: "Show all plugins with their state",
	RunE: func(cmd *cobra.Command, args []string) error {
		dm := getDynamicManager()
		statuses := dm.Status()

		if len(statuses) == 0 {
			cmd.Println("No plugins discovered. Run 'hawk plugin install' to add plugins.")
			return nil
		}

		jsonOut, _ := cmd.Flags().GetBool("json")
		if jsonOut {
			data, _ := json.MarshalIndent(statuses, "", "  ")
			cmd.Println(string(data))
			return nil
		}

		w := tabwriter.NewWriter(cmd.OutOrStdout(), 0, 0, 2, ' ', 0)
		_, _ = fmt.Fprintf(w, "NAME\tVERSION\tSTATE\tTOOLS\tHOOKS\n")
		for _, s := range statuses {
			_, _ = fmt.Fprintf(w, "%s\t%s\t%s\t%d\t%d\n",
				s.Name, s.Version, s.State, s.ToolCount, s.HookCount)
		}
		_ = w.Flush()

		return nil
	},
}

var pluginInstallDynamicCmd = &cobra.Command{
	Use:   "install <repo-or-dir>",
	Short: "Install a plugin from GitHub or local directory",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		source := args[0]

		// Check if it is a local directory
		if info, err := os.Stat(source); err == nil && info.IsDir() {
			if err := plugin.Install(source); err != nil {
				return err
			}
			cmd.Printf("Installed plugin from %s.\n", source)
			return nil
		}

		// Otherwise treat as GitHub repo
		dm := getDynamicManager()
		if err := dm.InstallFromGitHub(source); err != nil {
			return err
		}
		cmd.Printf("Installed plugin from %s.\n", source)

		// Re-discover
		_ = dm.DiscoverAll()
		return nil
	},
}

var pluginListCmd = &cobra.Command{
	Use:   "list",
	Short: "List installed plugins",
	Args:  cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		cmd.Println(plugin.Summary())
		return nil
	},
}

var pluginUninstallCmd = &cobra.Command{
	Use:   "uninstall <name>",
	Short: "Uninstall a plugin (deactivate and remove from disk)",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		dm := getDynamicManager()
		name := args[0]
		if err := dm.Uninstall(name); err != nil {
			return err
		}
		cmd.Printf("Plugin %q uninstalled.\n", name)
		return nil
	},
}

var pluginCreateCmd = &cobra.Command{
	Use:   "create <name>",
	Short: "Scaffold a new plugin in the current directory",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		name := args[0]
		dir := filepath.Join(".", name)

		if _, err := os.Stat(dir); err == nil {
			return fmt.Errorf("directory %q already exists", dir)
		}

		if err := os.MkdirAll(dir, 0o750); err != nil {
			return fmt.Errorf("create directory: %w", err)
		}

		// Write plugin.json manifest
		manifest := &plugin.ManifestV2{
			Name:        name,
			Version:     "0.1.0",
			Description: fmt.Sprintf("A hawk plugin: %s", name),
			Author:      "",
			Mode:        "subprocess",
			Tools: []plugin.ManifestTool{
				{
					Name:        "hello",
					Description: fmt.Sprintf("Example tool from %s plugin", name),
					Command:     "go run .",
					InputSchema: map[string]interface{}{
						"type": "object",
						"properties": map[string]interface{}{
							"message": map[string]interface{}{
								"type":        "string",
								"description": "Input message",
							},
						},
					},
				},
			},
			Permissions: []string{},
			License:     "MIT",
		}

		if err := plugin.WriteManifestV2(dir, manifest); err != nil {
			return fmt.Errorf("write manifest: %w", err)
		}

		// Write main.go
		mainGo := fmt.Sprintf(`package main

import (
	"encoding/json"
	"fmt"
	"os"
)

// Input represents the tool input passed via stdin.
type Input struct {
	Message string `+"`"+`json:"message"`+"`"+`
}

// Output represents the tool response written to stdout.
type Output struct {
	Result string `+"`"+`json:"result"`+"`"+`
}

func main() {
	var input Input
	if err := json.NewDecoder(os.Stdin).Decode(&input); err != nil {
		_, _ = fmt.Fprintf(os.Stderr, "error reading input: %%v\n", err)
		os.Exit(1)
	}

	output := Output{
		Result: fmt.Sprintf("Hello from %s! You said: %%s", input.Message),
	}

	if err := json.NewEncoder(os.Stdout).Encode(output); err != nil {
		_, _ = fmt.Fprintf(os.Stderr, "error writing output: %%v\n", err)
		os.Exit(1)
	}
}
`, name)

		// #nosec G306 -- scaffolded project source file, intended to be normally readable/editable like any repo file
		if err := os.WriteFile(filepath.Join(dir, "main.go"), []byte(mainGo), 0o644); err != nil {
			return fmt.Errorf("write main.go: %w", err)
		}

		// Write README.md
		readme := fmt.Sprintf(`# %s

A hawk plugin.

## Installation

`+"```bash"+`
hawk plugin install ./%s
`+"```"+`

## Usage

Once installed and activated, the plugin provides the following tools:

- **hello** - Example tool that echoes input

## Development

Run the plugin locally:

`+"```bash"+`
echo '{"message": "world"}' | go run .
`+"```"+`

## Plugin Manifest

See `+"`plugin.json`"+` for the full manifest configuration.
`, name, name)

		// #nosec G306 -- scaffolded project doc file, intended to be normally readable/editable like any repo file
		if err := os.WriteFile(filepath.Join(dir, "README.md"), []byte(readme), 0o644); err != nil {
			return fmt.Errorf("write README.md: %w", err)
		}

		// Multi-component package skeleton (PACK-05)
		for _, sub := range []string{"skills", "hooks", "tools"} {
			if err := os.MkdirAll(filepath.Join(dir, sub), 0o750); err != nil {
				return fmt.Errorf("create %s: %w", sub, err)
			}
		}
		skillExample := filepath.Join(dir, "skills", "example")
		if err := os.MkdirAll(skillExample, 0o750); err != nil {
			return err
		}
		// #nosec G306
		_ = os.WriteFile(filepath.Join(skillExample, "SKILL.md"), []byte("---\nname: example\ndescription: Example skill from plugin\n---\n\n# Example skill\n"), 0o644)
		// #nosec G306
		_ = os.WriteFile(filepath.Join(dir, "mcp.json"), []byte("{\n  \"servers\": []\n}\n"), 0o644)

		cmd.Printf("Created multi-component plugin scaffold at ./%s/\n", name)
		cmd.Printf("  %s/plugin.json  - Plugin manifest\n", name)
		cmd.Printf("  %s/main.go      - Plugin entrypoint\n", name)
		cmd.Printf("  %s/skills/      - Bundled skills\n", name)
		cmd.Printf("  %s/hooks/       - Hook scripts\n", name)
		cmd.Printf("  %s/tools/       - Tool binaries\n", name)
		cmd.Printf("  %s/mcp.json     - MCP server specs\n", name)
		cmd.Printf("  %s/README.md    - Documentation\n", name)
		cmd.Println()
		cmd.Printf("Next steps:\n")
		cmd.Printf("  cd %s && go mod init %s\n", name, name)
		cmd.Printf("  hawk plugin install ./%s\n", name)
		cmd.Printf("  hawk plugin activate %s\n", name)
		return nil
	},
}

var pluginLogsCmd = &cobra.Command{
	Use:   "logs [name]",
	Short: "Show recent plugin lifecycle events",
	RunE: func(cmd *cobra.Command, args []string) error {
		dm := getDynamicManager()
		events := dm.Events()

		// Collect recent events (non-blocking drain)
		var collected []plugin.PluginEvent
		for {
			select {
			case ev := <-events:
				if len(args) == 0 || ev.PluginName == args[0] {
					collected = append(collected, ev)
				}
			default:
				goto done
			}
		}
	done:

		if len(collected) == 0 {
			// Show current status as fallback
			statuses := dm.Status()
			if len(args) > 0 {
				name := args[0]
				for _, s := range statuses {
					if s.Name == name {
						cmd.Printf("Plugin: %s\n", s.Name)
						cmd.Printf("State:  %s\n", s.State)
						if s.Error != "" {
							cmd.Printf("Error:  %s\n", s.Error)
						}
						if !s.ActivatedAt.IsZero() {
							cmd.Printf("Activated: %s\n", s.ActivatedAt.Format(time.RFC3339))
						}
						return nil
					}
				}
				return fmt.Errorf("plugin %q not found", name)
			}
			cmd.Println("No recent plugin events.")
			return nil
		}

		w := tabwriter.NewWriter(cmd.OutOrStdout(), 0, 0, 2, ' ', 0)
		_, _ = fmt.Fprintf(w, "TIME\tPLUGIN\tEVENT\tERROR\n")
		for _, ev := range collected {
			errStr := ""
			if ev.Error != "" {
				errStr = truncateWithEllipsis(ev.Error, 50)
			}
			_, _ = fmt.Fprintf(
				w, "%s\t%s\t%s\t%s\n",
				ev.Timestamp.Format("15:04:05"),
				ev.PluginName,
				ev.Type,
				errStr,
			)
		}
		_ = w.Flush()
		return nil
	},
}

var pluginMarketplaceCmd = &cobra.Command{
	Use:   "marketplace",
	Short: "Browse and install plugins from marketplace sources",
}

var pluginMarketplaceListCmd = &cobra.Command{
	Use:   "list",
	Short: "List plugins available from marketplace sources",
	RunE: func(cmd *cobra.Command, _ []string) error {
		mc := plugin.NewMarketplaceClient()
		entries, err := mc.FetchAll()
		if err != nil {
			return fmt.Errorf("fetch marketplace: %w (indexes may be unpublished; add a source with hawk plugin marketplace add)", err)
		}
		if len(entries) == 0 {
			cmd.Println("No marketplace plugins found.")
			cmd.Println("Add a source: hawk plugin marketplace add <name> <index-url>")
			return nil
		}
		w := tabwriter.NewWriter(cmd.OutOrStdout(), 0, 0, 2, ' ', 0)
		_, _ = fmt.Fprintf(w, "NAME\tREPO\tVERSION\tDESCRIPTION\n")
		for _, e := range entries {
			desc := e.Description
			if len(desc) > 48 {
				// Rune-safe truncation: never split a multibyte UTF-8 sequence.
				if runes := []rune(desc); len(runes) > 48 {
					desc = string(runes[:45]) + "..."
				}
			}
			_, _ = fmt.Fprintf(w, "%s\t%s\t%s\t%s\n", e.Name, e.Repo, e.Version, desc)
		}
		return w.Flush()
	},
}

var pluginMarketplaceInstallCmd = &cobra.Command{
	Use:   "install <name>",
	Short: "Install a plugin by marketplace name",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		mc := plugin.NewMarketplaceClient()
		entry, err := mc.Find(args[0])
		if err != nil {
			return err
		}
		dir, err := mc.Install(*entry)
		if err != nil {
			return err
		}
		cmd.Printf("Installed %s to %s\n", entry.Name, dir)
		// re-discover
		_ = getDynamicManager().DiscoverAll()
		return nil
	},
}

var pluginMarketplaceAddCmd = &cobra.Command{
	Use:   "add <name> <index-url>",
	Short: "Add a marketplace source (JSON index URL)",
	Args:  cobra.ExactArgs(2),
	RunE: func(cmd *cobra.Command, args []string) error {
		if err := plugin.AddSource(args[0], args[1]); err != nil {
			return err
		}
		cmd.Printf("Added marketplace source %q → %s\n", args[0], args[1])
		return nil
	},
}

var pluginMarketplaceSourcesCmd = &cobra.Command{
	Use:   "sources",
	Short: "List configured marketplace sources",
	RunE: func(cmd *cobra.Command, _ []string) error {
		mc := plugin.NewMarketplaceClient()
		for _, s := range mc.Sources {
			cmd.Printf("%s\t%s\n", s.Name, s.URL)
		}
		return nil
	},
}

var pluginInspectCmd = &cobra.Command{
	Use:   "inspect <dir-or-name>",
	Short: "Show multi-component package contents (tools/hooks/skills/mcp)",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		dir := args[0]
		if st, err := os.Stat(dir); err != nil || !st.IsDir() {
			fromState := filepath.Join(plugin.PluginsStateDir(), dir)
			if st, err := os.Stat(fromState); err == nil && st.IsDir() {
				dir = fromState
			} else {
				return fmt.Errorf("plugin directory not found: %s", args[0])
			}
		}
		comp, err := plugin.DiscoverComponents(dir)
		if err != nil {
			return err
		}
		cmd.Printf("Root: %s\n", comp.Root)
		cmd.Printf("Components: %s\n", comp.ComponentSummary())
		cmd.Printf("Tools: %v\n", comp.HasTools)
		cmd.Printf("Skills (%d):\n", len(comp.Skills))
		for _, s := range comp.Skills {
			cmd.Printf("  - %s\n", s)
		}
		cmd.Printf("Hooks (%d):\n", len(comp.HookFiles))
		for _, h := range comp.HookFiles {
			cmd.Printf("  - %s\n", h)
		}
		cmd.Printf("MCP servers (%d):\n", len(comp.MCPServers))
		for _, m := range comp.MCPServers {
			cmd.Printf("  - %s cmd=%s url=%s\n", m.Name, m.Command, m.URL)
		}
		return nil
	},
}

func init() {
	pluginStatusCmd.Flags().Bool("json", false, "output as JSON")

	pluginMarketplaceCmd.AddCommand(pluginMarketplaceListCmd)
	pluginMarketplaceCmd.AddCommand(pluginMarketplaceInstallCmd)
	pluginMarketplaceCmd.AddCommand(pluginMarketplaceAddCmd)
	pluginMarketplaceCmd.AddCommand(pluginMarketplaceSourcesCmd)

	pluginCmd.AddCommand(pluginListCmd)
	pluginCmd.AddCommand(pluginActivateCmd)
	pluginCmd.AddCommand(pluginDeactivateCmd)
	pluginCmd.AddCommand(pluginReloadCmd)
	pluginCmd.AddCommand(pluginStatusCmd)
	pluginCmd.AddCommand(pluginCreateCmd)
	pluginCmd.AddCommand(pluginInstallDynamicCmd)
	pluginCmd.AddCommand(pluginUninstallCmd)
	pluginCmd.AddCommand(pluginLogsCmd)
	pluginCmd.AddCommand(pluginMarketplaceCmd)
	pluginCmd.AddCommand(pluginInspectCmd)
}

// pluginInstallDynamicCmd overrides the default "install" subcommand behavior.
// The original pluginCmd handles "install" as args[0], but now it's also
// a proper subcommand. Cobra handles this gracefully since subcommands take
// priority over args-based dispatching.
