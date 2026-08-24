package cmd

import (
	"fmt"
	"strings"

	hawkconfig "github.com/GrayCodeAI/hawk/internal/config"
	"github.com/GrayCodeAI/hawk/internal/errhint"
	"github.com/GrayCodeAI/hawk/internal/hawkerr"
)

// friendlyErrorMessage returns the user-friendly message for an error.
// Delegates to the shared hawkerr.ClassifyErrorMessage for the base message,
// then enriches specific cases with dynamic hints that require the config
// package (hawkerr can't import internal/config without a cycle).
func friendlyErrorMessage(err error) string {
	msg := hawkerr.ClassifyErrorMessage(err)

	if err == nil {
		return msg
	}

	ec := hawkerr.ClassifyError(err)
	low := strings.ToLower(err.Error())

	switch ec.ExitCode {
	case hawkerr.ExitNotFound:
		// Enrich model-not-found errors with concrete examples from the catalog.
		if strings.Contains(low, "model") || strings.Contains(low, "unknown") ||
			strings.Contains(low, "does not exist") {
			ex1, ex2 := hawkconfig.ExampleModelHints()
			msg = fmt.Sprintf(
				"Model not found. Check your model name with /model.\n  Examples from the eyrie catalog: %s, %s\n  Use /models to list all models, or /config to change provider.",
				ex1, ex2,
			)
		}
	case hawkerr.ExitAuth:
		msg += "\n  Check your API key with /config. Keys can expire or be revoked."
	case hawkerr.ExitNetwork:
		msg += "\n  Check your internet connection. If you're behind a proxy, configure it with /config."
	case hawkerr.ExitTimeout:
		msg += "\n  The request took too long. Try again, or use /model to switch to a faster provider."
	}

	// Provider-specific one-line hint. errhint.Classify is deliberately
	// conservative (gates on a provider-origin marker), so local errors draw no
	// hint here.
	if h := errhint.TUIHint(err); h != "" {
		msg += "\n  " + h
	}

	return msg
}
