package auth

import "testing"

func TestTokenStore(t *testing.T) {
	store := NewTokenStore()

	store.Set("anthropic", "sk-test-123")
	if !store.Has("anthropic") {
		t.Fatal("expected token to exist")
	}
	if store.Get("anthropic") != "sk-test-123" {
		t.Fatal("token mismatch")
	}
	if store.Has("openai") {
		t.Fatal("expected no token for openai")
	}
}

func TestGenerateNonce(t *testing.T) {
	nonce1 := GenerateNonce()
	nonce2 := GenerateNonce()
	if nonce1 == nonce2 {
		t.Fatal("nonces should be unique")
	}
	if len(nonce1) == 0 {
		t.Fatal("nonce should not be empty")
	}
}
