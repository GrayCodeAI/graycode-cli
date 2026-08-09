package tool

import "strings"

// IntentBundle describes a small group of tools that commonly work together
// for one kind of request. Bundles are used only to promote already-registered
// tools onto the model surface; they never execute a tool or bypass approval.
type IntentBundle struct {
	Name        string
	Keywords    []string
	Tools       []string
	Description string
}

var intentBundles = []IntentBundle{
	{
		Name:        "web",
		Keywords:    []string{"http://", "https://", "url", "website", "web page", "webpage", "internet", "browse", "browser"},
		Tools:       []string{"WebFetch", "WebSearch", "Browser", "Screenshot", "AgenticFetch"},
		Description: "Inspect public web pages and JavaScript-rendered sites.",
	},
	{
		Name:        "code-understanding",
		Keywords:    []string{"understand", "explain", "inspect", "trace", "where is", "find", "architecture", "outline", "symbol", "reference"},
		Tools:       []string{"SmartRead", "Outline", "CodeSearch", "CodeGraph", "LSP", "Impact", "GitHistory"},
		Description: "Build a compact, symbol-aware view of the codebase before editing.",
	},
	{
		Name:        "verification",
		Keywords:    []string{"test", "tests", "testing", "lint", "linting", "build", "compile", "verify", "diagnostic", "diagnostics", "ci", "check"},
		Tools:       []string{"ProjectVerify", "DependencyAudit", "Diagnostics", "DevEnv", "Workflow", "NilAway", "Revive"},
		Description: "Run project-aware build, test, lint, and static checks.",
	},
	{
		Name:        "git",
		Keywords:    []string{"git", "commit", "branch", "merge", "rebase", "diff", "worktree", "conflict", "pull request", "pr"},
		Tools:       []string{"Git", "GitHub", "GitHistory", "Impact", "CodeGraph", "EnterWorktree", "ExitWorktree", "ResolveConflicts", "pr_generate"},
		Description: "Inspect and manage repository history and review state.",
	},
	{
		Name:        "editing",
		Keywords:    []string{"refactor", "rename", "rewrite", "edit", "patch", "fix", "imports", "organize imports", "apply"},
		Tools:       []string{"Patch", "AtomicMultiEdit", "AutoImport", "OrganizeImports", "Refactor"},
		Description: "Use precise, structured editing and refactoring operations.",
	},
	{
		Name:        "data",
		Keywords:    []string{"sql", "database", "query", "schema", "notebook", "jupyter", "migration"},
		Tools:       []string{"SQL", "NotebookEdit", "Diagnostics"},
		Description: "Inspect data and notebooks with explicit write controls.",
	},
	{
		Name:        "security",
		Keywords:    []string{"security", "secure", "vulnerability", "vulnerabilities", "audit", "cve", "secret", "secrets", "ssrf", "threat"},
		Tools:       []string{"DependencyAudit", "Diagnostics", "NilAway", "Revive", "CodeSearch", "GitHistory"},
		Description: "Review code and history for security and quality risks.",
	},
	{
		Name:        "tool-health",
		Keywords:    []string{"tool health", "tool status", "available tools", "missing tool", "prerequisite", "doctor", "capability"},
		Tools:       []string{"ToolHealth", "Diagnostics", "DevEnv"},
		Description: "Check registered capabilities and runtime prerequisites before acting.",
	},
}

// MatchIntentBundles returns bundles whose keyword expressions occur in the
// request. Matching is intentionally deterministic and conservative: it only
// controls which schemas are shown to the model and cannot cause execution.
func MatchIntentBundles(request string) []IntentBundle {
	text := strings.ToLower(strings.TrimSpace(request))
	if text == "" {
		return nil
	}

	matched := make([]IntentBundle, 0, 2)
	for _, bundle := range intentBundles {
		for _, keyword := range bundle.Keywords {
			if strings.Contains(text, keyword) {
				matched = append(matched, bundle)
				break
			}
		}
	}
	return matched
}

// PromoteForIntent makes relevant registered tools model-visible for the
// current turn. It returns the canonical names that were newly promoted.
// Promotion never changes the executable registry and never grants approval.
func (r *Registry) PromoteForIntent(request string) []string {
	if r == nil {
		return nil
	}

	seen := make(map[string]bool)
	var promoted []string
	for _, bundle := range MatchIntentBundles(request) {
		for _, name := range bundle.Tools {
			if seen[name] || r.IsModelVisible(name) {
				seen[name] = true
				continue
			}
			if r.PromoteModelTool(name) {
				seen[name] = true
				promoted = append(promoted, name)
			}
		}
	}
	return promoted
}

// IntentBundleSummary returns concise descriptions suitable for diagnostics
// and the startup/help UI without exposing every tool schema to the model.
func IntentBundleSummary() []string {
	result := make([]string, 0, len(intentBundles))
	for _, bundle := range intentBundles {
		result = append(result, bundle.Name+": "+bundle.Description)
	}
	return result
}

// IntentCategoriesForTool returns the intent bundles that can promote name.
// It is used by diagnostics and `hawk tools --json` to make the registry
// explainable without leaking prompt-matching internals.
func IntentCategoriesForTool(name string) []string {
	var categories []string
	for _, bundle := range intentBundles {
		for _, candidate := range bundle.Tools {
			if candidate == name {
				categories = append(categories, bundle.Name)
				break
			}
		}
	}
	return categories
}
