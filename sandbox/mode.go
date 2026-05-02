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
}

// ParseMode converts a string to a Mode, defaulting to ModeOff for
// unrecognised values.
func ParseMode(s string) Mode {
	switch s {
	case "strict":
		return ModeStrict
	case "workspace":
		return ModeWorkspace
	case "off", "":
		return ModeOff
	default:
		return ModeOff
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
