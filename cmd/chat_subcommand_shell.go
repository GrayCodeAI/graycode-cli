package cmd

import (
	"context"
	"strings"
)

func runSlashShellCommand(m *chatModel, command string) (string, bool) {
	if m == nil || m.session == nil {
		return "Error: no active session", true
	}
	out, isErr := m.session.RunUserShellCommand(context.Background(), command, 0)
	return strings.TrimSpace(out), isErr
}

func shellCommandFailed(output string, isErr bool) bool {
	if isErr {
		return true
	}
	return strings.Contains(output, "exit code:") || strings.Contains(output, "(command timed out)")
}
