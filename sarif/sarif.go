// Package sarif emits SARIF 2.1.0 JSON for static-analysis-style tools.
//
// It is intentionally small: a single Builder type that accumulates a tool
// description, rules, and results, then serialises to canonical SARIF 2.1.0.
// Consumers are responsible for mapping their domain Finding types into the
// Rule / Result shape exposed here.
//
// Spec: https://docs.oasis-open.org/sarif/sarif/v2.1.0/sarif-v2.1.0.html
package sarif

import (
	_ "embed"
	"encoding/json"
	"strings"
)

//go:embed VERSION
var versionFile string

// Version of this sarif package. Sourced from the VERSION file at the repo
// root (single source of truth, see hawk VERSIONING.md).
var Version = strings.TrimSpace(versionFile)

const (
	schemaURL   = "https://raw.githubusercontent.com/oasis-tcs/sarif-spec/main/sarif-2.1/schema/sarif-schema-2.1.0.json"
	specVersion = "2.1.0"
)

// ---------------------------------------------------------------------------
// Public API.
// ---------------------------------------------------------------------------

// Severity is the normalised severity model exposed by this package. It maps
// onto SARIF's `level` field via the level() method.
type Severity int

const (
	// SeverityNone is "none" — informational, never failing.
	SeverityNone Severity = iota
	// SeverityNote is "note" — low-severity advisory.
	SeverityNote
	// SeverityWarning is "warning" — medium-severity issue.
	SeverityWarning
	// SeverityError is "error" — high or critical severity issue.
	SeverityError
)

func (s Severity) level() string {
	switch s {
	case SeverityError:
		return "error"
	case SeverityWarning:
		return "warning"
	case SeverityNote:
		return "note"
	default:
		return "none"
	}
}

// Tool describes the analysing tool itself (the SARIF "driver").
type Tool struct {
	Name           string
	Version        string
	InformationURI string
}

// Rule defines a check that can produce results. IDs must be unique within a
// run; the Builder dedups by ID so callers can re-add the same rule freely.
type Rule struct {
	ID               string
	Name             string
	ShortDescription string
	FullDescription  string
	HelpURI          string
	Severity         Severity
	Tags             []string
}

// Region describes the file region a Result references. All fields are
// optional; zero values are omitted from output.
type Region struct {
	StartLine   int
	EndLine     int
	StartColumn int
	EndColumn   int
}

// TaxaRef references an external taxonomy entry (e.g. CWE-89).
type TaxaRef struct {
	ID        string // taxonomy entry ID, e.g. "CWE-89"
	Component string // taxonomy name, e.g. "CWE"
}

// Result is a single finding against a Rule.
type Result struct {
	RuleID   string
	Severity Severity
	Message  string
	URI      string  // artifact location (file path or URL)
	Region   *Region // optional file region
	Fix      string  // optional fix description (text only — no patch)
	Taxa     []TaxaRef
}

// ---------------------------------------------------------------------------
// Builder.
// ---------------------------------------------------------------------------

// Builder accumulates rules and results for a single SARIF run.
//
// Builders are not safe for concurrent use; build the run on one goroutine
// then publish the JSON. Re-adding the same Rule by ID is a no-op.
type Builder struct {
	tool    Tool
	rules   []Rule
	ruleIdx map[string]int
	results []Result
}

// New starts a new SARIF run for the given tool.
func New(tool Tool) *Builder {
	return &Builder{
		tool:    tool,
		ruleIdx: make(map[string]int),
	}
}

// AddRule registers a rule. Calls with a duplicate Rule.ID are no-ops, so it's
// safe to call this from a per-result loop.
func (b *Builder) AddRule(r Rule) *Builder {
	if _, exists := b.ruleIdx[r.ID]; exists {
		return b
	}
	b.ruleIdx[r.ID] = len(b.rules)
	b.rules = append(b.rules, r)
	return b
}

// AddResult appends a result to the run. The RuleID should refer to a rule
// added via AddRule; if it doesn't, the result is still emitted but tools
// may flag the SARIF as malformed.
func (b *Builder) AddResult(r Result) *Builder {
	b.results = append(b.results, r)
	return b
}

// JSON serialises the run to canonical SARIF 2.1.0 JSON (indented).
func (b *Builder) JSON() ([]byte, error) {
	return json.MarshalIndent(b.buildLog(), "", "  ")
}

// String is JSON() with errors swallowed. Returns "{}" on error so the result
// is always valid JSON for callers that have nowhere to surface an error.
func (b *Builder) String() string {
	out, err := b.JSON()
	if err != nil {
		return "{}"
	}
	return string(out)
}

// ---------------------------------------------------------------------------
// Internal SARIF wire types — unexported to keep the public API minimal.
// ---------------------------------------------------------------------------

type sarifLog struct {
	Schema  string     `json:"$schema"`
	Version string     `json:"version"`
	Runs    []sarifRun `json:"runs"`
}

type sarifRun struct {
	Tool    sarifTool     `json:"tool"`
	Results []sarifResult `json:"results"`
}

type sarifTool struct {
	Driver sarifDriver `json:"driver"`
}

type sarifDriver struct {
	Name            string      `json:"name"`
	Version         string      `json:"version"`
	SemanticVersion string      `json:"semanticVersion,omitempty"`
	InformationURI  string      `json:"informationUri,omitempty"`
	Rules           []sarifRule `json:"rules,omitempty"`
}

type sarifRule struct {
	ID               string            `json:"id"`
	Name             string            `json:"name,omitempty"`
	ShortDescription *sarifMultiformat `json:"shortDescription,omitempty"`
	FullDescription  *sarifMultiformat `json:"fullDescription,omitempty"`
	HelpURI          string            `json:"helpUri,omitempty"`
	DefaultConfig    *sarifRuleConfig  `json:"defaultConfiguration,omitempty"`
	Properties       *sarifProps       `json:"properties,omitempty"`
}

type sarifRuleConfig struct {
	Level string `json:"level"`
}

type sarifMultiformat struct {
	Text string `json:"text"`
}

type sarifProps struct {
	Tags []string `json:"tags,omitempty"`
}

type sarifResult struct {
	RuleID    string           `json:"ruleId"`
	RuleIndex int              `json:"ruleIndex,omitempty"`
	Level     string           `json:"level"`
	Message   sarifMultiformat `json:"message"`
	Locations []sarifLocation  `json:"locations,omitempty"`
	Fixes     []sarifFix       `json:"fixes,omitempty"`
	Taxa      []sarifTaxaRef   `json:"taxa,omitempty"`
}

type sarifLocation struct {
	PhysicalLocation sarifPhysical `json:"physicalLocation"`
}

type sarifPhysical struct {
	ArtifactLocation sarifArtifact `json:"artifactLocation"`
	Region           *sarifRegion  `json:"region,omitempty"`
}

type sarifArtifact struct {
	URI string `json:"uri"`
}

type sarifRegion struct {
	StartLine   int `json:"startLine,omitempty"`
	EndLine     int `json:"endLine,omitempty"`
	StartColumn int `json:"startColumn,omitempty"`
	EndColumn   int `json:"endColumn,omitempty"`
}

type sarifFix struct {
	Description sarifMultiformat `json:"description"`
}

type sarifTaxaRef struct {
	ID            string           `json:"id"`
	ToolComponent sarifMultiformat `json:"toolComponent"`
}

// ---------------------------------------------------------------------------
// Conversion: public -> wire.
// ---------------------------------------------------------------------------

func (b *Builder) buildLog() sarifLog {
	rules := make([]sarifRule, 0, len(b.rules))
	for _, r := range b.rules {
		rules = append(rules, toSARIFRule(r))
	}

	results := make([]sarifResult, 0, len(b.results))
	for _, r := range b.results {
		results = append(results, b.toSARIFResult(r))
	}

	return sarifLog{
		Schema:  schemaURL,
		Version: specVersion,
		Runs: []sarifRun{
			{
				Tool: sarifTool{
					Driver: sarifDriver{
						Name:            b.tool.Name,
						Version:         b.tool.Version,
						SemanticVersion: b.tool.Version,
						InformationURI:  b.tool.InformationURI,
						Rules:           rules,
					},
				},
				Results: results,
			},
		},
	}
}

func toSARIFRule(r Rule) sarifRule {
	out := sarifRule{
		ID:   r.ID,
		Name: r.Name,
	}
	if r.ShortDescription != "" {
		out.ShortDescription = &sarifMultiformat{Text: r.ShortDescription}
	}
	if r.FullDescription != "" {
		out.FullDescription = &sarifMultiformat{Text: r.FullDescription}
	}
	if r.HelpURI != "" {
		out.HelpURI = r.HelpURI
	}
	out.DefaultConfig = &sarifRuleConfig{Level: r.Severity.level()}
	if len(r.Tags) > 0 {
		out.Properties = &sarifProps{Tags: r.Tags}
	}
	return out
}

func (b *Builder) toSARIFResult(r Result) sarifResult {
	out := sarifResult{
		RuleID:  r.RuleID,
		Level:   r.Severity.level(),
		Message: sarifMultiformat{Text: r.Message},
	}
	if idx, ok := b.ruleIdx[r.RuleID]; ok {
		out.RuleIndex = idx
	}
	if r.URI != "" {
		loc := sarifLocation{
			PhysicalLocation: sarifPhysical{
				ArtifactLocation: sarifArtifact{URI: r.URI},
			},
		}
		if r.Region != nil {
			reg := &sarifRegion{
				StartLine:   r.Region.StartLine,
				EndLine:     r.Region.EndLine,
				StartColumn: r.Region.StartColumn,
				EndColumn:   r.Region.EndColumn,
			}
			// SARIF: EndLine defaults to StartLine if absent. Make it explicit
			// so consumers that don't apply that default still highlight the
			// right line.
			if reg.EndLine == 0 && reg.StartLine > 0 {
				reg.EndLine = reg.StartLine
			}
			loc.PhysicalLocation.Region = reg
		}
		out.Locations = []sarifLocation{loc}
	}
	if r.Fix != "" {
		out.Fixes = []sarifFix{
			{Description: sarifMultiformat{Text: r.Fix}},
		}
	}
	for _, t := range r.Taxa {
		out.Taxa = append(out.Taxa, sarifTaxaRef{
			ID:            t.ID,
			ToolComponent: sarifMultiformat{Text: t.Component},
		})
	}
	return out
}
