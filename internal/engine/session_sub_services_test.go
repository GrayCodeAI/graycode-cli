package engine

import (
	"testing"
)

// TestSession_SubServices verifies that the SubServices() accessor
// returns the 6 sub-services and that each is the same instance as
// the per-service accessor (so a service-side state mutation is
// visible through both views). This is the canonical migration
// target for new code.
func TestSession_SubServices(t *testing.T) {
	t.Parallel()
	mc := newMockClient()
	s := newMockSession(mc)

	subs := s.SubServices()

	// All 6 sub-services must be non-nil for a session built via
	// NewSessionWithClient (the only production constructor).
	if subs.LLM == nil {
		t.Error("SubServices().LLM is nil; want *ChatService")
	}
	if subs.Perms == nil {
		t.Error("SubServices().Perms is nil; want *PermissionService")
	}
	if subs.Life == nil {
		t.Error("SubServices().Life is nil; want *LifecycleService")
	}
	if subs.Memory == nil {
		t.Error("SubServices().Memory is nil; want *MemoryService")
	}
	if subs.Persistence == nil {
		t.Error("SubServices().Persistence is nil; want *PersistenceService")
	}
	if subs.Tools == nil {
		t.Error("SubServices().Tools is nil; want *ToolService")
	}

	// Each sub-service must be the SAME INSTANCE as the per-service
	// accessor, so state mutations are visible through both views.
	if subs.LLM != s.ChatLLM() {
		t.Error("SubServices().LLM != s.ChatLLM() — sub-service is not the same instance")
	}
	if subs.Perms != s.PermSvc() {
		t.Error("SubServices().Perms != s.PermSvc() — sub-service is not the same instance")
	}
	if subs.Life != s.LifecycleSvc() {
		t.Error("SubServices().Life != s.LifecycleSvc() — sub-service is not the same instance")
	}
	if subs.Memory != s.MemorySvc() {
		t.Error("SubServices().Memory != s.MemorySvc() — sub-service is not the same instance")
	}
	if subs.Persistence != s.Persistence() {
		t.Error("SubServices().Persistence != s.Persistence() — sub-service is not the same instance")
	}
	if subs.Tools != s.Tools() {
		t.Error("SubServices().Tools != s.Tools() — sub-service is not the same instance")
	}
}
