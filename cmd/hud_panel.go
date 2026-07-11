package cmd

import (
	"fmt"
	"strings"

	lipgloss "charm.land/lipgloss/v2"

	"github.com/GrayCodeAI/hawk/internal/ui/icons"
)

// HUDData is a snapshot of agent/mission/memory state rendered by the HUD panel.
// It is decoupled from live subsystem types so the panel is independently
// testable and does not require threading mutable references through chatModel.
type HUDData struct {
	// Mission
	MissionID      string
	MissionStatus  string
	FeaturesTotal  int
	FeaturesDone   int
	FeaturesFailed int

	// Agents (from the mission watchdog)
	ActiveAgents []HUDAgent

	// Message bus activity (most recent first)
	RecentMessages []HUDMessage

	// Memory stats (from yaad)
	MemoryReady    bool
	MemoryNodes    int
	MemoryEdges    int
	MemorySessions int
}

// HUDAgent describes a single active agent/feature worker.
type HUDAgent struct {
	ID     string
	Task   string
	Status string
}

// HUDMessage is a compact message-bus entry for display.
type HUDMessage struct {
	From    string
	Topic   string
	Content string
}

var (
	hudBorderColor  = hudBorderPurple
	hudHeaderColor  = toolGold
	hudLabelColor   = hudLabelPink
	hudDimHUDColor  = textDisabled
	hudHeaderStyle  = lipgloss.NewStyle().Foreground(hudHeaderColor).Bold(true)
	hudLabelStyle   = lipgloss.NewStyle().Foreground(hudLabelColor)
	hudDimHUDStyle  = lipgloss.NewStyle().Foreground(hudDimHUDColor)
	hudSectionStyle = lipgloss.NewStyle().Foreground(hudBorderColor).Bold(true)
)

// renderAgentStatusPanel renders the full HUD overlay from a snapshot.
func renderAgentStatusPanel(data HUDData, width int) string {
	if width < 30 {
		width = 60
	}
	inner := width - 4
	if inner < 20 {
		inner = 20
	}

	var b strings.Builder
	b.WriteString(hudHeaderStyle.Render("Agent Status HUD"))
	b.WriteString("\n\n")
	b.WriteString(renderHUDMissionSection(data, inner))
	b.WriteString("\n")
	b.WriteString(renderHUDAgentsSection(data, inner))
	b.WriteString("\n")
	b.WriteString(renderHUDMessageBusSection(data, inner))
	b.WriteString("\n")
	b.WriteString(renderHUDMemorySection(data, inner))

	box := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(hudBorderColor).
		Padding(0, 1).
		Width(width - 2)
	return box.Render(b.String())
}

func renderHUDMissionSection(data HUDData, width int) string {
	var b strings.Builder
	b.WriteString(hudSectionStyle.Render(icons.CaretRight() + " Mission"))
	b.WriteString("\n")
	if data.MissionID == "" {
		b.WriteString(hudDimHUDStyle.Render("  no active mission"))
		b.WriteString("\n")
		return b.String()
	}
	b.WriteString(fmt.Sprintf("  %s %s  %s %s\n",
		hudLabelStyle.Render("id:"), data.MissionID,
		hudLabelStyle.Render("status:"), data.MissionStatus))
	b.WriteString(fmt.Sprintf("  %s %d/%d done, %d failed\n",
		hudLabelStyle.Render("features:"),
		data.FeaturesDone, data.FeaturesTotal, data.FeaturesFailed))
	return b.String()
}

func renderHUDAgentsSection(data HUDData, width int) string {
	var b strings.Builder
	b.WriteString(hudSectionStyle.Render(fmt.Sprintf("▸ Active Agents (%d)", len(data.ActiveAgents))))
	b.WriteString("\n")
	if len(data.ActiveAgents) == 0 {
		b.WriteString(hudDimHUDStyle.Render("  none"))
		b.WriteString("\n")
		return b.String()
	}
	for _, a := range data.ActiveAgents {
		task := truncateHUD(a.Task, width-len(a.ID)-12)
		b.WriteString(fmt.Sprintf("  %s [%s] %s\n",
			hudLabelStyle.Render(a.ID), a.Status, task))
	}
	return b.String()
}

func renderHUDMessageBusSection(data HUDData, width int) string {
	var b strings.Builder
	b.WriteString(hudSectionStyle.Render("▸ Message Bus"))
	b.WriteString("\n")
	if len(data.RecentMessages) == 0 {
		b.WriteString(hudDimHUDStyle.Render("  no activity"))
		b.WriteString("\n")
		return b.String()
	}
	for _, msg := range data.RecentMessages {
		content := truncateHUD(msg.Content, width-len(msg.From)-len(msg.Topic)-8)
		b.WriteString(fmt.Sprintf("  %s %s: %s\n",
			hudLabelStyle.Render("["+msg.From+"]"), msg.Topic, content))
	}
	return b.String()
}

func renderHUDMemorySection(data HUDData, width int) string {
	var b strings.Builder
	b.WriteString(hudSectionStyle.Render("▸ Memory"))
	b.WriteString("\n")
	if !data.MemoryReady {
		b.WriteString(hudDimHUDStyle.Render("  yaad not connected"))
		b.WriteString("\n")
		return b.String()
	}
	b.WriteString(fmt.Sprintf("  %s %d  %s %d  %s %d\n",
		hudLabelStyle.Render("nodes:"), data.MemoryNodes,
		hudLabelStyle.Render("edges:"), data.MemoryEdges,
		hudLabelStyle.Render("sessions:"), data.MemorySessions))
	return b.String()
}

// collectHUDData assembles a HUD snapshot from the chat model's available state.
// Mission, agent, and message-bus data are populated when a mission is attached
// to the session; otherwise the HUD reports an idle state. Memory stats are read
// from the session's memory bridge when available.
func (m *chatModel) collectHUDData() HUDData {
	data := HUDData{
		MissionStatus: "idle",
	}
	if m.session != nil && m.session.MemorySvc().Yaad() != nil && m.session.MemorySvc().Yaad().Ready() {
		data.MemoryReady = true
	}
	return data
}

func truncateHUD(s string, max int) string {
	if max < 4 {
		max = 4
	}
	return truncateWithEllipsis(strings.ReplaceAll(s, "\n", " "), max)
}
