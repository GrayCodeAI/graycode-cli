package cmd

import (
	"fmt"
	"strings"

	tea "charm.land/bubbletea/v2"
	hawkconfig "github.com/GrayCodeAI/hawk/internal/config"
	"github.com/GrayCodeAI/hawk/internal/engine"
)

const defaultPermissionSandbox = "workspace"

func normalizePermissionTier(raw string) (engine.AutonomyLevel, string, bool) {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case "scout", "basic", "read":
		return engine.AutonomyBasic, "Scout", true
	case "builder", "semi", "edit":
		return engine.AutonomySemi, "Builder", true
	case "operator", "full", "run":
		return engine.AutonomyFull, "Operator", true
	case "autonomous", "yolo", "auto":
		return engine.AutonomyYOLO, "Autonomous", true
	default:
		return 0, "", false
	}
}

func permissionTierSettingValue(level engine.AutonomyLevel) int {
	switch level {
	case engine.AutonomyBasic:
		return 1
	case engine.AutonomySemi:
		return 2
	case engine.AutonomyFull:
		return 3
	case engine.AutonomyYOLO:
		return 4
	default:
		return 0
	}
}

func effectivePermissionTier(sess *engine.Session) engine.AutonomyLevel {
	if sess == nil {
		return DefaultContainerAutonomy
	}
	perms := sess.PermSvc()
	if perms == nil {
		return DefaultContainerAutonomy
	}
	if perms.Autonomy() == 0 {
		return DefaultContainerAutonomy
	}
	return perms.Autonomy()
}

func normalizePermissionSandbox(raw string) (string, string, bool) {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case "":
		return defaultPermissionSandbox, "Workspace", true
	case "strict":
		return "strict", "Strict", true
	case "workspace":
		return "workspace", "Workspace", true
	case "off":
		return "off", "Off", true
	default:
		return "", "", false
	}
}

func effectivePermissionSandbox(settings hawkconfig.Settings) string {
	if normalized, _, ok := normalizePermissionSandbox(sandboxFlag); ok && strings.TrimSpace(sandboxFlag) != "" {
		return normalized
	}
	if normalized, _, ok := normalizePermissionSandbox(settings.Sandbox); ok {
		return normalized
	}
	return defaultPermissionSandbox
}

func permissionBehaviorSummary(level engine.AutonomyLevel) string {
	switch level {
	case engine.AutonomyBasic:
		return "reads auto-approve; edits and commands ask first"
	case engine.AutonomySemi:
		return "reads and file changes auto-approve; commands ask first"
	case engine.AutonomyFull:
		return "reads, edits, and normal commands auto-run; risky actions ask first"
	case engine.AutonomyYOLO:
		return "minimal prompts; only highest-risk actions stop"
	default:
		return "reads and file changes auto-approve; commands ask first"
	}
}

// specStageLabel returns the display label for the current spec workflow stage.
func specStageLabel(sess *engine.Session) string {
	return specStageDisplayName(currentSpecStage(sess))
}

// currentSpecStage returns the session's active spec stage, or
// SpecStageNone if the session (or its permission engine) isn't set up yet.
func currentSpecStage(sess *engine.Session) engine.SpecStage {
	if sess == nil || sess.PermSvc() == nil {
		return engine.SpecStageNone
	}
	return sess.PermSvc().SpecStage()
}

// currentDryRun returns whether the session's dry-run kill switch is
// active, or false if the session (or its permission engine) isn't set up
// yet — mirrors currentSpecStage's nil-safety, since PermSvc() can return
// nil for sessions built via a raw struct literal (e.g. in tests) rather
// than NewSession.
func currentDryRun(sess *engine.Session) bool {
	if sess == nil || sess.PermSvc() == nil {
		return false
	}
	return sess.PermSvc().DryRun()
}

func autonomyCommandHelp() string {
	return "Autonomy Center\n" +
		"  /autonomy                          Show current tier, sandbox, spec stage, and rules\n" +
		"  /autonomy tier <scout|builder|operator|autonomous>\n" +
		"  /autonomy sandbox <strict|workspace|off>\n" +
		"                                      Permission policy inside the Docker sandbox\n" +
		"                                      (strict=always ask, workspace=allow project files, off=allow all)\n" +
		"  /autonomy dry-run <on|off>         Deny every tool call unconditionally (kill switch)\n" +
		"  /autonomy allow <rule>\n" +
		"  /autonomy deny <rule>\n" +
		"  /autonomy rules                    Show current allow/deny rules\n" +
		"  /autonomy rules clear              Clear current session rules\n" +
		"  /autonomy reset                    Reset tier, sandbox, dry-run, and rules\n" +
		"  /autonomy save [project|global]    Persist the current policy\n" +
		"\n" +
		"Note: Docker isolation is always required; this setting controls the\n" +
		"      approval policy applied inside the container.\n" +
		"\n" +
		"For the spec-driven workflow (gates Write/Edit/Bash until approved), see /spec."
}

func autonomyCenterSummary(m *chatModel) string {
	if m == nil || m.session == nil {
		return "Autonomy Center unavailable."
	}
	level := effectivePermissionTier(m.session)
	tier := autonomyTierName(level)
	_, sandboxLabel, _ := normalizePermissionSandbox(effectivePermissionSandbox(m.settings))
	allowRules := effectiveAllowRules(m.settings)
	denyRules := effectiveDenyRules(m.settings)
	var b strings.Builder
	b.WriteString("Autonomy Center\n")
	b.WriteString(fmt.Sprintf("  Tier: %s\n", tier))
	b.WriteString(fmt.Sprintf("  Permission sandbox: %s\n", sandboxLabel))
	b.WriteString(fmt.Sprintf("  Spec stage: %s\n", specStageLabel(m.session)))
	if currentDryRun(m.session) {
		b.WriteString("  Dry-run: ON — every tool call is being denied unconditionally\n")
	}
	b.WriteString(fmt.Sprintf("  Rules: %d allow, %d deny\n", len(allowRules), len(denyRules)))
	b.WriteString(fmt.Sprintf("  Behavior: %s\n", permissionBehaviorSummary(level)))
	if len(allowRules) > 0 {
		b.WriteString("  Allow: " + strings.Join(allowRules, ", ") + "\n")
	}
	if len(denyRules) > 0 {
		b.WriteString("  Deny: " + strings.Join(denyRules, ", ") + "\n")
	}
	b.WriteString("\n")
	b.WriteString(autonomyCommandHelp())
	return strings.TrimRight(b.String(), "\n")
}

func permissionRulesSummary(m *chatModel) string {
	if m == nil {
		return "No active permission state."
	}
	allowRules := effectiveAllowRules(m.settings)
	denyRules := effectiveDenyRules(m.settings)
	var b strings.Builder
	b.WriteString("Permission Rules\n")
	if len(allowRules) == 0 {
		b.WriteString("  Allow: none\n")
	} else {
		b.WriteString("  Allow:\n")
		for _, rule := range allowRules {
			b.WriteString("    - " + rule + "\n")
		}
	}
	if len(denyRules) == 0 {
		b.WriteString("  Deny: none")
	} else {
		b.WriteString("  Deny:\n")
		for _, rule := range denyRules {
			b.WriteString("    - " + rule + "\n")
		}
	}
	return strings.TrimRight(b.String(), "\n")
}

func effectiveAllowRules(settings hawkconfig.Settings) []string {
	var rules []string
	rules = append(rules, settings.AutoAllow...)
	rules = append(rules, settings.AllowedTools...)
	rules = append(rules, parseToolListFromCLI(allowedToolsFlag)...)
	return dedupeStrings(rules)
}

func effectiveDenyRules(settings hawkconfig.Settings) []string {
	rules := append([]string{}, settings.DisallowedTools...)
	rules = append(rules, parseToolListFromCLI(disallowedToolsFlag)...)
	return dedupeStrings(rules)
}

func dedupeStrings(values []string) []string {
	var out []string
	seen := make(map[string]bool, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" || seen[value] {
			continue
		}
		seen[value] = true
		out = append(out, value)
	}
	return out
}

func rebuildSessionPermissionRules(sess *engine.Session, settings hawkconfig.Settings) {
	if sess == nil {
		return
	}
	mem := sess.PermSvc().Memory()
	if mem == nil {
		mem = engine.NewPermissionMemory()
		sess.PermSvc().SetMemory(mem)
	}
	mem.Reset()
	for _, spec := range settings.AutoAllow {
		mem.AllowSpec(spec)
	}
	for _, spec := range settings.AllowedTools {
		mem.AllowSpec(spec)
	}
	for _, spec := range settings.DisallowedTools {
		mem.DenySpec(spec)
	}
	for _, spec := range parseToolListFromCLI(allowedToolsFlag) {
		mem.AllowSpec(spec)
	}
	for _, spec := range parseToolListFromCLI(disallowedToolsFlag) {
		mem.DenySpec(spec)
	}
}

func savePermissionSettings(scope string, settings hawkconfig.Settings, level engine.AutonomyLevel) (string, error) {
	scope = strings.ToLower(strings.TrimSpace(scope))
	if scope == "" {
		scope = "global"
	}
	settings.Autonomy = permissionTierSettingValue(level)
	settings.Sandbox = effectivePermissionSandbox(settings)
	settings.AllowedTools = dedupeStrings(settings.AllowedTools)
	settings.DisallowedTools = dedupeStrings(settings.DisallowedTools)

	switch scope {
	case "project":
		return "", fmt.Errorf("project-local settings writes are disabled; use scope \"global\" or an explicit --settings file")
	case "global":
		target := hawkconfig.LoadGlobalSettings()
		target.AutoAllow = append([]string{}, settings.AutoAllow...)
		target.AllowedTools = append([]string{}, settings.AllowedTools...)
		target.DisallowedTools = append([]string{}, settings.DisallowedTools...)
		target.Autonomy = settings.Autonomy
		target.Sandbox = settings.Sandbox
		if err := hawkconfig.SaveGlobal(target); err != nil {
			return "", err
		}
		return "user settings", nil
	default:
		return "", fmt.Errorf("valid save scopes: project, global")
	}
}

func resetPermissionCenter(m *chatModel) {
	if m == nil || m.session == nil {
		return
	}
	m.session.PermSvc().SetAutonomy(DefaultContainerAutonomy)
	m.settings.Autonomy = permissionTierSettingValue(DefaultContainerAutonomy)
	m.settings.Sandbox = defaultPermissionSandbox
	sandboxFlag = defaultPermissionSandbox
	m.settings.AutoAllow = nil
	m.settings.AllowedTools = nil
	m.settings.DisallowedTools = nil
	m.session.PermSvc().SetSpecStage(engine.SpecStageNone)
	m.session.PermSvc().SetDryRun(false)
	rebuildSessionPermissionRules(m.session, m.settings)
}

func (m *chatModel) handleAutonomyCommand(parts []string) (chatModel, tea.Cmd) {
	if m.session == nil {
		m.messages = append(m.messages, displayMsg{role: "error", content: "No active session."})
		return *m, nil
	}
	if len(parts) == 1 {
		if m.autonomyPicker == nil {
			m.autonomyPicker = NewAutonomyPicker(m.width)
		}
		m.autonomyPicker.Open(effectivePermissionTier(m.session))
		return *m, nil
	}

	switch strings.ToLower(strings.TrimSpace(parts[1])) {
	case "help", "status":
		m.messages = append(m.messages, displayMsg{role: "system", content: autonomyCenterSummary(m)})
	case "tier":
		if len(parts) < 3 {
			m.messages = append(m.messages, displayMsg{role: "error", content: "Usage: /autonomy tier <scout|builder|operator|autonomous>"})
			return *m, nil
		}
		level, label, ok := normalizePermissionTier(parts[2])
		if !ok {
			m.messages = append(m.messages, displayMsg{role: "error", content: "Valid tiers: scout, builder, operator, autonomous"})
			return *m, nil
		}
		m.session.PermSvc().SetAutonomy(level)
		m.settings.Autonomy = permissionTierSettingValue(level)
		m.messages = append(m.messages, displayMsg{role: "system", content: fmt.Sprintf("Autonomy tier → %s\nBehavior: %s", label, permissionBehaviorSummary(level))})
	case "sandbox":
		if len(parts) < 3 {
			_, label, _ := normalizePermissionSandbox(effectivePermissionSandbox(m.settings))
			m.messages = append(m.messages, displayMsg{role: "system", content: "Permission sandbox: " + label + "\nUsage: /autonomy sandbox <strict|workspace|off>"})
			return *m, nil
		}
		mode, label, ok := normalizePermissionSandbox(parts[2])
		if !ok {
			m.messages = append(m.messages, displayMsg{role: "error", content: "Valid permission sandbox modes: strict, workspace, off"})
			return *m, nil
		}
		m.settings.Sandbox = mode
		sandboxFlag = mode
		m.messages = append(m.messages, displayMsg{role: "system", content: fmt.Sprintf("Permission sandbox → %s\nControls approval policy inside the mandatory Docker container.", label)})
	case "dry-run":
		if len(parts) < 3 {
			state := "off"
			if currentDryRun(m.session) {
				state = "on"
			}
			m.messages = append(m.messages, displayMsg{role: "system", content: "Dry-run: " + state + "\nUsage: /autonomy dry-run <on|off>"})
			return *m, nil
		}
		switch strings.ToLower(strings.TrimSpace(parts[2])) {
		case "on", "true", "1":
			m.session.PermSvc().SetDryRun(true)
			m.messages = append(m.messages, displayMsg{role: "system", content: "Dry-run → on. Every tool call will be denied unconditionally, regardless of tier or spec stage."})
		case "off", "false", "0":
			m.session.PermSvc().SetDryRun(false)
			m.messages = append(m.messages, displayMsg{role: "system", content: "Dry-run → off. Normal tier/spec-gate rules apply again."})
		default:
			m.messages = append(m.messages, displayMsg{role: "error", content: "Usage: /autonomy dry-run <on|off>"})
		}
	case "allow":
		if len(parts) < 3 {
			m.messages = append(m.messages, displayMsg{role: "error", content: "Usage: /autonomy allow <rule>  e.g. /autonomy allow Bash(git:*)"})
			return *m, nil
		}
		specs := parseToolListFromCLI([]string{strings.Join(parts[2:], " ")})
		if len(specs) == 0 {
			m.messages = append(m.messages, displayMsg{role: "error", content: "No valid allow rule provided."})
			return *m, nil
		}
		m.settings.AllowedTools = dedupeStrings(append(m.settings.AllowedTools, specs...))
		rebuildSessionPermissionRules(m.session, m.settings)
		m.messages = append(m.messages, displayMsg{role: "system", content: "Allow rules updated.\n" + permissionRulesSummary(m)})
	case "deny":
		if len(parts) < 3 {
			m.messages = append(m.messages, displayMsg{role: "error", content: "Usage: /autonomy deny <rule>  e.g. /autonomy deny Bash(rm -rf *)"})
			return *m, nil
		}
		specs := parseToolListFromCLI([]string{strings.Join(parts[2:], " ")})
		if len(specs) == 0 {
			m.messages = append(m.messages, displayMsg{role: "error", content: "No valid deny rule provided."})
			return *m, nil
		}
		m.settings.DisallowedTools = dedupeStrings(append(m.settings.DisallowedTools, specs...))
		rebuildSessionPermissionRules(m.session, m.settings)
		m.messages = append(m.messages, displayMsg{role: "system", content: "Deny rules updated.\n" + permissionRulesSummary(m)})
	case "rules":
		if len(parts) > 2 && strings.EqualFold(strings.TrimSpace(parts[2]), "clear") {
			m.settings.AutoAllow = nil
			m.settings.AllowedTools = nil
			m.settings.DisallowedTools = nil
			rebuildSessionPermissionRules(m.session, m.settings)
			m.messages = append(m.messages, displayMsg{role: "system", content: "Autonomy rules cleared for the current session."})
			return *m, nil
		}
		m.messages = append(m.messages, displayMsg{role: "system", content: permissionRulesSummary(m)})
	case "save":
		scope := ""
		if len(parts) > 2 {
			scope = parts[2]
		}
		path, err := savePermissionSettings(scope, m.settings, effectivePermissionTier(m.session))
		if err != nil {
			m.messages = append(m.messages, displayMsg{role: "error", content: fmt.Sprintf("Save failed: %v", err)})
			return *m, nil
		}
		m.messages = append(m.messages, displayMsg{role: "system", content: "Autonomy policy saved to " + path})
	case "reset":
		resetPermissionCenter(m)
		m.messages = append(m.messages, displayMsg{role: "system", content: "Autonomy Center reset to defaults.\n" + autonomyCenterSummary(m)})
	default:
		m.messages = append(m.messages, displayMsg{role: "system", content: autonomyCommandHelp()})
	}
	return *m, nil
}
