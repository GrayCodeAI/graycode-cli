package memory

import (
	"strings"
)

// MemoryManager is a unified facade that coordinates all memory subsystems
// and implements the engine.MemoryRecaller interface.
type MemoryManager struct {
	Core     *Memory         // nil-safe; used via package-level funcs
	Auto     *AutoMemory
	Evolving *EvolvingMemory
	Zen      *ZenBrain
	Yaad     *YaadBridge
}

// NewMemoryManager creates a MemoryManager with all subsystems initialized.
func NewMemoryManager(projectDir string) *MemoryManager {
	return &MemoryManager{
		Auto:     NewAutoMemory(projectDir),
		Evolving: NewEvolvingMemory(),
		Zen:      NewZenBrain(),
		Yaad:     NewYaadBridge(),
	}
}

// LoadStartup initializes all subsystems by loading persisted state from disk.
func (mm *MemoryManager) LoadStartup() error {
	if err := mm.Evolving.Load(); err != nil {
		return err
	}
	if err := mm.Zen.Load(); err != nil {
		return err
	}
	// AutoMemory.LoadStartup is a read that returns content, no error to handle.
	// YaadBridge initializes in its constructor; nothing extra needed.
	return nil
}

// Recall queries all memory subsystems and merges results, deduplicating by content.
// Implements engine.MemoryRecaller.
func (mm *MemoryManager) Recall(query string, tokenBudget int) (string, error) {
	seen := make(map[string]bool)
	var parts []string
	budgetPer := tokenBudget / 4

	// 1. Core memories (package-level Search).
	if mems, err := Search(query); err == nil {
		for _, m := range mems {
			key := strings.ToLower(strings.TrimSpace(m.Content))
			if !seen[key] {
				seen[key] = true
				parts = append(parts, m.Content)
			}
		}
	}

	// 2. AutoMemory.
	for _, line := range mm.Auto.Search(query) {
		key := strings.ToLower(strings.TrimSpace(line))
		if key != "" && !seen[key] {
			seen[key] = true
			parts = append(parts, line)
		}
	}

	// 3. Evolving guidelines.
	for _, g := range mm.Evolving.Retrieve(query, 5) {
		key := strings.ToLower(strings.TrimSpace(g.Lesson))
		if !seen[key] {
			seen[key] = true
			parts = append(parts, g.Pattern+": "+g.Lesson)
		}
	}

	// 4. ZenBrain.
	for _, e := range mm.Zen.Retrieve(query, nil, 5) {
		key := strings.ToLower(strings.TrimSpace(e.Content))
		if !seen[key] {
			seen[key] = true
			parts = append(parts, e.Content)
		}
	}

	// 5. Yaad bridge.
	if yaadResult, err := mm.Yaad.Recall(query, budgetPer); err == nil && yaadResult != "" {
		for _, line := range strings.Split(yaadResult, "\n") {
			key := strings.ToLower(strings.TrimSpace(line))
			if key != "" && !seen[key] {
				seen[key] = true
				parts = append(parts, line)
			}
		}
	}

	return strings.Join(parts, "\n"), nil
}

// Remember routes content to the appropriate subsystem based on category.
// Implements engine.MemoryRecaller.
func (mm *MemoryManager) Remember(content, category string) error {
	switch category {
	case "guideline", "lesson":
		mm.Evolving.Learn(content, content, "manager")
		return mm.Evolving.Save()
	case "core", "preference":
		mm.Zen.Store(LayerCore, content, []string{category})
		return mm.Zen.Save()
	case "procedural", "howto":
		mm.Zen.Store(LayerProcedural, content, []string{category})
		return mm.Zen.Save()
	case "fact", "semantic":
		mm.Zen.Store(LayerSemantic, content, []string{category})
		return mm.Zen.Save()
	case "session", "episodic":
		mm.Zen.Store(LayerEpisodic, content, []string{category})
		return mm.Zen.Save()
	default:
		// Default: store in yaad if ready, otherwise fall back to core Memory.
		if mm.Yaad.Ready() {
			return mm.Yaad.Remember(content, category)
		}
		return Save(&Memory{Content: content, Tags: []string{category}})
	}
}

// FormatForPrompt gathers context from all subsystems into a single string
// suitable for prompt injection.
func (mm *MemoryManager) FormatForPrompt() string {
	var sections []string

	if s := mm.Auto.Format(); s != "" {
		sections = append(sections, s)
	}
	if s := mm.Evolving.Format(5); s != "" {
		sections = append(sections, s)
	}
	if s := mm.Zen.FormatForPrompt(500); s != "" {
		sections = append(sections, s)
	}

	return strings.Join(sections, "\n\n")
}
