package engine

import (
	"fmt"
	"strings"
	"sync"
	"unicode"
)

// HallucinationGuard validates agent outputs against source context,
// rejecting responses that contain unsupported factual claims.
type HallucinationGuard struct {
	Enabled    bool
	Threshold  float64 // fraction of claims that must be grounded (e.g. 0.7 = 70%)
	MaxRetries int
	mu         sync.Mutex
}

// GroundingResult holds the outcome of checking a response against context.
type GroundingResult struct {
	Score             float64
	SupportedClaims   []string
	UnsupportedClaims []string
	TotalClaims       int
	Grounded          bool
}

// NewHallucinationGuard creates a HallucinationGuard with sensible defaults.
func NewHallucinationGuard() *HallucinationGuard {
	return &HallucinationGuard{
		Enabled:    true,
		Threshold:  0.7,
		MaxRetries: 2,
	}
}

// Check validates a response against the provided context sources.
// It extracts factual claims, verifies each against context, and returns
// a grounding result indicating whether the response is sufficiently supported.
func (hg *HallucinationGuard) Check(response string, context []string) *GroundingResult {
	hg.mu.Lock()
	defer hg.mu.Unlock()

	claims := hg.ExtractClaims(response)

	result := &GroundingResult{
		SupportedClaims:   make([]string, 0),
		UnsupportedClaims: make([]string, 0),
		TotalClaims:       len(claims),
	}

	if len(claims) == 0 {
		result.Score = 1.0
		result.Grounded = true
		return result
	}

	for _, claim := range claims {
		if hg.VerifyClaim(claim, context) {
			result.SupportedClaims = append(result.SupportedClaims, claim)
		} else {
			result.UnsupportedClaims = append(result.UnsupportedClaims, claim)
		}
	}

	result.Score = float64(len(result.SupportedClaims)) / float64(result.TotalClaims)
	result.Grounded = result.Score >= hg.Threshold

	return result
}

// ExtractClaims splits text into sentences and filters to those that
// contain specific factual assertions (names, numbers, paths, technical terms).
// Opinions, questions, and hedged statements are excluded.
func (hg *HallucinationGuard) ExtractClaims(text string) []string {
	sentences := splitSentences(text)
	claims := make([]string, 0)

	for _, s := range sentences {
		s = strings.TrimSpace(s)
		if s == "" {
			continue
		}
		// Skip questions
		if strings.HasSuffix(s, "?") {
			continue
		}
		// Skip hedged statements
		if isHedged(s) {
			continue
		}
		// Keep only factual claims (contain specific details)
		if isFactualClaim(s) {
			claims = append(claims, s)
		}
	}

	return claims
}

// VerifyClaim checks whether a claim's key terms appear sufficiently in the
// provided context. A word overlap threshold of > 0.5 is required.
func (hg *HallucinationGuard) VerifyClaim(claim string, context []string) bool {
	keyTerms := hg.ExtractKeyTerms(claim)
	if len(keyTerms) == 0 {
		return true // no verifiable terms, assume supported
	}

	combinedContext := strings.ToLower(strings.Join(context, " "))

	matchCount := 0
	for _, term := range keyTerms {
		if strings.Contains(combinedContext, strings.ToLower(term)) {
			matchCount++
		}
	}

	overlap := float64(matchCount) / float64(len(keyTerms))
	return overlap > 0.5
}

// ExtractKeyTerms removes stop words and returns nouns, numbers, paths,
// and identifiers from the claim text.
func (hg *HallucinationGuard) ExtractKeyTerms(claim string) []string {
	words := tokenizeClaim(claim)
	terms := make([]string, 0)

	for _, w := range words {
		if isStopWord(w) {
			continue
		}
		if isKeyTerm(w) {
			terms = append(terms, w)
		}
	}

	return terms
}

// BuildRejectionMessage constructs a human-readable message explaining
// which claims in the response were not supported by context.
func BuildRejectionMessage(result *GroundingResult) string {
	var b strings.Builder
	b.WriteString("Response contains unsupported claims:\n")

	for _, claim := range result.UnsupportedClaims {
		b.WriteString(fmt.Sprintf("- %q (not found in context)\n", claim))
	}

	b.WriteString("\nPlease verify these claims or rephrase with less certainty.")
	return b.String()
}

// FormatGroundingResult returns a human-readable summary of the grounding analysis.
func FormatGroundingResult(result *GroundingResult) string {
	var b strings.Builder

	status := "GROUNDED"
	if !result.Grounded {
		status = "NOT GROUNDED"
	}

	b.WriteString(fmt.Sprintf("Hallucination Check: %s\n", status))
	b.WriteString(fmt.Sprintf("Score: %.2f (%d/%d claims supported)\n",
		result.Score, len(result.SupportedClaims), result.TotalClaims))

	if len(result.SupportedClaims) > 0 {
		b.WriteString("\nSupported:\n")
		for _, c := range result.SupportedClaims {
			b.WriteString(fmt.Sprintf("  + %s\n", c))
		}
	}

	if len(result.UnsupportedClaims) > 0 {
		b.WriteString("\nUnsupported:\n")
		for _, c := range result.UnsupportedClaims {
			b.WriteString(fmt.Sprintf("  - %s\n", c))
		}
	}

	return b.String()
}

// splitSentences breaks text into sentences on common delimiters.
func splitSentences(text string) []string {
	// Split on sentence-ending punctuation followed by space or end of string
	var sentences []string
	var current strings.Builder

	runes := []rune(text)
	for i := 0; i < len(runes); i++ {
		current.WriteRune(runes[i])

		if runes[i] == '.' || runes[i] == '!' || runes[i] == '?' {
			// Check if this is likely a sentence boundary (followed by space+uppercase, end of text, or newline)
			if i == len(runes)-1 {
				sentences = append(sentences, current.String())
				current.Reset()
			} else if i+1 < len(runes) && (runes[i+1] == ' ' || runes[i+1] == '\n') {
				// Avoid splitting on abbreviations or decimals
				if runes[i] == '.' && i > 0 && unicode.IsDigit(runes[i-1]) && i+1 < len(runes) && unicode.IsDigit(runes[i+1]) {
					continue
				}
				sentences = append(sentences, current.String())
				current.Reset()
			}
		} else if runes[i] == '\n' {
			s := strings.TrimSpace(current.String())
			if s != "" {
				sentences = append(sentences, s)
			}
			current.Reset()
		}
	}

	if s := strings.TrimSpace(current.String()); s != "" {
		sentences = append(sentences, s)
	}

	return sentences
}

// isHedged returns true if a sentence contains hedging language indicating uncertainty.
func isHedged(s string) bool {
	lower := strings.ToLower(s)
	hedgeWords := []string{
		"might", "probably", "perhaps", "maybe", "possibly",
		"could be", "i think", "i believe", "it seems",
		"not sure", "uncertain", "likely", "unlikely",
	}
	for _, hw := range hedgeWords {
		if strings.Contains(lower, hw) {
			return true
		}
	}
	return false
}

// isFactualClaim returns true if a sentence contains specific factual details
// (numbers, paths, identifiers, technical terms) that can be verified.
func isFactualClaim(s string) bool {
	// Contains a number
	for _, r := range s {
		if unicode.IsDigit(r) {
			return true
		}
	}
	// Contains a file path
	if strings.Contains(s, "/") || strings.Contains(s, "\\") {
		return true
	}
	// Contains identifiers (camelCase, snake_case, or dot notation)
	if strings.Contains(s, "_") || strings.Contains(s, ".") {
		return true
	}
	// Contains backtick-quoted code references
	if strings.Contains(s, "`") {
		return true
	}
	// Contains type annotations or brackets indicating technical content
	if strings.Contains(s, "[]") || strings.Contains(s, "map[") {
		return true
	}
	// Contains specific technical keywords that imply factual claims
	lower := strings.ToLower(s)
	techTerms := []string{
		"returns", "accepts", "implements", "calls",
		"defined in", "located at", "version",
		"requires", "depends on", "contains",
	}
	for _, t := range techTerms {
		if strings.Contains(lower, t) {
			return true
		}
	}
	return false
}

// tokenizeClaim splits text into words, handling punctuation and special characters.
func tokenizeClaim(text string) []string {
	// Split on whitespace and common punctuation boundaries
	words := make([]string, 0)
	var current strings.Builder

	for _, r := range text {
		if unicode.IsLetter(r) || unicode.IsDigit(r) || r == '_' || r == '/' || r == '.' || r == '-' {
			current.WriteRune(r)
		} else {
			if current.Len() > 0 {
				words = append(words, current.String())
				current.Reset()
			}
		}
	}
	if current.Len() > 0 {
		words = append(words, current.String())
	}

	// Trim trailing punctuation from tokens (sentence-ending dots, etc.)
	for i, w := range words {
		words[i] = strings.TrimRight(w, ".")
	}

	return words
}

// isStopWord returns true if the word is a common English stop word.
func isStopWord(w string) bool {
	stops := map[string]bool{
		"the": true, "a": true, "an": true, "is": true, "are": true,
		"was": true, "were": true, "be": true, "been": true, "being": true,
		"have": true, "has": true, "had": true, "do": true, "does": true,
		"did": true, "will": true, "would": true, "could": true, "should": true,
		"may": true, "might": true, "shall": true, "can": true,
		"it": true, "its": true, "this": true, "that": true, "these": true,
		"those": true, "i": true, "you": true, "he": true, "she": true,
		"we": true, "they": true, "me": true, "him": true, "her": true,
		"us": true, "them": true, "my": true, "your": true, "his": true,
		"our": true, "their": true, "which": true, "who": true, "whom": true,
		"what": true, "where": true, "when": true, "how": true, "why": true,
		"in": true, "on": true, "at": true, "to": true, "for": true,
		"with": true, "from": true, "by": true, "of": true, "about": true,
		"into": true, "through": true, "during": true, "before": true,
		"after": true, "above": true, "below": true, "between": true,
		"and": true, "but": true, "or": true, "nor": true, "not": true,
		"so": true, "if": true, "then": true, "than": true, "as": true,
		"also": true, "very": true, "just": true, "only": true,
	}
	return stops[strings.ToLower(w)]
}

// isKeyTerm returns true if the word looks like a noun, number, path,
// or identifier worth verifying against context.
func isKeyTerm(w string) bool {
	// Numbers
	if len(w) > 0 && unicode.IsDigit(rune(w[0])) {
		return true
	}
	// Paths (contain / or \)
	if strings.Contains(w, "/") {
		return true
	}
	// Identifiers with underscores or dots
	if strings.Contains(w, "_") || strings.Contains(w, ".") {
		return true
	}
	// CamelCase identifiers
	hasUpper := false
	hasLower := false
	for _, r := range w {
		if unicode.IsUpper(r) {
			hasUpper = true
		}
		if unicode.IsLower(r) {
			hasLower = true
		}
	}
	if hasUpper && hasLower && len(w) > 2 {
		return true
	}
	// Version strings (start with v followed by digit)
	if len(w) > 1 && (w[0] == 'v' || w[0] == 'V') && unicode.IsDigit(rune(w[1])) {
		return true
	}
	// Words that are long enough to be meaningful (likely nouns/technical terms)
	if len(w) >= 4 && !isCommonVerb(w) {
		return true
	}
	return false
}

// isCommonVerb returns true for common verbs that aren't useful as key terms.
func isCommonVerb(w string) bool {
	verbs := map[string]bool{
		"returns": true, "accepts": true, "calls": true, "uses": true,
		"creates": true, "takes": true, "makes": true, "gets": true,
		"sets": true, "adds": true, "runs": true, "puts": true,
		"gives": true, "tells": true, "shows": true, "finds": true,
		"keeps": true, "lets": true, "begins": true, "seems": true,
		"helps": true, "turns": true, "starts": true, "provides": true,
		"works": true, "includes": true, "contains": true, "defined": true,
		"located": true, "implements": true, "requires": true, "depends": true,
	}
	return verbs[strings.ToLower(w)]
}
