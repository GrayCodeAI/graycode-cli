package main

import (
	"fmt"
	"os"

	"github.com/GrayCodeAI/hawk/cmd"
)

// Version is set at build time via ldflags.
var Version = "0.0.1"

func main() {
	cmd.SetVersion(Version)
	if err := cmd.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
