package plugin

import (
	"os"
	"testing"

	"github.com/GrayCodeAI/graycode-cli/internal/testutil"
)

func TestMain(m *testing.M) {
	cleanupStorage, err := testutil.InstallHermeticStorage()
	if err != nil {
		os.Exit(1)
	}
	if _, err := os.UserHomeDir(); err != nil {
		dir, mkErr := os.MkdirTemp("", "graycode-plugin-home-*")
		if mkErr != nil {
			os.Exit(1)
		}
		if setErr := os.Setenv("HOME", dir); setErr != nil {
			_ = os.RemoveAll(dir)
			os.Exit(1)
		}
		code := m.Run()
		cleanupStorage()
		_ = os.RemoveAll(dir)
		os.Exit(code)
	}
	code := m.Run()
	cleanupStorage()
	os.Exit(code)
}
