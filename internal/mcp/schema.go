package mcp

// SanitizeToolSchema prunes an MCP tool's JSON Schema down to the subset that
// strict OpenAI-compatible function-calling endpoints accept. Many MCP servers
// publish full JSON Schema documents (with "$schema", "title", "definitions",
// "additionalProperties", "examples", etc.); several providers (and some local
// vLLM/SGLang routes) reject a function's parameter schema outright when it
// carries keys outside the function-calling subset. Qwen-Agent solves this by
// keeping only {type, properties, required} per tool — hawk does the same.
//
// The function is conservative: it never invents structure. If the input does
// not look like an object schema (no "type":"object" and no "properties"), it is
// returned unchanged so non-object tools (rare, but valid) are not corrupted. A
// nil input yields the canonical empty-object schema so the model still sees a
// well-formed parameter block.
func SanitizeToolSchema(schema map[string]interface{}) map[string]interface{} {
	if schema == nil {
		return map[string]interface{}{
			"type":       "object",
			"properties": map[string]interface{}{},
		}
	}

	_, hasProps := schema["properties"]
	if schema["type"] != "object" && !hasProps {
		// Not an object schema — leave it alone.
		return schema
	}

	out := map[string]interface{}{"type": "object"}

	if props, ok := schema["properties"].(map[string]interface{}); ok {
		out["properties"] = sanitizeProperties(props)
	} else {
		out["properties"] = map[string]interface{}{}
	}

	// "required" must be a list of strings; copy it only when well-formed and
	// non-empty (an empty required list is the default and adds nothing).
	if req, ok := schema["required"].([]interface{}); ok && len(req) > 0 {
		out["required"] = req
	}

	return out
}

// allowedPropKeys is the per-property keyword subset preserved during pruning.
// These are the keywords function-calling endpoints actually consume to build a
// prompt-time tool description; everything else (validation-only or
// documentation-only keywords) is dropped.
var allowedPropKeys = map[string]struct{}{
	"type":        {},
	"description": {},
	"enum":        {},
	"items":       {},
	"properties":  {},
	"required":    {},
	"default":     {},
}

// sanitizeProperties prunes each property to allowedPropKeys, recursing into
// nested object/array schemas so the subset is applied at every depth.
func sanitizeProperties(props map[string]interface{}) map[string]interface{} {
	cleaned := make(map[string]interface{}, len(props))
	for name, raw := range props {
		spec, ok := raw.(map[string]interface{})
		if !ok {
			cleaned[name] = raw
			continue
		}
		cp := make(map[string]interface{}, len(spec))
		for k, v := range spec {
			if _, allow := allowedPropKeys[k]; !allow {
				continue
			}
			switch k {
			case "properties":
				if nested, ok := v.(map[string]interface{}); ok {
					cp[k] = sanitizeProperties(nested)
					continue
				}
			case "items":
				if item, ok := v.(map[string]interface{}); ok {
					cp[k] = sanitizeItems(item)
					continue
				}
			}
			cp[k] = v
		}
		cleaned[name] = cp
	}
	return cleaned
}

// sanitizeItems prunes an array "items" sub-schema, recursing into its own
// properties when it describes objects.
func sanitizeItems(item map[string]interface{}) map[string]interface{} {
	cp := make(map[string]interface{}, len(item))
	for k, v := range item {
		if _, allow := allowedPropKeys[k]; !allow {
			continue
		}
		if k == "properties" {
			if nested, ok := v.(map[string]interface{}); ok {
				cp[k] = sanitizeProperties(nested)
				continue
			}
		}
		cp[k] = v
	}
	return cp
}
