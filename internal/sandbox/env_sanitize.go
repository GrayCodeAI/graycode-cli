package sandbox

import (
	"sort"
	"strings"
)

// DisallowedEnvVars are environment variables that, when overridden in a child
// process environment, enable command/library/hijacking. Ported from goose
// `extension.rs::Envs`: blocking these in extension/MCP/plugin configs prevents
// an untrusted config from redirecting `PATH`, preloading libraries, or
// monkey-patching the Python/Node/Go toolchains of the spawned process.
var DisallowedEnvVars = map[string]bool{
	"PATH":                       true,
	"LD_PRELOAD":                 true,
	"LD_LIBRARY_PATH":            true,
	"DYLD_INSERT_LIBRARIES":      true,
	"DYLD_LIBRARY_PATH":          true,
	"PYTHONPATH":                 true,
	"PYTHONHOME":                 true,
	"NODE_OPTIONS":               true,
	"NODE_PATH":                  true,
	"GOROOT":                     true,
	"GOPATH":                     true,
	"RUSTFLAGS":                  true,
	"CARGO_HOME":                 true,
	"RUBYLIB":                    true,
	"PERL5LIB":                   true,
	"LD_DEBUG":                   true,
	"DYLD_FRAMEWORK_PATH":        true,
	"DYLD_FALLBACK_LIBRARY_PATH": true,
}

// SanitizedEnv is the result of filtering a proposed environment.
type SanitizedEnv struct {
	// Env is the child environment (sorted "KEY=VALUE" entries) with disallowed
	// keys removed.
	Env []string
	// Removed lists the disallowed keys that were dropped (not their values).
	Removed []string
}

// SanitizeEnv filters a proposed child environment (as "KEY=VALUE" entries),
// removing any disallowed override key. It returns the sanitized env plus the
// removed key names. It never fails: benign keys pass through unchanged.
func SanitizeEnv(env []string) SanitizedEnv {
	seen := make(map[string]int, len(env))
	var out []string
	var removed []string
	for _, kv := range env {
		key := kv
		if i := strings.IndexByte(kv, '='); i >= 0 {
			key = kv[:i]
		}
		key = strings.TrimSpace(key)
		if DisallowedEnvVars[key] {
			removed = append(removed, key)
			continue
		}
		// De-duplicate by key, last wins, preserving order of first occurrence.
		if _, dup := seen[key]; dup {
			// Replace the existing entry at its original position.
			out[seen[key]] = kv
			continue
		}
		seen[key] = len(out)
		out = append(out, kv)
	}
	sort.Strings(out)
	sort.Strings(removed)
	return SanitizedEnv{Env: out, Removed: removed}
}
