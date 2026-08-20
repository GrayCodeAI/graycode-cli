package engine

import (
	"crypto/rand"
	"encoding/hex"
	"sync"
	"time"
)

// AnnouncementKind denotes the category/urgency of an in-session announcement.
type AnnouncementKind string

const (
	AnnouncementInfo     AnnouncementKind = "info"
	AnnouncementWarning  AnnouncementKind = "warning"
	AnnouncementSystem   AnnouncementKind = "system"
	AnnouncementSchedule AnnouncementKind = "schedule"
)

// Announcement represents a single broadcast notice within an active session.
type Announcement struct {
	ID        string           `json:"id"`
	Kind      AnnouncementKind `json:"kind"`
	Message   string           `json:"message"`
	CreatedAt time.Time        `json:"created_at"`
	ExpiresAt time.Time        `json:"expires_at,omitempty"`
	Read      bool             `json:"read"`
}

// AnnouncementFeed provides a thread-safe registry of active session announcements.
type AnnouncementFeed struct {
	mu            sync.RWMutex
	announcements []*Announcement
}

// NewAnnouncementFeed creates an empty AnnouncementFeed.
func NewAnnouncementFeed() *AnnouncementFeed {
	return &AnnouncementFeed{
		announcements: make([]*Announcement, 0),
	}
}

// Post broadcasts a new announcement with an optional TTL (0 = never expires).
func (af *AnnouncementFeed) Post(kind AnnouncementKind, message string, ttl time.Duration) *Announcement {
	af.mu.Lock()
	defer af.mu.Unlock()

	now := time.Now()
	var expiresAt time.Time
	if ttl > 0 {
		expiresAt = now.Add(ttl)
	}

	a := &Announcement{
		ID:        generateAnnouncementID(),
		Kind:      kind,
		Message:   message,
		CreatedAt: now,
		ExpiresAt: expiresAt,
		Read:      false,
	}

	af.announcements = append(af.announcements, a)
	return a
}

// Active returns all non-expired announcements.
func (af *AnnouncementFeed) Active() []Announcement {
	af.mu.Lock()
	defer af.mu.Unlock()

	now := time.Now()
	active := make([]Announcement, 0, len(af.announcements))
	remaining := make([]*Announcement, 0, len(af.announcements))

	for _, a := range af.announcements {
		if a.ExpiresAt.IsZero() || a.ExpiresAt.After(now) {
			active = append(active, *a)
			remaining = append(remaining, a)
		}
	}

	af.announcements = remaining
	return active
}

// Unread returns all active announcements that have not been acknowledged.
func (af *AnnouncementFeed) Unread() []Announcement {
	active := af.Active()
	unread := make([]Announcement, 0, len(active))
	for _, a := range active {
		if !a.Read {
			unread = append(unread, a)
		}
	}
	return unread
}

// MarkRead marks an announcement as read by its ID.
func (af *AnnouncementFeed) MarkRead(id string) bool {
	af.mu.Lock()
	defer af.mu.Unlock()

	for _, a := range af.announcements {
		if a.ID == id {
			a.Read = true
			return true
		}
	}
	return false
}

// MarkAllRead marks all active announcements as read.
func (af *AnnouncementFeed) MarkAllRead() {
	af.mu.Lock()
	defer af.mu.Unlock()

	for _, a := range af.announcements {
		a.Read = true
	}
}

// Clear removes all announcements.
func (af *AnnouncementFeed) Clear() {
	af.mu.Lock()
	defer af.mu.Unlock()
	af.announcements = af.announcements[:0]
}

func generateAnnouncementID() string {
	b := make([]byte, 6)
	_, _ = rand.Read(b)
	return "ann-" + hex.EncodeToString(b)
}
