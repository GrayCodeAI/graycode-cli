package engine

// ToolInspector inspects tool calls before execution with confidence-based decisions.
// More nuanced than binary allow/deny — provides Allow, Deny, or RequireApproval.
type ToolInspector struct {
	Router *ToolConfirmationRouter
}

// InspectionAction is the decision for a tool call.
type InspectionAction int

const (
	ActionAllow           InspectionAction = iota // execute without asking
	ActionRequireApproval                         // ask user with context
	ActionDeny                                    // block execution
)

// InspectionResult holds the inspection decision with reasoning.
type InspectionResult struct {
	Action     InspectionAction
	Confidence float64 // 0.0-1.0 how confident the decision is
	Reason     string
	ToolName   string
}

// NewToolInspector creates an inspector backed by the confirmation router.
func NewToolInspector() *ToolInspector {
	return &ToolInspector{Router: NewToolConfirmationRouter()}
}

// Inspect analyzes a tool call and returns a decision with confidence.
func (ti *ToolInspector) Inspect(toolName string, args map[string]interface{}) InspectionResult {
	risk := ti.Router.Classify(toolName, args)

	switch risk {
	case RiskNone:
		return InspectionResult{
			Action:     ActionAllow,
			Confidence: 1.0,
			Reason:     "read-only operation",
			ToolName:   toolName,
		}
	case RiskLow:
		return InspectionResult{
			Action:     ActionAllow,
			Confidence: 0.9,
			Reason:     "low-risk modification",
			ToolName:   toolName,
		}
	case RiskHigh:
		return InspectionResult{
			Action:     ActionRequireApproval,
			Confidence: 0.95,
			Reason:     "potentially destructive operation",
			ToolName:   toolName,
		}
	default: // RiskMedium
		return InspectionResult{
			Action:     ActionRequireApproval,
			Confidence: 0.7,
			Reason:     "side-effect operation requires confirmation",
			ToolName:   toolName,
		}
	}
}

// ShouldExecute returns true if the tool can proceed without user input.
func (r InspectionResult) ShouldExecute() bool {
	return r.Action == ActionAllow
}
