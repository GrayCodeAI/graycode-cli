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
//
// Security: the file is project-controlled and agent-writable, and its deps
// become root shell layers at image build time (network unrestricted). Every
// entry is therefore validated: commands containing network-fetch or
// arbitrary-code-exec tools, and startup env vars that can hijack the runtime
// (PATH, HOME, LD_*, DYLD_*, credential patterns), are rejected with a
// warning. Rejected entries are dropped — never executed.
func LoadRuntimeConfig(projectDir string) RuntimeConfig {
	path := filepath.Join(projectDir, ".agents", "runtime.jsonc")
	data, err := os.ReadFile(path) // #nosec G304 -- path is rooted in the project directory, a trusted internal config location
	if err != nil {
		return RuntimeConfig{}
	}
	cleaned := stripJSONComments(string(data))
	var cfg RuntimeConfig
	if err := json.Unmarshal([]byte(cleaned), &cfg); err != nil {
		slog.Warn("sandbox: failed to parse runtime config", "path", path, "error", err)
		return RuntimeConfig{}
	}
	cfg = sanitizeRuntimeConfig(cfg, path)
	return cfg
}

// blocklistedDepTerms are substrings that make a runtime_extra_deps command
// inadmissible: shell constructs that compose arbitrary execution. Tools that
// fetch binaries or exfiltrate data (curl, python, ...) are matched separately
// by token boundary in toolNameTerms. Package managers (apt-get, apk, dnf,
// yum, brew, go install, npm install, pip install, cargo install, make, cmake)
// remain allowed — that is the feature.
var blocklistedDepTerms = []string{
	"$(", "`", "| sh", "| bash", "sh -c", "bash -c", "chmod +s", "eval ",
	"nohup ", "setsid ", "xargs ", "aria2c", "openssl s_client",
}

// toolNameTerms are executables that fetch binaries or exfiltrate data from
// the image build. They are matched as whole tokens (with optional version
// suffixes such as python3 or node20) so package names like "nodejs" or words
// like "sync" are not false positives.
var toolNameTerms = []string{
	"curl", "wget", "nc", "ncat", "socat", "telnet", "ftp", "sftp", "scp",
	"rsync", "ssh", "python", "perl", "ruby", "php", "lua", "node", "npx",
}

// blocklistedEnvKeyPatterns are substrings that make a startup env var
// inadmissible: runtime hijack vectors and anything that looks like a
// credential (which must never be injected into the sandbox image from an
// untrusted project file).
var blocklistedEnvKeyPatterns = []string{
	"PATH", "HOME", "LD_", "DYLD_", "API_KEY", "TOKEN", "SECRET",
	"PASSWORD", "CREDENTIAL", "SSL_CERT", "GIT_SSH", "GIT_ASKPASS",
	"SSH_AUTH_SOCK",
}

// sanitizeRuntimeConfig drops every dep command and env var that fails
// validation, logging each rejection with the offending value so the user can
// fix the file. The returned config is guaranteed to contain only validated
// entries.
func sanitizeRuntimeConfig(cfg RuntimeConfig, path string) RuntimeConfig {
	valid := cfg.RuntimeExtraDeps[:0]
	for _, dep := range cfg.RuntimeExtraDeps {
		dep = strings.TrimSpace(dep)
		if dep == "" {
			continue
		}
		if term := blockedDepTerm(dep); term != "" {
			slog.Warn("sandbox: rejecting runtime_extra_deps entry (contains blocked term)",
				"path", path, "term", term, "command", dep)
			continue
		}
		valid = append(valid, dep)
	}
	cfg.RuntimeExtraDeps = valid

	if len(cfg.RuntimeStartupEnvVars) > 0 {
		envs := make(map[string]string, len(cfg.RuntimeStartupEnvVars))
		for k, v := range cfg.RuntimeStartupEnvVars {
			if pattern := blockedEnvKey(k); pattern != "" {
				slog.Warn("sandbox: rejecting runtime_startup_env_vars key",
					"path", path, "pattern", pattern, "key", k)
				continue
			}
			envs[k] = v
		}
		cfg.RuntimeStartupEnvVars = envs
	}
	return cfg
}

// blockedDepTerm returns the first blocked term contained in the command, or
// "" when the command passes validation.
func blockedDepTerm(command string) string {
	lower := strings.ToLower(command)
	for _, term := range blocklistedDepTerms {
		if strings.Contains(lower, term) {
			return term
		}
	}
	for _, tool := range toolNameTerms {
		if toolTokenAtBoundary(lower, tool) {
			return tool
		}
	}
	return ""
}

// toolTokenAtBoundary reports whether tool appears in s as a standalone token,
// allowing version suffixes (python3, node20, go1.22) but not word
// continuations (nodejs, sync, curlew).
func toolTokenAtBoundary(s, tool string) bool {
	for i := 0; ; {
		j := strings.Index(s[i:], tool)
		if j < 0 {
			return false
		}
		j += i
		startOK := j == 0 || !isWordChar(s[j-1])
		k := j + len(tool)
		for k < len(s) && isVersionChar(s[k]) {
			k++
		}
		endOK := k == len(s) || !isWordChar(s[k])
		if startOK && endOK {
			return true
		}
		i = j + len(tool)
	}
}

// isWordChar matches the letters/digits/underscore that form a shell word.
func isWordChar(c byte) bool {
	return c == '_' || (c >= 'a' && c <= 'z') || (c >= '0' && c <= '9')
}

// isVersionChar matches version-suffix characters (python3.12, node20).
func isVersionChar(c byte) bool {
	return c == '.' || c == '-' || c == '+' || (c >= '0' && c <= '9')
}

// blockedEnvKey returns the first blocked pattern contained in the env var
// key, or "" when the key passes validation.
func blockedEnvKey(key string) string {
	upper := strings.ToUpper(key)
	for _, pattern := range blocklistedEnvKeyPatterns {
		if strings.Contains(upper, pattern) {
			return pattern
		}
	}
	return ""
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
