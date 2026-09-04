package cmd

import (
	"bytes"
	"encoding/json"
	"path/filepath"
	"testing"
)

// preflightReportShape mirrors the JSON-tagged PreflightReport returned by
// EnginePreflightReportWithSettings. We assert against a local shape to
// avoid coupling the test to the exact gateway/graycode-router import path.
type preflightReportShape struct {
	Ready  bool                  `json:"ready"`
	Checks []preflightCheckShape `json:"checks"`
}

type preflightCheckShape struct {
	Name   string `json:"name"`
	Status string `json:"status"`
	Detail string `json:"detail"`
}

func TestPreflightJSON_Structure(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("GRAYCODE_STATE_DIR", filepath.Join(dir, "state"))

	old := preflightJSON
	oldLive := preflightLiveFlag
	t.Cleanup(func() {
		preflightJSON = old
		preflightLiveFlag = oldLive
	})
	preflightJSON = true
	preflightLiveFlag = false

	var buf bytes.Buffer
	preflightCmd.SetOut(&buf)
	preflightCmd.SetErr(&buf)

	// preflight returns a non-nil error when readiness checks fail — expected
	// in a bare test environment with no provider credentials. The JSON report
	// is emitted before that error, so the buffer must contain valid JSON.
	_ = preflightCmd.RunE(preflightCmd, nil)

	if buf.Len() == 0 {
		t.Fatal("preflight --json produced no output")
	}

	var report preflightReportShape
	if err := json.Unmarshal(buf.Bytes(), &report); err != nil {
		t.Fatalf("decoding preflight JSON %q: %v", buf.String(), err)
	}
	// A bare environment has no provider credentials: readiness is false.
	if report.Ready {
		t.Logf("note: preflight reported ready=true in test environment (not a failure)")
	}
}

func TestPreflightText_NotJSON(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("GRAYCODE_STATE_DIR", filepath.Join(dir, "state"))

	old := preflightJSON
	oldLive := preflightLiveFlag
	t.Cleanup(func() {
		preflightJSON = old
		preflightLiveFlag = oldLive
	})
	preflightJSON = false
	preflightLiveFlag = false

	var buf bytes.Buffer
	preflightCmd.SetOut(&buf)
	preflightCmd.SetErr(&buf)

	_ = preflightCmd.RunE(preflightCmd, nil)

	if buf.Len() == 0 {
		t.Fatal("preflight (text) produced no output")
	}
	// Text output is human-readable; it must not be a JSON object.
	if buf.Len() > 0 && buf.Bytes()[0] == '{' {
		t.Errorf("text mode emitted JSON: %q", buf.String())
	}
}
