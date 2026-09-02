package cmd

import (
	swiftcli "github.com/GrayCodeAI/swift/cli"
)

// swift is a sibling library consumed by hawk, not a standalone product. Its full
// command tree is built by swiftcli.NewRootCmd() (Use: "swift"), so mounting it here
// surfaces every swift feature under `hawk swift ...` without porting any code.
func init() {
	rootCmd.AddCommand(swiftcli.NewRootCmd())
}
