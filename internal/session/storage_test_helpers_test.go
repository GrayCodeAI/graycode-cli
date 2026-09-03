package session

import (
	"testing"

	"github.com/GrayCodeAI/graycode-cli/internal/storage"
)

func setTestSessionsDir(t *testing.T, root string) string {
	t.Helper()
	storage.SetTestDirs(t, root)
	return storage.SessionsDir()
}
