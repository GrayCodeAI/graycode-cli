package tool

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
)

func TestDependencyAuditToolMetadata(t *testing.T) {
	tool := DependencyAuditTool{}
	if tool.Name() != "DependencyAudit" || tool.RiskLevel() != "medium" {
		t.Fatalf("metadata = %q/%q", tool.Name(), tool.RiskLevel())
	}
}

func TestDependencyAuditRejectsInstallLikeActions(t *testing.T) {
	_, err := (DependencyAuditTool{}).Execute(context.Background(), json.RawMessage(`{"action":"install"}`))
	if err == nil || !strings.Contains(err.Error(), "unsupported action") {
		t.Fatalf("error = %v", err)
	}
}
