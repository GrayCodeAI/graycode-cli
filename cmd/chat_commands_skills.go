package cmd

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/GrayCodeAI/hawk/internal/plugin"
	"github.com/GrayCodeAI/hawk/internal/tool"
	"github.com/GrayCodeAI/hawk/internal/ui/icons"
)

// handleSkillsCommand handles the /skills command and all its subcommands.
func (m *chatModel) handleSkillsCommand(parts []string, text string) (tea.Model, tea.Cmd) {
	if len(parts) >= 2 {
		switch parts[1] {
		case "install":
			if len(parts) < 3 {
				m.messages = append(m.messages, displayMsg{role: "error", content: "Usage: /skills install <owner/repo> [skill-name]"})
				return m, nil
			}
			repo := parts[2]
			if !strings.Contains(repo, "/") {
				m.messages = append(m.messages, displayMsg{role: "error", content: "Usage: /skills install <owner/repo> [skill-name]"})
				return m, nil
			}
			skillName := ""
			if len(parts) >= 4 {
				skillName = parts[3]
			}
			rc := plugin.NewRegistryClient()
			msg, err := rc.Install(repo, skillName, "user")
			if err != nil {
				m.messages = append(m.messages, displayMsg{role: "error", content: err.Error()})
			} else {
				m.messages = append(m.messages, displayMsg{role: "system", content: msg})
			}
			return m, nil

		case "search":
			query := ""
			category := ""
			for i := 2; i < len(parts); i++ {
				if parts[i] == "--category" && i+1 < len(parts) {
					category = parts[i+1]
					i++
				} else {
					if query != "" {
						query += " "
					}
					query += parts[i]
				}
			}
			rc := plugin.NewRegistryClient()
			results, err := rc.Search(query, category)
			if err != nil {
				m.messages = append(m.messages, displayMsg{role: "error", content: err.Error()})
				return m, nil
			}
			if len(results) == 0 {
				m.messages = append(m.messages, displayMsg{role: "system", content: "No skills found."})
				return m, nil
			}
			var b strings.Builder
			b.WriteString(fmt.Sprintf("Found %d skill(s):\n\n", len(results)))
			limit := 20
			if len(results) < limit {
				limit = len(results)
			}
			for _, e := range results[:limit] {
				b.WriteString(plugin.FormatSkillEntry(e))
			}
			if len(results) > 20 {
				_, _ = fmt.Fprintf(&b, "\n  ... and %d more. Refine your search.\n", len(results)-20)
			}
			m.messages = append(m.messages, displayMsg{role: "system", content: b.String()})
			return m, nil

		case "trending":
			limit := 10
			if len(parts) >= 3 {
				if n, err := strconv.Atoi(parts[2]); err == nil && n > 0 {
					limit = n
				}
			}
			rc := plugin.NewRegistryClient()
			results, err := rc.Trending(limit)
			if err != nil {
				m.messages = append(m.messages, displayMsg{role: "error", content: err.Error()})
				return m, nil
			}
			if len(results) == 0 {
				m.messages = append(m.messages, displayMsg{role: "system", content: "No trending skills found."})
				return m, nil
			}
			var b strings.Builder
			b.WriteString("Trending skills:\n\n")
			for i, e := range results {
				_, _ = fmt.Fprintf(&b, "  %d. ", i+1)
				b.WriteString(strings.TrimLeft(plugin.FormatSkillEntry(e), " "))
			}
			m.messages = append(m.messages, displayMsg{role: "system", content: b.String()})
			return m, nil

		case "info":
			if len(parts) < 3 {
				m.messages = append(m.messages, displayMsg{role: "error", content: "Usage: /skills info <name>"})
				return m, nil
			}
			name := parts[2]
			// Check local first.
			if skill, path, ok := plugin.InstalledSkillInfo(name); ok {
				m.messages = append(m.messages, displayMsg{role: "system", content: plugin.FormatSkillInfo(skill, path)})
				return m, nil
			}
			// Fall back to registry.
			rc := plugin.NewRegistryClient()
			entry, err := rc.Info(name)
			if err != nil {
				m.messages = append(m.messages, displayMsg{role: "error", content: err.Error()})
				return m, nil
			}
			var b strings.Builder
			_, _ = fmt.Fprintf(&b, "Skill: %s (not installed)\n", entry.Name)
			if entry.Version != "" {
				_, _ = fmt.Fprintf(&b, "Version: %s\n", entry.Version)
			}
			if entry.Author != "" {
				_, _ = fmt.Fprintf(&b, "Author: %s\n", entry.Author)
			}
			if entry.Description != "" {
				_, _ = fmt.Fprintf(&b, "Description: %s\n", entry.Description)
			}
			if entry.Repo != "" {
				_, _ = fmt.Fprintf(&b, "Repo: %s\n", entry.Repo)
			}
			_, _ = fmt.Fprintf(&b, "Installs: %d\n", entry.Installs)
			_, _ = fmt.Fprintf(&b, "\nInstall with: /skills install %s %s\n", entry.Repo, entry.Name)
			m.messages = append(m.messages, displayMsg{role: "system", content: b.String()})
			return m, nil

		case "remove":
			if len(parts) < 3 {
				m.messages = append(m.messages, displayMsg{role: "error", content: "Usage: /skills remove <name>"})
				return m, nil
			}
			if err := plugin.Remove(parts[2]); err != nil {
				m.messages = append(m.messages, displayMsg{role: "error", content: err.Error()})
			} else {
				m.messages = append(m.messages, displayMsg{role: "system", content: fmt.Sprintf("Removed skill %q.", parts[2])})
			}
			return m, nil

		case "update":
			name := ""
			if len(parts) >= 3 {
				name = parts[2]
			}
			// Find installed skills with source metadata and re-install.
			updated := 0
			skills := plugin.LoadSmartSkills(plugin.DefaultSkillDirs())
			for _, s := range skills {
				if s.Source.Repo == "" {
					continue
				}
				if name != "" && !strings.EqualFold(s.Name, name) {
					continue
				}
				rc := plugin.NewRegistryClient()
				if _, err := rc.Install(s.Source.Repo, s.Name, "user"); err == nil {
					updated++
				}
			}
			if updated == 0 {
				m.messages = append(m.messages, displayMsg{role: "system", content: "No skills to update (only skills with source tracking can be updated)."})
			} else {
				m.messages = append(m.messages, displayMsg{role: "system", content: fmt.Sprintf("Updated %d skill(s).", updated)})
			}
			return m, nil

		case "publish":
			if len(parts) < 3 {
				m.messages = append(m.messages, displayMsg{role: "error", content: "Usage: /skills publish <skill-dir>\nValidates the skill and shows the command to submit it."})
				return m, nil
			}
			skillDir := parts[2]
			skillFile := filepath.Join(skillDir, "SKILL.md")
			if _, err := os.Stat(skillFile); err != nil {
				m.messages = append(m.messages, displayMsg{role: "error", content: fmt.Sprintf("No SKILL.md found in %s", skillDir)})
				return m, nil
			}
			findings, _ := plugin.AuditSkillFile(skillFile)
			for _, f := range findings {
				if f.Severity == plugin.SeverityCritical {
					m.messages = append(m.messages, displayMsg{role: "error", content: fmt.Sprintf("Publish blocked: %s has CRITICAL security findings. Run /skills audit first.", skillFile)})
					return m, nil
				}
			}
			data, _ := os.ReadFile(skillFile)
			skill := plugin.ParseSmartSkillPublic(string(data))
			var issues []string
			if skill.Name == "" {
				issues = append(issues, "missing 'name' in frontmatter")
			}
			if skill.Description == "" {
				issues = append(issues, "missing 'description' in frontmatter")
			}
			if len(issues) > 0 {
				m.messages = append(m.messages, displayMsg{role: "error", content: "Validation failed:\n  - " + strings.Join(issues, "\n  - ")})
				return m, nil
			}
			var b strings.Builder
			b.WriteString(icons.CheckBold() + " Skill validated successfully.\n\n")
			_, _ = fmt.Fprintf(&b, "  Name: %s\n", skill.Name)
			_, _ = fmt.Fprintf(&b, "  Description: %s\n", skill.Description)
			if skill.Version != "" {
				_, _ = fmt.Fprintf(&b, "  Version: %s\n", skill.Version)
			}
			b.WriteString("\nTo publish:\n")
			b.WriteString("  1. Push your skill to a GitHub repo with skills/<name>/SKILL.md\n")
			b.WriteString("  2. Submit a PR to github.com/GrayCodeAI/hawk-skills to add your repo\n")
			b.WriteString("  3. Or install directly: /skills install <your-org>/<your-repo>\n")
			m.messages = append(m.messages, displayMsg{role: "system", content: b.String()})
			return m, nil

		case "audit":
			if len(parts) >= 3 {
				target := parts[2]
				if info, err := os.Stat(target); err == nil && !info.IsDir() {
					findings, err := plugin.AuditSkillFile(target)
					if err != nil {
						m.messages = append(m.messages, displayMsg{role: "error", content: err.Error()})
						return m, nil
					}
					r := plugin.AuditResult{Findings: findings, Files: 1}
					m.messages = append(m.messages, displayMsg{role: "system", content: plugin.FormatAuditResult(r)})
					return m, nil
				}
				if _, path, ok := plugin.InstalledSkillInfo(target); ok {
					findings, _ := plugin.AuditSkillFile(path)
					r := plugin.AuditResult{Findings: findings, Files: 1}
					m.messages = append(m.messages, displayMsg{role: "system", content: plugin.FormatAuditResult(r)})
					return m, nil
				}
				m.messages = append(m.messages, displayMsg{role: "error", content: fmt.Sprintf("Skill or file %q not found.", target)})
				return m, nil
			}
			result := plugin.AuditAllSkills()
			m.messages = append(m.messages, displayMsg{role: "system", content: plugin.FormatAuditResult(result)})
			return m, nil

		case "feedback":
			if len(parts) < 4 {
				m.messages = append(m.messages, displayMsg{role: "error", content: "Usage: /skills feedback <name> <1-5> [comment]"})
				return m, nil
			}
			name := parts[2]
			rating, err := strconv.Atoi(parts[3])
			if err != nil || rating < 1 || rating > 5 {
				m.messages = append(m.messages, displayMsg{role: "error", content: "Rating must be 1-5."})
				return m, nil
			}
			comment := ""
			if len(parts) > 4 {
				comment = strings.Join(parts[4:], " ")
			}
			fs := plugin.NewFeedbackStore()
			if err := fs.Rate(name, rating, comment); err != nil {
				m.messages = append(m.messages, displayMsg{role: "error", content: err.Error()})
			} else {
				m.messages = append(m.messages, displayMsg{role: "system", content: fmt.Sprintf("Rated %s %s", name, plugin.FormatRating(rating))})
			}
			return m, nil

		case "use":
			if len(parts) < 3 {
				m.messages = append(m.messages, displayMsg{role: "error", content: "Usage: /skills use <name>"})
				return m, nil
			}
			name := parts[2]
			skills := plugin.LoadSmartSkills(plugin.DefaultSkillDirs())
			for _, s := range skills {
				if strings.EqualFold(s.Name, name) {
					if m.activeSkills == nil {
						m.activeSkills = make(map[string]plugin.SmartSkill)
					}
					m.activeSkills[s.Name] = s
					m.session.AddUser(fmt.Sprintf("[Skill activated: %s]\n\n%s", s.Name, s.Content))
					m.messages = append(m.messages, displayMsg{role: "system", content: fmt.Sprintf("Activated skill: %s", s.Name)})
					return m, nil
				}
			}
			m.messages = append(m.messages, displayMsg{role: "error", content: fmt.Sprintf("Skill %q not found. Run /skills to see available skills.", name)})
			return m, nil

		case "deactivate":
			if len(parts) < 3 {
				m.messages = append(m.messages, displayMsg{role: "error", content: "Usage: /skills deactivate <name>"})
				return m, nil
			}
			name := parts[2]
			if m.activeSkills != nil {
				if _, ok := m.activeSkills[name]; ok {
					delete(m.activeSkills, name)
					m.messages = append(m.messages, displayMsg{role: "system", content: fmt.Sprintf("Deactivated skill: %s", name)})
					return m, nil
				}
			}
			m.messages = append(m.messages, displayMsg{role: "error", content: fmt.Sprintf("Skill %q is not active.", name)})
			return m, nil

		case "new":
			desc := "a useful coding skill for this project"
			if len(parts) >= 3 {
				desc = strings.Join(parts[2:], " ")
			}
			prompt := plugin.BuildNewSkillPrompt(desc)
			return m.startPromptCommand("/skills new "+desc, prompt)
		}
	}
	// Default: list local skills.
	out, err := (tool.SkillTool{}).Execute(context.Background(), nil)
	if err != nil {
		m.messages = append(m.messages, displayMsg{role: "error", content: err.Error()})
	} else {
		m.messages = append(m.messages, displayMsg{role: "system", content: out})
	}
	return m, nil
}
