package cmd

import "fmt"

// Build-time variables injected by goreleaser/ldflags.
var (
	Version = "dev"
	Commit  = "none"
	Date    = "unknown"
)

// VersionString returns the full version string.
func VersionString() string {
	return fmt.Sprintf("hawk %s (commit: %s, built: %s)", Version, Commit, Date)
}

// ShortVersion returns just the version number.
func ShortVersion() string {
	return Version
}
