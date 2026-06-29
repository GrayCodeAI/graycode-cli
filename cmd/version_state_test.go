package cmd

import "testing"

func preserveCLICompilerVersionState(t *testing.T) {
	t.Helper()

	oldVersion := version
	oldBuildDate := buildDate
	t.Cleanup(func() {
		version = oldVersion
		buildDate = oldBuildDate
	})
}

func preserveLibraryVersionState(t *testing.T) {
	t.Helper()

	oldVersion := Version
	oldCommit := Commit
	oldDate := Date
	t.Cleanup(func() {
		Version = oldVersion
		Commit = oldCommit
		Date = oldDate
	})
}
