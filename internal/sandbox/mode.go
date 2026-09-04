package sandbox

import (
	"context"
	"os"
	"path/filepath"
	"strings"

	"github.com/GrayCodeAI/graycode-cli/internal/storage"
)

// Mode represents the sandbox isolation level — what the sandbox *does* to
// the process (filesystem/network restrictions).
type Mode string

const (
	ModeStrict    Mode = "strict"    // read-only filesystem
	ModeWorkspace Mode = "workspace" // write only in project dir + /tmp
	ModeOff       Mode = "off"       // no restrictions
)

// Security controls the sandbox's security posture — what the user *wants*
// for safety. It is orthogonal to Mode: Mode governs *how* the sandbox
// isolates, Security governs *how much* isolation the user desires.
//
// The new default is SecurityWorkspace (allow workspace writes, deny process
// exec) which is safer than the legacy SecurityOff default. Existing users
// who rely on process exec can opt back in via Security=SecurityOff.
type Security string

const (
	// SecurityStrict denies everything: no writes, no process exec,
	// no network. The agent can only read.
	SecurityStrict Security = "strict"
	// SecurityWorkspace is the new default. Allows writes to the
	// workspace + scratch dir, but denies process exec. An agent
	// that needs to run Bash must either be in container mode
	// (ContainerExecutor) or have Security set to SecurityOff.
	SecurityWorkspace Security = "workspace"
	// SecurityOff is the legacy default. Allow everything: writes,
	// process exec, network. Used by users who need the full
	// pre-security behavior.
	SecurityOff Security = "off"
)

// Deprecated: Tier is renamed to Security. These aliases are provided for
// backward compatibility and will be removed in a future release.
type Tier = Security

const (
	TierStrict    = SecurityStrict
	TierWorkspace = SecurityWorkspace
	TierOff       = SecurityOff
)

// SandboxConfig describes how a command should be sandboxed.
type SandboxConfig struct {
	Mode         Mode
	WorkspaceDir string
	AllowNetwork bool
	// Security selects the security posture (strict / workspace / off).
	// Empty defaults to SecurityOff for back-compat with legacy
	// callers that don't know about security. New callers should
	// set Security explicitly (typically SecurityWorkspace to match
	// the Config.Security default in DefaultConfig).
	Security Security
}

// DefaultGraycodePolicy creates a sensible default SeatbeltPolicy for graycode
// operations in the given working directory. The security parameter
// selects the security posture:
//
//   - SecurityStrict: deny everything
//   - SecurityWorkspace (new default): allow workspace writes, no process
//   - SecurityOff: legacy behavior (allow everything)
func DefaultGraycodePolicy(workDir string, security Security) *SeatbeltPolicy {
	home := os.Getenv("HOME")
	gopath := os.Getenv("GOPATH")
	if gopath == "" {
		gopath = filepath.Join(home, "go")
	}

	configDir := storage.ConfigDir()
	stateDir := storage.StateDir()
	cacheDir := storage.CacheDir()

	readPaths := []string{
		workDir,
		"/usr",
		"/bin",
		"/Library",
		"/System",
		"/dev",
		"/tmp",
		"/private/tmp",
		configDir,
		stateDir,
		cacheDir,
		gopath,
	}

	writePaths := []string{
		workDir,
		"/tmp",
		"/private/tmp",
		"/dev/null",
		configDir,
		stateDir,
		cacheDir,
	}

	p := &SeatbeltPolicy{
		AllowNetwork:  true,
		AllowSysctl:   true,
		ReadablePaths: readPaths,
		WritablePaths: writePaths,
		Security:      security,
	}

	// Apply the security policy on top of the defaults. Security takes
	// precedence over the legacy AllowWrite/AllowProcess fields
	// so the new safe default is enforced regardless of legacy
	// config values.
	switch security {
	case SecurityStrict:
		p.AllowWrite = false
		p.AllowProcess = false
		p.AllowNetwork = false
	case SecurityWorkspace:
		p.AllowWrite = true
		p.AllowProcess = false
	case SecurityOff, "":
		// Legacy behavior: allow everything.
		p.AllowWrite = true
		p.AllowProcess = true
	default:
		// Unknown security: log via fallback to SecurityOff. Caller can
		// override by setting Security explicitly to a known value.
		p.AllowWrite = true
		p.AllowProcess = true
	}

	return p
}

// ParseMode converts a string to a Mode. Unrecognized values default to
// ModeStrict (fail-closed) to prevent accidental sandbox bypass via typos.
func ParseMode(s string) Mode {
	switch s {
	case "strict":
		return ModeStrict
	case "workspace":
		return ModeWorkspace
	case "off":
		return ModeOff
	case "":
		return ModeStrict
	default:
		return ModeStrict
	}
}

// ParseSecurity converts a string to a Security. Unrecognized values default
// to SecurityStrict (fail-closed) to prevent accidental sandbox bypass.
func ParseSecurity(s string) Security {
	switch s {
	case "strict":
		return SecurityStrict
	case "workspace":
		return SecurityWorkspace
	case "off":
		return SecurityOff
	case "":
		return SecurityStrict
	default:
		return SecurityStrict
	}
}

// ModeAllowsNetwork reports whether a command run under the given mode
// should be allowed outbound network access.
//
// The seatbelt policy historically allowed network for every tier, so a
// sandboxed command could still exfiltrate data. This makes network denial
// actually reachable and gives strict mode its documented "no network"
// posture, while keeping it on for workspace builds (package managers,
// module fetches). GRAYCODE_SANDBOX_NETWORK overrides the default either way:
//
//	GRAYCODE_SANDBOX_NETWORK=0|off|no|false → deny in all modes
//	GRAYCODE_SANDBOX_NETWORK=1|on|yes|true  → allow in all modes
func ModeAllowsNetwork(m Mode) bool {
	switch strings.ToLower(strings.TrimSpace(os.Getenv("GRAYCODE_SANDBOX_NETWORK"))) {
	case "0", "off", "no", "false":
		return false
	case "1", "on", "yes", "true":
		return true
	}
	// Default: strict denies (read-only, no exfiltration path); workspace
	// keeps network so ordinary builds still work.
	return m != ModeStrict
}

// modeCtxKey is the context key for sandbox Mode.
type modeCtxKey struct{}

// ContextWithMode attaches a sandbox Mode to a context.
func ContextWithMode(ctx context.Context, m Mode) context.Context {
	return context.WithValue(ctx, modeCtxKey{}, m)
}

// ModeFromContext retrieves the sandbox Mode from a context.
// Returns ModeOff when no mode is set.
func ModeFromContext(ctx context.Context) Mode {
	if m, ok := ctx.Value(modeCtxKey{}).(Mode); ok {
		return m
	}
	return ModeOff
}
