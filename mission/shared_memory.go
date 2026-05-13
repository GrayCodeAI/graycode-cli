package mission

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"sync"
	"time"
)

// MemEntry represents a single entry in shared memory.
type MemEntry struct {
	Key       string      `json:"key"`
	Value     interface{} `json:"value"`
	Type      string      `json:"type"` // "string", "int", "bool", "json", "list"
	Owner     string      `json:"owner"`
	CreatedAt time.Time   `json:"created_at"`
	UpdatedAt time.Time   `json:"updated_at"`
	Version   int         `json:"version"`
	Readers   []string    `json:"readers,omitempty"`
}

// SharedMemory provides a concurrent-safe key-value store for multi-agent
// workflows, allowing agents to read and write common state without message passing.
type SharedMemory struct {
	Entries    map[string]*MemEntry   `json:"entries"`
	Namespaces map[string][]string   `json:"namespaces"` // namespace -> list of keys
	mu         sync.RWMutex
	watchers   map[string][]chan *MemEntry
}

// NewSharedMemory creates and returns an initialized SharedMemory instance.
func NewSharedMemory() *SharedMemory {
	return &SharedMemory{
		Entries:    make(map[string]*MemEntry),
		Namespaces: make(map[string][]string),
		watchers:   make(map[string][]chan *MemEntry),
	}
}

// Set writes a value into shared memory. If the key already exists, the version
// is incremented and the UpdatedAt timestamp is refreshed. The owner is recorded
// as the agent that last wrote the entry.
func (sm *SharedMemory) Set(key string, value interface{}, owner string) {
	sm.mu.Lock()
	defer sm.mu.Unlock()

	now := time.Now()
	typ := detectType(value)

	if entry, exists := sm.Entries[key]; exists {
		entry.Value = value
		entry.Type = typ
		entry.Owner = owner
		entry.UpdatedAt = now
		entry.Version++
		sm.notifyWatchers(key, entry)
	} else {
		entry := &MemEntry{
			Key:       key,
			Value:     value,
			Type:      typ,
			Owner:     owner,
			CreatedAt: now,
			UpdatedAt: now,
			Version:   1,
			Readers:   []string{},
		}
		sm.Entries[key] = entry
		sm.notifyWatchers(key, entry)
	}
}

// Get retrieves a value from shared memory by key. Returns the value and a
// boolean indicating whether the key was found.
func (sm *SharedMemory) Get(key string) (interface{}, bool) {
	sm.mu.RLock()
	defer sm.mu.RUnlock()

	entry, exists := sm.Entries[key]
	if !exists {
		return nil, false
	}
	return entry.Value, true
}

// GetString retrieves a value as a string. Returns empty string if the key
// does not exist or the value cannot be converted to a string.
func (sm *SharedMemory) GetString(key string) string {
	sm.mu.RLock()
	defer sm.mu.RUnlock()

	entry, exists := sm.Entries[key]
	if !exists {
		return ""
	}
	if s, ok := entry.Value.(string); ok {
		return s
	}
	return fmt.Sprintf("%v", entry.Value)
}

// GetInt retrieves a value as an int. Returns 0 if the key does not exist or
// the value cannot be interpreted as an integer.
func (sm *SharedMemory) GetInt(key string) int {
	sm.mu.RLock()
	defer sm.mu.RUnlock()

	entry, exists := sm.Entries[key]
	if !exists {
		return 0
	}
	switch v := entry.Value.(type) {
	case int:
		return v
	case int64:
		return int(v)
	case float64:
		return int(v)
	default:
		return 0
	}
}

// GetBool retrieves a value as a bool. Returns false if the key does not exist
// or the value cannot be interpreted as a boolean.
func (sm *SharedMemory) GetBool(key string) bool {
	sm.mu.RLock()
	defer sm.mu.RUnlock()

	entry, exists := sm.Entries[key]
	if !exists {
		return false
	}
	if b, ok := entry.Value.(bool); ok {
		return b
	}
	return false
}

// Delete removes an entry from shared memory. Only the owner who wrote the
// entry is permitted to delete it; otherwise an error is returned.
func (sm *SharedMemory) Delete(key string, owner string) error {
	sm.mu.Lock()
	defer sm.mu.Unlock()

	entry, exists := sm.Entries[key]
	if !exists {
		return fmt.Errorf("shared memory: key %q not found", key)
	}
	if entry.Owner != owner {
		return fmt.Errorf("shared memory: agent %q cannot delete key %q owned by %q", owner, key, entry.Owner)
	}

	delete(sm.Entries, key)

	// Remove from any namespaces.
	for ns, keys := range sm.Namespaces {
		for i, k := range keys {
			if k == key {
				sm.Namespaces[ns] = append(keys[:i], keys[i+1:]...)
				break
			}
		}
	}

	return nil
}

// List returns all entries belonging to a given namespace.
func (sm *SharedMemory) List(namespace string) []MemEntry {
	sm.mu.RLock()
	defer sm.mu.RUnlock()

	keys, exists := sm.Namespaces[namespace]
	if !exists {
		return nil
	}

	var results []MemEntry
	for _, key := range keys {
		if entry, ok := sm.Entries[key]; ok {
			results = append(results, *entry)
		}
	}
	return results
}

// SetNamespace assigns a key to a namespace for organizational grouping.
// A key can belong to multiple namespaces.
func (sm *SharedMemory) SetNamespace(key, namespace string) {
	sm.mu.Lock()
	defer sm.mu.Unlock()

	// Check key exists.
	if _, exists := sm.Entries[key]; !exists {
		return
	}

	// Avoid duplicates.
	for _, k := range sm.Namespaces[namespace] {
		if k == key {
			return
		}
	}
	sm.Namespaces[namespace] = append(sm.Namespaces[namespace], key)
}

// Watch returns a channel that receives notifications whenever the specified
// key is updated. The channel is buffered (size 16) and non-blocking on send.
func (sm *SharedMemory) Watch(key string) <-chan *MemEntry {
	sm.mu.Lock()
	defer sm.mu.Unlock()

	ch := make(chan *MemEntry, 16)
	sm.watchers[key] = append(sm.watchers[key], ch)
	return ch
}

// Snapshot returns a shallow copy of all key-value pairs currently in shared memory.
func (sm *SharedMemory) Snapshot() map[string]interface{} {
	sm.mu.RLock()
	defer sm.mu.RUnlock()

	snap := make(map[string]interface{}, len(sm.Entries))
	for k, entry := range sm.Entries {
		snap[k] = entry.Value
	}
	return snap
}

// Restore populates shared memory from a previously captured snapshot. Existing
// entries are cleared and replaced with the snapshot contents. All restored
// entries are assigned owner "snapshot" and version 1.
func (sm *SharedMemory) Restore(snapshot map[string]interface{}) {
	sm.mu.Lock()
	defer sm.mu.Unlock()

	sm.Entries = make(map[string]*MemEntry, len(snapshot))
	sm.Namespaces = make(map[string][]string)

	now := time.Now()
	for k, v := range snapshot {
		sm.Entries[k] = &MemEntry{
			Key:       k,
			Value:     v,
			Type:      detectType(v),
			Owner:     "snapshot",
			CreatedAt: now,
			UpdatedAt: now,
			Version:   1,
			Readers:   []string{},
		}
	}
}

// FormatState returns a human-readable representation of all entries in
// shared memory, formatted for display in agent context.
func (sm *SharedMemory) FormatState() string {
	sm.mu.RLock()
	defer sm.mu.RUnlock()

	count := len(sm.Entries)
	if count == 0 {
		return "Shared Memory (0 entries):\n─────────────────────────\n(empty)"
	}

	// Sort keys for deterministic output.
	keys := make([]string, 0, count)
	for k := range sm.Entries {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	var b strings.Builder
	fmt.Fprintf(&b, "Shared Memory (%d entries):\n", count)
	b.WriteString("─────────────────────────\n")

	for _, key := range keys {
		entry := sm.Entries[key]
		b.WriteString(fmt.Sprintf("%s = %s (by: %s, v%d)\n", key, formatValue(entry.Value), entry.Owner, entry.Version))
	}

	return strings.TrimRight(b.String(), "\n")
}

// Diff returns all entries that have been modified since the given timestamp.
func (sm *SharedMemory) Diff(since time.Time) []MemEntry {
	sm.mu.RLock()
	defer sm.mu.RUnlock()

	var results []MemEntry
	for _, entry := range sm.Entries {
		if entry.UpdatedAt.After(since) {
			results = append(results, *entry)
		}
	}

	// Sort by UpdatedAt for deterministic output.
	sort.Slice(results, func(i, j int) bool {
		return results[i].UpdatedAt.Before(results[j].UpdatedAt)
	})

	return results
}

// Clear resets all entries, namespaces, and watchers.
func (sm *SharedMemory) Clear() {
	sm.mu.Lock()
	defer sm.mu.Unlock()

	// Close all watcher channels.
	for _, chs := range sm.watchers {
		for _, ch := range chs {
			close(ch)
		}
	}

	sm.Entries = make(map[string]*MemEntry)
	sm.Namespaces = make(map[string][]string)
	sm.watchers = make(map[string][]chan *MemEntry)
}

// notifyWatchers sends an entry update to all watchers registered for the key.
// Must be called while holding sm.mu (at least read lock).
func (sm *SharedMemory) notifyWatchers(key string, entry *MemEntry) {
	watchers, exists := sm.watchers[key]
	if !exists {
		return
	}

	// Make a copy to send through channels.
	copied := *entry
	for _, ch := range watchers {
		select {
		case ch <- &copied:
		default:
			// Channel full, skip to avoid blocking.
		}
	}
}

// detectType infers the type string for a given value.
func detectType(value interface{}) string {
	switch value.(type) {
	case string:
		return "string"
	case int, int64, int32:
		return "int"
	case float64, float32:
		return "int"
	case bool:
		return "bool"
	case []interface{}, []string:
		return "list"
	default:
		return "json"
	}
}

// formatValue renders a value as a human-readable string for display.
func formatValue(value interface{}) string {
	switch v := value.(type) {
	case string:
		return fmt.Sprintf("%q", v)
	case bool:
		return fmt.Sprintf("%t", v)
	case int:
		return fmt.Sprintf("%d", v)
	case int64:
		return fmt.Sprintf("%d", v)
	case float64:
		if v == float64(int64(v)) {
			return fmt.Sprintf("%d", int64(v))
		}
		return fmt.Sprintf("%g", v)
	case []interface{}:
		items := make([]string, len(v))
		for i, item := range v {
			items[i] = formatValue(item)
		}
		return "[" + strings.Join(items, ", ") + "]"
	case []string:
		items := make([]string, len(v))
		for i, item := range v {
			items[i] = fmt.Sprintf("%q", item)
		}
		return "[" + strings.Join(items, ", ") + "]"
	default:
		b, err := json.Marshal(v)
		if err != nil {
			return fmt.Sprintf("%v", v)
		}
		return string(b)
	}
}
