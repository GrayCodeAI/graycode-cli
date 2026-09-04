package engine

import (
	"os"
)

// redactToolResult strips secrets from tool output before the result is fed
// back to the model. It uses the session pipeline's OutputRedactor (graycode's 25+
// built-in patterns plus registered environment secrets) and collapses the
// user's home directory so absolute paths do not leak. A session without a
// pipeline (tests, zero-value Session) passes output through unchanged.
func (s *Session) redactToolResult(output string) string {
	if s == nil {
		return output
	}
	life := s.LifecycleSvc()
	if life == nil {
		return output
	}
	pipeline := life.Pipeline()
	if pipeline == nil || pipeline.OutputRedactor == nil {
		return output
	}
	// Idempotent: imports values of secret-named environment variables into
	// the known-secrets table so tool output that echoes them is redacted.
	pipeline.OutputRedactor.RegisterEnvSecrets()
	home, _ := os.UserHomeDir()
	redacted := pipeline.OutputRedactor.Redact(output)
	redacted = pipeline.OutputRedactor.RedactEnvVars(redacted)
	return pipeline.OutputRedactor.RedactPaths(redacted, home)
}
