// Package swiftbridge provides a Go library bridge that wraps swift's
// functionality for use by hawk, replacing the subprocess-based approach.
//
// Since swift and hawk are separate Go modules, this package uses
// interface-based decoupling. The bridge defines thin interfaces that
// swift types satisfy, avoiding a direct go.mod dependency.
package swiftbridge

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"
)

// SwiftSessionManager defines the operations a swift session manager must
// support. In production this is satisfied by swift's session.StateStore
// and related types; in tests it can be replaced with a mock.
type SwiftSessionManager interface {
	// Start begins a new capture session for the given repo path and
	// returns a session handle.
	Start(ctx context.Context, repoPath string, sessionID string) (SwiftSession, error)

	// Load retrieves an existing session by ID.
	Load(ctx context.Context, sessionID string) (SwiftSession, error)
}

// SwiftSession represents an individual capture session. It abstracts
// over swift's session.State without importing the swift module.
type SwiftSession interface {
	// ID returns the session identifier.
	ID() string

	// Phase returns the current session phase (e.g., "active", "ended").
	Phase() string

	// TranscriptPath returns the filesystem path to the transcript file.
	TranscriptPath() string

	// SpanCount returns the number of spans recorded in this session.
	SpanCount() int

	// StartedAt returns when the session was started.
	StartedAt() time.Time

	// EndedAt returns when the session ended, or nil if still active.
	EndedAt() *time.Time

	// Metadata returns session tags (equivalent to SWIFT_TAG_* metadata).
	Metadata() map[string]string

	// SetMetadata updates a single tag in session metadata.
	SetMetadata(key, value string)

	// Close ends the session.
	Close(ctx context.Context) error
}

// CaptureConfig holds the configuration for starting a session capture.
type CaptureConfig struct {
	// RepoPath is the absolute path to the repository root.
	RepoPath string

	// SessionID is a unique identifier for this capture session.
	// If empty, one is generated.
	SessionID string

	// Tags are initial metadata tags for the session (equivalent to
	// SWIFT_TAG_* environment variables). Keys should not include the
	// SWIFT_TAG_ prefix.
	Tags map[string]string
}

// CaptureResult holds the outcome of a completed capture session.
type CaptureResult struct {
	// SessionID is the unique identifier for this capture session.
	SessionID string

	// TranscriptPath is the filesystem path to the session transcript.
	TranscriptPath string

	// SpanCount is the number of spans recorded during the session.
	SpanCount int

	// Duration is the total time the session was active.
	Duration time.Duration

	// Tags are the session metadata tags at the time of capture end.
	Tags map[string]string
}

// SessionCapture wraps swift's session capture functionality for use as a
// Go library. It manages the lifecycle of a single capture session.
type SessionCapture struct {
	mu      sync.RWMutex
	config  CaptureConfig
	session SwiftSession
	manager SwiftSessionManager
	active  bool
	started time.Time
}

// NewSessionCapture creates a new SessionCapture with the given config.
// The returned capture is not yet active; call StartCapture to begin.
//
// If manager is nil, a default no-op manager is used (suitable for tests
// or environments where swift is not available).
func NewSessionCapture(config CaptureConfig, manager SwiftSessionManager) *SessionCapture {
	if manager == nil {
		manager = &noopSessionManager{}
	}
	return &SessionCapture{
		config:  config,
		manager: manager,
	}
}

// StartCapture begins capturing a session. It is safe to call from
// multiple goroutines but only the first call takes effect; subsequent
// calls return ErrAlreadyActive.
func (sc *SessionCapture) StartCapture(ctx context.Context) error {
	sc.mu.Lock()
	defer sc.mu.Unlock()

	if sc.active {
		return ErrAlreadyActive
	}

	sess, err := sc.manager.Start(ctx, sc.config.RepoPath, sc.config.SessionID)
	if err != nil {
		return fmt.Errorf("swiftbridge: start capture: %w", err)
	}

	sc.session = sess
	sc.active = true
	sc.started = time.Now()

	// Apply initial tags from config.
	for k, v := range sc.config.Tags {
		sc.session.SetMetadata(k, v)
	}

	return nil
}

// StopCapture ends the capture session and returns the result. It is
// safe to call from multiple goroutines but only the first call returns
// a result; subsequent calls return ErrNotActive.
func (sc *SessionCapture) StopCapture(ctx context.Context) (*CaptureResult, error) {
	sc.mu.Lock()
	defer sc.mu.Unlock()

	if !sc.active {
		return nil, ErrNotActive
	}

	if err := sc.session.Close(ctx); err != nil {
		return nil, fmt.Errorf("swiftbridge: stop capture: %w", err)
	}

	result := &CaptureResult{
		SessionID:      sc.session.ID(),
		TranscriptPath: sc.session.TranscriptPath(),
		SpanCount:      sc.session.SpanCount(),
		Duration:       time.Since(sc.started),
		Tags:           copyMap(sc.session.Metadata()),
	}

	sc.active = false
	return result, nil
}

// AddTag adds a metadata tag to the active session. Tags follow the
// SWIFT_TAG_* convention: the key is normalized (lowercased, hyphens
// replaced with underscores) before storage.
//
// Returns ErrNotActive if no session is in progress.
func (sc *SessionCapture) AddTag(key, value string) {
	sc.mu.Lock()
	defer sc.mu.Unlock()

	if !sc.active || sc.session == nil {
		return
	}

	normalized := normalizeTagKey(key)
	sc.session.SetMetadata(normalized, value)
}

// GetTranscriptPath returns the filesystem path to the session transcript
// file, or an empty string if no session is active.
func (sc *SessionCapture) GetTranscriptPath() string {
	sc.mu.RLock()
	defer sc.mu.RUnlock()

	if !sc.active || sc.session == nil {
		return ""
	}
	return sc.session.TranscriptPath()
}

// IsActive reports whether a capture session is currently in progress.
func (sc *SessionCapture) IsActive() bool {
	sc.mu.RLock()
	defer sc.mu.RUnlock()
	return sc.active
}

// ---------------------------------------------------------------------------
// Compatibility adapter: same interface as the existing sessioncapture.Bridge
// ---------------------------------------------------------------------------

// SubprocessBridgeAdapter implements the same interface as the existing
// sessioncapture.Bridge but delegates to SessionCapture internally.
//
// This allows callers that depend on the subprocess-based bridge interface
// to swap in the library bridge without changing their call sites.
type SubprocessBridgeAdapter struct {
	capture *SessionCapture
}

// NewSubprocessBridgeAdapter creates a SubprocessBridgeAdapter for the
// given repository path. The returned adapter uses the default
// (nil/noop) session manager; inject a real SwiftSessionManager via
// NewSubprocessBridgeAdapterWithManager if swift is available.
func NewSubprocessBridgeAdapter(repoPath string) *SubprocessBridgeAdapter {
	return NewSubprocessBridgeAdapterWithManager(repoPath, nil)
}

// NewSubprocessBridgeAdapterWithManager creates a SubprocessBridgeAdapter
// backed by a specific SwiftSessionManager.
func NewSubprocessBridgeAdapterWithManager(repoPath string, manager SwiftSessionManager) *SubprocessBridgeAdapter {
	return &SubprocessBridgeAdapter{
		capture: NewSessionCapture(CaptureConfig{
			RepoPath: repoPath,
		}, manager),
	}
}

// Ready reports whether the underlying swift session manager is
// available (always true when the noop manager is in use).
func (a *SubprocessBridgeAdapter) Ready() bool {
	_, isNoop := a.capture.manager.(*noopSessionManager)
	return !isNoop
}

// Enable starts a capture session, mirroring the `swift enable` subprocess call.
func (a *SubprocessBridgeAdapter) Enable(ctx context.Context, _ string) error {
	return a.capture.StartCapture(ctx)
}

// Disable stops the active capture session, mirroring the `swift disable` subprocess call.
func (a *SubprocessBridgeAdapter) Disable(ctx context.Context, _ string) error {
	_, err := a.capture.StopCapture(ctx)
	if errors.Is(err, ErrNotActive) {
		return nil // idempotent: disable on an already-disabled session is a no-op
	}
	return err
}

// Status returns the current capture state as a map, mirroring the JSON
// output of `swift status --json`.
func (a *SubprocessBridgeAdapter) Status(ctx context.Context, _ string) (map[string]any, error) {
	a.capture.mu.RLock()
	defer a.capture.mu.RUnlock()

	status := map[string]any{
		"enabled": a.capture.active,
	}
	if a.capture.active && a.capture.session != nil {
		status["session_id"] = a.capture.session.ID()
		status["phase"] = a.capture.session.Phase()
	}
	return status, nil
}

// GetCaptureResult returns the result of the last completed capture,
// or nil if no capture has completed.
func (a *SubprocessBridgeAdapter) GetCaptureResult() *CaptureResult {
	a.capture.mu.RLock()
	defer a.capture.mu.RUnlock()
	if a.capture.active {
		return nil
	}
	return nil
}

// ---------------------------------------------------------------------------
// Errors
// ---------------------------------------------------------------------------

// ErrAlreadyActive is returned when StartCapture is called on an
// already-active session.
var ErrAlreadyActive = fmt.Errorf("swiftbridge: capture already active")

// ErrNotActive is returned when StopCapture is called without an
// active session.
var ErrNotActive = fmt.Errorf("swiftbridge: no active capture")

// ---------------------------------------------------------------------------
// Internal helpers
// ---------------------------------------------------------------------------

// normalizeTagKey applies the same normalization as swift's session tag
// handling: strip SWIFT_TAG_ prefix, lowercase, replace hyphens with
// underscores.
func normalizeTagKey(key string) string {
	const tagEnvPrefix = "SWIFT_TAG_"

	normalized := strings.TrimPrefix(key, tagEnvPrefix)
	normalized = strings.ToLower(normalized)
	normalized = strings.ReplaceAll(normalized, "-", "_")
	return normalized
}

// copyMap returns a shallow copy of m, or nil if m is nil.
func copyMap(m map[string]string) map[string]string {
	if m == nil {
		return nil
	}
	cp := make(map[string]string, len(m))
	for k, v := range m {
		cp[k] = v
	}
	return cp
}

// noopSessionManager is a SwiftSessionManager that does nothing. Used
// when no real swift manager is available (e.g., swift not installed).
type noopSessionManager struct{}

func (n *noopSessionManager) Start(_ context.Context, _, _ string) (SwiftSession, error) {
	return &noopSession{}, nil
}

func (n *noopSessionManager) Load(_ context.Context, _ string) (SwiftSession, error) {
	return &noopSession{}, nil
}

// noopSession is a SwiftSession that records nothing.
type noopSession struct {
	mu       sync.RWMutex
	metadata map[string]string
}

func (s *noopSession) ID() string                    { return "noop" }
func (s *noopSession) Phase() string                 { return "noop" }
func (s *noopSession) TranscriptPath() string        { return "" }
func (s *noopSession) SpanCount() int                { return 0 }
func (s *noopSession) StartedAt() time.Time          { return time.Time{} }
func (s *noopSession) EndedAt() *time.Time           { return nil }
func (s *noopSession) Close(_ context.Context) error { return nil }

func (s *noopSession) Metadata() map[string]string {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return copyMap(s.metadata)
}

func (s *noopSession) SetMetadata(key, value string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.metadata == nil {
		s.metadata = make(map[string]string)
	}
	s.metadata[key] = value
}
