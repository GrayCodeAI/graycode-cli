// announcements.go — System announcements for Hawk.
//
// Provides notification banners for releases, important updates, and system notices.
// Announcements can be shown/dismissed and support expiry dates.

package tool

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/GrayCodeAI/hawk/internal/storage"
)

// Announcement represents a system announcement.
type Announcement struct {
	ID        string           `json:"id,omitempty"`
	Message   string           `json:"message,omitempty"`
	Severity  string           `json:"severity,omitempty"`
	Title     string           `json:"title,omitempty"`
	CTA       *AnnouncementCTA `json:"cta,omitempty"`
	UpdatedAt string           `json:"updated_at,omitempty"`
	ExpiresAt string           `json:"expires_at,omitempty"`
}

// AnnouncementCTA is a call-to-action button for an announcement.
type AnnouncementCTA struct {
	Label   string `json:"label,omitempty"`
	URL     string `json:"url,omitempty"`
	Caption string `json:"caption,omitempty"`
}

// AnnouncementsState holds persisted announcement data.
type AnnouncementsState struct {
	HiddenIDs []string `json:"hidden_ids,omitempty"`
}

// announcementHideKey returns the key used to identify an announcement for hiding.
// Prefers the ID field; falls back to a content-derived key.
func announcementHideKey(a *Announcement) string {
	if trimmed := strings.TrimSpace(a.ID); trimmed != "" {
		return trimmed
	}
	// Fallback: join title/message with unit separator
	return "content:" + strings.TrimSpace(a.Title) + "\x1f" + strings.TrimSpace(a.Message)
}

// ReadAnnouncements reads hidden announcement IDs from state.
func ReadAnnouncements() (*AnnouncementsState, error) {
	state := &AnnouncementsState{}
	data, err := os.ReadFile(announcementsStatePath())
	if err != nil {
		if os.IsNotExist(err) {
			return state, nil // No state file = nothing hidden
		}
		return state, err
	}

	// Try to parse; if malformed, return empty state
	if err := json.Unmarshal(data, state); err != nil {
		return &AnnouncementsState{}, nil
	}
	return state, nil
}

// WriteAnnouncements writes hidden announcement IDs to state.
func WriteAnnouncements(state *AnnouncementsState) error {
	data, err := json.MarshalIndent(state, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(announcementsStatePath(), data, 0o600)
}

// announcementsStatePath returns the path to the announcements state file.
func announcementsStatePath() string {
	return filepath.Join(storage.StateDir(), "announcements.json")
}

// IsAnnouncementExpired checks if an announcement has expired.
func IsAnnouncementExpired(a *Announcement) bool {
	if a.ExpiresAt == "" {
		return false
	}
	t, err := time.Parse(time.RFC3339, a.ExpiresAt)
	if err != nil {
		return false
	}
	return t.Before(time.Now()) || t.Equal(time.Now())
}

// VisibleAnnouncements filters out announcements with empty messages or that are expired/hidden.
func VisibleAnnouncements(announcements []*Announcement, hiddenIDs []string) []*Announcement {
	hiddenSet := make(map[string]bool)
	for _, id := range hiddenIDs {
		hiddenSet[id] = true
	}

	var result []*Announcement
	for _, a := range announcements {
		if a.Message == "" || strings.TrimSpace(a.Message) == "" {
			continue
		}
		if IsAnnouncementExpired(a) {
			continue
		}
		if hiddenSet[announcementHideKey(a)] {
			continue
		}
		result = append(result, a)
	}
	return result
}

// HideAnnouncement adds an announcement ID to the hidden state.
func HideAnnouncement(announcements []*Announcement, index int) error {
	state, err := ReadAnnouncements()
	if err != nil {
		return err
	}

	if index < 0 || index >= len(announcements) {
		return nil // Invalid index, nothing to hide
	}

	key := announcementHideKey(announcements[index])
	// Check if already hidden
	for _, id := range state.HiddenIDs {
		if id == key {
			return nil // Already hidden
		}
	}

	state.HiddenIDs = append(state.HiddenIDs, key)
	// Sort for deterministic order
	sort.Strings(state.HiddenIDs)

	return WriteAnnouncements(state)
}

// PruneHiddenIDs removes hidden IDs that are no longer in active announcements.
func PruneHiddenIDs(state *AnnouncementsState, active []*Announcement) bool {
	activeKeys := make(map[string]bool)
	for _, a := range active {
		activeKeys[announcementHideKey(a)] = true
	}

	changed := false
	newIDs := make([]string, 0, len(state.HiddenIDs))
	for _, id := range state.HiddenIDs {
		if activeKeys[id] {
			newIDs = append(newIDs, id)
		} else {
			changed = true
		}
	}
	state.HiddenIDs = newIDs

	return changed
}

// ResolveStartupAnnouncements resolves announcements with env override support.
// HAWK_ANNOUNCEMENTS_OVERRIDE can be set to a JSON array of announcements.
func ResolveStartupAnnouncements(remote []*Announcement) []*Announcement {
	override := os.Getenv("HAWK_ANNOUNCEMENTS_OVERRIDE")
	if override != "" {
		var overrideAnnouncements []*Announcement
		if err := json.Unmarshal([]byte(override), &overrideAnnouncements); err == nil {
			return overrideAnnouncements
		}
		// Invalid JSON, fall through to remote
	}
	return remote
}
