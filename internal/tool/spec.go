package tool

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"
)

var reSlugInvalid = regexp.MustCompile(`[^a-z0-9]+`)

func slugify(s string) string {
	s = strings.ToLower(strings.TrimSpace(s))
	s = reSlugInvalid.ReplaceAllString(s, "-")
	s = strings.Trim(s, "-")
	if len(s) > 40 {
		s = s[:40]
	}
	if s == "" {
		s = "spec"
	}
	return s
}

// specSlug reads the active spec slug via the per-session ToolContext
// closures. Session-scoped (not a package-level variable) so concurrent
// sessions/sub-agents in the same process never share or clobber each
// other's spec directory.
func specSlug(ctx context.Context) (string, error) {
	tc := GetToolContext(ctx)
	if tc == nil || tc.SpecSlugGet == nil {
		return "", fmt.Errorf("no active session context for spec workflow")
	}
	return tc.SpecSlugGet(), nil
}

func setSpecSlug(ctx context.Context, slug string) error {
	tc := GetToolContext(ctx)
	if tc == nil || tc.SpecSlugSet == nil {
		return fmt.Errorf("no active session context for spec workflow")
	}
	tc.SpecSlugSet(slug)
	return nil
}

func specDir(ctx context.Context) (string, error) {
	slug, err := specSlug(ctx)
	if err != nil {
		return "", err
	}
	if slug == "" {
		return "", fmt.Errorf("no active spec — call Specify first")
	}
	cwd, err := os.Getwd()
	if err != nil {
		return "", err
	}
	return filepath.Join(cwd, ".hawk", "specs", slug), nil
}

func writeSpecArtifact(ctx context.Context, filename, content string) (string, error) {
	dir, err := specDir(ctx)
	if err != nil {
		return "", err
	}
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return "", fmt.Errorf("mkdir: %w", err)
	}
	path := filepath.Join(dir, filename)
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		return "", fmt.Errorf("write %s: %w", filename, err)
	}
	return path, nil
}

// SpecifyTool starts (or restarts) a spec workflow: writes spec.md with the
// model's understanding of the problem. First of the Specify -> Plan ->
// Tasks -> ApproveImplementation sequence.
type SpecifyTool struct{}

func (SpecifyTool) Name() string      { return "Specify" }
func (SpecifyTool) Aliases() []string { return []string{"specify"} }
func (SpecifyTool) Description() string {
	return "Write spec.md describing the problem and requirements, starting a spec-driven workflow. Call this first when working through a gated spec stage. Write/Edit/Bash stay blocked until ApproveImplementation is called and approved."
}

func (SpecifyTool) Parameters() map[string]interface{} {
	return map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"title": map[string]interface{}{"type": "string", "description": "Short title for this spec, used to name its directory"},
			"spec":  map[string]interface{}{"type": "string", "description": "The spec content: problem statement, requirements, constraints"},
		},
		"required": []string{"spec"},
	}
}

func (SpecifyTool) Execute(ctx context.Context, input json.RawMessage) (string, error) {
	var p struct {
		Title string `json:"title"`
		Spec  string `json:"spec"`
	}
	if err := json.Unmarshal(input, &p); err != nil {
		return "", err
	}
	if strings.TrimSpace(p.Spec) == "" {
		return "", fmt.Errorf("spec is required")
	}
	slug := slugify(p.Title)
	if slug == "spec" {
		slug = slugify(firstLine(p.Spec))
	}
	if err := setSpecSlug(ctx, fmt.Sprintf("%s-%d", slug, time.Now().Unix())); err != nil {
		return "", err
	}
	path, err := writeSpecArtifact(ctx, "spec.md", p.Spec)
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("Wrote %s. Next, call Plan with your technical approach.", path), nil
}

// PlanTool writes plan.md — the technical approach for an active spec.
type PlanTool struct{}

func (PlanTool) Name() string      { return "Plan" }
func (PlanTool) Aliases() []string { return []string{"plan"} }
func (PlanTool) Description() string {
	return "Write plan.md describing the technical approach for the active spec. Call after Specify."
}

func (PlanTool) Parameters() map[string]interface{} {
	return map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"plan": map[string]interface{}{"type": "string", "description": "The technical approach: architecture, files to change, key decisions"},
		},
		"required": []string{"plan"},
	}
}

func (PlanTool) Execute(ctx context.Context, input json.RawMessage) (string, error) {
	var p struct {
		Plan string `json:"plan"`
	}
	if err := json.Unmarshal(input, &p); err != nil {
		return "", err
	}
	if strings.TrimSpace(p.Plan) == "" {
		return "", fmt.Errorf("plan is required")
	}
	path, err := writeSpecArtifact(ctx, "plan.md", p.Plan)
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("Wrote %s. Next, call Tasks with a breakdown.", path), nil
}

// TasksTool writes tasks.md — the implementation breakdown for an active spec.
type TasksTool struct{}

func (TasksTool) Name() string      { return "Tasks" }
func (TasksTool) Aliases() []string { return []string{"tasks"} }
func (TasksTool) Description() string {
	return "Write tasks.md breaking the plan into concrete implementation steps. Call after Plan."
}

func (TasksTool) Parameters() map[string]interface{} {
	return map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"tasks": map[string]interface{}{"type": "string", "description": "The task breakdown, as a list"},
		},
		"required": []string{"tasks"},
	}
}

func (TasksTool) Execute(ctx context.Context, input json.RawMessage) (string, error) {
	var p struct {
		Tasks string `json:"tasks"`
	}
	if err := json.Unmarshal(input, &p); err != nil {
		return "", err
	}
	if strings.TrimSpace(p.Tasks) == "" {
		return "", fmt.Errorf("tasks is required")
	}
	path, err := writeSpecArtifact(ctx, "tasks.md", p.Tasks)
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("Wrote %s. Call ApproveImplementation to ask the user to approve moving to implementation.", path), nil
}

// ApproveImplementationTool requests the user's approval to lift the spec
// gate and unlock Write/Edit/Bash. Unlike Specify/Plan/Tasks, this call
// always goes through a real permission prompt (see
// PermissionEngine.CheckTool) regardless of autonomy tier.
type ApproveImplementationTool struct{}

func (ApproveImplementationTool) Name() string      { return "ApproveImplementation" }
func (ApproveImplementationTool) Aliases() []string { return []string{"approve_implementation"} }
func (ApproveImplementationTool) Description() string {
	return "Ask the user to approve moving from spec to implementation. Only after approval will Write/Edit/Bash be permitted. Call this once spec.md, plan.md, and tasks.md are all written."
}

func (ApproveImplementationTool) Parameters() map[string]interface{} {
	return map[string]interface{}{"type": "object", "properties": map[string]interface{}{}}
}

func (ApproveImplementationTool) Execute(_ context.Context, _ json.RawMessage) (string, error) {
	return "Approved. You may now implement the plan and make changes.", nil
}

func firstLine(s string) string {
	if i := strings.IndexByte(s, '\n'); i >= 0 {
		return s[:i]
	}
	return s
}
