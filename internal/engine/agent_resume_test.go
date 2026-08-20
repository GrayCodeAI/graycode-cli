package engine

import (
	"testing"

	agentcontracts "github.com/GrayCodeAI/hawk-core-contracts/agent"
	"github.com/GrayCodeAI/hawk/internal/session"
	"github.com/GrayCodeAI/hawk/internal/storage"
	"github.com/GrayCodeAI/hawk/internal/tool"
)

func TestSubAgentResume_ReplaysTranscriptMessages(t *testing.T) {
	tempDir := t.TempDir()
	storage.SetTestDirs(t, tempDir)

	// 1. Create and save a prior session
	prior := &session.Session{
		ID:    "subagent-prior-123",
		Model: "test-model",
		Messages: []session.Message{
			{Role: "user", Content: "Find all auth handlers."},
			{Role: "assistant", Content: "Auth handlers are in internal/auth/handler.go."},
		},
	}
	if err := session.Save(prior); err != nil {
		t.Fatalf("failed to save prior session: %v", err)
	}

	// 2. Set up parent session
	reg := tool.NewRegistry()
	parent := NewSession("", "", "You are parent assistant", reg)

	req := agentcontracts.SpawnRequest{
		Prompt:       "Where is the token validation function?",
		SubagentType: "explore",
		ResumeFrom:   "subagent-prior-123",
	}

	norm, err := req.Normalize()
	if err != nil {
		t.Fatalf("Normalize failed: %v", err)
	}

	sub := parent.SubSession("", "", reg)
	if norm.ResumeFrom != "" {
		if priorSession, loadErr := session.Load(norm.ResumeFrom); loadErr == nil && priorSession != nil {
			for _, m := range priorSession.Messages {
				sub.Persistence().AddMessage(m.Role, m.Content)
			}
		}
	}
	sub.AddUser(norm.Prompt)

	// 3. Verify that the sub-session transcript has the restored prior messages
	msgs := sub.Persistence().Messages()
	if len(msgs) != 3 {
		t.Fatalf("expected 3 messages in sub-session transcript, got %d", len(msgs))
	}

	if msgs[0].Role != "user" || msgs[0].Content != "Find all auth handlers." {
		t.Errorf("msg[0] = %+v, want user 'Find all auth handlers.'", msgs[0])
	}
	if msgs[1].Role != "assistant" || msgs[1].Content != "Auth handlers are in internal/auth/handler.go." {
		t.Errorf("msg[1] = %+v, want assistant findings", msgs[1])
	}
	if msgs[2].Role != "user" || msgs[2].Content != "Where is the token validation function?" {
		t.Errorf("msg[2] = %+v, want user new prompt", msgs[2])
	}
}

func TestSubAgentResume_FallbackOnMissingSession(t *testing.T) {
	tempDir := t.TempDir()
	storage.SetTestDirs(t, tempDir)

	reg := tool.NewRegistry()
	parent := NewSession("", "", "You are parent assistant", reg)

	req := agentcontracts.SpawnRequest{
		Prompt:       "Continue analysis.",
		SubagentType: "explore",
		ResumeFrom:   "nonexistent-subagent-999",
	}

	norm, err := req.Normalize()
	if err != nil {
		t.Fatalf("Normalize failed: %v", err)
	}

	sub := parent.SubSession("", "", reg)
	prompt := norm.Prompt
	if norm.ResumeFrom != "" {
		if priorSession, loadErr := session.Load(norm.ResumeFrom); loadErr == nil && priorSession != nil {
			for _, m := range priorSession.Messages {
				sub.Persistence().AddMessage(m.Role, m.Content)
			}
		} else {
			prompt = "Resume prior subagent " + norm.ResumeFrom + ".\n\n" + prompt
		}
	}
	sub.AddUser(prompt)

	msgs := sub.Persistence().Messages()
	if len(msgs) != 1 {
		t.Fatalf("expected 1 message in fallback, got %d", len(msgs))
	}
	if msgs[0].Content != "Resume prior subagent nonexistent-subagent-999.\n\nContinue analysis." {
		t.Errorf("unexpected fallback prompt: %q", msgs[0].Content)
	}
}
