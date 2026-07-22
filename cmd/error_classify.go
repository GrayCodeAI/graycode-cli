package cmd

import (
	"fmt"
	"strings"

	hawkconfig "github.com/GrayCodeAI/hawk/internal/config"
	"github.com/GrayCodeAI/hawk/internal/hawkerr"
)

// friendlyErrorMessage returns the user-friendly message for an error.
// Delegates to the shared hawkerr.ClassifyErrorMessage for the base message,
// then enriches specific cases (like model-not-found) with dynamic hints
// that require the config package.
func friendlyErrorMessage(err error) string {
	msg := hawkerr.ClassifyErrorMessage(err)

	// Enrich model-not-found errors with concrete examples from the catalog.
	// The base message from hawkerr lacks these because hawkerr can't import
	// internal/config (would create an import cycle).
	if err != nil {
		ec := hawkerr.ClassifyError(err)
		if ec.ExitCode == hawkerr.ExitNotFound {
			low := strings.ToLower(err.Error())
			if strings.Contains(low, "model") || strings.Contains(low, "unknown") ||
				strings.Contains(low, "does not exist") {
				ex1, ex2 := hawkconfig.ExampleModelHints()
				msg = fmt.Sprintf(
					"Model not found. Check your model name with /model.\n  Examples from the eyrie catalog: %s, %s\n  Use /models to list all models, or /config to change provider.",
					ex1, ex2,
				)
			}
		}
	}

	return msg
}
