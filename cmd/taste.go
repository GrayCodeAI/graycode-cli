package cmd

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/GrayCodeAI/graycode-cli/internal/feature/taste"
	"github.com/spf13/cobra"
)

var (
	tasteProjectID string
	tasteFile      string
)

var tasteCmd = &cobra.Command{
	Use:   "taste",
	Short: "Manage taste profile (learned coding style preferences)",
	Long: `The taste system learns your coding preferences over time by observing
how you edit, accept, or reject agent-generated code. Use these commands
to view, export, import, and manage your taste profile.`,
}

var tasteShowCmd = &cobra.Command{
	Use:   "show",
	Short: "Display current taste preferences with confidence scores",
	RunE:  runTasteShow,
}

var tastePushCmd = &cobra.Command{
	Use:   "push [file]",
	Short: "Export taste profile to a file for sharing",
	Long: `Export your current taste profile to a JSON file that can be shared
with teammates or used across machines.

Examples:
  graycode taste push                    # Export to stdout
  graycode taste push --file team.json   # Export to file`,
	RunE: runTastePush,
}

var tastePullCmd = &cobra.Command{
	Use:   "pull <file>",
	Short: "Import a taste profile from a file",
	Long: `Import a taste profile from a JSON file. The imported preferences
will be merged with your existing profile (not replaced).

Examples:
  graycode taste pull team.json
  cat team.json | graycode taste pull -`,
	Args: cobra.ExactArgs(1),
	RunE: runTastePull,
}

var tasteResetCmd = &cobra.Command{
	Use:   "reset",
	Short: "Clear all taste preferences",
	Long:  `Remove all learned preferences. This cannot be undone.`,
	RunE:  runTasteReset,
}

func init() {
	tasteCmd.PersistentFlags().StringVar(&tasteProjectID, "project", "", "project ID (defaults to current directory name)")
	tastePushCmd.Flags().StringVar(&tasteFile, "file", "", "output file path (defaults to stdout)")

	tasteCmd.AddCommand(tasteShowCmd)
	tasteCmd.AddCommand(tastePushCmd)
	tasteCmd.AddCommand(tastePullCmd)
	tasteCmd.AddCommand(tasteResetCmd)
	rootCmd.AddCommand(tasteCmd)
}

func getProjectID() string {
	if tasteProjectID != "" {
		return tasteProjectID
	}
	cwd, err := os.Getwd()
	if err != nil {
		return "default"
	}
	return filepath.Base(cwd)
}

func getTasteStore() (*taste.Store, error) {
	return taste.NewStore("")
}

func runTasteShow(_ *cobra.Command, _ []string) error {
	store, err := getTasteStore()
	if err != nil {
		return fmt.Errorf("open taste store: %w", err)
	}

	projectID := getProjectID()
	profile, err := store.Load(projectID)
	if err != nil {
		return fmt.Errorf("load profile: %w", err)
	}

	fmt.Println(profile.Summary())

	// Also show prompt context if anything is learned.
	ctx := profile.ToPromptContext()
	if ctx != "" {
		fmt.Println("\nSystem prompt fragment that would be injected:")
		fmt.Println(strings.Repeat("-", 50))
		fmt.Println(ctx)
	}

	return nil
}

func runTastePush(_ *cobra.Command, _ []string) error {
	store, err := getTasteStore()
	if err != nil {
		return fmt.Errorf("open taste store: %w", err)
	}

	projectID := getProjectID()
	data, err := store.Export(projectID)
	if err != nil {
		return fmt.Errorf("export profile: %w", err)
	}

	if tasteFile != "" {
		if err := os.WriteFile(tasteFile, data, 0o600); err != nil {
			return fmt.Errorf("write file: %w", err)
		}
		fmt.Printf("Taste profile exported to %s\n", tasteFile)
	} else {
		fmt.Println(string(data))
	}

	return nil
}

func runTastePull(_ *cobra.Command, args []string) error {
	store, err := getTasteStore()
	if err != nil {
		return fmt.Errorf("open taste store: %w", err)
	}

	var data []byte
	if args[0] == "-" {
		data, err = readStdin()
		if err != nil {
			return fmt.Errorf("read stdin: %w", err)
		}
	} else {
		data, err = os.ReadFile(args[0])
		if err != nil {
			return fmt.Errorf("read file: %w", err)
		}
	}

	if err := store.Import(data); err != nil {
		return fmt.Errorf("import profile: %w", err)
	}

	fmt.Println("Taste profile imported successfully.")
	return nil
}

func runTasteReset(_ *cobra.Command, _ []string) error {
	store, err := getTasteStore()
	if err != nil {
		return fmt.Errorf("open taste store: %w", err)
	}

	projectID := getProjectID()
	if err := store.Delete(projectID); err != nil {
		return fmt.Errorf("reset profile: %w", err)
	}

	fmt.Printf("Taste profile for %q has been reset.\n", projectID)
	return nil
}

func readStdin() ([]byte, error) {
	var buf []byte
	tmp := make([]byte, 4096)
	for {
		n, err := os.Stdin.Read(tmp)
		if n > 0 {
			buf = append(buf, tmp[:n]...)
		}
		if err != nil {
			break
		}
	}
	return buf, nil
}
