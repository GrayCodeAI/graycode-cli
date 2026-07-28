package session

import "fmt"

const maxSessionIDLength = 128

// ValidateID rejects session identifiers that are unsafe to use in file paths.
func ValidateID(id string) error {
	if id == "" {
		return fmt.Errorf("session ID is required")
	}
	if len(id) > maxSessionIDLength {
		return fmt.Errorf("session ID exceeds %d characters", maxSessionIDLength)
	}
	if id == "." || id == ".." {
		return fmt.Errorf("session ID %q is reserved", id)
	}
	for _, r := range id {
		if (r >= 'a' && r <= 'z') ||
			(r >= 'A' && r <= 'Z') ||
			(r >= '0' && r <= '9') ||
			r == '-' || r == '_' || r == '.' {
			continue
		}
		return fmt.Errorf("session ID contains invalid character %q", r)
	}
	return nil
}

// ValidID reports whether id is safe to use as a session identifier.
func ValidID(id string) bool {
	return ValidateID(id) == nil
}
