package sandbox

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/BurntSushi/toml"

	"github.com/GrayCodeAI/hawk/internal/fsutil"
	"github.com/GrayCodeAI/hawk/internal/storage"
)

// Named profiles from the Year 0 plan.
const (
	ProfileOff       = "off"
	ProfileWorkspace = "workspace"
	ProfileReadOnly  = "read-only"
	ProfileStrict    = "strict"
	ProfileDevbox    = "devbox"
	ProfileCustom    = "custom"
)

// TOMLConfig is the on-disk sandbox.toml shape.
type TOMLConfig struct {
	Profile  string                   `toml:"profile"`
	Profiles map[string]ProfileConfig `toml:"profiles"`
	// DenyGlobs are always fail-closed path patterns (user or merged).
	DenyGlobs []string `toml:"deny_globs"`
}

// ProfileConfig describes one named profile.
type ProfileConfig struct {
	// Mode: off | workspace | strict (maps to sandbox.Mode).
	Mode string `toml:"mode"`
	// Extends another profile name (resolved once).
	Extends string `toml:"extends"`
	// AllowNetwork overrides ModeAllowsNetwork when set.
	AllowNetwork *bool `toml:"allow_network"`
	// DenyGlobs are fail-closed globs for this profile.
	DenyGlobs []string `toml:"deny_globs"`
}

// Effective is the resolved sandbox configuration after merge.
type Effective struct {
	Profile      string
	Mode         Mode
	AllowNetwork bool
	DenyGlobs    []string
}

// UserSandboxTOMLPath is ~/.hawk/sandbox.toml (or HAWK_CONFIG_DIR).
func UserSandboxTOMLPath() string {
	return filepath.Join(storage.ConfigDir(), "sandbox.toml")
}

// ProjectSandboxTOMLPath is <project>/.hawk/sandbox.toml.
func ProjectSandboxTOMLPath(projectRoot string) string {
	return filepath.Join(projectRoot, ".hawk", "sandbox.toml")
}

// LoadTOML reads a sandbox.toml file. Missing file returns empty config.
func LoadTOML(path string) (TOMLConfig, error) {
	var cfg TOMLConfig
	data, err := fsutil.ReadPinnedFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return cfg, nil
		}
		return cfg, err
	}
	if _, err := toml.Decode(string(data), &cfg); err != nil {
		return cfg, fmt.Errorf("sandbox.toml: %w", err)
	}
	if cfg.Profiles == nil {
		cfg.Profiles = map[string]ProfileConfig{}
	}
	return cfg, nil
}

// builtinProfiles returns built-in profile definitions.
func builtinProfiles() map[string]ProfileConfig {
	netOn := true
	netOff := false
	return map[string]ProfileConfig{
		ProfileOff: {
			Mode:         string(ModeOff),
			AllowNetwork: &netOn,
		},
		ProfileWorkspace: {
			Mode:         string(ModeWorkspace),
			AllowNetwork: &netOn,
		},
		ProfileReadOnly: {
			Mode:         string(ModeStrict),
			AllowNetwork: &netOff,
		},
		ProfileStrict: {
			Mode:         string(ModeStrict),
			AllowNetwork: &netOff,
		},
		ProfileDevbox: {
			Mode:         string(ModeWorkspace),
			AllowNetwork: &netOn,
		},
		ProfileCustom: {
			Mode:         string(ModeWorkspace),
			AllowNetwork: &netOn,
		},
	}
}

// MergeConfigs merges user and project sandbox.toml.
// Project may only ADD profile names (not redefine built-ins/user profiles)
// and may only APPEND deny_globs (never remove). Project cannot lower the
// active profile to a weaker mode than user — if project sets profile, it
// must not be weaker than user's mode.
func MergeConfigs(user, project TOMLConfig) (TOMLConfig, error) {
	out := TOMLConfig{
		Profile:   user.Profile,
		Profiles:  map[string]ProfileConfig{},
		DenyGlobs: append([]string{}, user.DenyGlobs...),
	}
	// Start with builtins then user overrides.
	for k, v := range builtinProfiles() {
		out.Profiles[k] = v
	}
	for k, v := range user.Profiles {
		out.Profiles[k] = v
	}
	// Project: additive profiles only (new names).
	for k, v := range project.Profiles {
		if _, exists := out.Profiles[k]; exists {
			// Skip redefinition of existing profiles (security).
			continue
		}
		out.Profiles[k] = v
	}
	// Project may only append deny globs.
	out.DenyGlobs = append(out.DenyGlobs, project.DenyGlobs...)

	if project.Profile != "" {
		// Allow project to select a profile name that exists after merge,
		// but not weaken vs user mode.
		userMode := resolveMode(out, user.Profile)
		projMode := resolveMode(out, project.Profile)
		if weaker(projMode, userMode) {
			return out, fmt.Errorf("sandbox.toml: project profile %q is weaker than user profile %q — denied", project.Profile, user.Profile)
		}
		out.Profile = project.Profile
	}
	if out.Profile == "" {
		out.Profile = ProfileWorkspace
	}
	return out, nil
}

func resolveMode(cfg TOMLConfig, name string) Mode {
	if name == "" {
		name = ProfileWorkspace
	}
	p, ok := cfg.Profiles[name]
	if !ok {
		// Try builtins via name aliases
		switch strings.ToLower(name) {
		case "readonly", "read_only":
			return ModeStrict
		default:
			return ParseMode(name)
		}
	}
	if p.Extends != "" {
		if base, ok := cfg.Profiles[p.Extends]; ok && p.Mode == "" {
			return ParseMode(base.Mode)
		}
	}
	if p.Mode == "" {
		return ModeWorkspace
	}
	return ParseMode(p.Mode)
}

// weaker reports whether a is weaker isolation than b.
// off < workspace < strict
func weaker(a, b Mode) bool {
	rank := func(m Mode) int {
		switch m {
		case ModeOff:
			return 0
		case ModeWorkspace:
			return 1
		case ModeStrict:
			return 2
		default:
			return 1
		}
	}
	return rank(a) < rank(b)
}

// Resolve loads user + optional project sandbox.toml and returns Effective.
func Resolve(projectRoot string) (Effective, error) {
	user, err := LoadTOML(UserSandboxTOMLPath())
	if err != nil {
		return Effective{}, err
	}
	var project TOMLConfig
	if projectRoot != "" {
		project, err = LoadTOML(ProjectSandboxTOMLPath(projectRoot))
		if err != nil {
			return Effective{}, err
		}
	}
	merged, err := MergeConfigs(user, project)
	if err != nil {
		return Effective{}, err
	}
	return EffectiveFrom(merged)
}

// EffectiveFrom resolves profile fields on a merged config.
func EffectiveFrom(cfg TOMLConfig) (Effective, error) {
	name := cfg.Profile
	if name == "" {
		name = ProfileWorkspace
	}
	p, ok := cfg.Profiles[name]
	if !ok {
		// Allow bare mode names as profile.
		p = ProfileConfig{Mode: name}
	}
	if p.Extends != "" {
		if base, ok := cfg.Profiles[p.Extends]; ok {
			if p.Mode == "" {
				p.Mode = base.Mode
			}
			if p.AllowNetwork == nil {
				p.AllowNetwork = base.AllowNetwork
			}
			p.DenyGlobs = append(append([]string{}, base.DenyGlobs...), p.DenyGlobs...)
		}
	}
	mode := ParseMode(p.Mode)
	if p.Mode == "" {
		mode = ModeWorkspace
	}
	allowNet := ModeAllowsNetwork(mode)
	if p.AllowNetwork != nil {
		allowNet = *p.AllowNetwork
	}
	globs := append([]string{}, cfg.DenyGlobs...)
	globs = append(globs, p.DenyGlobs...)
	return Effective{
		Profile:      name,
		Mode:         mode,
		AllowNetwork: allowNet,
		DenyGlobs:    globs,
	}, nil
}

// PathDenied reports whether path matches any deny glob (fail-closed).
// Supports simple ** and * via filepath.Match on each path segment pattern,
// and substring suffix checks for patterns like **/.env.
func (e Effective) PathDenied(path string) bool {
	clean := filepath.Clean(path)
	base := filepath.Base(clean)
	for _, g := range e.DenyGlobs {
		g = strings.TrimSpace(g)
		if g == "" {
			continue
		}
		// **/.env style
		if strings.HasPrefix(g, "**/") {
			suf := strings.TrimPrefix(g, "**/")
			if base == suf || strings.HasSuffix(clean, string(filepath.Separator)+suf) {
				return true
			}
			if matched, _ := filepath.Match(suf, base); matched {
				return true
			}
			continue
		}
		if matched, _ := filepath.Match(g, base); matched {
			return true
		}
		if matched, _ := filepath.Match(g, clean); matched {
			return true
		}
	}
	return false
}
