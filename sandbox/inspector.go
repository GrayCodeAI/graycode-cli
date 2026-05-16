// Package sandbox provides adversary detection and egress inspection.
// Detects prompt injection, data exfiltration, and suspicious tool outputs.
package sandbox

import (
	"regexp"
	"strings"
)

// ThreatLevel classifies the severity of a detected threat.
type ThreatLevel int

const (
	ThreatNone ThreatLevel = iota
	ThreatLow
	ThreatMedium
	ThreatHigh
	ThreatCritical
)

func (t ThreatLevel) String() string {
	switch t {
	case ThreatLow:
		return "LOW"
	case ThreatMedium:
		return "MEDIUM"
	case ThreatHigh:
		return "HIGH"
	case ThreatCritical:
		return "CRITICAL"
	default:
		return "NONE"
	}
}

// Finding represents a detected security issue.
type Finding struct {
	Type    string
	Level   ThreatLevel
	Message string
	Content string // the offending content snippet
}

// InspectionResult holds all findings from an inspection.
type InspectionResult struct {
	Safe     bool
	Findings []Finding
}

// AdversaryInspector detects prompt injection attempts in tool outputs.
type AdversaryInspector struct{}

var injectionPatterns = []*regexp.Regexp{
	regexp.MustCompile(`(?i)ignore\s+(all\s+)?previous\s+instructions`),
	regexp.MustCompile(`(?i)you\s+are\s+now\s+(a|an)\s+`),
	regexp.MustCompile(`(?i)system\s*:\s*you\s+are`),
	regexp.MustCompile(`(?i)forget\s+(everything|all|your)\s+(above|previous)`),
	regexp.MustCompile(`(?i)new\s+instructions?\s*:`),
	regexp.MustCompile(`(?i)disregard\s+(all|any|the)(\s+\w+)*\s+(above|previous|prior|instructions)`),
	regexp.MustCompile(`(?i)\[SYSTEM\]|\[INST\]|\<\|im_start\|>`),
}

// Inspect checks content for prompt injection patterns.
func (ai *AdversaryInspector) Inspect(content string) *InspectionResult {
	result := &InspectionResult{Safe: true}
	for _, pat := range injectionPatterns {
		if loc := pat.FindStringIndex(content); loc != nil {
			result.Safe = false
			snippet := content[loc[0]:min(loc[1]+20, len(content))]
			result.Findings = append(result.Findings, Finding{
				Type:    "prompt_injection",
				Level:   ThreatCritical,
				Message: "Prompt injection attempt detected",
				Content: snippet,
			})
		}
	}
	return result
}

// EgressInspector detects data exfiltration attempts in tool commands/outputs.
type EgressInspector struct {
	AllowedDomains []string
}

var egressPatterns = []*regexp.Regexp{
	regexp.MustCompile(`(?i)curl\s+(-[a-zA-Z]*\s+)*https?://[^\s]+`),
	regexp.MustCompile(`(?i)wget\s+https?://[^\s]+`),
	regexp.MustCompile(`(?i)nc\s+(-[a-z]+\s+)*[\w.-]+\s+\d+`),
	regexp.MustCompile(`(?i)ssh\s+[\w@.-]+`),
	regexp.MustCompile(`(?i)scp\s+.*[\w@.-]+:`),
}

var sensitiveDataPatterns = []*regexp.Regexp{
	regexp.MustCompile(`(?i)(api[_-]?key|secret|token|password)\s*[=:]\s*\S+`),
	regexp.MustCompile(`(?i)-----BEGIN\s+(RSA\s+)?PRIVATE\s+KEY-----`),
	regexp.MustCompile(`(?i)AKIA[0-9A-Z]{16}`), // AWS access key
}

// Inspect checks content for data exfiltration attempts.
func (ei *EgressInspector) Inspect(content string) *InspectionResult {
	result := &InspectionResult{Safe: true}

	// Check for outbound network calls
	for _, pat := range egressPatterns {
		if loc := pat.FindStringIndex(content); loc != nil {
			snippet := content[loc[0]:min(loc[1], len(content))]
			// Check if domain is allowed
			if ei.isDomainAllowed(snippet) {
				continue
			}
			result.Findings = append(result.Findings, Finding{
				Type:    "egress",
				Level:   ThreatHigh,
				Message: "Outbound network call to non-allowed domain",
				Content: snippet,
			})
		}
	}

	// Check for sensitive data in output
	for _, pat := range sensitiveDataPatterns {
		if loc := pat.FindStringIndex(content); loc != nil {
			result.Safe = false
			snippet := content[loc[0]:min(loc[0]+30, len(content))]
			result.Findings = append(result.Findings, Finding{
				Type:    "data_leak",
				Level:   ThreatCritical,
				Message: "Sensitive data detected in output",
				Content: snippet + "...",
			})
		}
	}

	if len(result.Findings) > 0 {
		result.Safe = false
	}
	return result
}

func (ei *EgressInspector) isDomainAllowed(cmd string) bool {
	for _, domain := range ei.AllowedDomains {
		if strings.Contains(cmd, domain) {
			return true
		}
	}
	return false
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
