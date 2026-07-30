package hawkerr

import (
	"errors"
	"testing"
)

// --- ClassifyError tests ---

func TestClassifyError_NilError(t *testing.T) {
	result := ClassifyError(nil)
	if result.ExitCode != ExitOK {
		t.Errorf("ExitCode = %d, want %d", result.ExitCode, ExitOK)
	}
	if result.Message != "" {
		t.Errorf("Message = %q, want empty", result.Message)
	}
}

func TestClassifyError_AnthropicAPIKey(t *testing.T) {
	err := errors.New("anthropic_api_key is missing")
	result := ClassifyError(err)
	if result.ExitCode != ExitAuth {
		t.Errorf("ExitCode = %d, want %d", result.ExitCode, ExitAuth)
	}
	if result.Message == "" {
		t.Error("expected non-empty message")
	}
}

func TestClassifyError_OpenAIAPIKey(t *testing.T) {
	err := errors.New("openai api key not found")
	result := ClassifyError(err)
	if result.ExitCode != ExitAuth {
		t.Errorf("ExitCode = %d, want %d", result.ExitCode, ExitAuth)
	}
}

func TestClassifyError_SSHConnection(t *testing.T) {
	err := errors.New("ssh connection refused")
	result := ClassifyError(err)
	if result.ExitCode != ExitNetwork {
		t.Errorf("ExitCode = %d, want %d", result.ExitCode, ExitNetwork)
	}
}

func TestClassifyError_MCPNotResponding(t *testing.T) {
	err := errors.New("mcp server not responding")
	result := ClassifyError(err)
	if result.ExitCode != ExitToolFailure {
		t.Errorf("ExitCode = %d, want %d", result.ExitCode, ExitToolFailure)
	}
}

func TestClassifyError_ToolTimeout(t *testing.T) {
	err := errors.New("tool timed out")
	result := ClassifyError(err)
	if result.ExitCode != ExitToolFailure {
		t.Errorf("ExitCode = %d, want %d", result.ExitCode, ExitToolFailure)
	}
}

func TestClassifyError_ReasoningOnly(t *testing.T) {
	err := errors.New("error_only_reasoning")
	result := ClassifyError(err)
	if result.ExitCode != ExitGeneral {
		t.Errorf("ExitCode = %d, want %d", result.ExitCode, ExitGeneral)
	}
}

func TestClassifyError_RateLimit(t *testing.T) {
	err := errors.New("429 rate limit exceeded")
	result := ClassifyError(err)
	if result.ExitCode != ExitRateLimit {
		t.Errorf("ExitCode = %d, want %d", result.ExitCode, ExitRateLimit)
	}
}

func TestClassifyError_RetryAfter(t *testing.T) {
	err := errors.New("429 retry-after: 30")
	result := ClassifyError(err)
	if result.ExitCode != ExitRateLimit {
		t.Errorf("ExitCode = %d, want %d", result.ExitCode, ExitRateLimit)
	}
	if result.Message == "" {
		t.Error("expected non-empty message")
	}
}

func TestClassifyError_InsufficientCredits(t *testing.T) {
	err := errors.New("insufficient credits")
	result := ClassifyError(err)
	if result.ExitCode != ExitRateLimit {
		t.Errorf("ExitCode = %d, want %d", result.ExitCode, ExitRateLimit)
	}
}

// Agnes AI returns HTTP 403 with an insufficient_user_quota code (and a
// Chinese "预扣费" pre-deduction message) when the account balance cannot
// cover the maximum-token pre-authorization hold. That must surface as a
// quota problem, NOT as "check your API key".
func TestClassifyError_AgnesPreDeductionQuota(t *testing.T) {
	err := errors.New("eyrie: agnes chat failed (HTTP 403) [request_id=202607300111184425982109LnnftLG]: " +
		"billing/quota problem — check the provider account's balance and limits — " +
		"AgnesAI_error: 预扣费额度失败, 用户剩余额度: $0.000740, 需要预扣费额度: $0.002068")
	result := ClassifyError(err)
	if result.ExitCode != ExitRateLimit {
		t.Errorf("ExitCode = %d, want %d (ExitRateLimit)", result.ExitCode, ExitRateLimit)
	}
	if result.Message == "" {
		t.Error("expected non-empty message")
	}
	// Must not mislead the user into checking their API key.
	if contains(result.Message, "API key") || contains(result.Message, "Access denied") {
		t.Errorf("message should not blame the API key, got %q", result.Message)
	}
}

// The eyrie layer tags quota holds as "billing/quota problem"; a bare hint
// (without the full Agnes body) must still be classified as a quota problem.
func TestClassifyError_QuotaHintOnly(t *testing.T) {
	err := errors.New("billing/quota problem — check the provider account's balance and limits")
	result := ClassifyError(err)
	if result.ExitCode != ExitRateLimit {
		t.Errorf("ExitCode = %d, want %d (ExitRateLimit)", result.ExitCode, ExitRateLimit)
	}
}

// A generic 403 with no quota signal must still fall through to the auth
// branch — the new quota branch must not swallow real access denials.
func TestClassifyError_403Forbidden_StillAuth(t *testing.T) {
	err := errors.New("403 forbidden")
	result := ClassifyError(err)
	if result.ExitCode != ExitAuth {
		t.Errorf("ExitCode = %d, want %d (ExitAuth)", result.ExitCode, ExitAuth)
	}
}

func TestClassifyError_401Unauthorized(t *testing.T) {
	err := errors.New("401 unauthorized")
	result := ClassifyError(err)
	if result.ExitCode != ExitAuth {
		t.Errorf("ExitCode = %d, want %d", result.ExitCode, ExitAuth)
	}
}

func TestClassifyError_403Forbidden(t *testing.T) {
	err := errors.New("403 forbidden")
	result := ClassifyError(err)
	if result.ExitCode != ExitAuth {
		t.Errorf("ExitCode = %d, want %d", result.ExitCode, ExitAuth)
	}
}

func TestClassifyError_ContextTooLong(t *testing.T) {
	err := errors.New("context length exceeded")
	result := ClassifyError(err)
	if result.ExitCode != ExitContextLimit {
		t.Errorf("ExitCode = %d, want %d", result.ExitCode, ExitContextLimit)
	}
}

func TestClassifyError_ModelNotFound(t *testing.T) {
	err := errors.New("model not found")
	result := ClassifyError(err)
	if result.ExitCode != ExitNotFound {
		t.Errorf("ExitCode = %d, want %d", result.ExitCode, ExitNotFound)
	}
}

func TestClassifyError_NetworkUnreachable(t *testing.T) {
	err := errors.New("network is unreachable")
	result := ClassifyError(err)
	if result.ExitCode != ExitNetwork {
		t.Errorf("ExitCode = %d, want %d", result.ExitCode, ExitNetwork)
	}
}

func TestClassifyError_ConnectionRefused(t *testing.T) {
	err := errors.New("connection refused")
	result := ClassifyError(err)
	if result.ExitCode != ExitNetwork {
		t.Errorf("ExitCode = %d, want %d", result.ExitCode, ExitNetwork)
	}
}

func TestClassifyError_DNSFailure(t *testing.T) {
	err := errors.New("no such host")
	result := ClassifyError(err)
	if result.ExitCode != ExitNetwork {
		t.Errorf("ExitCode = %d, want %d", result.ExitCode, ExitNetwork)
	}
}

func TestClassifyError_ConnectionReset(t *testing.T) {
	err := errors.New("connection reset")
	result := ClassifyError(err)
	if result.ExitCode != ExitNetwork {
		t.Errorf("ExitCode = %d, want %d", result.ExitCode, ExitNetwork)
	}
}

func TestClassifyError_404NotFound(t *testing.T) {
	err := errors.New("404 not found")
	result := ClassifyError(err)
	if result.ExitCode != ExitNotFound {
		t.Errorf("ExitCode = %d, want %d", result.ExitCode, ExitNotFound)
	}
}

func TestClassifyError_500ServerError(t *testing.T) {
	err := errors.New("500 internal server error")
	result := ClassifyError(err)
	if result.ExitCode != ExitNetwork {
		t.Errorf("ExitCode = %d, want %d", result.ExitCode, ExitNetwork)
	}
}

func TestClassifyError_502BadGateway(t *testing.T) {
	err := errors.New("502 bad gateway")
	result := ClassifyError(err)
	if result.ExitCode != ExitNetwork {
		t.Errorf("ExitCode = %d, want %d", result.ExitCode, ExitNetwork)
	}
}

func TestClassifyError_503ServiceUnavailable(t *testing.T) {
	err := errors.New("503 service unavailable")
	result := ClassifyError(err)
	if result.ExitCode != ExitNetwork {
		t.Errorf("ExitCode = %d, want %d", result.ExitCode, ExitNetwork)
	}
}

func TestClassifyError_504GatewayTimeout(t *testing.T) {
	err := errors.New("504 gateway timeout")
	result := ClassifyError(err)
	if result.ExitCode != ExitNetwork {
		t.Errorf("ExitCode = %d, want %d", result.ExitCode, ExitNetwork)
	}
}

func TestClassifyError_Timeout(t *testing.T) {
	err := errors.New("request timed out")
	result := ClassifyError(err)
	if result.ExitCode != ExitTimeout {
		t.Errorf("ExitCode = %d, want %d", result.ExitCode, ExitTimeout)
	}
}

func TestClassifyError_PermissionDenied(t *testing.T) {
	err := errors.New("permission denied")
	result := ClassifyError(err)
	if result.ExitCode != ExitPolicyBlock {
		t.Errorf("ExitCode = %d, want %d", result.ExitCode, ExitPolicyBlock)
	}
}

func TestClassifyError_DiskFull(t *testing.T) {
	err := errors.New("no space left on device")
	result := ClassifyError(err)
	if result.ExitCode != ExitDiskFull {
		t.Errorf("ExitCode = %d, want %d", result.ExitCode, ExitDiskFull)
	}
}

func TestClassifyError_InvalidJSON(t *testing.T) {
	err := errors.New("invalid character in json config")
	result := ClassifyError(err)
	if result.ExitCode != ExitConfig {
		t.Errorf("ExitCode = %d, want %d", result.ExitCode, ExitConfig)
	}
}

func TestClassifyError_TLSCertificate(t *testing.T) {
	err := errors.New("certificate signed by unknown authority")
	result := ClassifyError(err)
	if result.ExitCode != ExitNetwork {
		t.Errorf("ExitCode = %d, want %d", result.ExitCode, ExitNetwork)
	}
}

func TestClassifyError_Fallback(t *testing.T) {
	err := errors.New("some unknown error")
	result := ClassifyError(err)
	if result.ExitCode != ExitGeneral {
		t.Errorf("ExitCode = %d, want %d", result.ExitCode, ExitGeneral)
	}
	if result.Message != "some unknown error" {
		t.Errorf("Message = %q, want %q", result.Message, "some unknown error")
	}
}

// --- ClassifyErrorMessage tests ---

func TestClassifyErrorMessage_NilError(t *testing.T) {
	msg := ClassifyErrorMessage(nil)
	if msg != "" {
		t.Errorf("ClassifyErrorMessage(nil) = %q, want empty", msg)
	}
}

func TestClassifyErrorMessage_AuthError(t *testing.T) {
	msg := ClassifyErrorMessage(errors.New("401 unauthorized"))
	if msg == "" {
		t.Error("expected non-empty message")
	}
}

func TestClassifyErrorMessage_Fallback(t *testing.T) {
	msg := ClassifyErrorMessage(errors.New("unknown error"))
	if msg != "unknown error" {
		t.Errorf("ClassifyErrorMessage(unknown error) = %q, want %q", msg, "unknown error")
	}
}

// --- BridgeError tests ---

func TestBridgeError_Error_WithErr(t *testing.T) {
	inner := errors.New("inner error")
	be := NewBridgeError("yaad", "Remember", "failed to connect", inner)
	result := be.Error()
	if !contains(result, "yaad bridge") {
		t.Errorf("Error() should contain 'yaad bridge', got %q", result)
	}
	if !contains(result, "Remember") {
		t.Errorf("Error() should contain 'Remember', got %q", result)
	}
	if !contains(result, "failed to connect") {
		t.Errorf("Error() should contain 'failed to connect', got %q", result)
	}
	if !contains(result, "inner error") {
		t.Errorf("Error() should contain 'inner error', got %q", result)
	}
}

func TestBridgeError_Error_WithoutErr(t *testing.T) {
	be := NewBridgeError("trace", "Recall", "no data", nil)
	result := be.Error()
	if !contains(result, "trace bridge") {
		t.Errorf("Error() should contain 'trace bridge', got %q", result)
	}
	if !contains(result, "no data") {
		t.Errorf("Error() should contain 'no data', got %q", result)
	}
	// Should not contain ": <nil>" or similar
	if contains(result, "<nil>") {
		t.Errorf("Error() should not contain '<nil>', got %q", result)
	}
}

func TestBridgeError_Unwrap(t *testing.T) {
	inner := errors.New("inner error")
	be := NewBridgeError("sight", "Enable", "failed", inner)
	if be.Unwrap() != inner {
		t.Error("Unwrap() should return the inner error")
	}
}

func TestBridgeError_Unwrap_Nil(t *testing.T) {
	be := NewBridgeError("inspect", "Query", "failed", nil)
	if be.Unwrap() != nil {
		t.Error("Unwrap() should return nil for no inner error")
	}
}

// --- Helper ---

func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || indexOf(s, substr) >= 0)
}

func indexOf(s, substr string) int {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return i
		}
	}
	return -1
}
