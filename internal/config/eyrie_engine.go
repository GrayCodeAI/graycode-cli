package config

import (
	eyrieengine "github.com/GrayCodeAI/eyrie/engine"
)

// newEyrieEngine is Hawk's composition root for Eyrie's stable host facade.
// Eyrie retains ownership of paths and the platform secret store through its
// backward-compatible defaults during migration. Tests may continue injecting
// lower-level stores through Eyrie's existing seams.
func newEyrieEngine() (*eyrieengine.Engine, error) {
	return eyrieengine.New(eyrieengine.Options{})
}
