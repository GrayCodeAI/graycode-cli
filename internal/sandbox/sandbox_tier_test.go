//nolint:errcheck
package sandbox

import (
	"encoding/json"
	"testing"
)

// --- Config tier ---

func TestDefaultConfig_DefaultTierIsWorkspace(t *testing.T) {
	c := DefaultConfig()
	if c.Security != TierWorkspace {
		t.Errorf("default Tier = %q, want %q", c.Security, TierWorkspace)
	}
}

func TestConfig_TierJSONRoundTrip(t *testing.T) {
	cases := []struct {
		name string
		tier Security
	}{
		{"strict", SecurityStrict},
		{"workspace", SecurityWorkspace},
		{"off", SecurityOff},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			c := &Config{Security: tc.tier}
			data, err := json.Marshal(c)
			if err != nil {
				t.Fatalf("marshal: %v", err)
			}
			// Confirm the JSON contains the tier field.
			if !tierContains(string(data), `"tier":"`+string(tc.tier)+`"`) {
				t.Errorf("JSON does not contain tier=%q: %s", tc.tier, data)
			}
			var decoded Config
			if err := json.Unmarshal(data, &decoded); err != nil {
				t.Fatalf("unmarshal: %v", err)
			}
			if decoded.Security != tc.tier {
				t.Errorf("round-trip Tier = %q, want %q", decoded.Security, tc.tier)
			}
		})
	}
}

func TestConfig_TierUnmarshalDefaultsToWorkspace(t *testing.T) {
	// A config with no tier field (legacy JSON) defaults to empty
	// string. The seatbelt layer treats empty as TierOff (legacy).
	var c Config
	if err := json.Unmarshal([]byte(`{"enabled":true}`), &c); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if c.Security != "" {
		t.Errorf("legacy config Tier = %q, want empty (treated as TierOff downstream)", c.Security)
	}
}

// --- DefaultHawkPolicy tier ---

func TestDefaultHawkPolicy_TierWorkspace(t *testing.T) {
	p := DefaultHawkPolicy("/tmp/work", TierWorkspace)
	if p.Security != TierWorkspace {
		t.Errorf("Tier = %q, want %q", p.Security, TierWorkspace)
	}
	if !p.AllowWrite {
		t.Error("AllowWrite = false, want true (TierWorkspace allows workspace writes)")
	}
	if p.AllowProcess {
		t.Error("AllowProcess = true, want false (TierWorkspace denies process exec)")
	}
}

func TestDefaultHawkPolicy_TierStrict(t *testing.T) {
	p := DefaultHawkPolicy("/tmp/work", TierStrict)
	if p.Security != TierStrict {
		t.Errorf("Tier = %q, want %q", p.Security, TierStrict)
	}
	if p.AllowWrite {
		t.Error("AllowWrite = true, want false (TierStrict denies all writes)")
	}
	if p.AllowProcess {
		t.Error("AllowProcess = true, want false (TierStrict denies process exec)")
	}
	if p.AllowNetwork {
		t.Error("AllowNetwork = true, want false (TierStrict denies network)")
	}
}

func TestDefaultHawkPolicy_TierOff(t *testing.T) {
	// TierOff is the legacy behavior: allow everything.
	p := DefaultHawkPolicy("/tmp/work", TierOff)
	if p.Security != TierOff {
		t.Errorf("Tier = %q, want %q", p.Security, TierOff)
	}
	if !p.AllowWrite {
		t.Error("AllowWrite = false, want true (TierOff allows writes)")
	}
	if !p.AllowProcess {
		t.Error("AllowProcess = false, want true (TierOff allows process exec)")
	}
}

func TestDefaultHawkPolicy_EmptyTierFallsBackToOff(t *testing.T) {
	// Empty tier (legacy config) → TierOff behavior. This is the
	// silent-preserve migration: a user with no Tier field keeps
	// the old default-deny-nothing behavior.
	p := DefaultHawkPolicy("/tmp/work", "")
	if p.AllowWrite != true {
		t.Error("empty Tier: AllowWrite = false, want true (silent preserve)")
	}
	if p.AllowProcess != true {
		t.Error("empty Tier: AllowProcess = false, want true (silent preserve)")
	}
}

func TestDefaultHawkPolicy_UnknownTierFallsBackToOff(t *testing.T) {
	// Unknown tier values fall back to TierOff rather than silently
	// applying a wrong policy.
	p := DefaultHawkPolicy("/tmp/work", Tier("nonsense"))
	if p.AllowWrite != true {
		t.Error("unknown Tier: AllowWrite = false, want true (fallback to TierOff)")
	}
	if p.AllowProcess != true {
		t.Error("unknown Tier: AllowProcess = false, want true (fallback to TierOff)")
	}
}

// --- Tier values ---

func TestTierConstants(t *testing.T) {
	if TierStrict != "strict" {
		t.Errorf("TierStrict = %q, want \"strict\"", TierStrict)
	}
	if TierWorkspace != "workspace" {
		t.Errorf("TierWorkspace = %q, want \"workspace\"", TierWorkspace)
	}
	if TierOff != "off" {
		t.Errorf("TierOff = %q, want \"off\"", TierOff)
	}
}

func TestDefaultHawkPolicy_NetworkByTier(t *testing.T) {
	cases := []struct {
		tier Tier
		want bool
	}{
		{TierStrict, false},
		{TierWorkspace, true},
		{TierOff, true},
		{"", true},
		{Tier("nonsense"), true},
	}
	for _, tc := range cases {
		t.Run(string(tc.tier), func(t *testing.T) {
			p := DefaultHawkPolicy("/tmp/work", tc.tier)
			if p.AllowNetwork != tc.want {
				t.Errorf("AllowNetwork = %v, want %v", p.AllowNetwork, tc.want)
			}
			if !p.AllowSysctl {
				t.Errorf("tier=%s: AllowSysctl = false, want true", tc.tier)
			}
		})
	}
}

// --- DefaultHawkPolicy always populates the path lists ---

func TestDefaultHawkPolicy_PathsPopulated(t *testing.T) {
	p := DefaultHawkPolicy("/tmp/work", TierWorkspace)
	if len(p.ReadablePaths) == 0 {
		t.Error("ReadablePaths is empty")
	}
	if len(p.WritablePaths) == 0 {
		t.Error("WritablePaths is empty")
	}
	// workDir should be in the readable paths.
	found := false
	for _, p := range p.ReadablePaths {
		if p == "/tmp/work" {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("workDir /tmp/work not in ReadablePaths: %v", p.ReadablePaths)
	}
}

// --- Regression guard: the old default was AllowProcess=true; the
// new default is AllowProcess=false. This test pins the new default
// so a future refactor that accidentally restores the legacy
// default will fail. ---

func TestDefaultHawkPolicy_NewDefaultDeniesProcess(t *testing.T) {
	// The new default is TierWorkspace (set in DefaultConfig).
	// TierWorkspace MUST deny AllowProcess.
	workspace := DefaultHawkPolicy("/tmp/work", TierWorkspace)
	if workspace.AllowProcess {
		t.Error("TierWorkspace allows process exec; this is the legacy TierOff behavior. The new default is supposed to be safer.")
	}
}

// tierContains is a tiny strings.Contains shim to avoid an extra
// import in this test file.
func tierContains(haystack, needle string) bool {
	if len(needle) == 0 {
		return true
	}
	for i := 0; i+len(needle) <= len(haystack); i++ {
		if haystack[i:i+len(needle)] == needle {
			return true
		}
	}
	return false
}
