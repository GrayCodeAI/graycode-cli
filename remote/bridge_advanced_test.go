package remote

import (
	"encoding/base64"
	"encoding/json"
	"testing"
	"time"
)

func TestBoundedUUIDSet_Add(t *testing.T) {
	s := NewBoundedUUIDSet(3)

	if !s.Add("a") {
		t.Error("first add should return true")
	}
	if s.Add("a") {
		t.Error("duplicate add should return false")
	}
	if s.Size() != 1 {
		t.Errorf("expected size 1, got %d", s.Size())
	}
}

func TestBoundedUUIDSet_Eviction(t *testing.T) {
	s := NewBoundedUUIDSet(3)

	s.Add("a")
	s.Add("b")
	s.Add("c")
	// Adding fourth should evict "a"
	s.Add("d")

	if s.Contains("a") {
		t.Error("a should have been evicted")
	}
	if !s.Contains("d") {
		t.Error("d should be present")
	}
	if s.Size() != 3 {
		t.Errorf("expected size 3, got %d", s.Size())
	}
}

func TestFlushGate(t *testing.T) {
	var sent []BridgeMessage
	fg := NewFlushGate(func(msg BridgeMessage) error {
		sent = append(sent, msg)
		return nil
	})

	// Normal send
	fg.Send(BridgeMessage{Type: "msg1"})
	if len(sent) != 1 {
		t.Errorf("expected 1 sent, got %d", len(sent))
	}

	// Begin flush - messages queued
	fg.BeginFlush()
	fg.Send(BridgeMessage{Type: "msg2"})
	fg.Send(BridgeMessage{Type: "msg3"})
	if len(sent) != 1 {
		t.Error("messages should be queued during flush")
	}
	if fg.QueueSize() != 2 {
		t.Errorf("expected queue size 2, got %d", fg.QueueSize())
	}

	// End flush - queued messages sent
	fg.EndFlush()
	if len(sent) != 3 {
		t.Errorf("expected 3 sent after flush, got %d", len(sent))
	}
	if fg.QueueSize() != 0 {
		t.Error("queue should be empty after flush")
	}
}

func TestParseJWTExpiry(t *testing.T) {
	// Create a fake JWT with exp claim
	claims := map[string]interface{}{
		"sub": "user123",
		"exp": time.Now().Add(1 * time.Hour).Unix(),
		"iat": time.Now().Unix(),
	}
	payload, _ := json.Marshal(claims)
	header := base64.RawURLEncoding.EncodeToString([]byte(`{"alg":"HS256"}`))
	body := base64.RawURLEncoding.EncodeToString(payload)
	sig := base64.RawURLEncoding.EncodeToString([]byte("fakesig"))
	token := header + "." + body + "." + sig

	expiry, err := ParseJWTExpiry(token)
	if err != nil {
		t.Fatalf("ParseJWTExpiry error: %v", err)
	}
	if expiry.IsZero() {
		t.Error("expiry should not be zero")
	}
	if time.Until(expiry) < 50*time.Minute {
		t.Error("expiry should be about 1 hour from now")
	}
}

func TestParseJWTExpiry_Invalid(t *testing.T) {
	_, err := ParseJWTExpiry("not.a.jwt.token")
	if err == nil {
		t.Error("expected error for invalid JWT")
	}

	_, err = ParseJWTExpiry("invalid")
	if err == nil {
		t.Error("expected error for malformed JWT")
	}
}

func TestJWTRefreshScheduler(t *testing.T) {
	refreshCalled := false
	scheduler := NewJWTRefreshScheduler(5, func() (string, error) {
		refreshCalled = true
		return "new-token", nil
	})
	defer scheduler.Stop()

	// Create a token expiring in 6 minutes (should trigger refresh at 1 min)
	claims := map[string]interface{}{
		"exp": time.Now().Add(6 * time.Minute).Unix(),
	}
	payload, _ := json.Marshal(claims)
	header := base64.RawURLEncoding.EncodeToString([]byte(`{"alg":"HS256"}`))
	body := base64.RawURLEncoding.EncodeToString(payload)
	sig := base64.RawURLEncoding.EncodeToString([]byte("sig"))
	token := header + "." + body + "." + sig

	err := scheduler.SetToken(token)
	if err != nil {
		t.Fatalf("SetToken error: %v", err)
	}

	if scheduler.Token() != token {
		t.Error("Token() should return the set token")
	}
	_ = refreshCalled
}

func TestBridgePointer_IsExpired(t *testing.T) {
	bp := &BridgePointer{
		SessionID: "s1",
		CreatedAt: time.Now().Add(-5 * time.Hour),
		TTL:       DefaultBridgePointerTTL,
	}
	if !bp.IsExpired() {
		t.Error("5-hour old pointer with 4h TTL should be expired")
	}

	bp.CreatedAt = time.Now().Add(-1 * time.Hour)
	if bp.IsExpired() {
		t.Error("1-hour old pointer with 4h TTL should not be expired")
	}
}

func TestGenerateMessageID(t *testing.T) {
	id1 := GenerateMessageID()
	id2 := GenerateMessageID()
	if id1 == "" {
		t.Error("message ID should not be empty")
	}
	if id1 == id2 {
		t.Error("two message IDs should differ")
	}
}
