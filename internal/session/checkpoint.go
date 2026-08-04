package session

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/GrayCodeAI/hawk/internal/engine/token"
)

// ─────────────────────────────────────────────────────────────────────────────
// CheckpointManager — saves conversation state at key moments, enabling
// rollback to any checkpoint.
// ─────────────────────────────────────────────────────────────────────────────

// SessionCheckpoint represents a saved point-in-time state of a conversation,
// enabling rollback to any previously saved checkpoint.
type SessionCheckpoint struct {
	ID           string            `json:"id"`
	Name         string            `json:"name"`
	MessageCount int               `json:"message_count"`
	TokenCount   int               `json:"token_count"`
	FilesState   map[string]string `json:"files_state"` // file path → content hash
	Timestamp    time.Time         `json:"timestamp"`
	Description  string            `json:"description"`
	Auto         bool              `json:"auto"`
	// messages stored separately on disk
}

// CheckpointManager manages session checkpoints with persistence.
type CheckpointManager struct {
	Checkpoints    []*SessionCheckpoint `json:"checkpoints"`
	MaxCheckpoints int                  `json:"max_checkpoints"`
	Dir            string               `json:"dir"`
	mu             sync.RWMutex
}

// NewCheckpointManager creates a CheckpointManager that persists to dir.
func NewCheckpointManager(dir string) *CheckpointManager {
	return &CheckpointManager{
		Checkpoints:    make([]*SessionCheckpoint, 0),
		MaxCheckpoints: 20,
		Dir:            dir,
	}
}

// Create saves a named checkpoint capturing current conversation and file state.
func (cm *CheckpointManager) Create(name, description string, messages []Message, files []string) (*SessionCheckpoint, error) {
	cm.mu.Lock()
	defer cm.mu.Unlock()

	id := generateCheckpointID(name, time.Now())

	filesState := hashFiles(files)

	tokenCount := estimateTokens(messages)

	cp := &SessionCheckpoint{
		ID:           id,
		Name:         name,
		MessageCount: len(messages),
		TokenCount:   tokenCount,
		FilesState:   filesState,
		Timestamp:    time.Now(),
		Description:  description,
		Auto:         false,
	}

	if err := cm.saveMessages(id, messages); err != nil {
		return nil, fmt.Errorf("save checkpoint messages: %w", err)
	}

	if err := cm.saveFileContents(id, files); err != nil {
		return nil, fmt.Errorf("save checkpoint files: %w", err)
	}

	cm.Checkpoints = append(cm.Checkpoints, cp)

	if len(cm.Checkpoints) > cm.MaxCheckpoints {
		cm.pruneUnlocked()
	}

	if err := cm.saveIndex(); err != nil {
		return nil, fmt.Errorf("save checkpoint index: %w", err)
	}

	return cp, nil
}

// AutoCheckpoint creates an automatic checkpoint named by timestamp.
func (cm *CheckpointManager) AutoCheckpoint(messages []Message, files []string) (*SessionCheckpoint, error) {
	name := fmt.Sprintf("auto-checkpoint-%s", time.Now().Format("150405"))
	description := fmt.Sprintf("Automatic checkpoint at %d messages", len(messages))
	cm.mu.Lock()
	defer cm.mu.Unlock()

	id := generateCheckpointID(name, time.Now())
	filesState := hashFiles(files)
	tokenCount := estimateTokens(messages)

	cp := &SessionCheckpoint{
		ID:           id,
		Name:         name,
		MessageCount: len(messages),
		TokenCount:   tokenCount,
		FilesState:   filesState,
		Timestamp:    time.Now(),
		Description:  description,
		Auto:         true,
	}

	if err := cm.saveMessages(id, messages); err != nil {
		return nil, fmt.Errorf("save auto-checkpoint messages: %w", err)
	}

	if err := cm.saveFileContents(id, files); err != nil {
		return nil, fmt.Errorf("save auto-checkpoint files: %w", err)
	}

	cm.Checkpoints = append(cm.Checkpoints, cp)

	if len(cm.Checkpoints) > cm.MaxCheckpoints {
		cm.pruneUnlocked()
	}

	if err := cm.saveIndex(); err != nil {
		return nil, fmt.Errorf("save auto-checkpoint index: %w", err)
	}

	return cp, nil
}

// Restore loads a checkpoint and restores file states, returning the messages
// at that point.
func (cm *CheckpointManager) Restore(checkpointID string) ([]Message, error) {
	cm.mu.RLock()
	var cp *SessionCheckpoint
	for _, c := range cm.Checkpoints {
		if c.ID == checkpointID {
			cp = c
			break
		}
	}
	cm.mu.RUnlock()

	if cp == nil {
		return nil, fmt.Errorf("checkpoint %q not found", checkpointID)
	}

	// Load messages from disk
	messages, err := cm.loadMessages(checkpointID)
	if err != nil {
		return nil, fmt.Errorf("load checkpoint messages: %w", err)
	}

	// Restore file contents
	if err := cm.restoreFileContents(checkpointID, cp.FilesState); err != nil {
		return nil, fmt.Errorf("restore file contents: %w", err)
	}

	return messages, nil
}

// List returns all checkpoints in chronological order.
func (cm *CheckpointManager) List() []*SessionCheckpoint {
	cm.mu.RLock()
	defer cm.mu.RUnlock()

	out := make([]*SessionCheckpoint, len(cm.Checkpoints))
	copy(out, cm.Checkpoints)
	return out
}

// Get returns a checkpoint by ID, or nil if not found.
func (cm *CheckpointManager) Get(id string) *SessionCheckpoint {
	cm.mu.RLock()
	defer cm.mu.RUnlock()

	for _, cp := range cm.Checkpoints {
		if cp.ID == id {
			return cp
		}
	}
	return nil
}

// Delete removes a checkpoint by ID.
func (cm *CheckpointManager) Delete(id string) error {
	cm.mu.Lock()
	defer cm.mu.Unlock()

	idx := -1
	for i, cp := range cm.Checkpoints {
		if cp.ID == id {
			idx = i
			break
		}
	}
	if idx == -1 {
		return fmt.Errorf("checkpoint %q not found", id)
	}

	// Remove checkpoint data from disk
	cpDir := filepath.Join(cm.Dir, id)
	_ = os.RemoveAll(cpDir)

	cm.Checkpoints = append(cm.Checkpoints[:idx], cm.Checkpoints[idx+1:]...)

	return cm.saveIndex()
}

// ShouldAutoCheckpoint returns true if an auto-checkpoint should be taken
// based on message count or tool call count thresholds.
func (cm *CheckpointManager) ShouldAutoCheckpoint(messageCount, toolCalls int) bool {
	cm.mu.RLock()
	defer cm.mu.RUnlock()

	if len(cm.Checkpoints) == 0 {
		// Checkpoint after first threshold
		return messageCount >= 10 || toolCalls >= 5
	}

	last := cm.Checkpoints[len(cm.Checkpoints)-1]
	msgsSince := messageCount - last.MessageCount

	return msgsSince >= 10 || toolCalls >= 5
}

// DiffFromCheckpoint returns a list of files that changed since the given
// checkpoint.
func (cm *CheckpointManager) DiffFromCheckpoint(checkpointID string, currentFiles []string) []string {
	cm.mu.RLock()
	defer cm.mu.RUnlock()

	var cp *SessionCheckpoint
	for _, c := range cm.Checkpoints {
		if c.ID == checkpointID {
			cp = c
			break
		}
	}
	if cp == nil {
		return nil
	}

	currentState := hashFiles(currentFiles)
	var changed []string

	// Files in current that differ from checkpoint
	for file, hash := range currentState {
		if oldHash, exists := cp.FilesState[file]; !exists || oldHash != hash {
			changed = append(changed, file)
		}
	}

	// Files in checkpoint that are missing from current
	currentSet := make(map[string]bool)
	for _, f := range currentFiles {
		currentSet[f] = true
	}
	for file := range cp.FilesState {
		if !currentSet[file] {
			changed = append(changed, file+" (deleted)")
		}
	}

	sort.Strings(changed)
	return changed
}

// FormatCheckpoints returns a human-readable display of all checkpoints.
func (cm *CheckpointManager) FormatCheckpoints() string {
	cm.mu.RLock()
	defer cm.mu.RUnlock()

	if len(cm.Checkpoints) == 0 {
		return "No checkpoints."
	}

	var b strings.Builder
	b.WriteString(fmt.Sprintf("Checkpoints (%d):\n", len(cm.Checkpoints)))
	b.WriteString(strings.Repeat("─", 17))
	b.WriteString("\n")

	now := time.Now()
	for i, cp := range cm.Checkpoints {
		age := formatAge(now.Sub(cp.Timestamp))
		var name string
		if cp.Auto {
			name = "auto-checkpoint"
		} else {
			name = fmt.Sprintf("%q", cp.Name)
		}
		shortID := cp.ID
		if len(shortID) > 6 {
			shortID = shortID[:6]
		}
		b.WriteString(fmt.Sprintf("%d. [%s] %s (%s, %d msgs)\n",
			i+1, shortID, name, age, cp.MessageCount))
	}

	return b.String()
}

// Prune removes old auto-checkpoints, keeping all named checkpoints and the
// most recent auto-checkpoints within MaxCheckpoints total.
func (cm *CheckpointManager) Prune() {
	cm.mu.Lock()
	defer cm.mu.Unlock()
	cm.pruneUnlocked()
	_ = cm.saveIndex()
}

// pruneUnlocked removes excess auto-checkpoints. Must be called with mu held.
func (cm *CheckpointManager) pruneUnlocked() {
	if len(cm.Checkpoints) <= cm.MaxCheckpoints {
		return
	}

	// Separate named and auto checkpoints
	var named []*SessionCheckpoint
	var auto []*SessionCheckpoint
	for _, cp := range cm.Checkpoints {
		if cp.Auto {
			auto = append(auto, cp)
		} else {
			named = append(named, cp)
		}
	}

	// Keep all named checkpoints. Remove oldest auto-checkpoints.
	allowedAuto := cm.MaxCheckpoints - len(named)
	if allowedAuto < 0 {
		allowedAuto = 0
	}

	if len(auto) > allowedAuto {
		// Sort auto by timestamp ascending
		sort.Slice(auto, func(i, j int) bool {
			return auto[i].Timestamp.Before(auto[j].Timestamp)
		})
		// Remove oldest
		toRemove := auto[:len(auto)-allowedAuto]
		auto = auto[len(auto)-allowedAuto:]

		for _, cp := range toRemove {
			cpDir := filepath.Join(cm.Dir, cp.ID)
			_ = os.RemoveAll(cpDir)
		}
	}

	// Rebuild checkpoint list in chronological order
	all := append(named, auto...)
	sort.Slice(all, func(i, j int) bool {
		return all[i].Timestamp.Before(all[j].Timestamp)
	})
	cm.Checkpoints = all
}

// Save persists the checkpoint index to disk.
func (cm *CheckpointManager) Save() error {
	cm.mu.RLock()
	defer cm.mu.RUnlock()
	return cm.saveIndex()
}

// Load reads the checkpoint index from disk.
func (cm *CheckpointManager) Load() error {
	cm.mu.Lock()
	defer cm.mu.Unlock()

	indexPath := filepath.Join(cm.Dir, "checkpoints.json")
	data, err := os.ReadFile(indexPath) // #nosec G304 -- path built from internal checkpoint manager dir, fixed filename
	if err != nil {
		if os.IsNotExist(err) {
			cm.Checkpoints = make([]*SessionCheckpoint, 0)
			return nil
		}
		return fmt.Errorf("read checkpoint index: %w", err)
	}

	var checkpoints []*SessionCheckpoint
	if err := json.Unmarshal(data, &checkpoints); err != nil {
		return fmt.Errorf("parse checkpoint index: %w", err)
	}
	cm.Checkpoints = checkpoints
	return nil
}

// ─── internal helpers ────────────────────────────────────────────────────────

func (cm *CheckpointManager) saveIndex() error {
	if err := os.MkdirAll(cm.Dir, 0o750); err != nil {
		return err
	}
	indexPath := filepath.Join(cm.Dir, "checkpoints.json")
	data, err := json.MarshalIndent(cm.Checkpoints, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(indexPath, data, 0o600)
}

func (cm *CheckpointManager) saveMessages(id string, messages []Message) error {
	cpDir := filepath.Join(cm.Dir, id)
	if err := os.MkdirAll(cpDir, 0o750); err != nil {
		return err
	}
	path := filepath.Join(cpDir, "messages.json")
	data, err := json.Marshal(messages)
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0o600)
}

func (cm *CheckpointManager) loadMessages(id string) ([]Message, error) {
	path := filepath.Join(cm.Dir, id, "messages.json")
	data, err := os.ReadFile(path) // #nosec G304 -- path built from internal checkpoint manager dir + checkpoint ID
	if err != nil {
		return nil, err
	}
	var messages []Message
	if err := json.Unmarshal(data, &messages); err != nil {
		return nil, err
	}
	return messages, nil
}

func (cm *CheckpointManager) saveFileContents(id string, files []string) error {
	cpDir := filepath.Join(cm.Dir, id, "files")
	if err := os.MkdirAll(cpDir, 0o750); err != nil {
		return err
	}

	for _, file := range files {
		content, err := os.ReadFile(file) // #nosec G304 -- file paths come from internal tracked-files list for the current session, not raw external input
		if err != nil {
			continue // skip unreadable files
		}
		// Store with a sanitized name based on hash of path
		safeName := fmt.Sprintf("%x", sha256.Sum256([]byte(file)))
		entry := map[string]string{
			"path":    file,
			"content": string(content),
		}
		data, _ := json.Marshal(entry)
		if err := os.WriteFile(filepath.Join(cpDir, safeName+".json"), data, 0o600); err != nil {
			return err
		}
	}
	return nil
}

func (cm *CheckpointManager) restoreFileContents(id string, filesState map[string]string) error {
	cpDir := filepath.Join(cm.Dir, id, "files")
	entries, err := os.ReadDir(cpDir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}

	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		data, err := os.ReadFile(filepath.Join(cpDir, e.Name())) // #nosec G304 -- path built from internal checkpoint files dir + directory entry name from os.ReadDir
		if err != nil {
			continue
		}
		var entry map[string]string
		if err := json.Unmarshal(data, &entry); err != nil {
			continue
		}
		filePath := entry["path"]
		content := entry["content"]
		if filePath == "" {
			continue
		}
		// Ensure parent directory exists
		if err := os.MkdirAll(filepath.Dir(filePath), 0o750); err != nil {
			continue
		}
		if err := os.WriteFile(filePath, []byte(content), 0o600); err != nil {
			return fmt.Errorf("restore file %s: %w", filePath, err)
		}
	}
	return nil
}

// generateCheckpointID creates a short unique ID from name and time.
func generateCheckpointID(name string, t time.Time) string {
	h := sha256.New()
	h.Write([]byte(name))
	h.Write([]byte(t.Format(time.RFC3339Nano)))
	return fmt.Sprintf("%x", h.Sum(nil))[:12]
}

// hashFiles computes SHA-256 hashes for a list of file paths.
func hashFiles(files []string) map[string]string {
	state := make(map[string]string)
	for _, file := range files {
		content, err := os.ReadFile(file) // #nosec G304 -- file paths come from internal tracked-files list for the current session, not raw external input
		if err != nil {
			continue
		}
		hash := sha256.Sum256(content)
		state[file] = fmt.Sprintf("%x", hash)
	}
	return state
}

func estimateTokens(messages []Message) int {
	var b strings.Builder
	for _, msg := range messages {
		b.WriteString(msg.Content)
		for _, tc := range msg.ToolUse {
			b.WriteString(tc.Name)
		}
		for _, tr := range msg.ToolResults {
			b.WriteString(tr.Content)
		}
	}
	total := token.CountTokensFast(b.String())
	if total == 0 && len(messages) > 0 {
		total = len(messages)
	}
	return total
}

// formatAge returns a human-readable relative time string.
func formatAge(d time.Duration) string {
	switch {
	case d < time.Minute:
		return "now"
	case d < time.Hour:
		mins := int(d.Minutes())
		return fmt.Sprintf("%dm ago", mins)
	case d < 24*time.Hour:
		hours := int(d.Hours())
		return fmt.Sprintf("%dh ago", hours)
	default:
		days := int(d.Hours() / 24)
		return fmt.Sprintf("%dd ago", days)
	}
}

// ─────────────────────────────────────────────────────────────────────────────
// SmartCheckpointer — event-driven checkpointer that wraps SnapshotStore.
// Takes snapshots only when meaningful state changes occur.
// ─────────────────────────────────────────────────────────────────────────────

// CheckpointTrigger classifies events that may trigger a checkpoint.
type CheckpointTrigger int

const (
	TriggerFileWrite    CheckpointTrigger = iota // file was modified
	TriggerToolError                             // tool execution failed
	TriggerUserFeedback                          // user gave correction
	TriggerPlanChange                            // plan/subtask status changed
	TriggerContextShift                          // topic changed significantly
)

// String returns a human-readable name for the trigger.
func (ct CheckpointTrigger) String() string {
	switch ct {
	case TriggerFileWrite:
		return "file_write"
	case TriggerToolError:
		return "tool_error"
	case TriggerUserFeedback:
		return "user_feedback"
	case TriggerPlanChange:
		return "plan_change"
	case TriggerContextShift:
		return "context_shift"
	default:
		return "unknown"
	}
}

// SmartCheckpointer takes snapshots only when meaningful state changes occur,
// filtering out redundant checkpoints.
type SmartCheckpointer struct {
	mu          sync.Mutex
	store       *SnapshotStore
	lastContent string // hash of last checkpointed state
	triggers    map[CheckpointTrigger]bool

	// stats
	eventsSeen       int
	checkpointsTaken int
	eventsFiltered   int
}

// NewSmartCheckpointer creates a checkpointer that wraps a SnapshotStore.
// All trigger types are enabled by default.
func NewSmartCheckpointer(store *SnapshotStore) *SmartCheckpointer {
	return &SmartCheckpointer{
		store: store,
		triggers: map[CheckpointTrigger]bool{
			TriggerFileWrite:    true,
			TriggerToolError:    true,
			TriggerUserFeedback: true,
			TriggerPlanChange:   true,
			TriggerContextShift: true,
		},
	}
}

// SetTrigger enables or disables a specific trigger type.
func (sc *SmartCheckpointer) SetTrigger(trigger CheckpointTrigger, enabled bool) {
	sc.mu.Lock()
	defer sc.mu.Unlock()
	sc.triggers[trigger] = enabled
}

// ShouldCheckpoint returns true only if the state has meaningfully changed
// since the last checkpoint.
func (sc *SmartCheckpointer) ShouldCheckpoint(event CheckpointTrigger, session *Session) bool {
	sc.mu.Lock()
	defer sc.mu.Unlock()

	// Check if trigger type is enabled.
	if !sc.triggers[event] {
		return false
	}

	// Compute content hash of the current session state.
	currentHash := sessionContentHash(session)

	// If the hash matches the last checkpoint, state hasn't changed.
	if currentHash == sc.lastContent {
		return false
	}

	return true
}

// OnEvent processes a checkpoint event. If meaningful state change is detected,
// it takes a snapshot with the provided action label.
func (sc *SmartCheckpointer) OnEvent(event CheckpointTrigger, session *Session, action string) {
	sc.mu.Lock()
	sc.eventsSeen++

	// Check if trigger type is enabled.
	if !sc.triggers[event] {
		sc.eventsFiltered++
		sc.mu.Unlock()
		return
	}

	// Compute content hash.
	currentHash := sessionContentHash(session)
	if currentHash == sc.lastContent {
		sc.eventsFiltered++
		sc.mu.Unlock()
		return
	}

	sc.lastContent = currentHash
	sc.checkpointsTaken++
	store := sc.store
	sc.mu.Unlock()

	// Take the snapshot outside the lock (it does its own I/O).
	label := fmt.Sprintf("[%s] %s", event, action)
	if store != nil {
		_ = store.Take(label, session)
	}
}

// Stats returns a human-readable summary of checkpoint activity.
func (sc *SmartCheckpointer) Stats() string {
	sc.mu.Lock()
	defer sc.mu.Unlock()

	if sc.eventsSeen == 0 {
		return "0 events seen, 0 checkpoints taken"
	}
	filteredPct := float64(sc.eventsSeen-sc.checkpointsTaken) / float64(sc.eventsSeen) * 100
	return fmt.Sprintf("%d events seen, %d checkpoints taken (%.0f%% filtered)",
		sc.eventsSeen, sc.checkpointsTaken, filteredPct)
}

// EventsSeen returns the total number of events processed.
func (sc *SmartCheckpointer) EventsSeen() int {
	sc.mu.Lock()
	defer sc.mu.Unlock()
	return sc.eventsSeen
}

// CheckpointsTaken returns the number of checkpoints actually created.
func (sc *SmartCheckpointer) CheckpointsTaken() int {
	sc.mu.Lock()
	defer sc.mu.Unlock()
	return sc.checkpointsTaken
}

// sessionContentHash computes a deterministic hash of the session's message
// content to detect meaningful state changes.
func sessionContentHash(session *Session) string {
	if session == nil {
		return ""
	}
	h := sha256.New()
	for _, msg := range session.Messages {
		h.Write([]byte(msg.Role))
		h.Write([]byte(msg.Content))
		for _, tc := range msg.ToolUse {
			h.Write([]byte(tc.Name))
			h.Write([]byte(tc.ID))
		}
		for _, tr := range msg.ToolResults {
			h.Write([]byte(tr.Content))
		}
	}
	return fmt.Sprintf("%x", h.Sum(nil))
}

// FormatTriggers returns a summary of which triggers are enabled.
func (sc *SmartCheckpointer) FormatTriggers() string {
	sc.mu.Lock()
	defer sc.mu.Unlock()

	var enabled []string
	var disabled []string
	allTriggers := []CheckpointTrigger{
		TriggerFileWrite, TriggerToolError, TriggerUserFeedback,
		TriggerPlanChange, TriggerContextShift,
	}
	for _, t := range allTriggers {
		if sc.triggers[t] {
			enabled = append(enabled, t.String())
		} else {
			disabled = append(disabled, t.String())
		}
	}

	var b strings.Builder
	b.WriteString(fmt.Sprintf("Enabled: %s", strings.Join(enabled, ", ")))
	if len(disabled) > 0 {
		b.WriteString(fmt.Sprintf("\nDisabled: %s", strings.Join(disabled, ", ")))
	}
	return b.String()
}
