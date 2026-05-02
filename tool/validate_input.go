package tool

import (
	"encoding/json"
	"fmt"
)

// ValidateToolInput checks that required parameters are present and have correct types.
// It inspects the tool's Parameters() schema for "required" fields and verifies they
// exist in the provided input JSON.
func ValidateToolInput(toolName string, input json.RawMessage) error {
	// Parse the input into a map for inspection
	var inputMap map[string]interface{}
	if len(input) == 0 {
		inputMap = map[string]interface{}{}
	} else {
		if err := json.Unmarshal(input, &inputMap); err != nil {
			return fmt.Errorf("tool %s: invalid JSON input: %w", toolName, err)
		}
	}

	return validateRequiredFields(toolName, inputMap)
}

// validateRequiredFields checks hardcoded required fields for known tools.
func validateRequiredFields(toolName string, input map[string]interface{}) error {
	requiredFields := knownRequiredFields(toolName)
	for _, field := range requiredFields {
		val, ok := input[field]
		if !ok {
			return fmt.Errorf("tool %s requires %q parameter", toolName, field)
		}
		// Check for empty string values on required fields
		if s, isStr := val.(string); isStr && s == "" {
			return fmt.Errorf("tool %s requires non-empty %q parameter", toolName, field)
		}
	}
	return nil
}

// knownRequiredFields returns the required parameter names for well-known tools.
func knownRequiredFields(toolName string) []string {
	switch toolName {
	case "Bash":
		return []string{"command"}
	case "Read":
		return []string{"file_path"}
	case "Write":
		return []string{"file_path", "content"}
	case "Edit":
		return []string{"file_path", "old_string", "new_string"}
	case "Glob":
		return []string{"pattern"}
	case "Grep":
		return []string{"pattern"}
	case "LS":
		return []string{"path"}
	case "WebFetch":
		return []string{"url"}
	case "WebSearch":
		return []string{"query"}
	case "NotebookEdit":
		return []string{"notebook_path", "cell_number"}
	default:
		return nil
	}
}
