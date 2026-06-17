package sandbox

import "context"

// Mode represents the sandbox isolation level.
type Mode string

const (
	ModeStrict    Mode = "strict"    // read-only filesystem
	ModeWorkspace Mode = "workspace" // write only in project dir + /tmp
	ModeOff       Mode = "off"       // no restrictions
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
