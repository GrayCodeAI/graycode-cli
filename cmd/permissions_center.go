package cmd

import (
	"fmt"
	"strings"

	hawkconfig "github.com/GrayCodeAI/hawk/internal/config"
	"github.com/GrayCodeAI/hawk/internal/engine"
	tea "github.com/charmbracelet/bubbletea"
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

func normalizePermissionMode(raw string) (engine.PermissionMode, string, bool) {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case "", "default", "normal", "standard":
		return engine.PermissionModeDefault, "Default", true
	case "acceptedits", "edit", "edits":
		return engine.PermissionModeAcceptEdits, "Accept Edits", true
	case "bypasspermissions", "bypass", "full-auto", "fullauto":
		return engine.PermissionModeBypassPermissions, "Bypass Permissions", true
	case "dontask", "deny", "blocked":
		return engine.PermissionModeDontAsk, "Don't Ask", true
	case "plan":
		return engine.PermissionModePlan, "Plan", true
	default:
		return "", "", false
	}
}

func permissionModeSummary(mode engine.PermissionMode) string {
	switch mode {
	case engine.PermissionModeAcceptEdits:
		return "file edits auto-approve even when commands still ask"
	case engine.PermissionModeBypassPermissions:
		return "all permission prompts are bypassed"
	case engine.PermissionModeDontAsk:
		return "mutating and gated tools are blocked"
	case engine.PermissionModePlan:
		return "read-only planning workflow; write and mutating actions are denied"
	default:
		return "normal approval flow uses tier, sandbox, and rules"
	}
}

func permissionCommandHelp() string {
	return "Permission Center\n" +
		"  /permissions                          Show current tier, sandbox, and rules\n" +
		"  /permissions tier <scout|builder|operator|autonomous>\n" +
		"  /permissions sandbox <strict|workspace|off>\n" +
		"  /permissions mode <default|edits|bypass|dontask|plan>\n" +
		"  /permissions allow <rule>\n" +
		"  /permissions deny <rule>\n" +
		"  /permissions rules                    Show current allow/deny rules\n" +
		"  /permissions rules clear              Clear current session rules\n" +
		"  /permissions reset                    Reset tier, sandbox, mode, and rules\n" +
		"  /permissions save [project|global]    Persist the current policy"
}

func permissionCenterSummary(m *chatModel) string {
	if m == nil || m.session == nil {
		return "Permission Center unavailable."
	}
	level := effectivePermissionTier(m.session)
	tier := autonomyTierName(level)
	_, sandboxLabel, _ := normalizePermissionSandbox(effectivePermissionSandbox(m.settings))
	allowRules := effectiveAllowRules(m.settings)
	denyRules := effectiveDenyRules(m.settings)
	var b strings.Builder
	b.WriteString("Permission Center\n")
	b.WriteString(fmt.Sprintf("  Tier: %s\n", tier))
	b.WriteString(fmt.Sprintf("  Sandbox: %s\n", sandboxLabel))
	b.WriteString(fmt.Sprintf("  Mode: %s\n", permissionModeLabel(m.session)))
	b.WriteString(fmt.Sprintf("  Rules: %d allow, %d deny\n", len(allowRules), len(denyRules)))
	b.WriteString(fmt.Sprintf("  Behavior: %s\n", permissionBehaviorSummary(level)))
	b.WriteString(fmt.Sprintf("  Mode behavior: %s\n", permissionModeSummary(m.session.ModeValue())))
	if len(allowRules) > 0 {
		b.WriteString("  Allow: " + strings.Join(allowRules, ", ") + "\n")
	}
	if len(denyRules) > 0 {
		b.WriteString("  Deny: " + strings.Join(denyRules, ", ") + "\n")
	}
	b.WriteString("\n")
	b.WriteString(permissionCommandHelp())
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
		if sess.Perm != nil {
			sess.Perm.Memory = mem
		}
	}
	mem.Reset()
	if sess.Perm != nil && sess.Perm.Memory == nil {
		sess.Perm.Memory = mem
	}
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
		scope = "project"
	}
	settings.Autonomy = permissionTierSettingValue(level)
	settings.Sandbox = effectivePermissionSandbox(settings)
	settings.AllowedTools = dedupeStrings(settings.AllowedTools)
	settings.DisallowedTools = dedupeStrings(settings.DisallowedTools)

	switch scope {
	case "project":
		target := hawkconfig.LoadProjectSettings()
		target.AutoAllow = append([]string{}, settings.AutoAllow...)
		target.AllowedTools = append([]string{}, settings.AllowedTools...)
		target.DisallowedTools = append([]string{}, settings.DisallowedTools...)
		target.Autonomy = settings.Autonomy
		target.Sandbox = settings.Sandbox
		if err := hawkconfig.SaveProject(target); err != nil {
			return "", err
		}
		return ".hawk/settings.json", nil
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
		return "~/.hawk/settings.json", nil
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
	_ = m.session.SetPermissionMode(string(engine.PermissionModeDefault))
	rebuildSessionPermissionRules(m.session, m.settings)
}

func (m *chatModel) handlePermissionsCommand(parts []string) (chatModel, tea.Cmd) {
	if m.session == nil {
		m.messages = append(m.messages, displayMsg{role: "error", content: "No active session."})
		return *m, nil
	}
	if len(parts) == 1 {
		m.messages = append(m.messages, displayMsg{role: "system", content: permissionCenterSummary(m)})
		return *m, nil
	}

	switch strings.ToLower(strings.TrimSpace(parts[1])) {
	case "help", "status":
		m.messages = append(m.messages, displayMsg{role: "system", content: permissionCenterSummary(m)})
	case "mode":
		if len(parts) < 3 {
			m.messages = append(m.messages, displayMsg{role: "system", content: fmt.Sprintf("Current mode: %s\nBehavior: %s\nUsage: /permissions mode <default|edits|bypass|dontask|plan>", permissionModeLabel(m.session), permissionModeSummary(m.session.ModeValue()))})
			return *m, nil
		}
		mode, label, ok := normalizePermissionMode(parts[2])
		if !ok {
			m.messages = append(m.messages, displayMsg{role: "error", content: "Valid modes: default, edits, bypass, dontask, plan"})
			return *m, nil
		}
		if err := m.session.SetPermissionMode(string(mode)); err != nil {
			m.messages = append(m.messages, displayMsg{role: "error", content: fmt.Sprintf("Mode change failed: %v", err)})
			return *m, nil
		}
		m.messages = append(m.messages, displayMsg{role: "system", content: fmt.Sprintf("Permission mode → %s\nBehavior: %s", label, permissionModeSummary(mode))})
	case "tier":
		if len(parts) < 3 {
			m.messages = append(m.messages, displayMsg{role: "error", content: "Usage: /permissions tier <scout|builder|operator|autonomous>"})
			return *m, nil
		}
		level, label, ok := normalizePermissionTier(parts[2])
		if !ok {
			m.messages = append(m.messages, displayMsg{role: "error", content: "Valid tiers: scout, builder, operator, autonomous"})
			return *m, nil
		}
		m.session.PermSvc().SetAutonomy(level)
		m.settings.Autonomy = permissionTierSettingValue(level)
		m.messages = append(m.messages, displayMsg{role: "system", content: fmt.Sprintf("Permission tier → %s\nBehavior: %s", label, permissionBehaviorSummary(level))})
	case "sandbox":
		if len(parts) < 3 {
			_, label, _ := normalizePermissionSandbox(effectivePermissionSandbox(m.settings))
			m.messages = append(m.messages, displayMsg{role: "system", content: "Current sandbox: " + label + "\nUsage: /permissions sandbox <strict|workspace|off>"})
			return *m, nil
		}
		mode, label, ok := normalizePermissionSandbox(parts[2])
		if !ok {
			m.messages = append(m.messages, displayMsg{role: "error", content: "Valid sandbox modes: strict, workspace, off"})
			return *m, nil
		}
		m.settings.Sandbox = mode
		sandboxFlag = mode
		m.messages = append(m.messages, displayMsg{role: "system", content: fmt.Sprintf("Sandbox preference → %s\nApplies to host Bash policy; container isolation is unchanged until restart.", label)})
	case "allow":
		if len(parts) < 3 {
			m.messages = append(m.messages, displayMsg{role: "error", content: "Usage: /permissions allow <rule>  e.g. /permissions allow Bash(git:*)"})
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
			m.messages = append(m.messages, displayMsg{role: "error", content: "Usage: /permissions deny <rule>  e.g. /permissions deny Bash(rm -rf *)"})
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
			m.messages = append(m.messages, displayMsg{role: "system", content: "Permission rules cleared for the current session."})
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
		m.messages = append(m.messages, displayMsg{role: "system", content: "Permission policy saved to " + path})
	case "reset":
		resetPermissionCenter(m)
		m.messages = append(m.messages, displayMsg{role: "system", content: "Permission Center reset to defaults.\n" + permissionCenterSummary(m)})
	default:
		m.messages = append(m.messages, displayMsg{role: "system", content: permissionCommandHelp()})
	}
	return *m, nil
}
