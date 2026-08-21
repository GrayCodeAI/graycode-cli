package usage

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestAppendRoundTrip(t *testing.T) {
	path := filepath.Join(t.TempDir(), "usage.jsonl")
	rec := Record{
		CreatedAtMS:  1700000000000,
		Model:        "acme/m1",
		Provider:     "acme",
		InputTokens:  100,
		OutputTokens: 20,
		TotalCost:    0.0012,
	}
	if err := appendTo(path, rec); err != nil {
		t.Fatalf("appendTo: %v", err)
	}
	if err := appendTo(path, Record{CreatedAtMS: 1700000001000, Model: "acme/m1", InputTokens: 5, OutputTokens: 1, TotalCost: 0.0001}); err != nil {
		t.Fatalf("appendTo #2: %v", err)
	}

	got, err := ReadFrom(path)
	if err != nil {
		t.Fatalf("ReadFrom: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("ReadFrom returned %d records, want 2", len(got))
	}
	if got[0].Model != "acme/m1" || got[0].InputTokens != 100 || got[0].TotalCost != 0.0012 {
		t.Errorf("record 0 = %+v, want model acme/m1, in=100, cost=0.0012", got[0])
	}
	if got[1].OutputTokens != 1 {
		t.Errorf("record 1 out = %d, want 1", got[1].OutputTokens)
	}
}

func TestReadFromToleratesMalformedLine(t *testing.T) {
	path := filepath.Join(t.TempDir(), "usage.jsonl")
	content := `{"schema_version":1,"kind":"coverage","started_at_ms":1700000000000}
{"schema_version":1,"kind":"generation","fact":{"created_at_ms":1700000000000,"model":"m","input_tokens":1,"output_tokens":0,"total_cost":0}}
{"schema_version":1,"kind":"generation","fact":{"created_at_ms":1700000000
`
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatalf("write fixture: %v", err)
	}
	got, err := ReadFrom(path)
	if err != nil {
		t.Fatalf("ReadFrom: %v", err)
	}
	if len(got) != 1 || got[0].Model != "m" {
		t.Fatalf("ReadFrom = %+v, want 1 good record", got)
	}
}

func TestMissingLedgerIsEmpty(t *testing.T) {
	got, err := ReadFrom(filepath.Join(t.TempDir(), "nope.jsonl"))
	if err != nil {
		t.Fatalf("ReadFrom missing: %v", err)
	}
	if got == nil || len(got) != 0 {
		t.Fatalf("ReadFrom missing = %v, want empty non-nil", got)
	}
}

func TestSummarize(t *testing.T) {
	const hourMS = int64(60 * 60 * 1000)
	const dayMS = 24 * hourMS
	now := time.Now().UnixMilli()
	records := []Record{
		{CreatedAtMS: now - 2*hourMS, Model: "a/m", InputTokens: 100, OutputTokens: 10, TotalCost: 0.01},
		{CreatedAtMS: now - 2*hourMS, Model: "a/m", InputTokens: 50, OutputTokens: 5, CacheReadTokens: 20, TotalCost: 0.005},
		{CreatedAtMS: now - 10*dayMS, Model: "b/m", InputTokens: 999, OutputTokens: 999, TotalCost: 9.0},
	}

	sum := Summarize(records, now-7*dayMS)
	if sum.Generations != 2 {
		t.Fatalf("Generations = %d, want 2 (b/m excluded)", sum.Generations)
	}
	if len(sum.ByModel) != 1 || sum.ByModel[0].Model != "a/m" {
		t.Fatalf("ByModel = %+v, want only a/m", sum.ByModel)
	}
	m := sum.ByModel[0]
	if m.InputTokens != 150 || m.OutputTokens != 15 || m.CacheReadTokens != 20 || m.Generations != 2 {
		t.Errorf("a/m usage = %+v, want in=150 out=15 cache_read=20 gens=2", m)
	}
	if m.TotalTokens != 150+15+20 {
		t.Errorf("TotalTokens = %d, want %d", m.TotalTokens, 150+15+20)
	}
	if sum.TotalCostUSD != 0.015 {
		t.Errorf("TotalCostUSD = %v, want 0.015", sum.TotalCostUSD)
	}
}

func TestParsePeriod(t *testing.T) {
	for _, p := range []string{"24h", "7d", "30d", ""} {
		since, d, err := ParsePeriod(p)
		if err != nil {
			t.Errorf("ParsePeriod(%q) unexpected error %v", p, err)
			continue
		}
		if d <= 0 {
			t.Errorf("ParsePeriod(%q) d = %v, want >0", p, d)
		}
		if since > time.Now().UnixMilli() {
			t.Errorf("ParsePeriod(%q) since %d in the future", p, since)
		}
	}
	if _, _, err := ParsePeriod("1d"); err == nil {
		t.Error("ParsePeriod(\"1d\") = nil error, want error")
	}
	if _, _, err := ParsePeriod("1y"); err == nil {
		t.Error("ParsePeriod(\"1y\") = nil error, want error")
	}
}
