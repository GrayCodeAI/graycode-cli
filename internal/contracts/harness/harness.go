// Vendored from github.com/GrayCodeAI/eagle/harness at v0.0.0-20260902153929-5877bed17503 (MIT, Copyright (c) 2026 GrayCode AI).
// The upstream repository no longer exists; this copy is owned by Graycode as its contract surface.
package harness

import (
	"time"

	contracts "github.com/GrayCodeAI/graycode-cli/internal/contracts/types"
)

// Dimension represents one of the 5 core Agent Work Loop dimensions.
type Dimension string

const (
	DimensionFeedforward       Dimension = "Feedforward Guidance"
	DimensionFeedback          Dimension = "Feedback Sensors"
	DimensionTaskUnderstanding Dimension = "Task Understanding"
	DimensionStepPlanning      Dimension = "Step Planning & Execution"
	DimensionVerification      Dimension = "Verification & Safeguards"
)

// EvidenceState describes whether an evidence mechanism is Present, Partial, Missing, or Unobserved.
type EvidenceState string

const (
	EvidenceStatePresent    EvidenceState = "Present"
	EvidenceStatePartial    EvidenceState = "Partial"
	EvidenceStateMissing    EvidenceState = "Missing"
	EvidenceStateUnobserved EvidenceState = "Unobserved"
)

// Finding represents a single actionable evaluation discovery in the neutral harness contract.
type Finding struct {
	ID              string             `json:"id"`
	Dimension       Dimension          `json:"dimension"`
	Severity        contracts.Severity `json:"severity"`
	Title           string             `json:"title"`
	Description     string             `json:"description"`
	Impact          string             `json:"impact"`
	EvidenceSource  string             `json:"evidence_source"`
	EvidenceState   EvidenceState      `json:"evidence_state"`
	ExpectedOutcome string             `json:"expected_outcome"`
	ScopedRepair    string             `json:"scoped_repair"`
	ValidationRoute string             `json:"validation_route"`
}

// DimensionScore holds aggregated scoring for a single Agent Work Loop dimension.
type DimensionScore struct {
	Dimension     Dimension     `json:"dimension"`
	Score         int           `json:"score"` // 0 to 100
	State         EvidenceState `json:"state"`
	Summary       string        `json:"summary"`
	FindingsCount int           `json:"findings_count"`
}

// AssetsDetected lists the project harness assets detected during evaluation.
type AssetsDetected struct {
	AgentsMD      bool     `json:"agents_md"`
	AgentsMDPath  string   `json:"agents_md_path,omitempty"`
	ZeroMD        bool     `json:"zero_md"`
	ZeroMDPath    string   `json:"zero_md_path,omitempty"`
	Skills        []string `json:"skills"`
	SpecsCount    int      `json:"specs_count"`
	Linters       []string `json:"linters"`
	TestRunners   []string `json:"test_runners"`
	Hooks         []string `json:"hooks"`
	SandboxPolicy string   `json:"sandbox_policy"`
	AutonomyTier  string   `json:"autonomy_tier"`
	MerlinBridge  bool     `json:"merlin_bridge"`
	KestrelBridge bool     `json:"kestrel_bridge"`
}

// Report is the neutral cross-repo contract for Graycode Agent Harness evaluations.
type Report struct {
	TargetPath    string                       `json:"target_path"`
	GeneratedAt   time.Time                    `json:"generated_at"`
	OverallScore  int                          `json:"overall_score"` // 0 to 100
	OverallStatus string                       `json:"overall_status"`
	Dimensions    map[Dimension]DimensionScore `json:"dimensions"`
	Findings      []Finding                    `json:"findings"`
	Assets        AssetsDetected               `json:"assets"`
	Summary       string                       `json:"summary"`
}

// MaxSeverity returns the highest severity finding in the harness report.
func (r *Report) MaxSeverity() contracts.Severity {
	if r == nil {
		return contracts.SeverityInfo
	}
	max := contracts.SeverityInfo
	for _, f := range r.Findings {
		if f.Severity > max {
			max = f.Severity
		}
	}
	return max
}
