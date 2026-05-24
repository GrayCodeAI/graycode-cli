package cmd

import (
	"strings"
	"testing"
)

func TestYaadCmdRuns(t *testing.T) {
	old := yaadLimit
	defer func() { yaadLimit = old }()
	yaadLimit = 3
	if err := yaadCmd.RunE(yaadCmd, nil); err != nil {
		t.Fatalf("yaad command: %v", err)
	}
}

func TestYaadCmdRejectsInvalidLimit(t *testing.T) {
	old := yaadLimit
	defer func() { yaadLimit = old }()
	yaadLimit = 0
	err := yaadCmd.RunE(yaadCmd, nil)
	if err == nil || !strings.Contains(err.Error(), "limit") {
		t.Fatalf("expected limit error, got %v", err)
	}
}
