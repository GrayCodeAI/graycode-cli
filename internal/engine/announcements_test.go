package engine

import (
	"testing"
	"time"
)

func TestAnnouncementFeed_PostAndActive(t *testing.T) {
	af := NewAnnouncementFeed()

	a1 := af.Post(AnnouncementInfo, "System maintenance in 1 hour", 1*time.Hour)
	a2 := af.Post(AnnouncementWarning, "Rate limit approaching", 0)

	active := af.Active()
	if len(active) != 2 {
		t.Fatalf("expected 2 active announcements, got %d", len(active))
	}

	unread := af.Unread()
	if len(unread) != 2 {
		t.Fatalf("expected 2 unread announcements, got %d", len(unread))
	}

	if !af.MarkRead(a1.ID) {
		t.Error("expected MarkRead to succeed for a1")
	}

	unread = af.Unread()
	if len(unread) != 1 || unread[0].ID != a2.ID {
		t.Errorf("expected 1 unread announcement (a2), got %v", unread)
	}

	af.MarkAllRead()
	if len(af.Unread()) != 0 {
		t.Error("expected 0 unread announcements after MarkAllRead")
	}
}

func TestAnnouncementFeed_Expiration(t *testing.T) {
	af := NewAnnouncementFeed()

	// Post with short TTL
	af.Post(AnnouncementInfo, "Temporary notice", 10*time.Millisecond)
	af.Post(AnnouncementSystem, "Permanent notice", 0)

	time.Sleep(25 * time.Millisecond)

	active := af.Active()
	if len(active) != 1 || active[0].Message != "Permanent notice" {
		t.Errorf("expected only permanent notice after expiration, got %v", active)
	}
}

func TestAnnouncementFeed_Clear(t *testing.T) {
	af := NewAnnouncementFeed()
	af.Post(AnnouncementInfo, "Notice 1", 0)
	af.Post(AnnouncementInfo, "Notice 2", 0)

	af.Clear()
	if len(af.Active()) != 0 {
		t.Error("expected 0 announcements after Clear")
	}
}
