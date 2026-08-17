package eventlog

import (
	"testing"
)

func TestTurnEndToStopReason(t *testing.T) {
	tests := []struct {
		reason string
		want   ACPStopReason
	}{
		{"completed", ACPStopEndTurn},
		{"max-tokens", ACPStopMaxTokens},
		{"interrupted", ACPStopCancelled},
		{"aborted", ACPStopEndTurn},
		{"blocked", ACPStopEndTurn},
		{"error", ACPStopEndTurn},
		{"unknown", ACPStopEndTurn},
	}
	for _, tt := range tests {
		got := ACPStopReason(TurnEndToStopReason(tt.reason))
		if got != tt.want {
			t.Errorf("TurnEndToStopReason(%q) = %q, want %q", tt.reason, got, tt.want)
		}
	}
}
