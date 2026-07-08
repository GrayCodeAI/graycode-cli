package cmd

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"runtime"
	"strings"
	"sync"
	"time"

	"github.com/GrayCodeAI/hawk/internal/ui/icons"
)

// Notification represents a single notification event.
type Notification struct {
	ID        string
	Title     string
	Message   string
	Level     string // "info", "success", "warning", "error"
	Timestamp time.Time
	Read      bool
}

// Notifier manages terminal notifications for hawk events.
type Notifier struct {
	Enabled bool
	Level   string // "all", "important", "critical"
	Sound   bool
	Desktop bool
	History []Notification
	mu      sync.Mutex
}

// NewNotifier creates a new Notifier with default settings.
func NewNotifier() *Notifier {
	return &Notifier{
		Enabled: true,
		Level:   "all",
		Sound:   true,
		Desktop: true,
		History: make([]Notification, 0),
	}
}

// Notify creates a notification and dispatches it via configured methods.
func (n *Notifier) Notify(title, message, level string) {
	if !n.Enabled {
		return
	}

	if !n.shouldNotify(level) {
		return
	}

	notif := Notification{
		ID:        fmt.Sprintf("%d", time.Now().UnixNano()),
		Title:     title,
		Message:   message,
		Level:     level,
		Timestamp: time.Now(),
		Read:      false,
	}

	n.mu.Lock()
	n.History = append(n.History, notif)
	n.mu.Unlock()

	if n.Sound {
		n.Bell()
	}

	if n.Desktop {
		_ = n.DesktopNotify(title, message)
	}

	n.SetTerminalTitle(title)
}

// shouldNotify determines if a notification should fire based on the level filter.
func (n *Notifier) shouldNotify(level string) bool {
	switch n.Level {
	case "all":
		return true
	case "important":
		return level == "warning" || level == "error" || level == "success"
	case "critical":
		return level == "error"
	default:
		return true
	}
}

// NotifyTaskComplete sends a task completion notification.
func (n *Notifier) NotifyTaskComplete(task string, duration time.Duration) {
	message := fmt.Sprintf("Task complete: %s (%s)", task, formatDuration(duration))
	n.Notify("Task Complete", message, "success")
}

// NotifyError sends an error notification.
func (n *Notifier) NotifyError(err string) {
	message := fmt.Sprintf("Error: %s", err)
	n.Notify("Error", message, "error")
}

// NotifyBudgetWarning sends a budget warning notification.
func (n *Notifier) NotifyBudgetWarning(pct float64, remaining float64) {
	message := fmt.Sprintf("Budget %.0f%% used ($%.2f remaining)", pct, remaining)
	n.Notify("Budget Warning", message, "warning")
}

// NotifyCostMilestone sends a notification when spending crosses a cost milestone.
func (n *Notifier) NotifyCostMilestone(cost float64) {
	milestones := []float64{5, 10, 25}
	for _, m := range milestones {
		if cost >= m && cost < m+1 {
			message := fmt.Sprintf("Cost milestone reached: $%.0f", m)
			n.Notify("Cost Milestone", message, "info")
			return
		}
	}
}

// SetTerminalTitle sets the terminal title using escape sequences.
func (n *Notifier) SetTerminalTitle(title string) {
	_, _ = fmt.Fprintf(os.Stdout, "\033]0;%s\007", title)
}

// ClearTitle resets the terminal title.
func (n *Notifier) ClearTitle() {
	_, _ = fmt.Fprintf(os.Stdout, "\033]0;\007")
}

// DesktopNotify sends a desktop notification using OS-specific mechanisms.
func (n *Notifier) DesktopNotify(title, message string) error {
	switch runtime.GOOS {
	case "darwin":
		script := fmt.Sprintf(`display notification "%s" with title "%s"`,
			escapeAppleScript(message), escapeAppleScript(title))
		cmd := exec.CommandContext(context.Background(), "osascript", "-e", script) // #nosec G204 -- fixed command 'osascript'; script built from escaped internal strings
		return cmd.Run()
	case "linux":
		cmd := exec.CommandContext(context.Background(), "notify-send", title, message)
		return cmd.Run()
	case "windows":
		script := fmt.Sprintf(`
[Windows.UI.Notifications.ToastNotificationManager, Windows.UI.Notifications, ContentType = WindowsRuntime] | Out-Null
$template = [Windows.UI.Notifications.ToastNotificationManager]::GetTemplateContent([Windows.UI.Notifications.ToastTemplateType]::ToastText02)
$textNodes = $template.GetElementsByTagName('text')
$textNodes.Item(0).AppendChild($template.CreateTextNode('%s')) | Out-Null
$textNodes.Item(1).AppendChild($template.CreateTextNode('%s')) | Out-Null
$toast = [Windows.UI.Notifications.ToastNotification]::new($template)
[Windows.UI.Notifications.ToastNotificationManager]::CreateToastNotifier('hawk').Show($toast)`,
			escapePowerShell(title), escapePowerShell(message))
		cmd := exec.CommandContext(context.Background(), "powershell", "-Command", script) // #nosec G204 -- fixed command 'powershell'; script built from escaped internal strings
		return cmd.Run()
	default:
		return fmt.Errorf("desktop notifications not supported on %s", runtime.GOOS)
	}
}

// Bell writes the terminal bell character to stdout.
func (n *Notifier) Bell() {
	_, _ = fmt.Fprint(os.Stdout, "\a")
}

// GetUnread returns all unread notifications.
func (n *Notifier) GetUnread() []Notification {
	n.mu.Lock()
	defer n.mu.Unlock()

	var unread []Notification
	for _, notif := range n.History {
		if !notif.Read {
			unread = append(unread, notif)
		}
	}
	return unread
}

// MarkRead marks a notification as read by ID.
func (n *Notifier) MarkRead(id string) {
	n.mu.Lock()
	defer n.mu.Unlock()

	for i := range n.History {
		if n.History[i].ID == id {
			n.History[i].Read = true
			return
		}
	}
}

// GetHistory returns the most recent notifications, limited by count.
func (n *Notifier) GetHistory(limit int) []Notification {
	n.mu.Lock()
	defer n.mu.Unlock()

	if limit <= 0 || limit >= len(n.History) {
		result := make([]Notification, len(n.History))
		copy(result, n.History)
		return result
	}

	start := len(n.History) - limit
	result := make([]Notification, limit)
	copy(result, n.History[start:])
	return result
}

// FormatNotification formats a single notification for display.
func FormatNotification(notif Notification) string {
	icon := notificationIcon(notif.Level)
	timeStr := notif.Timestamp.Format("15:04")
	return fmt.Sprintf("%s [%s] %s", icon, timeStr, notif.Message)
}

// FormatHistory formats a list of notifications for display.
func FormatHistory(notifications []Notification) string {
	if len(notifications) == 0 {
		return "No notifications."
	}

	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("Notifications (%d):\n", len(notifications)))
	for _, notif := range notifications {
		readMarker := " "
		if !notif.Read {
			readMarker = "*"
		}
		sb.WriteString(fmt.Sprintf(" %s %s\n", readMarker, FormatNotification(notif)))
	}
	return sb.String()
}

// notificationIcon returns an icon string based on notification level.
func notificationIcon(level string) string {
	switch level {
	case "info":
		return icons.Bell()
	case "success":
		return icons.CheckDecagram() + " "
	case "warning":
		return icons.Alert() + " "
	case "error":
		return icons.Cancel() + " "
	default:
		return icons.Bell()
	}
}

// formatDuration formats a duration into a human-readable string.
func formatDuration(d time.Duration) string {
	if d < time.Minute {
		return fmt.Sprintf("%.0fs", d.Seconds())
	}
	minutes := int(d.Minutes())
	seconds := int(d.Seconds()) % 60
	if seconds == 0 {
		return fmt.Sprintf("%dm", minutes)
	}
	return fmt.Sprintf("%dm %ds", minutes, seconds)
}

// escapeAppleScript escapes special characters for AppleScript strings.
func escapeAppleScript(s string) string {
	s = strings.ReplaceAll(s, `\`, `\\`)
	s = strings.ReplaceAll(s, `"`, `\"`)
	return s
}

// escapePowerShell escapes single quotes for PowerShell strings.
func escapePowerShell(s string) string {
	return strings.ReplaceAll(s, "'", "''")
}
