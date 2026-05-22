// Package lifecycle is the Stage-1 namespace for session lifecycle, limits,
// timeouts, and sleep-time operations. After Stage 2 the implementation lives
// here and the engine root re-exports the public API. See ../REFACTOR_PLAN.md.
//
// Note: engine.go (the Engine type itself) stays in the root engine package
// as the coordinator — it is NOT re-exported here. This cluster covers the
// supporting lifecycle infrastructure only.
package lifecycle
