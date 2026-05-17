package engine

import (
	"fmt"
	"strings"
	"sync"
)

// EfficientPrompter implements a token-efficient prompting system that minimizes
// token usage while maintaining output quality through various text optimization strategies.
type EfficientPrompter struct {
	Strategies []PromptOpt
	Stats      EfficientStats
	mu         sync.RWMutex
}

// PromptOpt represents a single optimization strategy that can be applied to prompts.
type PromptOpt struct {
	Name    string
	ApplyFn func(string) string
	Enabled bool
}

// EfficientStats tracks cumulative token savings across all optimization calls.
type EfficientStats struct {
	OriginalTokens  int
	OptimizedTokens int
	TotalSavings    int
	CallCount       int
}

// OptimizedResult contains the result of optimizing a single prompt.
type OptimizedResult struct {
	Original    string
	Optimized   string
	TokensSaved int
	Applied     []string
}

// PromptMsg represents a message in a conversation with role, content, and token count.
type PromptMsg struct {
	Role    string
	Content string
	Tokens  int
}

// NewEfficientPrompter creates a new EfficientPrompter with all default strategies enabled.
func NewEfficientPrompter() *EfficientPrompter {
	ep := &EfficientPrompter{}
	ep.Strategies = []PromptOpt{
		{
			Name:    "compress_whitespace",
			Enabled: true,
			ApplyFn: compressWhitespace,
		},
		{
			Name:    "remove_filler",
			Enabled: true,
			ApplyFn: removeFiller,
		},
		{
			Name:    "abbreviate_phrases",
			Enabled: true,
			ApplyFn: abbreviatePhrases,
		},
		{
			Name:    "shorten_paths",
			Enabled: true,
			ApplyFn: shortenPaths,
		},
		{
			Name:    "collapse_repeated",
			Enabled: true,
			ApplyFn: collapseRepeated,
		},
		{
			Name:    "strip_pleasantries",
			Enabled: true,
			ApplyFn: stripPleasantries,
		},
	}
	return ep
}

// Optimize applies all enabled strategies to the given prompt and returns an OptimizedResult.
func (ep *EfficientPrompter) Optimize(prompt string) *OptimizedResult {
	ep.mu.Lock()
	defer ep.mu.Unlock()

	result := &OptimizedResult{
		Original: prompt,
		Applied:  []string{},
	}

	optimized := prompt
	for _, strategy := range ep.Strategies {
		if !strategy.Enabled {
			continue
		}
		before := optimized
		optimized = strategy.ApplyFn(optimized)
		if optimized != before {
			result.Applied = append(result.Applied, strategy.Name)
		}
	}

	result.Optimized = optimized

	originalTokens := epEstimateTokens(prompt)
	optimizedTokens := epEstimateTokens(optimized)
	result.TokensSaved = originalTokens - optimizedTokens

	ep.Stats.OriginalTokens += originalTokens
	ep.Stats.OptimizedTokens += optimizedTokens
	ep.Stats.TotalSavings += result.TokensSaved
	ep.Stats.CallCount++

	return result
}

// OptimizeOutput removes noise from tool outputs and collapses repeated lines.
func (ep *EfficientPrompter) OptimizeOutput(output string) string {
	ep.mu.Lock()
	defer ep.mu.Unlock()

	result := stripPleasantries(output)
	result = collapseRepeated(result)
	result = compressWhitespace(result)

	originalTokens := epEstimateTokens(output)
	optimizedTokens := epEstimateTokens(result)

	ep.Stats.OriginalTokens += originalTokens
	ep.Stats.OptimizedTokens += optimizedTokens
	ep.Stats.TotalSavings += (originalTokens - optimizedTokens)
	ep.Stats.CallCount++

	return result
}

// OptimizeMessages compresses older messages more aggressively while keeping
// recent messages mostly intact.
func (ep *EfficientPrompter) OptimizeMessages(messages []PromptMsg) []PromptMsg {
	ep.mu.Lock()
	defer ep.mu.Unlock()

	if len(messages) == 0 {
		return messages
	}

	result := make([]PromptMsg, len(messages))
	copy(result, messages)

	// Keep the last 3 messages mostly intact, compress older ones more aggressively
	threshold := len(result) - 3
	if threshold < 0 {
		threshold = 0
	}

	for i := 0; i < len(result); i++ {
		if i < threshold {
			// Aggressive compression for older messages
			content := result[i].Content
			for _, strategy := range ep.Strategies {
				if strategy.Enabled {
					content = strategy.ApplyFn(content)
				}
			}
			// Additional aggressive compression for old messages: truncate long content
			if len(content) > 200 {
				content = content[:200] + "..."
			}
			originalTokens := epEstimateTokens(result[i].Content)
			optimizedTokens := epEstimateTokens(content)
			ep.Stats.OriginalTokens += originalTokens
			ep.Stats.OptimizedTokens += optimizedTokens
			ep.Stats.TotalSavings += (originalTokens - optimizedTokens)
			result[i].Content = content
			result[i].Tokens = optimizedTokens
		} else {
			// Light compression for recent messages
			content := result[i].Content
			content = compressWhitespace(content)
			content = removeFiller(content)
			originalTokens := epEstimateTokens(result[i].Content)
			optimizedTokens := epEstimateTokens(content)
			ep.Stats.OriginalTokens += originalTokens
			ep.Stats.OptimizedTokens += optimizedTokens
			ep.Stats.TotalSavings += (originalTokens - optimizedTokens)
			result[i].Content = content
			result[i].Tokens = optimizedTokens
		}
	}

	ep.Stats.CallCount++
	return result
}

// EstimateSavings returns the estimated number of tokens that would be saved
// by optimizing the given prompt, without actually modifying stats.
func (ep *EfficientPrompter) EstimateSavings(prompt string) int {
	ep.mu.RLock()
	defer ep.mu.RUnlock()

	optimized := prompt
	for _, strategy := range ep.Strategies {
		if !strategy.Enabled {
			continue
		}
		optimized = strategy.ApplyFn(optimized)
	}

	return epEstimateTokens(prompt) - epEstimateTokens(optimized)
}

// FormatEfficientStats returns a human-readable summary of token savings.
func (ep *EfficientPrompter) FormatEfficientStats() string {
	ep.mu.RLock()
	defer ep.mu.RUnlock()

	if ep.Stats.OriginalTokens == 0 {
		return "Token Efficiency:\nCalls: 0 | Saved: 0 tokens (0.0%)"
	}

	pct := float64(ep.Stats.TotalSavings) / float64(ep.Stats.OriginalTokens) * 100.0

	return fmt.Sprintf(
		"Token Efficiency:\nCalls: %d | Saved: %s tokens (%.1f%%)",
		ep.Stats.CallCount,
		epFormatNumber(ep.Stats.TotalSavings),
		pct,
	)
}

// EnableStrategy enables the named strategy.
func (ep *EfficientPrompter) EnableStrategy(name string) {
	ep.mu.Lock()
	defer ep.mu.Unlock()

	for i := range ep.Strategies {
		if ep.Strategies[i].Name == name {
			ep.Strategies[i].Enabled = true
			return
		}
	}
}

// DisableStrategy disables the named strategy.
func (ep *EfficientPrompter) DisableStrategy(name string) {
	ep.mu.Lock()
	defer ep.mu.Unlock()

	for i := range ep.Strategies {
		if ep.Strategies[i].Name == name {
			ep.Strategies[i].Enabled = false
			return
		}
	}
}

// --- Strategy implementations ---

// compressWhitespace collapses multiple consecutive whitespace characters into a single space.
func compressWhitespace(s string) string {
	var b strings.Builder
	b.Grow(len(s))
	inSpace := false
	for _, r := range s {
		if r == ' ' || r == '\t' {
			if !inSpace {
				b.WriteRune(' ')
				inSpace = true
			}
		} else if r == '\n' {
			// Preserve newlines but collapse blank lines
			if !inSpace {
				b.WriteRune('\n')
			} else {
				// Replace trailing space + newline with just newline
				// Rewrite last character if it was a space
				str := b.String()
				if len(str) > 0 && str[len(str)-1] == ' ' {
					b.Reset()
					b.WriteString(str[:len(str)-1])
				}
				b.WriteRune('\n')
			}
			inSpace = false
		} else {
			inSpace = false
			b.WriteRune(r)
		}
	}
	// Collapse multiple consecutive blank lines into one
	result := b.String()
	for strings.Contains(result, "\n\n\n") {
		result = strings.ReplaceAll(result, "\n\n\n", "\n\n")
	}
	return result
}

// removeFiller strips common filler words and phrases.
func removeFiller(s string) string {
	fillers := []string{
		"please ",
		"Please ",
		"could you ",
		"Could you ",
		"I would like ",
		"I would like to ",
		"i would like ",
		"i would like to ",
		"can you please ",
		"Can you please ",
		"would you please ",
		"Would you please ",
		"kindly ",
		"Kindly ",
	}
	result := s
	for _, filler := range fillers {
		result = strings.ReplaceAll(result, filler, "")
	}
	return result
}

// abbreviatePhrases replaces common verbose phrases with shorter equivalents.
func abbreviatePhrases(s string) string {
	replacements := []struct {
		old string
		new string
	}{
		{"for example", "e.g."},
		{"For example", "E.g."},
		{"in order to", "to"},
		{"In order to", "To"},
		{"as well as", "and"},
		{"due to the fact that", "because"},
		{"Due to the fact that", "Because"},
		{"in the event that", "if"},
		{"In the event that", "If"},
		{"at this point in time", "now"},
		{"At this point in time", "Now"},
		{"a large number of", "many"},
		{"A large number of", "Many"},
		{"in spite of the fact that", "although"},
		{"In spite of the fact that", "Although"},
		{"with regard to", "about"},
		{"With regard to", "About"},
		{"on the other hand", "however"},
		{"On the other hand", "However"},
		{"it is important to note that", "note:"},
		{"It is important to note that", "Note:"},
	}
	result := s
	for _, r := range replacements {
		result = strings.ReplaceAll(result, r.old, r.new)
	}
	return result
}

// shortenPaths attempts to shorten absolute file paths by using relative paths from project root.
func shortenPaths(s string) string {
	// Look for common path prefixes and shorten them
	prefixes := []string{
		"/home/",
		"/Users/",
		"/tmp/",
	}
	result := s
	for _, prefix := range prefixes {
		for {
			idx := strings.Index(result, prefix)
			if idx == -1 {
				break
			}
			// Find the end of the path (next space or end of string)
			end := idx
			for end < len(result) && result[end] != ' ' && result[end] != '\n' && result[end] != '\t' && result[end] != '"' && result[end] != '\'' {
				end++
			}
			fullPath := result[idx:end]
			// Find last significant directory component
			parts := strings.Split(fullPath, "/")
			if len(parts) > 3 {
				// Keep last 3 path components
				shortened := "./" + strings.Join(parts[len(parts)-3:], "/")
				result = result[:idx] + shortened + result[end:]
			} else {
				break
			}
		}
	}
	return result
}

// collapseRepeated removes duplicate consecutive lines.
func collapseRepeated(s string) string {
	lines := strings.Split(s, "\n")
	if len(lines) <= 1 {
		return s
	}

	var result []string
	var prev string
	repeatCount := 0

	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if trimmed == prev && trimmed != "" {
			repeatCount++
			if repeatCount == 1 {
				// First repeat: keep it but note it
				result = append(result, line)
			}
			// Skip additional repeats
			continue
		}
		if repeatCount > 1 {
			result = append(result, fmt.Sprintf("  ... (%d more identical lines)", repeatCount-1))
		}
		prev = trimmed
		repeatCount = 0
		result = append(result, line)
	}

	if repeatCount > 1 {
		result = append(result, fmt.Sprintf("  ... (%d more identical lines)", repeatCount-1))
	}

	return strings.Join(result, "\n")
}

// stripPleasantries removes common AI pleasantry phrases from output.
func stripPleasantries(s string) string {
	pleasantries := []string{
		"Here's ",
		"Here is ",
		"Sure! ",
		"Sure, ",
		"Certainly! ",
		"Certainly, ",
		"Of course! ",
		"Of course, ",
		"Absolutely! ",
		"Absolutely, ",
		"Great question! ",
		"That's a great question! ",
		"I'd be happy to help! ",
		"I'd be happy to help. ",
		"Let me help you with that. ",
		"Let me help you with that! ",
	}

	result := s
	// Only strip pleasantries at the beginning of the text or after newlines
	for _, p := range pleasantries {
		if strings.HasPrefix(result, p) {
			result = result[len(p):]
		}
		result = strings.ReplaceAll(result, "\n"+p, "\n")
	}
	return result
}

// --- Helpers ---

// epEstimateTokens provides a rough token count estimate (approximately 4 characters per token).
func epEstimateTokens(s string) int {
	if len(s) == 0 {
		return 0
	}
	// Rough approximation: ~4 chars per token for English text
	tokens := len(s) / 4
	if tokens == 0 && len(s) > 0 {
		tokens = 1
	}
	return tokens
}

// epFormatNumber formats an integer with comma separators for readability.
func epFormatNumber(n int) string {
	if n < 0 {
		return "-" + epFormatNumber(-n)
	}
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
		result.WriteString(s[i : i+3])
		if i+3 < len(s) {
			result.WriteString(",")
		}
	}
	return result.String()
}
