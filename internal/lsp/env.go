package lsp

import (
	"os"
	"sort"
	"strings"

	"github.com/GrayCodeAI/tok"
)

var sensitiveKeySubstrings = []string{
	"KEY",
	"PASSWORD",
	"SECRET",
	"TOKEN",
	"AUTH",
	"CREDENTIAL",
	"PRIVATE",
	"BEARER",
	"SIGNATURE",
	"APIKEY",
}

// IsSensitiveEnvKey returns true if the environment variable key matches known secret/credential patterns.
func IsSensitiveEnvKey(key string) bool {
	upper := strings.ToUpper(strings.TrimSpace(key))
	for _, sub := range sensitiveKeySubstrings {
		if strings.Contains(upper, sub) {
			return true
		}
	}
	return false
}

// IsSensitiveEnvEntry returns true if the key is sensitive or the value contains detected secrets.
func IsSensitiveEnvEntry(key, val string) bool {
	if IsSensitiveEnvKey(key) {
		return true
	}
	if len(val) > 0 {
		matches := tok.DefaultSecretDetector().DetectSecrets(val)
		if len(matches) > 0 {
			return true
		}
	}
	return false
}

// ScrubEnvironment filters ambient environment variables, removing sensitive tokens and secrets,
// and then merges explicit configuration key-value pairs.
func ScrubEnvironment(ambient []string, explicit map[string]string) []string {
	if ambient == nil {
		ambient = os.Environ()
	}

	resultMap := make(map[string]string)

	for _, envEntry := range ambient {
		parts := strings.SplitN(envEntry, "=", 2)
		if len(parts) == 0 {
			continue
		}
		key := parts[0]
		val := ""
		if len(parts) > 1 {
			val = parts[1]
		}

		if !IsSensitiveEnvEntry(key, val) {
			resultMap[key] = val
		}
	}

	// Merge explicit configuration values (overriding or adding keys)
	for k, v := range explicit {
		resultMap[k] = v
	}

	var result []string
	for k, v := range resultMap {
		result = append(result, k+"="+v)
	}
	sort.Strings(result)

	return result
}
