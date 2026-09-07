package cmd

import (
	"encoding/json"
	"fmt"
	"image/color"
	"os"
	"path/filepath"
	"time"

	"github.com/GrayCodeAI/graycode-cli/internal/plugin"
	"github.com/GrayCodeAI/graycode-cli/internal/theme"
	"github.com/spf13/cobra"
)

var dynamicManager *plugin.DynamicPluginManager

// pluginStateColor maps a plugin lifecycle state to a theme color.
func pluginStateColor(state plugin.PluginState) color.Color {
	switch state {
	case plugin.StateActive:
		return doneGreen
	case plugin.StateFailed:
		return errorCoral
	case plugin.StateDisabled:
		return textDisabled
	default: // discovered, loaded
		return infoSky
	}
}

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
		cmd.Printf("%s\n", auditTint("Plugin "+name+" activated.", doneGreen))
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
		cmd.Printf("%s\n", auditTint("Plugin "+name+" deactivated.", textPrimary))
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
		cmd.Printf("%s\n", auditTint("Plugin "+name+" reloaded.", textPrimary))
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
			cmd.Println(auditTint("No plugins discovered. Run 'graycode plugin install' to add plugins.", textMuted))
			return nil
		}

		jsonOut, _ := cmd.Flags().GetBool("json")
		if jsonOut {
			data, err := json.MarshalIndent(statuses, "", "  ")
			if err != nil {
				return fmt.Errorf("marshaling plugin statuses: %w", err)
			}
			cmd.Println(string(data))
			return nil
		}

		rows := make([][]string, 0, len(statuses))
		for _, s := range statuses {
			rows = append(rows, []string{
				s.Name, s.Version,
				auditTint(string(s.State), pluginStateColor(s.State)),
				fmt.Sprintf("%d", s.ToolCount), fmt.Sprintf("%d", s.HookCount),
			})
		}
		return theme.PrintTable(cmd.OutOrStdout(), []string{"NAME", "VERSION", "STATE", "TOOLS", "HOOKS"}, rows)
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
			cmd.Printf("%s\n", auditTint("Installed plugin from "+source+".", doneGreen))
			return nil
		}

		// Otherwise treat as GitHub repo
		dm := getDynamicManager()
		prog := NewCLIProgress("Plugin install", []string{"Installing plugin from GitHub"})
		prog.StartStep(0)
		if err := dm.InstallFromGitHub(source); err != nil {
			prog.Abort()
			return err
		}
		prog.CompleteStep(0)
		prog.Done()
		cmd.Printf("%s\n", auditTint("Installed plugin from "+source+".", doneGreen))

		// Re-discover
		_ = dm.DiscoverAll()
		return nil
	},
}

var pluginListJSON bool

var pluginListCmd = &cobra.Command{
	Use:   "list",
	Short: "List installed plugins",
	Args:  cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		if pluginListJSON {
			plugins, err := plugin.List()
			if err != nil {
				return err
			}
			data, err := json.MarshalIndent(plugins, "", "  ")
			if err != nil {
				return fmt.Errorf("marshaling plugins: %w", err)
			}
			fmt.Println(string(data))
			return nil
		}
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
		cmd.Printf("%s\n", auditTint("Plugin "+name+" uninstalled.", textPrimary))
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
			Description: fmt.Sprintf("A graycode plugin: %s", name),
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

A graycode plugin.

## Installation

`+"```bash"+`
graycode plugin install ./%s
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

		cmd.Printf("%s\n", auditTint("Created multi-component plugin scaffold at ./"+name+"/", doneGreen))
		cmd.Printf("%s\n", auditTint("  "+name+"/plugin.json  - Plugin manifest", textMuted))
		cmd.Printf("%s\n", auditTint("  "+name+"/main.go      - Plugin entrypoint", textMuted))
		cmd.Printf("%s\n", auditTint("  "+name+"/skills/      - Bundled skills", textMuted))
		cmd.Printf("%s\n", auditTint("  "+name+"/hooks/       - Hook scripts", textMuted))
		cmd.Printf("%s\n", auditTint("  "+name+"/tools/       - Tool binaries", textMuted))
		cmd.Printf("%s\n", auditTint("  "+name+"/mcp.json     - MCP server specs", textMuted))
		cmd.Printf("%s\n", auditTint("  "+name+"/README.md    - Documentation", textMuted))
		cmd.Println()
		cmd.Printf("%s\n", auditTint("Next steps:", textPrimary))
		cmd.Printf("%s\n", auditTint("  cd "+name+" && go mod init "+name, textMuted))
		cmd.Printf("%s\n", auditTint("  graycode plugin install ./"+name, textMuted))
		cmd.Printf("%s\n", auditTint("  graycode plugin activate "+name, textMuted))
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
						cmd.Printf("%s\n", auditTint("Plugin: ", textMuted)+auditTint(s.Name, textPrimary))
						cmd.Printf("%s\n", auditTint("State:  ", textMuted)+auditTint(string(s.State), pluginStateColor(s.State)))
						if s.Error != "" {
							cmd.Printf("%s\n", auditTint("Error:  ", textMuted)+auditTint(s.Error, errorCoral))
						}
						if !s.ActivatedAt.IsZero() {
							cmd.Printf("%s\n", auditTint("Activated: ", textMuted)+auditTint(s.ActivatedAt.Format(time.RFC3339), textPrimary))
						}
						return nil
					}
				}
				return fmt.Errorf("plugin %q not found", name)
			}
			cmd.Println(auditTint("No recent plugin events.", textMuted))
			return nil
		}

		rows := make([][]string, 0, len(collected))
		for _, ev := range collected {
			errStr := ""
			if ev.Error != "" {
				errStr = truncateWithEllipsis(ev.Error, 50)
			}
			rows = append(rows, []string{
				ev.Timestamp.Format("15:04:05"),
				ev.PluginName,
				ev.Type,
				errStr,
			})
		}
		return theme.PrintTable(cmd.OutOrStdout(), []string{"TIME", "PLUGIN", "EVENT", "ERROR"}, rows)
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
		prog := NewCLIProgress("Marketplace", []string{"Fetching marketplace indexes"})
		prog.StartStep(0)
		entries, err := mc.FetchAll()
		if err != nil {
			prog.Abort()
			return fmt.Errorf("fetch marketplace: %w (indexes may be unpublished; add a source with graycode plugin marketplace add)", err)
		}
		prog.CompleteStep(0)
		prog.Done()
		if len(entries) == 0 {
			cmd.Println(auditTint("No marketplace plugins found.", textMuted))
			cmd.Println(auditTint("Add a source: graycode plugin marketplace add <name> <index-url>", textMuted))
			return nil
		}
		rows := make([][]string, 0, len(entries))
		for _, e := range entries {
			desc := e.Description
			if len(desc) > 48 {
				// Rune-safe truncation: never split a multibyte UTF-8 sequence.
				if runes := []rune(desc); len(runes) > 48 {
					desc = string(runes[:45]) + "..."
				}
			}
			rows = append(rows, []string{e.Name, e.Repo, e.Version, desc})
		}
		return theme.PrintTable(cmd.OutOrStdout(), []string{"NAME", "REPO", "VERSION", "DESCRIPTION"}, rows)
	},
}

var pluginMarketplaceInstallCmd = &cobra.Command{
	Use:   "install <name>",
	Short: "Install a plugin by marketplace name",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		mc := plugin.NewMarketplaceClient()
		prog := NewCLIProgress("Plugin install", []string{"Fetching plugin", "Installing plugin"})
		prog.StartStep(0)
		entry, err := mc.Find(args[0])
		if err != nil {
			prog.Abort()
			return err
		}
		prog.CompleteStep(0)
		prog.StartStep(1)
		dir, err := mc.Install(*entry)
		if err != nil {
			prog.Abort()
			return err
		}
		prog.CompleteStep(1)
		prog.Done()
		cmd.Printf("%s\n", auditTint("Installed "+entry.Name+" to ", doneGreen)+auditTint(dir, textPrimary))
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
		cmd.Printf("%s\n", auditTint("Added marketplace source "+args[0]+" → ", doneGreen)+auditTint(args[1], textPrimary))
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
		cmd.Printf("%s\n", auditTint("Root: ", textMuted)+auditTint(comp.Root, textPrimary))
		cmd.Printf("%s\n", auditTint("Components: ", textMuted)+auditTint(comp.ComponentSummary(), textPrimary))
		cmd.Printf("%s\n", auditTint("Tools: ", textMuted)+auditTint(fmt.Sprintf("%v", comp.HasTools), textPrimary))
		cmd.Printf("%s\n", auditTint(fmt.Sprintf("Skills (%d):", len(comp.Skills)), textPrimary))
		for _, s := range comp.Skills {
			cmd.Printf("%s\n", auditTint("  - "+s, textMuted))
		}
		cmd.Printf("%s\n", auditTint(fmt.Sprintf("Hooks (%d):", len(comp.HookFiles)), textPrimary))
		for _, h := range comp.HookFiles {
			cmd.Printf("%s\n", auditTint("  - "+h, textMuted))
		}
		cmd.Printf("%s\n", auditTint(fmt.Sprintf("MCP servers (%d):", len(comp.MCPServers)), textPrimary))
		for _, m := range comp.MCPServers {
			cmd.Printf("%s\n", auditTint(fmt.Sprintf("  - %s cmd=%s url=%s", m.Name, m.Command, m.URL), textMuted))
		}
		return nil
	},
}

func init() {
	pluginStatusCmd.Flags().Bool("json", false, "output as JSON")
	pluginListCmd.Flags().BoolVar(&pluginListJSON, "json", false, "output plugins as JSON")

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
