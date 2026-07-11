package cmd

import (
	"fmt"
	"strings"
	"sync"
	"time"

	"charm.land/bubbles/v2/viewport"
			tea "charm.land/bubbletea/v2"
		lipgloss "charm.land/lipgloss/v2"
	"github.com/GrayCodeAI/hawk/internal/ui/icons"
)

var (
	// Agent grid borders — each state has its own hue so the eye
	// can read agent state at a glance. Active matches brand (orange),
	// done matches the success palette, fail matches error, idle
	// matches disabled.
	agentActiveStyle = lipgloss.NewStyle().Border(lipgloss.NormalBorder()).BorderForeground(hawkColor).Padding(0, 1)
	agentDoneStyle   = lipgloss.NewStyle().Border(lipgloss.NormalBorder()).BorderForeground(doneGreen).Padding(0, 1)
	agentFailStyle   = lipgloss.NewStyle().Border(lipgloss.NormalBorder()).BorderForeground(errorCoral).Padding(0, 1)
	agentIdleStyle   = lipgloss.NewStyle().Border(lipgloss.NormalBorder()).BorderForeground(textDisabled).Padding(0, 1)
	// Title is a slightly lighter gold than tool gold so the two read
	// as related but distinct (agent = person running, tool = thing
	// being used).
	agentTitleStyle  = lipgloss.NewStyle().Bold(true).Foreground(agentGold)
	agentStatusStyle = lipgloss.NewStyle().Foreground(textMuted)
)

// AgentState represents the current state of a parallel agent.
type AgentState int

const (
	AgentIdle AgentState = iota
	AgentRunning
	AgentDone
	AgentFailed
)

// String returns a human-readable label for the state.
func (s AgentState) String() string {
	switch s {
	case AgentIdle:
		return icons.Hourglass() + " Idle"
	case AgentRunning:
		return icons.Refresh() + " Running"
	case AgentDone:
		return icons.CheckDecagram() + " Done"
	case AgentFailed:
		return icons.Cancel() + " Failed"
	default:
		return "Unknown"
	}
}

// AgentPane represents a single agent's display pane in the grid.
type AgentPane struct {
	ID        string
	Task      string
	State     AgentState
	Output    []string
	viewport  viewport.Model
	mu        sync.Mutex
	startTime time.Time
	endTime   time.Time
	width     int
	height    int
}

// NewAgentPane creates a new agent pane for the given task.
func NewAgentPane(id, task string, width, height int) *AgentPane {
	vp := viewport.New(viewport.WithWidth(width - 2), viewport.WithHeight(height - 3)) // account for border + title
	return &AgentPane{
		ID:       id,
		Task:     task,
		State:    AgentIdle,
		Output:   make([]string, 0),
		viewport: vp,
		width:    width,
		height:   height,
	}
}

// Append adds a line of output to the pane.
func (p *AgentPane) Append(line string) {
	p.mu.Lock()
	defer p.mu.Unlock()

	p.Output = append(p.Output, line)
	// Keep last 100 lines
	if len(p.Output) > 100 {
		p.Output = p.Output[len(p.Output)-100:]
	}
	p.viewport.SetContent(strings.Join(p.Output, "\n"))
	p.viewport.GotoBottom()
}

// SetState updates the agent's state.
func (p *AgentPane) SetState(state AgentState) {
	p.mu.Lock()
	defer p.mu.Unlock()

	p.State = state
	if state == AgentRunning && p.startTime.IsZero() {
		p.startTime = time.Now()
	}
	if state == AgentDone || state == AgentFailed {
		p.endTime = time.Now()
	}
}

// Render draws the agent pane.
func (p *AgentPane) Render() string {
	p.mu.Lock()
	defer p.mu.Unlock()

	// Choose style based on state
	var style lipgloss.Style
	switch p.State {
	case AgentRunning:
		style = agentActiveStyle
	case AgentDone:
		style = agentDoneStyle
	case AgentFailed:
		style = agentFailStyle
	default:
		style = agentIdleStyle
	}

	// Build title bar
	title := agentTitleStyle.Render(fmt.Sprintf("Agent %s", p.ID))
	status := agentStatusStyle.Render(p.State.String())
	elapsed := ""
	if !p.startTime.IsZero() {
		if p.endTime.IsZero() {
			elapsed = fmt.Sprintf(" (%s)", time.Since(p.startTime).Round(time.Second))
		} else {
			elapsed = fmt.Sprintf(" (%s)", p.endTime.Sub(p.startTime).Round(time.Second))
		}
	}
	header := lipgloss.JoinHorizontal(lipgloss.Left, title, " ", status, elapsed)

	// Task description (truncated)
	task := p.Task
	if len(task) > p.width-6 {
		task = task[:p.width-9] + "..."
	}

	// Render viewport
	content := p.viewport.View()

	// Combine
	return style.Width(p.width - 2).Render(
		lipgloss.JoinVertical(
			lipgloss.Left,
			header,
			agentStatusStyle.Render(task),
			content,
		),
	)
}

// AgentGrid manages multiple agent panes in a grid layout.
type AgentGrid struct {
	panes  []*AgentPane
	width  int
	height int
}

// NewAgentGrid creates a grid for displaying multiple agents.
func NewAgentGrid(tasks []string, width, height int) *AgentGrid {
	panes := make([]*AgentPane, len(tasks))
	for i, task := range tasks {
		panes[i] = NewAgentPane(fmt.Sprintf("%d", i+1), task, width/2, height/2)
	}
	return &AgentGrid{
		panes:  panes,
		width:  width,
		height: height,
	}
}

// GetPane returns the pane for the given agent ID.
func (g *AgentGrid) GetPane(id string) *AgentPane {
	for _, p := range g.panes {
		if p.ID == id {
			return p
		}
	}
	return nil
}

// Render draws all agent panes in a grid layout.
func (g *AgentGrid) Render() string {
	if len(g.panes) == 0 {
		return ""
	}

	// Calculate grid dimensions
	cols := 2
	if len(g.panes) == 1 {
		cols = 1
	}
	rows := (len(g.panes) + cols - 1) / cols

	// Pane dimensions
	paneWidth := (g.width - 4) / cols   // 4 for spacing
	paneHeight := (g.height - 4) / rows // 4 for header + spacing

	// Build rows
	var rowStrings []string
	for r := 0; r < rows; r++ {
		var colStrings []string
		for c := 0; c < cols; c++ {
			idx := r*cols + c
			if idx >= len(g.panes) {
				// Empty pane
				colStrings = append(colStrings, strings.Repeat(" ", paneWidth))
				continue
			}
			pane := g.panes[idx]
			pane.width = paneWidth
			pane.height = paneHeight
			pane.viewport.SetWidth(paneWidth - 2)
			pane.viewport.SetHeight(paneHeight - 3)
			colStrings = append(colStrings, pane.Render())
		}
		rowStrings = append(rowStrings, lipgloss.JoinHorizontal(lipgloss.Top, colStrings...))
	}

	return lipgloss.JoinVertical(lipgloss.Left, rowStrings...)
}

// Update processes a message for the grid.
func (g *AgentGrid) Update(msg tea.Msg) tea.Cmd {
	var cmds []tea.Cmd
	for _, p := range g.panes {
		var cmd tea.Cmd
		p.viewport, cmd = p.viewport.Update(msg)
		if cmd != nil {
			cmds = append(cmds, cmd)
		}
	}
	return tea.Batch(cmds...)
}

// AllDone returns true if all agents have completed.
func (g *AgentGrid) AllDone() bool {
	for _, p := range g.panes {
		if p.State != AgentDone && p.State != AgentFailed {
			return false
		}
	}
	return true
}

// Summary returns a summary of all agent states.
func (g *AgentGrid) Summary() string {
	done := 0
	failed := 0
	running := 0
	for _, p := range g.panes {
		switch p.State {
		case AgentDone:
			done++
		case AgentFailed:
			failed++
		case AgentRunning:
			running++
		}
	}
	return fmt.Sprintf("%d done, %d failed, %d running", done, failed, running)
}
