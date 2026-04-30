package main

import (
	"fmt"
	"os"

	"github.com/GrayCodeAI/hawk/cmd"
)

// Version is set at build time via ldflags.
// Example: go build -ldflags "-X main.Version=1.0.0" .
var Version = "dev"

// BuildDate is set at build time via ldflags.
var BuildDate = "unknown"

func main() {
	cmd.SetVersion(Version)
	cmd.SetBuildDate(BuildDate)
	if err := cmd.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
