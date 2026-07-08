package cmd

import (
	"context"
	"fmt"
	"os/exec"
	"strings"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/GrayCodeAI/hawk/internal/feature/shellmode"
	"github.com/GrayCodeAI/hawk/internal/plugin"
	"github.com/GrayCodeAI/hawk/internal/ui/icons"
)

// explainCode traces a file/line back to the git commit and session that created it.
func explainCode(path string, line int) (string, error) {
	// Step 1: git blame to find the commit
	args := []string{"blame", "-L", fmt.Sprintf("%d,%d", line, line), "--porcelain", path}
	out, err := exec.CommandContext(context.Background(), "git", args...).Output() // #nosec G204 -- fixed command 'git' with args, not user-controlled binary
	if err != nil {
		return "", fmt.Errorf("git blame failed: %w", err)
	}
	lines := strings.Split(string(out), "\n")
	if len(lines) == 0 {
		return "", fmt.Errorf("no blame output")
	}
	commitHash := strings.Fields(lines[0])[0]
	if commitHash == "0000000000000000000000000000000000000000" {
		return "This line is uncommitted (not yet in git history).", nil
	}

	// Step 2: get commit info
	info, err := exec.CommandContext(context.Background(), "git", "log", "-1", "--format=%h %s (%an, %ar)", commitHash).Output() // #nosec G204 -- fixed command 'git' with args, not user-controlled binary
	if err != nil {
		return fmt.Sprintf("Commit: %s (details unavailable)", commitHash[:7]), nil
	}

	// Step 3: get the diff for context
	diff, _ := exec.CommandContext(context.Background(), "git", "log", "-1", "--format=", "-p", "--", path, commitHash).Output() // #nosec G204 -- fixed command 'git' with args, not user-controlled binary
	diffStr := string(diff)
	if len(diffStr) > 2000 {
		diffStr = diffStr[:2000] + "\n... (truncated)"
	}

	result := fmt.Sprintf("**Origin:** %s\n", strings.TrimSpace(string(info)))
	if diffStr != "" {
		result += fmt.Sprintf("\n**Changes in that commit:**\n```diff\n%s\n```", diffStr)
	}
	return result, nil
}

// handleShellEscape runs a shell command directly (triggered by ! prefix).
func (m *chatModel) handleShellEscape(command string) (tea.Model, tea.Cmd) {
	command = strings.TrimSpace(command)
	if command == "" {
		return m, nil
	}

	// Warn about destructive commands.
	if shellmode.IsDestructive(command) {
		m.messages = append(m.messages, displayMsg{role: "error", content: "Warning: potentially destructive command detected. Use with caution."})
	}

	m.messages = append(m.messages, displayMsg{role: "system", content: "$ " + command})
	m.viewDirty = true

	result := shellmode.ExecuteShell(context.Background(), command)
	output := result.Stdout + result.Stderr
	output = strings.TrimRight(output, "\n")

	if output != "" {
		// Truncate very long output.
		if len(output) > 4000 {
			lines := strings.Split(output, "\n")
			if len(lines) > 40 {
				head := strings.Join(lines[:20], "\n")
				tail := strings.Join(lines[len(lines)-20:], "\n")
				output = head + fmt.Sprintf("\n\n... (%d lines omitted) ...\n\n", len(lines)-40) + tail
			}
		}
		m.messages = append(m.messages, displayMsg{role: "tool_result", content: output})
	}
	if result.ExitCode != 0 && output == "" {
		m.messages = append(m.messages, displayMsg{role: "error", content: fmt.Sprintf("exit code: %d", result.ExitCode)})
	}

	// Smart reroute: if command failed with NL markers, offer to send to AI
	if result.ExitCode != 0 && shellmode.RerouteCandidate(command, result.Stderr, result.ExitCode) {
		m.messages = append(m.messages, displayMsg{role: "system", content: icons.Refresh() + " Natural language detected in failed command — rerouting to AI..."})
		m.termCtx.MarkExitCode(result.ExitCode)
		query := m.termCtx.BuildContext(command)
		m.messages = append(m.messages, displayMsg{role: "user", content: command})
		m.session.AddUser(query)
		m.ghostText.SuggestExplicit(command) // suggest the original command for retry
		m.waiting = true
		m.autoScroll = true
		m.viewDirty = true
		m.partial.Reset()
		m.startStream()
		return m, nil
	}

	m.termCtx.MarkExitCode(result.ExitCode)
	m.viewDirty = true
	return m, nil
}

// handleNamespacedSkill handles /vendor:skill-name invocations.
func (m *chatModel) handleNamespacedSkill(cmd, fullText string) (tea.Model, tea.Cmd) {
	// Parse /vendor:skill-name
	invoke := cmd // e.g. "/hawk:go-review"

	// Search active and installed skills for matching invoke pattern
	var matched *plugin.SmartSkill
	for name, skill := range m.activeSkills {
		if skill.Invoke == invoke || "/hawk:"+name == invoke {
			matched = &skill
			break
		}
	}

	if matched == nil {
		// Try loading from installed skills
		skills := plugin.LoadSmartSkills(plugin.DefaultSkillDirs())
		for i := range skills {
			if skills[i].Invoke == invoke || "/hawk:"+skills[i].Name == invoke {
				matched = &skills[i]
				break
			}
		}
	}

	if matched == nil {
		m.messages = append(m.messages, displayMsg{role: "error", content: fmt.Sprintf("Skill not found: %s", invoke)})
		return m, nil
	}

	// Activate the skill and send as context
	// Check for chain conflicts
	conflicts := plugin.ResolveChainConflicts(*matched, m.activeSkills)
	if len(conflicts) > 0 {
		m.messages = append(m.messages, displayMsg{role: "system", content: fmt.Sprintf("%s Conflicts with active skill(s): %s", icons.Alert(), strings.Join(conflicts, ", "))})
	}

	m.activeSkills[matched.Name] = *matched
	args := strings.TrimSpace(strings.TrimPrefix(fullText, cmd))
	prompt := matched.Content
	if args != "" {
		prompt = fmt.Sprintf("[Skill: %s]\n%s\n\n[User request]: %s", matched.Name, prompt, args)
	} else {
		prompt = fmt.Sprintf("[Skill: %s activated]\n%s", matched.Name, prompt)
	}

	m.messages = append(m.messages, displayMsg{role: "system", content: fmt.Sprintf("%s Skill activated: %s", icons.Bolt(), matched.Name)})
	m.messages = append(m.messages, displayMsg{role: "user", content: args})
	m.session.AddUser(prompt)
	m.waiting = true
	m.autoScroll = true
	m.viewDirty = true
	m.partial.Reset()
	m.startStream()
	return m, nil
}
