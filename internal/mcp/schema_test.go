package mcp

import "testing"

func TestSanitizeToolSchema_PrunesTopLevelKeys(t *testing.T) {
	in := map[string]interface{}{
		"$schema":              "https://json-schema.org/draft/2020-12/schema",
		"title":                "DoThing",
		"description":          "does a thing",
		"additionalProperties": false,
		"type":                 "object",
		"properties": map[string]interface{}{
			"path": map[string]interface{}{"type": "string", "description": "a path"},
		},
		"required": []interface{}{"path"},
	}
	out := SanitizeToolSchema(in)

	for _, banned := range []string{"$schema", "title", "additionalProperties", "description"} {
		if _, ok := out[banned]; ok {
			t.Errorf("expected %q to be pruned, got %v", banned, out[banned])
		}
	}
	if out["type"] != "object" {
		t.Errorf("type = %v, want object", out["type"])
	}
	if _, ok := out["properties"].(map[string]interface{})["path"]; !ok {
		t.Errorf("properties.path dropped: %v", out["properties"])
	}
	req, ok := out["required"].([]interface{})
	if !ok || len(req) != 1 || req[0] != "path" {
		t.Errorf("required = %v, want [path]", out["required"])
	}
}

func TestSanitizeToolSchema_PrunesPerPropertyKeys(t *testing.T) {
	in := map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"q": map[string]interface{}{
				"type":        "string",
				"description": "query",
				"minLength":   1,       // validation-only — should be dropped
				"examples":    []any{}, // doc-only — should be dropped
				"enum":        []interface{}{"a", "b"},
			},
		},
	}
	out := SanitizeToolSchema(in)
	prop := out["properties"].(map[string]interface{})["q"].(map[string]interface{})
	if _, ok := prop["minLength"]; ok {
		t.Error("minLength should be pruned")
	}
	if _, ok := prop["examples"]; ok {
		t.Error("examples should be pruned")
	}
	if prop["type"] != "string" || prop["description"] != "query" {
		t.Errorf("kept keys lost: %v", prop)
	}
	if _, ok := prop["enum"]; !ok {
		t.Error("enum should be kept")
	}
}

func TestSanitizeToolSchema_RecursesNestedAndItems(t *testing.T) {
	in := map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"filter": map[string]interface{}{
				"type":  "object",
				"title": "Filter", // dropped at nested level
				"properties": map[string]interface{}{
					"tags": map[string]interface{}{
						"type":  "array",
						"items": map[string]interface{}{"type": "string", "pattern": "x"}, // pattern dropped
					},
				},
			},
		},
	}
	out := SanitizeToolSchema(in)
	filter := out["properties"].(map[string]interface{})["filter"].(map[string]interface{})
	if _, ok := filter["title"]; ok {
		t.Error("nested title should be pruned")
	}
	tags := filter["properties"].(map[string]interface{})["tags"].(map[string]interface{})
	items := tags["items"].(map[string]interface{})
	if _, ok := items["pattern"]; ok {
		t.Error("items.pattern should be pruned")
	}
	if items["type"] != "string" {
		t.Errorf("items.type lost: %v", items)
	}
}

func TestSanitizeToolSchema_NilYieldsEmptyObject(t *testing.T) {
	out := SanitizeToolSchema(nil)
	if out["type"] != "object" {
		t.Fatalf("nil schema = %v, want empty object schema", out)
	}
	if _, ok := out["properties"].(map[string]interface{}); !ok {
		t.Errorf("expected empty properties map, got %v", out["properties"])
	}
}

func TestSanitizeToolSchema_NonObjectLeftAlone(t *testing.T) {
	in := map[string]interface{}{"type": "string", "minLength": 3}
	out := SanitizeToolSchema(in)
	if out["minLength"] != 3 {
		t.Errorf("non-object schema should be returned unchanged, got %v", out)
	}
}
