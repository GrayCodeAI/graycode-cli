package config

import (
	"os"
	"testing"

	"github.com/GrayCodeAI/graycode-cli/internal/catalogtest"
	"github.com/GrayCodeAI/graycode-cli/internal/testutil"
)

func TestMain(m *testing.M) {
	cleanupStorage, err := testutil.InstallHermeticStorage()
	if err != nil {
		os.Exit(1)
	}
	cleanup := catalogtest.InstallGlobal()
	code := m.Run()
	cleanup()
	cleanupStorage()
	os.Exit(code)
}
