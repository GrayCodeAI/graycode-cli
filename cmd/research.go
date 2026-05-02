package cmd

import (
	"fmt"
	"strings"
)

// ResearchConfig controls the autonomous research loop.
// The loop itself is executed by the LLM agent via the generated prompt.
type ResearchConfig struct {
	MetricCmd    string // command to run the experiment (e.g. "go test -bench .")
	MetricGrep   string // grep pattern to extract the metric (e.g. "^val_bpb:")
	Direction    string // "lower" or "higher" — whether lower or higher metric is better
	Budget       int    // time budget per experiment in minutes (default: 5)
	BranchPrefix string // git branch prefix (default: "autoresearch")
	ResultsFile  string // TSV results file (default: "results.tsv")
}

// DefaultResearchConfig returns sensible defaults.
func DefaultResearchConfig() ResearchConfig {
	return ResearchConfig{
		Direction:    "lower",
		Budget:       5,
		BranchPrefix: "autoresearch",
		ResultsFile:  "results.tsv",
	}
}

// BuildResearchPrompt generates the full autonomous research prompt
// based on Karpathy's autoresearch program.md pattern.
func BuildResearchPrompt(cfg ResearchConfig) string {
	if cfg.Budget <= 0 {
		cfg.Budget = 5
	}
	if cfg.BranchPrefix == "" {
		cfg.BranchPrefix = "autoresearch"
	}
	if cfg.ResultsFile == "" {
		cfg.ResultsFile = "results.tsv"
	}
	if cfg.Direction == "" {
		cfg.Direction = "lower"
	}

	better := "lower"
	comparator := "decreased"
	if cfg.Direction == "higher" {
		better = "higher"
		comparator = "increased"
	}

	grepLine := ""
	if cfg.MetricGrep != "" {
		grepLine = fmt.Sprintf("   grep '%s' run.log", cfg.MetricGrep)
	} else {
		grepLine = "   cat run.log  # (extract the metric value from the output)"
	}

	timeout := cfg.Budget * 2

	var b strings.Builder

	b.WriteString(fmt.Sprintf(`You are an autonomous researcher. Your job is to iteratively improve a codebase by running experiments and keeping only what works.

## Goal

Run the metric command, measure the result, and make changes to improve it. %s is better.

## Metric command

  %s

## Extracting the metric

%s

## Results file

Log every experiment to %s (tab-separated, NOT comma-separated). Columns:

  commit	metric	status	description

- commit: short git hash (7 chars)
- metric: the numeric value extracted (use 0.000000 for crashes)
- status: keep, discard, or crash
- description: short text of what this experiment tried

**Never commit %s** — leave it untracked by git.

## Setup (do this once before the loop)

1. Read the codebase to understand the project structure and what can be changed.
2. Create a git branch: git checkout -b %s/run1 (pick a unique tag if that branch exists).
3. Initialize %s with just the header row.
4. Run the baseline (the very first run is always the unmodified code).

## The experiment loop

LOOP FOREVER:

1. **Check state**: Look at the current git branch and commit.
2. **Plan**: Decide on ONE specific change to try. Keep changes small and focused.
3. **Edit**: Apply the change to the code.
4. **Commit**: git add the changed files and git commit with a short message describing the experiment.
5. **Run**: Execute the experiment:
   %s > run.log 2>&1
   (redirect everything — do NOT let output flood your context)
6. **Extract metric**: Read the result:
%s
7. **Handle crashes**: If the grep output is empty or the run crashed:
   - Run: tail -n 50 run.log
   - If it is a trivial fix (typo, missing import), fix and re-run. Max 3 fix attempts per experiment.
   - If fundamentally broken, log status=crash in the TSV and move on.
8. **Log**: Append the result to %s.
9. **Keep or discard**:
   - If the metric %s: KEEP the commit (the branch advances).
   - If the metric is equal or worse: DISCARD via git reset --hard HEAD~1 (revert to before this experiment).

## Rules

**NEVER STOP**: Once the loop begins, do NOT pause to ask if you should continue. Do NOT ask "should I keep going?" or "is this a good stopping point?". The user may be away and expects you to work indefinitely until manually stopped. If you run out of ideas, think harder — re-read the code, try combining previous near-misses, try more radical changes, try simplifications.

**Timeout**: Each experiment should complete within ~%d minutes. If a run exceeds %d minutes, kill it (pkill -f or similar) and treat it as a failure — discard and revert.

**Simplicity criterion**: All else being equal, simpler is better. A tiny improvement that adds ugly complexity is not worth it. Removing code and getting equal or better results is a great outcome. Weigh complexity cost against improvement magnitude.

**Baseline first**: Your very first run must be the unmodified code to establish the baseline metric.

**Git discipline**: Every experiment gets its own commit BEFORE running. On failure, git reset --hard back. On success, the commit stays. The branch is a clean chain of improvements.

Begin now. Read the project, set up the branch, run the baseline, then start the experiment loop.
`, better, cfg.MetricCmd, grepLine, cfg.ResultsFile, cfg.ResultsFile,
		cfg.BranchPrefix, cfg.ResultsFile, cfg.MetricCmd, grepLine,
		cfg.ResultsFile, comparator, cfg.Budget, timeout))

	return b.String()
}

// parseResearchArgs parses /research arguments into a ResearchConfig.
// Format: /research [flags] <metric-command>
// Flags: --grep <pattern>, --direction <lower|higher>, --budget <minutes>,
//        --branch <prefix>, --results <file>
func parseResearchArgs(args string) ResearchConfig {
	cfg := DefaultResearchConfig()
	parts := strings.Fields(args)

	var metricParts []string
	for i := 0; i < len(parts); i++ {
		switch parts[i] {
		case "--grep":
			if i+1 < len(parts) {
				i++
				cfg.MetricGrep = parts[i]
			}
		case "--direction":
			if i+1 < len(parts) {
				i++
				cfg.Direction = parts[i]
			}
		case "--budget":
			if i+1 < len(parts) {
				i++
				fmt.Sscanf(parts[i], "%d", &cfg.Budget)
			}
		case "--branch":
			if i+1 < len(parts) {
				i++
				cfg.BranchPrefix = parts[i]
			}
		case "--results":
			if i+1 < len(parts) {
				i++
				cfg.ResultsFile = parts[i]
			}
		default:
			metricParts = append(metricParts, parts[i])
		}
	}

	cfg.MetricCmd = strings.Join(metricParts, " ")
	return cfg
}
