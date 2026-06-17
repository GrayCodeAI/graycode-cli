// Package repomap: semantic_search.go is the BM25-ranked full-text search
// engine over the Document set produced by the navigation index. It
// tokenises queries, expands them with ExpandQuery, and returns SearchHit
// results with snippet extraction. This is the search backend used by the
// Hawk CLI's "find" subcommand.
package repomap

import (
	"fmt"
	"io/fs"
	"math"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"unicode"

	"github.com/GrayCodeAI/hawk/internal/scoring"
)

// Document represents a single indexable unit (function, type, file, or block).
type Document struct {
	ID      string
	Path    string
	Content string
	Terms   map[string]int
	Length  int
	Type    string // "function", "type", "file", "block"
}

// SearchHit represents a single search result with scoring information.
type SearchHit struct {
	Document     *Document
	Score        float64
	MatchedTerms []string
	Snippet      string
}

// SemanticSearchIndex holds documents indexed for BM25 semantic search.
type SemanticSearchIndex struct {
	Documents map[string]*Document
	IDF       map[string]float64
	AvgDocLen float64
	TotalDocs int
	scorer    *scoring.BM25Scorer
	mu        sync.RWMutex
}

// NewSemanticIndex creates a new empty SemanticSearchIndex ready for use.
func NewSemanticSearchIndex() *SemanticSearchIndex {
	return &SemanticSearchIndex{
		Documents: make(map[string]*Document),
		IDF:       make(map[string]float64),
		scorer:    scoring.NewBM25Scorer(0, 0), // defaults: k1=1.2, b=0.75
	}
}

// IndexFile splits a file into documents (functions, types, blocks) and indexes them.
func (si *SemanticSearchIndex) IndexFile(path, content string) {
	si.mu.Lock()
	defer si.mu.Unlock()

	docs := splitIntoDocuments(path, content)
	for _, doc := range docs {
		si.Documents[doc.ID] = doc
	}

	si.rebuildIDFLocked()
}

// IndexDirectory walks a directory and indexes all supported source files.
func (si *SemanticSearchIndex) IndexDirectory(dir string) error {
	var files []string

	err := filepath.WalkDir(dir, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			base := filepath.Base(path)
			if base == ".git" || base == "node_modules" || base == "vendor" || base == "__pycache__" || base == ".cache" {
				return filepath.SkipDir
			}
			return nil
		}
		ext := filepath.Ext(path)
		if isSupportedSourceExt(ext) {
			files = append(files, path)
		}
		return nil
	})
	if err != nil {
		return err
	}

	for _, f := range files {
		data, readErr := os.ReadFile(f)
		if readErr != nil {
			continue
		}
		relPath, relErr := filepath.Rel(dir, f)
		if relErr != nil {
			relPath = f
		}
		si.IndexFile(relPath, string(data))
	}

	return nil
}

// Search tokenizes the query and returns the top-scoring documents using BM25.
func (si *SemanticSearchIndex) Search(query string, limit int) []SearchHit {
	si.mu.RLock()
	defer si.mu.RUnlock()

	if limit <= 0 {
		limit = 10
	}

	queryTerms := tokenizeSearch(query)
	if len(queryTerms) == 0 {
		return nil
	}

	var hits []SearchHit
	for _, doc := range si.Documents {
		score := si.BM25Score(queryTerms, doc)
		if score > 0 {
			matched := matchedTerms(queryTerms, doc)
			snippet := ExtractSnippet(doc, queryTerms, 120)
			hits = append(hits, SearchHit{
				Document:     doc,
				Score:        score,
				MatchedTerms: matched,
				Snippet:      snippet,
			})
		}
	}

	sort.Slice(hits, func(i, j int) bool {
		return hits[i].Score > hits[j].Score
	})

	if len(hits) > limit {
		hits = hits[:limit]
	}

	return hits
}

// SearchByIntent searches using expanded query terms to find code by meaning/intent.
// For example, "authentication" will find auth-related code even if the exact word isn't present.
func (si *SemanticSearchIndex) SearchByIntent(intent string) []SearchHit {
	expanded := ExpandQuery(intent)
	combinedQuery := strings.Join(expanded, " ")
	return si.Search(combinedQuery, 10)
}

// ExpandQuery expands a query term into related terms and synonyms.
func ExpandQuery(query string) []string {
	synonymMap := map[string][]string{
		"auth":           {"auth", "authenticate", "authentication", "authorization", "login", "logout", "token", "jwt", "session", "credential"},
		"authenticate":   {"auth", "authenticate", "authentication", "authorization", "login", "token", "jwt"},
		"authentication": {"auth", "authenticate", "authentication", "authorization", "login", "token", "jwt", "session"},
		"authorization":  {"auth", "authorize", "authorization", "permission", "role", "acl", "rbac"},
		"login":          {"login", "auth", "authenticate", "signin", "sign_in", "credential"},
		"test":           {"test", "spec", "assert", "expect", "verify", "mock", "stub", "fixture"},
		"error":          {"error", "err", "fail", "failure", "panic", "exception", "fatal", "warning"},
		"err":            {"error", "err", "fail", "panic", "exception"},
		"fail":           {"fail", "failure", "error", "err", "panic"},
		"database":       {"database", "db", "sql", "query", "table", "schema", "migration", "orm"},
		"db":             {"database", "db", "sql", "query", "table", "schema", "migration"},
		"http":           {"http", "request", "response", "handler", "middleware", "route", "endpoint", "api"},
		"api":            {"api", "endpoint", "rest", "handler", "route", "http", "request", "response"},
		"config":         {"config", "configuration", "settings", "env", "environment", "option", "parameter"},
		"cache":          {"cache", "store", "redis", "memcache", "ttl", "invalidate", "evict"},
		"log":            {"log", "logger", "logging", "debug", "info", "warn", "trace"},
		"parse":          {"parse", "parser", "tokenize", "lex", "ast", "syntax"},
		"file":           {"file", "io", "read", "write", "open", "close", "path", "directory"},
		"net":            {"net", "network", "tcp", "udp", "socket", "connect", "listen"},
		"concurrent":     {"concurrent", "goroutine", "channel", "mutex", "lock", "sync", "parallel", "async"},
		"mutex":          {"mutex", "lock", "sync", "concurrent", "race", "atomic"},
		"sort":           {"sort", "order", "rank", "compare", "less", "swap"},
		"search":         {"search", "find", "query", "lookup", "index", "match", "filter"},
		"validate":       {"validate", "validation", "check", "verify", "sanitize", "constraint"},
		"encrypt":        {"encrypt", "encryption", "decrypt", "cipher", "hash", "crypto", "aes", "rsa"},
		"middleware":     {"middleware", "handler", "intercept", "filter", "chain", "pipe"},
	}

	queryLower := strings.ToLower(strings.TrimSpace(query))
	terms := tokenizeSearch(queryLower)

	seen := make(map[string]bool)
	var result []string

	for _, term := range terms {
		if !seen[term] {
			seen[term] = true
			result = append(result, term)
		}
		if synonyms, ok := synonymMap[term]; ok {
			for _, syn := range synonyms {
				if !seen[syn] {
					seen[syn] = true
					result = append(result, syn)
				}
			}
		}
	}

	if len(result) == 0 {
		result = append(result, queryLower)
	}

	return result
}

// BM25Score computes the BM25 score for a document given query terms.
// Delegates to the shared BM25Scorer with precomputed IDF values.
func (si *SemanticSearchIndex) BM25Score(queryTerms []string, doc *Document) float64 {
	if doc.Length == 0 || si.AvgDocLen == 0 {
		return 0
	}
	return si.scorer.ScoreWithIDF(queryTerms, doc.Terms, si.IDF, float64(doc.Length), si.AvgDocLen)
}

// ExtractSnippet finds the most relevant lines from a document containing query terms.
func ExtractSnippet(doc *Document, queryTerms []string, maxLen int) string {
	if maxLen <= 0 {
		maxLen = 120
	}

	lines := strings.Split(doc.Content, "\n")
	if len(lines) == 0 {
		return ""
	}

	// Score each line by how many query terms it contains
	type lineScore struct {
		index int
		score int
		line  string
	}

	querySet := make(map[string]bool)
	for _, t := range queryTerms {
		querySet[strings.ToLower(t)] = true
	}

	var scored []lineScore
	for i, line := range lines {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" {
			continue
		}
		lineTokens := tokenizeSearch(line)
		hits := 0
		for _, lt := range lineTokens {
			if querySet[lt] {
				hits++
			}
		}
		if hits > 0 {
			scored = append(scored, lineScore{index: i, score: hits, line: trimmed})
		}
	}

	// If no lines matched query terms, return the first non-empty line
	if len(scored) == 0 {
		for _, line := range lines {
			trimmed := strings.TrimSpace(line)
			if trimmed != "" {
				if len(trimmed) > maxLen {
					return trimmed[:maxLen]
				}
				return trimmed
			}
		}
		return ""
	}

	// Sort by score descending
	sort.Slice(scored, func(i, j int) bool {
		return scored[i].score > scored[j].score
	})

	best := scored[0].line
	if len(best) > maxLen {
		best = best[:maxLen]
	}
	return best
}

// FormatResults produces a human-readable string of search results.
func FormatResults(query string, hits []SearchHit) string {
	if len(hits) == 0 {
		return fmt.Sprintf("Search: %q\n%s\nNo results found.\n", query, strings.Repeat("─", 33))
	}

	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("Search: %q\n", query))
	sb.WriteString(strings.Repeat("─", 33))
	sb.WriteString("\n")

	for i, hit := range hits {
		docName := hit.Document.Path
		if hit.Document.ID != "" && hit.Document.ID != hit.Document.Path {
			docName = fmt.Sprintf("%s:%s", hit.Document.Path, extractShortID(hit.Document.ID))
		}
		sb.WriteString(fmt.Sprintf("%d. [%.2f] %s\n", i+1, hit.Score, docName))
		if hit.Snippet != "" {
			sb.WriteString(fmt.Sprintf("   %q\n", hit.Snippet))
		}
		sb.WriteString("\n")
	}

	return sb.String()
}

// RebuildIndex recalculates IDF values and average document length.
func (si *SemanticSearchIndex) RebuildIndex() error {
	si.mu.Lock()
	defer si.mu.Unlock()
	si.rebuildIDFLocked()
	return nil
}

// --- Internal helpers ---

// rebuildIDFLocked recalculates IDF and AvgDocLen. Must be called with lock held.
func (si *SemanticSearchIndex) rebuildIDFLocked() {
	si.TotalDocs = len(si.Documents)
	if si.TotalDocs == 0 {
		si.IDF = make(map[string]float64)
		si.AvgDocLen = 0
		return
	}

	totalLen := 0
	termDocFreq := make(map[string]int)

	for _, doc := range si.Documents {
		totalLen += doc.Length
		seen := make(map[string]bool)
		for term := range doc.Terms {
			if !seen[term] {
				seen[term] = true
				termDocFreq[term]++
			}
		}
	}

	si.AvgDocLen = float64(totalLen) / float64(si.TotalDocs)

	si.IDF = make(map[string]float64, len(termDocFreq))
	n := float64(si.TotalDocs)
	for term, df := range termDocFreq {
		// BM25 IDF formula: log((N - df + 0.5) / (df + 0.5) + 1)
		si.IDF[term] = math.Log((n-float64(df)+0.5)/(float64(df)+0.5) + 1)
	}
}

// splitIntoDocuments splits source file content into logical documents (functions, types, blocks).
func splitIntoDocuments(path, content string) []*Document {
	var docs []*Document
	lines := strings.Split(content, "\n")

	// Detect language from extension
	ext := strings.ToLower(filepath.Ext(path))

	// Try to extract functions and types
	var currentDoc *documentBuilder
	var builders []*documentBuilder

	for i, line := range lines {
		trimmed := strings.TrimSpace(line)

		docType := detectDocumentType(trimmed, ext)
		if docType != "" {
			// Start a new document
			if currentDoc != nil {
				builders = append(builders, currentDoc)
			}
			name := extractName(trimmed, ext)
			currentDoc = &documentBuilder{
				startLine: i,
				name:      name,
				docType:   docType,
				lines:     []string{line},
			}
		} else if currentDoc != nil {
			currentDoc.lines = append(currentDoc.lines, line)
			// Check if this closes the current doc (simple brace counting)
			if isDocEnd(trimmed, currentDoc) {
				builders = append(builders, currentDoc)
				currentDoc = nil
			}
		}
	}
	if currentDoc != nil {
		builders = append(builders, currentDoc)
	}

	// Convert builders to documents
	for _, b := range builders {
		blockContent := strings.Join(b.lines, "\n")
		terms := buildTermFrequency(blockContent)
		docID := fmt.Sprintf("%s:%s", path, b.name)
		docs = append(docs, &Document{
			ID:      docID,
			Path:    path,
			Content: blockContent,
			Terms:   terms,
			Length:  countTerms(terms),
			Type:    b.docType,
		})
	}

	// Also index the whole file as a block if we found no sub-documents,
	// or always create a file-level document
	if len(docs) == 0 {
		terms := buildTermFrequency(content)
		docs = append(docs, &Document{
			ID:      path,
			Path:    path,
			Content: content,
			Terms:   terms,
			Length:  countTerms(terms),
			Type:    "file",
		})
	} else {
		// Add file-level document too for broader matching
		terms := buildTermFrequency(content)
		docs = append(docs, &Document{
			ID:      path + ":file",
			Path:    path,
			Content: content,
			Terms:   terms,
			Length:  countTerms(terms),
			Type:    "file",
		})
	}

	return docs
}

type documentBuilder struct {
	startLine  int
	name       string
	docType    string
	lines      []string
	braceDepth int
}

// detectDocumentType returns the type of document a line starts, or "" if none.
func detectDocumentType(line, ext string) string {
	switch ext {
	case ".go":
		if strings.HasPrefix(line, "func ") {
			return "function"
		}
		if strings.HasPrefix(line, "type ") && (strings.Contains(line, "struct") || strings.Contains(line, "interface")) {
			return "type"
		}
	case ".js", ".ts", ".jsx", ".tsx":
		if strings.HasPrefix(line, "function ") || strings.HasPrefix(line, "export function ") {
			return "function"
		}
		if strings.HasPrefix(line, "class ") || strings.HasPrefix(line, "export class ") {
			return "type"
		}
		if strings.Contains(line, "=> {") || strings.Contains(line, "=> (") {
			return "function"
		}
	case ".py":
		if strings.HasPrefix(line, "def ") {
			return "function"
		}
		if strings.HasPrefix(line, "class ") {
			return "type"
		}
	case ".rs":
		if strings.HasPrefix(line, "fn ") || strings.HasPrefix(line, "pub fn ") {
			return "function"
		}
		if strings.HasPrefix(line, "struct ") || strings.HasPrefix(line, "pub struct ") || strings.HasPrefix(line, "enum ") || strings.HasPrefix(line, "pub enum ") {
			return "type"
		}
	case ".java", ".kt":
		if strings.HasPrefix(line, "public ") || strings.HasPrefix(line, "private ") || strings.HasPrefix(line, "protected ") {
			if strings.Contains(line, "class ") {
				return "type"
			}
			if strings.Contains(line, "(") {
				return "function"
			}
		}
		if strings.HasPrefix(line, "class ") {
			return "type"
		}
	case ".rb":
		if strings.HasPrefix(line, "def ") {
			return "function"
		}
		if strings.HasPrefix(line, "class ") || strings.HasPrefix(line, "module ") {
			return "type"
		}
	case ".c", ".cpp", ".h", ".hpp":
		if strings.Contains(line, "(") && !strings.HasPrefix(line, "#") && !strings.HasPrefix(line, "//") && !strings.HasPrefix(line, "/*") {
			if strings.HasSuffix(line, "{") || strings.HasSuffix(line, ")") {
				return "function"
			}
		}
		if strings.HasPrefix(line, "struct ") || strings.HasPrefix(line, "class ") {
			return "type"
		}
	}
	return ""
}

// extractName tries to get a meaningful name from a declaration line.
func extractName(line, ext string) string {
	// Remove common prefixes
	line = strings.TrimPrefix(line, "export ")
	line = strings.TrimPrefix(line, "pub ")
	line = strings.TrimPrefix(line, "public ")
	line = strings.TrimPrefix(line, "private ")
	line = strings.TrimPrefix(line, "protected ")

	switch ext {
	case ".go":
		line = strings.TrimPrefix(line, "func ")
		line = strings.TrimPrefix(line, "type ")
		// Handle method receivers: (r *Receiver) MethodName(...)
		if strings.HasPrefix(line, "(") {
			if idx := strings.Index(line, ")"); idx != -1 {
				line = strings.TrimSpace(line[idx+1:])
			}
		}
	case ".py":
		line = strings.TrimPrefix(line, "def ")
		line = strings.TrimPrefix(line, "class ")
	case ".js", ".ts", ".jsx", ".tsx":
		line = strings.TrimPrefix(line, "function ")
		line = strings.TrimPrefix(line, "class ")
		line = strings.TrimPrefix(line, "const ")
		line = strings.TrimPrefix(line, "let ")
		line = strings.TrimPrefix(line, "var ")
	case ".rs":
		line = strings.TrimPrefix(line, "fn ")
		line = strings.TrimPrefix(line, "struct ")
		line = strings.TrimPrefix(line, "enum ")
	case ".rb":
		line = strings.TrimPrefix(line, "def ")
		line = strings.TrimPrefix(line, "class ")
		line = strings.TrimPrefix(line, "module ")
	case ".java", ".kt":
		line = strings.TrimPrefix(line, "class ")
		// For methods, strip return type
		parts := strings.Fields(line)
		if len(parts) >= 2 && strings.Contains(parts[len(parts)-1], "(") {
			line = parts[len(parts)-1]
		}
	default:
		line = strings.TrimPrefix(line, "func ")
		line = strings.TrimPrefix(line, "function ")
		line = strings.TrimPrefix(line, "def ")
		line = strings.TrimPrefix(line, "class ")
	}

	// Extract just the name (up to first paren, space, or brace)
	var nameBuilder strings.Builder
	for _, r := range line {
		if r == '(' || r == '{' || r == ' ' || r == ':' || r == '<' {
			break
		}
		nameBuilder.WriteRune(r)
	}
	name := strings.TrimSpace(nameBuilder.String())
	if name == "" {
		name = "anonymous"
	}
	return name
}

// isDocEnd checks if a line signals the end of a document (simple heuristic).
func isDocEnd(line string, builder *documentBuilder) bool {
	// Count braces in all lines
	for _, r := range line {
		if r == '{' {
			builder.braceDepth++
		} else if r == '}' {
			builder.braceDepth--
		}
	}

	// For Python/Ruby, use dedent heuristic
	if builder.braceDepth == 0 && len(builder.lines) > 1 {
		// If we started tracking and braces balanced, end here
		hasOpener := false
		for _, l := range builder.lines {
			if strings.Contains(l, "{") {
				hasOpener = true
				break
			}
		}
		if hasOpener {
			return true
		}
	}

	// For Python: if we see another def/class at same indentation, end previous
	if len(builder.lines) > 3 && (strings.HasPrefix(line, "def ") || strings.HasPrefix(line, "class ")) {
		return true
	}

	// Limit document size
	if len(builder.lines) > 100 {
		return true
	}

	return false
}

// buildTermFrequency tokenizes content and builds a term frequency map.
func buildTermFrequency(content string) map[string]int {
	terms := make(map[string]int)
	tokens := tokenizeSearch(content)
	for _, t := range tokens {
		terms[t]++
	}
	return terms
}

// countTerms returns the total number of term occurrences.
func countTerms(terms map[string]int) int {
	total := 0
	for _, count := range terms {
		total += count
	}
	return total
}

// tokenizeSearch splits text into lowercase tokens for search indexing.
// It handles camelCase and snake_case splitting, emitting both full words and sub-words.
func tokenizeSearch(text string) []string {
	var tokens []string
	var current strings.Builder

	// First pass: split on non-alphanumeric boundaries and underscores
	for _, r := range text {
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			current.WriteRune(r)
		} else if r == '_' {
			if current.Len() > 1 {
				tokens = append(tokens, strings.ToLower(current.String()))
			}
			current.Reset()
		} else {
			if current.Len() > 1 {
				tokens = append(tokens, strings.ToLower(current.String()))
			}
			current.Reset()
		}
	}
	if current.Len() > 1 {
		tokens = append(tokens, strings.ToLower(current.String()))
	}

	// Second pass: split camelCase tokens into sub-words
	var expanded []string
	for _, tok := range tokens {
		expanded = append(expanded, tok)
		subWords := splitCamelCaseSearch(tok)
		if len(subWords) > 1 {
			for _, sw := range subWords {
				if len(sw) > 1 {
					expanded = append(expanded, sw)
				}
			}
		}
	}

	return expanded
}

// splitCamelCaseSearch splits a lowercase token that was originally camelCase into sub-words.
// Since we already lowercased, we try to detect word boundaries using the original structure.
// We re-split the original by looking for common patterns.
func splitCamelCaseSearch(token string) []string {
	// Since the token is already lowercased, we need to detect boundaries
	// by looking at common word patterns. We'll use a simple heuristic:
	// look for positions where a common short word ends.
	// A better approach: split using the original text. But since we already lowered,
	// we try a prefix-match approach for known common prefixes.
	commonPrefixes := []string{
		"get", "set", "new", "is", "has", "can", "do", "to",
		"on", "handle", "create", "delete", "update", "find",
		"auth", "validate", "check", "parse", "build", "make",
		"read", "write", "open", "close", "start", "stop",
		"add", "remove", "init", "load", "save", "send",
		"http", "json", "xml", "sql", "api", "url", "uri",
		"require", "process", "generate", "compute", "calculate",
		"middleware", "handler", "service", "controller", "repository",
		"request", "response", "error", "status", "config",
		"user", "token", "session", "login", "logout",
	}

	// Try to find if this token can be decomposed into known words
	if len(token) < 4 {
		return []string{token}
	}

	// Attempt greedy decomposition
	result := greedyDecompose(token, commonPrefixes)
	if len(result) > 1 {
		return result
	}

	return []string{token}
}

// greedyDecompose attempts to split a token into known sub-words using greedy matching.
func greedyDecompose(token string, vocab []string) []string {
	if len(token) == 0 {
		return nil
	}

	// Sort vocab by length descending for greedy matching
	sorted := make([]string, len(vocab))
	copy(sorted, vocab)
	sort.Slice(sorted, func(i, j int) bool {
		return len(sorted[i]) > len(sorted[j])
	})

	var parts []string
	remaining := token

	for len(remaining) > 0 {
		found := false
		for _, word := range sorted {
			if strings.HasPrefix(remaining, word) && len(word) < len(remaining) {
				parts = append(parts, word)
				remaining = remaining[len(word):]
				found = true
				break
			}
		}
		if !found {
			// Can't decompose further, include remainder if long enough
			if len(remaining) > 1 {
				parts = append(parts, remaining)
			}
			break
		}
	}

	if len(parts) <= 1 {
		return []string{token}
	}
	return parts
}

// matchedTerms returns the query terms that appear in the document.
func matchedTerms(queryTerms []string, doc *Document) []string {
	var matched []string
	for _, t := range queryTerms {
		if doc.Terms[t] > 0 {
			matched = append(matched, t)
		}
	}
	return matched
}

// extractShortID extracts the short name part from a document ID like "path:name".
func extractShortID(id string) string {
	if idx := strings.LastIndex(id, ":"); idx != -1 {
		return id[idx+1:]
	}
	return id
}

// isSupportedSourceExt returns whether the file extension is a supported source type.
func isSupportedSourceExt(ext string) bool {
	switch strings.ToLower(ext) {
	case ".go", ".py", ".js", ".ts", ".jsx", ".tsx",
		".java", ".kt", ".rs", ".rb", ".c", ".cpp", ".h", ".hpp",
		".cs", ".swift", ".scala", ".php", ".lua", ".zig",
		".sh", ".bash", ".zsh", ".fish",
		".yaml", ".yml", ".toml", ".json", ".xml",
		".sql", ".graphql", ".proto",
		".md", ".txt", ".rst":
		return true
	}
	return false
}
