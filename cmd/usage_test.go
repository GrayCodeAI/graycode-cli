package cmd

import (
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"
)

func TestRunUsageSummarizesLedger(t *testing.T) {
	path := filepath.Join(t.TempDir(), "usage.jsonl")
	now := time.Now().UnixMilli()
	const hourMS = int64(60 * 60 * 1000)
	const dayMS = 24 * hourMS
	content := `{"schema_version":1,"kind":"coverage","started_at_ms":` + strconv.FormatInt(now, 10) + `}
{"schema_version":1,"kind":"generation","fact":{"created_at_ms":` + strconv.FormatInt(now-2*hourMS, 10) + `,"model":"acme/m1","input_tokens":100,"output_tokens":20,"total_cost":0.0012}}
{"schema_version":1,"kind":"generation","fact":{"created_at_ms":` + strconv.FormatInt(now-10*dayMS, 10) + `,"model":"old/m1","input_tokens":999,"output_tokens":999,"total_cost":9.0}}
`
	_ = os.WriteFile(path, []byte(content), 0o600)

	oldLedger := usageLedger
	usageLedger = path
	defer func() { usageLedger = oldLedger }()

	var sb strings.Builder
	cmd := usageCmd
	cmd.SetOut(&sb)
	cmd.SetErr(&sb)
	if err := runUsage(cmd, nil); err != nil {
		t.Fatalf("runUsage: %v", err)
	}
	out := sb.String()
	if !strings.Contains(out, "acme/m1") {
		t.Errorf("output missing acme/m1:\n%s", out)
	}
	if strings.Contains(out, "old/m1") {
		t.Errorf("output includes old/m1, want excluded by period:\n%s", out)
	}
	if !strings.Contains(out, "total") {
		t.Errorf("output missing total row:\n%s", out)
	}
}

func TestRunUsageJSON(t *testing.T) {
	path := filepath.Join(t.TempDir(), "usage.jsonl")
	now := time.Now().UnixMilli()
	content := `{"schema_version":1,"kind":"coverage","started_at_ms":` + strconv.FormatInt(now, 10) + `}
{"schema_version":1,"kind":"generation","fact":{"created_at_ms":` + strconv.FormatInt(now, 10) + `,"model":"m","input_tokens":2,"output_tokens":1,"total_cost":0.0003}}
`
	_ = os.WriteFile(path, []byte(content), 0o600)

	oldLedger := usageLedger
	usageLedger = path
	defer func() { usageLedger = oldLedger }()
	oldJSON := usageJSON
	usageJSON = true
	defer func() { usageJSON = oldJSON }()

	var sb strings.Builder
	cmd := usageCmd
	cmd.SetOut(&sb)
	cmd.SetErr(&sb)
	if err := runUsage(cmd, nil); err != nil {
		t.Fatalf("runUsage: %v", err)
	}
	out := sb.String()
	if !strings.Contains(out, "\"total_cost_usd\": 0.0003") {
		t.Errorf("json output missing expected cost:\n%s", out)
	}
	if !strings.Contains(out, "\"total_tokens\": 3") {
		t.Errorf("json output missing expected tokens:\n%s", out)
	}
}

func TestRunUsageEmpty(t *testing.T) {
	oldLedger := usageLedger
	usageLedger = filepath.Join(t.TempDir(), "absent.jsonl")
	defer func() { usageLedger = oldLedger }()

	var sb strings.Builder
	cmd := usageCmd
	cmd.SetOut(&sb)
	cmd.SetErr(&sb)
	if err := runUsage(cmd, nil); err != nil {
		t.Fatalf("runUsage: %v", err)
	}
	if !strings.Contains(sb.String(), "No usage recorded") {
		t.Errorf("empty output = %q, want 'No usage recorded'", sb.String())
	}
}
