package engine

import (
	"fmt"
	"strings"
	"sync"
)

// AgentTeam coordinates multiple agents working in parallel on related tasks.
type AgentTeam struct {
	mu        sync.RWMutex
	members   map[string]*TeamMember
	taskBoard *TaskBoard
	mailbox   chan TeamMessage
	nextID    int
}

// TeamMember represents a single agent in the team.
type TeamMember struct {
	ID       string
	Role     string   // "researcher", "implementer", "reviewer"
	Session  *Session
	Status   string // "idle", "working", "done"
	Assigned string // current task ID
}

// TaskBoard holds all tasks for the team.
type TaskBoard struct {
	tasks map[string]*TeamTask
}

// TeamTask represents a unit of work on the task board.
type TeamTask struct {
	ID          string
	Description string
	AssignedTo  string
	Status      string // "pending", "in_progress", "done", "blocked"
	DependsOn   []string
	Result      string
}

// TeamMessage is a message passed between team members.
type TeamMessage struct {
	From    string
	To      string // "*" for broadcast
	Content string
}

// NewAgentTeam creates an empty team with a task board and message mailbox.
func NewAgentTeam() *AgentTeam {
	return &AgentTeam{
		members: make(map[string]*TeamMember),
		taskBoard: &TaskBoard{
			tasks: make(map[string]*TeamTask),
		},
		mailbox: make(chan TeamMessage, 256),
	}
}

// AddMember registers a new team member with the given ID and role.
func (at *AgentTeam) AddMember(id, role string, session *Session) {
	at.mu.Lock()
	defer at.mu.Unlock()
	at.members[id] = &TeamMember{
		ID:      id,
		Role:    role,
		Session: session,
		Status:  "idle",
	}
}

// GetMember returns a team member by ID, or nil if not found.
func (at *AgentTeam) GetMember(id string) *TeamMember {
	at.mu.RLock()
	defer at.mu.RUnlock()
	return at.members[id]
}

// CreateTask adds a new task to the task board and returns its ID.
func (at *AgentTeam) CreateTask(desc string, deps []string) string {
	at.mu.Lock()
	defer at.mu.Unlock()
	at.nextID++
	id := fmt.Sprintf("task_%d", at.nextID)

	if deps == nil {
		deps = []string{}
	}

	at.taskBoard.tasks[id] = &TeamTask{
		ID:          id,
		Description: desc,
		Status:      "pending",
		DependsOn:   deps,
	}
	return id
}

// AssignTask assigns a task to a team member.
func (at *AgentTeam) AssignTask(taskID, memberID string) {
	at.mu.Lock()
	defer at.mu.Unlock()

	task, ok := at.taskBoard.tasks[taskID]
	if !ok {
		return
	}
	member, ok := at.members[memberID]
	if !ok {
		return
	}

	task.AssignedTo = memberID
	task.Status = "in_progress"
	member.Status = "working"
	member.Assigned = taskID
}

// CompleteTask marks a task as done and records its result.
func (at *AgentTeam) CompleteTask(taskID, result string) {
	at.mu.Lock()
	defer at.mu.Unlock()

	task, ok := at.taskBoard.tasks[taskID]
	if !ok {
		return
	}
	task.Status = "done"
	task.Result = result

	// Mark the member as idle
	if member, ok := at.members[task.AssignedTo]; ok {
		member.Status = "idle"
		member.Assigned = ""
	}
}

// SendMessage delivers a message to the team mailbox.
func (at *AgentTeam) SendMessage(msg TeamMessage) {
	select {
	case at.mailbox <- msg:
	default:
		// mailbox full, drop message
	}
}

// PendingMessages returns all messages addressed to the given member ID
// (or broadcast messages). It drains matching messages from the mailbox.
func (at *AgentTeam) PendingMessages(memberID string) []TeamMessage {
	var pending []TeamMessage
	var requeue []TeamMessage

	// Drain the channel
	for {
		select {
		case msg := <-at.mailbox:
			if msg.To == memberID || msg.To == "*" {
				pending = append(pending, msg)
			} else {
				requeue = append(requeue, msg)
			}
		default:
			goto done
		}
	}
done:
	// Put back messages for other members
	for _, msg := range requeue {
		select {
		case at.mailbox <- msg:
		default:
		}
	}
	return pending
}

// Status returns a formatted summary of the team and task board.
func (at *AgentTeam) Status() string {
	at.mu.RLock()
	defer at.mu.RUnlock()

	var b strings.Builder
	b.WriteString("=== Team Members ===\n")
	if len(at.members) == 0 {
		b.WriteString("  (no members)\n")
	}
	for _, m := range at.members {
		b.WriteString(fmt.Sprintf("  %s [%s] status=%s", m.ID, m.Role, m.Status))
		if m.Assigned != "" {
			b.WriteString(fmt.Sprintf(" assigned=%s", m.Assigned))
		}
		b.WriteString("\n")
	}

	b.WriteString("\n=== Task Board ===\n")
	if len(at.taskBoard.tasks) == 0 {
		b.WriteString("  (no tasks)\n")
	}
	for _, t := range at.taskBoard.tasks {
		b.WriteString(fmt.Sprintf("  %s [%s] %s", t.ID, t.Status, t.Description))
		if t.AssignedTo != "" {
			b.WriteString(fmt.Sprintf(" (assigned to %s)", t.AssignedTo))
		}
		if len(t.DependsOn) > 0 {
			b.WriteString(fmt.Sprintf(" deps=%v", t.DependsOn))
		}
		if t.Result != "" {
			b.WriteString(fmt.Sprintf(" result=%q", t.Result))
		}
		b.WriteString("\n")
	}
	return b.String()
}
