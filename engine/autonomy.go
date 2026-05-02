package engine

import (
	"strings"
)

// AutonomyLevel controls how much the agent can do without asking the user.
type AutonomyLevel int

const (
	// AutonomySupervised asks for permission on every tool call.
	AutonomySupervised AutonomyLevel = 0
	// AutonomyBasic auto-allows read-only tools.
	AutonomyBasic AutonomyLevel = 1
	// AutonomySemi auto-allows reads and writes, asks for Bash.
	AutonomySemi AutonomyLevel = 2
	// AutonomyFull auto-allows everything except destructive commands.
	AutonomyFull AutonomyLevel = 3
	// AutonomyYOLO never asks for permission.
	AutonomyYOLO AutonomyLevel = 4
)

// AutonomyConfig holds the derived permission flags for an autonomy level.
type AutonomyConfig struct {
	Level           AutonomyLevel
	AutoContinue    bool
	AutoApplyEdits  bool
	AutoExecuteBash bool
	AutoCommit      bool
}

// readOnlyTools are tools that only observe and never mutate.
var readOnlyTools = map[string]bool{
	"Read":      true,
	"Grep":      true,
	"Glob":      true,
	"LS":        true,
	"WebSearch": true,
	"file_read": true,
	"grep":      true,
	"glob":      true,
	"ls":        true,
	"web_search": true,
}

// writeTools are tools that create or modify files.
var writeTools = map[string]bool{
	"Write":      true,
	"Edit":       true,
	"file_write": true,
	"file_edit":  true,
}

// PresetConfig returns the AutonomyConfig for a given level.
func PresetConfig(level AutonomyLevel) AutonomyConfig {
	switch level {
	case AutonomySupervised:
		return AutonomyConfig{Level: level}
	case AutonomyBasic:
		return AutonomyConfig{
			Level:        level,
			AutoContinue: true,
		}
	case AutonomySemi:
		return AutonomyConfig{
			Level:          level,
			AutoContinue:   true,
			AutoApplyEdits: true,
		}
	case AutonomyFull:
		return AutonomyConfig{
			Level:           level,
			AutoContinue:    true,
			AutoApplyEdits:  true,
			AutoExecuteBash: true,
			AutoCommit:      true,
		}
	case AutonomyYOLO:
		return AutonomyConfig{
			Level:           level,
			AutoContinue:    true,
			AutoApplyEdits:  true,
			AutoExecuteBash: true,
			AutoCommit:      true,
		}
	default:
		return AutonomyConfig{Level: AutonomySupervised}
	}
}

// NeedsPermission returns true when the tool call should prompt the user.
// isSafe indicates whether the specific invocation has been classified as safe
// (e.g. a non-destructive bash command).
func (c AutonomyConfig) NeedsPermission(toolName string, isSafe bool) bool {
	switch c.Level {
	case AutonomyYOLO:
		return false
	case AutonomyFull:
		// Auto-allow everything except destructive commands.
		// A non-safe bash command at Full level still needs permission.
		if toolName == "Bash" || toolName == "bash" {
			return !isSafe
		}
		return false
	case AutonomySemi:
		if readOnlyTools[toolName] || writeTools[toolName] {
			return false
		}
		return true
	case AutonomyBasic:
		if readOnlyTools[toolName] {
			return false
		}
		return true
	default: // Supervised
		return true
	}
}

// ParseAutonomyLevel converts a string name or number to an AutonomyLevel.
func ParseAutonomyLevel(s string) AutonomyLevel {
	s = strings.TrimSpace(strings.ToLower(s))
	switch s {
	case "0", "supervised":
		return AutonomySupervised
	case "1", "basic":
		return AutonomyBasic
	case "2", "semi":
		return AutonomySemi
	case "3", "full":
		return AutonomyFull
	case "4", "yolo":
		return AutonomyYOLO
	default:
		return AutonomySupervised
	}
}

// String returns the human-readable name of an autonomy level.
func (l AutonomyLevel) String() string {
	switch l {
	case AutonomySupervised:
		return "supervised"
	case AutonomyBasic:
		return "basic"
	case AutonomySemi:
		return "semi"
	case AutonomyFull:
		return "full"
	case AutonomyYOLO:
		return "yolo"
	default:
		return "supervised"
	}
}
