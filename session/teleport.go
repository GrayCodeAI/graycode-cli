package session

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"
)

// TeleportPackage is a portable session bundle for cross-machine transfer.
type TeleportPackage struct {
	Version    string    `json:"version"`
	SessionID  string    `json:"session_id"`
	CreatedAt  time.Time `json:"created_at"`
	SourceHost string    `json:"source_host"`
	Session    *Session  `json:"session"`
	Checksum   string    `json:"checksum"`
}

// ExportForTeleport packages a session for transfer to another machine.
func ExportForTeleport(sessionID string) (*TeleportPackage, error) {
	sess, err := Load(sessionID)
	if err != nil {
		return nil, fmt.Errorf("loading session %s: %w", sessionID, err)
	}

	hostname, _ := os.Hostname()

	pkg := &TeleportPackage{
		Version:    "1",
		SessionID:  sessionID,
		CreatedAt:  time.Now(),
		SourceHost: hostname,
		Session:    sess,
	}

	// Compute checksum over session content
	data, _ := json.Marshal(sess)
	hash := sha256.Sum256(data)
	pkg.Checksum = hex.EncodeToString(hash[:])

	return pkg, nil
}

// SaveTeleportPackage writes a teleport package to a file.
func SaveTeleportPackage(pkg *TeleportPackage, path string) error {
	data, err := json.MarshalIndent(pkg, "", "  ")
	if err != nil {
		return fmt.Errorf("marshaling teleport package: %w", err)
	}
	return os.WriteFile(path, data, 0o644)
}

// LoadTeleportPackage reads a teleport package from a file.
func LoadTeleportPackage(path string) (*TeleportPackage, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("reading teleport package: %w", err)
	}

	var pkg TeleportPackage
	if err := json.Unmarshal(data, &pkg); err != nil {
		return nil, fmt.Errorf("parsing teleport package: %w", err)
	}
	return &pkg, nil
}

// ImportFromTeleport imports a teleported session onto this machine.
func ImportFromTeleport(pkg *TeleportPackage) error {
	if pkg.Session == nil {
		return fmt.Errorf("teleport package has no session data")
	}

	// Verify checksum
	data, _ := json.Marshal(pkg.Session)
	hash := sha256.Sum256(data)
	checksum := hex.EncodeToString(hash[:])
	if checksum != pkg.Checksum {
		return fmt.Errorf("checksum mismatch: session data may be corrupted")
	}

	// Save with a new ID to avoid conflicts
	sess := pkg.Session
	sess.ID = fmt.Sprintf("teleport_%s_%d", pkg.SessionID, time.Now().UnixNano())

	return Save(sess)
}

// TeleportDir returns the directory for teleport packages.
func TeleportDir() string {
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".hawk", "teleport")
}

// ExportToFile exports a session to a teleport file in the default directory.
func ExportToFile(sessionID string) (string, error) {
	pkg, err := ExportForTeleport(sessionID)
	if err != nil {
		return "", err
	}

	dir := TeleportDir()
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", err
	}

	filename := fmt.Sprintf("%s_%d.teleport.json", sessionID, time.Now().Unix())
	path := filepath.Join(dir, filename)

	if err := SaveTeleportPackage(pkg, path); err != nil {
		return "", err
	}
	return path, nil
}
