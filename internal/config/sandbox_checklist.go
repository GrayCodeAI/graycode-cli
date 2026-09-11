package config

import (
	"context"
	"fmt"
	"strings"

	"github.com/GrayCodeAI/hawk/internal/sandbox"
)

// SandboxChecklistItem is one ordered step in the Docker onboarding checklist.
type SandboxChecklistItem struct {
	Step   string          `json:"step"`
	Label  string          `json:"label"`
	Status PathCheckStatus `json:"status"`
	Detail string          `json:"detail,omitempty"`
	FixCmd string          `json:"fix_cmd,omitempty"`
}

// EvaluateSandboxChecklist returns the ordered Docker onboarding checklist
// (daemon -> image cached -> registry reachable -> local build available),
// emitted identically by path, preflight, and doctor. It is diagnostic only:
// it never mutates Docker state. hawk is fail-closed — there is no
// host-execution fallback (see docs/SECURITY-DEVELOPER.md).
func EvaluateSandboxChecklist(ctx context.Context) []SandboxChecklistItem {
	if ctx == nil {
		ctx = context.Background()
	}

	daemon := sandbox.DockerAvailable()
	items := make([]SandboxChecklistItem, 0, 4)

	items = append(items, SandboxChecklistItem{
		Step:   "daemon",
		Label:  "Start the Docker daemon",
		Status: statusFor(daemon),
		Detail: boolDetail(daemon,
			"Docker daemon running — Bash runs in container by default",
			"Docker not available — agent tools are locked"),
		FixCmd: "Start Docker Desktop, or: systemctl start docker (Linux) / open -a Docker (macOS)",
	})

	img, present := "", false
	if daemon {
		img, present = sandbox.ImagePresent(ctx)
	}
	items = append(items, SandboxChecklistItem{
		Step:   "image",
		Label:  "Sandbox image cached locally",
		Status: statusFor(daemon && present),
		Detail: boolDetail(present,
			"Image present: "+img,
			"Image not cached — Hawk pulls or builds it on first run"),
		FixCmd: "docker pull " + sandbox.DefaultSandboxImage(),
	})

	reachable := false
	if daemon && !present {
		reachable = sandbox.RegistryReachable(ctx)
	}
	items = append(items, SandboxChecklistItem{
		Step:   "registry",
		Label:  "Reach the image registry",
		Status: statusFor(daemon && (present || reachable)),
		Detail: boolDetail(present || reachable,
			"Registry reachable — image can be pulled",
			"Registry unreachable — falling back to a local Docker build"),
		FixCmd: "Check network/proxy, or rely on the local build fallback below",
	})

	items = append(items, SandboxChecklistItem{
		Step:   "build",
		Label:  "Local sandbox build available",
		Status: statusFor(daemon),
		Detail: "Bundled sandbox Dockerfile can build the image locally through Docker",
		FixCmd: "Requires only a running Docker daemon",
	})

	return items
}

func statusFor(ok bool) PathCheckStatus {
	if ok {
		return PathPass
	}
	return PathFail
}

func boolDetail(ok bool, yes, no string) string {
	if ok {
		return yes
	}
	return no
}

// FormatSandboxChecklist renders the ordered checklist for human output.
func FormatSandboxChecklist(items []SandboxChecklistItem) string {
	var b strings.Builder
	b.WriteString("Docker sandbox checklist (fail-closed; no host-execution fallback):\n")
	for _, it := range items {
		b.WriteString(fmt.Sprintf("  [%s] %s — %s\n", it.Status, it.Label, it.Detail))
		if it.FixCmd != "" {
			b.WriteString(fmt.Sprintf("      fix: %s\n", it.FixCmd))
		}
	}
	return strings.TrimRight(b.String(), "\n")
}
