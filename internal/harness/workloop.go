package harness

import (
	"time"
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
