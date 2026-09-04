package sessionquery

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/GrayCodeAI/graycode-cli/internal/session"
)

func createTestSessionFile(t *testing.T, dir, sessionID, workspace string, messages []session.Message) {
	t.Helper()
	sess := &session.Session{
		ID:        sessionID,
		Model:     "gpt-4o",
		Provider:  "openai",
		CWD:       workspace,
		Messages:  messages,
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}

	// We set test session dir environment so session.Save and session.Load work
	t.Setenv("GRAYCODE_STATE_DIR", dir)
	sessDir := filepath.Join(dir, "sessions")
	_ = os.MkdirAll(sessDir, 0o755)

	if err := session.Save(sess); err != nil {
		t.Fatalf("failed to save test session %s: %v", sessionID, err)
	}
}

func setupTestService(t *testing.T) (*Service, string, string) {
	t.Helper()
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test_session_query.db")
	sessDir := filepath.Join(tmpDir, "sessions")
	_ = os.MkdirAll(sessDir, 0o755)
	t.Setenv("GRAYCODE_STATE_DIR", tmpDir)

	svc, err := NewService(dbPath, sessDir)
	if err != nil {
		t.Fatalf("failed to create sessionquery service: %v", err)
	}
	t.Cleanup(func() {
		_ = svc.Close()
	})

	return svc, tmpDir, sessDir
}

func TestIndexingAndTokenization(t *testing.T) {
	svc, tmpDir, _ := setupTestService(t)
	ctx := context.Background()

	createTestSessionFile(t, tmpDir, "sess-1", "/projects/backend", []session.Message{
		{Role: "user", Content: "How do we deploy the microservice to Kubernetes cluster?"},
		{Role: "assistant", Content: "You can use kubectl apply with deployment.yaml."},
	})

	createTestSessionFile(t, tmpDir, "sess-2", "/projects/frontend", []session.Message{
		{Role: "user", Content: "The React button component is failing to render."},
		{Role: "assistant", Content: "Check your CSS imports and Vite config."},
	})

	// 1. Search for "Kubernetes"
	res, err := svc.Search(ctx, SearchParams{
		Query: "Kubernetes",
	})
	if err != nil {
		t.Fatalf("Search failed: %v", err)
	}
	if len(res.Matches) != 1 {
		t.Fatalf("expected 1 match for 'Kubernetes', got %d", len(res.Matches))
	}
	if res.Matches[0].SessionID != "sess-1" {
		t.Fatalf("expected match from sess-1, got %s", res.Matches[0].SessionID)
	}

	// 2. Stemming test: searching "deploying" should match "deploy" (Porter stemmer)
	resStem, err := svc.Search(ctx, SearchParams{
		Query: "deploying",
	})
	if err != nil {
		t.Fatalf("Search with stemming failed: %v", err)
	}
	if len(resStem.Matches) != 1 {
		t.Fatalf("expected stem match for 'deploying', got %d", len(resStem.Matches))
	}
	if resStem.Matches[0].SessionID != "sess-1" {
		t.Fatalf("expected match from sess-1, got %s", resStem.Matches[0].SessionID)
	}
}

func TestWorkspaceAuthorization(t *testing.T) {
	svc, tmpDir, _ := setupTestService(t)
	ctx := context.Background()

	createTestSessionFile(t, tmpDir, "sess-auth-1", "/workspaces/team-a", []session.Message{
		{Role: "user", Content: "Configuring Redis cache cluster for team A."},
	})

	createTestSessionFile(t, tmpDir, "sess-auth-2", "/workspaces/team-b", []session.Message{
		{Role: "user", Content: "Configuring Redis cache cluster for team B."},
	})

	// Sync index
	_, _ = svc.Indexer().SyncAll(ctx)

	// Caller in /workspaces/team-a searching without sessionID
	resA, err := svc.Search(ctx, SearchParams{
		CallerWorkspace: "/workspaces/team-a",
		Query:           "Redis",
	})
	if err != nil {
		t.Fatalf("Search team-a failed: %v", err)
	}
	if len(resA.Matches) != 1 || resA.Matches[0].SessionID != "sess-auth-1" {
		t.Fatalf("expected only team-a session, got %#v", resA.Matches)
	}

	// Caller in /workspaces/team-a attempting to target team-b's sessionID -> ErrUnauthorized
	_, err = svc.Search(ctx, SearchParams{
		CallerWorkspace: "/workspaces/team-a",
		SessionID:       "sess-auth-2",
		Query:           "Redis",
	})
	if err == nil {
		t.Fatal("expected ErrUnauthorized when querying unauthorized session, got nil")
	}
	if !errorsIs(err, ErrUnauthorized) {
		t.Fatalf("expected ErrUnauthorized, got %v", err)
	}
}

func TestSecretRedaction(t *testing.T) {
	svc, tmpDir, _ := setupTestService(t)
	ctx := context.Background()

	secretKey := "sk-ant-api03-abcdef12345678901234567890abcdef12345678"
	createTestSessionFile(t, tmpDir, "sess-secret", "/workspaces/app", []session.Message{
		{Role: "user", Content: "Connecting to provider using secret key " + secretKey},
	})

	res, err := svc.Search(ctx, SearchParams{
		Query: "secret key",
	})
	if err != nil {
		t.Fatalf("Search failed: %v", err)
	}
	if len(res.Matches) == 0 {
		t.Fatal("expected match for 'secret key'")
	}

	match := res.Matches[0]
	if strings.Contains(match.Content, secretKey) {
		t.Errorf("secret was not redacted in Content: %s", match.Content)
	}
	if strings.Contains(match.Snippet, secretKey) {
		t.Errorf("secret was not redacted in Snippet: %s", match.Snippet)
	}
	if !strings.Contains(match.Content, "[REDACTED") {
		t.Errorf("expected [REDACTED] in Content: %s", match.Content)
	}
}

func TestPagingAndByteBounding(t *testing.T) {
	svc, tmpDir, _ := setupTestService(t)
	ctx := context.Background()

	var msgs []session.Message
	for i := 0; i < 20; i++ {
		msgs = append(msgs, session.Message{
			Role:    "user",
			Content: "Database migration step and schema updates for transaction processing.",
		})
	}
	createTestSessionFile(t, tmpDir, "sess-paged", "/workspaces/app", msgs)

	// Fetch page 1 (limit 5)
	p1, err := svc.Search(ctx, SearchParams{
		Query:  "transaction processing",
		Limit:  5,
		Offset: 0,
	})
	if err != nil {
		t.Fatalf("page 1 search failed: %v", err)
	}
	if len(p1.Matches) != 5 {
		t.Fatalf("expected 5 matches on page 1, got %d", len(p1.Matches))
	}
	if !p1.HasMore {
		t.Fatal("expected HasMore=true on page 1")
	}

	// Fetch page 2 (limit 5, offset 5)
	p2, err := svc.Search(ctx, SearchParams{
		Query:  "transaction processing",
		Limit:  5,
		Offset: 5,
	})
	if err != nil {
		t.Fatalf("page 2 search failed: %v", err)
	}
	if len(p2.Matches) != 5 {
		t.Fatalf("expected 5 matches on page 2, got %d", len(p2.Matches))
	}
	if p2.Matches[0].MsgIndex != p1.Matches[4].MsgIndex+1 {
		t.Errorf("expected sequential message indexing across pages")
	}
}

func TestIndexRebuildFromScratch(t *testing.T) {
	svc, tmpDir, _ := setupTestService(t)
	ctx := context.Background()

	createTestSessionFile(t, tmpDir, "sess-rebuild", "/workspaces/app", []session.Message{
		{Role: "user", Content: "Initial indexing test data."},
	})

	_, err := svc.Indexer().SyncAll(ctx)
	if err != nil {
		t.Fatalf("SyncAll failed: %v", err)
	}

	// Rebuild index
	if err := svc.RebuildIndex(ctx); err != nil {
		t.Fatalf("RebuildIndex failed: %v", err)
	}

	res, err := svc.Search(ctx, SearchParams{Query: "indexing"})
	if err != nil {
		t.Fatalf("Search after rebuild failed: %v", err)
	}
	if len(res.Matches) != 1 || res.Matches[0].SessionID != "sess-rebuild" {
		t.Fatalf("expected sess-rebuild after index rebuild, got %#v", res.Matches)
	}
}

func TestIncrementalTailAfterSessionRewrite(t *testing.T) {
	svc, tmpDir, _ := setupTestService(t)
	ctx := context.Background()

	createTestSessionFile(t, tmpDir, "sess-rewrite", "/workspaces/app", []session.Message{
		{Role: "user", Content: "Version 1 message about architecture."},
	})

	_, _ = svc.Indexer().SyncAll(ctx)

	// Verify v1 is found
	res1, _ := svc.Search(ctx, SearchParams{Query: "architecture"})
	if len(res1.Matches) != 1 {
		t.Fatalf("expected 1 match for architecture in v1")
	}

	// Rewrite session with new message
	time.Sleep(10 * time.Millisecond) // Ensure modTime changes
	createTestSessionFile(t, tmpDir, "sess-rewrite", "/workspaces/app", []session.Message{
		{Role: "user", Content: "Version 2 message about telemetry and distributed tracing."},
	})

	// Search for "telemetry" -> should trigger incremental sync and match immediately
	res2, err := svc.Search(ctx, SearchParams{Query: "telemetry"})
	if err != nil {
		t.Fatalf("Search v2 failed: %v", err)
	}
	if len(res2.Matches) != 1 || res2.Matches[0].SessionID != "sess-rewrite" {
		t.Fatalf("expected match for rewritten telemetry message, got %#v", res2.Matches)
	}

	// Old term should no longer match
	resOld, _ := svc.Search(ctx, SearchParams{Query: "architecture"})
	if len(resOld.Matches) != 0 {
		t.Fatalf("expected 0 matches for old content after rewrite, got %d", len(resOld.Matches))
	}
}

func errorsIs(err, target error) bool {
	if err == target {
		return true
	}
	return strings.Contains(err.Error(), target.Error())
}
