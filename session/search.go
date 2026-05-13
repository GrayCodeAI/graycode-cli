package session

import (
	"bufio"
	"encoding/json"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"sync"
	"time"
)

// SearchEngine provides full-text search across hawk sessions/conversations.
type SearchEngine struct {
	SessionDir string
	Index      map[string]*SessionIndex
	mu         sync.RWMutex
}

// SessionIndex holds the inverted index for a single session.
type SessionIndex struct {
	ID        string
	Terms     map[string][]int // term → message indices containing it
	Messages  []IndexedMessage
	CreatedAt time.Time
	Model     string
	Provider  string
}

// IndexedMessage stores metadata about a message for search results.
type IndexedMessage struct {
	Index     int
	Role      string
	Preview   string // first 100 chars
	Timestamp time.Time
}

// FTSResult represents a single match from a full-text search query.
type FTSResult struct {
	SessionID    string
	MessageIndex int
	Role         string
	Content      string
	Preview      string
	Score        float64
	Highlights   []Highlight
	Timestamp    time.Time
}

// Highlight marks a matched region within content.
type Highlight struct {
	Start int
	End   int
}

// SearchOptions configures a search query.
type SearchOptions struct {
	MaxResults    int
	SessionFilter string
	RoleFilter    string
	DateAfter     time.Time
	DateBefore    time.Time
}

// NewSearchEngine creates a new SearchEngine for the given session directory.
func NewSearchEngine(sessionDir string) *SearchEngine {
	return &SearchEngine{
		SessionDir: sessionDir,
		Index:      make(map[string]*SessionIndex),
	}
}

// IndexSession tokenizes messages and builds an inverted index for the session.
func (se *SearchEngine) IndexSession(sessionID string, messages []Message) error {
	se.mu.Lock()
	defer se.mu.Unlock()

	idx := &SessionIndex{
		ID:       sessionID,
		Terms:    make(map[string][]int),
		Messages: make([]IndexedMessage, 0, len(messages)),
	}

	for i, msg := range messages {
		preview := msg.Content
		if len(preview) > 100 {
			preview = preview[:100]
		}

		idx.Messages = append(idx.Messages, IndexedMessage{
			Index:   i,
			Role:    msg.Role,
			Preview: preview,
		})

		// Tokenize and build inverted index
		terms := tokenize(msg.Content)
		seen := make(map[string]bool)
		for _, term := range terms {
			if !seen[term] {
				seen[term] = true
				idx.Terms[term] = append(idx.Terms[term], i)
			}
		}
	}

	se.Index[sessionID] = idx
	return nil
}

// Search performs a full-text search using BM25 scoring across indexed sessions.
func (se *SearchEngine) Search(query string, opts SearchOptions) []FTSResult {
	if strings.TrimSpace(query) == "" {
		return nil
	}

	se.mu.RLock()
	defer se.mu.RUnlock()

	queryTerms := tokenize(query)
	if len(queryTerms) == 0 {
		return nil
	}

	if opts.MaxResults <= 0 {
		opts.MaxResults = 20
	}

	// Compute corpus-wide statistics for BM25
	totalDocs := 0
	df := make(map[string]int) // document frequency per term
	var allDocLens []int

	for _, idx := range se.Index {
		if opts.SessionFilter != "" && idx.ID != opts.SessionFilter {
			continue
		}

		// Build set of eligible message indices (respecting role filter)
		eligible := make(map[int]bool)
		for _, msg := range idx.Messages {
			if opts.RoleFilter != "" && msg.Role != opts.RoleFilter {
				continue
			}
			eligible[msg.Index] = true
			totalDocs++
			allDocLens = append(allDocLens, len(msg.Preview))
		}

		// Compute document frequency only for eligible messages
		for term, indices := range idx.Terms {
			for _, msgIdx := range indices {
				if eligible[msgIdx] {
					df[term]++
				}
			}
		}
	}

	if totalDocs == 0 {
		return nil
	}

	avgDocLen := float64(0)
	if len(allDocLens) > 0 {
		sum := 0
		for _, l := range allDocLens {
			sum += l
		}
		avgDocLen = float64(sum) / float64(len(allDocLens))
	}

	var results []FTSResult

	for _, idx := range se.Index {
		if opts.SessionFilter != "" && idx.ID != opts.SessionFilter {
			continue
		}

		// Find candidate messages that contain at least one query term
		candidates := make(map[int]bool)
		for _, term := range queryTerms {
			if indices, ok := idx.Terms[term]; ok {
				for _, i := range indices {
					candidates[i] = true
				}
			}
		}

		for msgIdx := range candidates {
			if msgIdx >= len(idx.Messages) {
				continue
			}
			msg := idx.Messages[msgIdx]

			if opts.RoleFilter != "" && msg.Role != opts.RoleFilter {
				continue
			}

			if !opts.DateAfter.IsZero() && msg.Timestamp.Before(opts.DateAfter) {
				continue
			}
			if !opts.DateBefore.IsZero() && msg.Timestamp.After(opts.DateBefore) {
				continue
			}

			// Score using BM25
			content := msg.Preview // Use preview for scoring
			docLen := len(content)
			score := BuildBM25Score(queryTerms, content, docLen, avgDocLen, df, totalDocs)

			if score > 0 {
				highlights := HighlightMatches(content, query)
				results = append(results, FTSResult{
					SessionID:    idx.ID,
					MessageIndex: msgIdx,
					Role:         msg.Role,
					Content:      content,
					Preview:      msg.Preview,
					Score:        score,
					Highlights:   highlights,
					Timestamp:    msg.Timestamp,
				})
			}
		}
	}

	// Sort by score descending
	sort.Slice(results, func(i, j int) bool {
		return results[i].Score > results[j].Score
	})

	if len(results) > opts.MaxResults {
		results = results[:opts.MaxResults]
	}

	return results
}

// SearchRegex performs a regex-based search for exact pattern matching.
func (se *SearchEngine) SearchRegex(pattern string, opts SearchOptions) []FTSResult {
	re, err := regexp.Compile(pattern)
	if err != nil {
		return nil
	}

	se.mu.RLock()
	defer se.mu.RUnlock()

	if opts.MaxResults <= 0 {
		opts.MaxResults = 20
	}

	var results []FTSResult

	for _, idx := range se.Index {
		if opts.SessionFilter != "" && idx.ID != opts.SessionFilter {
			continue
		}

		for _, msg := range idx.Messages {
			if opts.RoleFilter != "" && msg.Role != opts.RoleFilter {
				continue
			}

			if !opts.DateAfter.IsZero() && msg.Timestamp.Before(opts.DateAfter) {
				continue
			}
			if !opts.DateBefore.IsZero() && msg.Timestamp.After(opts.DateBefore) {
				continue
			}

			matches := re.FindAllStringIndex(msg.Preview, -1)
			if len(matches) > 0 {
				var highlights []Highlight
				for _, m := range matches {
					highlights = append(highlights, Highlight{Start: m[0], End: m[1]})
				}

				results = append(results, FTSResult{
					SessionID:    idx.ID,
					MessageIndex: msg.Index,
					Role:         msg.Role,
					Content:      msg.Preview,
					Preview:      msg.Preview,
					Score:        float64(len(matches)),
					Highlights:   highlights,
					Timestamp:    msg.Timestamp,
				})
			}
		}
	}

	// Sort by score descending
	sort.Slice(results, func(i, j int) bool {
		return results[i].Score > results[j].Score
	})

	if len(results) > opts.MaxResults {
		results = results[:opts.MaxResults]
	}

	return results
}

// BuildBM25Score computes the BM25 score for a document against query terms.
// Uses standard parameters k1=1.2, b=0.75.
func BuildBM25Score(queryTerms []string, doc string, docLen int, avgDocLen float64, df map[string]int, totalDocs int) float64 {
	const (
		k1 = 1.2
		b  = 0.75
	)

	if avgDocLen == 0 || totalDocs == 0 {
		return 0
	}

	docLower := strings.ToLower(doc)
	score := float64(0)

	for _, term := range queryTerms {
		// Term frequency in this document
		tf := float64(strings.Count(docLower, term))
		if tf == 0 {
			continue
		}

		// Inverse document frequency
		docFreq := df[term]
		if docFreq == 0 {
			docFreq = 1
		}
		idf := math.Log(1 + (float64(totalDocs)-float64(docFreq)+0.5)/(float64(docFreq)+0.5))

		// BM25 formula
		numerator := tf * (k1 + 1)
		denominator := tf + k1*(1-b+b*float64(docLen)/avgDocLen)

		score += idf * numerator / denominator
	}

	return score
}

// HighlightMatches finds positions of query terms in content for highlighting.
func HighlightMatches(content string, query string) []Highlight {
	var highlights []Highlight

	contentLower := strings.ToLower(content)
	terms := tokenize(query)

	for _, term := range terms {
		start := 0
		for {
			idx := strings.Index(contentLower[start:], term)
			if idx == -1 {
				break
			}
			absStart := start + idx
			highlights = append(highlights, Highlight{
				Start: absStart,
				End:   absStart + len(term),
			})
			start = absStart + len(term)
		}
	}

	// Sort highlights by start position
	sort.Slice(highlights, func(i, j int) bool {
		return highlights[i].Start < highlights[j].Start
	})

	return highlights
}

// FormatResults formats search results for terminal display.
func FormatResults(results []FTSResult, showContext int) string {
	if len(results) == 0 {
		return "No results found."
	}

	var sb strings.Builder

	for i, r := range results {
		if i > 0 {
			sb.WriteString("\n")
		}

		// Header line: [session-id] timestamp (role)
		ts := r.Timestamp.Format("2006-01-02 15:04")
		if r.Timestamp.IsZero() {
			ts = "unknown"
		}
		sb.WriteString(fmt.Sprintf("[%s] %s (%s)\n", r.SessionID, ts, r.Role))

		// Content with highlights marked by **
		content := r.Preview
		if showContext > 0 && len(content) > showContext {
			content = content[:showContext]
		}

		highlighted := applyHighlights(content, r.Highlights)
		sb.WriteString("...")
		sb.WriteString(highlighted)
		sb.WriteString("...\n")
	}

	return sb.String()
}

// applyHighlights wraps matched terms with ** markers for display.
func applyHighlights(content string, highlights []Highlight) string {
	if len(highlights) == 0 {
		return content
	}

	// Apply highlights in reverse order to preserve positions
	result := content
	for i := len(highlights) - 1; i >= 0; i-- {
		h := highlights[i]
		if h.Start >= len(result) || h.End > len(result) {
			continue
		}
		result = result[:h.Start] + "**" + result[h.Start:h.End] + "**" + result[h.End:]
	}

	return result
}

// RebuildIndex walks the session directory and re-indexes all sessions from their JSONL files.
func (se *SearchEngine) RebuildIndex() error {
	se.mu.Lock()
	se.Index = make(map[string]*SessionIndex)
	se.mu.Unlock()

	entries, err := os.ReadDir(se.SessionDir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return fmt.Errorf("read session directory: %w", err)
	}

	for _, entry := range entries {
		ext := filepath.Ext(entry.Name())
		if ext != ".jsonl" && ext != ".json" {
			continue
		}

		id := entry.Name()[:len(entry.Name())-len(ext)]
		messages, meta, err := se.loadSessionMessages(filepath.Join(se.SessionDir, entry.Name()))
		if err != nil {
			continue // skip unreadable sessions
		}

		if err := se.IndexSession(id, messages); err != nil {
			continue
		}

		// Apply metadata if available
		se.mu.Lock()
		if idx, ok := se.Index[id]; ok {
			idx.CreatedAt = meta.createdAt
			idx.Model = meta.model
			idx.Provider = meta.provider
		}
		se.mu.Unlock()
	}

	return nil
}

type sessionMeta struct {
	createdAt time.Time
	model     string
	provider  string
}

// loadSessionMessages reads a session file and returns its messages.
func (se *SearchEngine) loadSessionMessages(path string) ([]Message, sessionMeta, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, sessionMeta{}, err
	}
	defer f.Close()

	ext := filepath.Ext(path)
	if ext == ".json" {
		return se.loadLegacyJSON(f)
	}

	return se.loadJSONL(f)
}

func (se *SearchEngine) loadJSONL(f *os.File) ([]Message, sessionMeta, error) {
	var messages []Message
	var meta sessionMeta

	scanner := bufio.NewScanner(f)
	scanner.Buffer(make([]byte, 1024*1024), 1024*1024)
	firstLine := true

	for scanner.Scan() {
		line := scanner.Bytes()
		if len(line) == 0 {
			continue
		}

		if firstLine {
			firstLine = false
			var raw map[string]interface{}
			if json.Unmarshal(line, &raw) == nil {
				if raw["type"] == "session_meta" {
					if v, ok := raw["model"].(string); ok {
						meta.model = v
					}
					if v, ok := raw["provider"].(string); ok {
						meta.provider = v
					}
					if v, ok := raw["created_at"].(string); ok {
						meta.createdAt, _ = time.Parse(time.RFC3339, v)
					}
					continue
				}
			}
		}

		var msg Message
		if err := json.Unmarshal(line, &msg); err != nil {
			continue
		}
		messages = append(messages, msg)
	}

	return messages, meta, scanner.Err()
}

func (se *SearchEngine) loadLegacyJSON(f *os.File) ([]Message, sessionMeta, error) {
	var s Session
	decoder := json.NewDecoder(f)
	if err := decoder.Decode(&s); err != nil {
		return nil, sessionMeta{}, err
	}
	meta := sessionMeta{
		createdAt: s.CreatedAt,
		model:     s.Model,
		provider:  s.Provider,
	}
	return s.Messages, meta, nil
}

// tokenize splits text into lowercase terms for indexing and searching.
func tokenize(text string) []string {
	text = strings.ToLower(text)

	// Split on non-alphanumeric characters
	var terms []string
	var current strings.Builder

	for _, r := range text {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') {
			current.WriteRune(r)
		} else {
			if current.Len() > 0 {
				terms = append(terms, current.String())
				current.Reset()
			}
		}
	}
	if current.Len() > 0 {
		terms = append(terms, current.String())
	}

	return terms
}
