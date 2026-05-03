package cmd

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/GrayCodeAI/hawk/planner"
	"github.com/spf13/cobra"
)

var planCmd = &cobra.Command{
	Use:   "plan",
	Short: "Create and manage structured development plans",
	Long: `plan helps you create, list, and track structured development plans.
Plans are stored as JSON files in .hawk/plans/.

Subcommands:
  create <description>   Create a new plan (generates a plan prompt)
  list                   List all saved plans
  show <name>            Display a plan in markdown format
  done <task-id>         Mark a task as completed`,
}

var planCreateCmd = &cobra.Command{
	Use:   "create <description>",
	Short: "Create a new plan from a feature description",
	Args:  cobra.MinimumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		description := strings.Join(args, " ")

		// Generate the plan prompt (would normally be sent to an LLM).
		prompt := planner.Generate(description, "")
		cmd.Println("Plan prompt generated. Send this to an LLM to produce a plan:")
		cmd.Println("--- System ---")
		cmd.Println(prompt.System)
		cmd.Println()
		cmd.Println("--- User ---")
		cmd.Println(prompt.User)
		cmd.Println()
		cmd.Println("Once you have the LLM response, save it as a JSON file in .hawk/plans/.")
		return nil
	},
}

var planListCmd = &cobra.Command{
	Use:   "list",
	Short: "List all saved plans",
	RunE: func(cmd *cobra.Command, args []string) error {
		plansDir := filepath.Join(".", ".hawk", "plans")
		entries, err := os.ReadDir(plansDir)
		if err != nil {
			if os.IsNotExist(err) {
				cmd.Println("No plans found. Create one with: hawk plan create <description>")
				return nil
			}
			return fmt.Errorf("read plans directory: %w", err)
		}

		found := false
		for _, e := range entries {
			if e.IsDir() || filepath.Ext(e.Name()) != ".json" {
				continue
			}
			found = true

			path := filepath.Join(plansDir, e.Name())
			plan, err := planner.Load(path)
			if err != nil {
				cmd.Println(fmt.Sprintf("  [error] %s: %v", e.Name(), err))
				continue
			}

			pending := len(planner.PendingTasks(plan))
			total := len(plan.Tasks)
			done := total - pending
			cmd.Println(fmt.Sprintf("  %s  [%d/%d done]  %s",
				strings.TrimSuffix(e.Name(), ".json"),
				done, total,
				plan.Title,
			))
		}

		if !found {
			cmd.Println("No plans found. Create one with: hawk plan create <description>")
		}

		return nil
	},
}

var planShowCmd = &cobra.Command{
	Use:   "show <name>",
	Short: "Display a plan in markdown format",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		name := args[0]
		path := resolvePlanPath(name)

		plan, err := planner.Load(path)
		if err != nil {
			return fmt.Errorf("load plan %q: %w", name, err)
		}

		fmt.Print(planner.FormatMarkdown(plan))
		return nil
	},
}

var planDoneCmd = &cobra.Command{
	Use:   "done <task-id>",
	Short: "Mark a task as completed in the most recent plan",
	Long: `Mark a task as done by its numeric ID. This operates on the most
recently modified plan in .hawk/plans/.

To target a specific plan, set the plan name as the first argument
followed by the task ID: hawk plan done <name> <task-id>`,
	Args: cobra.RangeArgs(1, 2),
	RunE: func(cmd *cobra.Command, args []string) error {
		var planPath string
		var taskIDStr string

		if len(args) == 2 {
			planPath = resolvePlanPath(args[0])
			taskIDStr = args[1]
		} else {
			taskIDStr = args[0]
			// Find the most recently modified plan.
			path, err := mostRecentPlan()
			if err != nil {
				return err
			}
			planPath = path
		}

		taskID, err := strconv.Atoi(taskIDStr)
		if err != nil {
			return fmt.Errorf("invalid task ID: %s", taskIDStr)
		}

		plan, err := planner.Load(planPath)
		if err != nil {
			return fmt.Errorf("load plan: %w", err)
		}

		planner.MarkDone(plan, taskID)

		// Save the updated plan back.
		data, err := json.MarshalIndent(plan, "", "  ")
		if err != nil {
			return fmt.Errorf("marshal plan: %w", err)
		}
		if err := os.WriteFile(planPath, data, 0o644); err != nil {
			return fmt.Errorf("write plan: %w", err)
		}

		cmd.Println(fmt.Sprintf("Task %d marked as done.", taskID))
		return nil
	},
}

func init() {
	planCmd.AddCommand(planCreateCmd)
	planCmd.AddCommand(planListCmd)
	planCmd.AddCommand(planShowCmd)
	planCmd.AddCommand(planDoneCmd)
}

// resolvePlanPath converts a plan name to a file path in .hawk/plans/.
func resolvePlanPath(name string) string {
	// If the name already has a .json extension, use it directly.
	if strings.HasSuffix(name, ".json") {
		return filepath.Join(".", ".hawk", "plans", name)
	}
	return filepath.Join(".", ".hawk", "plans", name+".json")
}

// mostRecentPlan returns the path to the most recently modified plan.
func mostRecentPlan() (string, error) {
	plansDir := filepath.Join(".", ".hawk", "plans")
	entries, err := os.ReadDir(plansDir)
	if err != nil {
		return "", fmt.Errorf("no plans directory found: %w", err)
	}

	var newest string
	var newestTime int64

	for _, e := range entries {
		if e.IsDir() || filepath.Ext(e.Name()) != ".json" {
			continue
		}
		info, err := e.Info()
		if err != nil {
			continue
		}
		if info.ModTime().UnixNano() > newestTime {
			newestTime = info.ModTime().UnixNano()
			newest = filepath.Join(plansDir, e.Name())
		}
	}

	if newest == "" {
		return "", fmt.Errorf("no plans found in .hawk/plans/")
	}
	return newest, nil
}
