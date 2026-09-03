package mission

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/GrayCodeAI/graycode-cli/internal/eventlog"
	"github.com/GrayCodeAI/graycode-cli/internal/session"
	"github.com/GrayCodeAI/graycode-cli/internal/storage"
	"github.com/google/uuid"
)

var (
	// ErrUnauthorizedCaller is returned when a follow-up or operation is attempted
	// by a caller that does not match the child's durable parent session.
	ErrUnauthorizedCaller = errors.New("subagent: unauthorized: caller is not the authoritative direct parent of this child session")
	// ErrContinuableLabelRequired is returned when starting a continuable child without a label.
	ErrContinuableLabelRequired = errors.New("subagent: label is required for continuable subagent")
	// ErrInvalidDelegationDepth is returned when delegation depth violates monotonicity.
	ErrInvalidDelegationDepth = errors.New("subagent: delegation depth must be strictly greater than parent depth")
	// ErrChildSessionClosed is returned when enqueueing turns to a closed activation.
	ErrChildSessionClosed = errors.New("subagent: child session activation is closed")
	// ErrMissingParentSession is returned when parent session ID is missing.
	ErrMissingParentSession = errors.New("subagent: parent session is required")
)

// ContinuableSpec specifies the configuration for establishing a durable, continuable child subagent.
type ContinuableSpec struct {
	ParentSessionID string                 `json:"parent_session_id"`
	ChildID         string                 `json:"child_id,omitempty"`
	Provider        string                 `json:"provider"`
	Label           string                 `json:"label"`
	Model           string                 `json:"model,omitempty"`
	Persona         string                 `json:"persona,omitempty"`
	ToolFilterAllow []string               `json:"tool_filter_allow,omitempty"`
	ToolFilterDeny  []string               `json:"tool_filter_deny,omitempty"`
	InitialPrompt   string                 `json:"initial_prompt"`
	CWD             string                 `json:"cwd,omitempty"`
	Depth           int                    `json:"depth"`
	OutputSchema    map[string]interface{} `json:"output_schema,omitempty"`
	ApprovalGate    *MissionApprovalGate   `json:"-"`
	ParentSession   any                    `json:"-"`
}

// TurnRequest represents a queued FIFO turn for an active subagent child.
type TurnRequest struct {
	Prompt string
	Ctx    context.Context
	Done   chan TurnResponse
}

// TurnResponse represents the result of a completed subagent turn.
type TurnResponse struct {
	Output   string
	Error    error
	Duration time.Duration
}

// Activation represents one process-local residency epoch for an active subagent child.
type Activation struct {
	mu           sync.Mutex
	ChildID      string
	ParentID     string
	Provider     string
	Model        string
	Label        string
	Persona      string
	Depth        int
	ToolAllow    []string
	ToolDeny     []string
	CWD          string
	OutputSchema map[string]interface{}
	ApprovalGate *MissionApprovalGate

	inbox     chan TurnRequest
	closed    bool
	closeOnce sync.Once

	sess         *session.Session
	providerImpl SubagentProvider

	createdAt    time.Time
	lastActiveAt time.Time
}

// Close gracefully closes the activation turn loop.
func (a *Activation) Close() {
	a.closeOnce.Do(func() {
		a.mu.Lock()
		a.closed = true
		a.mu.Unlock()
		close(a.inbox)
	})
}

// runLoop executes the FIFO turn queue.
func (a *Activation) runLoop() {
	for req := range a.inbox {
		start := time.Now()
		a.mu.Lock()
		a.lastActiveAt = start
		a.mu.Unlock()

		select {
		case <-req.Ctx.Done():
			req.Done <- TurnResponse{Error: req.Ctx.Err(), Duration: time.Since(start)}
			continue
		default:
		}

		// Execute turn via provider
		var out string
		var runErr error

		if a.providerImpl != nil {
			subReq := SubagentRequest{
				Name:          a.ChildID,
				Task:          req.Prompt,
				CWD:           a.CWD,
				Persona:       a.Persona,
				OutputSchema:  a.OutputSchema,
				Depth:         a.Depth,
				ApprovalGate:  a.ApprovalGate,
				ParentSession: a.ParentID,
			}
			res, err := a.providerImpl.Run(req.Ctx, subReq)
			if err != nil {
				runErr = err
			} else if res != nil {
				out = res.Output
				if res.Error != "" {
					runErr = errors.New(res.Error)
				}
			}
		} else {
			// Builtin fallback echo response
			out = fmt.Sprintf("subagent (%s) completed: %s", a.ChildID, req.Prompt)
		}

		// Append messages and persist
		a.mu.Lock()
		if a.sess != nil {
			a.sess.Messages = append(a.sess.Messages, session.Message{
				Role:    "user",
				Content: req.Prompt,
			})
			if out != "" {
				a.sess.Messages = append(a.sess.Messages, session.Message{
					Role:    "assistant",
					Content: out,
				})
			}
			a.sess.UpdatedAt = time.Now()
			_ = session.Save(a.sess)
		}
		a.mu.Unlock()

		req.Done <- TurnResponse{
			Output:   out,
			Error:    runErr,
			Duration: time.Since(start),
		}
	}
}

// ContinuableManager manages continuable subagent lifecycles, FIFO turns, and cold resume.
type ContinuableManager struct {
	mu          sync.RWMutex
	activations map[string]*Activation
	providers   *ProviderRegistry
}

var (
	defaultContinuableManager     *ContinuableManager
	defaultContinuableManagerOnce sync.Once
)

// DefaultContinuableManager returns the singleton ContinuableManager.
func DefaultContinuableManager() *ContinuableManager {
	defaultContinuableManagerOnce.Do(func() {
		defaultContinuableManager = NewContinuableManager(DefaultProviders())
	})
	return defaultContinuableManager
}

// NewContinuableManager creates a new ContinuableManager with the given provider registry.
func NewContinuableManager(providers *ProviderRegistry) *ContinuableManager {
	if providers == nil {
		providers = DefaultProviders()
	}
	return &ContinuableManager{
		activations: make(map[string]*Activation),
		providers:   providers,
	}
}

// StartContinuable establishes a durable child session and delivers the initial prompt via FIFO queue.
func (cm *ContinuableManager) StartContinuable(ctx context.Context, spec ContinuableSpec) (*Activation, TurnResponse, error) {
	if spec.ParentSessionID == "" {
		return nil, TurnResponse{}, ErrMissingParentSession
	}
	if strings.TrimSpace(spec.Label) == "" {
		return nil, TurnResponse{}, ErrContinuableLabelRequired
	}
	if spec.ChildID == "" {
		spec.ChildID = fmt.Sprintf("subagent-%s", uuid.New().String()[:8])
	}
	if spec.Depth <= 0 {
		spec.Depth = 1
	}

	// Verify depth monotonicity against parent if parent exists
	if parentSess, err := session.Load(spec.ParentSessionID); err == nil && parentSess != nil {
		if spec.Depth <= parentSess.DelegationDepth {
			return nil, TurnResponse{}, ErrInvalidDelegationDepth
		}
	}

	// Resolve provider
	providerName := spec.Provider
	if providerName == "" {
		providerName = "default"
	}
	pImpl, _ := cm.providers.Get(providerName)

	now := time.Now().UTC()
	sess := &session.Session{
		ID:              spec.ChildID,
		ParentSessionID: spec.ParentSessionID,
		DelegationDepth: spec.Depth,
		Model:           spec.Model,
		Provider:        providerName,
		CWD:             spec.CWD,
		Name:            spec.Label,
		CreatedAt:       now,
		UpdatedAt:       now,
	}

	// Append durable SubagentDescriptorFact
	descFact := eventlog.SubagentDescriptorFact{
		Version:         eventlog.SubagentDescriptorVersion,
		Mode:            "continuable",
		Provider:        providerName,
		Label:           spec.Label,
		AgentProvider:   providerName,
		AgentModel:      spec.Model,
		Persona:         spec.Persona,
		ToolFilterAllow: spec.ToolFilterAllow,
		ToolFilterDeny:  spec.ToolFilterDeny,
		Depth:           spec.Depth,
	}
	descFactBytes, _ := json.Marshal(descFact)
	sess.Events = append(sess.Events, eventlog.WireEvent{
		Type: eventlog.SubagentDescriptor,
		Seq:  1,
		At:   now,
		Data: descFactBytes,
	})

	// Pre-publication transactional persistence: rollback on failure
	if err := session.Save(sess); err != nil {
		return nil, TurnResponse{}, fmt.Errorf("subagent: failed to persist child session: %w", err)
	}

	act := &Activation{
		ChildID:      spec.ChildID,
		ParentID:     spec.ParentSessionID,
		Provider:     providerName,
		Model:        spec.Model,
		Label:        spec.Label,
		Persona:      spec.Persona,
		Depth:        spec.Depth,
		ToolAllow:    spec.ToolFilterAllow,
		ToolDeny:     spec.ToolFilterDeny,
		CWD:          spec.CWD,
		OutputSchema: spec.OutputSchema,
		ApprovalGate: spec.ApprovalGate,
		inbox:        make(chan TurnRequest, 32),
		sess:         sess,
		providerImpl: pImpl,
		createdAt:    now,
		lastActiveAt: now,
	}

	cm.mu.Lock()
	cm.activations[spec.ChildID] = act
	cm.mu.Unlock()

	go act.runLoop()

	// Deliver initial prompt
	done := make(chan TurnResponse, 1)
	select {
	case act.inbox <- TurnRequest{Prompt: spec.InitialPrompt, Ctx: ctx, Done: done}:
	case <-ctx.Done():
		cm.Dispose(spec.ChildID)
		_ = cm.deletePersistedSession(spec.ChildID)
		return nil, TurnResponse{}, ctx.Err()
	}

	select {
	case res := <-done:
		return act, res, res.Error
	case <-ctx.Done():
		return act, TurnResponse{Error: ctx.Err()}, ctx.Err()
	}
}

// Followup sends a follow-up turn to a child subagent with FIFO ordering, cold-resuming from disk if absent.
func (cm *ContinuableManager) Followup(ctx context.Context, parentSessionID, childID, content string) (TurnResponse, error) {
	if parentSessionID == "" {
		return TurnResponse{}, ErrMissingParentSession
	}
	if childID == "" {
		return TurnResponse{}, errors.New("subagent: child ID is required")
	}

	cm.mu.Lock()
	act, resident := cm.activations[childID]
	if !resident || act.closed {
		// Cold Resume from persisted session log
		resumedAct, err := cm.coldResumeLocked(childID, parentSessionID)
		if err != nil {
			cm.mu.Unlock()
			return TurnResponse{}, err
		}
		act = resumedAct
		cm.activations[childID] = act
	}
	cm.mu.Unlock()

	// Authority verification against durable header
	if act.ParentID != parentSessionID {
		return TurnResponse{}, ErrUnauthorizedCaller
	}

	done := make(chan TurnResponse, 1)
	act.mu.Lock()
	if act.closed {
		act.mu.Unlock()
		return TurnResponse{}, ErrChildSessionClosed
	}
	inbox := act.inbox
	act.mu.Unlock()

	select {
	case inbox <- TurnRequest{Prompt: content, Ctx: ctx, Done: done}:
	case <-ctx.Done():
		return TurnResponse{Error: ctx.Err()}, ctx.Err()
	}

	select {
	case res := <-done:
		return res, res.Error
	case <-ctx.Done():
		return TurnResponse{Error: ctx.Err()}, ctx.Err()
	}
}

// coldResumeLocked reconstructs an Activation from the durable session log. Caller must hold cm.mu.
func (cm *ContinuableManager) coldResumeLocked(childID, callerParentID string) (*Activation, error) {
	sess, err := session.Load(childID)
	if err != nil {
		return nil, fmt.Errorf("cold resume: %w", err)
	}

	// Recheck authority against durable parent header
	if sess.ParentSessionID != "" && sess.ParentSessionID != callerParentID {
		return nil, ErrUnauthorizedCaller
	}

	// Extract descriptor fact from session events
	var desc eventlog.SubagentDescriptorFact
	for _, ev := range sess.Events {
		if ev.Type == eventlog.SubagentDescriptor {
			if len(ev.Data) > 0 {
				_ = json.Unmarshal(ev.Data, &desc)
				break
			}
		}
	}

	pName := sess.Provider
	if pName == "" {
		pName = desc.Provider
	}
	if pName == "" {
		pName = "default"
	}
	pImpl, _ := cm.providers.Get(pName)

	now := time.Now().UTC()
	act := &Activation{
		ChildID:      sess.ID,
		ParentID:     sess.ParentSessionID,
		Provider:     pName,
		Model:        sess.Model,
		Label:        sess.Name,
		Persona:      desc.Persona,
		Depth:        sess.DelegationDepth,
		ToolAllow:    desc.ToolFilterAllow,
		ToolDeny:     desc.ToolFilterDeny,
		CWD:          sess.CWD,
		inbox:        make(chan TurnRequest, 32),
		sess:         sess,
		providerImpl: pImpl,
		createdAt:    sess.CreatedAt,
		lastActiveAt: now,
	}

	go act.runLoop()
	return act, nil
}

// ListChildren returns direct child sessions established by parentSessionID.
func (cm *ContinuableManager) ListChildren(parentSessionID string) ([]*session.Session, error) {
	if parentSessionID == "" {
		return nil, ErrMissingParentSession
	}

	dir := storage.SessionsDir()
	entries, err := os.ReadDir(dir)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, nil
		}
		return nil, err
	}

	var children []*session.Session
	for _, e := range entries {
		if !strings.HasSuffix(e.Name(), ".jsonl") && !strings.HasSuffix(e.Name(), ".json") {
			continue
		}
		name := strings.TrimSuffix(strings.TrimSuffix(e.Name(), ".jsonl"), ".json")
		if !session.ValidID(name) {
			continue
		}
		s, err := session.Load(name)
		if err == nil && s != nil && s.ParentSessionID == parentSessionID {
			children = append(children, s)
		}
	}

	sort.Slice(children, func(i, j int) bool {
		return children[i].CreatedAt.Before(children[j].CreatedAt)
	})
	return children, nil
}

// ListDescendants recursively returns all descendant child sessions rooted at parentSessionID.
func (cm *ContinuableManager) ListDescendants(parentSessionID string) ([]*session.Session, error) {
	direct, err := cm.ListChildren(parentSessionID)
	if err != nil {
		return nil, err
	}

	var all []*session.Session
	for _, child := range direct {
		all = append(all, child)
		descendants, err := cm.ListDescendants(child.ID)
		if err == nil && len(descendants) > 0 {
			all = append(all, descendants...)
		}
	}
	return all, nil
}

// Dispose gracefully terminates the child activation.
func (cm *ContinuableManager) Dispose(childID string) {
	cm.mu.Lock()
	act, ok := cm.activations[childID]
	if ok {
		delete(cm.activations, childID)
	}
	cm.mu.Unlock()

	if ok && act != nil {
		act.Close()
	}
}

func (cm *ContinuableManager) deletePersistedSession(id string) error {
	dir := storage.SessionsDir()
	_ = os.Remove(filepath.Join(dir, id+".jsonl"))
	_ = os.Remove(filepath.Join(dir, id+".json"))
	return nil
}
