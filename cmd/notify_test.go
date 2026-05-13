package cmd

import (
	"strings"
	"testing"
	"time"
)

func TestNewNotifier(t *testing.T) {
	n := NewNotifier()
	if !n.Enabled {
		t.Error("expected Enabled to be true")
	}
	if n.Level != "all" {
		t.Errorf("expected Level 'all', got %q", n.Level)
	}
	if !n.Sound {
		t.Error("expected Sound to be true")
	}
	if !n.Desktop {
		t.Error("expected Desktop to be true")
	}
	if len(n.History) != 0 {
		t.Error("expected empty History")
	}
}

func TestNotificationCreationAndHistory(t *testing.T) {
	n := NewNotifier()
	// Disable sound and desktop to avoid side effects in tests
	n.Sound = false
	n.Desktop = false

	n.Notify("Test Title", "Test message", "info")

	if len(n.History) != 1 {
		t.Fatalf("expected 1 notification in history, got %d", len(n.History))
	}

	notif := n.History[0]
	if notif.Title != "Test Title" {
		t.Errorf("expected title 'Test Title', got %q", notif.Title)
	}
	if notif.Message != "Test message" {
		t.Errorf("expected message 'Test message', got %q", notif.Message)
	}
	if notif.Level != "info" {
		t.Errorf("expected level 'info', got %q", notif.Level)
	}
	if notif.Read {
		t.Error("expected notification to be unread")
	}
	if notif.ID == "" {
		t.Error("expected non-empty ID")
	}
}

func TestLevelFiltering(t *testing.T) {
	tests := []struct {
		filterLevel string
		notifLevel  string
		shouldFire  bool
	}{
		{"all", "info", true},
		{"all", "success", true},
		{"all", "warning", true},
		{"all", "error", true},
		{"important", "info", false},
		{"important", "success", true},
		{"important", "warning", true},
		{"important", "error", true},
		{"critical", "info", false},
		{"critical", "success", false},
		{"critical", "warning", false},
		{"critical", "error", true},
	}

	for _, tt := range tests {
		t.Run(tt.filterLevel+"_"+tt.notifLevel, func(t *testing.T) {
			n := NewNotifier()
			n.Sound = false
			n.Desktop = false
			n.Level = tt.filterLevel

			n.Notify("Test", "msg", tt.notifLevel)

			fired := len(n.History) == 1
			if fired != tt.shouldFire {
				t.Errorf("filter=%q notif=%q: expected fired=%v, got %v",
					tt.filterLevel, tt.notifLevel, tt.shouldFire, fired)
			}
		})
	}
}

func TestUnreadTracking(t *testing.T) {
	n := NewNotifier()
	n.Sound = false
	n.Desktop = false

	n.Notify("One", "msg1", "info")
	n.Notify("Two", "msg2", "info")
	n.Notify("Three", "msg3", "info")

	unread := n.GetUnread()
	if len(unread) != 3 {
		t.Fatalf("expected 3 unread, got %d", len(unread))
	}
}

func TestMarkRead(t *testing.T) {
	n := NewNotifier()
	n.Sound = false
	n.Desktop = false

	n.Notify("One", "msg1", "info")
	n.Notify("Two", "msg2", "info")

	id := n.History[0].ID
	n.MarkRead(id)

	unread := n.GetUnread()
	if len(unread) != 1 {
		t.Fatalf("expected 1 unread after MarkRead, got %d", len(unread))
	}
	if unread[0].Title != "Two" {
		t.Errorf("expected remaining unread to be 'Two', got %q", unread[0].Title)
	}

	// Verify the marked one is actually read
	n.mu.Lock()
	if !n.History[0].Read {
		t.Error("expected first notification to be marked as read")
	}
	n.mu.Unlock()
}

func TestMarkReadNonexistent(t *testing.T) {
	n := NewNotifier()
	n.Sound = false
	n.Desktop = false

	n.Notify("One", "msg1", "info")
	n.MarkRead("nonexistent-id")

	// Should not panic or change anything
	unread := n.GetUnread()
	if len(unread) != 1 {
		t.Fatalf("expected 1 unread, got %d", len(unread))
	}
}

func TestFormatNotification(t *testing.T) {
	notif := Notification{
		ID:        "test-id",
		Title:     "Task Complete",
		Message:   `Task complete: "Fix auth bug" (2m 15s)`,
		Level:     "success",
		Timestamp: time.Date(2024, 1, 15, 10, 30, 0, 0, time.UTC),
		Read:      false,
	}

	result := FormatNotification(notif)

	if !strings.Contains(result, "10:30") {
		t.Errorf("expected time '10:30' in output, got %q", result)
	}
	if !strings.Contains(result, "Fix auth bug") {
		t.Errorf("expected task name in output, got %q", result)
	}
	if !strings.Contains(result, "2m 15s") {
		t.Errorf("expected duration in output, got %q", result)
	}
	if !strings.Contains(result, "✅") {
		t.Errorf("expected success icon in output, got %q", result)
	}
}

func TestFormatNotificationLevels(t *testing.T) {
	levels := map[string]string{
		"info":    "\U0001f514",
		"success": "✅",
		"warning": "⚠️",
		"error":   "❌",
	}

	for level, expectedIcon := range levels {
		notif := Notification{
			ID:        "test",
			Title:     "Test",
			Message:   "test msg",
			Level:     level,
			Timestamp: time.Now(),
		}
		result := FormatNotification(notif)
		if !strings.Contains(result, expectedIcon) {
			t.Errorf("level %q: expected icon %q in %q", level, expectedIcon, result)
		}
	}
}

func TestFormatHistory(t *testing.T) {
	notifications := []Notification{
		{
			ID:        "1",
			Title:     "First",
			Message:   "First message",
			Level:     "info",
			Timestamp: time.Date(2024, 1, 15, 10, 0, 0, 0, time.UTC),
			Read:      true,
		},
		{
			ID:        "2",
			Title:     "Second",
			Message:   "Second message",
			Level:     "error",
			Timestamp: time.Date(2024, 1, 15, 10, 5, 0, 0, time.UTC),
			Read:      false,
		},
	}

	result := FormatHistory(notifications)

	if !strings.Contains(result, "Notifications (2):") {
		t.Errorf("expected header with count, got %q", result)
	}
	if !strings.Contains(result, "First message") {
		t.Error("expected first message in output")
	}
	if !strings.Contains(result, "Second message") {
		t.Error("expected second message in output")
	}
	// Unread marker
	if !strings.Contains(result, "* ") {
		t.Error("expected unread marker '*' for second notification")
	}
}

func TestFormatHistoryEmpty(t *testing.T) {
	result := FormatHistory(nil)
	if result != "No notifications." {
		t.Errorf("expected 'No notifications.', got %q", result)
	}
}

func TestSetTerminalTitle(t *testing.T) {
	// We can't easily capture os.Stdout in a test, but we can verify
	// the escape sequence format by testing the function doesn't panic
	n := NewNotifier()
	n.Sound = false
	n.Desktop = false

	// Just verify it doesn't panic
	n.SetTerminalTitle("hawk: running")
	n.ClearTitle()
}

func TestBellCharacter(t *testing.T) {
	// Verify Bell doesn't panic
	n := NewNotifier()
	n.Bell()
}

func TestDesktopNotifyCommandConstruction(t *testing.T) {
	// We can't actually send desktop notifications in tests,
	// but we can verify the function handles various inputs without panic
	n := NewNotifier()

	// The actual command will likely fail in CI, but should not panic
	err := n.DesktopNotify("Test Title", "Test Message")
	// We don't assert on error because the notification tool may not be available
	_ = err
}

func TestDesktopNotifySpecialCharacters(t *testing.T) {
	n := NewNotifier()
	// Test with special characters that need escaping
	err := n.DesktopNotify(`Title with "quotes"`, `Message with "quotes" and \ backslash`)
	_ = err
}

func TestHistoryLimit(t *testing.T) {
	n := NewNotifier()
	n.Sound = false
	n.Desktop = false

	for i := 0; i < 10; i++ {
		n.Notify("Title", "msg", "info")
	}

	// Get last 3
	history := n.GetHistory(3)
	if len(history) != 3 {
		t.Fatalf("expected 3 notifications, got %d", len(history))
	}

	// Get all with 0
	history = n.GetHistory(0)
	if len(history) != 10 {
		t.Fatalf("expected 10 notifications with limit 0, got %d", len(history))
	}

	// Get all with limit larger than history
	history = n.GetHistory(100)
	if len(history) != 10 {
		t.Fatalf("expected 10 notifications with limit 100, got %d", len(history))
	}
}

func TestNotifyTaskComplete(t *testing.T) {
	n := NewNotifier()
	n.Sound = false
	n.Desktop = false

	n.NotifyTaskComplete("Fix auth bug", 2*time.Minute+15*time.Second)

	if len(n.History) != 1 {
		t.Fatal("expected 1 notification")
	}
	notif := n.History[0]
	if notif.Level != "success" {
		t.Errorf("expected level 'success', got %q", notif.Level)
	}
	if !strings.Contains(notif.Message, "Fix auth bug") {
		t.Errorf("expected task name in message, got %q", notif.Message)
	}
	if !strings.Contains(notif.Message, "2m 15s") {
		t.Errorf("expected duration in message, got %q", notif.Message)
	}
}

func TestNotifyError(t *testing.T) {
	n := NewNotifier()
	n.Sound = false
	n.Desktop = false

	n.NotifyError("connection timeout")

	if len(n.History) != 1 {
		t.Fatal("expected 1 notification")
	}
	notif := n.History[0]
	if notif.Level != "error" {
		t.Errorf("expected level 'error', got %q", notif.Level)
	}
	if !strings.Contains(notif.Message, "connection timeout") {
		t.Errorf("expected error text in message, got %q", notif.Message)
	}
}

func TestNotifyBudgetWarning(t *testing.T) {
	n := NewNotifier()
	n.Sound = false
	n.Desktop = false

	n.NotifyBudgetWarning(80, 5.00)

	if len(n.History) != 1 {
		t.Fatal("expected 1 notification")
	}
	notif := n.History[0]
	if notif.Level != "warning" {
		t.Errorf("expected level 'warning', got %q", notif.Level)
	}
	if !strings.Contains(notif.Message, "80%") {
		t.Errorf("expected percentage in message, got %q", notif.Message)
	}
	if !strings.Contains(notif.Message, "$5.00") {
		t.Errorf("expected remaining amount in message, got %q", notif.Message)
	}
}

func TestNotifyCostMilestone(t *testing.T) {
	n := NewNotifier()
	n.Sound = false
	n.Desktop = false

	n.NotifyCostMilestone(5.50)
	if len(n.History) != 1 {
		t.Fatal("expected 1 notification for $5 milestone")
	}
	if !strings.Contains(n.History[0].Message, "$5") {
		t.Errorf("expected $5 milestone, got %q", n.History[0].Message)
	}

	n.History = nil
	n.NotifyCostMilestone(10.99)
	if len(n.History) != 1 {
		t.Fatal("expected 1 notification for $10 milestone")
	}
	if !strings.Contains(n.History[0].Message, "$10") {
		t.Errorf("expected $10 milestone, got %q", n.History[0].Message)
	}

	n.History = nil
	n.NotifyCostMilestone(25.01)
	if len(n.History) != 1 {
		t.Fatal("expected 1 notification for $25 milestone")
	}
	if !strings.Contains(n.History[0].Message, "$25") {
		t.Errorf("expected $25 milestone, got %q", n.History[0].Message)
	}

	// Below any milestone
	n.History = nil
	n.NotifyCostMilestone(3.00)
	if len(n.History) != 0 {
		t.Error("expected no notification below $5 milestone")
	}

	// Between milestones (e.g., $7 is not a milestone)
	n.History = nil
	n.NotifyCostMilestone(7.00)
	if len(n.History) != 0 {
		t.Error("expected no notification at $7 (between milestones)")
	}
}

func TestNotifierDisabled(t *testing.T) {
	n := NewNotifier()
	n.Enabled = false
	n.Sound = false
	n.Desktop = false

	n.Notify("Test", "msg", "error")

	if len(n.History) != 0 {
		t.Error("expected no notifications when disabled")
	}
}

func TestFormatDuration(t *testing.T) {
	tests := []struct {
		dur      time.Duration
		expected string
	}{
		{30 * time.Second, "30s"},
		{2*time.Minute + 15*time.Second, "2m 15s"},
		{5 * time.Minute, "5m"},
		{1*time.Minute + 1*time.Second, "1m 1s"},
	}

	for _, tt := range tests {
		result := formatDuration(tt.dur)
		if result != tt.expected {
			t.Errorf("formatDuration(%v): expected %q, got %q", tt.dur, tt.expected, result)
		}
	}
}

func TestEscapeAppleScript(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{`hello`, `hello`},
		{`say "hi"`, `say \"hi\"`},
		{`path\to\file`, `path\\to\\file`},
	}

	for _, tt := range tests {
		result := escapeAppleScript(tt.input)
		if result != tt.expected {
			t.Errorf("escapeAppleScript(%q): expected %q, got %q", tt.input, tt.expected, result)
		}
	}
}

func TestEscapePowerShell(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{`hello`, `hello`},
		{`it's`, `it''s`},
		{`can't won't`, `can''t won''t`},
	}

	for _, tt := range tests {
		result := escapePowerShell(tt.input)
		if result != tt.expected {
			t.Errorf("escapePowerShell(%q): expected %q, got %q", tt.input, tt.expected, result)
		}
	}
}

func TestConcurrentNotifications(t *testing.T) {
	n := NewNotifier()
	n.Sound = false
	n.Desktop = false

	done := make(chan struct{})
	for i := 0; i < 50; i++ {
		go func(idx int) {
			n.Notify("Concurrent", "msg", "info")
			done <- struct{}{}
		}(i)
	}

	for i := 0; i < 50; i++ {
		<-done
	}

	if len(n.History) != 50 {
		t.Errorf("expected 50 notifications, got %d", len(n.History))
	}
}
