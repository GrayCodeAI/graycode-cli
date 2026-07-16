// Package flags provides process-level feature flags for staged Year 0 work.
//
// Flags are read from environment variables.
// Folder trust defaults on (PACK-03). Spawn v2 and marketplace remain opt-in
// until their cutovers complete.
package flags

import (
	"os"
	"strings"
	"sync"
)

// Year 0 flag names (environment variables).
const (
	EnvSpawnV2     = "HAWK_Y0_SPAWN_V2"
	EnvFolderTrust = "HAWK_Y0_FOLDER_TRUST"
	EnvMarketplace = "HAWK_Y0_MARKETPLACE"
)

var (
	mu       sync.RWMutex
	override = map[string]*bool{}
)

// ResetForTest clears test overrides. Not for production use.
func ResetForTest() {
	mu.Lock()
	defer mu.Unlock()
	override = map[string]*bool{}
}

// SetForTest forces a flag value in tests.
func SetForTest(env string, enabled bool) {
	mu.Lock()
	defer mu.Unlock()
	v := enabled
	override[env] = &v
}

func envEnabled(env string, defaultOn bool) bool {
	mu.RLock()
	if o, ok := override[env]; ok && o != nil {
		v := *o
		mu.RUnlock()
		return v
	}
	mu.RUnlock()

	raw, ok := os.LookupEnv(env)
	if !ok || strings.TrimSpace(raw) == "" {
		return defaultOn
	}
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case "1", "true", "yes", "on":
		return true
	case "0", "false", "no", "off":
		return false
	default:
		return defaultOn
	}
}

// SpawnV2 reports whether typed SpawnRequest wiring is active.
// Default false until PACK-02 lands and flips the default.
func SpawnV2() bool {
	return envEnabled(EnvSpawnV2, false)
}

// FolderTrust reports whether project folder trust gates automation.
// Default true after PACK-03 (secure by default). Set HAWK_Y0_FOLDER_TRUST=0 to disable.
func FolderTrust() bool {
	return envEnabled(EnvFolderTrust, true)
}

// Marketplace reports whether marketplace install paths are enabled.
// Default false until PACK-05 + folder trust.
func Marketplace() bool {
	return envEnabled(EnvMarketplace, false)
}
