package cmd

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"time"

	reviewcontracts "github.com/GrayCodeAI/eagle/review"
	hawkKestrel "github.com/GrayCodeAI/hawk/internal/bridge/kestrel"
	hawkconfig "github.com/GrayCodeAI/hawk/internal/config"
	"github.com/GrayCodeAI/hawk/internal/engine"
	"github.com/GrayCodeAI/hawk/internal/ui/icons"
	kestrelLib "github.com/GrayCodeAI/kestrel"
	"github.com/spf13/cobra"
)

var (
	analyzeModel   string
	analyzeFix     bool
	analyzeWait    bool
	analyzeTimeout time.Duration
)

var reviewAnalyzeCmd = &cobra.Command{
	Use:   "analyze <type> [files...]",
	Short: "Run targeted code analysis",
	Long: `Run on-demand analysis across your codebase.

Types:
  security       Find security vulnerabilities
  duplication    Detect duplicated code
  complexity     Flag overly complex functions
  dead-code      Find unused exports and dead code
  refactor       Suggest refactoring opportunities
  test-fixtures  Find test helper opportunities

Examples:
  hawk review analyze security ./...
  hawk review analyze complexity --fix main.go
  hawk review analyze duplication ./internal/...`,
	Args: cobra.MinimumNArgs(1),
	RunE: runReviewAnalyze,
}

func init() {
	reviewAnalyzeCmd.Flags().StringVarP(&analyzeModel, "model", "m", "", "LLM model for analysis")
	reviewAnalyzeCmd.Flags().BoolVar(&analyzeFix, "fix", false, "Auto-fix findings immediately")
	reviewAnalyzeCmd.Flags().BoolVar(&analyzeWait, "wait", false, "Wait and display results (don't background)")
	reviewAnalyzeCmd.Flags().DurationVar(&analyzeTimeout, "timeout", 3*time.Minute, "Analysis timeout")
	reviewCmd.AddCommand(reviewAnalyzeCmd)
}

var analysisPrompts = map[string]string{
	"security": `Analyze the following code for security vulnerabilities:
- Injection flaws (SQL, command, path traversal)
- Authentication/authorization issues
- Sensitive data exposure
- Insecure cryptography
- Missing input validation
Report each finding with file, line, severity, description, and fix.`,

	"duplication": `Analyze the following code for duplication:
- Repeated logic that could be extracted into shared functions
- Copy-pasted blocks with minor variations
- Similar patterns that could use generics or interfaces
Report each finding with the duplicated locations and a refactoring suggestion.`,

	"complexity": `Analyze the following code for excessive complexity:
- Functions with high cyclomatic complexity (>10)
- Deeply nested conditionals (>3 levels)
- Functions longer than 50 lines
- God objects or functions doing too many things
Report each finding with file, line, complexity metric, and simplification suggestion.`,

	"dead-code": `Analyze the following code for dead code:
- Unused exported functions/types
- Unreachable code paths
- Commented-out code blocks
- Unused variables and imports
Report each finding with file, line, and what can be safely removed.`,

	"refactor": `Analyze the following code for refactoring opportunities:
- Functions that violate single responsibility
- Missing error handling patterns
- Opportunities for better abstractions
- Interface segregation improvements
Report each finding with file, line, current issue, and suggested refactoring.`,

	"test-fixtures": `Analyze the following test code for improvement opportunities:
- Repeated test setup that could be shared fixtures
- Missing table-driven test patterns
- Test helpers that could reduce boilerplate
- Missing edge case coverage
Report each finding with file, line, and suggested improvement.`,
}

func runReviewAnalyze(_ *cobra.Command, args []string) error {
	analysisType := args[0]
	prompt, ok := analysisPrompts[analysisType]
	if !ok {
		valid := make([]string, 0, len(analysisPrompts))
		for k := range analysisPrompts {
			valid = append(valid, k)
		}
		return fmt.Errorf("unknown analysis type %q; valid types: %s", analysisType, strings.Join(valid, ", "))
	}

	// Collect files to analyze.
	files := args[1:]
	if len(files) == 0 {
		files = []string{"./..."}
	}

	// Get file contents via git ls-files + read.
	content, err := getAnalysisContent(files)
	if err != nil {
		return fmt.Errorf("gather files: %w", err)
	}
	if content == "" {
		fmt.Println("No files matched.")
		return nil
	}

	// Build the Kestrel bridge through Hawk's Eyrie engine boundary.
	ctx := context.Background()
	selection := hawkconfig.EffectiveSelection(ctx, hawkconfig.SelectionOptions{
		ProviderOverride: strings.TrimSpace(provider),
		ModelOverride:    strings.TrimSpace(analyzeModel),
	})
	chatProvider, providerID, err := engine.BuildChatProvider(ctx, selection, strings.TrimSpace(provider))
	if err != nil {
		return fmt.Errorf("resolve engine transport: %w", err)
	}

	var opts []kestrelLib.Option
	if analyzeModel != "" {
		opts = append(opts, kestrelLib.WithModel(analyzeModel))
	}
	opts = append(opts, kestrelLib.WithConcerns(analysisType))

	bridge := hawkKestrel.NewBridge(chatProvider, providerID, opts...)
	if !bridge.Ready() {
		return fmt.Errorf("kestrel bridge not ready (check API key)")
	}

	if analyzeTimeout > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, analyzeTimeout)
		defer cancel()
	}

	// Use the analysis prompt as a "diff" — kestrel will review it.
	analysisInput := fmt.Sprintf("# Analysis Type: %s\n\n%s\n\n---\n\n%s", analysisType, prompt, content)

	fmt.Printf("Analyzing (%s)...\n", analysisType)
	result, err := bridge.ReviewContracts(ctx, analysisInput)
	if err != nil {
		return fmt.Errorf("analysis failed: %w", err)
	}

	// Store as a review record.
	projectDir, _ := os.Getwd()
	store, err := OpenReviewStore(projectDir)
	if err == nil {
		defer func() { _ = store.Close() }()
		sha := fmt.Sprintf("analyze-%s-%d", analysisType, time.Now().Unix())
		id, _ := store.Create(sha)
		status := ReviewStatusPassed
		if len(result.Findings) > 0 {
			status = ReviewStatusOpen
		}
		_ = store.Update(id, status, result)
	}

	// Print results.
	if len(result.Findings) == 0 {
		fmt.Printf("%s No %s issues found.\n", icons.CheckBold(), analysisType)
		return nil
	}

	fmt.Printf("%s %d %s finding(s):\n\n", icons.Alert(), len(result.Findings), analysisType)
	for i, f := range result.Findings {
		sev := severityStyle(f.Severity.String())
		fmt.Printf("  %d. %s %s:%d\n", i+1, sev, f.File, f.Line)
		fmt.Printf("     %s\n", f.Message)
		if f.Fix != "" {
			fmt.Printf("     Fix: %s\n", f.Fix)
		}
		fmt.Println()
	}

	// Auto-fix if requested.
	if analyzeFix && len(result.Findings) > 0 {
		fmt.Println("Applying fixes...")
		return autoFixAnalysis(result)
	}

	return nil
}

func getAnalysisContent(patterns []string) (string, error) {
	// Use git ls-files to expand patterns, then read files.
	args := append([]string{"ls-files", "--"}, patterns...)
	out, err := exec.CommandContext(context.Background(), "git", args...).Output() // #nosec G204 -- fixed command 'git' with args, not user-controlled binary
	if err != nil {
		// Fallback: treat patterns as literal file paths.
		var b strings.Builder
		for _, p := range patterns {
			data, err := os.ReadFile(p) // #nosec G304 -- p is from repo-relative pattern list, not external input
			if err != nil {
				continue
			}
			b.WriteString(fmt.Sprintf("--- %s ---\n%s\n\n", p, string(data)))
		}
		return b.String(), nil
	}

	files := strings.Split(strings.TrimSpace(string(out)), "\n")
	var b strings.Builder
	maxFiles := 30 // Cap to avoid token explosion.
	for i, f := range files {
		if i >= maxFiles || f == "" {
			break
		}
		data, err := os.ReadFile(f) // #nosec G304 -- f is from git ls-files output, repo-relative path
		if err != nil {
			continue
		}
		b.WriteString(fmt.Sprintf("--- %s ---\n%s\n\n", f, string(data)))
	}
	return b.String(), nil
}

func autoFixAnalysis(result *reviewcontracts.Result) error {
	var b strings.Builder
	b.WriteString("Fix the following analysis findings:\n\n")
	for i, f := range result.Findings {
		b.WriteString(fmt.Sprintf("%d. [%s] %s:%d — %s\n", i+1, f.Severity, f.File, f.Line, f.Message))
		if f.Fix != "" {
			b.WriteString(fmt.Sprintf("   Suggested: %s\n", f.Fix))
		}
	}
	b.WriteString("\nApply minimal, focused fixes. Commit with 'fix: address analysis findings'.")

	hawkBin, err := os.Executable()
	if err != nil {
		hawkBin = "hawk"
	}

	cmd := exec.CommandContext(context.Background(), hawkBin, "exec", "--auto", "full", b.String()) // #nosec G204 -- hawkBin resolved via os.Executable() or literal 'hawk'; args are internal flags
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}
