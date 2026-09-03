package cmd

import (
	"fmt"
	"strconv"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"
	graycodeconfig "github.com/GrayCodeAI/graycode-cli/internal/config"
	"github.com/GrayCodeAI/graycode-cli/internal/engine"
	"github.com/GrayCodeAI/graycode-cli/internal/sandbox"
)

const defaultPermissionSandbox = "workspace"

func normalizePermissionTier(raw string) (engine.AutonomyLevel, string, bool) {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case "always_ask", "always-ask", "supervised", "ask":
		return engine.AutonomySupervised, "Always Ask", true
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
	if perms.Autonomy() == 0 && !perms.AutonomyExplicit() {
		return DefaultContainerAutonomy
	}
	return perms.Autonomy()
}

// containerNetworkFlag is the CLI override for container network mode.
var containerNetworkFlag string

func normalizeContainerNetwork(raw string) (string, bool) {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case "none", "bridge", "isolated":
		return strings.ToLower(strings.TrimSpace(raw)), true
	default:
		return "", false
	}
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

func effectivePermissionSandbox(settings graycodeconfig.Settings) string {
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
	case engine.AutonomySupervised:
		return "prompts for every tool call"
	case engine.AutonomyBasic:
		return "reads auto-approve; edits and commands ask first"
	case engine.AutonomySemi:
		return "reads and file changes auto-approve; commands ask first"
	case engine.AutonomyFull:
		return "reads, edits, and normal commands auto-run; risky actions ask first"
	case engine.AutonomyYOLO:
		return "minimal prompts; only highest-risk actions stop"
	default:
		return "prompts for every tool call"
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

// parseBypassFlags extracts --scope, --for, --reason from /autonomy bypass args.
func parseBypassFlags(args []string) (scope []string, expires time.Time, reason string) {
	for _, a := range args {
		a = strings.TrimSpace(a)
		switch {
		case strings.HasPrefix(a, "--scope="):
			v := strings.TrimPrefix(a, "--scope=")
			for _, s := range strings.Split(v, ",") {
				s = strings.TrimSpace(s)
				if s != "" {
					scope = append(scope, s)
				}
			}
		case strings.HasPrefix(a, "--for="):
			v := strings.TrimPrefix(a, "--for=")
			if d, err := time.ParseDuration(v); err == nil {
				expires = time.Now().Add(d)
			}
		case strings.HasPrefix(a, "--reason="):
			reason = strings.Trim(strings.TrimPrefix(a, "--reason="), `"`)
		}
	}
	return scope, expires, reason
}

// markOverridden returns " *" if the flag was explicitly overridden by the
// user, so the profile display can mark customized flags.
func markOverridden(profile *engine.AutonomyProfile, flag string) string {
	if profile != nil && profile.IsOverridden(flag) {
		return " *"
	}
	return ""
}

func autonomyCommandHelp() string {
	return "Autonomy Center\n" +
		"  /autonomy                          Show current tier, sandbox, spec stage, and rules\n" +
		"  /autonomy tier <scout|builder|operator|autonomous>\n" +
		"  /autonomy sandbox <strict|workspace|off>\n" +
		"                                      Permission policy inside the Docker sandbox\n" +
		"                                      (strict=always ask, workspace=allow project files, off=allow all)\n" +
		"  /autonomy bypass <on|off>          Break-glass bypass (optionally --scope --for --reason)\n" +
		"  /autonomy dry-run <on|off>         Deny every tool call unconditionally (kill switch)\n" +
		"  /autonomy allow <rule>\n" +
		"  /autonomy deny <rule>\n" +
		"  /autonomy rules                    Show current allow/deny rules\n" +
		"  /autonomy rules clear              Clear current session rules\n" +
		"  /autonomy profile [flag=<on|off>]  Show or override per-flag autonomy (auto_execute_bash, auto_network)\n" +
		"  /autonomy audit                    Show recent permission decisions with reasons\n" +
		"  /autonomy metrics                  Show permission decision counters\n" +
		"  /autonomy grants cleanup           Rebuild active rules from settings (clear learned)\n" +
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
	var b strings.Builder
	b.WriteString("Permission Rules\n")

	// Show unified grants from the engine (Memory + AutoMode + ApprovalStore)
	// when available, with source labels. Fall back to settings-based rules.
	if m.session != nil && m.session.PermSvc() != nil && m.session.PermSvc().Engine() != nil {
		pe := m.session.PermSvc().Engine()
		if pe.UnifiedGrants != nil {
			grants := pe.UnifiedGrants.All(time.Now())
			var allows, denies []string
			for _, g := range grants {
				label := g.Tool + "(" + g.Pattern + ") [" + g.Source.String() + "]"
				if g.Label != "" {
					label += " (" + g.Label + ")"
				}
				if g.Allow {
					allows = append(allows, label)
				} else {
					denies = append(denies, label)
				}
			}
			if len(allows) == 0 {
				b.WriteString("  Allow: none\n")
			} else {
				b.WriteString("  Allow:\n")
				for _, r := range allows {
					b.WriteString("    - " + r + "\n")
				}
			}
			if len(denies) == 0 {
				b.WriteString("  Deny: none\n")
			} else {
				b.WriteString("  Deny:\n")
				for _, r := range denies {
					b.WriteString("    - " + r + "\n")
				}
			}
			return strings.TrimRight(b.String(), "\n")
		}
	}

	// Fallback: settings-based rules.
	allowRules := effectiveAllowRules(m.settings)
	denyRules := effectiveDenyRules(m.settings)
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

func effectiveAllowRules(settings graycodeconfig.Settings) []string {
	var rules []string
	rules = append(rules, settings.AutoAllow...)
	rules = append(rules, settings.AllowedTools...)
	rules = append(rules, parseToolListFromCLI(allowedToolsFlag)...)
	return dedupeStrings(rules)
}

func effectiveDenyRules(settings graycodeconfig.Settings) []string {
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

func rebuildSessionPermissionRules(sess *engine.Session, settings graycodeconfig.Settings) {
	if sess == nil {
		return
	}
	perm := sess.PermSvc()
	if perm == nil {
		return
	}
	mem := perm.Memory()
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

func savePermissionSettings(scope string, settings graycodeconfig.Settings, level engine.AutonomyLevel) (string, error) {
	scope = strings.ToLower(strings.TrimSpace(scope))
	if scope == "" {
		scope = "global"
	}
	settings.Autonomy = permissionTierSettingValue(level)
	settings.AutonomyExplicit = true
	settings.Sandbox = effectivePermissionSandbox(settings)
	settings.AllowedTools = dedupeStrings(settings.AllowedTools)
	settings.DisallowedTools = dedupeStrings(settings.DisallowedTools)

	switch scope {
	case "project":
		return "", fmt.Errorf("project-local settings writes are disabled; use scope \"global\" or an explicit --settings file")
	case "global":
		target := graycodeconfig.LoadGlobalSettings()
		target.AutoAllow = append([]string{}, settings.AutoAllow...)
		target.AllowedTools = append([]string{}, settings.AllowedTools...)
		target.DisallowedTools = append([]string{}, settings.DisallowedTools...)
		target.Autonomy = settings.Autonomy
		target.AutonomyExplicit = true
		target.Sandbox = settings.Sandbox
		if err := graycodeconfig.SaveGlobal(target); err != nil {
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
	m.session.PermSvc().SetSandboxMode(sandbox.ParseMode(defaultPermissionSandbox))
	m.settings.Autonomy = permissionTierSettingValue(DefaultContainerAutonomy)
	m.settings.AutonomyExplicit = true
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
		m.settings.AutonomyExplicit = true
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
		m.session.PermSvc().SetSandboxMode(sandbox.ParseMode(mode))
		m.messages = append(m.messages, displayMsg{role: "system", content: fmt.Sprintf("Permission sandbox → %s\nControls tool filesystem/process policy independently of the autonomy tier.", label)})
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
			m.messages = append(m.messages, displayMsg{role: "error", content: "Usage: /autonomy allow <rule> [--for=N]  e.g. /autonomy allow Bash(git:*)"})
			return *m, nil
		}
		// Extract optional --for=N before parsing the rule.
		var ruleParts []string
		var forN int
		for _, p := range parts[2:] {
			if strings.HasPrefix(p, "--for=") {
				if n, err := strconv.Atoi(strings.TrimPrefix(p, "--for=")); err == nil && n > 0 {
					forN = n
				}
			} else {
				ruleParts = append(ruleParts, p)
			}
		}
		specs := parseToolListFromCLI([]string{strings.Join(ruleParts, " ")})
		if len(specs) == 0 {
			m.messages = append(m.messages, displayMsg{role: "error", content: "No valid allow rule provided."})
			return *m, nil
		}
		m.settings.AllowedTools = dedupeStrings(append(m.settings.AllowedTools, specs...))
		rebuildSessionPermissionRules(m.session, m.settings)
		msg := "Allow rules updated"
		if forN > 0 {
			msg += fmt.Sprintf(" (this pattern auto-allowed for next %d uses)", forN)
		}
		m.messages = append(m.messages, displayMsg{role: "system", content: msg + ".\n" + permissionRulesSummary(m)})
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
	case "grants":
		if len(parts) > 2 && strings.EqualFold(strings.TrimSpace(parts[2]), "cleanup") {
			if m.session != nil && m.session.PermSvc() != nil {
				mem := m.session.PermSvc().Memory()
				if mem != nil {
					// Reset clears all learned + user rules; rebuild from settings.
					rebuildSessionPermissionRules(m.session, m.settings)
					m.messages = append(m.messages, displayMsg{role: "system", content: "Grants cleaned up. Active rules rebuilt from settings.\n" + permissionRulesSummary(m)})
					return *m, nil
				}
			}
			m.messages = append(m.messages, displayMsg{role: "error", content: "No active permission state."})
			return *m, nil
		}
		m.messages = append(m.messages, displayMsg{role: "system", content: "Usage: /autonomy grants cleanup — rebuild active rules from settings (clears learned grants)"})
	case "rules":
		if len(parts) > 2 && strings.EqualFold(strings.TrimSpace(parts[2]), "clear") {
			m.settings.AutoAllow = nil
			m.settings.AllowedTools = nil
			m.settings.DisallowedTools = nil
			m.settings.NeverAllow = nil
			rebuildSessionPermissionRules(m.session, m.settings)
			if m.session != nil && m.session.PermSvc() != nil {
				m.session.PermSvc().SetNeverAllow(nil)
			}
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
	case "bypass":
		if m.session == nil || m.session.PermSvc() == nil {
			m.messages = append(m.messages, displayMsg{role: "error", content: "No active session."})
			return *m, nil
		}
		if len(parts) < 3 {
			bypass := m.session.PermSvc().BypassKill()
			state := "off"
			if bypass != nil && bypass.IsEnabled() {
				state = "on"
				g := bypass.Grant()
				if g != nil && len(g.Scope) > 0 {
					state += " (scope: " + strings.Join(g.Scope, ",") + ")"
					if !g.ExpiresAt.IsZero() {
						state += " (expires: " + g.ExpiresAt.Format("15:04:05") + ")"
					}
				}
			}
			m.messages = append(m.messages, displayMsg{role: "system", content: "Bypass: " + state + "\nUsage: /autonomy bypass <on|off> [--scope=bash,network] [--for=5m] [--reason=\"debugging\"]"})
			return *m, nil
		}
		switch strings.ToLower(strings.TrimSpace(parts[2])) {
		case "on", "true", "1":
			scope, expires, reason := parseBypassFlags(parts[3:])
			m.session.PermSvc().BypassKill().EnableScoped(scope, expires, reason)
			scopeLabel := "all"
			if len(scope) > 0 {
				scopeLabel = strings.Join(scope, ",")
			}
			m.messages = append(m.messages, displayMsg{role: "system", content: fmt.Sprintf("Bypass → on (scope: %s, reason: %s). Use with care.", scopeLabel, reason)})
		case "off", "false", "0":
			m.session.PermSvc().BypassKill().Disable()
			m.messages = append(m.messages, displayMsg{role: "system", content: "Bypass → off. Normal permission checks resume."})
		default:
			m.messages = append(m.messages, displayMsg{role: "error", content: "Usage: /autonomy bypass <on|off> [--scope=...] [--for=...] [--reason=...]"})
		}
	case "profile":
		if m.session == nil || m.session.PermSvc() == nil {
			m.messages = append(m.messages, displayMsg{role: "error", content: "No active session."})
			return *m, nil
		}
		if len(parts) < 3 {
			// Show current profile flags.
			profile := m.session.PermSvc().AutonomyProfile()
			if profile == nil {
				m.messages = append(m.messages, displayMsg{role: "system", content: "No active profile."})
				return *m, nil
			}
			var b strings.Builder
			b.WriteString("Autonomy Profile\n")
			b.WriteString(fmt.Sprintf("  Level: %s\n", profile.Level.String()))
			b.WriteString(fmt.Sprintf("  auto_continue:    %v\n", profile.AutoContinue))
			b.WriteString(fmt.Sprintf("  auto_apply_edits:  %v\n", profile.AutoApplyEdits))
			b.WriteString(fmt.Sprintf("  auto_execute_bash: %v%s\n", profile.AutoExecuteBash, markOverridden(profile, "autoexecutebash")))
			b.WriteString(fmt.Sprintf("  auto_commit:       %v\n", profile.AutoCommit))
			b.WriteString(fmt.Sprintf("  auto_network:      %v%s\n", profile.AutoNetwork, markOverridden(profile, "autonetwork")))
			b.WriteString("\nUsage: /autonomy profile <flag>=<on|off>\n  e.g. /autonomy profile auto_execute_bash=off")
			m.messages = append(m.messages, displayMsg{role: "system", content: b.String()})
			return *m, nil
		}
		// Parse flag=value.
		flagSet := strings.Join(parts[2:], " ")
		idx := strings.Index(flagSet, "=")
		if idx < 0 {
			m.messages = append(m.messages, displayMsg{role: "error", content: "Usage: /autonomy profile <flag>=<on|off>"})
			return *m, nil
		}
		flagName := strings.TrimSpace(flagSet[:idx])
		flagVal := strings.ToLower(strings.TrimSpace(flagSet[idx+1:]))
		val := flagVal == "on" || flagVal == "true" || flagVal == "1"
		before := m.session.PermSvc().AutonomyProfile()
		if before == nil || !before.Override(flagName, val) {
			m.messages = append(m.messages, displayMsg{role: "error", content: fmt.Sprintf("Unknown flag %q. Valid: auto_continue, auto_apply_edits, auto_execute_bash, auto_commit, auto_network", flagName)})
			return *m, nil
		}
		m.session.PermSvc().ApplyAutonomyOverrides(before.Overrides())
		m.settings.AutonomyOverrides = before.Overrides()
		m.messages = append(m.messages, displayMsg{role: "system", content: fmt.Sprintf("Profile updated: %s=%v\n%s", flagName, val, func() string {
			p := m.session.PermSvc().AutonomyProfile()
			if p == nil {
				return ""
			}
			return fmt.Sprintf("  auto_execute_bash=%v auto_network=%v", p.AutoExecuteBash, p.AutoNetwork)
		}())})
	case "spec-tests":
		if len(parts) < 3 {
			state := "off"
			if m.settings.SpecAllowTests {
				state = "on"
			}
			m.messages = append(m.messages, displayMsg{role: "system", content: "Spec-stage test allowance: " + state + "\nUsage: /autonomy spec-tests <on|off>\n  When on, safe test commands (go test, npm test, pytest, etc.) are permitted during the spec workflow."})
			return *m, nil
		}
		switch strings.ToLower(strings.TrimSpace(parts[2])) {
		case "on", "true", "1":
			m.settings.SpecAllowTests = true
			m.session.PermSvc().SetSpecAllowTests(true)
			m.messages = append(m.messages, displayMsg{role: "system", content: "Spec-stage test allowance → on"})
		case "off", "false", "0":
			m.settings.SpecAllowTests = false
			m.session.PermSvc().SetSpecAllowTests(false)
			m.messages = append(m.messages, displayMsg{role: "system", content: "Spec-stage test allowance → off"})
		default:
			m.messages = append(m.messages, displayMsg{role: "error", content: "Usage: /autonomy spec-tests <on|off>"})
		}
	case "isolation":
		if len(parts) < 3 {
			cur := strings.TrimSpace(containerNetworkFlag)
			if cur == "" {
				cur = "bridge"
			}
			m.messages = append(m.messages, displayMsg{role: "system", content: "Container network isolation: " + cur + "\nUsage: /autonomy isolation <none|bridge|isolated>\n  none     — no network access\n  bridge   — shared bridge (default)\n  isolated — per-container network, concurrent containers can't probe each other"})
			return *m, nil
		}
		mode, ok := normalizeContainerNetwork(parts[2])
		if !ok {
			m.messages = append(m.messages, displayMsg{role: "error", content: "Valid modes: none, bridge, isolated"})
			return *m, nil
		}
		containerNetworkFlag = mode
		m.settings.ContainerNetwork = mode
		m.messages = append(m.messages, displayMsg{role: "system", content: "Container network isolation → " + mode + "\n(affects next container start)"})
	case "never":
		if m.session == nil || m.session.PermSvc() == nil {
			m.messages = append(m.messages, displayMsg{role: "error", content: "No active session."})
			return *m, nil
		}
		if len(parts) < 3 {
			never := m.session.PermSvc().NeverAllow()
			var b strings.Builder
			b.WriteString("Personal Hard Ceiling (never rules)\n")
			if len(never) == 0 {
				b.WriteString("  none — YOLO can do anything. Add one with /autonomy never <rule>\n")
			} else {
				for _, r := range never {
					b.WriteString("  - " + r + "\n")
				}
			}
			b.WriteString("\nUsage: /autonomy never <rule>   e.g. /autonomy never Write(*.env)\n")
			b.WriteString("       /autonomy never clear")
			m.messages = append(m.messages, displayMsg{role: "system", content: b.String()})
			return *m, nil
		}
		if strings.EqualFold(strings.TrimSpace(parts[2]), "clear") {
			m.settings.NeverAllow = nil
			m.session.PermSvc().SetNeverAllow(nil)
			m.messages = append(m.messages, displayMsg{role: "system", content: "Never rules cleared."})
			return *m, nil
		}
		specs := parseToolListFromCLI([]string{strings.Join(parts[2:], " ")})
		if len(specs) == 0 {
			m.messages = append(m.messages, displayMsg{role: "error", content: "No valid never rule provided."})
			return *m, nil
		}
		m.settings.NeverAllow = append(m.settings.NeverAllow, specs...)
		m.session.PermSvc().SetNeverAllow(m.settings.NeverAllow)
		var nb strings.Builder
		nb.WriteString("Never rule added. Even YOLO will be blocked.\n")
		for _, r := range m.settings.NeverAllow {
			nb.WriteString("  - " + r + "\n")
		}
		m.messages = append(m.messages, displayMsg{role: "system", content: nb.String()})
	case "audit":
		if m.session == nil || m.session.PermSvc() == nil {
			m.messages = append(m.messages, displayMsg{role: "error", content: "No active session."})
			return *m, nil
		}
		m.messages = append(m.messages, displayMsg{role: "system", content: m.session.PermSvc().AuditLog()})
	case "metrics":
		if m.session == nil || m.session.PermSvc() == nil {
			m.messages = append(m.messages, displayMsg{role: "error", content: "No active session."})
			return *m, nil
		}
		m.messages = append(m.messages, displayMsg{role: "system", content: m.session.PermSvc().PermissionMetrics()})
	case "reset":
		resetPermissionCenter(m)
		m.messages = append(m.messages, displayMsg{role: "system", content: "Autonomy Center reset to defaults.\n" + autonomyCenterSummary(m)})
	default:
		m.messages = append(m.messages, displayMsg{role: "system", content: autonomyCommandHelp()})
	}
	return *m, nil
}
