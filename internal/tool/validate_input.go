package tool

import (
	"encoding/json"
	"fmt"
	"strings"
)

// ValidateToolInput checks that the required parameters declared in the
// tool's Parameters() schema are present in the input JSON, and that the
// input parses as a JSON object. A required field is satisfied by its value
// or by an alias property (a sibling property whose description marks it as
// an alias for the field, e.g. Read's "file_path" alias for "path").
//
// It is invoked by Registry.Execute before dispatch (H5); previously the
// validator was dead code and malformed input reached the tool
// implementations, which each threw their own inconsistent errors.
func ValidateToolInput(t Tool, input json.RawMessage) error {
	if t == nil {
		return nil
	}
	var inputMap map[string]interface{}
	if len(input) == 0 {
		inputMap = map[string]interface{}{}
	} else if err := json.Unmarshal(input, &inputMap); err != nil {
		return fmt.Errorf("tool %s: invalid JSON input: %w", t.Name(), err)
	}

	params := t.Parameters()
	for _, field := range requiredFields(params) {
		if fieldPresent(inputMap, field, params) {
			continue
		}
		return fmt.Errorf("tool %s requires %q parameter", t.Name(), field)
	}
	return nil
}

// requiredFields extracts the "required" array from a tool schema.
func requiredFields(params map[string]interface{}) []string {
	raw, ok := params["required"]
	if !ok {
		return nil
	}
	var names []string
	switch arr := raw.(type) {
	case []string:
		names = arr
	case []interface{}:
		for _, v := range arr {
			if s, ok := v.(string); ok {
				names = append(names, s)
			}
		}
	}
	return names
}

// fieldPresent reports whether input carries a non-empty value for field,
// accepting alias properties of the field (matching how the core tools
// resolve e.g. path vs file_path).
func fieldPresent(input map[string]interface{}, field string, params map[string]interface{}) bool {
	if v, ok := input[field]; ok && !isEmptyValue(v) {
		return true
	}
	for _, alias := range aliasesFor(params, field) {
		if v, ok := input[alias]; ok && !isEmptyValue(v) {
			return true
		}
	}
	return false
}

// aliasesFor returns sibling properties whose description marks them as an
// alias for the given field.
func aliasesFor(params map[string]interface{}, field string) []string {
	propsRaw, ok := params["properties"]
	if !ok {
		return nil
	}
	props, ok := propsRaw.(map[string]interface{})
	if !ok {
		return nil
	}
	var aliases []string
	for name, specRaw := range props {
		if name == field {
			continue
		}
		spec, ok := specRaw.(map[string]interface{})
		if !ok {
			continue
		}
		desc, _ := spec["description"].(string)
		if strings.Contains(desc, "alias for "+field) {
			aliases = append(aliases, name)
		}
	}
	return aliases
}

func isEmptyValue(v interface{}) bool {
	if s, ok := v.(string); ok && s == "" {
		return true
	}
	return false
}
