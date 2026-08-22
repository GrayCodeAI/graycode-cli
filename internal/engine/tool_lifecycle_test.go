package engine

import "testing"

func TestToolLifecycleTerminalValues(t *testing.T) {
	t.Parallel()
	if ToolStateValidating == ToolStateCompleted {
		t.Fatal("validation and completion states must differ")
	}
	if ToolReasonTimeout == ToolReasonCompleted {
		t.Fatal("timeout and completion reasons must differ")
	}
}

func TestToolLifecycleEventCarriesTerminalReason(t *testing.T) {
	t.Parallel()
	event := StreamEvent{
		Type:       "tool_result",
		ToolState:  ToolStateTimedOut,
		ToolReason: ToolReasonTimeout,
	}
	if event.ToolState != ToolStateTimedOut {
		t.Fatalf("state = %q, want timed_out", event.ToolState)
	}
	if event.ToolReason != ToolReasonTimeout {
		t.Fatalf("reason = %q, want timeout", event.ToolReason)
	}
}
