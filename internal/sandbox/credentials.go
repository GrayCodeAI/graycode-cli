// Package sandbox — credential access gating.
//
// The sandbox container starts with all candidate host credentials mounted
// read-only into a staging area. The "expected" paths (e.g. ~/.kube/config)
// are symlinks to a "denied" placeholder. When the AI requests a credential
// and the user approves, the symlink is flipped to the staging copy. This
// avoids the Docker limitation that mounts cannot be added to a running
// container.
package sandbox

import (
	"fmt"
	"os"
	"path/filepath"
	"sync"
)

// stagingDir is where host credentials are mounted read-only.
const stagingDir = "/_credentials/staging"

// deniedTarget is the symlink target for credentials that have not been
// approved. Reading it returns a clear "not approved" message.
const deniedTarget = "/_credentials/denied"

// homeDir is the writable home inside the container where symlinks at
// "expected" paths live.
const homeDir = "/root"

// CredentialDescriptor describes one candidate host credential.
type CredentialDescriptor struct {
	// ID is the stable identifier used by the AI to request this credential.
	ID string
	// HostPath is the path on the host (e.g. "~/.kube/config"). "~" is
	// expanded to the host home at container-start time.
	HostPath string
	// ContainerPath is the path inside the container where the credential
	// is expected by tools (e.g. "/root/.kube/config").
	ContainerPath string
	// Name is the human-readable label shown in approval prompts.
	Name string
	// Description explains what this credential is for.
	Description string
}

// registry is the set of candidate credentials. Only credentials whose
// host paths exist at container-start time are actually mounted.
var registry = []CredentialDescriptor{
	{
		ID: "gitconfig", HostPath: "~/.gitconfig", ContainerPath: filepath.Join(homeDir, ".gitconfig"),
		Name: "Git config", Description: "user name, email, and credential helpers",
	},
	{
		ID: "kube", HostPath: "~/.kube", ContainerPath: filepath.Join(homeDir, ".kube"),
		Name: "Kubernetes config", Description: "cluster credentials for kubectl",
	},
	{
		ID: "aws", HostPath: "~/.aws", ContainerPath: filepath.Join(homeDir, ".aws"),
		Name: "AWS credentials", Description: "access keys and profiles for AWS CLI",
	},
	{
		ID: "gh", HostPath: "~/.config/gh", ContainerPath: filepath.Join(homeDir, ".config", "gh"),
		Name: "GitHub CLI auth", Description: "gh authentication tokens",
	},
	{
		ID: "docker", HostPath: "~/.docker", ContainerPath: filepath.Join(homeDir, ".docker"),
		Name: "Docker config", Description: "registry auth and Docker settings",
	},
	{
		ID: "gnupg", HostPath: "~/.gnupg", ContainerPath: filepath.Join(homeDir, ".gnupg"),
		Name: "GPG keys", Description: "signing and encryption keys",
	},
	{
		ID: "terraform", HostPath: "~/.terraform.d", ContainerPath: filepath.Join(homeDir, ".terraform.d"),
		Name: "Terraform plugins", Description: "provider plugins and cache",
	},
}

// CredentialGate manages the symlink-based access control for one running
// container. It is safe for concurrent use.
type CredentialGate struct {
	mu        sync.Mutex
	approved  map[string]bool // credential ID -> approved
	container *ContainerSandbox
}

// NewCredentialGate creates a gate for the given container. The container
// must have been started with the staging mounts in place.
func NewCredentialGate(c *ContainerSandbox) *CredentialGate {
	return &CredentialGate{
		approved:  make(map[string]bool),
		container: c,
	}
}

// Registry returns the credential descriptors.
func Registry() []CredentialDescriptor {
	return registry
}

// FindCredential returns the descriptor for a given ID, or nil if unknown.
func FindCredential(id string) *CredentialDescriptor {
	for i := range registry {
		if registry[i].ID == id {
			return &registry[i]
		}
	}
	return nil
}

// IsApproved reports whether a credential has been approved.
func (g *CredentialGate) IsApproved(id string) bool {
	g.mu.Lock()
	defer g.mu.Unlock()
	return g.approved[id]
}

// Approve flips the symlink for a credential from the denied placeholder to
// the staging copy. Returns an error if the credential is unknown or the
// staging copy does not exist.
func (g *CredentialGate) Approve(id string) error {
	g.mu.Lock()
	defer g.mu.Unlock()

	desc := FindCredential(id)
	if desc == nil {
		return fmt.Errorf("unknown credential: %s", id)
	}

	// Verify the staging copy exists.
	stagingPath := StagingPath(id)
	if _, err := os.Stat(stagingPath); err != nil {
		return fmt.Errorf("credential %q not available on host (no staging copy at %s): %w", id, stagingPath, err)
	}

	// Flip the symlink.
	if err := flipSymlink(desc.ContainerPath, stagingPath); err != nil {
		return fmt.Errorf("failed to grant access to %q: %w", id, err)
	}

	g.approved[id] = true
	return nil
}

// Deny ensures a credential remains inaccessible (symlink points to denied).
func (g *CredentialGate) Deny(id string) error {
	g.mu.Lock()
	defer g.mu.Unlock()

	desc := FindCredential(id)
	if desc == nil {
		return fmt.Errorf("unknown credential: %s", id)
	}

	if err := flipSymlink(desc.ContainerPath, deniedTarget); err != nil {
		return fmt.Errorf("failed to revoke access to %q: %w", id, err)
	}
	delete(g.approved, id)
	return nil
}

// Approved returns the set of approved credential IDs.
func (g *CredentialGate) Approved() []string {
	g.mu.Lock()
	defer g.mu.Unlock()
	out := make([]string, 0, len(g.approved))
	for id := range g.approved {
		out = append(out, id)
	}
	return out
}

// StagingPath returns the staging mount path for a credential ID.
func StagingPath(id string) string {
	return filepath.Join(stagingDir, id)
}

// flipSymlink replaces the symlink at linkPath with a symlink to target.
// If linkPath does not exist, it creates the parent directory.
func flipSymlink(linkPath, target string) error {
	// Remove existing symlink or file.
	_ = os.Remove(linkPath)
	// Ensure parent exists.
	if dir := filepath.Dir(linkPath); dir != "" {
		_ = os.MkdirAll(dir, 0o700)
	}
	return os.Symlink(target, linkPath)
}

// InitCredentialLayout sets up the denied placeholder and the initial
// denied symlinks for all credentials. Called once at container start.
func InitCredentialLayout(descs []CredentialDescriptor) error {
	// Create the denied placeholder file.
	if err := os.MkdirAll(filepath.Dir(deniedTarget), 0o755); err != nil {
		return err
	}
	content := []byte("# Credential access not approved.\n# This symlink will be replaced when access is granted.\n")
	if err := os.WriteFile(deniedTarget, content, 0o644); err != nil {
		return err
	}

	// Point each credential's container path to the denied placeholder.
	for _, desc := range descs {
		if err := flipSymlink(desc.ContainerPath, deniedTarget); err != nil {
			return fmt.Errorf("init denied symlink for %s: %w", desc.ID, err)
		}
	}
	return nil
}
