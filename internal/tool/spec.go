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

	"github.com/GrayCodeAI/hawk/internal/engine/spec"
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

// SpecStatusTool reports the current spec stage and the status of all spec artifacts.
type SpecStatusTool struct{}

func (SpecStatusTool) Name() string      { return "SpecStatus" }
func (SpecStatusTool) Aliases() []string { return []string{"spec_status"} }
func (SpecStatusTool) Description() string {
	return "Show the current spec stage and validation status of spec artifacts (spec.md, plan.md, tasks.md). Use this to check progress in the spec workflow."
}

func (SpecStatusTool) Parameters() map[string]interface{} {
	return map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"slug": map[string]interface{}{"type": "string", "description": "Optional: check a specific spec slug instead of the active one"},
		},
	}
}

func (SpecStatusTool) Execute(ctx context.Context, input json.RawMessage) (string, error) {
	var p struct {
		Slug string `json:"slug"`
	}
	if input != nil {
		_ = json.Unmarshal(input, &p)
	}

	var slug string
	if p.Slug != "" {
		slug = p.Slug
	} else {
		var err error
		slug, err = specSlug(ctx)
		if err != nil || slug == "" {
			return "No active spec workflow. Use Specify to start, or check `SpecList` to see existing specs.", nil
		}
	}

	dir, err := specsDir()
	if err != nil {
		return "", err
	}
	specDir := filepath.Join(dir, slug)

	meta := spec.LoadStageMeta(slug)

	var b strings.Builder
	if meta != nil {
		fmt.Fprintf(&b, "Spec: %s\n", meta.Title)
		fmt.Fprintf(&b, "Stage: %s\n", spec.StageEnumDisplayName(meta.Stage))
		if !meta.CreatedAt.IsZero() {
			fmt.Fprintf(&b, "Created: %s\n", meta.CreatedAt.Format(time.RFC3339))
		}
		if !meta.UpdatedAt.IsZero() {
			fmt.Fprintf(&b, "Updated: %s\n", meta.UpdatedAt.Format(time.RFC3339))
		}
		b.WriteString("\n")
	}

	b.WriteString("Artifacts:\n")
	for _, f := range []string{"spec.md", "plan.md", "tasks.md", "specs.md"} {
		path := filepath.Join(specDir, f)
		info, err := os.Stat(path)
		if err != nil {
			fmt.Fprintf(&b, "  %s — missing\n", f)
			continue
		}
		fmt.Fprintf(&b, "  %s — %d bytes, modified %s\n", f, info.Size(), info.ModTime().Format(time.RFC3339))
	}

	b.WriteString("\n")

	// Run validation on existing artifacts
	hasErrors := false
	for _, f := range []string{"spec.md", "plan.md", "tasks.md"} {
		path := filepath.Join(specDir, f)
		if _, err := os.Stat(path); os.IsNotExist(err) {
			continue
		}
		data, err := os.ReadFile(path)
		if err != nil {
			continue
		}
		var vr spec.ValidationResult
		switch f {
		case "spec.md":
			vr = spec.ValidateSpec(string(data))
		case "plan.md":
			vr = spec.ValidatePlan(string(data))
		case "tasks.md":
			vr = spec.ValidateTasks(string(data))
		}
		if len(vr.Issues) > 0 {
			hasErrors = true
			fmt.Fprintf(&b, "  %s validation:\n", f)
			for _, iss := range vr.Issues {
				icon := "i"
				switch iss.Level {
				case spec.ValidationError:
					icon = "x"
				case spec.ValidationWarning:
					icon = "!"
				}
				fmt.Fprintf(&b, "    %s [%s] %s\n", icon, iss.Code, iss.Message)
			}
		}
	}
	if !hasErrors {
		b.WriteString("  All artifacts validated clean.\n")
	}

	// Definition of Done check
	b.WriteString("\nDefinition of Done:\n")
	dod := checkDefinitionOfDone(specDir)
	for _, item := range dod {
		icon := "+"
		if !item.Done {
			icon = "x"
		}
		fmt.Fprintf(&b, "  %s %s\n", icon, item.Description)
	}

	return strings.TrimSpace(b.String()), nil
}

type dodItem struct {
	Description string
	Done        bool
}

func checkDefinitionOfDone(specDir string) []dodItem {
	var items []dodItem

	// Check spec.md exists and is non-empty
	specContent := readFileStr(filepath.Join(specDir, "spec.md"))
	items = append(items, dodItem{
		Description: "spec.md written with requirements",
		Done:        specContent != "" && strings.TrimSpace(specContent) != "",
	})

	// Check plan.md exists and is non-empty
	planContent := readFileStr(filepath.Join(specDir, "plan.md"))
	items = append(items, dodItem{
		Description: "plan.md written with technical approach",
		Done:        planContent != "" && strings.TrimSpace(planContent) != "",
	})

	// Check tasks.md exists and is non-empty
	tasksContent := readFileStr(filepath.Join(specDir, "tasks.md"))
	items = append(items, dodItem{
		Description: "tasks.md written with implementation steps",
		Done:        tasksContent != "" && strings.TrimSpace(tasksContent) != "",
	})

	// Check all tasks are complete
	if tasksContent != "" {
		incomplete := countUncheckedTasks(tasksContent)
		items = append(items, dodItem{
			Description: "All tasks marked complete",
			Done:        incomplete == 0,
		})
	}

	// Check spec has SHALL/MUST requirements
	if specContent != "" {
		items = append(items, dodItem{
			Description: "Spec uses normative language (SHALL/MUST)",
			Done:        strings.Contains(specContent, "SHALL") || strings.Contains(specContent, "MUST"),
		})
	}

	// Check spec has test scenarios
	if specContent != "" {
		hasScenarios := strings.Contains(specContent, "#### Scenario:") || strings.Contains(specContent, "### Scenario:")
		items = append(items, dodItem{
			Description: "Spec includes test scenarios",
			Done:        hasScenarios,
		})
	}

	// Check constitution exists
	constPath := filepath.Join(specDir, "constitution.md")
	_, err := os.Stat(constPath)
	items = append(items, dodItem{
		Description: "Project constitution defined",
		Done:        err == nil,
	})

	return items
}

func countUncheckedTasks(content string) int {
	count := 0
	lines := strings.Split(content, "\n")
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "- [ ]") {
			count++
		}
	}
	return count
}

// SpecEditTool modifies the active spec by applying a delta spec or replacing artifact content.
type SpecEditTool struct{}

func (SpecEditTool) Name() string      { return "SpecEdit" }
func (SpecEditTool) Aliases() []string { return []string{"spec_edit"} }
func (SpecEditTool) Description() string {
	return "Edit the active spec: apply a delta (ADDED/MODIFIED/REMOVED/RENAMED requirements) to spec.md, or replace an artifact entirely with new content. Call this to refine requirements without starting over."
}

func (SpecEditTool) Parameters() map[string]interface{} {
	return map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"artifact": map[string]interface{}{
				"type":        "string",
				"description": "Which file to edit: spec.md, plan.md, tasks.md, or specs.md",
				"enum":        []string{"spec.md", "plan.md", "tasks.md", "specs.md"},
			},
			"delta": map[string]interface{}{
				"type":        "string",
				"description": "Delta spec content with ## ADDED/MODIFIED/REMOVED/RENAMED Requirements sections. Applies structured changes to the artifact.",
			},
			"content": map[string]interface{}{
				"type":        "string",
				"description": "Full replacement content for the artifact (replaces the entire file). Use this instead of delta for wholesale changes.",
			},
		},
		"oneOf": []interface{}{
			map[string]interface{}{"required": []string{"artifact", "delta"}},
			map[string]interface{}{"required": []string{"artifact", "content"}},
		},
	}
}

func (SpecEditTool) Execute(ctx context.Context, input json.RawMessage) (string, error) {
	var p struct {
		Artifact string `json:"artifact"`
		Delta    string `json:"delta"`
		Content  string `json:"content"`
	}
	if err := json.Unmarshal(input, &p); err != nil {
		return "", err
	}

	slug, err := specSlug(ctx)
	if err != nil || slug == "" {
		return "", fmt.Errorf("no active spec — call Specify first")
	}

	dir, err := specsDir()
	if err != nil {
		return "", err
	}
	specDir := filepath.Join(dir, slug)
	path := filepath.Join(specDir, p.Artifact)

	// Ensure directory exists
	if err := os.MkdirAll(specDir, 0o700); err != nil {
		return "", fmt.Errorf("mkdir: %w", err)
	}

	if p.Content != "" {
		// Full replacement
		if err := os.WriteFile(path, []byte(p.Content), 0o600); err != nil {
			return "", fmt.Errorf("write %s: %w", p.Artifact, err)
		}
		// Update stage meta
		_ = spec.WriteStageMeta(slug, "", "", "")
		return fmt.Sprintf("Replaced %s (%d bytes)", path, len(p.Content)), nil
	}

	if p.Delta != "" {
		// Parse delta
		delta, err := spec.ParseDeltaSpec(p.Delta)
		if err != nil {
			return "", fmt.Errorf("invalid delta spec: %w", err)
		}

		// Validate delta
		vr := spec.ValidateDeltaSpec(delta)
		if !vr.Valid {
			return "", fmt.Errorf("delta validation failed:\n%s", vr.Format())
		}

		// Read existing content
		existing, err := os.ReadFile(path)
		if err != nil {
			// File doesn't exist yet — just write the delta as-is
			if writeErr := os.WriteFile(path, []byte(p.Delta), 0o600); writeErr != nil {
				return "", fmt.Errorf("write %s: %w", p.Artifact, writeErr)
			}
			return fmt.Sprintf("Created %s with delta content (%d requirements)", path, len(delta.Requirements)), nil
		}

		// Apply delta merge
		merged, err := spec.ApplyDelta(string(existing), delta)
		if err != nil {
			return "", fmt.Errorf("apply delta: %w", err)
		}

		if err := os.WriteFile(path, []byte(merged), 0o600); err != nil {
			return "", fmt.Errorf("write merged %s: %w", p.Artifact, err)
		}

		_ = spec.WriteStageMeta(slug, "", "", "")
		return fmt.Sprintf("Applied delta to %s (%d requirements processed)", path, len(delta.Requirements)), nil
	}

	return "", fmt.Errorf("specify either delta or content")
}

// SpecListTool lists all spec workflows with their current stage.
type SpecListTool struct{}

func (SpecListTool) Name() string      { return "SpecList" }
func (SpecListTool) Aliases() []string { return []string{"spec_list"} }
func (SpecListTool) Description() string {
	return "List all spec workflows in .hawk/specs/ with their stage and title. Useful for finding existing specs to resume."
}

func (SpecListTool) Parameters() map[string]interface{} {
	return map[string]interface{}{
		"type":       "object",
		"properties": map[string]interface{}{},
	}
}

func (SpecListTool) Execute(_ context.Context, _ json.RawMessage) (string, error) {
	metas, err := spec.ListSpecs()
	if err != nil {
		return "", fmt.Errorf("list specs: %w", err)
	}

	if len(metas) == 0 {
		return "No spec workflows found. Use Specify to start a new one.", nil
	}

	var b strings.Builder
	b.WriteString("Found spec workflows:\n\n")
	for _, m := range metas {
		title := m.Title
		if title == "" {
			title = m.Slug
		}
		stage := spec.StageEnumDisplayName(m.Stage)
		created := ""
		if !m.CreatedAt.IsZero() {
			created = m.CreatedAt.Format("2006-01-02 15:04")
		}
		fmt.Fprintf(&b, "  %s\n", title)
		fmt.Fprintf(&b, "    Slug:   %s\n", m.Slug)
		fmt.Fprintf(&b, "    Stage:  %s\n", stage)
		if created != "" {
			fmt.Fprintf(&b, "    Created: %s\n", created)
		}
		b.WriteString("\n")
	}

	return strings.TrimSpace(b.String()), nil
}

// SpecResetTool resets the active spec workflow, clearing its stage.
type SpecResetTool struct{}

func (SpecResetTool) Name() string      { return "SpecReset" }
func (SpecResetTool) Aliases() []string { return []string{"spec_reset"} }
func (SpecResetTool) Description() string {
	return "Reset/delete the active spec workflow. Clears the spec stage so Write/Edit/Bash follow the trust tier again. Optionally delete the spec artifacts entirely."
}

func (SpecResetTool) Parameters() map[string]interface{} {
	return map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"slug":   map[string]interface{}{"type": "string", "description": "Optional: target a specific slug instead of the active one"},
			"delete": map[string]interface{}{"type": "boolean", "description": "If true, delete the spec artifacts from disk"},
		},
	}
}

func (SpecResetTool) Execute(ctx context.Context, input json.RawMessage) (string, error) {
	var p struct {
		Slug   string `json:"slug"`
		Delete bool   `json:"delete"`
	}
	if input != nil {
		_ = json.Unmarshal(input, &p)
	}

	var slug string
	if p.Slug != "" {
		slug = p.Slug
	} else {
		var err error
		slug, err = specSlug(ctx)
		if err != nil || slug == "" {
			return "No active spec to reset.", nil
		}
	}

	if p.Delete {
		if err := spec.DeleteSpec(slug); err != nil {
			return "", fmt.Errorf("delete spec %s: %w", slug, err)
		}
	} else {
		// Just reset the stage — keep artifacts
		_ = spec.WriteStageMeta(slug, "none", "", "")
	}

	// If this is the active spec, reset the session slug too
	activeSlug, err := specSlug(ctx)
	if err == nil && activeSlug == slug {
		_ = setSpecSlug(ctx, "")
	}

	if p.Delete {
		return fmt.Sprintf("Deleted spec %s entirely.", slug), nil
	}
	return fmt.Sprintf("Reset spec %s — stage cleared, artifacts preserved.", slug), nil
}

// specsDir returns the .hawk/specs directory path.
func specsDir() (string, error) {
	cwd, err := os.Getwd()
	if err != nil {
		return "", err
	}
	return filepath.Join(cwd, ".hawk", "specs"), nil
}

// SpecConfigTool allows the agent to read and update spec configuration.
type SpecConfigTool struct{}

func (SpecConfigTool) Name() string      { return "SpecConfig" }
func (SpecConfigTool) Aliases() []string { return []string{"spec_config"} }
func (SpecConfigTool) Description() string {
	return "Read or update spec configuration (language, framework, methodology, architecture). Call this to check preferences before writing specs, or to update config as needed."
}

func (SpecConfigTool) Parameters() map[string]interface{} {
	return map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"action": map[string]interface{}{
				"type":        "string",
				"description": "'get' to read config, 'set' to update, 'list' to show available fields",
				"enum":        []string{"get", "set", "list"},
			},
			"field": map[string]interface{}{
				"type":        "string",
				"description": "The config field to update (required for 'set'). One of: language, framework, methodology, architecture, repo_structure, custom_prompt",
			},
			"value": map[string]interface{}{
				"type":        "string",
				"description": "The new value for the field. Use 'ai' to let the AI decide.",
			},
		},
		"required": []string{"action"},
	}
}

func (SpecConfigTool) Execute(_ context.Context, input json.RawMessage) (string, error) {
	var p struct {
		Action string `json:"action"`
		Field  string `json:"field"`
		Value  string `json:"value"`
	}
	if err := json.Unmarshal(input, &p); err != nil {
		return "", err
	}

	switch p.Action {
	case "get":
		cfg := spec.LoadSpecConfig()
		return "Current spec configuration:\n" + cfg.Format(), nil

	case "list":
		var b strings.Builder
		b.WriteString("Available config fields:\n\n")
		for _, f := range spec.SpecConfigFields() {
			fmt.Fprintf(&b, "  %s (%s)\n", f.Label, f.Key)
			fmt.Fprintf(&b, "    %s\n", f.Help)
			if len(f.Examples) > 0 {
				fmt.Fprintf(&b, "    Examples: %s\n", strings.Join(f.Examples, ", "))
			}
			b.WriteString("\n")
		}
		b.WriteString("Use `SpecConfig` with action='set', field='key', value='your value'.\n")
		b.WriteString("Use value 'ai' to let the AI decide any field.")
		return strings.TrimSpace(b.String()), nil

	case "set":
		if p.Field == "" {
			return "", fmt.Errorf("field is required for 'set' action")
		}
		valid := false
		for _, f := range spec.SpecConfigFields() {
			if f.Key == p.Field {
				valid = true
				break
			}
		}
		if !valid {
			return "", fmt.Errorf("unknown field %q. Use action='list' to see available fields", p.Field)
		}

		cfg := spec.LoadSpecConfig()
		switch p.Field {
		case "language":
			cfg.Language = p.Value
		case "framework":
			cfg.Framework = p.Value
		case "methodology":
			cfg.Methodology = p.Value
		case "architecture":
			cfg.Architecture = p.Value
		case "repo_structure":
			cfg.RepoStructure = p.Value
		case "custom_prompt":
			cfg.CustomPrompt = p.Value
		}

		if err := spec.SaveSpecConfig(cfg); err != nil {
			return "", fmt.Errorf("save config: %w", err)
		}
		return fmt.Sprintf("Updated spec config field %q to %q.\n\n%s", p.Field, p.Value, cfg.Format()), nil

	default:
		return "", fmt.Errorf("unknown action %q. Use 'get', 'set', or 'list'", p.Action)
	}
}

func firstLine(s string) string {
	if i := strings.IndexByte(s, '\n'); i >= 0 {
		return s[:i]
	}
	return s
}
