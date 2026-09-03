package cmd

import (
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"
)

func TestNormalizeSwiftCorrelationDeterministicAndDeduplicated(t *testing.T) {
	t.Parallel()

	correlation := swiftCorrelation{
		SchemaVersion:            swiftCorrelationSchemaVersion,
		GraycodeSessionID:        "graycode-session",
		CheckpointLookupComplete: true,
		Matches: []swiftCorrelationMatch{
			{
				SwiftSessionID: "swift-beta",
				CheckpointIDs:  []string{"bbbbbbbbbbbb", "aaaaaaaaaaaa", "bbbbbbbbbbbb"},
			},
			{
				SwiftSessionID: "swift-alpha",
				CheckpointIDs:  []string{},
			},
		},
	}
	if err := normalizeSwiftCorrelation(&correlation, "graycode-session"); err != nil {
		t.Fatalf("normalizeSwiftCorrelation() error = %v", err)
	}
	if correlation.Matches[0].SwiftSessionID != "swift-alpha" {
		t.Fatalf("first match = %q, want swift-alpha", correlation.Matches[0].SwiftSessionID)
	}
	got := correlation.Matches[1].CheckpointIDs
	if len(got) != 2 || got[0] != "aaaaaaaaaaaa" || got[1] != "bbbbbbbbbbbb" {
		t.Fatalf("normalized checkpoint IDs = %#v", got)
	}
}

func TestNormalizeSwiftCorrelationRejectsUntrustedIdentityData(t *testing.T) {
	t.Parallel()

	valid := func() swiftCorrelation {
		return swiftCorrelation{
			SchemaVersion:            swiftCorrelationSchemaVersion,
			GraycodeSessionID:        "graycode-session",
			CheckpointLookupComplete: true,
			Matches: []swiftCorrelationMatch{{
				SwiftSessionID: "swift-session",
				CheckpointIDs:  []string{"abc123def456"},
			}},
		}
	}
	tests := []struct {
		name   string
		mutate func(*swiftCorrelation)
	}{
		{
			name: "schema",
			mutate: func(value *swiftCorrelation) {
				value.SchemaVersion = "swift.correlation/v2"
			},
		},
		{
			name: "graycode identity",
			mutate: func(value *swiftCorrelation) {
				value.GraycodeSessionID = "other-session"
			},
		},
		{
			name: "path separator",
			mutate: func(value *swiftCorrelation) {
				value.Matches[0].SwiftSessionID = "../swift-session"
			},
		},
		{
			name: "checkpoint",
			mutate: func(value *swiftCorrelation) {
				value.Matches[0].CheckpointIDs = []string{"not-a-checkpoint"}
			},
		},
		{
			name: "duplicate session",
			mutate: func(value *swiftCorrelation) {
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
			if err := normalizeSwiftCorrelation(&value, "graycode-session"); err == nil {
				t.Fatal("normalizeSwiftCorrelation() error = nil")
			}
		})
	}
}

func TestNormalizeSwiftCorrelationDropsUnverifiedCheckpointList(t *testing.T) {
	t.Parallel()

	correlation := swiftCorrelation{
		SchemaVersion:            swiftCorrelationSchemaVersion,
		GraycodeSessionID:        "graycode-session",
		CheckpointLookupComplete: false,
		Matches: []swiftCorrelationMatch{{
			SwiftSessionID: "swift-session",
			CheckpointIDs:  []string{"abc123def456"},
		}},
	}
	if err := normalizeSwiftCorrelation(&correlation, "graycode-session"); err != nil {
		t.Fatalf("normalizeSwiftCorrelation() error = %v", err)
	}
	if len(correlation.Matches) != 1 || correlation.Matches[0].CheckpointIDs == nil ||
		len(correlation.Matches[0].CheckpointIDs) != 0 {
		t.Fatalf("unverified checkpoints were retained: %#v", correlation.Matches)
	}
}

func TestDecodeSwiftCorrelationAcceptsCompleteSwiftV1Envelope(t *testing.T) {
	t.Parallel()

	payload := []byte(`{
	  "schema_version": "swift.correlation/v1",
	  "graycode_session_id": "graycode-session",
	  "checkpoint_lookup_complete": true,
	  "matches": [{
	    "swift_session_id": "swift-session",
	    "checkpoint_ids": ["abc123def456"],
	    "started_at": "2026-07-25T06:00:00Z",
	    "ended_at": "2026-07-25T07:00:00Z",
	    "phase": "ENDED"
	  }]
	}`)
	correlation, err := decodeSwiftCorrelation(payload, "graycode-session")
	if err != nil {
		t.Fatalf("decodeSwiftCorrelation() error = %v", err)
	}
	if len(correlation.Matches) != 1 ||
		correlation.Matches[0].EndedAt == nil ||
		correlation.Matches[0].Phase != "ENDED" {
		t.Fatalf("decoded correlation = %#v", correlation)
	}
}

func TestSwiftReferencesFromCorrelationUsesStableTimes(t *testing.T) {
	t.Parallel()

	fallback := time.Date(2026, time.July, 25, 7, 0, 0, 0, time.UTC)
	startedAt := fallback.Add(-time.Hour)
	sessions, checkpoints := swiftReferencesFromCorrelation(swiftCorrelation{
		Matches: []swiftCorrelationMatch{
			{
				SwiftSessionID: "swift-alpha",
				CheckpointIDs:  []string{"abc123def456"},
				StartedAt:      startedAt,
			},
			{
				SwiftSessionID: "swift-beta",
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
	if checkpoints[0].SwiftSessionID != "swift-alpha" ||
		!checkpoints[0].CreatedAt.Equal(fallback) {
		t.Fatalf("checkpoint reference = %#v", checkpoints[0])
	}
}

func TestCappedBufferRejectsOversizedSwiftOutput(t *testing.T) {
	t.Parallel()

	buffer := cappedBuffer{max: 4}
	written, err := buffer.Write([]byte("12345"))
	if written != 4 || !errors.Is(err, errSwiftCorrelationOutputLimit) {
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
