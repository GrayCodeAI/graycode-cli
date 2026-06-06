package agent

// SubAgentMode determines the capabilities and cost profile of a sub-agent.
type SubAgentMode string

const (
	SubAgentExplore SubAgentMode = "explore"
	SubAgentGeneral SubAgentMode = "general"
	// SubAgentPlan is a read-only planning mode: it decomposes a task into
	// ordered, actionable steps without making changes. It mirrors Claude
	// Code's "plan" subagent. Like explore it is restricted to read-only
	// tools, but it gets a larger turn budget so it can reason over more
	// context before producing a plan.
	SubAgentPlan SubAgentMode = "plan"
)

// Sub-agent budget defaults per mode.
const (
	DefaultExploreTurns = 15
	DefaultGeneralTurns = 20
	// DefaultPlanTurns is the turn budget for plan-mode sub-agents. Planning
	// benefits from broad read-only exploration before synthesis, so it sits
	// above explore but below a full general agent.
	DefaultPlanTurns = 18
	MaxAgentDepth    = 2
)

// ExploreTools are the read-only tools available to explore-mode sub-agents.
var ExploreTools = []string{
	"Glob", "Grep", "Read", "Bash", "LS",
}

// PlanTools are the read-only tools available to plan-mode sub-agents.
// Planning is non-destructive, so it shares explore's read-only surface.
var PlanTools = []string{
	"Glob", "Grep", "Read", "Bash", "LS",
}

// ExploreThoroughness controls how much turn budget an explore-mode sub-agent
// receives. Higher thoroughness trades latency/cost for more complete coverage.
// It mirrors Claude Code's explore thoroughness levels.
type ExploreThoroughness string

const (
	ThoroughnessQuick        ExploreThoroughness = "quick"
	ThoroughnessMedium       ExploreThoroughness = "medium"
	ThoroughnessVeryThorough ExploreThoroughness = "very-thorough"
)

// Per-thoroughness explore turn budgets. These are distinct so that callers
// can tune exploration depth without touching the default explore budget.
const (
	ThoroughnessQuickTurns        = 8
	ThoroughnessMediumTurns       = 15
	ThoroughnessVeryThoroughTurns = 30
)

// ExploreConfig configures an explore-mode sub-agent. The zero value resolves
// to medium thoroughness via DefaultExploreConfig / Turns().
type ExploreConfig struct {
	Thoroughness ExploreThoroughness
}

// DefaultExploreConfig returns the default explore configuration (medium).
func DefaultExploreConfig() ExploreConfig {
	return ExploreConfig{Thoroughness: ThoroughnessMedium}
}

// Turns returns the turn budget for the configured thoroughness. Unknown or
// empty thoroughness values fall back to the default explore budget.
func (c ExploreConfig) Turns() int {
	return ThoroughnessTurns(c.Thoroughness)
}

// ThoroughnessTurns maps a thoroughness level to its explore turn budget.
// Unknown or empty levels return DefaultExploreTurns.
func ThoroughnessTurns(t ExploreThoroughness) int {
	switch t {
	case ThoroughnessQuick:
		return ThoroughnessQuickTurns
	case ThoroughnessMedium:
		return ThoroughnessMediumTurns
	case ThoroughnessVeryThorough:
		return ThoroughnessVeryThoroughTurns
	default:
		return DefaultExploreTurns
	}
}

// DefaultTurnsForMode returns the default turn budget for a sub-agent mode.
// It is the single source of truth for mode->budget resolution and is used
// to wire new modes (such as plan) into the dispatch path.
func DefaultTurnsForMode(mode SubAgentMode) int {
	switch mode {
	case SubAgentExplore:
		return DefaultExploreTurns
	case SubAgentPlan:
		return DefaultPlanTurns
	case SubAgentGeneral:
		return DefaultGeneralTurns
	default:
		return DefaultGeneralTurns
	}
}

// IsReadOnlyMode reports whether a mode is restricted to read-only tools.
// Both explore and plan are non-destructive.
func IsReadOnlyMode(mode SubAgentMode) bool {
	return mode == SubAgentExplore || mode == SubAgentPlan
}
