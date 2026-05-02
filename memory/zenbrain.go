package memory

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"
)

// MemoryLayer identifies which layer a memory belongs to.
type MemoryLayer int

const (
	LayerWorking   MemoryLayer = iota // current task context (cleared each task)
	LayerShortTerm                     // this session (cleared on session end)
	LayerEpisodic                      // past sessions (what happened)
	LayerSemantic                      // facts and knowledge (what is true)
	LayerProcedural                    // how-to knowledge (how to do things)
	LayerCore                          // fundamental preferences (never expires)
	LayerCross                         // cross-project patterns

	layerCount = 7
)

// String returns a human-readable name for the layer.
func (l MemoryLayer) String() string {
	switch l {
	case LayerWorking:
		return "working"
	case LayerShortTerm:
		return "short-term"
	case LayerEpisodic:
		return "episodic"
	case LayerSemantic:
		return "semantic"
	case LayerProcedural:
		return "procedural"
	case LayerCore:
		return "core"
	case LayerCross:
		return "cross-project"
	default:
		return "unknown"
	}
}

// MemoryEntry is a single memory stored in the ZenBrain.
type MemoryEntry struct {
	ID          string      `json:"id"`
	Layer       MemoryLayer `json:"layer"`
	Content     string      `json:"content"`
	Tags        []string    `json:"tags,omitempty"`
	Priority    float64     `json:"priority"`
	CreatedAt   time.Time   `json:"created_at"`
	AccessedAt  time.Time   `json:"accessed_at"`
	AccessCount int         `json:"access_count"`
}

// ZenBrain is a 7-layer memory system inspired by neuroscience.
type ZenBrain struct {
	mu     sync.Mutex
	layers [layerCount][]MemoryEntry
	path   string
}

// NewZenBrain creates a new ZenBrain with the default storage path.
func NewZenBrain() *ZenBrain {
	home, _ := os.UserHomeDir()
	return &ZenBrain{
		path: filepath.Join(home, ".hawk", "memory", "zenbrain.json"),
	}
}

// Store adds a memory to the specified layer.
func (zb *ZenBrain) Store(layer MemoryLayer, content string, tags []string) {
	zb.mu.Lock()
	defer zb.mu.Unlock()

	if layer < 0 || int(layer) >= layerCount {
		return
	}

	entry := MemoryEntry{
		ID:          fmt.Sprintf("zen_%d_%d", layer, time.Now().UnixNano()),
		Layer:       layer,
		Content:     content,
		Tags:        tags,
		Priority:    1.0,
		CreatedAt:   time.Now(),
		AccessedAt:  time.Now(),
		AccessCount: 0,
	}
	zb.layers[layer] = append(zb.layers[layer], entry)
}

// Retrieve returns the top-K entries across the specified layers that best
// match the query (by keyword overlap), sorted by relevance.
func (zb *ZenBrain) Retrieve(query string, layers []MemoryLayer, topK int) []MemoryEntry {
	zb.mu.Lock()
	defer zb.mu.Unlock()

	if topK <= 0 {
		topK = 5
	}
	queryLower := strings.ToLower(query)
	queryTokens := tokenizeSimple(queryLower)

	type scored struct {
		entry MemoryEntry
		layer MemoryLayer
		idx   int
		score float64
	}
	var candidates []scored

	searchLayers := layers
	if len(searchLayers) == 0 {
		// Search all layers.
		for i := 0; i < layerCount; i++ {
			searchLayers = append(searchLayers, MemoryLayer(i))
		}
	}

	for _, layer := range searchLayers {
		if int(layer) >= layerCount {
			continue
		}
		for i, entry := range zb.layers[layer] {
			contentTokens := tokenizeSimple(strings.ToLower(entry.Content))
			overlap := tokenOverlap(queryTokens, contentTokens)

			// Also check tags.
			for _, tag := range entry.Tags {
				tagTokens := tokenizeSimple(strings.ToLower(tag))
				tagOverlap := tokenOverlap(queryTokens, tagTokens)
				if tagOverlap > overlap {
					overlap = tagOverlap
				}
			}

			if overlap > 0 {
				// Score combines relevance, priority, and recency.
				recency := 1.0 / (1.0 + time.Since(entry.AccessedAt).Hours()/24.0)
				score := overlap*0.6 + entry.Priority*0.2 + recency*0.2
				candidates = append(candidates, scored{
					entry: entry,
					layer: layer,
					idx:   i,
					score: score,
				})
			}
		}
	}

	sort.Slice(candidates, func(a, b int) bool {
		return candidates[a].score > candidates[b].score
	})

	if len(candidates) > topK {
		candidates = candidates[:topK]
	}

	// Update access timestamps and counts.
	for _, c := range candidates {
		if int(c.layer) < layerCount && c.idx < len(zb.layers[c.layer]) {
			zb.layers[c.layer][c.idx].AccessedAt = time.Now()
			zb.layers[c.layer][c.idx].AccessCount++
		}
	}

	out := make([]MemoryEntry, len(candidates))
	for i, c := range candidates {
		out[i] = c.entry
	}
	return out
}

// Consolidate promotes memories between layers based on access frequency:
//   - ShortTerm entries accessed 3+ times move to Episodic
//   - Episodic entries accessed 5+ times move to Semantic
func (zb *ZenBrain) Consolidate() {
	zb.mu.Lock()
	defer zb.mu.Unlock()

	// Promote short-term -> episodic.
	zb.promoteLayer(LayerShortTerm, LayerEpisodic, 3)
	// Promote episodic -> semantic.
	zb.promoteLayer(LayerEpisodic, LayerSemantic, 5)
}

// promoteLayer moves entries from src to dst if their access count meets the threshold.
func (zb *ZenBrain) promoteLayer(src, dst MemoryLayer, threshold int) {
	var kept []MemoryEntry
	for _, entry := range zb.layers[src] {
		if entry.AccessCount >= threshold {
			entry.Layer = dst
			zb.layers[dst] = append(zb.layers[dst], entry)
		} else {
			kept = append(kept, entry)
		}
	}
	zb.layers[src] = kept
}

// Sleep runs consolidation, decay, and strengthening — analogous to memory sleep.
// It consolidates layers, decays unused memories, and strengthens frequently accessed ones.
func (zb *ZenBrain) Sleep() {
	zb.Consolidate()

	zb.mu.Lock()
	defer zb.mu.Unlock()

	for layer := range zb.layers {
		// Skip Core layer — it never expires.
		if MemoryLayer(layer) == LayerCore {
			continue
		}

		var kept []MemoryEntry
		for i := range zb.layers[layer] {
			entry := &zb.layers[layer][i]

			// Strengthen frequently accessed entries.
			if entry.AccessCount > 0 {
				entry.Priority = clampFloat(entry.Priority+0.1*float64(entry.AccessCount), 0, 5)
				entry.AccessCount = 0 // reset for next sleep cycle
			}

			// Decay old, unaccessed entries.
			hoursSinceAccess := time.Since(entry.AccessedAt).Hours()
			if hoursSinceAccess > 24*7 { // more than a week
				entry.Priority -= 0.2
			}

			// Remove entries with priority below threshold.
			if entry.Priority > 0.1 {
				kept = append(kept, *entry)
			}
		}
		zb.layers[layer] = kept
	}
}

// FormatForPrompt selects the most relevant memories across all layers
// and formats them for prompt injection, within the given token estimate.
func (zb *ZenBrain) FormatForPrompt(maxTokens int) string {
	zb.mu.Lock()
	defer zb.mu.Unlock()

	if maxTokens <= 0 {
		maxTokens = 1000
	}

	// Collect all entries with their effective priority.
	type prioritized struct {
		entry    MemoryEntry
		effScore float64
	}
	var all []prioritized

	// Layer weights: Core > Procedural > Semantic > Episodic > Cross > ShortTerm > Working.
	layerWeights := [layerCount]float64{
		0.3, // Working
		0.5, // ShortTerm
		0.7, // Episodic
		0.9, // Semantic
		1.0, // Procedural
		1.2, // Core
		0.8, // Cross
	}

	for layer := range zb.layers {
		weight := layerWeights[layer]
		for _, entry := range zb.layers[layer] {
			eff := entry.Priority * weight
			all = append(all, prioritized{entry: entry, effScore: eff})
		}
	}

	sort.Slice(all, func(i, j int) bool {
		return all[i].effScore > all[j].effScore
	})

	var b strings.Builder
	b.WriteString("## Memory Context\n\n")
	tokenCount := 20 // header estimate

	for _, p := range all {
		line := fmt.Sprintf("[%s] %s\n", p.entry.Layer.String(), p.entry.Content)
		lineTokens := len(line) / 4
		if tokenCount+lineTokens > maxTokens {
			break
		}
		b.WriteString(line)
		tokenCount += lineTokens
	}

	if tokenCount <= 20 {
		return "" // no entries written
	}
	return b.String()
}

// Save persists all layers to disk.
func (zb *ZenBrain) Save() error {
	zb.mu.Lock()
	defer zb.mu.Unlock()

	dir := filepath.Dir(zb.path)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("create memory dir: %w", err)
	}

	// Flatten all layers into a single list for serialization.
	var all []MemoryEntry
	for _, layer := range zb.layers {
		all = append(all, layer...)
	}

	data, err := json.MarshalIndent(all, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal zenbrain: %w", err)
	}
	return os.WriteFile(zb.path, data, 0o644)
}

// Load reads persisted memories from disk and distributes them to layers.
func (zb *ZenBrain) Load() error {
	zb.mu.Lock()
	defer zb.mu.Unlock()

	data, err := os.ReadFile(zb.path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return fmt.Errorf("load zenbrain: %w", err)
	}

	var all []MemoryEntry
	if err := json.Unmarshal(data, &all); err != nil {
		return fmt.Errorf("parse zenbrain: %w", err)
	}

	// Clear and redistribute.
	for i := range zb.layers {
		zb.layers[i] = nil
	}
	for _, entry := range all {
		if int(entry.Layer) < layerCount {
			zb.layers[entry.Layer] = append(zb.layers[entry.Layer], entry)
		}
	}
	return nil
}

// LayerSize returns the number of entries in a specific layer.
func (zb *ZenBrain) LayerSize(layer MemoryLayer) int {
	zb.mu.Lock()
	defer zb.mu.Unlock()
	if int(layer) >= layerCount {
		return 0
	}
	return len(zb.layers[layer])
}

// TotalSize returns the total number of entries across all layers.
func (zb *ZenBrain) TotalSize() int {
	zb.mu.Lock()
	defer zb.mu.Unlock()
	total := 0
	for _, layer := range zb.layers {
		total += len(layer)
	}
	return total
}
