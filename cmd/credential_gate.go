package cmd

import (
	"sync/atomic"

	"github.com/GrayCodeAI/graycode-cli/internal/tool"
)

// credentialGate holds the current host-side credential gate callback. It is
// loaded atomicically so the tool (which may be created before the session
// wires the callback) can read it safely at execution time.
var credentialGate atomic.Value // tool.CredentialGateFn

// SetCredentialGate stores the host-side credential gate callback that the
// RequestCredential tool invokes to prompt the user.
func SetCredentialGate(fn tool.CredentialGateFn) {
	credentialGate.Store(fn)
}
