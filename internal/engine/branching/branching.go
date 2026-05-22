package branching

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"strings"
	"sync"
	"time"
)

// BranchMessage represents a single message within a conversation branch.
type BranchMessage struct {
	Role      string    `json:"role"`
	Content   string    `json:"content"`
	ToolUse   []string  `json:"tool_use,omitempty"`
	Timestamp time.Time `json:"timestamp"`
}

// ConversationBranch represents a named branch in the conversation tree.
type ConversationBranch struct {
	ID        string            `json:"id"`
	ParentID  string            `json:"parent_id"`
	Name      string            `json:"name"`
	ForkPoint int               `json:"fork_point"`
	Messages  []BranchMessage   `json:"messages"`
	CreatedAt time.Time         `json:"created_at"`
	Status    string            `json:"status"`
	Metadata  map[string]string `json:"metadata,omitempty"`
}

// BranchManager manages conversation branches and switching between them.
type BranchManager struct {
	Branches     map[string]*ConversationBranch `json:"branches"`
	ActiveBranch string                         `json:"active_branch"`
	RootBranch   string                         `json:"root_branch"`
	mu           sync.RWMutex
}

// NewBranchManager creates a new BranchManager with a "main" root branch.
func NewBranchManager() *BranchManager {
	rootID := generateBranchID()
	root := &ConversationBranch{
		ID:        rootID,
		ParentID:  "",
		Name:      "main",
		ForkPoint: 0,
		Messages:  []BranchMessage{},
		CreatedAt: time.Now().UTC(),
		Status:    "active",
		Metadata:  make(map[string]string),
	}

	bm := &BranchManager{
		Branches:     make(map[string]*ConversationBranch),
		ActiveBranch: rootID,
		RootBranch:   rootID,
	}
	bm.Branches[rootID] = root
	return bm
}

// Fork creates a new branch from the current active branch at the specified message index.
// Messages up to atMessage are copied into the new branch. The new branch becomes active.
func (bm *BranchManager) Fork(name string, atMessage int) (*ConversationBranch, error) {
	bm.mu.Lock()
	defer bm.mu.Unlock()

	active, ok := bm.Branches[bm.ActiveBranch]
	if !ok {
		return nil, fmt.Errorf("active branch %q not found", bm.ActiveBranch)
	}

	if atMessage < 0 {
		return nil, fmt.Errorf("fork point %d is negative", atMessage)
	}
	if atMessage > len(active.Messages) {
		return nil, fmt.Errorf("fork point %d exceeds message count %d", atMessage, len(active.Messages))
	}

	newID := generateBranchID()
	copied := make([]BranchMessage, atMessage)
	copy(copied, active.Messages[:atMessage])

	branch := &ConversationBranch{
		ID:        newID,
		ParentID:  bm.ActiveBranch,
		Name:      name,
		ForkPoint: atMessage,
		Messages:  copied,
		CreatedAt: time.Now().UTC(),
		Status:    "active",
		Metadata:  make(map[string]string),
	}

	bm.Branches[newID] = branch
	bm.ActiveBranch = newID
	return branch, nil
}

// Switch changes the active branch to the specified branch ID.
func (bm *BranchManager) Switch(branchID string) error {
	bm.mu.Lock()
	defer bm.mu.Unlock()

	branch, ok := bm.Branches[branchID]
	if !ok {
		return fmt.Errorf("branch %q not found", branchID)
	}
	if branch.Status == "abandoned" {
		return fmt.Errorf("cannot switch to abandoned branch %q", branchID)
	}

	bm.ActiveBranch = branchID
	return nil
}

// Merge merges the source branch into the currently active branch.
// Messages after the fork point are appended to the active branch.
// The source branch is marked as "merged".
func (bm *BranchManager) Merge(sourceBranchID string) error {
	bm.mu.Lock()
	defer bm.mu.Unlock()

	if sourceBranchID == bm.ActiveBranch {
		return fmt.Errorf("cannot merge branch into itself")
	}

	source, ok := bm.Branches[sourceBranchID]
	if !ok {
		return fmt.Errorf("source branch %q not found", sourceBranchID)
	}

	active, ok := bm.Branches[bm.ActiveBranch]
	if !ok {
		return fmt.Errorf("active branch %q not found", bm.ActiveBranch)
	}

	// Append messages from source that are after the fork point.
	if source.ForkPoint < len(source.Messages) {
		newMessages := source.Messages[source.ForkPoint:]
		active.Messages = append(active.Messages, newMessages...)
	}

	source.Status = "merged"
	return nil
}

// Abandon marks a branch as abandoned.
func (bm *BranchManager) Abandon(branchID string) error {
	bm.mu.Lock()
	defer bm.mu.Unlock()

	if branchID == bm.RootBranch {
		return fmt.Errorf("cannot abandon root branch")
	}

	branch, ok := bm.Branches[branchID]
	if !ok {
		return fmt.Errorf("branch %q not found", branchID)
	}

	branch.Status = "abandoned"

	// If abandoning the active branch, switch back to root.
	if bm.ActiveBranch == branchID {
		bm.ActiveBranch = bm.RootBranch
	}

	return nil
}

// GetBranches returns all branches.
func (bm *BranchManager) GetBranches() []*ConversationBranch {
	bm.mu.RLock()
	defer bm.mu.RUnlock()

	branches := make([]*ConversationBranch, 0, len(bm.Branches))
	for _, b := range bm.Branches {
		branches = append(branches, b)
	}
	return branches
}

// GetActiveMessages returns messages from the currently active branch.
func (bm *BranchManager) GetActiveMessages() []BranchMessage {
	bm.mu.RLock()
	defer bm.mu.RUnlock()

	active, ok := bm.Branches[bm.ActiveBranch]
	if !ok {
		return nil
	}

	result := make([]BranchMessage, len(active.Messages))
	copy(result, active.Messages)
	return result
}

// CompareBranches returns a diff-like comparison of two branches showing
// where they diverged and what each branch did differently.
func (bm *BranchManager) CompareBranches(branchA, branchB string) string {
	bm.mu.RLock()
	defer bm.mu.RUnlock()

	a, okA := bm.Branches[branchA]
	b, okB := bm.Branches[branchB]

	if !okA {
		return fmt.Sprintf("branch %q not found", branchA)
	}
	if !okB {
		return fmt.Sprintf("branch %q not found", branchB)
	}

	aMsgsAfterFork := len(a.Messages) - a.ForkPoint
	bMsgsAfterFork := len(b.Messages) - b.ForkPoint
	if aMsgsAfterFork < 0 {
		aMsgsAfterFork = 0
	}
	if bMsgsAfterFork < 0 {
		bMsgsAfterFork = 0
	}

	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("Branch: %s (%d messages after fork)\n", a.Name, aMsgsAfterFork))
	sb.WriteString(fmt.Sprintf("Branch: %s (%d messages after fork)\n", b.Name, bMsgsAfterFork))
	sb.WriteString("\n")

	// Show divergence point.
	forkIdx := a.ForkPoint
	if forkIdx > 0 && forkIdx <= len(a.Messages) {
		divergeMsg := a.Messages[forkIdx-1].Content
		if len(divergeMsg) > 60 {
			divergeMsg = divergeMsg[:60] + "..."
		}
		sb.WriteString(fmt.Sprintf("Diverged at message %d: %q\n", forkIdx, divergeMsg))
	} else {
		sb.WriteString(fmt.Sprintf("Diverged at message %d\n", forkIdx))
	}

	sb.WriteString("\n")

	// Summarize each branch's post-fork content.
	if aMsgsAfterFork > 0 {
		lastA := a.Messages[len(a.Messages)-1]
		summary := lastA.Content
		if len(summary) > 80 {
			summary = summary[:80] + "..."
		}
		sb.WriteString(fmt.Sprintf("%s: %s\n", a.Name, summary))
	} else {
		sb.WriteString(fmt.Sprintf("%s: (no messages after fork)\n", a.Name))
	}

	if bMsgsAfterFork > 0 {
		lastB := b.Messages[len(b.Messages)-1]
		summary := lastB.Content
		if len(summary) > 80 {
			summary = summary[:80] + "..."
		}
		sb.WriteString(fmt.Sprintf("%s: %s\n", b.Name, summary))
	} else {
		sb.WriteString(fmt.Sprintf("%s: (no messages after fork)\n", b.Name))
	}

	return sb.String()
}

// BuildBranchContext formats branch info suitable for inclusion in a system prompt.
func (bm *BranchManager) BuildBranchContext() string {
	bm.mu.RLock()
	defer bm.mu.RUnlock()

	active, ok := bm.Branches[bm.ActiveBranch]
	if !ok {
		return ""
	}

	var sb strings.Builder
	sb.WriteString("## Conversation Branches\n")
	sb.WriteString(fmt.Sprintf("Active: %s (%d messages)\n", active.Name, len(active.Messages)))

	var others []string
	for _, b := range bm.Branches {
		if b.ID == bm.ActiveBranch {
			continue
		}
		msgsAfterFork := len(b.Messages) - b.ForkPoint
		if msgsAfterFork < 0 {
			msgsAfterFork = 0
		}
		switch b.Status {
		case "abandoned":
			others = append(others, fmt.Sprintf("%s (abandoned)", b.Name))
		case "merged":
			others = append(others, fmt.Sprintf("%s (merged)", b.Name))
		default:
			others = append(others, fmt.Sprintf("%s (%d msgs after fork at msg %d)", b.Name, msgsAfterFork, b.ForkPoint))
		}
	}

	if len(others) > 0 {
		sb.WriteString("Branches: ")
		sb.WriteString(strings.Join(others, ", "))
		sb.WriteString("\n")
	}

	return sb.String()
}

// Prune removes abandoned branches that are older than the specified duration.
func (bm *BranchManager) Prune(olderThan time.Duration) {
	bm.mu.Lock()
	defer bm.mu.Unlock()

	cutoff := time.Now().UTC().Add(-olderThan)
	for id, b := range bm.Branches {
		if b.Status == "abandoned" && b.CreatedAt.Before(cutoff) {
			delete(bm.Branches, id)
		}
	}
}

// ExportBranch serializes the specified branch as JSON.
func (bm *BranchManager) ExportBranch(branchID string) ([]byte, error) {
	bm.mu.RLock()
	defer bm.mu.RUnlock()

	branch, ok := bm.Branches[branchID]
	if !ok {
		return nil, fmt.Errorf("branch %q not found", branchID)
	}

	data, err := json.Marshal(branch)
	if err != nil {
		return nil, fmt.Errorf("marshal branch: %w", err)
	}
	return data, nil
}

// ImportBranch deserializes a branch from JSON and adds it to the manager.
// If a branch with the same ID already exists, a new ID is generated.
func (bm *BranchManager) ImportBranch(data []byte) (*ConversationBranch, error) {
	var branch ConversationBranch
	if err := json.Unmarshal(data, &branch); err != nil {
		return nil, fmt.Errorf("unmarshal branch: %w", err)
	}

	bm.mu.Lock()
	defer bm.mu.Unlock()

	// If ID collides, generate a new one.
	if _, exists := bm.Branches[branch.ID]; exists {
		branch.ID = generateBranchID()
	}

	if branch.Metadata == nil {
		branch.Metadata = make(map[string]string)
	}

	bm.Branches[branch.ID] = &branch
	return &branch, nil
}

// AddMessage appends a message to the currently active branch.
func (bm *BranchManager) AddMessage(role, content string, toolUse []string) {
	bm.mu.Lock()
	defer bm.mu.Unlock()

	active, ok := bm.Branches[bm.ActiveBranch]
	if !ok {
		return
	}

	msg := BranchMessage{
		Role:      role,
		Content:   content,
		ToolUse:   toolUse,
		Timestamp: time.Now().UTC(),
	}
	active.Messages = append(active.Messages, msg)
}

// generateBranchID produces a 16-character hex ID from crypto/rand.
func generateBranchID() string {
	b := make([]byte, 8)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}
