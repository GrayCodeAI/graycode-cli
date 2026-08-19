package tool

import (
	"context"
	"encoding/json"
	"testing"
)

type stubRegistryTool struct{}

func (stubRegistryTool) Name() string        { return "StubRegistry" }
func (stubRegistryTool) Description() string { return "stub" }
func (stubRegistryTool) Parameters() map[string]interface{} {
	return map[string]interface{}{"type": "object", "properties": map[string]interface{}{}}
}
func (stubRegistryTool) Execute(context.Context, json.RawMessage) (string, error) { return "", nil }
func (stubRegistryTool) Aliases() []string                                        { return []string{"stub-alias", "stub-alias-2"} }

func TestRegistryUnregisterRemovesPrimaryAndAliases(t *testing.T) {
	stub := stubRegistryTool{}
	r := NewRegistry(stub)
	for _, name := range []string{"StubRegistry", "stub-alias", "stub-alias-2"} {
		if got, ok := r.Get(name); !ok || got.(stubRegistryTool) != stub {
			t.Fatalf("expected registry to resolve %q", name)
		}
	}
	if !r.Unregister("stub-alias") {
		t.Fatal("Unregister returned false for existing alias")
	}
	if _, ok := r.Get("StubRegistry"); ok {
		t.Fatal("primary still present after alias unregister")
	}
	if len(r.PrimaryTools()) != 0 {
		t.Fatalf("primary list still has %d entries", len(r.PrimaryTools()))
	}
	if r.Unregister("stub-alias-2") {
		t.Fatal("second unregister returned true after tool removed")
	}
}
