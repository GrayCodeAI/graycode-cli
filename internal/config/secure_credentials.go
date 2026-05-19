package config

import (
	"os"
	"strings"
)

// SecureCredentialsEnabled is true when API keys should prefer keychain over plain ~/.hawk/env only.
// Default on for solo secure mode; set HAWK_SECURE_CREDENTIALS=0 to disable.
func SecureCredentialsEnabled() bool {
	v := strings.TrimSpace(os.Getenv("HAWK_SECURE_CREDENTIALS"))
	if v == "" {
		return true
	}
	switch strings.ToLower(v) {
	case "0", "false", "no", "off":
		return false
	default:
		return true
	}
}
