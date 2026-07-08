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
