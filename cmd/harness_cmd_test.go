package cmd

import (
	"testing"
)

func TestHarnessCommandRegistered(t *testing.T) {
	if harnessCmd == nil {
		t.Fatal("Expected harnessCmd to be initialized")
	}

	if harnessCmd.Use != "harness [review|fix]" {
		t.Errorf("Unexpected use string: %s", harnessCmd.Use)
	}

	sub, ok := subcommandRegistry.Lookup("harness")
	if !ok || sub == nil {
		t.Fatal("Expected slash command /harness to be registered in subcommandRegistry")
	}

	if sub.Name() != "harness" {
		t.Errorf("Unexpected subcommand name: %s", sub.Name())
	}
}
