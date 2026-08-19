package eventlog

// TurnEndToStopReason maps a durable turn-end reason to ACP's (Agent
// Communication Protocol) StopReason vocabulary, ported from DSH's
// acp/codec.ts turnEndToStopReason. `cancelled` is reserved for explicit
// client cancellation and disposal (settled out-of-band); hook aborts report
// end_turn as ordinary quiescence.
func TurnEndToStopReason(reason string) string {
	switch reason {
	case "completed":
		return "end_turn"
	case "max-tokens":
		return "max_tokens"
	case "interrupted":
		return "cancelled"
	case "aborted":
		return "end_turn"
	case "blocked":
		return "end_turn"
	case "error":
		return "end_turn"
	default:
		return "end_turn"
	}
}

// ACPStopReason is the agent client protocol's terminal reason vocabulary.
// Ported from @agentclientprotocol/sdk StopReason.
type ACPStopReason string

const (
	ACPStopEndTurn    ACPStopReason = "end_turn"
	ACPStopMaxTokens  ACPStopReason = "max_tokens"
	ACPStopCancelled  ACPStopReason = "cancelled"
	ACPStopToolCalled ACPStopReason = "tool_called"
	ACPStopError      ACPStopReason = "error"
)
