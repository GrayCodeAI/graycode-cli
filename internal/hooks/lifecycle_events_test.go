package hooks

import "testing"

func TestCanonicalKimiLifecycleEvents(t *testing.T) {
	tests := map[string]EventType{
		"UserPromptQueued": EventUserPromptQueued,
		"TurnStarted":      EventTurnStarted,
		"PostToolFailure":  EventPostToolFailure,
		"PermissionResult": EventPermissionResult,
		"SessionHeartbeat": EventSessionHeartbeat,
		"TaskStarted":      EventSubagentTask,
		"StopFailure":      EventStopFailure,
		"Interrupt":        EventInterrupt,
		"Notification":     EventNotification,
	}
	for input, want := range tests {
		if got := EventType(CanonicalEvent(input)); got != want {
			t.Errorf("CanonicalEvent(%q) = %q, want %q", input, got, want)
		}
	}
}
