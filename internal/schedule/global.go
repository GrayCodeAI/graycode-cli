package schedule

import "sync"

var (
	defaultManager     *Manager
	defaultManagerOnce sync.Once
)

// DefaultManager returns the global fallback schedule manager.
func DefaultManager() *Manager {
	defaultManagerOnce.Do(func() {
		defaultManager = NewManager()
	})
	return defaultManager
}
