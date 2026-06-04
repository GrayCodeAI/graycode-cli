// validate.go: pre-write validation for Claude Code settings fields.
//
// Settings.json hook-field validation.
// performs a Zod-style pre-check on Claude Code settings.json before
// writing. This file ports that check to native Go, using the
// encoding/json conventions of the existing internal/config package.
//
// Validation is permissive by design: unknown fields are allowed
// (forward compat), but known fields are type-checked and value
// validated. The intent is to catch typos and accidental schema
// changes before they corrupt a user's settings.
package jsonc

import (
	"errors"
	"fmt"
	"regexp"
)

// ErrValidation is the base class for all field validation errors.
type ErrValidation struct {
	Field  string
	Reason string
}

func (e *ErrValidation) Error() string {
	return fmt.Sprintf("jsonc: invalid field %q: %s", e.Field, e.Reason)
}

// IsValidation reports whether err is an ErrValidation.
func IsValidation(err error) bool {
	var v *ErrValidation
	return errors.As(err, &v)
}

// ValidationResult is the outcome of validating a Claude Code
// settings document. Errors are non-empty only if validation failed.
type ValidationResult struct {
	Errors []error
}

// Valid reports whether r contains no validation errors.
func (r *ValidationResult) Valid() bool {
	return len(r.Errors) == 0
}

// Add appends a validation error to r.
func (r *ValidationResult) Add(field, reason string) {
	r.Errors = append(r.Errors, &ErrValidation{Field: field, Reason: reason})
}

// AddErr appends a pre-built error to r.
func (r *ValidationResult) AddErr(err error) {
	if err == nil {
		return
	}
	r.Errors = append(r.Errors, err)
}

// ValidateClaudeSettings validates a parsed Claude Code settings
// document (a map[string]interface{}). Returns a ValidationResult
// describing any issues. The function never returns a non-nil error;
// validation problems are reported through the result.
func ValidateClaudeSettings(doc map[string]interface{}) *ValidationResult {
	r := &ValidationResult{}

	// Top-level field whitelist (unknown fields are allowed but flagged)
	// We don't reject unknown fields — Claude Code is additive — but
	// we do check the well-known ones.

	// model: string
	if v, ok := doc["model"]; ok {
		if s, ok := v.(string); !ok || s == "" {
			r.Add("model", "must be a non-empty string")
		}
	}

	// permissions: object
	if v, ok := doc["permissions"]; ok {
		validatePermissions(v, r)
	}

	// hooks: object
	if v, ok := doc["hooks"]; ok {
		validateHooks(v, r)
	}

	// mcpServers: object
	if v, ok := doc["mcpServers"]; ok {
		validateMCPServers(v, r)
	}

	// env: object (string -> string)
	if v, ok := doc["env"]; ok {
		validateEnv(v, r)
	}

	// includeCoAuthoredBy: bool
	if v, ok := doc["includeCoAuthoredBy"]; ok {
		if _, ok := v.(bool); !ok {
			r.Add("includeCoAuthoredBy", "must be a boolean")
		}
	}

	// cleanupPeriodDays: number
	if v, ok := doc["cleanupPeriodDays"]; ok {
		if n, ok := v.(float64); !ok || n < 0 {
			r.Add("cleanupPeriodDays", "must be a non-negative number")
		}
	}

	// forceLoginMethod: string
	if v, ok := doc["forceLoginMethod"]; ok {
		if s, ok := v.(string); !ok || s == "" {
			r.Add("forceLoginMethod", "must be a non-empty string")
		}
	}

	// apiKeyHelper: string (script path)
	if v, ok := doc["apiKeyHelper"]; ok {
		if s, ok := v.(string); !ok || s == "" {
			r.Add("apiKeyHelper", "must be a non-empty string")
		}
	}

	return r
}

// validatePermissions validates the permissions field.
func validatePermissions(v interface{}, r *ValidationResult) {
	perms, ok := v.(map[string]interface{})
	if !ok {
		r.Add("permissions", "must be an object")
		return
	}
	for _, field := range []string{"allow", "deny", "ask"} {
		if vv, ok := perms[field]; ok {
			if arr, ok := vv.([]interface{}); !ok {
				r.Add("permissions."+field, "must be an array of strings")
			} else {
				for i, item := range arr {
					if s, ok := item.(string); !ok || s == "" {
						r.Add(fmt.Sprintf("permissions.%s[%d]", field, i), "must be a non-empty string")
					}
				}
			}
		}
	}
	// defaultMode: "acceptEdits" | "ask" | "deny" | "plan"
	if vv, ok := perms["defaultMode"]; ok {
		s, ok := vv.(string)
		if !ok {
			r.Add("permissions.defaultMode", "must be a string")
		} else {
			switch s {
			case "acceptEdits", "ask", "deny", "plan":
				// OK
			default:
				r.Add("permissions.defaultMode", "must be one of acceptEdits, ask, deny, plan")
			}
		}
	}
}

// validateHooks validates the hooks field.
func validateHooks(v interface{}, r *ValidationResult) {
	hooks, ok := v.(map[string]interface{})
	if !ok {
		r.Add("hooks", "must be an object")
		return
	}
	for event, hooksForEvent := range hooks {
		arr, ok := hooksForEvent.([]interface{})
		if !ok {
			r.Add("hooks."+event, "must be an array of hook matchers")
			continue
		}
		for i, item := range arr {
			matcher, ok := item.(map[string]interface{})
			if !ok {
				r.Add(fmt.Sprintf("hooks.%s[%d]", event, i), "must be an object")
				continue
			}
			// matcher is optional (string or absent)
			if m, ok := matcher["matcher"]; ok {
				if s, ok := m.(string); !ok {
					r.Add(fmt.Sprintf("hooks.%s[%d].matcher", event, i), "must be a string")
				} else if !validMatcherRegex(s) {
					r.Add(fmt.Sprintf("hooks.%s[%d].matcher", event, i),
						"must be a valid regex (try escaping special chars)")
				}
			}
			// hooks is required
			if h, ok := matcher["hooks"]; ok {
				harr, ok := h.([]interface{})
				if !ok {
					r.Add(fmt.Sprintf("hooks.%s[%d].hooks", event, i), "must be an array")
				} else {
					for j, hh := range harr {
						validateHookEntry(fmt.Sprintf("hooks.%s[%d].hooks[%d]", event, i, j), hh, r)
					}
				}
			} else {
				r.Add(fmt.Sprintf("hooks.%s[%d]", event, i), "missing required field 'hooks'")
			}
		}
	}
}

// validateHookEntry validates a single hook entry.
func validateHookEntry(path string, v interface{}, r *ValidationResult) {
	entry, ok := v.(map[string]interface{})
	if !ok {
		r.Add(path, "must be an object")
		return
	}
	t, ok := entry["type"].(string)
	if !ok {
		r.Add(path+".type", "must be a string")
		return
	}
	switch t {
	case "command":
		if c, ok := entry["command"].(string); !ok || c == "" {
			r.Add(path+".command", "must be a non-empty string for type=command")
		}
	case "prompt":
		// optional prompt string
		if p, ok := entry["prompt"]; ok {
			if _, ok := p.(string); !ok {
				r.Add(path+".prompt", "must be a string")
			}
		}
	case "agent":
		// agent hooks require a prompt
		if p, ok := entry["prompt"].(string); !ok || p == "" {
			r.Add(path+".prompt", "must be a non-empty string for type=agent")
		}
	default:
		r.Add(path+".type", "must be one of command, prompt, agent")
	}
}

// validateMCPServers validates the mcpServers field.
func validateMCPServers(v interface{}, r *ValidationResult) {
	servers, ok := v.(map[string]interface{})
	if !ok {
		r.Add("mcpServers", "must be an object")
		return
	}
	for name, srv := range servers {
		entry, ok := srv.(map[string]interface{})
		if !ok {
			r.Add("mcpServers."+name, "must be an object")
			continue
		}
		// type: "stdio" | "http" | "sse"
		t, ok := entry["type"].(string)
		if !ok {
			r.Add("mcpServers."+name+".type", "must be a string")
		} else {
			switch t {
			case "stdio":
				if cmd, ok := entry["command"].(string); !ok || cmd == "" {
					r.Add("mcpServers."+name+".command", "must be a non-empty string for type=stdio")
				}
			case "http", "sse":
				if u, ok := entry["url"].(string); !ok || u == "" {
					r.Add("mcpServers."+name+".url", "must be a non-empty URL for type="+t)
				}
			default:
				r.Add("mcpServers."+name+".type", "must be one of stdio, http, sse")
			}
		}
	}
}

// validateEnv validates the env field (string -> string mapping).
func validateEnv(v interface{}, r *ValidationResult) {
	env, ok := v.(map[string]interface{})
	if !ok {
		r.Add("env", "must be an object")
		return
	}
	for k, vv := range env {
		if _, ok := vv.(string); !ok {
			r.Add("env."+k, "must be a string value")
		}
	}
}

// matcherRegex is used to validate hook matcher strings without
// fully compiling them. A basic sanity check catches the most
// common mistakes (unescaped parens, etc.).
var matcherRegex = regexp.MustCompile(`^[^()\[\]]*$`)

// validMatcherRegex returns true if s is plausibly a valid regex
// for a hook matcher. It only does a basic check; full validation
// happens when the hook is actually invoked.
func validMatcherRegex(s string) bool {
	if s == "" {
		// Empty matcher matches everything
		return true
	}
	// Quick sanity: no unescaped parens
	return matcherRegex.MatchString(s)
}
