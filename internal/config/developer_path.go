package config

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	eyriecfg "github.com/GrayCodeAI/eyrie/config"
	"github.com/GrayCodeAI/eyrie/credentials"
	"github.com/GrayCodeAI/eyrie/runtime"
	"github.com/GrayCodeAI/hawk/internal/home"
	"github.com/GrayCodeAI/hawk/internal/intelligence/memory"
	"github.com/GrayCodeAI/hawk/internal/sandbox"
	"github.com/GrayCodeAI/hawk/internal/tool"
	"github.com/GrayCodeAI/tok"
)

// PathCheckStatus is pass, warn, or fail for one readiness row.
type PathCheckStatus string

const (
	PathPass PathCheckStatus = "pass"
	PathWarn PathCheckStatus = "warn"
	PathFail PathCheckStatus = "fail"
)

// PathCheck is one row in the developer path readiness report.
type PathCheck struct {
	Section  string
	Name     string
	Status   PathCheckStatus
	Detail   string
	FixHint  string
	Blocking bool
}

// DeveloperPathReport summarizes developer readiness (setup, security, sandbox, ecosystem).
type DeveloperPathReport struct {
	Checks      []PathCheck
	ChatReady   bool
	SecureReady bool
	Ready       bool
	NextStep    string
}

// EvaluateDeveloperPath builds the developer path readiness report.
func EvaluateDeveloperPath(ctx context.Context) DeveloperPathReport {
	if ctx == nil {
		ctx = context.Background()
	}
	PrepareCredentialDiscovery(ctx)

	var checks []PathCheck

	setup := EvaluateSetup(ctx)
	if setup.HasCredentials {
		checks = append(checks, PathCheck{
			Section: "Setup", Name: "credentials", Status: PathPass,
			Detail: "API key in OS secret store", Blocking: true,
		})
	} else {
		checks = append(checks, PathCheck{
			Section: "Setup", Name: "credentials", Status: PathFail,
			Detail:   "No provider credentials configured",
			FixHint:  "Run hawk and /config to paste an API key (or configure Ollama)",
			Blocking: true,
		})
	}

	if setup.HasModel {
		model := strings.TrimSpace(ActiveModel(ctx))
		checks = append(checks, PathCheck{
			Section: "Setup", Name: "model", Status: PathPass,
			Detail: model, Blocking: true,
		})
	} else {
		checks = append(checks, PathCheck{
			Section: "Setup", Name: "model", Status: PathFail,
			Detail:   "No model selected",
			FixHint:  "Run /config and pick a model from the catalog",
			Blocking: true,
		})
	}

	cat := CatalogHealthReport(ctx)
	switch {
	case cat.Exists && cat.Models > 0:
		checks = append(checks, PathCheck{
			Section: "Setup", Name: "catalog", Status: PathPass,
			Detail: fmt.Sprintf("%d models cached", cat.Models),
		})
	case cat.Exists:
		checks = append(checks, PathCheck{
			Section: "Setup", Name: "catalog", Status: PathWarn,
			Detail:  "Catalog file present but empty",
			FixHint: "Run hawk models refresh after adding credentials",
		})
	default:
		checks = append(checks, PathCheck{
			Section: "Setup", Name: "catalog", Status: PathWarn,
			Detail:  CatalogEmptyHint(ctx),
			FixHint: "Add credentials then run hawk models refresh",
		})
	}

	if ok, detail := credentials.KeychainWriteAvailable(ctx); ok {
		checks = append(checks, PathCheck{
			Section: "Security", Name: "keychain", Status: PathPass,
			Detail:   credentials.PlatformSecretStoreName() + " writable",
			Blocking: true,
		})
	} else {
		checks = append(checks, PathCheck{
			Section: "Security", Name: "keychain", Status: PathWarn,
			Detail:   detail,
			FixHint:  "Unlock keychain or enable secret service (Linux)",
			Blocking: true,
		})
	}

	if hasSecrets, detail := providerJSONHasSecretsOnDisk(); hasSecrets {
		checks = append(checks, PathCheck{
			Section: "Security", Name: "provider.json", Status: PathFail,
			Detail:   detail,
			FixHint:  "Run hawk start once (MigrateProviderSecrets) or remove secret fields manually",
			Blocking: true,
		})
	} else {
		checks = append(checks, PathCheck{
			Section: "Security", Name: "provider.json", Status: PathPass,
			Detail:   "No API secrets on disk (routing only)",
			Blocking: true,
		})
	}

	if legacy, paths := legacyCredentialFilesPresent(); legacy {
		checks = append(checks, PathCheck{
			Section: "Security", Name: "legacy env", Status: PathWarn,
			Detail:  "Plaintext credential files: " + strings.Join(paths, ", "),
			FixHint: "Run hawk credentials migrate",
		})
	} else {
		checks = append(checks, PathCheck{
			Section: "Security", Name: "legacy env", Status: PathPass,
			Detail:   "No ~/.hawk/env or ~/.hawk/.env files",
			Blocking: true,
		})
	}

	hawkDir := home.Dir()
	provPath := eyriecfg.GetProviderConfigPath()
	if provPath == "" {
		provPath = filepath.Join(hawkDir, ".hawk", "provider.json")
	}
	if reason := tool.IsSensitivePath(provPath); reason != "" {
		checks = append(checks, PathCheck{
			Section: "Security", Name: "read guard", Status: PathPass,
			Detail:   "Sensitive paths blocked for Read tool",
			Blocking: true,
		})
	} else {
		checks = append(checks, PathCheck{
			Section: "Security", Name: "read guard", Status: PathFail,
			Detail:   "provider.json not blocked by Read tool",
			Blocking: true,
		})
	}

	if sandbox.DockerAvailable() {
		checks = append(checks, PathCheck{
			Section: "Sandbox", Name: "docker", Status: PathPass,
			Detail: "Docker daemon running — Bash runs in container by default",
		})
	} else {
		checks = append(checks, PathCheck{
			Section: "Sandbox", Name: "docker", Status: PathWarn,
			Detail:  "Docker not available — Bash runs on host",
			FixHint: "Start Docker for isolated Bash, or use --no-container knowingly",
		})
	}

	pre := runtime.Preflight(ctx)
	if pre.Ready {
		checks = append(checks, PathCheck{
			Section: "Ecosystem", Name: "eyrie", Status: PathPass,
			Detail:   "Preflight ready to chat",
			Blocking: true,
		})
	} else {
		status := PathWarn
		for _, c := range pre.Checks {
			if c.Status == runtime.PreflightFail {
				status = PathFail
				break
			}
		}
		checks = append(checks, PathCheck{
			Section: "Ecosystem", Name: "eyrie", Status: status,
			Detail:   "Preflight not ready — see hawk preflight",
			FixHint:  "Complete /config (credentials + model)",
			Blocking: true,
		})
	}

	bridge := memory.NewYaadBridge()
	if bridge.Ready() {
		first := strings.Split(memory.YaadStatus(), "\n")[0]
		checks = append(checks, PathCheck{
			Section: "Ecosystem", Name: "yaad", Status: PathPass,
			Detail: first + " (optional persistent memory)",
		})
	} else {
		checks = append(checks, PathCheck{
			Section: "Ecosystem", Name: "yaad", Status: PathWarn,
			Detail:  "Not initialized — memory ops skipped",
			FixHint: "Ensure ~/.yaad/data/ is writable for cross-session memory",
		})
	}

	sample := tok.EstimateTokens("hawk developer path readiness")
	checks = append(checks, PathCheck{
		Section: "Ecosystem", Name: "tok", Status: PathPass,
		Detail: fmt.Sprintf("Embedded token/compress pipeline OK (sample=%d tokens)", sample),
	})

	report := DeveloperPathReport{Checks: checks}
	report.SecureReady = !anyBlockingFail(checks, "Security")
	report.ChatReady = setup.HasCredentials && setup.HasModel && pre.Ready
	report.Ready = report.ChatReady && report.SecureReady
	report.NextStep = developerPathNextStep(report, setup)
	return report
}

func anyBlockingFail(checks []PathCheck, section string) bool {
	for _, c := range checks {
		if c.Section == section && c.Blocking && c.Status == PathFail {
			return true
		}
	}
	return false
}

func developerPathNextStep(r DeveloperPathReport, setup SetupState) string {
	if r.Ready {
		return "Ready — run hawk and start chatting"
	}
	if !setup.HasCredentials {
		return "Run hawk → /config → paste API key (or Ollama local)"
	}
	if !setup.HasModel {
		return "Run /config → pick a model from the catalog"
	}
	if !r.SecureReady {
		return "Fix security items above (provider.json secrets, read guard)"
	}
	return "Run hawk preflight for details, then /config if needed"
}

// FormatDeveloperPathReport renders the developer path readiness report for CLI/TUI.
func FormatDeveloperPathReport(ctx context.Context) string {
	r := EvaluateDeveloperPath(ctx)
	var b strings.Builder
	b.WriteString("Developer path (hawk · eyrie · tok · yaad)\n\n")

	status := "NEEDS SETUP"
	switch {
	case r.Ready:
		status = "READY"
	case r.ChatReady && !r.SecureReady:
		status = "SECURITY FIX NEEDED"
	case r.SecureReady && !r.ChatReady:
		status = "ALMOST READY"
	}
	b.WriteString("Status: " + status + "\n\n")

	sections := []string{"Setup", "Security", "Sandbox", "Ecosystem"}
	for _, sec := range sections {
		b.WriteString(sec + "\n")
		for _, c := range r.Checks {
			if c.Section != sec {
				continue
			}
			b.WriteString(fmt.Sprintf("  %s %s — %s\n", pathStatusGlyph(c.Status), c.Name, c.Detail))
			if c.FixHint != "" && c.Status != PathPass {
				b.WriteString("      → " + c.FixHint + "\n")
			}
		}
		b.WriteByte('\n')
	}

	b.WriteString("Next: " + r.NextStep + "\n")
	b.WriteString("\nDocs: docs/SECURITY-DEVELOPER.md · hawk doctor · hawk preflight\n")
	return strings.TrimRight(b.String(), "\n")
}

func pathStatusGlyph(s PathCheckStatus) string {
	switch s {
	case PathPass:
		return "✓"
	case PathWarn:
		return "!"
	case PathFail:
		return "✗"
	default:
		return "?"
	}
}

func providerJSONHasSecretsOnDisk() (bool, string) {
	path := eyriecfg.GetProviderConfigPath()
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return false, ""
		}
		return false, ""
	}
	text := string(data)
	for _, needle := range []string{`"api_key"`, `"secret_access_key"`, `"session_token"`} {
		if strings.Contains(text, needle+`": "`) && !strings.Contains(text, needle+`": ""`) {
			return true, "provider.json contains " + needle + " field with value"
		}
	}
	var cfg eyriecfg.ProviderConfig
	if err := json.Unmarshal(data, &cfg); err != nil {
		return false, ""
	}
	for id, dep := range cfg.Deployments {
		if deploymentHasSecrets(dep) {
			return true, "deployment " + id + " has secret fields on disk"
		}
	}
	return false, ""
}

func legacyCredentialFilesPresent() (bool, []string) {
	hawkDir := filepath.Join(home.Dir(), ".hawk")
	var paths []string
	for _, name := range []string{"env", ".env"} {
		p := filepath.Join(hawkDir, name)
		if _, err := os.Stat(p); err == nil {
			paths = append(paths, "~/.hawk/"+name)
		}
	}
	return len(paths) > 0, paths
}
