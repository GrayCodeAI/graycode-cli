package cmd

import (
	tracecli "github.com/GrayCodeAI/trace/cli"
)

// trace is a sibling library consumed by hawk, not a standalone product. Its full
// command tree is built by tracecli.NewRootCmd() (Use: "trace"), so mounting it here
// surfaces every trace feature under `hawk trace ...` without porting any code.
func init() {
	rootCmd.AddCommand(tracecli.NewRootCmd())
}
