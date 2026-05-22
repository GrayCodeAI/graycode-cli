package streaming

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"regexp"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

// DefaultMaxEntries is the default maximum number of cache entries.
const DefaultMaxEntries = 1000

// DefaultMaxAge is the default maximum age of cache entries.
const DefaultMaxAge = 24 * time.Hour

// CacheEntry holds a cached LLM response.
type CacheEntry struct {
	Key        string    `json:"key"`
	Prompt     string    `json:"prompt"`
	Response   string    `json:"response"`
	Model      string    `json:"model"`
	Tokens     int       `json:"tokens"`
	CreatedAt  time.Time `json:"created_at"`
	LastHit    time.Time `json:"last_hit"`
	HitCount   int       `json:"hit_count"`
	Similarity float64   `json:"similarity"`
}

// CacheStats holds statistics about the response cache.
type CacheStats struct {
	Entries      int     `json:"entries"`
	HitCount     int64   `json:"hit_count"`
	MissCount    int64   `json:"miss_count"`
	HitRate      float64 `json:"hit_rate"`
	SavedTokens  int64   `json:"saved_tokens"`
	SavedCostUSD float64 `json:"saved_cost_usd"`
}

// ResponseCache caches LLM responses for identical or similar prompts.
type ResponseCache struct {
	Entries    map[string]*CacheEntry
	MaxEntries int
	MaxAge     time.Duration
	HitCount   int64
	MissCount  int64
	mu         sync.RWMutex
}

// NewResponseCache creates a new ResponseCache with the given parameters.
// If maxEntries is 0, DefaultMaxEntries is used.
// If maxAge is 0, DefaultMaxAge is used.
func NewResponseCache(maxEntries int, maxAge time.Duration) *ResponseCache {
	if maxEntries <= 0 {
		maxEntries = DefaultMaxEntries
	}
	if maxAge <= 0 {
		maxAge = DefaultMaxAge
	}
	return &ResponseCache{
		Entries:    make(map[string]*CacheEntry),
		MaxEntries: maxEntries,
		MaxAge:     maxAge,
	}
}

// Get retrieves a cached response for the given prompt and model.
// It first tries an exact hash match, then falls back to similarity matching.
func (rc *ResponseCache) Get(prompt, model string) (*CacheEntry, bool) {
	rc.mu.Lock()
	defer rc.mu.Unlock()

	key := HashPrompt(prompt)
	modelKey := key + ":" + model

	// Exact match first.
	if entry, ok := rc.Entries[modelKey]; ok {
		if time.Since(entry.CreatedAt) > rc.MaxAge {
			delete(rc.Entries, modelKey)
			atomic.AddInt64(&rc.MissCount, 1)
			return nil, false
		}
		entry.LastHit = time.Now()
		entry.HitCount++
		entry.Similarity = 1.0
		atomic.AddInt64(&rc.HitCount, 1)
		return entry, true
	}

	// Similarity match fallback.
	entry, sim := rc.similarityMatchLocked(prompt, model, 0.95)
	if entry != nil {
		entry.LastHit = time.Now()
		entry.HitCount++
		entry.Similarity = sim
		atomic.AddInt64(&rc.HitCount, 1)
		return entry, true
	}

	atomic.AddInt64(&rc.MissCount, 1)
	return nil, false
}

// Set stores a prompt/response pair in the cache.
// If the cache is at capacity, it evicts the least recently used entry.
func (rc *ResponseCache) Set(prompt, response, model string, tokens int) {
	rc.mu.Lock()
	defer rc.mu.Unlock()

	key := HashPrompt(prompt)
	modelKey := key + ":" + model

	now := time.Now()
	rc.Entries[modelKey] = &CacheEntry{
		Key:       modelKey,
		Prompt:    prompt,
		Response:  response,
		Model:     model,
		Tokens:    tokens,
		CreatedAt: now,
		LastHit:   now,
		HitCount:  0,
	}

	// Evict if over capacity.
	for len(rc.Entries) > rc.MaxEntries {
		rc.evictLRULocked()
	}
}

// HashPrompt returns a SHA-256 hash of the normalized prompt.
// Normalization: lowercase, collapse whitespace.
func HashPrompt(prompt string) string {
	normalized := normalizePrompt(prompt)
	hash := sha256.Sum256([]byte(normalized))
	return hex.EncodeToString(hash[:])
}

// normalizePrompt lowercases and collapses whitespace in a prompt.
func normalizePrompt(prompt string) string {
	lower := strings.ToLower(prompt)
	fields := strings.Fields(lower)
	return strings.Join(fields, " ")
}

// SimilarityMatch finds the most similar cached prompt above the threshold.
// Uses word-level Jaccard similarity.
func (rc *ResponseCache) SimilarityMatch(prompt string, threshold float64) (*CacheEntry, float64) {
	rc.mu.RLock()
	defer rc.mu.RUnlock()

	return rc.similarityMatchLocked(prompt, "", threshold)
}

// similarityMatchLocked finds the most similar entry (caller must hold lock).
// If model is non-empty, only entries with that model are considered.
func (rc *ResponseCache) similarityMatchLocked(prompt, model string, threshold float64) (*CacheEntry, float64) {
	words := rcWordSet(normalizePrompt(prompt))

	var bestEntry *CacheEntry
	var bestSim float64

	for _, entry := range rc.Entries {
		if model != "" && entry.Model != model {
			continue
		}
		if time.Since(entry.CreatedAt) > rc.MaxAge {
			continue
		}
		entryWords := rcWordSet(normalizePrompt(entry.Prompt))
		sim := responseCacheJaccard(words, entryWords)
		if sim >= threshold && sim > bestSim {
			bestSim = sim
			bestEntry = entry
		}
	}

	return bestEntry, bestSim
}

// rcWordSet returns a set of words from a string.
func rcWordSet(s string) map[string]struct{} {
	fields := strings.Fields(s)
	set := make(map[string]struct{}, len(fields))
	for _, f := range fields {
		set[f] = struct{}{}
	}
	return set
}

// responseCacheJaccard computes the Jaccard similarity between two word sets.
func responseCacheJaccard(a, b map[string]struct{}) float64 {
	if len(a) == 0 && len(b) == 0 {
		return 1.0
	}

	intersection := 0
	for w := range a {
		if _, ok := b[w]; ok {
			intersection++
		}
	}

	union := len(a) + len(b) - intersection
	if union == 0 {
		return 0
	}
	return float64(intersection) / float64(union)
}

// Invalidate removes entries whose prompt matches the given pattern (regex).
func (rc *ResponseCache) Invalidate(pattern string) {
	re, err := regexp.Compile(pattern)
	if err != nil {
		return
	}

	rc.mu.Lock()
	defer rc.mu.Unlock()

	for key, entry := range rc.Entries {
		if re.MatchString(entry.Prompt) {
			delete(rc.Entries, key)
		}
	}
}

// InvalidateByAge removes entries older than maxAge.
func (rc *ResponseCache) InvalidateByAge(maxAge time.Duration) {
	rc.mu.Lock()
	defer rc.mu.Unlock()

	now := time.Now()
	for key, entry := range rc.Entries {
		if now.Sub(entry.CreatedAt) > maxAge {
			delete(rc.Entries, key)
		}
	}
}

// EvictLRU removes the least-recently-hit entry from the cache.
func (rc *ResponseCache) EvictLRU() {
	rc.mu.Lock()
	defer rc.mu.Unlock()

	rc.evictLRULocked()
}

// evictLRULocked removes the LRU entry (caller must hold write lock).
func (rc *ResponseCache) evictLRULocked() {
	if len(rc.Entries) == 0 {
		return
	}

	var oldestKey string
	var oldestTime time.Time
	first := true

	for key, entry := range rc.Entries {
		if first || entry.LastHit.Before(oldestTime) {
			oldestKey = key
			oldestTime = entry.LastHit
			first = false
		}
	}

	delete(rc.Entries, oldestKey)
}

// Stats returns current cache statistics.
func (rc *ResponseCache) Stats() CacheStats {
	rc.mu.RLock()
	defer rc.mu.RUnlock()

	hits := atomic.LoadInt64(&rc.HitCount)
	misses := atomic.LoadInt64(&rc.MissCount)
	total := hits + misses

	var hitRate float64
	if total > 0 {
		hitRate = float64(hits) / float64(total)
	}

	var savedTokens int64
	for _, entry := range rc.Entries {
		savedTokens += int64(entry.Tokens) * int64(entry.HitCount)
	}

	// Estimate cost savings at $0.00003 per token (approximate blended rate).
	savedCost := float64(savedTokens) * 0.00003

	return CacheStats{
		Entries:      len(rc.Entries),
		HitCount:     hits,
		MissCount:    misses,
		HitRate:      hitRate,
		SavedTokens:  savedTokens,
		SavedCostUSD: savedCost,
	}
}

// ShouldCache determines whether a prompt should be cached.
// It skips prompts referencing current time, volatile file contents, or one-off questions.
func ShouldCache(prompt string) bool {
	lower := strings.ToLower(prompt)

	// Don't cache: prompts referencing current time.
	timePatterns := []string{
		"current time", "right now", "today's date", "what time",
		"what day", "current date", "as of now", "at this moment",
	}
	for _, p := range timePatterns {
		if strings.Contains(lower, p) {
			return false
		}
	}

	// Don't cache: prompts referencing volatile file contents.
	filePatterns := []string{
		"contents of", "read the file", "what's in the file",
		"show me the file", "cat the file", "current state of",
	}
	for _, p := range filePatterns {
		if strings.Contains(lower, p) {
			return false
		}
	}

	// Don't cache: one-off or highly specific questions.
	oneOffPatterns := []string{
		"my specific", "this particular", "just this once",
		"only for now",
	}
	for _, p := range oneOffPatterns {
		if strings.Contains(lower, p) {
			return false
		}
	}

	// Do cache: explanations, formatting, boilerplate.
	cachePatterns := []string{
		"explain", "how to", "what is", "format",
		"generate", "boilerplate", "template", "example",
		"convert", "translate", "summarize", "describe",
	}
	for _, p := range cachePatterns {
		if strings.Contains(lower, p) {
			return true
		}
	}

	// Default: cache if prompt is non-trivial length.
	return len(prompt) > 20
}

// FormatStats returns a human-readable summary of cache statistics.
func (rc *ResponseCache) FormatStats() string {
	stats := rc.Stats()

	total := stats.HitCount + stats.MissCount
	hitPct := 0
	if total > 0 {
		hitPct = int(stats.HitRate * 100)
	}

	rc.mu.RLock()
	var oldestAge time.Duration
	for _, entry := range rc.Entries {
		age := time.Since(entry.CreatedAt)
		if age > oldestAge {
			oldestAge = age
		}
	}
	rc.mu.RUnlock()

	oldestStr := "n/a"
	if len(rc.Entries) > 0 {
		oldestStr = responseCacheFormatDuration(oldestAge)
	}

	return fmt.Sprintf(
		"Response Cache:\nEntries: %d/%d\nHit rate: %d%% (%d hits / %d total)\nSaved: %s tokens (~$%.2f)\nOldest entry: %s",
		stats.Entries, rc.MaxEntries,
		hitPct, stats.HitCount, total,
		responseCacheFormatTokens(stats.SavedTokens), stats.SavedCostUSD,
		oldestStr,
	)
}

// responseCacheFormatDuration formats a duration in a human-readable way.
func responseCacheFormatDuration(d time.Duration) string {
	if d < time.Minute {
		return fmt.Sprintf("%ds ago", int(d.Seconds()))
	}
	if d < time.Hour {
		return fmt.Sprintf("%dm ago", int(d.Minutes()))
	}
	return fmt.Sprintf("%dh ago", int(d.Hours()))
}

// responseCacheFormatTokens formats a token count with commas.
func responseCacheFormatTokens(n int64) string {
	s := fmt.Sprintf("%d", n)
	if len(s) <= 3 {
		return s
	}

	var result strings.Builder
	remainder := len(s) % 3
	if remainder > 0 {
		result.WriteString(s[:remainder])
		if len(s) > remainder {
			result.WriteString(",")
		}
	}
	for i := remainder; i < len(s); i += 3 {
		if i > remainder {
			result.WriteString(",")
		}
		result.WriteString(s[i : i+3])
	}
	return result.String()
}

// cacheExport is the serializable form of the cache for Export/Import.
type cacheExport struct {
	Entries    []*CacheEntry `json:"entries"`
	MaxEntries int           `json:"max_entries"`
	MaxAge     string        `json:"max_age"`
}

// Export serializes the cache to JSON bytes.
func (rc *ResponseCache) Export() ([]byte, error) {
	rc.mu.RLock()
	defer rc.mu.RUnlock()

	entries := make([]*CacheEntry, 0, len(rc.Entries))
	for _, entry := range rc.Entries {
		entries = append(entries, entry)
	}

	exp := cacheExport{
		Entries:    entries,
		MaxEntries: rc.MaxEntries,
		MaxAge:     rc.MaxAge.String(),
	}

	return json.Marshal(exp)
}

// Import loads cache entries from JSON bytes, merging with existing entries.
func (rc *ResponseCache) Import(data []byte) error {
	var exp cacheExport
	if err := json.Unmarshal(data, &exp); err != nil {
		return fmt.Errorf("failed to unmarshal cache data: %w", err)
	}

	rc.mu.Lock()
	defer rc.mu.Unlock()

	for _, entry := range exp.Entries {
		if time.Since(entry.CreatedAt) <= rc.MaxAge {
			rc.Entries[entry.Key] = entry
		}
	}

	// Evict if over capacity after import.
	for len(rc.Entries) > rc.MaxEntries {
		rc.evictLRULocked()
	}

	return nil
}
