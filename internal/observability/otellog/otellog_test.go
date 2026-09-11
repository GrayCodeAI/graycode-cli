package otellog

import (
	"context"
	"sync"
	"testing"
	"time"

	"go.opentelemetry.io/otel/log"
	sdklog "go.opentelemetry.io/otel/sdk/log"
	"go.opentelemetry.io/otel/sdk/resource"
)

// --- in-memory pipeline for tests ---

// memExporter is a minimal sdklog.Exporter capturing exported records.
type memExporter struct {
	mu      sync.Mutex
	records []sdklog.Record
}

func (e *memExporter) Export(_ context.Context, records []sdklog.Record) error {
	e.mu.Lock()
	defer e.mu.Unlock()
	for _, r := range records {
		e.records = append(e.records, r.Clone())
	}
	return nil
}

func (e *memExporter) Shutdown(context.Context) error { return nil }
func (e *memExporter) ForceFlush(context.Context) error {
	return nil
}

func (e *memExporter) snapshot() []sdklog.Record {
	e.mu.Lock()
	defer e.mu.Unlock()
	return append([]sdklog.Record(nil), e.records...)
}

// testBackend builds a backend with a simple-processor pipeline over memExporter.
func testBackend(t *testing.T, cfg Config) (*Backend, *memExporter) {
	t.Helper()
	ex := &memExporter{}
	old := providerBuilder
	providerBuilder = func(_ context.Context, _ Config, res *resource.Resource) (*sdklog.LoggerProvider, error) {
		return sdklog.NewLoggerProvider(
			sdklog.WithResource(res),
			sdklog.WithProcessor(sdklog.NewSimpleProcessor(ex)),
		), nil
	}
	t.Cleanup(func() { providerBuilder = old })
	b, err := NewBackend(cfg)
	if err != nil {
		t.Fatal(err)
	}
	return b, ex
}

// --- config resolution ---

func TestResolveMode(t *testing.T) {
	cases := []struct {
		in   Mode
		want Mode
		ok   bool
	}{
		{"", ModeDisabled, true},
		{ModeFull, ModeFull, true},
		{ModeFeedbackOnly, ModeFeedbackOnly, true},
		{ModeDisabled, ModeDisabled, true},
		{"SOMETHING", "", false},
	}
	for _, c := range cases {
		got, err := resolveMode(c.in)
		if (err == nil) != c.ok {
			t.Errorf("resolveMode(%q) err = %v, want ok=%v", c.in, err, c.ok)
			continue
		}
		if c.ok && got != c.want {
			t.Errorf("resolveMode(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}

func TestConfigValidation(t *testing.T) {
	// URL required outside DISABLED.
	if _, err := NewBackend(Config{Mode: ModeFull, ShutdownTimeout: time.Second}); err == nil {
		t.Fatal("FULL without URL must fail")
	}
	// Invalid URL rejected.
	if _, err := NewBackend(Config{Mode: ModeFull, URL: "://bad", ShutdownTimeout: time.Second}); err == nil {
		t.Fatal("invalid URL must fail")
	}
	// Non-http scheme rejected.
	if _, err := NewBackend(Config{Mode: ModeFull, URL: "ftp://collector.example.com/v1/logs", ShutdownTimeout: time.Second}); err == nil {
		t.Fatal("non-http scheme must fail")
	}
	// Zero shutdown timeout resolves to the default (DSH config defaulting);
	// negative is rejected.
	b, err := NewBackend(Config{Mode: ModeFull, URL: "https://c.example/v1/logs", ShutdownTimeout: 0})
	if err != nil {
		t.Fatalf("zero shutdown timeout must default, got error: %v", err)
	}
	if b.shutdownTimeout != DefaultShutdownTimeout {
		t.Fatalf("zero shutdown timeout resolved to %s, want %s", b.shutdownTimeout, DefaultShutdownTimeout)
	}
	_ = b.Shutdown(context.Background())
	if _, err := NewBackend(Config{Mode: ModeFull, URL: "https://c.example/v1/logs", ShutdownTimeout: -time.Second}); err == nil {
		t.Fatal("negative shutdown timeout must fail")
	}
	// Non-positive batch size rejected (DSH load-time invariant).
	if _, err := NewBackend(Config{Mode: ModeFull, URL: "https://c.example/v1/logs", ShutdownTimeout: time.Second, MaxExportBatchSize: -1}); err == nil {
		t.Fatal("negative batch size must fail")
	}
	// DISABLED reads neither URL nor timeout.
	b, err = NewBackend(Config{Mode: ModeDisabled})
	if err != nil {
		t.Fatalf("DISABLED must construct without validation: %v", err)
	}
	if b.Sharing() != SharingDisabled || b.Mode() != ModeDisabled {
		t.Fatalf("disabled backend sharing/mode = %s/%s", b.Sharing(), b.Mode())
	}
	if err := b.Shutdown(context.Background()); err != nil {
		t.Fatalf("disabled shutdown = %v, want nil", err)
	}
}

func TestSeverityMapping(t *testing.T) {
	cases := []struct {
		in  Severity
		num log.Severity
		tex string
	}{
		{SeverityInfo, log.SeverityInfo, "INFO"},
		{SeverityWarn, log.SeverityWarn, "WARN"},
		{SeverityError, log.SeverityError, "ERROR"},
	}
	for _, c := range cases {
		num, tex := severityFor(c.in)
		if num != c.num || tex != c.tex {
			t.Errorf("severityFor(%q) = (%d, %q), want (%d, %q)", c.in, num, tex, c.num, c.tex)
		}
	}
}

// --- emission ---

func TestFullModeEmitsRecord(t *testing.T) {
	b, ex := testBackend(t, Config{
		Mode:            ModeFull,
		URL:             "https://c.example/v1/logs",
		ShutdownTimeout: time.Second,
	})
	now := time.UnixMilli(1_700_000_000_123)
	b.Emit(Record{
		Channel:  ChannelLedger,
		Time:     now,
		Severity: SeverityWarn,
		Attributes: map[string]any{
			"session.id": "sess-1",
			"event.seq":  int64(7),
			"active":     true,
		},
		Body: map[string]any{"count": 3, "ok": true},
	})
	if err := b.Flush(context.Background()); err != nil {
		t.Fatal(err)
	}
	got := ex.snapshot()
	if len(got) != 1 {
		t.Fatalf("exported %d records, want 1", len(got))
	}
	r := got[0]
	if !r.Timestamp().Equal(now) || !r.ObservedTimestamp().Equal(now) {
		t.Fatalf("timestamps = %v/%v, want %v", r.Timestamp(), r.ObservedTimestamp(), now)
	}
	if r.Severity() != log.SeverityWarn || r.SeverityText() != "WARN" {
		t.Fatalf("severity = %d/%q, want WARN", r.Severity(), r.SeverityText())
	}
	if r.Body().Kind() != log.KindMap {
		t.Fatalf("body kind = %v, want map", r.Body().Kind())
	}
	var gotSeq int64
	var gotActive bool
	r.WalkAttributes(func(kv log.KeyValue) bool {
		switch kv.Key {
		case "session.id":
			if kv.Value.AsString() != "sess-1" {
				t.Errorf("session.id = %q", kv.Value.AsString())
			}
		case "event.seq":
			gotSeq = kv.Value.AsInt64()
		case "active":
			gotActive = kv.Value.AsBool()
		}
		return true
	})
	if gotSeq != 7 || !gotActive {
		t.Errorf("attributes seq/active = %d/%v", gotSeq, gotActive)
	}
}

func TestOpsChannelAndSeverity(t *testing.T) {
	b, ex := testBackend(t, Config{
		Mode:            ModeFull,
		URL:             "https://c.example/v1/logs",
		ShutdownTimeout: time.Second,
	})
	b.Emit(Record{Channel: ChannelOps, Time: time.Now(), Severity: SeverityError, Body: "boom"})
	if err := b.Flush(context.Background()); err != nil {
		t.Fatal(err)
	}
	got := ex.snapshot()
	if len(got) != 1 {
		t.Fatalf("exported %d records, want 1", len(got))
	}
	if got[0].Severity() != log.SeverityError || got[0].SeverityText() != "ERROR" {
		t.Fatalf("severity = %d/%q, want ERROR", got[0].Severity(), got[0].SeverityText())
	}
	if got[0].Body().AsString() != "boom" {
		t.Fatalf("body = %q, want boom", got[0].Body().AsString())
	}
	// Channel routing: ops records live under the ops instrumentation scope.
	if scope := got[0].InstrumentationScope().Name; scope != "github.com/GrayCodeAI/hawk/internal/observability/otellog/ops" {
		t.Fatalf("scope = %q, want .../ops", scope)
	}
}

func TestLedgerScopeRouting(t *testing.T) {
	b, ex := testBackend(t, Config{
		Mode:            ModeFull,
		URL:             "https://c.example/v1/logs",
		ShutdownTimeout: time.Second,
	})
	b.Emit(Record{Channel: ChannelLedger, Time: time.Now(), Severity: SeverityInfo, Body: "ledger"})
	if err := b.Flush(context.Background()); err != nil {
		t.Fatal(err)
	}
	got := ex.snapshot()
	if len(got) != 1 {
		t.Fatalf("exported %d records, want 1", len(got))
	}
	if scope := got[0].InstrumentationScope().Name; scope != "github.com/GrayCodeAI/hawk/internal/observability/otellog" {
		t.Fatalf("scope = %q, want the ledger scope", scope)
	}
}

func TestFeedbackOnlyMode(t *testing.T) {
	b, ex := testBackend(t, Config{
		Mode:            ModeFeedbackOnly,
		URL:             "https://c.example/v1/logs",
		ShutdownTimeout: time.Second,
	})
	// Direct emission is a no-op; feedback capture enqueues.
	b.Emit(Record{Channel: ChannelLedger, Time: time.Now(), Severity: SeverityInfo, Body: "direct"})
	b.EmitFeedback(Record{Channel: ChannelLedger, Time: time.Now(), Severity: SeverityInfo, Body: "feedback"})
	if err := b.Flush(context.Background()); err != nil {
		t.Fatal(err)
	}
	got := ex.snapshot()
	if len(got) != 1 {
		t.Fatalf("exported %d records, want 1 (direct dropped)", len(got))
	}
	if got[0].Body().AsString() != "feedback" {
		t.Fatalf("body = %q, want feedback", got[0].Body().AsString())
	}
}

func TestDisabledModeDropsEverything(t *testing.T) {
	b, _ := testBackend(t, Config{Mode: ModeDisabled})
	b.Emit(Record{Channel: ChannelLedger, Time: time.Now(), Severity: SeverityError, Body: "x"})
	b.EmitFeedback(Record{Channel: ChannelOps, Time: time.Now(), Severity: SeverityError, Body: "y"})
	// No provider: nothing to flush; shutdown resolves immediately.
	if err := b.Flush(context.Background()); err != nil {
		t.Fatal(err)
	}
	if err := b.Shutdown(context.Background()); err != nil {
		t.Fatal(err)
	}
}

func TestShutdownIdempotent(t *testing.T) {
	b, _ := testBackend(t, Config{
		Mode:            ModeFull,
		URL:             "https://c.example/v1/logs",
		ShutdownTimeout: time.Second,
	})
	if err := b.Shutdown(context.Background()); err != nil {
		t.Fatal(err)
	}
	if err := b.Shutdown(context.Background()); err != nil {
		t.Fatal("second shutdown must be a no-op")
	}
}

// --- value conversion ---

func TestAnyToValue(t *testing.T) {
	if got := anyToValue(nil); got.AsString() != "" {
		t.Errorf("nil = %q", got.AsString())
	}
	if got := anyToValue("s"); got.AsString() != "s" {
		t.Errorf("string = %q", got.AsString())
	}
	if got := anyToValue(true); got.AsBool() != true {
		t.Errorf("bool = %v", got.AsBool())
	}
	if got := anyToValue(int64(42)); got.AsInt64() != 42 {
		t.Errorf("int64 = %d", got.AsInt64())
	}
	if got := anyToValue(3.5); got.AsFloat64() != 3.5 {
		t.Errorf("float64 = %v", got.AsFloat64())
	}
	sl := anyToValue([]any{"a", int64(1)})
	if sl.Kind() != log.KindSlice || sl.AsSlice()[1].AsInt64() != 1 {
		t.Errorf("slice = %+v", sl)
	}
	mp := anyToValue(map[string]any{"nested": map[string]any{"k": "v"}, "n": int64(2)})
	if mp.Kind() != log.KindMap {
		t.Fatalf("map kind = %v", mp.Kind())
	}
	seen := map[string]bool{}
	for _, kv := range mp.AsMap() {
		seen[kv.Key] = true
	}
	if !seen["nested"] || !seen["n"] {
		t.Errorf("map keys = %v", seen)
	}
	// Unsupported kind falls back to JSON text.
	if got := anyToValue(struct{ A int }{A: 1}); got.Kind() != log.KindString || got.AsString() != `{"A":1}` {
		t.Errorf("struct fallback = %v %q", got.Kind(), got.AsString())
	}
}

func TestAttrsToKeyValuesSkipsUnsupported(t *testing.T) {
	kvs := attrsToKeyValues(map[string]any{
		"s":    "x",
		"i":    5,
		"skip": struct{}{},
	})
	if len(kvs) != 2 {
		t.Fatalf("kvs = %d, want 2 (unsupported skipped)", len(kvs))
	}
}
