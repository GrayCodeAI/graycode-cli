package plugin

import (
	"os"
	"testing"
)

func TestMain(m *testing.M) {
	if _, err := os.UserHomeDir(); err != nil {
		dir, mkErr := os.MkdirTemp("", "hawk-plugin-home-*")
		if mkErr != nil {
			os.Exit(1)
		}
		if setErr := os.Setenv("HOME", dir); setErr != nil {
			_ = os.RemoveAll(dir)
			os.Exit(1)
		}
		code := m.Run()
		_ = os.RemoveAll(dir)
		os.Exit(code)
	}
	os.Exit(m.Run())
}
