package tool

import (
	"context"
	"fmt"

	"github.com/GrayCodeAI/hawk/internal/lint"
)

// lintConfig returns the lint configuration carried on the ToolContext, or a
// disabled config when there is none.
func lintConfig(ctx context.Context) lint.Config {
	tc := GetToolContext(ctx)
	if tc == nil {
		return lint.Config{}
	}
	return tc.Lint
}

// postWriteLint runs the configured linter against a freshly written/edited
// file and returns a human-readable note to append to the tool result. When
// linting is disabled, no linter is configured, or the file passes, it returns
// an empty string. On lint failures the returned note carries the linter
// output so the agent can auto-fix.
func postWriteLint(ctx context.Context, path string) string {
	cfg := lintConfig(ctx)
	if !cfg.Enabled {
		return ""
	}
	res := lint.RunLint(ctx, path, cfg)
	if !res.Ran || res.OK {
		return ""
	}
	return fmt.Sprintf("\n\nLint (%s) reported issues — please fix:\n%s", res.Linter, res.Output)
}
