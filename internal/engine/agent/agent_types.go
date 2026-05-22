package agent

// SubAgentMode determines the capabilities and cost profile of a sub-agent.
type SubAgentMode string

const (
	SubAgentExplore SubAgentMode = "explore"
	SubAgentGeneral SubAgentMode = "general"
)

// Sub-agent budget defaults per mode.
const (
	DefaultExploreTurns = 15
	DefaultGeneralTurns = 20
	MaxAgentDepth       = 2
)

// ExploreTools are the read-only tools available to explore-mode sub-agents.
var ExploreTools = []string{
	"Glob", "Grep", "Read", "Bash", "LS",
}
