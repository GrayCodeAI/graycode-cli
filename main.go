package main

import (
	"fmt"
	"os"

	"github.com/GrayCodeAI/hawk/internal/api"
	"github.com/GrayCodeAI/hawk/cmd"
	"github.com/GrayCodeAI/hawk/internal/mcp"
	"github.com/GrayCodeAI/hawk/internal/sandbox"
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
	// Propagate the canonical version to all sub-packages that surface it
	// (CLI version flag, HTTP API version field, MCP clientInfo, sandbox
	// container image tag). Each package keeps a private settable variable
	// to avoid an import cycle with main.
	cmd.SetVersion(Version)
	cmd.SetBuildDate(BuildDate)
	api.SetVersion(Version)
	mcp.SetClientVersion(Version)
	sandbox.ContainerImageTag = Version

	if err := cmd.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
