package engine

import "github.com/GrayCodeAI/graycode-cli/internal/tool"

// DefaultToolPipeline returns the standard tool interception pipeline for a session.
// It is intentionally empty today: every phase the pipeline is meant to observe —
// permission, approval, blast-radius, tracing, timeout/retry, execution, redaction,
// loop detection, and memory distillation — still runs in its existing, tested
// location. The pipeline is the declared seam for adding or replacing any of those
// phases without touching agentLoop or ToolService hot paths.
//
// Register interceptors with Pipeline().Register(tool.Stage?Stage, fn). The first
// interceptor to short-circuit stops the chain; StagePreExecute can deny a call
// before execution, and StagePostExecute can replace the normalized result.
func DefaultToolPipeline() *tool.Pipeline {
	return tool.NewPipeline()
}
