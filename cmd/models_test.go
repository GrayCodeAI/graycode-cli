package cmd

import (
	"encoding/json"
	"reflect"
	"sort"
	"strings"
	"testing"

	hawkconfig "github.com/GrayCodeAI/hawk/internal/config"
)

func TestMarshalModelListJSONCompatibilityGolden(t *testing.T) {
	entry := modelListJSONEntry{
		ID:               "vendor/model-v2",
		InputPricePer1M:  1.25,
		OutputPricePer1M: 5,
		ContextWindow:    128000,
		MaxOutput:        8192,
		ServerTools:      []string{"web_search", "code_execution"},
		DisplayName:      "Model V2",
		Description:      "A provider model",
		Owner:            "vendor",
		LiveMetadata:     json.RawMessage(`{"id":"provider-model-v2","owned_by":"vendor"}`),
	}

	got, err := marshalModelListEntriesJSON([]modelListJSONEntry{entry}, false, false)
	if err != nil {
		t.Fatalf("marshalModelListEntriesJSON() error = %v", err)
	}
	want := `[
  {
    "id": "vendor/model-v2",
    "input_price_per_1m": 1.25,
    "output_price_per_1m": 5,
    "context_window": 128000,
    "max_output": 8192,
    "server_tools": [
      "web_search",
      "code_execution"
    ],
    "display_name": "Model V2",
    "description": "A provider model",
    "owner": "vendor",
    "live_metadata": {
      "id": "provider-model-v2",
      "owned_by": "vendor"
    }
  }
]`
	if string(got) != want {
		t.Fatalf("compatibility JSON changed\n--- got ---\n%s\n--- want ---\n%s", got, want)
	}

	var rows []map[string]json.RawMessage
	if err := json.Unmarshal(got, &rows); err != nil {
		t.Fatalf("decode compatibility JSON: %v", err)
	}
	wantKeys := []string{
		"context_window", "description", "display_name", "id",
		"input_price_per_1m", "live_metadata", "max_output", "output_price_per_1m",
		"owner", "server_tools",
	}
	gotKeys := make([]string, 0, len(rows[0]))
	for key := range rows[0] {
		gotKeys = append(gotKeys, key)
	}
	sort.Strings(gotKeys)
	if !reflect.DeepEqual(gotKeys, wantKeys) {
		t.Fatalf("JSON keys = %v, want %v", gotKeys, wantKeys)
	}
}

func TestModelListJSONEntryFromEnginePreservesLegacyFieldMapping(t *testing.T) {
	model := hawkconfig.EngineModel{
		ID:               "vendor/model",
		DisplayName:      "Model",
		Description:      "Description",
		Owner:            "vendor",
		ProviderID:       "deployment",
		GatewayID:        "gateway",
		CanonicalID:      "vendor/model",
		ContextWindow:    200000,
		MaxOutputTokens:  16000,
		InputPricePer1M:  3,
		OutputPricePer1M: 15,
		PriceKnown:       true,
		Capabilities:     []string{"web_search"},
		Source:           "cache",
	}

	entry := modelListJSONEntryFromEngine(model)
	if entry.MaxOutput != model.MaxOutputTokens {
		t.Fatalf("MaxOutput = %d, want %d", entry.MaxOutput, model.MaxOutputTokens)
	}
	if !reflect.DeepEqual(entry.ServerTools, model.Capabilities) {
		t.Fatalf("ServerTools = %v, want %v", entry.ServerTools, model.Capabilities)
	}

	out, err := marshalModelListJSON([]hawkconfig.EngineModel{model}, false, false)
	if err != nil {
		t.Fatalf("marshalModelListJSON() error = %v", err)
	}
	for _, forbidden := range []string{
		`"provider_id"`, `"gateway_id"`, `"canonical_id"`, `"max_output_tokens"`,
		`"capabilities"`, `"price_known"`, `"source"`,
	} {
		if strings.Contains(string(out), forbidden) {
			t.Fatalf("compatibility JSON leaked engine field %s: %s", forbidden, out)
		}
	}
}

func TestMarshalModelListRawCachedUsesOnlyMetadataWhenPresent(t *testing.T) {
	entries := []modelListJSONEntry{
		{
			ID:           "cached-with-live-metadata",
			LiveMetadata: json.RawMessage(`{"id":"native-id","object":"model","owned_by":"vendor"}`),
		},
		{
			ID:            "cached-without-live-metadata",
			DisplayName:   "Cached Model",
			ContextWindow: 32000,
		},
	}

	got, err := marshalModelListEntriesJSON(entries, true, false)
	if err != nil {
		t.Fatalf("marshalModelListEntriesJSON(raw) error = %v", err)
	}
	want := `[
  {
    "id": "native-id",
    "object": "model",
    "owned_by": "vendor"
  }
]`
	if string(got) != want {
		t.Fatalf("raw JSON changed\n--- got ---\n%s\n--- want ---\n%s", got, want)
	}
}

func TestMarshalModelListRawLiveWithMetadataDoesNotMixFallbackRows(t *testing.T) {
	entries := []modelListJSONEntry{
		{ID: "native", LiveMetadata: json.RawMessage(`{"id":"native"}`)},
		{ID: "fallback"},
	}
	got, err := marshalModelListEntriesJSON(entries, true, true)
	if err != nil {
		t.Fatalf("marshalModelListEntriesJSON(raw live) error = %v", err)
	}
	if strings.Contains(string(got), `"fallback"`) || !strings.Contains(string(got), `"id": "native"`) {
		t.Fatalf("live raw with native metadata must not mix output shapes, got %s", got)
	}
}

func TestMarshalModelListRawCachedWithoutMetadataIsEmpty(t *testing.T) {
	got, err := marshalModelListEntriesJSON([]modelListJSONEntry{{
		ID: "safe-fallback", LiveMetadata: json.RawMessage(`not-json`),
	}}, true, false)
	if err != nil {
		t.Fatalf("marshalModelListEntriesJSON(raw) error = %v", err)
	}
	if string(got) != "[]" {
		t.Fatalf("cached raw without native metadata = %s, want []", got)
	}
}

func TestMarshalModelListRawLiveWithoutMetadataFallsBack(t *testing.T) {
	got, err := marshalModelListEntriesJSON([]modelListJSONEntry{{
		ID: "safe-fallback", DisplayName: "Fallback",
	}}, true, true)
	if err != nil {
		t.Fatalf("marshalModelListEntriesJSON(raw live) error = %v", err)
	}
	if strings.Contains(string(got), "null") || !strings.Contains(string(got), `"id": "safe-fallback"`) {
		t.Fatalf("live raw without native metadata should use compatible rows, got %s", got)
	}
}

func TestValidModelLiveMetadataPreservesProviderObject(t *testing.T) {
	want := json.RawMessage(`{"id":"native-id"}`)
	got := validModelLiveMetadata(want)
	if string(got) != string(want) {
		t.Fatalf("modelLiveMetadata() = %s, want %s", got, want)
	}
}
