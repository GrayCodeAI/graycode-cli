package cmd

import (
	"testing"
	"time"
)

func TestNotifyCompletion_ShortDuration(t *testing.T) {
	// Should not panic or error when called with short duration
	notifyCompletion(5 * time.Second)
}

func TestNotifyCompletion_LongDuration(t *testing.T) {
	// Should not panic or error when called with long duration
	// (notification is fire-and-forget, so we just verify no crash)
	notifyCompletion(31 * time.Second)
}
