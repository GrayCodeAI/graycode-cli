package safety

// Capability describes a concrete effect a tool may have. Policies should
// reason about capabilities instead of relying on tool-name allowlists.
type Capability string

// RiskLevel is the default severity associated with a tool capability set.
type RiskLevel string

const (
	RiskLow    RiskLevel = "low"
	RiskMedium RiskLevel = "medium"
	RiskHigh   RiskLevel = "high"
)

const (
	CapabilityUnknown            Capability = "unknown"
	CapabilityFilesystemRead     Capability = "filesystem.read"
	CapabilityFilesystemWrite    Capability = "filesystem.write"
	CapabilityFilesystemDelete   Capability = "filesystem.delete"
	CapabilityProcessExecute     Capability = "process.execute"
	CapabilityNetworkAccess      Capability = "network.access"
	CapabilityCredentialsAccess  Capability = "credentials.access"
	CapabilityUserInteraction    Capability = "user.interaction"
	CapabilitySpecRead           Capability = "spec.read"
	CapabilitySpecWrite          Capability = "spec.write"
	CapabilitySpecApprove        Capability = "spec.approve"
	CapabilityConfigurationRead  Capability = "configuration.read"
	CapabilityConfigurationWrite Capability = "configuration.write"
	CapabilityDestructive        Capability = "destructive"
)

// ToolPolicy is the declarative safety metadata for a canonical tool.
type ToolPolicy struct {
	Name         string
	Capabilities []Capability
	DefaultRisk  RiskLevel
}

var toolPolicies = map[string]ToolPolicy{
	"Read":                  {Name: "Read", Capabilities: []Capability{CapabilityFilesystemRead}, DefaultRisk: RiskLow},
	"Glob":                  {Name: "Glob", Capabilities: []Capability{CapabilityFilesystemRead}, DefaultRisk: RiskLow},
	"Grep":                  {Name: "Grep", Capabilities: []Capability{CapabilityFilesystemRead}, DefaultRisk: RiskLow},
	"LS":                    {Name: "LS", Capabilities: []Capability{CapabilityFilesystemRead}, DefaultRisk: RiskLow},
	"Bash":                  {Name: "Bash", Capabilities: []Capability{CapabilityProcessExecute}, DefaultRisk: RiskHigh},
	"Write":                 {Name: "Write", Capabilities: []Capability{CapabilityFilesystemWrite}, DefaultRisk: RiskMedium},
	"Edit":                  {Name: "Edit", Capabilities: []Capability{CapabilityFilesystemWrite}, DefaultRisk: RiskMedium},
	"Delete":                {Name: "Delete", Capabilities: []Capability{CapabilityFilesystemDelete, CapabilityDestructive}, DefaultRisk: RiskHigh},
	"AskUserQuestion":       {Name: "AskUserQuestion", Capabilities: []Capability{CapabilityUserInteraction}, DefaultRisk: RiskLow},
	"Specify":               {Name: "Specify", Capabilities: []Capability{CapabilitySpecWrite}, DefaultRisk: RiskLow},
	"Plan":                  {Name: "Plan", Capabilities: []Capability{CapabilitySpecRead, CapabilitySpecWrite}, DefaultRisk: RiskLow},
	"Tasks":                 {Name: "Tasks", Capabilities: []Capability{CapabilitySpecRead, CapabilitySpecWrite}, DefaultRisk: RiskLow},
	"ApproveImplementation": {Name: "ApproveImplementation", Capabilities: []Capability{CapabilitySpecApprove, CapabilityUserInteraction}, DefaultRisk: RiskHigh},
	"SpecStatus":            {Name: "SpecStatus", Capabilities: []Capability{CapabilitySpecRead}, DefaultRisk: RiskLow},
	"SpecList":              {Name: "SpecList", Capabilities: []Capability{CapabilitySpecRead}, DefaultRisk: RiskLow},
	"SpecEdit":              {Name: "SpecEdit", Capabilities: []Capability{CapabilitySpecWrite}, DefaultRisk: RiskMedium},
	"SpecReset":             {Name: "SpecReset", Capabilities: []Capability{CapabilitySpecWrite}, DefaultRisk: RiskMedium},
	"SpecConfig":            {Name: "SpecConfig", Capabilities: []Capability{CapabilityConfigurationRead, CapabilityConfigurationWrite}, DefaultRisk: RiskMedium},
	"Clarify":               {Name: "Clarify", Capabilities: []Capability{CapabilitySpecRead, CapabilityUserInteraction}, DefaultRisk: RiskLow},
	"Analyze":               {Name: "Analyze", Capabilities: []Capability{CapabilitySpecRead}, DefaultRisk: RiskLow},
	"Checklist":             {Name: "Checklist", Capabilities: []Capability{CapabilitySpecRead}, DefaultRisk: RiskLow},
	"Constitution":          {Name: "Constitution", Capabilities: []Capability{CapabilitySpecRead, CapabilitySpecWrite}, DefaultRisk: RiskMedium},
	"Converge":              {Name: "Converge", Capabilities: []Capability{CapabilitySpecRead, CapabilitySpecWrite}, DefaultRisk: RiskMedium},
}

// ToolPolicyFor returns a copy of the policy for a canonical tool. Unknown
// tools are explicitly represented and therefore fail closed in strict policy.
func ToolPolicyFor(name string) ToolPolicy {
	canonical := canonicalToolName(name)
	if policy, ok := toolPolicies[canonical]; ok {
		policy.Capabilities = append([]Capability(nil), policy.Capabilities...)
		return policy
	}
	return ToolPolicy{Name: canonical, Capabilities: []Capability{CapabilityUnknown}, DefaultRisk: RiskHigh}
}

// ToolCapabilities returns a defensive copy of a tool's capabilities.
func ToolCapabilities(name string) []Capability {
	return ToolPolicyFor(name).Capabilities
}
