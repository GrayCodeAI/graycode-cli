package cmd

import (
	"fmt"
	"strings"

	"github.com/GrayCodeAI/hawk/internal/engine"
)

// controlPlaneOnboardingHint is a short first-session tip (not a wall of text).
func controlPlaneOnboardingHint(sess *engine.Session) string {
	var lines []string
	lines = append(lines, "Quick path:  /start   ·   /mode plan|act   ·   /isolation workspace")

	tr := engine.ProjectTrust("")
	if tr.Blocked {
		lines = append(lines, "⚠ Folder not trusted — project hooks/MCP blocked.  /trust add")
	}
	if gi := engine.InspectGitBranch(""); gi.OnDefault {
		lines = append(lines, fmt.Sprintf("⚠ On %s — /branch-agent before large edits", gi.Branch))
	}
	if sess != nil {
		lines = append(lines, fmt.Sprintf("Now: work=%s · iso=%s · auto-commit=%v",
			sess.WorkMode(), sess.Isolation().String(), sess.AutoCommit()))
	}
	lines = append(lines, "Tip: edits show a unified diff; permissions show risk + why.")
	return strings.Join(lines, "\n")
}

// workModeSwitchSummary is the polished confirmation after /mode plan|act|review.
func workModeSwitchSummary(sess *engine.Session, wm engine.WorkMode) string {
	hint := ""
	switch wm {
	case engine.WorkModePlan:
		hint = "Research only — no file writes; bash is read-only."
	case engine.WorkModeReview:
		hint = "Inspect with evidence — no writes; bash is read-only."
	default:
		hint = "Build mode — essential tools visible; use ToolSearch select:Name for more."
	}
	visible := 0
	if sess != nil && sess.Tools() != nil && sess.Tools().Registry() != nil {
		visible = len(sess.Tools().Registry().EyrieTools())
	}
	return fmt.Sprintf("Work mode → %s\n%s\nModel-visible tools: %d  ·  /status for full control plane", wm, hint, visible)
}
