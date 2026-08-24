package plugin

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"unicode/utf8"
)

// Hardened manifest parsing, ported from Autohand Code CLI's extension
// loader (Apache-2.0, Copyright 2025 Autohand AI LLC): bounded file size,
// duplicate-JSON-key rejection, and strict identity patterns. Duplicate
// keys matter because encoding/json silently keeps the last occurrence,
// letting a hostile manifest show reviewers one value and hawk another.

const maxManifestBytes = 64 * 1024

var (
	manifestNamePattern = regexp.MustCompile(`^[a-zA-Z][a-zA-Z0-9._-]{1,99}$`)
	manifestSemver      = regexp.MustCompile(`^(?:0|[1-9]\d*)\.(?:0|[1-9]\d*)\.(?:0|[1-9]\d*)$`)
)

// readBoundedManifestFile reads a regular, non-symlink file enforcing a
// size cap and valid UTF-8.
func readBoundedManifestFile(path string) ([]byte, error) {
	info, err := os.Lstat(path) // #nosec G304 -- path is pluginDir/plugin.json under a Hawk-managed plugins root
	if err != nil {
		return nil, fmt.Errorf("stat manifest: %w", err)
	}
	if info.Mode()&os.ModeSymlink != 0 {
		return nil, fmt.Errorf("manifest may not be a symlink: %s", filepath.Base(path))
	}
	if !info.Mode().IsRegular() {
		return nil, fmt.Errorf("manifest is not a regular file: %s", filepath.Base(path))
	}
	if info.Size() > maxManifestBytes {
		return nil, fmt.Errorf("manifest exceeds the %d-byte limit", maxManifestBytes)
	}
	data, err := os.ReadFile(path) // #nosec G304 -- see Lstat above
	if err != nil {
		return nil, fmt.Errorf("read manifest: %w", err)
	}
	if !utf8.Valid(data) {
		return nil, fmt.Errorf("manifest is not valid UTF-8")
	}
	return data, nil
}

// findDuplicateJSONKey reports the first duplicated key in any JSON object
// in data, or "" when every object has unique keys. The scanner walks the
// token stream tracking whether each string sits in a key or value slot.
func findDuplicateJSONKey(data []byte) string {
	dec := json.NewDecoder(bytes.NewReader(data))
	dec.UseNumber()

	type frame struct {
		isObject    bool
		keys        map[string]bool
		expectValue bool // object frames: next string is a value, not a key
	}
	var stack []*frame
	top := func() *frame {
		if len(stack) == 0 {
			return nil
		}
		return stack[len(stack)-1]
	}

	for {
		tok, err := dec.Token()
		if err != nil {
			return ""
		}
		switch t := tok.(type) {
		case json.Delim:
			switch t {
			case '{':
				stack = append(stack, &frame{isObject: true, keys: map[string]bool{}})
			case '[':
				stack = append(stack, &frame{})
			default:
				stack = stack[:len(stack)-1]
				if p := top(); p != nil && p.isObject && p.expectValue {
					p.expectValue = false // closed container filled the value slot
				}
			}
		case string:
			f := top()
			if f != nil && f.isObject {
				if !f.expectValue {
					if f.keys[t] {
						return t
					}
					f.keys[t] = true
					f.expectValue = true
					continue
				}
				f.expectValue = false
			}
		default:
			// Numbers, booleans, null fill a value slot.
			if f := top(); f != nil && f.isObject && f.expectValue {
				f.expectValue = false
			}
		}
	}
}

// ValidateManifestIdentity enforces strict name/version patterns beyond
// the non-empty checks in ParseManifestV2.
func ValidateManifestIdentity(name, version string) []string {
	var issues []string
	if name != "" && !manifestNamePattern.MatchString(name) {
		issues = append(issues, fmt.Sprintf(
			"name %q must start with a letter and use only letters, digits, '.', '_' or '-' (max 100 chars)", name,
		))
	}
	if version != "" && !manifestSemver.MatchString(version) {
		issues = append(issues, fmt.Sprintf("version %q must be strict semver (MAJOR.MINOR.PATCH)", version))
	}
	return issues
}
