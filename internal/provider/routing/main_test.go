package routing

import (
	"os"
	"testing"

	"github.com/GrayCodeAI/graycode-cli/internal/catalogtest"
)

func TestMain(m *testing.M) {
	cleanup := catalogtest.InstallGlobal()
	defer cleanup()
	os.Exit(m.Run())
}
