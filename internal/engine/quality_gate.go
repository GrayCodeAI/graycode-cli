package engine

import (
	"context"
	"fmt"

	"github.com/GrayCodeAI/hawk/internal/ui/icons"
)

// GatePhase represents a phase in the spec-driven workflow.
type GatePhase int

const (
	GateSpec      GatePhase = iota // specification created
	GatePlan                       // plan covers all criteria
	GateImplement                  // code compiles, basic checks pass
	GateVerify                     // all acceptance criteria met
	GateDone                       // ready to commit
)

func (p GatePhase) String() string {
	switch p {
	case GateSpec:
		return "spec"
	case GatePlan:
		return "plan"
	case GateImplement:
		return "implement"
	case GateVerify:
		return "verify"
	case GateDone:
		return "done"
	default:
		return "unknown"
	}
}

// GateResult is the outcome of a quality gate check.
type GateResult struct {
	Phase  GatePhase
	Passed bool
	Reason string
}

// QualityGate defines a checkpoint between phases.
type QualityGate struct {
	Phase GatePhase
	Check func() GateResult
}

// QualityGates runs all gates in sequence. Stops at first failure.
func RunQualityGates(gates []QualityGate) ([]GateResult, bool) {
	var results []GateResult
	for _, g := range gates {
		result := g.Check()
		results = append(results, result)
		if !result.Passed {
			return results, false
		}
	}
	return results, true
}

// SpecGate checks that a spec is complete.
func SpecGate(spec *Spec) QualityGate {
	return QualityGate{
		Phase: GateSpec,
		Check: func() GateResult {
			if spec == nil {
				return GateResult{Phase: GateSpec, Passed: false, Reason: "no spec provided"}
			}
			if spec.Goal == "" {
				return GateResult{Phase: GateSpec, Passed: false, Reason: "spec missing goal"}
			}
			if len(spec.Criteria) == 0 {
				return GateResult{Phase: GateSpec, Passed: false, Reason: "spec has no acceptance criteria"}
			}
			if !spec.Approved {
				return GateResult{Phase: GateSpec, Passed: false, Reason: "spec not approved by user"}
			}
			return GateResult{Phase: GateSpec, Passed: true, Reason: "spec complete and approved"}
		},
	}
}

// ImplementGate checks that code compiles and basic tests pass.
func ImplementGate(validateCmd string, workDir string) QualityGate {
	return QualityGate{
		Phase: GateImplement,
		Check: func() GateResult {
			el := &ExperimentLoop{WorkDir: workDir, ValidateCmd: validateCmd, Timeout: 60_000_000_000}
			passed, output := el.validate(context.Background())
			if passed {
				return GateResult{Phase: GateImplement, Passed: true, Reason: "build/tests pass"}
			}
			return GateResult{Phase: GateImplement, Passed: false, Reason: fmt.Sprintf("validation failed: %s", truncateGateStr(output, 200))}
		},
	}
}

// FormatGateResults renders gate results for display.
func FormatGateResults(results []GateResult) string {
	var s string
	for _, r := range results {
		icon := icons.CheckBold()
		if !r.Passed {
			icon = icons.CloseThick()
		}
		s += fmt.Sprintf("  %s [%s] %s\n", icon, r.Phase, r.Reason)
	}
	return s
}

func truncateGateStr(s string, max int) string {
	if len(s) <= max {
		return s
	}
	return s[:max] + "..."
}
