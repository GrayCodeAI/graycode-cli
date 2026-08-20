package mission

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/GrayCodeAI/hawk/internal/session"
)

type echoTestProvider struct {
	name string
}

func (e *echoTestProvider) Name() string {
	if e.name != "" {
		return e.name
	}
	return "echo-provider"
}

func (e *echoTestProvider) Capabilities() SubagentCapabilities {
	return SubagentCapabilities{
		SupportsStreaming: true,
		MaxDepth:          5,
	}
}

func (e *echoTestProvider) Run(ctx context.Context, req SubagentRequest) (*SubagentResult, error) {
	return &SubagentResult{
		Status: "success",
		Output: "echo: " + req.Task,
	}, nil
}

func TestContinuable_LifecycleAndFIFOOrder(t *testing.T) {
	reg := NewProviderRegistry()
	echoP := &echoTestProvider{name: "test-echo"}
	reg.Register(echoP)

	cm := NewContinuableManager(reg)
	ctx := context.Background()

	parentID := "parent-sess-100"
	pSess := &session.Session{
		ID:              parentID,
		DelegationDepth: 1,
		CreatedAt:       time.Now(),
		UpdatedAt:       time.Now(),
	}
	if err := session.Save(pSess); err != nil {
		t.Fatalf("save parent sess failed: %v", err)
	}

	// 1. Start Continuable
	spec := ContinuableSpec{
		ParentSessionID: parentID,
		Provider:        "test-echo",
		Label:           "Worker Subagent",
		Model:           "hawk-code-1",
		InitialPrompt:   "initial turn",
		Depth:           2,
	}

	act, initRes, err := cm.StartContinuable(ctx, spec)
	if err != nil {
		t.Fatalf("StartContinuable failed: %v", err)
	}
	if act == nil {
		t.Fatal("expected non-nil activation")
	}
	defer cm.Dispose(act.ChildID)

	if !strings.Contains(initRes.Output, "echo: initial turn") {
		t.Errorf("expected initial output to contain echo, got %q", initRes.Output)
	}

	// 2. FIFO Follow-up turns in sequence
	for i := 1; i <= 3; i++ {
		prompt := fmt.Sprintf("turn %d", i)
		res, err := cm.Followup(ctx, parentID, act.ChildID, prompt)
		if err != nil {
			t.Fatalf("Followup %d failed: %v", i, err)
		}
		expected := fmt.Sprintf("echo: turn %d", i)
		if !strings.Contains(res.Output, expected) {
			t.Errorf("expected output %q, got %q", expected, res.Output)
		}
	}
}

func TestContinuable_ColdResumeFromPersistedLog(t *testing.T) {
	reg := NewProviderRegistry()
	echoP := &echoTestProvider{name: "test-echo"}
	reg.Register(echoP)

	cm1 := NewContinuableManager(reg)
	ctx := context.Background()

	parentID := "parent-cold-1"
	spec := ContinuableSpec{
		ParentSessionID: parentID,
		Provider:        "test-echo",
		Label:           "Cold Resume Subagent",
		InitialPrompt:   "hello first turn",
		Depth:           2,
	}

	act1, initRes, err := cm1.StartContinuable(ctx, spec)
	if err != nil {
		t.Fatalf("StartContinuable failed: %v", err)
	}
	childID := act1.ChildID

	if !strings.Contains(initRes.Output, "hello first turn") {
		t.Fatalf("unexpected initRes: %v", initRes.Output)
	}

	// Simulate complete process restart / memory eviction:
	cm1.Dispose(childID)

	// cm2 is a brand new manager with empty memory
	cm2 := NewContinuableManager(reg)

	// Follow-up must trigger Cold Resume from the persisted log!
	followRes, err := cm2.Followup(ctx, parentID, childID, "resumed follow-up turn")
	if err != nil {
		t.Fatalf("Cold resume Followup failed: %v", err)
	}
	defer cm2.Dispose(childID)

	if !strings.Contains(followRes.Output, "echo: resumed follow-up turn") {
		t.Errorf("expected resumed echo output, got %q", followRes.Output)
	}

	// Verify the persisted session contains all messages
	loaded, err := session.Load(childID)
	if err != nil {
		t.Fatalf("session.Load failed: %v", err)
	}
	if len(loaded.Messages) < 4 {
		t.Errorf("expected at least 4 messages (2 user, 2 assistant), got %d", len(loaded.Messages))
	}
}

func TestContinuable_AuthorityRejection(t *testing.T) {
	reg := NewProviderRegistry()
	reg.Register(&echoTestProvider{name: "test-echo"})
	cm := NewContinuableManager(reg)
	ctx := context.Background()

	parentID := "authorized-parent"
	spec := ContinuableSpec{
		ParentSessionID: parentID,
		Provider:        "test-echo",
		Label:           "Guarded Subagent",
		InitialPrompt:   "guard check",
		Depth:           2,
	}

	act, _, err := cm.StartContinuable(ctx, spec)
	if err != nil {
		t.Fatalf("StartContinuable failed: %v", err)
	}
	defer cm.Dispose(act.ChildID)

	// 1. Live resident authority check: unauthorized caller must fail
	_, err = cm.Followup(ctx, "imposter-parent", act.ChildID, "unauthorized turn")
	if !errors.Is(err, ErrUnauthorizedCaller) {
		t.Fatalf("expected ErrUnauthorizedCaller, got %v", err)
	}

	// 2. Self-targeting authority check must fail
	_, err = cm.Followup(ctx, act.ChildID, act.ChildID, "self turn")
	if !errors.Is(err, ErrUnauthorizedCaller) {
		t.Fatalf("expected ErrUnauthorizedCaller on self-targeting, got %v", err)
	}

	// 3. Cold-resume authority check: evict from memory and attempt cold resume with imposter
	cm.Dispose(act.ChildID)

	cmCold := NewContinuableManager(reg)
	_, err = cmCold.Followup(ctx, "imposter-parent", act.ChildID, "cold unauthorized turn")
	if !errors.Is(err, ErrUnauthorizedCaller) {
		t.Fatalf("expected ErrUnauthorizedCaller on cold resume, got %v", err)
	}
}

func TestContinuable_ValidationAndMonotonicity(t *testing.T) {
	reg := NewProviderRegistry()
	cm := NewContinuableManager(reg)
	ctx := context.Background()

	parentID := "parent-monotonic"
	pSess := &session.Session{
		ID:              parentID,
		DelegationDepth: 3,
		CreatedAt:       time.Now(),
		UpdatedAt:       time.Now(),
	}
	_ = session.Save(pSess)

	// Missing parent session
	_, _, err := cm.StartContinuable(ctx, ContinuableSpec{Label: "Test"})
	if !errors.Is(err, ErrMissingParentSession) {
		t.Errorf("expected ErrMissingParentSession, got %v", err)
	}

	// Missing label
	_, _, err = cm.StartContinuable(ctx, ContinuableSpec{ParentSessionID: parentID, Label: ""})
	if !errors.Is(err, ErrContinuableLabelRequired) {
		t.Errorf("expected ErrContinuableLabelRequired, got %v", err)
	}

	// Depth violation (child depth <= parent depth)
	_, _, err = cm.StartContinuable(ctx, ContinuableSpec{
		ParentSessionID: parentID,
		Label:           "Violating Child",
		Depth:           3, // not strictly greater than parent's 3
	})
	if !errors.Is(err, ErrInvalidDelegationDepth) {
		t.Errorf("expected ErrInvalidDelegationDepth, got %v", err)
	}
}

func TestContinuable_ListChildrenAndDescendants(t *testing.T) {
	reg := NewProviderRegistry()
	reg.Register(&echoTestProvider{name: "test-echo"})
	cm := NewContinuableManager(reg)
	ctx := context.Background()

	rootID := fmt.Sprintf("root-session-list-%d", time.Now().UnixNano())
	child1Spec := ContinuableSpec{
		ParentSessionID: rootID,
		Provider:        "test-echo",
		Label:           "Child 1",
		InitialPrompt:   "c1",
		Depth:           2,
	}
	c1, _, err := cm.StartContinuable(ctx, child1Spec)
	if err != nil {
		t.Fatalf("c1 failed: %v", err)
	}
	defer func() {
		cm.Dispose(c1.ChildID)
		_ = cm.deletePersistedSession(c1.ChildID)
	}()

	child2Spec := ContinuableSpec{
		ParentSessionID: rootID,
		Provider:        "test-echo",
		Label:           "Child 2",
		InitialPrompt:   "c2",
		Depth:           2,
	}
	c2, _, err := cm.StartContinuable(ctx, child2Spec)
	if err != nil {
		t.Fatalf("c2 failed: %v", err)
	}
	defer func() {
		cm.Dispose(c2.ChildID)
		_ = cm.deletePersistedSession(c2.ChildID)
	}()

	// Grandchild under Child 1
	grandchildSpec := ContinuableSpec{
		ParentSessionID: c1.ChildID,
		Provider:        "test-echo",
		Label:           "Grandchild",
		InitialPrompt:   "gc",
		Depth:           3,
	}
	gc, _, err := cm.StartContinuable(ctx, grandchildSpec)
	if err != nil {
		t.Fatalf("gc failed: %v", err)
	}
	defer func() {
		cm.Dispose(gc.ChildID)
		_ = cm.deletePersistedSession(gc.ChildID)
	}()

	// Direct children of root
	children, err := cm.ListChildren(rootID)
	if err != nil {
		t.Fatalf("ListChildren failed: %v", err)
	}
	if len(children) != 2 {
		t.Errorf("expected 2 direct children, got %d", len(children))
	}

	// All descendants of root
	descendants, err := cm.ListDescendants(rootID)
	if err != nil {
		t.Fatalf("ListDescendants failed: %v", err)
	}
	if len(descendants) != 3 {
		t.Errorf("expected 3 descendants, got %d", len(descendants))
	}
}

func TestContinuable_DisposeIdempotence(t *testing.T) {
	reg := NewProviderRegistry()
	cm := NewContinuableManager(reg)
	ctx := context.Background()

	spec := ContinuableSpec{
		ParentSessionID: "parent-idemp",
		Provider:        "test-echo",
		Label:           "Idempotent Child",
		InitialPrompt:   "init",
		Depth:           2,
	}

	act, _, err := cm.StartContinuable(ctx, spec)
	if err != nil {
		t.Fatalf("start failed: %v", err)
	}

	// Multiple concurrent and sequential disposes
	var wg sync.WaitGroup
	for i := 0; i < 5; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			cm.Dispose(act.ChildID)
		}()
	}
	wg.Wait()
}
