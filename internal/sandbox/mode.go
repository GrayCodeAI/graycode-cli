package sandbox

import (
	"context"
	"os"
	"path/filepath"
	"strings"

	"github.com/GrayCodeAI/hawk/internal/storage"
)

// Mode represents the sandbox isolation level.
type Mode string

const (
	ModeStrict    Mode = "strict"    // read-only filesystem
	ModeWorkspace Mode = "workspace" // write only in project dir + /tmp
	ModeOff       Mode = "off"       // no restrictions
)

// Tier controls the sandbox's security posture. The new default is
// TierWorkspace (allow workspace writes, deny process exec) which is
// safer than the legacy TierOff default. Existing users who rely on
// process exec can opt back in via Tier=TierOff in their config.
type Tier string

const (
	// TierStrict denies everything: no writes, no process exec,
	// no network. The agent can only read.
	TierStrict Tier = "strict"
	// TierWorkspace is the new default. Allows writes to the
	// workspace + scratch dir, but denies process exec. An agent
	// that needs to run Bash must either be in container mode
	// (ContainerExecutor) or have Tier set to TierOff.
	TierWorkspace Tier = "workspace"
	// TierOff is the legacy default. Allow everything: writes,
	// process exec, network. Used by users who need the full
	// pre-tier behavior.
	TierOff Tier = "off"
)

// SandboxConfig describes how a command should be sandboxed.
type SandboxConfig struct {
	Mode         Mode
	WorkspaceDir string
	AllowNetwork bool
	// Tier selects the security tier (strict / workspace / off).
	// Empty defaults to TierOff for back-compat with legacy
	// callers that don't know about tiers. New callers should
	// set Tier explicitly (typically TierWorkspace to match
	// the Config.Tier default in DefaultConfig).
	Tier Tier
}

// DefaultHawkPolicy creates a sensible default SeatbeltPolicy for hawk
// operations in the given working directory. The tier parameter
// selects the security posture:
//
//   - TierStrict: deny everything
//   - TierWorkspace (new default): allow workspace writes, no process
//   - TierOff: legacy behavior (allow everything)
func DefaultHawkPolicy(workDir string, tier Tier) *SeatbeltPolicy {
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
		Tier:          tier,
	}

	// Apply the tier's policy on top of the defaults. Tier takes
	// precedence over the legacy AllowWrite/AllowProcess fields
	// so the new safe default is enforced regardless of legacy
	// config values.
	switch tier {
	case TierStrict:
		p.AllowWrite = false
		p.AllowProcess = false
	case TierWorkspace:
		p.AllowWrite = true
		p.AllowProcess = false
	case TierOff, "":
		// Legacy behavior: allow everything.
		p.AllowWrite = true
		p.AllowProcess = true
	default:
		// Unknown tier: log via fallback to TierOff. Caller can
		// override by setting Tier explicitly to a known value.
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

// ModeAllowsNetwork reports whether a command run under the given mode
// should be allowed outbound network access.
//
// The seatbelt policy historically allowed network for every tier, so a
// sandboxed command could still exfiltrate data. This makes network denial
// actually reachable and gives strict mode its documented "no network"
// posture, while keeping it on for workspace builds (package managers,
// module fetches). HAWK_SANDBOX_NETWORK overrides the default either way:
//
//	HAWK_SANDBOX_NETWORK=0|off|no|false → deny in all modes
//	HAWK_SANDBOX_NETWORK=1|on|yes|true  → allow in all modes
func ModeAllowsNetwork(m Mode) bool {
	switch strings.ToLower(strings.TrimSpace(os.Getenv("HAWK_SANDBOX_NETWORK"))) {
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
