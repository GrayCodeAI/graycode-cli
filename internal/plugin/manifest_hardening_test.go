package plugin

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestFindDuplicateJSONKey(t *testing.T) {
	cases := []struct {
		name string
		json string
		want string
	}{
		{"object in array", `[{"a":1},{"b":2,"a":3}]`, ""},
		{"clean object", `{"a":1,"b":2}`, ""},
		{"top-level dup", `{"a":1,"a":2}`, "a"},
		{"dup inside nested object", `{"x":{"p":"value with a: colon","q":2,"p":3}}`, "p"},
		{"string value that looks like key", `{"k":"k","j":"k"}`, ""},
		{"array of strings", `{"a":["a","a","b"]}`, ""},
		{"numbers and bools", `{"n":1.5,"t":true,"f":null,"m":{"z":0,"z":1}}`, "z"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := findDuplicateJSONKey([]byte(tc.json)); got != tc.want {
				t.Fatalf("findDuplicateJSONKey(%s) = %q, want %q", tc.json, got, tc.want)
			}
		})
	}
}

func TestReadBoundedManifestFile(t *testing.T) {
	dir := t.TempDir()

	good := filepath.Join(dir, "plugin.json")
	if err := os.WriteFile(good, []byte(`{"name":"ok","version":"1.0.0"}`), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := readBoundedManifestFile(good); err != nil {
		t.Fatalf("regular file should parse: %v", err)
	}

	big := filepath.Join(dir, "big.json")
	if err := os.WriteFile(big, []byte(strings.Repeat("x", maxManifestBytes+1)), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := readBoundedManifestFile(big); err == nil || !strings.Contains(err.Error(), "limit") {
		t.Fatalf("oversized file must be rejected, got %v", err)
	}

	invalid := filepath.Join(dir, "bad.json")
	if err := os.WriteFile(invalid, []byte{0xff, 0xfe}, 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := readBoundedManifestFile(invalid); err == nil || !strings.Contains(err.Error(), "UTF-8") {
		t.Fatalf("invalid UTF-8 must be rejected, got %v", err)
	}
}

func TestValidateManifestIdentity(t *testing.T) {
	if issues := ValidateManifestIdentity("go-review", "1.2.3"); len(issues) != 0 {
		t.Fatalf("valid identity flagged: %v", issues)
	}
	issues := ValidateManifestIdentity("1bad name", "v1.0")
	if len(issues) != 2 {
		t.Fatalf("expected both patterns to fail, got %v", issues)
	}
	if !strings.Contains(issues[0], "name") || !strings.Contains(issues[1], "semver") {
		t.Fatalf("unexpected issue text: %v", issues)
	}
	// Empty strings are handled by the required-field checks elsewhere.
	if issues := ValidateManifestIdentity("", ""); len(issues) != 0 {
		t.Fatalf("empty identity should be skipped here, got %v", issues)
	}
}
