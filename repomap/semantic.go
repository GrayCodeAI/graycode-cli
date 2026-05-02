package repomap

import (
	"encoding/gob"
	"math"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"unicode"
)

// CodeChunk represents a chunk of source code for semantic search.
type CodeChunk struct {
	Path      string
	StartLine int
	EndLine   int
	Content   string
	Vector    []float32 // embedding (if computed)
}

// SemanticIndex holds chunked source files and supports TF-IDF search.
type SemanticIndex struct {
	chunks []CodeChunk
	// Pre-computed IDF values (lazily built on first search)
	idf     map[string]float64
	idfDone bool
}

// BuildSemanticIndex scans dir, chunks files into ~40-line blocks, and builds an index.
func BuildSemanticIndex(dir string, ignore []string, maxFiles int) (*SemanticIndex, error) {
	if maxFiles <= 0 {
		maxFiles = 500
	}

	ignoreSet := make(map[string]bool)
	for _, p := range defaultIgnorePatterns {
		ignoreSet[p] = true
	}
	for _, p := range ignore {
		ignoreSet[p] = true
	}

	var files []string
	err := filepath.Walk(dir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return nil
		}
		if info.IsDir() {
			if ignoreSet[filepath.Base(path)] {
				return filepath.SkipDir
			}
			return nil
		}
		if len(files) >= maxFiles {
			return filepath.SkipAll
		}
		if isSupportedExt(filepath.Ext(path)) {
			files = append(files, path)
		}
		return nil
	})
	if err != nil {
		return nil, err
	}

	idx := &SemanticIndex{}
	const chunkSize = 40

	for _, f := range files {
		data, err := os.ReadFile(f)
		if err != nil {
			continue
		}
		relPath, relErr := filepath.Rel(dir, f)
		if relErr != nil {
			relPath = f
		}

		lines := strings.Split(string(data), "\n")
		for start := 0; start < len(lines); start += chunkSize {
			end := start + chunkSize
			if end > len(lines) {
				end = len(lines)
			}
			content := strings.Join(lines[start:end], "\n")
			if strings.TrimSpace(content) == "" {
				continue
			}
			idx.chunks = append(idx.chunks, CodeChunk{
				Path:      relPath,
				StartLine: start + 1,
				EndLine:   end,
				Content:   content,
			})
		}
	}

	return idx, nil
}

// Search performs TF-IDF based search over the index, returning the top-K chunks.
func (idx *SemanticIndex) Search(query string, topK int) []CodeChunk {
	if len(idx.chunks) == 0 || query == "" {
		return nil
	}
	if topK <= 0 {
		topK = 5
	}

	// Build IDF on first search
	if !idx.idfDone {
		idx.buildIDF()
	}

	queryTerms := tokenize(query)
	if len(queryTerms) == 0 {
		return nil
	}

	type scored struct {
		idx   int
		score float64
	}
	var results []scored

	for i, chunk := range idx.chunks {
		score := idx.scoreTFIDF(chunk.Content, queryTerms)
		if score > 0 {
			results = append(results, scored{idx: i, score: score})
		}
	}

	sort.Slice(results, func(a, b int) bool {
		return results[a].score > results[b].score
	})

	if len(results) > topK {
		results = results[:topK]
	}

	out := make([]CodeChunk, len(results))
	for i, r := range results {
		out[i] = idx.chunks[r.idx]
	}
	return out
}

// Size returns the number of chunks in the index.
func (idx *SemanticIndex) Size() int {
	return len(idx.chunks)
}

// Save encodes the index to a file using gob.
func (idx *SemanticIndex) Save(path string) error {
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	defer f.Close()
	enc := gob.NewEncoder(f)
	return enc.Encode(idx.chunks)
}

// LoadSemanticIndex decodes a previously saved index from a gob file.
func LoadSemanticIndex(path string) (*SemanticIndex, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	var chunks []CodeChunk
	dec := gob.NewDecoder(f)
	if err := dec.Decode(&chunks); err != nil {
		return nil, err
	}
	return &SemanticIndex{chunks: chunks}, nil
}

// buildIDF computes inverse document frequency for all terms across all chunks.
func (idx *SemanticIndex) buildIDF() {
	docCount := float64(len(idx.chunks))
	termDocFreq := make(map[string]int)

	for _, chunk := range idx.chunks {
		seen := make(map[string]bool)
		for _, term := range tokenize(chunk.Content) {
			if !seen[term] {
				seen[term] = true
				termDocFreq[term]++
			}
		}
	}

	idx.idf = make(map[string]float64, len(termDocFreq))
	for term, df := range termDocFreq {
		idx.idf[term] = math.Log(1 + docCount/float64(df))
	}
	idx.idfDone = true
}

// scoreTFIDF computes the TF-IDF score for a document against query terms.
func (idx *SemanticIndex) scoreTFIDF(content string, queryTerms []string) float64 {
	docTerms := tokenize(content)
	if len(docTerms) == 0 {
		return 0
	}

	// Compute term frequencies in this document
	tf := make(map[string]int, len(docTerms))
	for _, t := range docTerms {
		tf[t]++
	}

	var score float64
	docLen := float64(len(docTerms))
	for _, qt := range queryTerms {
		freq, ok := tf[qt]
		if !ok {
			continue
		}
		termFreq := float64(freq) / docLen
		idfVal := idx.idf[qt]
		score += termFreq * idfVal
	}
	return score
}

// tokenize splits text into lowercase alphanumeric tokens.
func tokenize(text string) []string {
	text = strings.ToLower(text)
	var tokens []string
	var current strings.Builder

	for _, r := range text {
		if unicode.IsLetter(r) || unicode.IsDigit(r) || r == '_' {
			current.WriteRune(r)
		} else {
			if current.Len() > 1 { // skip single-char tokens
				tokens = append(tokens, current.String())
			}
			current.Reset()
		}
	}
	if current.Len() > 1 {
		tokens = append(tokens, current.String())
	}
	return tokens
}
