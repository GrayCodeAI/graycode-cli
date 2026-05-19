package cmd

import (
	"os"
	"testing"

	"github.com/GrayCodeAI/hawk/internal/catalogtest"
)

func TestMain(m *testing.M) {
	cleanup := catalogtest.InstallGlobal()
	defer cleanup()
	os.Exit(m.Run())
}
