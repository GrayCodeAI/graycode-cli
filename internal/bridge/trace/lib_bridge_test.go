package tracebridge

import (
	"context"
	"testing"
	"time"
)

// --- mock types for tests ---

type mockSession struct {
	id             string
	phase          string
	transcriptPath string
	spanCount      int
	startedAt      time.Time
	endedAt        *time.Time
	metadata       map[string]string
}

func (s *mockSession) ID() string                { return s.id }
func (s *mockSession) Phase() string              { return s.phase }
func (s *mockSession) TranscriptPath() string     { return s.transcriptPath }
func (s *mockSession) SpanCount() int             { return s.spanCount }
func (s *mockSession) StartedAt() time.Time       { return s.startedAt }
func (s *mockSession) EndedAt() *time.Time        { return s.endedAt }
func (s *mockSession) Close(_ context.Context) error { return nil }

func (s *mockSession) Metadata() map[string]string {
	if s.metadata == nil {
		return map[string]string{}
	}
	cp := make(map[string]string, len(s.metadata))
	for k, v := range s.metadata {
		cp[k] = v
	}
	return cp
}

func (s *mockSession) SetMetadata(key, value string) {
	if s.metadata == nil {
		s.metadata = make(map[string]string)
	}
	s.metadata[key] = value
}

type mockSessionManager struct {
	startFunc func(ctx context.Context, repoPath, sessionID string) (TraceSession, error)
	loadFunc  func(ctx context.Context, sessionID string) (TraceSession, error)
}

func (m *mockSessionManager) Start(ctx context.Context, repoPath, sessionID string) (TraceSession, error) {
	if m.startFunc != nil {
		return m.startFunc(ctx, repoPath, sessionID)
	}
	return &mockSession{id: "mock-session", phase: "active", transcriptPath: "/tmp/mock.jsonl"}, nil
}

func (m *mockSessionManager) Load(ctx context.Context, sessionID string) (TraceSession, error) {
	if m.loadFunc != nil {
		return m.loadFunc(ctx, sessionID)
	}
	return &mockSession{id: sessionID}, nil
}

// --- tests ---

// Test 1: NewSessionCapture initializes with config
func TestNewSessionCapture_InitializesWithConfig(t *testing.T) {
	config := CaptureConfig{
		RepoPath:  "/repo",
		SessionID: "test-session",
		Tags:      map[string]string{"env": "dev"},
	}
	sc := NewSessionCapture(config, nil)
	if sc == nil {
		t.Fatal("NewSessionCapture returned nil")
	}
	if sc.config.RepoPath != "/repo" {
		t.Errorf("RepoPath = %q, want %q", sc.config.RepoPath, "/repo")
	}
	if sc.config.SessionID != "test-session" {
		t.Errorf("SessionID = %q, want %q", sc.config.SessionID, "test-session")
	}
	if v, ok := sc.config.Tags["env"]; !ok || v != "dev" {
		t.Errorf("Tags[env] = %q (ok=%v), want %q", v, ok, "dev")
	}
}

// Test 2: AddTag stores tags (after capture is started)
func TestAddTag_StoresTags(t *testing.T) {
	sess := &mockSession{id: "tag-session", phase: "active", transcriptPath: "/tmp/tag.jsonl"}
	mgr := &mockSessionManager{
		startFunc: func(_ context.Context, _, _ string) (TraceSession, error) {
			return sess, nil
		},
	}
	sc := NewSessionCapture(CaptureConfig{RepoPath: "/repo"}, mgr)

	if err := sc.StartCapture(context.Background()); err != nil {
		t.Fatalf("StartCapture: %v", err)
	}

	sc.AddTag("env", "production")

	meta := sess.Metadata()
	if meta["env"] != "production" {
		t.Errorf("metadata[env] = %q, want %q", meta["env"], "production")
	}
}

// Test 3: IsActive starts false, becomes true after StartCapture
func TestIsActive_BecomesTrueAfterStart(t *testing.T) {
	sess := &mockSession{id: "active-session", phase: "active", transcriptPath: "/tmp/active.jsonl"}
	mgr := &mockSessionManager{
		startFunc: func(_ context.Context, _, _ string) (TraceSession, error) {
			return sess, nil
		},
	}
	sc := NewSessionCapture(CaptureConfig{RepoPath: "/repo"}, mgr)

	if sc.IsActive() {
		t.Fatal("expected IsActive() == false before StartCapture")
	}

	if err := sc.StartCapture(context.Background()); err != nil {
		t.Fatalf("StartCapture: %v", err)
	}

	if !sc.IsActive() {
		t.Fatal("expected IsActive() == true after StartCapture")
	}
}

// Test 4: GetTranscriptPath returns empty before capture
func TestGetTranscriptPath_EmptyBeforeCapture(t *testing.T) {
	sc := NewSessionCapture(CaptureConfig{RepoPath: "/repo"}, nil)
	if path := sc.GetTranscriptPath(); path != "" {
		t.Errorf("GetTranscriptPath() = %q, want empty string before capture", path)
	}
}

// Test 5: StopCapture without start returns error
func TestStopCapture_WithoutStart_ReturnsError(t *testing.T) {
	sc := NewSessionCapture(CaptureConfig{RepoPath: "/repo"}, nil)
	result, err := sc.StopCapture(context.Background())
	if err != ErrNotActive {
		t.Errorf("StopCapture error = %v, want ErrNotActive", err)
	}
	if result != nil {
		t.Errorf("StopCapture result = %v, want nil", result)
	}
}

// Test 6: Multiple tags can be added
func TestAddTag_MultipleTags(t *testing.T) {
	sess := &mockSession{id: "multi-tag", phase: "active", transcriptPath: "/tmp/multi.jsonl"}
	mgr := &mockSessionManager{
		startFunc: func(_ context.Context, _, _ string) (TraceSession, error) {
			return sess, nil
		},
	}
	sc := NewSessionCapture(CaptureConfig{RepoPath: "/repo"}, mgr)

	if err := sc.StartCapture(context.Background()); err != nil {
		t.Fatalf("StartCapture: %v", err)
	}

	sc.AddTag("env", "staging")
	sc.AddTag("region", "us-east-1")
	sc.AddTag("team", "backend")

	meta := sess.Metadata()
	if meta["env"] != "staging" {
		t.Errorf("metadata[env] = %q, want %q", meta["env"], "staging")
	}
	if meta["region"] != "us-east-1" {
		t.Errorf("metadata[region] = %q, want %q", meta["region"], "us-east-1")
	}
	if meta["team"] != "backend" {
		t.Errorf("metadata[team] = %q, want %q", meta["team"], "backend")
	}
}

// Test 7: SubprocessBridgeAdapter initializes correctly
func TestSubprocessBridgeAdapter_InitializesCorrectly(t *testing.T) {
	adapter := NewSubprocessBridgeAdapter("/repo")
	if adapter == nil {
		t.Fatal("NewSubprocessBridgeAdapter returned nil")
	}
	if adapter.capture == nil {
		t.Fatal("SubprocessBridgeAdapter.capture is nil")
	}
}
