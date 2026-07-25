package cmd

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"sort"
	"strings"
	"time"
	"unicode"

	tracecli "github.com/GrayCodeAI/trace/cli"
)

const (
	traceCorrelationSchemaVersion = "trace.correlation/v1"
	maxTraceCorrelationOutput     = 1 << 20
	maxTraceSessionIDLength       = 512
)

var errTraceCorrelationOutputLimit = errors.New("trace correlation output exceeds local limit")

type traceCorrelationResolver interface {
	Resolve(context.Context, string) (traceCorrelation, error)
}

type traceCLICorrelationResolver struct{}

type traceCorrelation struct {
	SchemaVersion            string                  `json:"schema_version"`
	HawkSessionID            string                  `json:"hawk_session_id"`
	CheckpointLookupComplete bool                    `json:"checkpoint_lookup_complete"`
	Matches                  []traceCorrelationMatch `json:"matches"`
}

type traceCorrelationMatch struct {
	TraceSessionID string     `json:"trace_session_id"`
	CheckpointIDs  []string   `json:"checkpoint_ids"`
	StartedAt      time.Time  `json:"started_at"`
	EndedAt        *time.Time `json:"ended_at,omitempty"`
	Phase          string     `json:"phase,omitempty"`
}

func (traceCLICorrelationResolver) Resolve(
	ctx context.Context,
	hawkSessionID string,
) (traceCorrelation, error) {
	hawkSessionID = strings.TrimSpace(hawkSessionID)
	if hawkSessionID == "" {
		return traceCorrelation{}, fmt.Errorf("resolve Trace correlation: Hawk session ID is required")
	}

	var stdout, stderr cappedBuffer
	stdout.max = maxTraceCorrelationOutput
	stderr.max = maxTraceCorrelationOutput
	root := tracecli.NewRootCmd()
	root.SetOut(&stdout)
	root.SetErr(&stderr)
	root.SetArgs([]string{"graph", "correlation", "--hawk-session", hawkSessionID})
	// Internal composition must not emit telemetry or launch an asynchronous
	// version check after the read-only lookup completes.
	root.PersistentPostRun = nil
	root.PersistentPostRunE = nil
	if err := root.ExecuteContext(ctx); err != nil {
		return traceCorrelation{}, fmt.Errorf("resolve Trace correlation through CLI: %w", err)
	}

	return decodeTraceCorrelation(stdout.Bytes(), hawkSessionID)
}

func decodeTraceCorrelation(payload []byte, hawkSessionID string) (traceCorrelation, error) {
	decoder := json.NewDecoder(bytes.NewReader(payload))
	decoder.DisallowUnknownFields()
	var correlation traceCorrelation
	if err := decoder.Decode(&correlation); err != nil {
		return traceCorrelation{}, fmt.Errorf("decode Trace correlation: %w", err)
	}
	if err := requireJSONEOF(decoder); err != nil {
		return traceCorrelation{}, err
	}
	if err := normalizeTraceCorrelation(&correlation, hawkSessionID); err != nil {
		return traceCorrelation{}, err
	}
	return correlation, nil
}

func normalizeTraceCorrelation(correlation *traceCorrelation, hawkSessionID string) error {
	if correlation == nil {
		return fmt.Errorf("validate Trace correlation: response is nil")
	}
	if correlation.SchemaVersion != traceCorrelationSchemaVersion {
		return fmt.Errorf(
			"validate Trace correlation: schema %q is not supported",
			correlation.SchemaVersion,
		)
	}
	if correlation.HawkSessionID != hawkSessionID {
		return fmt.Errorf("validate Trace correlation: Hawk session identity mismatch")
	}

	seenSessions := make(map[string]struct{}, len(correlation.Matches))
	for i := range correlation.Matches {
		match := &correlation.Matches[i]
		match.TraceSessionID = strings.TrimSpace(match.TraceSessionID)
		if err := validateTraceSessionID(match.TraceSessionID); err != nil {
			return err
		}
		if _, exists := seenSessions[match.TraceSessionID]; exists {
			return fmt.Errorf(
				"validate Trace correlation: duplicate Trace session %q",
				match.TraceSessionID,
			)
		}
		seenSessions[match.TraceSessionID] = struct{}{}

		if !correlation.CheckpointLookupComplete {
			match.CheckpointIDs = []string{}
			match.StartedAt = match.StartedAt.UTC()
			continue
		}
		seenCheckpoints := make(map[string]struct{}, len(match.CheckpointIDs))
		checkpointIDs := make([]string, 0, len(match.CheckpointIDs))
		for _, checkpointID := range match.CheckpointIDs {
			checkpointID = strings.TrimSpace(checkpointID)
			if err := validateTraceCheckpointID(checkpointID); err != nil {
				return fmt.Errorf("validate Trace correlation: %w", err)
			}
			if _, exists := seenCheckpoints[checkpointID]; exists {
				continue
			}
			seenCheckpoints[checkpointID] = struct{}{}
			checkpointIDs = append(checkpointIDs, checkpointID)
		}
		sort.Strings(checkpointIDs)
		match.CheckpointIDs = checkpointIDs
		match.StartedAt = match.StartedAt.UTC()
	}
	sort.Slice(correlation.Matches, func(i, j int) bool {
		return correlation.Matches[i].TraceSessionID < correlation.Matches[j].TraceSessionID
	})
	return nil
}

func validateTraceSessionID(sessionID string) error {
	if sessionID == "" {
		return fmt.Errorf("validate Trace correlation: Trace session ID is empty")
	}
	if len(sessionID) > maxTraceSessionIDLength {
		return fmt.Errorf("validate Trace correlation: Trace session ID exceeds local limit")
	}
	if strings.ContainsAny(sessionID, `/\`) {
		return fmt.Errorf("validate Trace correlation: Trace session ID contains a path separator")
	}
	for _, value := range sessionID {
		if unicode.IsControl(value) {
			return fmt.Errorf("validate Trace correlation: Trace session ID contains a control character")
		}
	}
	return nil
}

func requireJSONEOF(decoder *json.Decoder) error {
	var trailing any
	if err := decoder.Decode(&trailing); !errors.Is(err, io.EOF) {
		if err == nil {
			return fmt.Errorf("decode Trace correlation: unexpected trailing JSON")
		}
		return fmt.Errorf("decode Trace correlation trailing data: %w", err)
	}
	return nil
}

type cappedBuffer struct {
	bytes.Buffer
	max int
}

func (b *cappedBuffer) Write(value []byte) (int, error) {
	remaining := b.max - b.Len()
	if remaining <= 0 {
		return 0, errTraceCorrelationOutputLimit
	}
	if len(value) > remaining {
		written, _ := b.Buffer.Write(value[:remaining])
		return written, errTraceCorrelationOutputLimit
	}
	return b.Buffer.Write(value)
}
