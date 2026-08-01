package sandbox

import (
	"os"
	"testing"

	"github.com/GrayCodeAI/hawk/internal/testutil"
)

func TestMain(m *testing.M) {
	cleanup, err := testutil.InstallHermeticStorage()
	if err != nil {
		os.Exit(1)
	}
	code := m.Run()
	cleanup()
	os.Exit(code)
}
