package safety

import (
	"fmt"
	"math"
	"strings"
	"sync"

	"github.com/GrayCodeAI/graycode-cli/internal/ui/icons"
)

// RiskAssessment holds the result of evaluating how risky a proposed code change is.
type RiskAssessment struct {
	Score          float64
	Level          string
	Factors        []RiskFactor
	Mitigations    []string
	Recommendation string
}

// RiskFactor represents an individual evaluated risk factor.
type RiskFactor struct {
	Name        string
	Weight      float64
	Score       float64
	Description string
}

// RiskFactorDef defines a risk factor with its evaluation function.
type RiskFactorDef struct {
	Name       string
	Weight     float64
	EvaluateFn func(ctx *RiskContext) float64
}

// RiskContext provides the context needed to assess risk of a change.
type RiskContext struct {
	Files             []string
	Diff              string
	TestsExist        bool
	IsExported        bool
	HasBreakingChange bool
	LinesChanged      int
	FilesAffected     int
	Complexity        int
}

// RiskAssessor evaluates risk of proposed code changes.
type RiskAssessor struct {
	Factors []RiskFactorDef
	mu      sync.Mutex
}

// NewRiskAssessor creates a new RiskAssessor with built-in factors.
func NewRiskAssessor() *RiskAssessor {
	return &RiskAssessor{
		Factors: []RiskFactorDef{
			{
				Name:   "file_count",
				Weight: 0.15,
				EvaluateFn: func(ctx *RiskContext) float64 {
					count := ctx.FilesAffected
					if count <= 0 {
						count = len(ctx.Files)
					}
					switch {
					case count <= 1:
						return 0.1
					case count <= 3:
						return 0.4
					case count <= 5:
						return 0.6
					case count <= 10:
						return 0.8
					default:
						return 1.0
					}
				},
			},
			{
				Name:   "lines_changed",
				Weight: 0.15,
				EvaluateFn: func(ctx *RiskContext) float64 {
					lines := ctx.LinesChanged
					switch {
					case lines <= 10:
						return 0.1
					case lines <= 50:
						return 0.3
					case lines <= 100:
						return 0.5
					case lines <= 300:
						return 0.7
					case lines <= 500:
						return 0.85
					default:
						return 1.0
					}
				},
			},
			{
				Name:   "exported_changes",
				Weight: 0.20,
				EvaluateFn: func(ctx *RiskContext) float64 {
					if ctx.IsExported {
						return 0.8
					}
					return 0.1
				},
			},
			{
				Name:   "test_coverage",
				Weight: 0.15,
				EvaluateFn: func(ctx *RiskContext) float64 {
					if ctx.TestsExist {
						return 0.2
					}
					return 0.9
				},
			},
			{
				Name:   "complexity",
				Weight: 0.15,
				EvaluateFn: func(ctx *RiskContext) float64 {
					c := ctx.Complexity
					switch {
					case c <= 2:
						return 0.1
					case c <= 5:
						return 0.3
					case c <= 10:
						return 0.5
					case c <= 20:
						return 0.7
					default:
						return 1.0
					}
				},
			},
			{
				Name:   "breaking_changes",
				Weight: 0.20,
				EvaluateFn: func(ctx *RiskContext) float64 {
					if ctx.HasBreakingChange {
						return 0.9
					}
					return 0.1
				},
			},
		},
	}
}

// Assess evaluates all risk factors and returns a RiskAssessment.
func (ra *RiskAssessor) Assess(ctx *RiskContext) *RiskAssessment {
	ra.mu.Lock()
	defer ra.mu.Unlock()

	assessment := &RiskAssessment{}
	totalWeight := 0.0
	weightedSum := 0.0

	for _, def := range ra.Factors {
		score := def.EvaluateFn(ctx)
		// Clamp score between 0 and 1
		score = math.Max(0, math.Min(1, score))

		factor := RiskFactor{
			Name:        def.Name,
			Weight:      def.Weight,
			Score:       score,
			Description: factorDescription(def.Name, ctx),
		}
		assessment.Factors = append(assessment.Factors, factor)

		weightedSum += score * def.Weight
		totalWeight += def.Weight
	}

	if totalWeight > 0 {
		assessment.Score = weightedSum / totalWeight
	}

	// Clamp final score
	assessment.Score = math.Max(0, math.Min(1, assessment.Score))

	// Determine level
	assessment.Level = determineLevel(assessment.Score)

	// Generate mitigations
	assessment.Mitigations = GenerateMitigations(assessment)

	// Generate recommendation
	assessment.Recommendation = generateRecommendation(assessment)

	return assessment
}

// GenerateMitigations produces mitigation suggestions based on the assessment.
func GenerateMitigations(assessment *RiskAssessment) []string {
	var mitigations []string

	for _, f := range assessment.Factors {
		switch f.Name {
		case "test_coverage":
			if f.Score > 0.5 {
				mitigations = append(mitigations, "Add tests for modified functions")
			}
		case "exported_changes":
			if f.Score > 0.5 {
				mitigations = append(mitigations, "Review exported API changes carefully")
			}
		case "breaking_changes":
			if f.Score > 0.5 {
				mitigations = append(mitigations, "Run integration tests before merging")
			}
		case "complexity":
			if f.Score > 0.6 {
				mitigations = append(mitigations, "Consider breaking complex changes into smaller PRs")
			}
		case "file_count":
			if f.Score > 0.6 {
				mitigations = append(mitigations, "Verify all affected files are consistent")
			}
		case "lines_changed":
			if f.Score > 0.7 {
				mitigations = append(mitigations, "Request additional code review for large changes")
			}
		}
	}

	if len(mitigations) == 0 {
		mitigations = append(mitigations, "No specific mitigations needed")
	}

	return mitigations
}

// FormatAssessment produces a human-readable formatted string of the assessment.
func FormatAssessment(assessment *RiskAssessment) string {
	var sb strings.Builder

	levelUpper := strings.ToUpper(assessment.Level)
	sb.WriteString(fmt.Sprintf("Risk Assessment: %s (%.2f)\n", levelUpper, assessment.Score))
	sb.WriteString("═══════════════════════════════\n")
	sb.WriteString("Factors:\n")

	for _, f := range assessment.Factors {
		label := factorLabel(f.Name, f.Description)
		bar := riskRenderBar(f.Score)
		warning := ""
		if f.Score >= 0.9 {
			warning = "  " + "CRIT"
		} else if f.Score >= 0.7 {
			warning = "  " + icons.Alert()
		}
		sb.WriteString(fmt.Sprintf("  %-30s %.1f  %s%s\n", label, f.Score, bar, warning))
	}

	sb.WriteString("\nMitigations:\n")
	for _, m := range assessment.Mitigations {
		sb.WriteString(fmt.Sprintf("  • %s\n", m))
	}

	sb.WriteString(fmt.Sprintf("\nRecommendation: %s\n", assessment.Recommendation))

	return sb.String()
}

// ShouldProceed returns false for "critical" risk level, indicating the change
// should not proceed without further review.
func ShouldProceed(assessment *RiskAssessment) bool {
	return assessment.Level != "critical"
}

// determineLevel maps a numeric score to a risk level string.
func determineLevel(score float64) string {
	switch {
	case score >= 0.8:
		return "critical"
	case score >= 0.6:
		return "high"
	case score >= 0.35:
		return "medium"
	default:
		return "low"
	}
}

// generateRecommendation produces a recommendation string based on the assessment.
func generateRecommendation(assessment *RiskAssessment) string {
	switch assessment.Level {
	case "critical":
		return "Do not proceed without thorough review and testing"
	case "high":
		return "Proceed with caution — add tests first"
	case "medium":
		return "Proceed with standard review process"
	default:
		return "Safe to proceed"
	}
}

// factorDescription generates a description for a factor given the context.
func factorDescription(name string, ctx *RiskContext) string {
	switch name {
	case "file_count":
		count := ctx.FilesAffected
		if count <= 0 {
			count = len(ctx.Files)
		}
		return fmt.Sprintf("%d files", count)
	case "lines_changed":
		return fmt.Sprintf("%d lines", ctx.LinesChanged)
	case "exported_changes":
		if ctx.IsExported {
			return "exported symbols modified"
		}
		return "no exported symbols modified"
	case "test_coverage":
		if ctx.TestsExist {
			return "tests exist"
		}
		return "no tests found"
	case "complexity":
		return fmt.Sprintf("complexity score %d", ctx.Complexity)
	case "breaking_changes":
		if ctx.HasBreakingChange {
			return "breaking changes detected"
		}
		return "no breaking changes"
	default:
		return ""
	}
}

// factorLabel returns a display label for a factor.
func factorLabel(name, description string) string {
	displayName := ""
	switch name {
	case "file_count":
		displayName = "File count"
	case "lines_changed":
		displayName = "Lines changed"
	case "exported_changes":
		displayName = "Exported changes"
	case "test_coverage":
		displayName = "Test coverage"
	case "complexity":
		displayName = "Complexity"
	case "breaking_changes":
		displayName = "Breaking changes"
	default:
		displayName = name
	}
	if description != "" {
		return fmt.Sprintf("%s (%s):", displayName, description)
	}
	return displayName + ":"
}

// riskRenderBar produces a visual bar representation of a score (0-1).
func riskRenderBar(score float64) string {
	const barLen = 10
	filled := int(math.Round(score * barLen))
	if filled > barLen {
		filled = barLen
	}
	if filled < 0 {
		filled = 0
	}
	empty := barLen - filled
	return strings.Repeat("░", empty) + strings.Repeat("█", filled)
}
