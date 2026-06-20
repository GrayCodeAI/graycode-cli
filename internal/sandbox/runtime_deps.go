package sandbox

import (
	"encoding/json"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
)

// RuntimeConfig declares additive runtime customizations for a container
// sandbox. It is parsed from .agents/runtime.jsonc (project) and is fully
// optional: the empty value reproduces the current behavior exactly.
type RuntimeConfig struct {
	// RuntimeExtraDeps is a list of shell commands run during image build,
	// after the base image, to install extra dependencies. Each entry becomes
	// its own RUN layer in the generated Dockerfile fragment.
	RuntimeExtraDeps []string `json:"runtime_extra_deps"`
	// RuntimeStartupEnvVars are environment variables injected when the
	// container starts (docker run -e KEY=VALUE).
	RuntimeStartupEnvVars map[string]string `json:"runtime_startup_env_vars"`
}

// IsEmpty reports whether the config carries no customizations.
func (c RuntimeConfig) IsEmpty() bool {
	return len(c.RuntimeExtraDeps) == 0 && len(c.RuntimeStartupEnvVars) == 0
}

// LoadRuntimeConfig reads .agents/runtime.jsonc from projectDir. A missing file
// yields a zero RuntimeConfig and no error; a malformed file is logged and
// also yields a zero config (fail-open to current behavior, since this is
// purely additive).
func LoadRuntimeConfig(projectDir string) RuntimeConfig {
	path := filepath.Join(projectDir, ".agents", "runtime.jsonc")
	data, err := os.ReadFile(path)
	if err != nil {
		return RuntimeConfig{}
	}
	cleaned := stripJSONComments(string(data))
	var cfg RuntimeConfig
	if err := json.Unmarshal([]byte(cleaned), &cfg); err != nil {
		slog.Warn("sandbox: failed to parse runtime config", "path", path, "error", err)
		return RuntimeConfig{}
	}
	return cfg
}

// ExtraDepsDockerfileFragment composes the RUN layers for runtime_extra_deps.
// Returns "" when there are no extra deps. Each command is emitted as its own
// RUN instruction so build-cache invalidation is granular and errors are
// attributable to a single command. Blank/whitespace-only entries are skipped.
func (c RuntimeConfig) ExtraDepsDockerfileFragment() string {
	if len(c.RuntimeExtraDeps) == 0 {
		return ""
	}
	var b strings.Builder
	for _, dep := range c.RuntimeExtraDeps {
		dep = strings.TrimSpace(dep)
		if dep == "" {
			continue
		}
		b.WriteString("RUN ")
		b.WriteString(dep)
		b.WriteString("\n")
	}
	return b.String()
}

// AppendExtraDeps returns dockerfile with the extra-deps RUN layers appended.
// When there are no extra deps, dockerfile is returned unchanged. A trailing
// newline is ensured before appending so the fragment is well-formed.
func (c RuntimeConfig) AppendExtraDeps(dockerfile string) string {
	fragment := c.ExtraDepsDockerfileFragment()
	if fragment == "" {
		return dockerfile
	}
	if dockerfile != "" && !strings.HasSuffix(dockerfile, "\n") {
		dockerfile += "\n"
	}
	return dockerfile + fragment
}

// StartupEnvArgs returns the "-e KEY=VALUE" docker run arguments for the
// configured startup env vars, sorted by key for deterministic output. Returns
// nil when there are no env vars.
func (c RuntimeConfig) StartupEnvArgs() []string {
	if len(c.RuntimeStartupEnvVars) == 0 {
		return nil
	}
	keys := make([]string, 0, len(c.RuntimeStartupEnvVars))
	for k := range c.RuntimeStartupEnvVars {
		keys = append(keys, k)
	}
	sortStrings(keys)
	args := make([]string, 0, len(keys)*2)
	for _, k := range keys {
		args = append(args, "-e", k+"="+c.RuntimeStartupEnvVars[k])
	}
	return args
}

// sortStrings is a tiny insertion sort to avoid importing "sort" for a handful
// of env var keys (and to keep this file dependency-light).
func sortStrings(s []string) {
	for i := 1; i < len(s); i++ {
		for j := i; j > 0 && s[j-1] > s[j]; j-- {
			s[j-1], s[j] = s[j], s[j-1]
		}
	}
}
