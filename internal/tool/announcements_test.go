package tool

import (
	"testing"
	"time"
)

func TestAnnouncementHideKey(t *testing.T) {
	tests := []struct {
		name     string
		announcement *Announcement
		expected string
	}{
		{
			name: "id with whitespace",
			announcement: &Announcement{ID: "  spaced-id  ", Title: "T", Message: "M"},
			expected: "spaced-id",
		},
		{
			name: "blank id uses content fallback",
			announcement: &Announcement{ID: "   ", Title: "Title", Message: "Message"},
			expected: "content:Title\x1fMessage",
		},
		{
			name: "no id uses content fallback",
			announcement: &Announcement{Title: "", Message: ""},
			expected: "content:\x1f",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := announcementHideKey(tt.announcement)
			if got != tt.expected {
				t.Errorf("announcementHideKey() = %q, want %q", got, tt.expected)
			}
		})
	}
}

func TestIsAnnouncementExpired(t *testing.T) {
	past := time.Now().Add(-24 * time.Hour).Format(time.RFC3339)
	future := time.Now().Add(24 * time.Hour).Format(time.RFC3339)

	tests := []struct {
		name     string
		announcement *Announcement
		expired  bool
	}{
		{"no expiry", &Announcement{ExpiresAt: ""}, false},
		{"past expiry", &Announcement{ExpiresAt: past}, true},
		{"future expiry", &Announcement{ExpiresAt: future}, false},
		{"invalid format", &Announcement{ExpiresAt: "invalid"}, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := IsAnnouncementExpired(tt.announcement)
			if got != tt.expired {
				t.Errorf("IsAnnouncementExpired() = %v, want %v", got, tt.expired)
			}
		})
	}
}

func TestVisibleAnnouncements(t *testing.T) {
	tests := []struct {
		name        string
		announcements []*Announcement
		hiddenIDs   []string
		expectedCount int
	}{
		{
			name: "empty input",
			announcements: nil,
			hiddenIDs: nil,
			expectedCount: 0,
		},
		{
			name: "single visible",
			announcements: []*Announcement{{Message: "Hello"}},
			hiddenIDs: nil,
			expectedCount: 1,
		},
		{
			name: "empty message filtered",
			announcements: []*Announcement{{Message: "   "}},
			hiddenIDs: nil,
			expectedCount: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := VisibleAnnouncements(tt.announcements, tt.hiddenIDs)
			if len(got) != tt.expectedCount {
				t.Errorf("VisibleAnnouncements() returned %d items, want %d", len(got), tt.expectedCount)
			}
		})
	}
}

func TestPruneHiddenIDs(t *testing.T) {
	active := []*Announcement{
		{ID: "active"},
		{ID: "also-active"},
	}
	state := &AnnouncementsState{
		HiddenIDs: []string{"active", "gone"},
	}

	changed := PruneHiddenIDs(state, active)
	if !changed {
		t.Error("PruneHiddenIDs should return true when state changes")
	}
	if len(state.HiddenIDs) != 1 {
		t.Errorf("PruneHiddenIDs should have 1 ID, got %d", len(state.HiddenIDs))
	}
	if state.HiddenIDs[0] != "active" {
		t.Errorf("PruneHiddenIDs should keep 'active', got %s", state.HiddenIDs[0])
	}
}