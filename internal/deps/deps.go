// Package deps documents the dependency injection pattern used across hawk.
//
// # Pattern
//
// Each package that needs external dependencies defines a Deps struct with
// exported fields for each dependency. A zero value means "use default".
// Constructors accept the deps struct and fill in defaults for zero fields.
//
// Example:
//
//	type LoggerDeps struct {
//	    Output io.Writer  // default: os.Stderr
//	    Level  Level      // default: Info
//	}
//
//	func NewWithDeps(deps LoggerDeps) *Logger {
//	    if deps.Output == nil {
//	        deps.Output = os.Stderr
//	    }
//	    return &Logger{level: deps.Level, output: deps.Output}
//	}
//
// Tests override individual fields:
//
//	var buf bytes.Buffer
//	log := logger.NewWithDeps(logger.LoggerDeps{Output: &buf})
//
// This replaces positional constructor arguments and makes testing explicit.
// Keep legacy constructors (e.g., New(output, level)) as convenience wrappers
// that call NewWithDeps internally.
package deps
