package main

import (
	"errors"
	"fmt"
	"os"

	"github.com/GrayCodeAI/hawk/cmd"
	"github.com/GrayCodeAI/hawk/internal/hawkerr"
	"github.com/GrayCodeAI/hawk/internal/mcp"
)

// Version, Commit, and BuildDate are set at build time via ldflags.
//
// Source of truth: the VERSION file at the repo root, and the matching git
// tag created by release-please. The Makefile and goreleaser inject these
// values during release builds:
//
//	-X main.Version=$(cat VERSION)
//	-X main.Commit=$(git rev-parse --short HEAD)
//	-X main.BuildDate=$(date -u +%Y-%m-%dT%H:%M:%SZ)
//
// The "dev" / "none" / "unknown" defaults below apply only to local builds
// without ldflags so it's obvious when running an unreleased binary.
var (
	Version   = "dev"
	Commit    = "none"
	BuildDate = "unknown"
)

func main() {
	// Handle --version flag immediately
	if len(os.Args) > 1 && os.Args[1] == "--version" {
		fmt.Println("hawk " + Version)
		return
	}

	// Propagate the canonical version to all sub-packages that surface it
	// (CLI version flag, HTTP API version field, and MCP clientInfo).
	// The sandbox image has an independent compatibility version.
	cmd.SetVersion(Version)
	cmd.SetBuildDate(BuildDate)
	mcp.SetClientVersion(Version)

	if err := cmd.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		// An explicit ExitCodeError (e.g. a wrapped Bash exit status) wins —
		// it already carries the intended code. Otherwise classify the failure
		// into the stable exit-code taxonomy so callers can branch on the
		// reason (auth vs rate-limit vs network) instead of seeing a bare 1.
		var exitErr *cmd.ExitCodeError
		if errors.As(err, &exitErr) {
			os.Exit(exitErr.Code)
		}
		os.Exit(hawkerr.ClassifyExitCode(err))
	}
}
