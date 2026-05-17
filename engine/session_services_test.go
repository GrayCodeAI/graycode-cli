package engine

import (
	"testing"

	"github.com/GrayCodeAI/hawk/memory"
	"github.com/GrayCodeAI/hawk/metrics"
	"github.com/GrayCodeAI/hawk/tool"
	"github.com/GrayCodeAI/hawk/oteltrace"
)

// ---------------------------------------------------------------------------
// NewSessionServices tests
// ---------------------------------------------------------------------------

func TestNewSessionServices_Defaults(t *testing.T) {
	ss := NewSessionServices()

	if ss.Core == nil {
		t.Fatal("Core should not be nil with defaults")
	}
	if ss.Core.APIKeys == nil {
		t.Error("Core.APIKeys should be initialized")
	}
	if ss.Core.Log == nil {
		t.Error("Core.Log should default to logger.Default()")
	}

	if ss.Safety == nil {
		t.Fatal("Safety should not be nil with defaults")
	}
	if ss.Safety.Perm == nil {
		t.Error("Safety.Perm should be initialized")
	}
	if ss.Safety.Limits == nil {
		t.Error("Safety.Limits should be initialized")
	}

	if ss.Intel == nil {
		t.Fatal("Intel should not be nil with defaults")
	}
	if ss.Intel.Beliefs == nil {
		t.Error("Intel.Beliefs should be initialized")
	}

	if ss.Optim == nil {
		t.Fatal("Optim should not be nil with defaults")
	}
	if ss.Optim.Router == nil {
		t.Error("Optim.Router should be initialized")
	}

	if ss.Observe == nil {
		t.Fatal("Observe should not be nil with defaults")
	}
	if ss.Observe.Tracer == nil {
		t.Error("Observe.Tracer should be initialized")
	}
	if ss.Observe.Metrics == nil {
		t.Error("Observe.Metrics should be initialized")
	}

	if ss.Backtrack == nil {
		t.Error("Backtrack should be initialized with defaults")
	}
}

func TestNewSessionServices_WithProvider(t *testing.T) {
	ss := NewSessionServices(
		WithProvider("anthropic", "claude-opus-4-20250514"),
	)

	if ss.Core.Provider != "anthropic" {
		t.Errorf("expected provider 'anthropic', got %q", ss.Core.Provider)
	}
	if ss.Core.Model != "claude-opus-4-20250514" {
		t.Errorf("expected model 'claude-opus-4-20250514', got %q", ss.Core.Model)
	}
}

func TestNewSessionServices_WithTools(t *testing.T) {
	reg := tool.NewRegistry()
	ss := NewSessionServices(WithTools(reg))

	if ss.Core.Registry != reg {
		t.Error("expected registry to be set on Core")
	}
}

func TestNewSessionServices_WithMemory(t *testing.T) {
	mem := &mockMemoryRecaller{}
	ss := NewSessionServices(WithMemory(mem))

	if ss.Intel.Memory != mem {
		t.Error("expected memory to be set on Intel")
	}
}

func TestNewSessionServices_WithSandbox(t *testing.T) {
	sb := &DiffSandbox{}
	ss := NewSessionServices(WithSandbox(sb))

	if ss.Safety.Sandbox != sb {
		t.Error("expected sandbox to be set on Safety")
	}
}

func TestNewSessionServices_WithTracing(t *testing.T) {
	tracer := oteltrace.NewTracer()
	ss := NewSessionServices(WithTracing(tracer))

	if ss.Observe.Tracer != tracer {
		t.Error("expected tracer to be set on Observe")
	}
}

func TestNewSessionServices_WithCascade(t *testing.T) {
	cascade := &CascadeRouter{Enabled: true}
	ss := NewSessionServices(WithCascade(cascade))

	if ss.Optim.Cascade != cascade {
		t.Error("expected cascade to be set on Optim")
	}
	if !ss.Optim.Cascade.Enabled {
		t.Error("expected cascade.Enabled to be true")
	}
}

func TestNewSessionServices_WithMaxBudget(t *testing.T) {
	ss := NewSessionServices(WithMaxBudget(5.0))

	if ss.Optim.MaxBudget != 5.0 {
		t.Errorf("expected MaxBudget 5.0, got %f", ss.Optim.MaxBudget)
	}
}

func TestNewSessionServices_MultipleOptions(t *testing.T) {
	reg := tool.NewRegistry()
	mem := &mockMemoryRecaller{}
	tracer := oteltrace.NewTracer()

	ss := NewSessionServices(
		WithProvider("openai", "gpt-4o"),
		WithTools(reg),
		WithMemory(mem),
		WithTracing(tracer),
		WithMaxBudget(10.0),
	)

	if ss.Core.Provider != "openai" {
		t.Errorf("expected provider 'openai', got %q", ss.Core.Provider)
	}
	if ss.Core.Model != "gpt-4o" {
		t.Errorf("expected model 'gpt-4o', got %q", ss.Core.Model)
	}
	if ss.Core.Registry != reg {
		t.Error("expected registry set")
	}
	if ss.Intel.Memory != mem {
		t.Error("expected memory set")
	}
	if ss.Observe.Tracer != tracer {
		t.Error("expected tracer set")
	}
	if ss.Optim.MaxBudget != 10.0 {
		t.Errorf("expected MaxBudget 10.0, got %f", ss.Optim.MaxBudget)
	}
}

// ---------------------------------------------------------------------------
// Services() bridge tests
// ---------------------------------------------------------------------------

func TestSession_Services_Bridge(t *testing.T) {
	reg := tool.NewRegistry()
	s := NewSession("anthropic", "claude-sonnet-4-20250514", "You are helpful.", reg)
	s.MaxBudgetUSD = 3.50
	s.Autonomy = AutonomyFull
	s.Sandbox = &DiffSandbox{}
	s.Memory = &mockMemoryRecaller{}
	s.YaadBridge = &memory.YaadBridge{}
	s.Cascade = &CascadeRouter{Enabled: true}

	svc := s.Services()

	// Core mappings
	if svc.Core.Provider != "anthropic" {
		t.Errorf("Core.Provider: expected 'anthropic', got %q", svc.Core.Provider)
	}
	if svc.Core.Model != "claude-sonnet-4-20250514" {
		t.Errorf("Core.Model: expected 'claude-sonnet-4-20250514', got %q", svc.Core.Model)
	}
	if svc.Core.System != "You are helpful." {
		t.Errorf("Core.System: expected system prompt, got %q", svc.Core.System)
	}
	if svc.Core.Registry != reg {
		t.Error("Core.Registry should reference same registry")
	}

	// Safety mappings
	if svc.Safety.Perm != s.Perm {
		t.Error("Safety.Perm should reference same PermissionEngine")
	}
	if svc.Safety.Sandbox != s.Sandbox {
		t.Error("Safety.Sandbox should reference same DiffSandbox")
	}
	if svc.Safety.Limits != s.Limits {
		t.Error("Safety.Limits should reference same LimitTracker")
	}
	if svc.Safety.Autonomy != AutonomyFull {
		t.Error("Safety.Autonomy should be AutonomyFull")
	}

	// Intel mappings
	if svc.Intel.Beliefs != s.Beliefs {
		t.Error("Intel.Beliefs should reference same BeliefState")
	}
	if svc.Intel.Memory != s.Memory {
		t.Error("Intel.Memory should reference same MemoryRecaller")
	}
	if svc.Intel.YaadBridge != s.YaadBridge {
		t.Error("Intel.YaadBridge should reference same YaadBridge")
	}

	// Optim mappings
	if svc.Optim.MaxBudget != 3.50 {
		t.Errorf("Optim.MaxBudget: expected 3.50, got %f", svc.Optim.MaxBudget)
	}
	if svc.Optim.Router != s.Router {
		t.Error("Optim.Router should reference same Router")
	}
	if svc.Optim.Cascade != s.Cascade {
		t.Error("Optim.Cascade should reference same CascadeRouter")
	}

	// Observe mappings
	if svc.Observe.Tracer != s.Tracer {
		t.Error("Observe.Tracer should reference same Tracer")
	}
	if svc.Observe.Metrics != s.Metrics() {
		t.Error("Observe.Metrics should reference same metrics.Registry")
	}

	// Advanced features
	if svc.Backtrack != s.Backtrack {
		t.Error("Backtrack should reference same engine")
	}
}

func TestSession_Services_NilAdvancedFeatures(t *testing.T) {
	s := NewSession("openai", "gpt-4o", "test", tool.NewRegistry())
	svc := s.Services()

	// These are optional and should be nil when not configured
	if svc.Lifecycle != nil {
		t.Error("Lifecycle should be nil when not set")
	}
	if svc.Reflector != nil {
		t.Error("Reflector should be nil when not set")
	}
	if svc.Critic != nil {
		t.Error("Critic should be nil when not set")
	}
	if svc.Shadow != nil {
		t.Error("Shadow should be nil when not set")
	}
	if svc.ConvoDAG != nil {
		t.Error("ConvoDAG should be nil when not set")
	}
	if svc.Plan != nil {
		t.Error("Plan should be nil when not set")
	}
	if svc.Snapshots != nil {
		t.Error("Snapshots should be nil when not set")
	}
}

// ---------------------------------------------------------------------------
// Nil-safety tests: sub-services handle nil gracefully
// ---------------------------------------------------------------------------

func TestSafetyLayer_NilSafe(t *testing.T) {
	var sl *SafetyLayer

	// Calling IsPermitted on nil SafetyLayer should not panic
	if sl.IsPermitted("write") {
		t.Error("nil SafetyLayer should deny permissions")
	}

	// Non-nil SafetyLayer with nil Perm
	sl = &SafetyLayer{}
	if sl.IsPermitted("write") {
		t.Error("SafetyLayer with nil Perm should deny permissions")
	}
}

func TestOptimizer_NilSafe(t *testing.T) {
	var o *Optimizer

	// Calling WithinBudget on nil Optimizer should not panic
	if !o.WithinBudget() {
		t.Error("nil Optimizer should return true (no limit)")
	}

	// Zero budget means unlimited
	o = &Optimizer{}
	if !o.WithinBudget() {
		t.Error("zero MaxBudget should mean unlimited")
	}

	// Within budget
	o = &Optimizer{MaxBudget: 10.0, Cost: Cost{TotalCostUSD: 5.0}}
	if !o.WithinBudget() {
		t.Error("5.0 < 10.0 should be within budget")
	}

	// Over budget
	o = &Optimizer{MaxBudget: 10.0, Cost: Cost{TotalCostUSD: 15.0}}
	if o.WithinBudget() {
		t.Error("15.0 > 10.0 should be over budget")
	}
}

func TestSessionServices_NilSubservices(t *testing.T) {
	// A SessionServices where everything is nil should not panic on access
	ss := &SessionServices{}

	if ss.Core != nil {
		t.Log("Core is nil, that's fine")
	}
	if ss.Safety != nil {
		t.Log("Safety is nil, that's fine")
	}
	if ss.Intel != nil {
		t.Log("Intel is nil, that's fine")
	}
	if ss.Optim != nil {
		t.Log("Optim is nil, that's fine")
	}
	if ss.Observe != nil {
		t.Log("Observe is nil, that's fine")
	}

	// Verify nil Optimizer convenience method works
	if !ss.Optim.WithinBudget() {
		t.Error("nil Optim.WithinBudget() should return true")
	}
}

func TestObservability_NilFields(t *testing.T) {
	obs := &Observability{}

	// Accessing nil fields should be safe (no method calls, just nil checks)
	if obs.Tracer != nil {
		t.Error("expected nil Tracer")
	}
	if obs.Metrics != nil {
		t.Error("expected nil Metrics")
	}
	if obs.Log != nil {
		t.Error("expected nil Log")
	}
}

func TestIntelligence_NilFields(t *testing.T) {
	intel := &Intelligence{}

	if intel.Beliefs != nil {
		t.Error("expected nil Beliefs")
	}
	if intel.Memory != nil {
		t.Error("expected nil Memory")
	}
	if intel.YaadBridge != nil {
		t.Error("expected nil YaadBridge")
	}
	if intel.Enhanced != nil {
		t.Error("expected nil Enhanced")
	}
	if intel.Sleeptime != nil {
		t.Error("expected nil Sleeptime")
	}
	if intel.Activity != nil {
		t.Error("expected nil Activity")
	}
	if intel.SkillDistill != nil {
		t.Error("expected nil SkillDistill")
	}
}

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

// mockMemoryRecaller implements MemoryRecaller for testing.
type mockMemoryRecaller struct{}

func (m *mockMemoryRecaller) Recall(query string, tokenBudget int) (string, error) {
	return "recalled: " + query, nil
}

func (m *mockMemoryRecaller) Remember(content, category string) error {
	return nil
}

// Ensure mockMemoryRecaller satisfies the interface at compile time.
var _ MemoryRecaller = (*mockMemoryRecaller)(nil)

// Suppress unused import warnings for packages used only in type assertions.
var (
	_ *memory.YaadBridge = nil
	_ *metrics.Registry  = nil
	_ *oteltrace.Tracer      = nil
)
