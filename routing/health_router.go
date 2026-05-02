package routing

import (
	"bufio"
	"os"
	"strings"
)

// CodeHealth captures complexity metrics for a source file, used to route
// tasks to the cheapest model that can handle the file's complexity.
type CodeHealth struct {
	Complexity   float64 // cyclomatic complexity estimate
	FileSize     int     // lines of code
	Dependencies int     // import count
	TestCoverage float64 // if known (0-1)
	Language     string
}

// ModelTier groups models by the code complexity they can handle.
type ModelTier struct {
	Name          string   // "light", "standard", "heavy"
	Models        []string // model names in this tier
	MaxComplexity float64  // max code health score for this tier
}

// HealthRouter selects the cheapest appropriate model tier based on a file's
// code health metrics.
type HealthRouter struct {
	tiers []ModelTier
}

// NewHealthRouter creates a router with the default tier configuration.
func NewHealthRouter() *HealthRouter {
	return &HealthRouter{
		tiers: DefaultTiers(),
	}
}

// ComputeHealth estimates code health metrics for a file at the given path.
// It reads the file and analyses line count, nesting depth, and import count.
func (hr *HealthRouter) ComputeHealth(path string) CodeHealth {
	h := CodeHealth{}

	// Detect language from extension
	if idx := strings.LastIndex(path, "."); idx >= 0 {
		ext := strings.ToLower(path[idx:])
		switch ext {
		case ".go":
			h.Language = "go"
		case ".py":
			h.Language = "python"
		case ".js":
			h.Language = "javascript"
		case ".ts", ".tsx":
			h.Language = "typescript"
		case ".rs":
			h.Language = "rust"
		case ".java":
			h.Language = "java"
		default:
			h.Language = ext[1:]
		}
	}

	f, err := os.Open(path)
	if err != nil {
		return h
	}
	defer f.Close()

	scanner := bufio.NewScanner(f)
	maxNesting := 0
	currentNesting := 0
	inImportBlock := false

	for scanner.Scan() {
		line := scanner.Text()
		trimmed := strings.TrimSpace(line)
		h.FileSize++

		// Count imports
		if h.Language == "go" {
			if trimmed == "import (" {
				inImportBlock = true
				continue
			}
			if inImportBlock {
				if trimmed == ")" {
					inImportBlock = false
				} else if trimmed != "" && !strings.HasPrefix(trimmed, "//") {
					h.Dependencies++
				}
				continue
			}
			if strings.HasPrefix(trimmed, "import ") && !strings.HasPrefix(trimmed, "import (") {
				h.Dependencies++
			}
		} else if h.Language == "python" {
			if strings.HasPrefix(trimmed, "import ") || strings.HasPrefix(trimmed, "from ") {
				h.Dependencies++
			}
		} else {
			// JS/TS/generic: count import or require statements
			if strings.HasPrefix(trimmed, "import ") || strings.Contains(trimmed, "require(") {
				h.Dependencies++
			}
		}

		// Estimate nesting depth via braces
		for _, ch := range line {
			if ch == '{' {
				currentNesting++
				if currentNesting > maxNesting {
					maxNesting = currentNesting
				}
			} else if ch == '}' {
				currentNesting--
				if currentNesting < 0 {
					currentNesting = 0
				}
			}
		}
	}

	// Cyclomatic complexity estimate: based on file size and max nesting depth.
	// This is a heuristic, not a precise cyclomatic complexity calculation.
	h.Complexity = float64(h.FileSize)/50.0 + float64(maxNesting)*2.0 + float64(h.Dependencies)*0.5

	return h
}

// SelectTier returns the tier name appropriate for the given code health.
//   - "light":    simple files (<100 lines, low complexity)
//   - "standard": moderate files (100-500 lines)
//   - "heavy":    complex files (>500 lines, high complexity, many deps)
func (hr *HealthRouter) SelectTier(health CodeHealth) string {
	score := health.Complexity
	for _, tier := range hr.tiers {
		if score <= tier.MaxComplexity {
			return tier.Name
		}
	}
	// Default to heaviest tier
	if len(hr.tiers) > 0 {
		return hr.tiers[len(hr.tiers)-1].Name
	}
	return "heavy"
}

// ModelForTask returns the cheapest appropriate model for the file's health
// level. If the selected tier contains the primaryModel, it is returned.
// Otherwise, the first model in the selected tier is returned.
func (hr *HealthRouter) ModelForTask(path string, primaryModel string) string {
	health := hr.ComputeHealth(path)
	tierName := hr.SelectTier(health)

	for _, tier := range hr.tiers {
		if tier.Name == tierName {
			// If primary model is in this tier, use it
			for _, m := range tier.Models {
				if m == primaryModel {
					return primaryModel
				}
			}
			// Otherwise return the first model in the tier
			if len(tier.Models) > 0 {
				return tier.Models[0]
			}
		}
	}

	return primaryModel
}

// DefaultTiers returns the standard three-tier configuration.
func DefaultTiers() []ModelTier {
	return []ModelTier{
		{
			Name:          "light",
			Models:        []string{"claude-3-5-haiku-20241022", "gpt-4o-mini", "gemini-2.5-flash"},
			MaxComplexity: 10.0,
		},
		{
			Name:          "standard",
			Models:        []string{"claude-sonnet-4-20250514", "gpt-4o", "gemini-2.5-pro"},
			MaxComplexity: 30.0,
		},
		{
			Name:          "heavy",
			Models:        []string{"claude-opus-4-20250514", "o1-preview", "gemini-2.5-pro"},
			MaxComplexity: 1e9, // effectively unlimited
		},
	}
}
