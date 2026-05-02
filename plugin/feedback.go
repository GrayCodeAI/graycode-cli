package plugin

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"
)

// SkillRating stores a user's rating for a skill.
type SkillRating struct {
	Skill   string    `json:"skill"`
	Rating  int       `json:"rating"` // 1-5
	Comment string    `json:"comment,omitempty"`
	Date    time.Time `json:"date"`
}

// FeedbackStore manages skill ratings persisted to disk.
type FeedbackStore struct {
	path string
}

// NewFeedbackStore creates a store at ~/.hawk/feedback.json.
func NewFeedbackStore() *FeedbackStore {
	home, _ := os.UserHomeDir()
	return &FeedbackStore{path: filepath.Join(home, ".hawk", "feedback.json")}
}

// NewFeedbackStoreAt creates a store at a custom path (for testing).
func NewFeedbackStoreAt(path string) *FeedbackStore {
	return &FeedbackStore{path: path}
}

func (fs *FeedbackStore) load() ([]SkillRating, error) {
	data, err := os.ReadFile(fs.path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	var ratings []SkillRating
	if err := json.Unmarshal(data, &ratings); err != nil {
		return nil, err
	}
	return ratings, nil
}

func (fs *FeedbackStore) save(ratings []SkillRating) error {
	os.MkdirAll(filepath.Dir(fs.path), 0o755)
	data, err := json.MarshalIndent(ratings, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(fs.path, data, 0o644)
}

// Rate adds or updates a rating for a skill.
func (fs *FeedbackStore) Rate(skill string, rating int, comment string) error {
	if rating < 1 || rating > 5 {
		return fmt.Errorf("rating must be 1-5, got %d", rating)
	}
	ratings, _ := fs.load()

	// Update existing or append.
	found := false
	for i := range ratings {
		if ratings[i].Skill == skill {
			ratings[i].Rating = rating
			ratings[i].Comment = comment
			ratings[i].Date = time.Now()
			found = true
			break
		}
	}
	if !found {
		ratings = append(ratings, SkillRating{
			Skill:   skill,
			Rating:  rating,
			Comment: comment,
			Date:    time.Now(),
		})
	}
	return fs.save(ratings)
}

// Get returns the rating for a skill, or 0 if not rated.
func (fs *FeedbackStore) Get(skill string) (SkillRating, bool) {
	ratings, _ := fs.load()
	for _, r := range ratings {
		if r.Skill == skill {
			return r, true
		}
	}
	return SkillRating{}, false
}

// List returns all ratings.
func (fs *FeedbackStore) List() []SkillRating {
	ratings, _ := fs.load()
	return ratings
}

// FormatRating returns a star string like "★★★☆☆".
func FormatRating(rating int) string {
	s := ""
	for i := 0; i < 5; i++ {
		if i < rating {
			s += "★"
		} else {
			s += "☆"
		}
	}
	return s
}
