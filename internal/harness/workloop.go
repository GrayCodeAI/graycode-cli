package harness

import (
	"time"

	harnesscontracts "github.com/GrayCodeAI/hawk-core-contracts/harness"
	typescontracts "github.com/GrayCodeAI/hawk-core-contracts/types"
)

// Dimension represents one of the five core dimensions of the Agent Work Loop.
type Dimension string

const (
	DimensionFeedforward       Dimension = "Feedforward Guidance"
	DimensionFeedback          Dimension = "Feedback Sensors"
	DimensionTaskUnderstanding Dimension = "Task Understanding"
	DimensionStepPlanning      Dimension = "Step Planning & Execution"
	DimensionVerification      Dimension = "Verification & Safeguards"
)

// EvidenceState describes whether an evidence mechanism is verified, partial, missing, or unobserved.
type EvidenceState string

const (
	EvidenceStatePresent    EvidenceState = "Present"
	EvidenceStatePartial    EvidenceState = "Partial"
	EvidenceStateMissing    EvidenceState = "Missing"
	EvidenceStateUnobserved EvidenceState = "Unobserved"
)

// Severity indicates the urgency and risk level of a finding.
type Severity string

const (
	SeverityCritical Severity = "CRITICAL"
	SeverityHigh     Severity = "HIGH"
	SeverityMedium   Severity = "MEDIUM"
	SeverityLow      Severity = "LOW"
	SeverityInfo     Severity = "INFO"
)

// Finding represents a single actionable evaluation discovery tied to concrete evidence.
type Finding struct {
	ID              string        `json:"id"`
	Dimension       Dimension     `json:"dimension"`
	Severity        Severity      `json:"severity"`
	Title           string        `json:"title"`
	Description     string        `json:"description"`
	Impact          string        `json:"impact"`
	EvidenceSource  string        `json:"evidence_source"`
	EvidenceState   EvidenceState `json:"evidence_state"`
	ExpectedOutcome string        `json:"expected_outcome"`
	ScopedRepair    string        `json:"scoped_repair"`
	ValidationRoute string        `json:"validation_route"`
}

// DimensionScore holds aggregated scoring and state for a single Agent Work Loop dimension.
type DimensionScore struct {
	Dimension   Dimension     `json:"dimension"`
	Score       int           `json:"score"` // 0 to 100
	State       EvidenceState `json:"state"`
	Summary     string        `json:"summary"`
	FindingsCount int         `json:"findings_count"`
}

// AssetsDetected lists the project harness assets found during evaluation.
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
	InspectBridge bool     `json:"inspect_bridge"`
	SightBridge   bool     `json:"sight_bridge"`
}

// HarnessReport represents the complete self-contained evaluation report for a workspace.
type HarnessReport struct {
	TargetPath     string                   `json:"target_path"`
	GeneratedAt    time.Time                `json:"generated_at"`
	OverallScore   int                      `json:"overall_score"` // 0 to 100
	OverallStatus  string                   `json:"overall_status"`
	Dimensions     map[Dimension]DimensionScore `json:"dimensions"`
	Findings       []Finding                `json:"findings"`
	Assets         AssetsDetected           `json:"assets"`
	Summary        string                   `json:"summary"`
}

// EvaluateOptions defines options for workspace evaluation.
type EvaluateOptions struct {
	TargetPath      string
	IncludeSessions bool
	Formats         []string // "html", "markdown", "json"
	OutputDir       string
}

// ToContractReport converts the native Hawk HarnessReport to the neutral hawk-core-contracts Report.
func (r *HarnessReport) ToContractReport() *harnesscontracts.Report {
	if r == nil {
		return nil
	}

	dims := make(map[harnesscontracts.Dimension]harnesscontracts.DimensionScore, len(r.Dimensions))
	for k, v := range r.Dimensions {
		dims[harnesscontracts.Dimension(k)] = harnesscontracts.DimensionScore{
			Dimension:     harnesscontracts.Dimension(v.Dimension),
			Score:         v.Score,
			State:         harnesscontracts.EvidenceState(v.State),
			Summary:       v.Summary,
			FindingsCount: v.FindingsCount,
		}
	}

	findings := make([]harnesscontracts.Finding, len(r.Findings))
	for i, f := range r.Findings {
		var sev typescontracts.Severity
		switch f.Severity {
		case SeverityCritical:
			sev = typescontracts.SeverityCritical
		case SeverityHigh:
			sev = typescontracts.SeverityHigh
		case SeverityMedium:
			sev = typescontracts.SeverityMedium
		case SeverityLow:
			sev = typescontracts.SeverityLow
		default:
			sev = typescontracts.SeverityInfo
		}

		findings[i] = harnesscontracts.Finding{
			ID:              f.ID,
			Dimension:       harnesscontracts.Dimension(f.Dimension),
			Severity:        sev,
			Title:           f.Title,
			Description:     f.Description,
			Impact:          f.Impact,
			EvidenceSource:  f.EvidenceSource,
			EvidenceState:   harnesscontracts.EvidenceState(f.EvidenceState),
			ExpectedOutcome: f.ExpectedOutcome,
			ScopedRepair:    f.ScopedRepair,
			ValidationRoute: f.ValidationRoute,
		}
	}

	return &harnesscontracts.Report{
		TargetPath:    r.TargetPath,
		GeneratedAt:   r.GeneratedAt,
		OverallScore:  r.OverallScore,
		OverallStatus: r.OverallStatus,
		Dimensions:    dims,
		Findings:      findings,
		Assets: harnesscontracts.AssetsDetected{
			AgentsMD:      r.Assets.AgentsMD,
			AgentsMDPath:  r.Assets.AgentsMDPath,
			ZeroMD:        r.Assets.ZeroMD,
			ZeroMDPath:    r.Assets.ZeroMDPath,
			Skills:        r.Assets.Skills,
			SpecsCount:    r.Assets.SpecsCount,
			Linters:       r.Assets.Linters,
			TestRunners:   r.Assets.TestRunners,
			Hooks:         r.Assets.Hooks,
			SandboxPolicy: r.Assets.SandboxPolicy,
			AutonomyTier:  r.Assets.AutonomyTier,
			InspectBridge: r.Assets.InspectBridge,
			SightBridge:   r.Assets.SightBridge,
		},
		Summary: r.Summary,
	}
}
