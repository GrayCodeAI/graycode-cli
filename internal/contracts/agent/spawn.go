// Vendored from github.com/GrayCodeAI/eagle/agent at v0.0.0-20260902153929-5877bed17503 (MIT, Copyright (c) 2026 GrayCode AI).
// The upstream repository no longer exists; this copy is owned by Graycode as its contract surface.
// Package agent defines shared DTOs for typed subagent spawn across graycode-eco.
//
// Stdlib only. No engine, CLI, or storage imports.
package agent

import (
	"fmt"
	"strings"
)

// CapabilityMode limits what tools a subagent may use.
type CapabilityMode string

const (
	CapReadOnly  CapabilityMode = "read-only"
	CapReadWrite CapabilityMode = "read-write"
	CapExecute   CapabilityMode = "execute"
	CapAll       CapabilityMode = "all"
)

// IsolationMode selects filesystem isolation for a subagent.
type IsolationMode string

const (
	IsoNone     IsolationMode = "none"
	IsoWorktree IsolationMode = "worktree"
)

// SubagentType selects the built-in subagent profile.
type SubagentType string

const (
	TypeGeneralPurpose SubagentType = "general-purpose"
	TypeExplore        SubagentType = "explore"
	TypePlan           SubagentType = "plan"
)

// Thoroughness levels for explore subagents.
const (
	ThoroughnessQuick        = "quick"
	ThoroughnessMedium       = "medium"
	ThoroughnessVeryThorough = "very-thorough"
)

// Spawn status values for SpawnResult.Status.
const (
	StatusRunning   = "running"
	StatusCompleted = "completed"
	StatusFailed    = "failed"
)

// SpawnRequest is the cross-repo contract for spawning a subagent.
type SpawnRequest struct {
	Prompt         string `json:"prompt"`
	Description    string `json:"description,omitempty"`
	SubagentType   string `json:"subagent_type,omitempty"`
	CapabilityMode string `json:"capability_mode,omitempty"`
	Isolation      string `json:"isolation,omitempty"`
	ResumeFrom     string `json:"resume_from,omitempty"`
	CWD            string `json:"cwd,omitempty"`
	Model          string `json:"model,omitempty"`
	Background     bool   `json:"background,omitempty"`
	Thoroughness   string `json:"thoroughness,omitempty"`
	ParentSession  string `json:"parent_session,omitempty"`
}

// SpawnResult is the cross-repo contract returned after spawn completes or is accepted.
type SpawnResult struct {
	SubagentID   string `json:"subagent_id,omitempty"`
	SubagentType string `json:"subagent_type,omitempty"`
	Status       string `json:"status,omitempty"`
	Output       string `json:"output,omitempty"`
	Summary      string `json:"summary,omitempty"`
	ToolCalls    int    `json:"tool_calls,omitempty"`
	Turns        int    `json:"turns,omitempty"`
	DurationMs   int64  `json:"duration_ms,omitempty"`
	WorktreePath string `json:"worktree_path,omitempty"`
	Persona      string `json:"persona,omitempty"`
	Error        string `json:"error,omitempty"`
}

// ParseSubagentType normalizes aliases (e.g. "general" → general-purpose).
// Empty input defaults to explore (conservative read-oriented default).
func ParseSubagentType(s string) (SubagentType, error) {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "", "explore":
		return TypeExplore, nil
	case "plan":
		return TypePlan, nil
	case "general", "general-purpose", "general_purpose", "generalpurpose":
		return TypeGeneralPurpose, nil
	default:
		return "", fmt.Errorf("agent: unknown subagent_type %q", s)
	}
}

// ParseCapabilityMode normalizes capability aliases.
// Empty input returns CapReadOnly when defaultFromType is empty; callers
// should prefer DefaultCapabilityForType.
func ParseCapabilityMode(s string) (CapabilityMode, error) {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "", "read-only", "readonly", "read_only", "ro":
		return CapReadOnly, nil
	case "read-write", "readwrite", "read_write", "rw":
		return CapReadWrite, nil
	case "execute", "exec":
		return CapExecute, nil
	case "all", "full":
		return CapAll, nil
	default:
		return "", fmt.Errorf("agent: unknown capability_mode %q", s)
	}
}

// ParseIsolationMode normalizes isolation aliases. Empty → none.
func ParseIsolationMode(s string) (IsolationMode, error) {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "", "none", "off", "false":
		return IsoNone, nil
	case "worktree", "wt", "git-worktree":
		return IsoWorktree, nil
	default:
		return "", fmt.Errorf("agent: unknown isolation %q", s)
	}
}

// ParseThoroughness normalizes explore thoroughness. Empty → medium.
func ParseThoroughness(s string) (string, error) {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "", "medium", "med", "default":
		return ThoroughnessMedium, nil
	case "quick", "fast":
		return ThoroughnessQuick, nil
	case "very-thorough", "very_thorough", "verythorough", "deep":
		return ThoroughnessVeryThorough, nil
	default:
		return "", fmt.Errorf("agent: unknown thoroughness %q", s)
	}
}

// DefaultCapabilityForType returns the capability implied by a subagent type
// when the request does not set capability_mode.
func DefaultCapabilityForType(t SubagentType) CapabilityMode {
	switch t {
	case TypeGeneralPurpose:
		return CapAll
	case TypePlan, TypeExplore:
		return CapReadOnly
	default:
		return CapReadOnly
	}
}

// Normalized is a validated, alias-resolved spawn request.
type Normalized struct {
	Prompt         string
	Description    string
	SubagentType   SubagentType
	CapabilityMode CapabilityMode
	Isolation      IsolationMode
	ResumeFrom     string
	CWD            string
	Model          string
	Background     bool
	Thoroughness   string
	ParentSession  string
}

// Normalize validates and resolves aliases on r.
//
// Rules:
//   - prompt is required unless resume_from is set
//   - cwd and isolation=worktree are mutually exclusive
//   - thoroughness only applies to explore (ignored otherwise after parse)
//   - empty capability_mode uses DefaultCapabilityForType
func (r SpawnRequest) Normalize() (Normalized, error) {
	prompt := strings.TrimSpace(r.Prompt)
	resume := strings.TrimSpace(r.ResumeFrom)
	if prompt == "" && resume == "" {
		return Normalized{}, fmt.Errorf("agent: prompt is required unless resume_from is set")
	}

	st, err := ParseSubagentType(r.SubagentType)
	if err != nil {
		return Normalized{}, err
	}

	var capMode CapabilityMode
	if strings.TrimSpace(r.CapabilityMode) == "" {
		capMode = DefaultCapabilityForType(st)
	} else {
		capMode, err = ParseCapabilityMode(r.CapabilityMode)
		if err != nil {
			return Normalized{}, err
		}
	}

	iso, err := ParseIsolationMode(r.Isolation)
	if err != nil {
		return Normalized{}, err
	}

	cwd := strings.TrimSpace(r.CWD)
	if cwd != "" && iso == IsoWorktree {
		return Normalized{}, fmt.Errorf("agent: cwd and isolation=worktree are mutually exclusive")
	}

	thorough := ThoroughnessMedium
	if st == TypeExplore {
		thorough, err = ParseThoroughness(r.Thoroughness)
		if err != nil {
			return Normalized{}, err
		}
	} else if strings.TrimSpace(r.Thoroughness) != "" {
		// Explicit thoroughness on non-explore is an error to catch model mistakes.
		return Normalized{}, fmt.Errorf("agent: thoroughness is only valid for explore subagents")
	}

	return Normalized{
		Prompt:         prompt,
		Description:    strings.TrimSpace(r.Description),
		SubagentType:   st,
		CapabilityMode: capMode,
		Isolation:      iso,
		ResumeFrom:     resume,
		CWD:            cwd,
		Model:          strings.TrimSpace(r.Model),
		Background:     r.Background,
		Thoroughness:   thorough,
		ParentSession:  strings.TrimSpace(r.ParentSession),
	}, nil
}

// Validate is an alias for Normalize when only the error is needed.
func (r SpawnRequest) Validate() error {
	_, err := r.Normalize()
	return err
}
