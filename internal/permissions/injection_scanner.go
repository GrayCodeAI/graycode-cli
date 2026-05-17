package permissions

import (
	"fmt"
	"math"
	"regexp"
	"strings"
	"sync"
	"unicode"
)

// InjectionPattern defines a single pattern used to detect prompt injection attempts.
type InjectionPattern struct {
	Name     string
	Pattern  *regexp.Regexp
	Severity string // "critical", "high", "medium", "low"
	Category string // "system_override", "data_exfil", "role_hijack", "instruction_leak"
}

// Threat represents a detected injection threat with context about the match.
type Threat struct {
	Pattern  string
	Category string
	Severity string
	Match    string
	Position int
}

// ScanResult contains the outcome of scanning text for injection attempts.
type ScanResult struct {
	IsSafe         bool
	Threats        []Threat
	Score          float64
	Recommendation string
}

// InjectionScanner detects malicious prompt injection attempts in user input and tool outputs.
type InjectionScanner struct {
	Patterns  []*InjectionPattern
	Threshold float64
	mu        sync.RWMutex
}

// NewInjectionScanner creates an InjectionScanner pre-loaded with 30+ detection patterns.
func NewInjectionScanner() *InjectionScanner {
	scanner := &InjectionScanner{
		Threshold: 0.7,
	}

	// System override patterns
	scanner.Patterns = append(scanner.Patterns, &InjectionPattern{
		Name:     "ignore_previous_instructions",
		Pattern:  regexp.MustCompile(`(?i)ignore\s+(all\s+)?previous\s+instructions`),
		Severity: "critical",
		Category: "system_override",
	})
	scanner.Patterns = append(scanner.Patterns, &InjectionPattern{
		Name:     "you_are_now",
		Pattern:  regexp.MustCompile(`(?i)you\s+are\s+now\s+`),
		Severity: "critical",
		Category: "system_override",
	})
	scanner.Patterns = append(scanner.Patterns, &InjectionPattern{
		Name:     "forget_everything",
		Pattern:  regexp.MustCompile(`(?i)forget\s+(all|everything)\s+(you|about|that)`),
		Severity: "critical",
		Category: "system_override",
	})
	scanner.Patterns = append(scanner.Patterns, &InjectionPattern{
		Name:     "new_instructions",
		Pattern:  regexp.MustCompile(`(?i)new\s+instructions\s*:`),
		Severity: "critical",
		Category: "system_override",
	})
	scanner.Patterns = append(scanner.Patterns, &InjectionPattern{
		Name:     "system_prefix",
		Pattern:  regexp.MustCompile(`(?i)^\s*system\s*:`),
		Severity: "high",
		Category: "system_override",
	})
	scanner.Patterns = append(scanner.Patterns, &InjectionPattern{
		Name:     "triple_hash_system",
		Pattern:  regexp.MustCompile(`###\s*(system|instruction|new\s+role)`),
		Severity: "high",
		Category: "system_override",
	})
	scanner.Patterns = append(scanner.Patterns, &InjectionPattern{
		Name:     "disregard_above",
		Pattern:  regexp.MustCompile(`(?i)disregard\s+(the\s+)?(above|previous|prior)`),
		Severity: "critical",
		Category: "system_override",
	})
	scanner.Patterns = append(scanner.Patterns, &InjectionPattern{
		Name:     "override_instructions",
		Pattern:  regexp.MustCompile(`(?i)override\s+(your|the|all)\s+instructions`),
		Severity: "critical",
		Category: "system_override",
	})
	scanner.Patterns = append(scanner.Patterns, &InjectionPattern{
		Name:     "do_not_follow",
		Pattern:  regexp.MustCompile(`(?i)do\s+not\s+follow\s+(your|the|any)\s+(previous|original)`),
		Severity: "critical",
		Category: "system_override",
	})
	scanner.Patterns = append(scanner.Patterns, &InjectionPattern{
		Name:     "begin_new_conversation",
		Pattern:  regexp.MustCompile(`(?i)(begin|start)\s+(a\s+)?new\s+(conversation|session|context)`),
		Severity: "high",
		Category: "system_override",
	})

	// Role hijacking patterns
	scanner.Patterns = append(scanner.Patterns, &InjectionPattern{
		Name:     "pretend_you_are",
		Pattern:  regexp.MustCompile(`(?i)pretend\s+(you\s+are|to\s+be|you'?re)`),
		Severity: "high",
		Category: "role_hijack",
	})
	scanner.Patterns = append(scanner.Patterns, &InjectionPattern{
		Name:     "act_as",
		Pattern:  regexp.MustCompile(`(?i)act\s+as\s+(a|an|if|though)`),
		Severity: "high",
		Category: "role_hijack",
	})
	scanner.Patterns = append(scanner.Patterns, &InjectionPattern{
		Name:     "you_are_a",
		Pattern:  regexp.MustCompile(`(?i)from\s+now\s+on\s+you\s+are\s+a`),
		Severity: "high",
		Category: "role_hijack",
	})
	scanner.Patterns = append(scanner.Patterns, &InjectionPattern{
		Name:     "roleplay_as",
		Pattern:  regexp.MustCompile(`(?i)roleplay\s+as\s+`),
		Severity: "high",
		Category: "role_hijack",
	})
	scanner.Patterns = append(scanner.Patterns, &InjectionPattern{
		Name:     "jailbreak",
		Pattern:  regexp.MustCompile(`(?i)(jailbreak|DAN|do\s+anything\s+now)`),
		Severity: "critical",
		Category: "role_hijack",
	})
	scanner.Patterns = append(scanner.Patterns, &InjectionPattern{
		Name:     "developer_mode",
		Pattern:  regexp.MustCompile(`(?i)(developer|maintenance|debug)\s+mode\s+(enabled|activated|on)`),
		Severity: "high",
		Category: "role_hijack",
	})
	scanner.Patterns = append(scanner.Patterns, &InjectionPattern{
		Name:     "simulate_persona",
		Pattern:  regexp.MustCompile(`(?i)simulate\s+(being|a|an)\s+`),
		Severity: "medium",
		Category: "role_hijack",
	})

	// Data exfiltration patterns
	scanner.Patterns = append(scanner.Patterns, &InjectionPattern{
		Name:     "output_all",
		Pattern:  regexp.MustCompile(`(?i)output\s+(all|every|the\s+entire)`),
		Severity: "high",
		Category: "data_exfil",
	})
	scanner.Patterns = append(scanner.Patterns, &InjectionPattern{
		Name:     "reveal_prompt",
		Pattern:  regexp.MustCompile(`(?i)reveal\s+(your|the)\s+(system\s+)?prompt`),
		Severity: "critical",
		Category: "data_exfil",
	})
	scanner.Patterns = append(scanner.Patterns, &InjectionPattern{
		Name:     "show_system_message",
		Pattern:  regexp.MustCompile(`(?i)show\s+(me\s+)?(the\s+)?system\s+message`),
		Severity: "high",
		Category: "data_exfil",
	})
	scanner.Patterns = append(scanner.Patterns, &InjectionPattern{
		Name:     "repeat_everything_above",
		Pattern:  regexp.MustCompile(`(?i)repeat\s+everything\s+(above|before|prior)`),
		Severity: "high",
		Category: "data_exfil",
	})
	scanner.Patterns = append(scanner.Patterns, &InjectionPattern{
		Name:     "dump_context",
		Pattern:  regexp.MustCompile(`(?i)(dump|print|display|echo)\s+(the\s+)?(full\s+)?context`),
		Severity: "high",
		Category: "data_exfil",
	})
	scanner.Patterns = append(scanner.Patterns, &InjectionPattern{
		Name:     "send_to_url",
		Pattern:  regexp.MustCompile(`(?i)send\s+(\S+\s+)*to\s+https?://`),
		Severity: "critical",
		Category: "data_exfil",
	})

	// Instruction leak patterns
	scanner.Patterns = append(scanner.Patterns, &InjectionPattern{
		Name:     "what_are_instructions",
		Pattern:  regexp.MustCompile(`(?i)what\s+are\s+your\s+(instructions|rules|guidelines|constraints)`),
		Severity: "medium",
		Category: "instruction_leak",
	})
	scanner.Patterns = append(scanner.Patterns, &InjectionPattern{
		Name:     "print_your_rules",
		Pattern:  regexp.MustCompile(`(?i)print\s+your\s+(rules|instructions|prompt|system)`),
		Severity: "high",
		Category: "instruction_leak",
	})
	scanner.Patterns = append(scanner.Patterns, &InjectionPattern{
		Name:     "show_system_prompt",
		Pattern:  regexp.MustCompile(`(?i)show\s+(me\s+)?your\s+system\s+prompt`),
		Severity: "high",
		Category: "instruction_leak",
	})
	scanner.Patterns = append(scanner.Patterns, &InjectionPattern{
		Name:     "list_all_commands",
		Pattern:  regexp.MustCompile(`(?i)list\s+(all\s+)?(your\s+)?(available\s+)?(commands|tools|functions|capabilities)`),
		Severity: "low",
		Category: "instruction_leak",
	})
	scanner.Patterns = append(scanner.Patterns, &InjectionPattern{
		Name:     "tell_me_verbatim",
		Pattern:  regexp.MustCompile(`(?i)tell\s+me\s+verbatim\s+`),
		Severity: "high",
		Category: "instruction_leak",
	})

	// Delimiter injection patterns
	scanner.Patterns = append(scanner.Patterns, &InjectionPattern{
		Name:     "code_block_system",
		Pattern:  regexp.MustCompile("(?i)```\\s*system"),
		Severity: "high",
		Category: "system_override",
	})
	scanner.Patterns = append(scanner.Patterns, &InjectionPattern{
		Name:     "inst_tag",
		Pattern:  regexp.MustCompile(`(?i)\[INST\]`),
		Severity: "high",
		Category: "system_override",
	})
	scanner.Patterns = append(scanner.Patterns, &InjectionPattern{
		Name:     "im_start_system",
		Pattern:  regexp.MustCompile(`(?i)<\|im_start\|>\s*system`),
		Severity: "critical",
		Category: "system_override",
	})
	scanner.Patterns = append(scanner.Patterns, &InjectionPattern{
		Name:     "xml_system_tag",
		Pattern:  regexp.MustCompile(`(?i)<system>|</system>`),
		Severity: "high",
		Category: "system_override",
	})
	scanner.Patterns = append(scanner.Patterns, &InjectionPattern{
		Name:     "end_turn_token",
		Pattern:  regexp.MustCompile(`(?i)<\|endoftext\|>|<\|end_turn\|>`),
		Severity: "critical",
		Category: "system_override",
	})

	// Encoding attack patterns
	scanner.Patterns = append(scanner.Patterns, &InjectionPattern{
		Name:     "base64_ignore",
		Pattern:  regexp.MustCompile(`(?i)(aWdub3JlI|Zm9yZ2V0I|c3lzdGVt)`), // base64 fragments of "ignore", "forget", "system"
		Severity: "medium",
		Category: "system_override",
	})
	scanner.Patterns = append(scanner.Patterns, &InjectionPattern{
		Name:     "hex_encoded_payload",
		Pattern:  regexp.MustCompile(`(?i)(\\x[0-9a-f]{2}){4,}`),
		Severity: "medium",
		Category: "system_override",
	})

	return scanner
}

// Scan analyzes text for injection attempts and returns a structured result.
func (s *InjectionScanner) Scan(text string) *ScanResult {
	s.mu.RLock()
	defer s.mu.RUnlock()

	result := &ScanResult{
		IsSafe:  true,
		Threats: []Threat{},
		Score:   0.0,
	}

	if text == "" {
		result.Recommendation = "Input is empty, no threats detected."
		return result
	}

	// Run all pattern checks
	for _, p := range s.Patterns {
		loc := p.Pattern.FindStringIndex(text)
		if loc != nil {
			match := text[loc[0]:loc[1]]
			threat := Threat{
				Pattern:  p.Name,
				Category: p.Category,
				Severity: p.Severity,
				Match:    match,
				Position: loc[0],
			}
			result.Threats = append(result.Threats, threat)
		}
	}

	// Check for unicode attacks
	unicodeThreats := s.DetectUnicodeAttacks(text)
	result.Threats = append(result.Threats, unicodeThreats...)

	// Check high entropy sections (possible encoded payloads)
	if s.IsHighEntropy(text) {
		result.Threats = append(result.Threats, Threat{
			Pattern:  "high_entropy_content",
			Category: "system_override",
			Severity: "medium",
			Match:    "(high entropy detected)",
			Position: 0,
		})
	}

	// Calculate threat score
	result.Score = s.calculateScore(result.Threats)

	// Determine if safe: either score exceeds threshold OR any critical/high threat found
	if result.Score >= s.Threshold {
		result.IsSafe = false
	}
	for _, t := range result.Threats {
		if t.Severity == "critical" || t.Severity == "high" {
			result.IsSafe = false
			break
		}
	}

	// Set recommendation
	result.Recommendation = s.generateRecommendation(result)

	return result
}

// ScanToolOutput scans tool output for injection attempts that might be embedded
// in data returned by external tools (poisoned responses).
func (s *InjectionScanner) ScanToolOutput(output string) *ScanResult {
	result := s.Scan(output)

	// Additional checks for tool output: look for hidden instructions
	// in structured data that tools might return
	hiddenPatterns := []*InjectionPattern{
		{
			Name:     "hidden_instruction_in_json",
			Pattern:  regexp.MustCompile(`(?i)"(instruction|command|system|role)"\s*:\s*"[^"]*ignore`),
			Severity: "critical",
			Category: "system_override",
		},
		{
			Name:     "comment_injection",
			Pattern:  regexp.MustCompile(`(?i)(//|/\*|#|<!--)\s*(ignore|forget|override|new\s+instructions)`),
			Severity: "high",
			Category: "system_override",
		},
		{
			Name:     "multiline_hidden_payload",
			Pattern:  regexp.MustCompile(`(?i)\n\s*\n.*ignore\s+previous`),
			Severity: "critical",
			Category: "system_override",
		},
	}

	for _, p := range hiddenPatterns {
		loc := p.Pattern.FindStringIndex(output)
		if loc != nil {
			match := output[loc[0]:loc[1]]
			threat := Threat{
				Pattern:  p.Name,
				Category: p.Category,
				Severity: p.Severity,
				Match:    match,
				Position: loc[0],
			}
			result.Threats = append(result.Threats, threat)
		}
	}

	// Recalculate score with additional threats
	result.Score = s.calculateScore(result.Threats)
	if result.Score >= s.Threshold {
		result.IsSafe = false
	}
	for _, t := range result.Threats {
		if t.Severity == "critical" || t.Severity == "high" {
			result.IsSafe = false
			break
		}
	}
	result.Recommendation = s.generateRecommendation(result)

	return result
}

// IsHighEntropy detects potential encoded payloads by calculating Shannon entropy.
// Text with entropy above 4.5 bits per character is considered suspicious.
func (s *InjectionScanner) IsHighEntropy(text string) bool {
	if len(text) < 20 {
		return false
	}

	// Calculate Shannon entropy
	freq := make(map[rune]float64)
	total := 0.0
	for _, r := range text {
		freq[r]++
		total++
	}

	entropy := 0.0
	for _, count := range freq {
		p := count / total
		if p > 0 {
			entropy -= p * math.Log2(p)
		}
	}

	// Threshold: normal English text is ~3.5-4.0 bits, encoded/obfuscated content is typically >4.5
	return entropy > 4.5
}

// DetectUnicodeAttacks identifies homoglyphs, zero-width characters,
// bidirectional overrides, and invisible separators.
func (s *InjectionScanner) DetectUnicodeAttacks(text string) []Threat {
	var threats []Threat

	for i, r := range text {
		// Zero-width characters: U+200B, U+200C, U+200D, U+FEFF
		if r == 0x200B || r == 0x200C || r == 0x200D || r == 0xFEFF {
			threats = append(threats, Threat{
				Pattern:  "zero_width_character",
				Category: "system_override",
				Severity: "high",
				Match:    fmt.Sprintf("U+%04X", r),
				Position: i,
			})
		}

		// Bidirectional override characters
		if r == 0x202A || r == 0x202B || r == 0x202C || r == 0x202D || r == 0x202E ||
			r == 0x2066 || r == 0x2067 || r == 0x2068 || r == 0x2069 {
			threats = append(threats, Threat{
				Pattern:  "bidi_override",
				Category: "system_override",
				Severity: "critical",
				Match:    fmt.Sprintf("U+%04X", r),
				Position: i,
			})
		}

		// Invisible separators and format characters
		if r == 0x2060 || r == 0x2061 || r == 0x2062 || r == 0x2063 || r == 0x2064 {
			threats = append(threats, Threat{
				Pattern:  "invisible_separator",
				Category: "system_override",
				Severity: "medium",
				Match:    fmt.Sprintf("U+%04X", r),
				Position: i,
			})
		}

		// Homoglyph detection: Cyrillic/Greek characters that look like Latin
		if unicode.Is(unicode.Cyrillic, r) || unicode.Is(unicode.Greek, r) {
			// Check if it's mixed with Latin text (homoglyph attack)
			if containsLatin(text) {
				threats = append(threats, Threat{
					Pattern:  "homoglyph_mixed_script",
					Category: "system_override",
					Severity: "high",
					Match:    fmt.Sprintf("U+%04X (%c)", r, r),
					Position: i,
				})
				// Only report once per scan to avoid noise
				break
			}
		}

		// Tag characters (U+E0000-U+E007F) used for invisible text
		if r >= 0xE0000 && r <= 0xE007F {
			threats = append(threats, Threat{
				Pattern:  "tag_character",
				Category: "system_override",
				Severity: "critical",
				Match:    fmt.Sprintf("U+%05X", r),
				Position: i,
			})
		}
	}

	return threats
}

// FormatResult produces a human-readable string representation of a ScanResult.
func FormatResult(result *ScanResult) string {
	if result == nil {
		return "No scan result available."
	}

	var sb strings.Builder

	if result.IsSafe {
		sb.WriteString("SAFE: No significant injection threats detected.\n")
	} else {
		sb.WriteString("UNSAFE: Injection attempt detected!\n")
	}

	sb.WriteString(fmt.Sprintf("Threat Score: %.2f/1.00\n", result.Score))
	sb.WriteString(fmt.Sprintf("Recommendation: %s\n", result.Recommendation))

	if len(result.Threats) > 0 {
		sb.WriteString(fmt.Sprintf("\nThreats Detected (%d):\n", len(result.Threats)))
		for i, t := range result.Threats {
			sb.WriteString(fmt.Sprintf("  %d. [%s] %s (category: %s, position: %d)\n",
				i+1, strings.ToUpper(t.Severity), t.Pattern, t.Category, t.Position))
			sb.WriteString(fmt.Sprintf("     Match: %q\n", t.Match))
		}
	}

	return sb.String()
}

// calculateScore computes a threat score from 0.0 to 1.0 based on detected threats.
func (s *InjectionScanner) calculateScore(threats []Threat) float64 {
	if len(threats) == 0 {
		return 0.0
	}

	score := 0.0
	for _, t := range threats {
		switch t.Severity {
		case "critical":
			score += 0.4
		case "high":
			score += 0.25
		case "medium":
			score += 0.15
		case "low":
			score += 0.05
		}
	}

	// Cap at 1.0
	if score > 1.0 {
		score = 1.0
	}

	return score
}

// generateRecommendation returns actionable advice based on the scan result.
func (s *InjectionScanner) generateRecommendation(result *ScanResult) string {
	if len(result.Threats) == 0 {
		return "Input appears safe. No action required."
	}

	// Find the highest severity
	highestSeverity := "low"
	severityRank := map[string]int{"low": 0, "medium": 1, "high": 2, "critical": 3}

	categories := make(map[string]bool)
	for _, t := range result.Threats {
		categories[t.Category] = true
		if severityRank[t.Severity] > severityRank[highestSeverity] {
			highestSeverity = t.Severity
		}
	}

	switch highestSeverity {
	case "critical":
		return "BLOCK: Input contains critical injection patterns. Do not process this input."
	case "high":
		return "BLOCK: Input contains high-severity injection attempts. Reject and log this input."
	case "medium":
		if result.Score >= s.Threshold {
			return "REVIEW: Multiple medium-severity patterns detected. Manual review recommended before processing."
		}
		return "CAUTION: Suspicious patterns detected but below threshold. Monitor for repeated attempts."
	default:
		return "MONITOR: Low-severity patterns detected. Log for analysis but processing may continue."
	}
}

// containsLatin checks if a string contains any Latin script characters.
func containsLatin(text string) bool {
	for _, r := range text {
		if unicode.Is(unicode.Latin, r) {
			return true
		}
	}
	return false
}
