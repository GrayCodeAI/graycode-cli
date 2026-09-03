package mission

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os/exec"
	"sort"
	"sync"
	"time"

	"github.com/GrayCodeAI/graycode-cli/internal/acp"
)

var (
	// ErrProviderNotFound is returned when no subagent provider is registered under the given name.
	ErrProviderNotFound = errors.New("subagent: provider not found")
	// ErrUnsupportedCapability is returned when a subagent request requires a capability the provider lacks.
	ErrUnsupportedCapability = errors.New("subagent: unsupported capability")
	// ErrDepthExceeded is returned when the subagent delegation depth limit is exceeded.
	ErrDepthExceeded = errors.New("subagent: delegation depth limit exceeded")
	// ErrUnsupportedPersona is returned when the requested persona is not supported by the provider.
	ErrUnsupportedPersona = errors.New("subagent: unsupported persona")
)

// SubagentCapabilities declares the features supported by a subagent provider.
type SubagentCapabilities struct {
	SupportsStreaming bool     `json:"supports_streaming"`
	SupportsSchema    bool     `json:"supports_schema"`
	MaxDepth          int      `json:"max_depth,omitempty"`
	SupportedPersonas []string `json:"supported_personas,omitempty"`
	AllowedTools      []string `json:"allowed_tools,omitempty"`
}

// SubagentRequest defines the payload for delegating a task to an external subagent.
type SubagentRequest struct {
	Name          string                 `json:"name"`
	Task          string                 `json:"task"`
	CWD           string                 `json:"cwd,omitempty"`
	Persona       string                 `json:"persona,omitempty"`
	OutputSchema  map[string]interface{} `json:"output_schema,omitempty"`
	Depth         int                    `json:"depth,omitempty"`
	ApprovalGate  *MissionApprovalGate   `json:"-"`
	ParentSession any                    `json:"-"`
}

// SubagentResult represents the output of a completed subagent execution.
type SubagentResult struct {
	Status   string                 `json:"status"` // "success", "failure", "cancelled"
	Output   string                 `json:"output"`
	Data     map[string]interface{} `json:"data,omitempty"`
	Error    string                 `json:"error,omitempty"`
	Duration time.Duration          `json:"duration"`
}

// SubagentProvider represents a backend that can execute delegated subagent tasks.
type SubagentProvider interface {
	Name() string
	Capabilities() SubagentCapabilities
	Run(ctx context.Context, req SubagentRequest) (*SubagentResult, error)
}

// ProviderRegistry manages registered subagent providers.
type ProviderRegistry struct {
	mu        sync.RWMutex
	providers map[string]SubagentProvider
}

// NewProviderRegistry creates an empty provider registry.
func NewProviderRegistry() *ProviderRegistry {
	return &ProviderRegistry{
		providers: make(map[string]SubagentProvider),
	}
}

// Register registers a SubagentProvider and returns a disposer to unregister it.
func (r *ProviderRegistry) Register(p SubagentProvider) func() {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.providers[p.Name()] = p

	return func() {
		r.mu.Lock()
		defer r.mu.Unlock()
		delete(r.providers, p.Name())
	}
}

// Get retrieves a provider by name.
func (r *ProviderRegistry) Get(name string) (SubagentProvider, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	p, ok := r.providers[name]
	return p, ok
}

// List returns all registered providers sorted by name.
func (r *ProviderRegistry) List() []SubagentProvider {
	r.mu.RLock()
	defer r.mu.RUnlock()
	list := make([]SubagentProvider, 0, len(r.providers))
	for _, p := range r.providers {
		list = append(list, p)
	}
	sort.Slice(list, func(i, j int) bool {
		return list[i].Name() < list[j].Name()
	})
	return list
}

// Run validates capabilities and dispatches a task to the target provider.
func (r *ProviderRegistry) Run(ctx context.Context, req SubagentRequest) (*SubagentResult, error) {
	p, ok := r.Get(req.Name)
	if !ok {
		return nil, fmt.Errorf("%w: %s", ErrProviderNotFound, req.Name)
	}

	caps := p.Capabilities()

	// 1. OutputSchema capability check
	if len(req.OutputSchema) > 0 && !caps.SupportsSchema {
		return nil, fmt.Errorf("%w: output schema requested but provider %s does not support it",
			ErrUnsupportedCapability, req.Name)
	}

	// 2. Depth limit check
	if caps.MaxDepth > 0 && req.Depth > caps.MaxDepth {
		return nil, fmt.Errorf("%w: requested depth %d exceeds provider %s max depth %d",
			ErrDepthExceeded, req.Depth, req.Name, caps.MaxDepth)
	}

	// 3. Persona check
	if req.Persona != "" && len(caps.SupportedPersonas) > 0 {
		matched := false
		for _, persona := range caps.SupportedPersonas {
			if persona == req.Persona || persona == "*" {
				matched = true
				break
			}
		}
		if !matched {
			return nil, fmt.Errorf("%w: persona %q not supported by provider %s",
				ErrUnsupportedPersona, req.Persona, req.Name)
		}
	}

	start := time.Now()
	res, err := p.Run(ctx, req)
	if err != nil {
		return nil, err
	}
	if res.Duration == 0 {
		res.Duration = time.Since(start)
	}

	// Parse structured data if schema was requested and valid JSON output received
	if len(req.OutputSchema) > 0 && res.Output != "" && res.Data == nil {
		var parsed map[string]interface{}
		if jsonErr := json.Unmarshal([]byte(res.Output), &parsed); jsonErr == nil {
			res.Data = parsed
		}
	}

	return res, nil
}

// --- Built-in ACP Providers ---

// GenericACPProvider executes tasks via an external ACP-compatible CLI.
type GenericACPProvider struct {
	name    string
	command string
	args    []string
	caps    SubagentCapabilities
}

// NewGenericACPProvider creates a new GenericACPProvider.
func NewGenericACPProvider(name, command string, args []string, caps SubagentCapabilities) *GenericACPProvider {
	return &GenericACPProvider{
		name:    name,
		command: command,
		args:    args,
		caps:    caps,
	}
}

func (p *GenericACPProvider) Name() string                       { return p.name }
func (p *GenericACPProvider) Capabilities() SubagentCapabilities { return p.caps }

func (p *GenericACPProvider) Run(ctx context.Context, req SubagentRequest) (*SubagentResult, error) {
	start := time.Now()

	var opts []acp.ClientOption
	if req.ApprovalGate != nil {
		opts = append(opts, acp.WithOnPermissionRequest(func(permReq acp.PermissionRequest) (bool, error) {
			if err := req.ApprovalGate.Check(ctx, permReq.ToolName, permReq.Summary); err != nil {
				return false, nil
			}
			return true, nil
		}))
	}

	client, err := acp.Start(ctx, p.command, p.args, opts...)
	if err != nil {
		return nil, fmt.Errorf("failed to start ACP client %s: %w", p.name, err)
	}
	defer func() { _ = client.Close() }()

	sessionID, err := client.NewSession(ctx, req.CWD)
	if err != nil {
		return nil, fmt.Errorf("failed to create ACP session on %s: %w", p.name, err)
	}

	prompt := req.Task
	if req.Persona != "" {
		prompt = fmt.Sprintf("[Persona: %s]\n%s", req.Persona, prompt)
	}

	promptRes, err := client.Prompt(ctx, sessionID, prompt)
	if err != nil {
		if errors.Is(ctx.Err(), context.Canceled) {
			_ = client.Cancel(context.Background(), sessionID)
			return &SubagentResult{
				Status:   "cancelled",
				Error:    "context canceled",
				Duration: time.Since(start),
			}, nil
		}
		return nil, fmt.Errorf("ACP prompt execution failed: %w", err)
	}

	status := "success"
	if promptRes.Status == "error" {
		status = "failure"
	}

	return &SubagentResult{
		Status:   status,
		Output:   promptRes.Output,
		Duration: time.Since(start),
	}, nil
}

// DefaultRegistry is the singleton default provider registry.
var (
	defaultRegistry     *ProviderRegistry
	defaultRegistryOnce sync.Once
)

// DefaultProviders returns the global SubagentProvider registry.
func DefaultProviders() *ProviderRegistry {
	defaultRegistryOnce.Do(func() {
		defaultRegistry = NewProviderRegistry()

		// 1. Register generic acp provider if 'acp' binary is in PATH
		if _, err := exec.LookPath("acp"); err == nil {
			defaultRegistry.Register(NewGenericACPProvider("acp", "acp", nil, SubagentCapabilities{
				SupportsStreaming: true,
				SupportsSchema:    true,
				MaxDepth:          3,
			}))
		}

		// 2. Register claude-code provider if 'claude' is in PATH
		if _, err := exec.LookPath("claude"); err == nil {
			defaultRegistry.Register(NewGenericACPProvider("claude-code", "claude", []string{"--acp"}, SubagentCapabilities{
				SupportsStreaming: true,
				SupportsSchema:    true,
				MaxDepth:          2,
			}))
		}

		// 3. Register codex provider if 'codex' is in PATH
		if _, err := exec.LookPath("codex"); err == nil {
			defaultRegistry.Register(NewGenericACPProvider("codex", "codex", []string{"--acp"}, SubagentCapabilities{
				SupportsStreaming: true,
				SupportsSchema:    true,
				MaxDepth:          2,
			}))
		}
	})
	return defaultRegistry
}
