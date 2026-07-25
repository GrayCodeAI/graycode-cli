package cmd

import (
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"
)

func TestNormalizeTraceCorrelationDeterministicAndDeduplicated(t *testing.T) {
	t.Parallel()

	correlation := traceCorrelation{
		SchemaVersion:            traceCorrelationSchemaVersion,
		HawkSessionID:            "hawk-session",
		CheckpointLookupComplete: true,
		Matches: []traceCorrelationMatch{
			{
				TraceSessionID: "trace-beta",
				CheckpointIDs:  []string{"bbbbbbbbbbbb", "aaaaaaaaaaaa", "bbbbbbbbbbbb"},
			},
			{
				TraceSessionID: "trace-alpha",
				CheckpointIDs:  []string{},
			},
		},
	}
	if err := normalizeTraceCorrelation(&correlation, "hawk-session"); err != nil {
		t.Fatalf("normalizeTraceCorrelation() error = %v", err)
	}
	if correlation.Matches[0].TraceSessionID != "trace-alpha" {
		t.Fatalf("first match = %q, want trace-alpha", correlation.Matches[0].TraceSessionID)
	}
	got := correlation.Matches[1].CheckpointIDs
	if len(got) != 2 || got[0] != "aaaaaaaaaaaa" || got[1] != "bbbbbbbbbbbb" {
		t.Fatalf("normalized checkpoint IDs = %#v", got)
	}
}

func TestNormalizeTraceCorrelationRejectsUntrustedIdentityData(t *testing.T) {
	t.Parallel()

	valid := func() traceCorrelation {
		return traceCorrelation{
			SchemaVersion:            traceCorrelationSchemaVersion,
			HawkSessionID:            "hawk-session",
			CheckpointLookupComplete: true,
			Matches: []traceCorrelationMatch{{
				TraceSessionID: "trace-session",
				CheckpointIDs:  []string{"abc123def456"},
			}},
		}
	}
	tests := []struct {
		name   string
		mutate func(*traceCorrelation)
	}{
		{
			name: "schema",
			mutate: func(value *traceCorrelation) {
				value.SchemaVersion = "trace.correlation/v2"
			},
		},
		{
			name: "hawk identity",
			mutate: func(value *traceCorrelation) {
				value.HawkSessionID = "other-session"
			},
		},
		{
			name: "path separator",
			mutate: func(value *traceCorrelation) {
				value.Matches[0].TraceSessionID = "../trace-session"
			},
		},
		{
			name: "checkpoint",
			mutate: func(value *traceCorrelation) {
				value.Matches[0].CheckpointIDs = []string{"not-a-checkpoint"}
			},
		},
		{
			name: "duplicate session",
			mutate: func(value *traceCorrelation) {
				value.Matches = append(value.Matches, value.Matches[0])
			},
		},
	}
	for _, test := range tests {
		test := test
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			value := valid()
			test.mutate(&value)
			if err := normalizeTraceCorrelation(&value, "hawk-session"); err == nil {
				t.Fatal("normalizeTraceCorrelation() error = nil")
			}
		})
	}
}

func TestNormalizeTraceCorrelationDropsUnverifiedCheckpointList(t *testing.T) {
	t.Parallel()

	correlation := traceCorrelation{
		SchemaVersion:            traceCorrelationSchemaVersion,
		HawkSessionID:            "hawk-session",
		CheckpointLookupComplete: false,
		Matches: []traceCorrelationMatch{{
			TraceSessionID: "trace-session",
			CheckpointIDs:  []string{"abc123def456"},
		}},
	}
	if err := normalizeTraceCorrelation(&correlation, "hawk-session"); err != nil {
		t.Fatalf("normalizeTraceCorrelation() error = %v", err)
	}
	if len(correlation.Matches) != 1 || correlation.Matches[0].CheckpointIDs == nil ||
		len(correlation.Matches[0].CheckpointIDs) != 0 {
		t.Fatalf("unverified checkpoints were retained: %#v", correlation.Matches)
	}
}

func TestDecodeTraceCorrelationAcceptsCompleteTraceV1Envelope(t *testing.T) {
	t.Parallel()

	payload := []byte(`{
	  "schema_version": "trace.correlation/v1",
	  "hawk_session_id": "hawk-session",
	  "checkpoint_lookup_complete": true,
	  "matches": [{
	    "trace_session_id": "trace-session",
	    "checkpoint_ids": ["abc123def456"],
	    "started_at": "2026-07-25T06:00:00Z",
	    "ended_at": "2026-07-25T07:00:00Z",
	    "phase": "ENDED"
	  }]
	}`)
	correlation, err := decodeTraceCorrelation(payload, "hawk-session")
	if err != nil {
		t.Fatalf("decodeTraceCorrelation() error = %v", err)
	}
	if len(correlation.Matches) != 1 ||
		correlation.Matches[0].EndedAt == nil ||
		correlation.Matches[0].Phase != "ENDED" {
		t.Fatalf("decoded correlation = %#v", correlation)
	}
}

func TestTraceReferencesFromCorrelationUsesStableTimes(t *testing.T) {
	t.Parallel()

	fallback := time.Date(2026, time.July, 25, 7, 0, 0, 0, time.UTC)
	startedAt := fallback.Add(-time.Hour)
	sessions, checkpoints := traceReferencesFromCorrelation(traceCorrelation{
		Matches: []traceCorrelationMatch{
			{
				TraceSessionID: "trace-alpha",
				CheckpointIDs:  []string{"abc123def456"},
				StartedAt:      startedAt,
			},
			{
				TraceSessionID: "trace-beta",
				CheckpointIDs:  []string{"bbbbbbbbbbbb"},
			},
		},
	}, fallback)
	if len(sessions) != 2 || len(checkpoints) != 2 {
		t.Fatalf("references = %d sessions, %d checkpoints", len(sessions), len(checkpoints))
	}
	if !sessions[0].CreatedAt.Equal(startedAt) {
		t.Fatalf("session time = %s, want %s", sessions[0].CreatedAt, startedAt)
	}
	if !sessions[1].CreatedAt.Equal(fallback) {
		t.Fatalf("zero session time did not use stable fallback: %s", sessions[1].CreatedAt)
	}
	if checkpoints[0].TraceSessionID != "trace-alpha" ||
		!checkpoints[0].CreatedAt.Equal(fallback) {
		t.Fatalf("checkpoint reference = %#v", checkpoints[0])
	}
}

func TestCappedBufferRejectsOversizedTraceOutput(t *testing.T) {
	t.Parallel()

	buffer := cappedBuffer{max: 4}
	written, err := buffer.Write([]byte("12345"))
	if written != 4 || !errors.Is(err, errTraceCorrelationOutputLimit) {
		t.Fatalf("Write() = (%d, %v)", written, err)
	}
	if buffer.String() != "1234" {
		t.Fatalf("buffer = %q", buffer.String())
	}
}

func TestRequireJSONEOFRejectsTrailingValue(t *testing.T) {
	t.Parallel()

	decoder := json.NewDecoder(strings.NewReader(`{} {"unexpected":true}`))
	var first map[string]any
	if err := decoder.Decode(&first); err != nil {
		t.Fatalf("decode first value: %v", err)
	}
	if err := requireJSONEOF(decoder); err == nil {
		t.Fatal("requireJSONEOF() error = nil")
	}
}
