package engine

import (
	"os"
	"testing"

	"github.com/GrayCodeAI/hawk/internal/catalogtest"
	"github.com/GrayCodeAI/hawk/internal/testutil"
)

func TestMain(m *testing.M) {
	cleanupStorage, err := testutil.InstallHermeticStorage()
	if err != nil {
		os.Exit(1)
	}
	defer cleanupStorage()
	cleanup := catalogtest.InstallGlobal()
	defer cleanup()
	os.Exit(m.Run())
}
