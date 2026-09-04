// Package env provides environment helpers for graycode's process management.
package env

import (
	"os"
	"strings"
)

// ScrubSet is the canonical list of credential env vars that agent-launched
// subprocesses (the Bash tool, background tasks, seatbelt wrapper) must never
// inherit. It mirrors the provider key names in GraycodeRouter's provider profiles
// (graycode-router/config/profiles.go). Keeping the list explicit here avoids
// importing graycode-router internals and makes the boundary auditable.
var ScrubSet = []string{
	"ANTHROPIC_API_KEY",
	"AWS_ACCESS_KEY_ID",
	"AWS_SECRET_ACCESS_KEY",
	"AZURE_OPENAI_API_KEY",
	"CANOPYWAVE_API_KEY",
	"CLINE_API_KEY",
	"CONCENTRATE_API_KEY",
	"DEEPSEEK_API_KEY",
	"GEMINI_API_KEY",
	"GOOGLE_OAUTH_ACCESS_TOKEN",
	"GROQ_API_KEY",
	"MINIMAX_PAYG_API_KEY",
	"MINIMAX_TOKEN_PLAN_API_KEY",
	"MOONSHOT_API_KEY",
	"OLLAMA_API_KEY",
	"OPENAI_API_KEY",
	"OPENCODEGO_API_KEY",
	"OPENGATEWAY_API_KEY",
	"OPENROUTER_API_KEY",
	"POOLSIDE_API_KEY",
	"STEP_API_KEY",
	"VERTEX_ACCESS_TOKEN",
	"XAI_API_KEY",
	"ZAI_API_KEY",
	"ZAI_CODING_API_KEY",
}

// scrubSet is a lookup set built once from ScrubSet.
var scrubSet = func() map[string]struct{} {
	m := make(map[string]struct{}, len(ScrubSet))
	for _, k := range ScrubSet {
		m[k] = struct{}{}
	}
	return m
}()

// SubprocessEnv returns a copy of os.Environ() with all provider credential
// vars removed, so a subprocess launched by the agent (bash, background
// tasks, seatbelt) cannot read API keys out of its environment. All other
// variables pass through unchanged so existing workflows keep working.
func SubprocessEnv() []string {
	environ := os.Environ()
	out := make([]string, 0, len(environ))
	for _, kv := range environ {
		name, _, ok := strings.Cut(kv, "=")
		if !ok {
			continue
		}
		if _, secret := scrubSet[name]; secret {
			continue
		}
		out = append(out, kv)
	}
	return out
}
