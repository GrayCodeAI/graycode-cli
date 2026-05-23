package home

import (
	"log/slog"
	"os"
)

// Dir returns the user's home directory. It calls os.Exit(1) if the
// home directory cannot be determined. Use this for critical paths where an
// empty home would cause data to be written to the wrong location.
func Dir() string {
	home, err := os.UserHomeDir()
	if err != nil {
		slog.Error("cannot determine home directory", "error", err)
		os.Exit(1)
	}
	return home
}
