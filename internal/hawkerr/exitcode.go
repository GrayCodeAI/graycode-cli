package hawkerr

// Exit-code taxonomy.
//
// hawk historically collapsed every failure to exit code 1, which gives an
// agent or shell script driving `hawk --print` no way to branch on *why* the
// run failed without scraping stderr. The codes below assign a stable,
// documented small-integer meaning to each failure class so a caller can, for
// example, retry on a rate-limit (5) but stop immediately on bad credentials
// (3).
//
// The numbers are part of hawk's public CLI contract — append new codes, do
// not renumber existing ones. Codes are kept below 64 to avoid colliding with
// the shell's 128+signal convention and 126/127 (not-executable / not-found).
const (
	ExitOK           = 0  // success
	ExitGeneral      = 1  // unclassified / general failure
	ExitUsage        = 2  // bad flags or arguments (reserved for the CLI layer)
	ExitAuth         = 3  // missing/invalid/expired credentials, 401/403
	ExitNetwork      = 4  // DNS, connection refused/reset, provider 5xx, TLS
	ExitRateLimit    = 5  // 429 / quota / insufficient credits
	ExitTimeout      = 6  // request timeout, deadline exceeded, cancellation
	ExitToolFailure  = 7  // a tool execution failed or timed out
	ExitPolicyBlock  = 8  // permission/guardrail/policy denial
	ExitConfig       = 9  // malformed settings/config
	ExitContextLimit = 10 // prompt exceeds the model's context window
	ExitNotFound     = 11 // model/endpoint/resource not found (404)
	ExitDiskFull     = 12 // out of disk space or quota
)

// ClassifyExitCode maps an error to a stable exit code from the taxonomy above.
//
// The actual pattern matching lives in classify() so the human-readable
// message (ClassifyErrorMessage) and the exit code never disagree about what
// kind of failure occurred.
//
// A nil error yields ExitOK; an unrecognized error yields ExitGeneral.
func ClassifyExitCode(err error) int {
	return classify(err).exitCode
}
