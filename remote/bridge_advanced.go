package remote

import (
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"sync"
	"time"
)

// BoundedUUIDSet implements echo deduplication using a ring buffer.
// Prevents re-delivery of messages during transport swaps.
type BoundedUUIDSet struct {
	mu       sync.RWMutex
	ring     []string
	index    map[string]struct{}
	pos      int
	capacity int
}

// NewBoundedUUIDSet creates a dedup set with the given capacity.
func NewBoundedUUIDSet(capacity int) *BoundedUUIDSet {
	return &BoundedUUIDSet{
		ring:     make([]string, capacity),
		index:    make(map[string]struct{}, capacity),
		capacity: capacity,
	}
}

// Add adds a UUID and returns true if it was new (not a duplicate).
func (s *BoundedUUIDSet) Add(uuid string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()

	if _, exists := s.index[uuid]; exists {
		return false
	}

	// Evict oldest if at capacity
	if old := s.ring[s.pos]; old != "" {
		delete(s.index, old)
	}

	s.ring[s.pos] = uuid
	s.index[uuid] = struct{}{}
	s.pos = (s.pos + 1) % s.capacity
	return true
}

// Contains checks if a UUID is in the set.
func (s *BoundedUUIDSet) Contains(uuid string) bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	_, exists := s.index[uuid]
	return exists
}

// Size returns the number of entries in the set.
func (s *BoundedUUIDSet) Size() int {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return len(s.index)
}

// FlushGate queues writes while history is being flushed, preserving message order.
type FlushGate struct {
	mu      sync.Mutex
	flushing bool
	queue   []BridgeMessage
	sendFn  func(BridgeMessage) error
}

// NewFlushGate creates a flush gate with the given send function.
func NewFlushGate(sendFn func(BridgeMessage) error) *FlushGate {
	return &FlushGate{sendFn: sendFn}
}

// Send sends or queues a message depending on flush state.
func (fg *FlushGate) Send(msg BridgeMessage) error {
	fg.mu.Lock()
	if fg.flushing {
		fg.queue = append(fg.queue, msg)
		fg.mu.Unlock()
		return nil
	}
	fg.mu.Unlock()
	return fg.sendFn(msg)
}

// BeginFlush starts the flush gate (queues subsequent writes).
func (fg *FlushGate) BeginFlush() {
	fg.mu.Lock()
	defer fg.mu.Unlock()
	fg.flushing = true
}

// EndFlush ends the flush gate and sends all queued messages.
func (fg *FlushGate) EndFlush() error {
	fg.mu.Lock()
	fg.flushing = false
	queued := fg.queue
	fg.queue = nil
	fg.mu.Unlock()

	for _, msg := range queued {
		if err := fg.sendFn(msg); err != nil {
			return err
		}
	}
	return nil
}

// QueueSize returns the number of messages queued during flush.
func (fg *FlushGate) QueueSize() int {
	fg.mu.Lock()
	defer fg.mu.Unlock()
	return len(fg.queue)
}

// JWTClaims holds parsed JWT token claims.
type JWTClaims struct {
	Sub       string `json:"sub"`
	Exp       int64  `json:"exp"`
	Iat       int64  `json:"iat"`
	SessionID string `json:"session_id,omitempty"`
	Epoch     int    `json:"epoch,omitempty"`
}

// ParseJWTExpiry extracts the expiry time from a JWT without full verification.
// Used for proactive refresh scheduling.
func ParseJWTExpiry(token string) (time.Time, error) {
	parts := splitJWT(token)
	if len(parts) != 3 {
		return time.Time{}, fmt.Errorf("invalid JWT format")
	}

	payload, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		return time.Time{}, fmt.Errorf("decoding JWT payload: %w", err)
	}

	var claims JWTClaims
	if err := json.Unmarshal(payload, &claims); err != nil {
		return time.Time{}, fmt.Errorf("parsing JWT claims: %w", err)
	}

	if claims.Exp == 0 {
		return time.Time{}, fmt.Errorf("no exp claim in JWT")
	}
	return time.Unix(claims.Exp, 0), nil
}

// JWTRefreshScheduler proactively refreshes a JWT before it expires.
type JWTRefreshScheduler struct {
	mu          sync.Mutex
	token       string
	expiry      time.Time
	refreshFn   func() (string, error)
	timer       *time.Timer
	bufferMins  int
}

// NewJWTRefreshScheduler creates a scheduler that refreshes tokens ahead of expiry.
func NewJWTRefreshScheduler(bufferMins int, refreshFn func() (string, error)) *JWTRefreshScheduler {
	return &JWTRefreshScheduler{
		bufferMins: bufferMins,
		refreshFn:  refreshFn,
	}
}

// SetToken updates the current token and schedules the next refresh.
func (s *JWTRefreshScheduler) SetToken(token string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.token = token
	expiry, err := ParseJWTExpiry(token)
	if err != nil {
		return err
	}
	s.expiry = expiry

	// Schedule refresh bufferMins before expiry
	refreshAt := expiry.Add(-time.Duration(s.bufferMins) * time.Minute)
	delay := time.Until(refreshAt)
	if delay <= 0 {
		delay = 1 * time.Second
	}

	if s.timer != nil {
		s.timer.Stop()
	}
	s.timer = time.AfterFunc(delay, s.doRefresh)
	return nil
}

// Token returns the current token.
func (s *JWTRefreshScheduler) Token() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.token
}

// Stop stops the refresh scheduler.
func (s *JWTRefreshScheduler) Stop() {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.timer != nil {
		s.timer.Stop()
	}
}

func (s *JWTRefreshScheduler) doRefresh() {
	newToken, err := s.refreshFn()
	if err != nil {
		return
	}
	s.SetToken(newToken)
}

func splitJWT(token string) []string {
	var parts []string
	start := 0
	for i := 0; i < len(token); i++ {
		if token[i] == '.' {
			parts = append(parts, token[start:i])
			start = i + 1
		}
	}
	parts = append(parts, token[start:])
	return parts
}

// GenerateMessageID creates a unique message ID for deduplication.
func GenerateMessageID() string {
	b := make([]byte, 16)
	rand.Read(b)
	return base64.RawURLEncoding.EncodeToString(b)
}

// BridgePointer persists the active session for crash recovery (--continue).
type BridgePointer struct {
	SessionID   string    `json:"session_id"`
	CreatedAt   time.Time `json:"created_at"`
	TranscriptPath string `json:"transcript_path,omitempty"`
	TTL         time.Duration `json:"ttl"`
}

// IsExpired returns true if the bridge pointer has exceeded its TTL.
func (bp *BridgePointer) IsExpired() bool {
	return time.Since(bp.CreatedAt) > bp.TTL
}

// DefaultBridgePointerTTL is 4 hours matching the archive.
const DefaultBridgePointerTTL = 4 * time.Hour
