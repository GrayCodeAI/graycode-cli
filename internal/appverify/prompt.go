package appverify

import (
	"fmt"
	"strings"
)

// EvidenceDir is the stable, workspace-relative directory for verification
// artifacts. Stable paths let reports and downstream tooling rely on them.
const EvidenceDir = ".graycode/verify/artifacts"

// BuildVerifyPrompt renders the phased QA-engineer prompt for the recipe. The
// discipline it encodes (adopted from grok-cli) is that build/test passing
// means nothing unless the app actually boots and serves: the workflow is
// mandatory and evidence is mandatory even on failure.
func BuildVerifyPrompt(r Recipe) string {
	var b strings.Builder
	b.WriteString("You are verifying this project end-to-end as a QA engineer. ")
	b.WriteString("A green build is NOT success — the app must actually run. Follow every phase in order.\n\n")

	fmt.Fprintf(&b, "Project: %s (%s/%s)\n", r.AppLabel, r.Ecosystem, r.AppKind)
	if len(r.Notes) > 0 {
		b.WriteString("\nRecipe notes:\n")
		for _, n := range r.Notes {
			fmt.Fprintf(&b, "- %s\n", n)
		}
	}

	b.WriteString(`
## Phase 1 — Setup
- Probe for required runtimes before installing anything; only install what is missing.
`)

	if len(r.Install) > 0 {
		fmt.Fprintf(&b, "- Install dependencies with exactly: %s\n", argv(r.Install))
	}

	b.WriteString(`
## Phase 2 — Build and test
`)
	if len(r.Build) > 0 {
		fmt.Fprintf(&b, "- Build: %s\n", argv(r.Build))
	}
	if len(r.Test) > 0 {
		fmt.Fprintf(&b, "- Test: %s\n", argv(r.Test))
	}
	if len(r.Build) == 0 && len(r.Test) == 0 {
		b.WriteString("- No build/test commands in the recipe; record that plainly.\n")
	}

	b.WriteString(`
## Phase 3 — Boot the app (REQUIRED)
`)
	switch {
	case len(r.Start) > 0 && r.SmokeKind == SmokeHTTP:
		target := r.SmokeTarget()
		fmt.Fprintf(&b, "- Start the app in the background: %s\n", argv(r.Start))
		fmt.Fprintf(&b, "- Wait for readiness by polling %s until HTTP 200 (bounded loop, ~60s max).\n", target)
		b.WriteString("- If readiness never succeeds, capture the app log tail and report the exact command used.\n")
	case len(r.Start) > 0 && r.SmokeKind == SmokeCLI:
		fmt.Fprintf(&b, "- Run the entrypoint once: %s\n", argv(r.Start))
		b.WriteString("- Treat a zero exit code plus sane stdout as the boot signal.\n")
	default:
		b.WriteString("- No start command is known. Inspect the project to determine how it runs; if it cannot be booted, say so explicitly instead of guessing.\n")
	}

	b.WriteString(`
## Phase 4 — Evidence (REQUIRED even on failure)
`)
	fmt.Fprintf(&b, "- Save all artifacts under %s:\n", EvidenceDir)
	b.WriteString("  - app log tail -> artifacts/app.log\n")
	if r.SmokeKind == SmokeHTTP {
		b.WriteString("  - screenshot of the served page when a browser/screenshot tool is available\n")
	}
	b.WriteString(`- Report in this exact structure:
  Summary / Results / Evidence (exact artifact paths) / Blockers / Residual Risk.

## Phase 5 — Teardown
- Stop any background process you started BEFORE finishing, then verify no orphan listeners remain on the port.
`)

	if r.Ecosystem == "node" {
		b.WriteString(`
Bounded retry guidance:
- Native module build failures (lightningcss, sharp, @next/swc, esbuild): remove node_modules and reinstall once with optional deps enabled, then rebuild. At most one retry.
- Startup readiness failures: retry at most once binding HOST=0.0.0.0 and the explicit PORT.
- Anything else: do not thrash — report the blocker directly.
`)
	}
	return b.String()
}

func argv(args []string) string { return "`" + strings.Join(args, " ") + "`" }
