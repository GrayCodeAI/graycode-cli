package hawkerr

import (
	"fmt"
	"regexp"
	"strings"
)

// errorClass combines a stable exit code with a user-friendly message for a
// given error. It is the single source of truth for error classification —
// both ClassifyExitCode() and ClassifyErrorMessage() delegate here so they
// never disagree about what kind of failure occurred.
type errorClass struct {
	exitCode int
	message  string
}

// ClassifiedError is the public result of error classification, exposing both
// the exit code and the human-readable message.
type ClassifiedError struct {
	ExitCode int
	Message  string
}

var reRetryAfter = regexp.MustCompile(`(?i)retry[- ]?after[:\s]+(\d+)`)

// classify maps a raw error to a stable exit code + friendly message.
// The pattern matching order is deliberate — most specific / most actionable
// classes first. This function is the single source of truth used by both
// ClassifyExitCode() (script exit code) and ClassifyErrorMessage() (human
// output).
//
// A nil error yields (ExitOK, "").
func classify(err error) errorClass {
	if err == nil {
		return errorClass{exitCode: ExitOK, message: ""}
	}
	msg := err.Error()
	low := strings.ToLower(msg)

	// ── Provider-specific API key errors ──────────────────────────────────
	providerKeys := []struct {
		patterns []string
		provider string
	}{
		{[]string{"anthropic_api_key", "anthropic api key", "x-api-key"}, "Anthropic"},
		{[]string{"openai_api_key", "openai api key", "openai key"}, "OpenAI"},
		{[]string{"gemini_api_key", "google_api_key", "gemini api key"}, "Gemini"},
		{[]string{"openrouter_api_key", "openrouter api key"}, "OpenRouter"},
		{[]string{"canopywave_api_key", "canopywave api key"}, "CanopyWave"},
		{[]string{"zai_payg_api_key", "zai_api_key"}, "Z.AI"},
		{[]string{"zai_coding_api_key", "zai_coding_api_key"}, "Z.AI Coding Plan"},
		{[]string{"xai_api_key", "xai api key"}, "xAI (Grok)"},
		{[]string{"opencodego_api_key", "opencodego api key"}, "OpenCodeGo"},
		{[]string{"moonshot_api_key", "moonshot api key"}, "Kimi (Moonshot)"},
		{[]string{"xiaomi_mimo_payg_api_key", "xiaomi mimo payg"}, "Xiaomi (MiMo) Pay-as-you-go"},
		{[]string{"xiaomi_mimo_token_plan_api_key", "xiaomi mimo token plan"}, "Xiaomi (MiMo) Token Plan"},
		{[]string{"xiaomi_mimo_api_key", "xiaomi mimo api key"}, "Xiaomi (MiMo)"},
	}
	for _, pk := range providerKeys {
		for _, pat := range pk.patterns {
			if strings.Contains(low, pat) {
				return errorClass{
					exitCode: ExitAuth,
					message:  fmt.Sprintf("%s API key is missing or invalid. Run /config to save it in your OS credential store.", pk.provider),
				}
			}
		}
	}

	// ── SSH connection failures (check early, before generic network/auth) ──
	if strings.Contains(low, "ssh") && (strings.Contains(low, "connection") || strings.Contains(low, "refused") ||
		strings.Contains(low, "timeout") || strings.Contains(low, "auth") || strings.Contains(low, "handshake") ||
		strings.Contains(low, "key exchange")) {
		return errorClass{
			exitCode: ExitNetwork,
			message:  "SSH connection failed. Check your SSH configuration, keys, and that the remote host is reachable.\n  Try: ssh -vv <host> to diagnose.",
		}
	}

	// ── MCP server not responding (check early, before generic timeouts) ──
	if strings.Contains(low, "mcp") && (strings.Contains(low, "not responding") || strings.Contains(low, "connection") ||
		strings.Contains(low, "failed") || strings.Contains(low, "timeout") || strings.Contains(low, "refused")) {
		return errorClass{
			exitCode: ExitToolFailure,
			message:  "MCP server is not responding. Check that the server is running and accessible.\n  Use /mcp to see configured servers, or /doctor for diagnostics.",
		}
	}

	// ── Tool timeout (check early, before generic timeouts) ───────────────
	if strings.Contains(low, "tool timeout") || strings.Contains(low, "tool_timeout") ||
		(strings.Contains(low, "tool") && strings.Contains(low, "timed out")) {
		return errorClass{
			exitCode: ExitToolFailure,
			message:  "A tool execution timed out. The command may be taking too long.\n  Try breaking the task into smaller steps.",
		}
	}

	// ── Reasoning-only response (thinking tokens but no answer) ───────────
	if strings.Contains(low, "error_only_reasoning") ||
		strings.Contains(low, "reasoning tokens but no answer") ||
		strings.Contains(low, "reasoning but no answer") {
		return errorClass{
			exitCode: ExitGeneral,
			message: "The model produced internal reasoning but no reply.\n" +
				"  This often happens when thinking/reasoning consumes the whole token budget (LongCat defaults thinking on) or when OpenCode Go / MiniMax drops the answer after thinking.\n" +
				"  Try /model → toggle Think with t, switch model, or pick a non-reasoning model.",
		}
	}

	// ── Rate limiting (429) ───────────────────────────────────────────────
	if strings.Contains(low, "429") || strings.Contains(low, "rate limit") || strings.Contains(low, "rate_limit") || strings.Contains(low, "too many requests") {
		base := "Rate limited by the API provider."
		if match := reRetryAfter.FindStringSubmatch(msg); len(match) > 1 {
			base += fmt.Sprintf(" Retry after %s seconds.", match[1])
		}
		base += " Wait a moment and try again, or switch providers with /config."
		return errorClass{exitCode: ExitRateLimit, message: base}
	}

	// ── Provider billing / credits (OpenRouter free tier, etc.) ───────────
	if strings.Contains(low, "requires more credits") || strings.Contains(low, "can only afford") ||
		strings.Contains(low, "insufficient credits") || strings.Contains(low, "insufficient balance") ||
		strings.Contains(low, "payment required") || strings.Contains(low, "out of credits") {
		return errorClass{
			exitCode: ExitRateLimit,
			message:  "Insufficient provider credits for this request.\n  Add credits at your provider dashboard, switch to a cheaper model with /model, or try again with a shorter prompt.",
		}
	}

	// ── Provider quota / pre-deduction hold failures ──────────────────────
	// Some providers (e.g. Agnes AI) pre-authorize the *maximum* possible token
	// cost before fulfilling a request. When the account balance can't cover
	// that hold they return 403 with an insufficient_user_quota code rather
	// than a 401 — so this must be checked before the generic 403 branch below,
	// otherwise the user is misled into "check your API key". eyrie already
	// tags these as "billing/quota problem"; the Agnes body also carries the
	// Chinese pre-deduction phrasing (预扣费) and an insufficient_user_quota code.
	if strings.Contains(low, "insufficient_user_quota") || strings.Contains(low, "insufficient_quota") ||
		strings.Contains(low, "billing/quota problem") || strings.Contains(low, "预扣费") ||
		strings.Contains(low, "pre-deduct") || strings.Contains(low, "pre-deduction") {
		return errorClass{
			exitCode: ExitRateLimit,
			message:  "Request blocked by the provider's pre-deduction check: your account balance is too low to cover the maximum token cost of this request.\n  Top up your provider account, switch to a free/cheaper model with /model (e.g. agnes-2.5-flash), or try a shorter prompt.",
		}
	}

	// ── Authentication / authorization ────────────────────────────────────
	if strings.Contains(low, "401") || strings.Contains(low, "unauthorized") || strings.Contains(low, "invalid api key") || strings.Contains(low, "invalid_api_key") || strings.Contains(low, "authentication") {
		return errorClass{
			exitCode: ExitAuth,
			message:  "Authentication failed. Your API key may be invalid or expired.\n  Check with /env, or update it with /config.",
		}
	}
	if strings.Contains(low, "403") || strings.Contains(low, "forbidden") || strings.Contains(low, "access denied") {
		return errorClass{
			exitCode: ExitAuth,
			message:  "Access denied by the API provider. Verify your API key has the required permissions.",
		}
	}

	// ── Context too long / token limit ────────────────────────────────────
	if strings.Contains(low, "context length") || strings.Contains(low, "context_length") ||
		strings.Contains(low, "token limit") || strings.Contains(low, "too many tokens") ||
		strings.Contains(low, "maximum context") ||
		strings.Contains(low, "max_tokens exceeded") || strings.Contains(low, "max tokens exceeded") ||
		strings.Contains(low, "context window") || strings.Contains(low, "prompt is too long") {
		return errorClass{
			exitCode: ExitContextLimit,
			message:  "The conversation exceeds the model's context window.\n  Use /compact to summarize and free up space, or start a new session.",
		}
	}

	// ── Invalid model name ────────────────────────────────────────────────
	if strings.Contains(low, "model not found") || strings.Contains(low, "model_not_found") ||
		strings.Contains(low, "unknown model") || strings.Contains(low, "invalid model") ||
		strings.Contains(low, "does not exist") || (strings.Contains(low, "404") && strings.Contains(low, "model")) {
		return errorClass{
			exitCode: ExitNotFound,
			message:  "Model not found. Check your model name with /model.\n  Use /models to list all models, or /config to change provider.",
		}
	}

	// ── Network unreachable / connection refused / DNS ─────────────────────
	if strings.Contains(low, "network is unreachable") || strings.Contains(low, "network unreachable") {
		return errorClass{exitCode: ExitNetwork, message: "Network is unreachable. Check that you have an active internet connection."}
	}
	if strings.Contains(low, "connection refused") {
		return errorClass{
			exitCode: ExitNetwork,
			message:  "Connection refused. The API endpoint may be down, or a local proxy/firewall is blocking the connection.\n  If using Ollama, make sure it is running (ollama serve).",
		}
	}
	if strings.Contains(low, "no such host") || strings.Contains(low, "dns") ||
		strings.Contains(low, "lookup") && strings.Contains(low, "no such host") {
		return errorClass{
			exitCode: ExitNetwork,
			message:  "DNS resolution failed. Check your internet connection and DNS settings.",
		}
	}
	if strings.Contains(low, "connection reset") || strings.Contains(low, "broken pipe") ||
		strings.Contains(low, "eof") && (strings.Contains(low, "unexpected") || strings.Contains(low, "connection")) {
		return errorClass{exitCode: ExitNetwork, message: "Connection was reset by the server. This may be a transient issue -- try again."}
	}

	// ── HTTP status codes (generic) ───────────────────────────────────────
	if strings.Contains(low, "404") || strings.Contains(low, "not found") {
		return errorClass{
			exitCode: ExitNotFound,
			message:  "Endpoint or resource not found. Check your model with /model or provider with /config.",
		}
	}
	if strings.Contains(low, "500") || strings.Contains(low, "internal server error") {
		return errorClass{exitCode: ExitNetwork, message: "The API provider returned a server error (500). Try again shortly."}
	}
	if strings.Contains(low, "502") || strings.Contains(low, "bad gateway") {
		return errorClass{exitCode: ExitNetwork, message: "The API provider is temporarily unavailable (502). Try again shortly."}
	}
	if strings.Contains(low, "503") || strings.Contains(low, "service unavailable") {
		return errorClass{exitCode: ExitNetwork, message: "The API provider is temporarily unavailable (503). Try again shortly."}
	}
	if strings.Contains(low, "504") || strings.Contains(low, "gateway timeout") {
		return errorClass{
			exitCode: ExitNetwork,
			message:  "The API provider timed out (504). The request may have been too large -- try /compact.",
		}
	}

	// ── Tool / operation timeouts ─────────────────────────────────────────
	if strings.Contains(low, "timeout") || strings.Contains(low, "timed out") ||
		strings.Contains(low, "deadline exceeded") || strings.Contains(low, "context canceled") {
		return errorClass{
			exitCode: ExitTimeout,
			message:  "Request timed out. Check your connection and try again, or use /compact to reduce context size.",
		}
	}

	// ── Policy / permission / guardrail denial ────────────────────────────
	if strings.Contains(low, "permission denied") || strings.Contains(low, "guardrail") ||
		strings.Contains(low, "policy") || strings.Contains(low, "blocked by") ||
		strings.Contains(low, "approval denied") || strings.Contains(low, "not permitted") ||
		strings.Contains(low, "operation not allowed") {
		return errorClass{
			exitCode: ExitPolicyBlock,
			message:  "Permission denied. Check file/directory permissions.\n  You may need to adjust permissions or run from a writable directory.",
		}
	}

	// ── Disk full ─────────────────────────────────────────────────────────
	if strings.Contains(low, "no space left") || strings.Contains(low, "disk full") ||
		strings.Contains(low, "not enough space") || strings.Contains(low, "disk quota") {
		return errorClass{
			exitCode: ExitDiskFull,
			message:  "Disk is full or quota exceeded. Free up space and try again.\n  Check Hawk's user state sessions directory for old sessions you can remove.",
		}
	}

	// ── Invalid JSON in config/settings ───────────────────────────────────
	if (strings.Contains(low, "json") || strings.Contains(low, "unmarshal") || strings.Contains(low, "syntax error") || strings.Contains(low, "invalid character")) &&
		(strings.Contains(low, "settings") || strings.Contains(low, "config") || strings.Contains(low, "parse")) {
		return errorClass{
			exitCode: ExitConfig,
			message:  "Invalid JSON in configuration. Check your Hawk settings.json files for syntax errors.\n  Tip: use a JSON linter to find the issue.",
		}
	}

	// ── TLS / certificate errors ──────────────────────────────────────────
	if strings.Contains(low, "certificate") || strings.Contains(low, "tls") || strings.Contains(low, "x509") {
		return errorClass{
			exitCode: ExitNetwork,
			message:  "TLS/certificate error. This may be caused by a corporate proxy, expired certificate, or network issue.\n  If behind a proxy, you may need to configure custom CA certificates.",
		}
	}

	// ── Fallback ──────────────────────────────────────────────────────────
	return errorClass{exitCode: ExitGeneral, message: msg}
}

// ClassifyError maps a raw error to a stable exit code + friendly message.
// This is the single entry point for error classification.
func ClassifyError(err error) ClassifiedError {
	c := classify(err)
	return ClassifiedError{ExitCode: c.exitCode, Message: c.message}
}

// ClassifyErrorMessage returns the user-friendly message for an error.
// Delegates to the shared classify() so it never disagrees with
// ClassifyExitCode.
func ClassifyErrorMessage(err error) string {
	return classify(err).message
}
