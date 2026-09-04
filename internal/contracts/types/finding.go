// Vendored from github.com/GrayCodeAI/eagle/types at v0.0.0-20260902153929-5877bed17503 (MIT, Copyright (c) 2026 GrayCode AI).
// The upstream repository no longer exists; this copy is owned by Graycode as its contract surface.
package types

import (
	"fmt"
	"strings"
	"time"
)

// Finding represents a unified analysis concern sourced from Graycode support engines.
type Finding struct {
	ID         string            `json:"id"`
	Source     string            `json:"source"`
	Concern    string            `json:"concern"`
	Severity   Severity          `json:"severity"`
	File       string            `json:"file,omitempty"`
	URL        string            `json:"url,omitempty"`
	Line       int               `json:"line,omitempty"`
	EndLine    int               `json:"end_line,omitempty"`
	Message    string            `json:"message"`
	CWE        string            `json:"cwe,omitempty"`
	Confidence float64           `json:"confidence"`
	Fix        string            `json:"fix,omitempty"`
	Reasoning  string            `json:"reasoning,omitempty"`
	Tags       []string          `json:"tags,omitempty"`
	Metadata   map[string]string `json:"metadata,omitempty"`
	CreatedAt  time.Time         `json:"created_at"`
}

// Validate reports whether the finding satisfies the minimum contract
// invariants: a non-blank Message, a non-negative Line, and a Confidence
// within [0, 1]. It returns a descriptive error naming the first violated
// field.
func (f Finding) Validate() error {
	if strings.TrimSpace(f.Message) == "" {
		return fmt.Errorf("finding message is empty")
	}
	if f.Line < 0 {
		return fmt.Errorf("finding line %d is negative", f.Line)
	}
	if f.Confidence < 0 || f.Confidence > 1 {
		return fmt.Errorf("finding confidence %v is outside [0, 1]", f.Confidence)
	}
	return nil
}

// FindingSlice is sortable by severity descending and confidence descending.
type FindingSlice []Finding

func (s FindingSlice) Len() int { return len(s) }

func (s FindingSlice) Less(i, j int) bool {
	if s[i].Severity != s[j].Severity {
		return s[i].Severity > s[j].Severity
	}
	return s[i].Confidence > s[j].Confidence
}

func (s FindingSlice) Swap(i, j int) { s[i], s[j] = s[j], s[i] }

// FilterBySource returns findings whose Source matches the given value.
func (s FindingSlice) FilterBySource(source string) FindingSlice {
	out := make(FindingSlice, 0, len(s))
	for _, f := range s {
		if f.Source == source {
			out = append(out, f)
		}
	}
	return out
}

// FilterBySeverity returns findings whose Severity is at least min.
func (s FindingSlice) FilterBySeverity(min Severity) FindingSlice {
	out := make(FindingSlice, 0, len(s))
	for _, f := range s {
		if f.Severity.AtLeast(min) {
			out = append(out, f)
		}
	}
	return out
}

// FilterByConfidence returns findings whose Confidence is >= min.
func (s FindingSlice) FilterByConfidence(min float64) FindingSlice {
	out := make(FindingSlice, 0, len(s))
	for _, f := range s {
		if f.Confidence >= min {
			out = append(out, f)
		}
	}
	return out
}

// ByFile groups findings by their File field.
func (s FindingSlice) ByFile() map[string]FindingSlice {
	m := make(map[string]FindingSlice, len(s))
	for _, f := range s {
		m[f.File] = append(m[f.File], f)
	}
	return m
}

// FindingSummary provides aggregate counts over a set of findings.
type FindingSummary struct {
	Total         int            `json:"total"`
	BySource      map[string]int `json:"by_source"`
	BySeverity    map[string]int `json:"by_severity"`
	AvgConfidence float64        `json:"avg_confidence"`
}

// Summary returns a FindingSummary for the slice.
func (s FindingSlice) Summary() FindingSummary {
	bySrc := make(map[string]int)
	bySev := make(map[string]int)
	var confSum float64

	for _, f := range s {
		bySrc[f.Source]++
		bySev[f.Severity.String()]++
		confSum += f.Confidence
	}

	avg := 0.0
	if len(s) > 0 {
		avg = confSum / float64(len(s))
	}

	return FindingSummary{
		Total:         len(s),
		BySource:      bySrc,
		BySeverity:    bySev,
		AvgConfidence: avg,
	}
}

// FindingFromKestrel constructs a Finding from a kestrel review result.
func FindingFromKestrel(
	concern, file string,
	line int,
	message, cwe string,
	sev Severity,
	confidence float64,
) Finding {
	return Finding{
		ID:         fmt.Sprintf("kestrel:%s:%s:%d", concern, file, line),
		Source:     "kestrel",
		Concern:    concern,
		Severity:   sev,
		File:       file,
		Line:       line,
		Message:    message,
		CWE:        cwe,
		Confidence: confidence,
		CreatedAt:  time.Now(),
	}
}

// FindingFromMerlin constructs a Finding from an merlin analysis result.
func FindingFromMerlin(
	concern, url, message string,
	sev Severity,
	tags []string,
) Finding {
	return Finding{
		ID:        fmt.Sprintf("merlin:%s:%s", concern, url),
		Source:    "merlin",
		Concern:   concern,
		Severity:  sev,
		URL:       url,
		Message:   message,
		Tags:      tags,
		CreatedAt: time.Now(),
	}
}
