package cmd

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/GrayCodeAI/hawk/eval"
	"github.com/spf13/cobra"
)

var (
	evalTasks   string
	evalModel   string
	evalTags    string
	evalNoCache bool
	evalOutput  string
	evalTaskDir string
)

var evalCmd = &cobra.Command{
	Use:   "eval",
	Short: "Evaluate model performance on coding benchmarks",
}

var evalRunCmd = &cobra.Command{
	Use:   "run",
	Short: "Run evaluation tasks",
	RunE:  runEval,
}

var evalListCmd = &cobra.Command{
	Use:   "list",
	Short: "List available evaluation tasks",
	RunE:  runEvalList,
}

var evalResultsCmd = &cobra.Command{
	Use:   "results",
	Short: "Show past evaluation results",
	RunE:  runEvalResults,
}

var evalCacheCmd = &cobra.Command{
	Use:   "cache-clear",
	Short: "Clear the evaluation cache",
	RunE: func(_ *cobra.Command, _ []string) error {
		cache := eval.DefaultCache()
		if err := cache.Clear(); err != nil {
			return err
		}
		fmt.Println("Cache cleared.")
		return nil
	},
}

func init() {
	evalRunCmd.Flags().StringVar(&evalTasks, "tasks", "", "Comma-separated task IDs (default: all)")
	evalRunCmd.Flags().StringVar(&evalModel, "model", "", "Model to evaluate")
	evalRunCmd.Flags().StringVar(&evalTags, "tags", "", "Filter tasks by tags")
	evalRunCmd.Flags().BoolVar(&evalNoCache, "no-cache", false, "Disable result caching")
	evalRunCmd.Flags().StringVarP(&evalOutput, "output", "o", "markdown", "Output format: markdown, json")
	evalRunCmd.Flags().StringVar(&evalTaskDir, "task-dir", "", "Directory with YAML task definitions")

	evalCmd.AddCommand(evalRunCmd)
	evalCmd.AddCommand(evalListCmd)
	evalCmd.AddCommand(evalResultsCmd)
	evalCmd.AddCommand(evalCacheCmd)
}

func runEval(_ *cobra.Command, _ []string) error {
	// Load tasks
	goSuite := eval.GoTasks()
	tasks := goSuite.Tasks

	// Load YAML tasks if directory specified
	if evalTaskDir != "" {
		yamlTasks, err := eval.LoadTasksFromYAML(evalTaskDir)
		if err != nil {
			return fmt.Errorf("loading YAML tasks: %w", err)
		}
		tasks = append(tasks, yamlTasks...)
	}

	// Filter by task IDs
	if evalTasks != "" {
		ids := strings.Split(evalTasks, ",")
		idSet := make(map[string]bool)
		for _, id := range ids {
			idSet[strings.TrimSpace(id)] = true
		}
		var filtered []eval.BenchmarkTask
		for _, t := range tasks {
			if idSet[t.ID] {
				filtered = append(filtered, t)
			}
		}
		tasks = filtered
	}

	// Filter by tags
	if evalTags != "" {
		tags := strings.Split(evalTags, ",")
		tagSet := make(map[string]bool)
		for _, t := range tags {
			tagSet[strings.TrimSpace(t)] = true
		}
		var filtered []eval.BenchmarkTask
		for _, t := range tasks {
			for _, tag := range t.Tags {
				if tagSet[tag] {
					filtered = append(filtered, t)
					break
				}
			}
		}
		tasks = filtered
	}

	if len(tasks) == 0 {
		return fmt.Errorf("no tasks matched the given filters")
	}

	model := evalModel
	if model == "" {
		model = "default"
	}

	fmt.Printf("Running %d tasks with model %s...\n", len(tasks), model)

	suite := &eval.BenchmarkSuite{Name: "hawk-eval", Tasks: tasks}
	runner := eval.NewRunner(model, "")
	runner.NoCache = evalNoCache
	if !evalNoCache {
		runner.Cache = eval.DefaultCache()
	}
	runner.Filters = []eval.Filter{eval.ExtractCodeBlock("go")}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Minute)
	defer cancel()

	result, err := runner.Run(ctx, suite)
	if err != nil {
		return err
	}

	// Compute reproducibility hash
	hash := eval.ComputeHash(tasks)

	// Save results
	store := eval.DefaultResultStore()
	path, err := store.Save(result, model, "", hash)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Warning: failed to save results: %v\n", err)
	} else {
		fmt.Printf("Results saved to: %s\n", path)
	}

	// Group results
	groups := eval.GroupTasks(tasks, eval.DefaultGroups())
	groupResults := eval.AggregateGroupResults(groups, result.Results)

	// Output
	switch evalOutput {
	case "json":
		type jsonOutput struct {
			*eval.SuiteResult
			Groups []eval.GroupResult `json:"groups,omitempty"`
		}
		out := jsonOutput{SuiteResult: result, Groups: groupResults}
		data, _ := json.MarshalIndent(out, "", "  ")
		fmt.Println(string(data))
	default:
		fmt.Println(eval.GenerateReport(result))
		if len(groupResults) > 0 {
			fmt.Println("## Group Results")
			fmt.Println("| Group | Pass Rate |")
			fmt.Println("|-------|-----------|")
			for _, gr := range groupResults {
				if gr.Total > 0 {
					fmt.Printf("| %s | %.0f%% (%d/%d) |\n", gr.Name, gr.PassRate*100, gr.Passed, gr.Total)
				}
			}
		}
	}

	return nil
}

func runEvalList(_ *cobra.Command, _ []string) error {
	suite := eval.GoTasks()
	tasks := suite.Tasks

	if evalTaskDir != "" {
		yamlTasks, err := eval.LoadTasksFromYAML(evalTaskDir)
		if err != nil {
			return err
		}
		tasks = append(tasks, yamlTasks...)
	}

	fmt.Printf("Available tasks (%d):\n\n", len(tasks))
	fmt.Println("| ID | Description | Tags |")
	fmt.Println("|----|-------------|------|")
	for _, t := range tasks {
		tags := strings.Join(t.Tags, ", ")
		desc := t.Description
		if len(desc) > 50 {
			desc = desc[:50] + "..."
		}
		fmt.Printf("| %s | %s | %s |\n", t.ID, desc, tags)
	}
	return nil
}

func runEvalResults(_ *cobra.Command, _ []string) error {
	store := eval.DefaultResultStore()
	files, err := store.List()
	if err != nil {
		return err
	}
	if len(files) == 0 {
		fmt.Println("No saved results found.")
		return nil
	}
	fmt.Printf("Saved results (%d):\n\n", len(files))
	for _, f := range files {
		r, err := store.Load(f)
		if err != nil {
			continue
		}
		fmt.Printf("  %s  %s  %s  %.0f%% (%d/%d)\n",
			r.Timestamp.Format("2006-01-02 15:04"),
			r.Model, r.Suite,
			r.Summary.PassRate*100, r.Summary.Passed, r.Summary.TotalTasks)
	}
	return nil
}
