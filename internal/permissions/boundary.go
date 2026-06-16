package permissions

import (
	"context"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"strings"
	"sync"
)

// BoundaryChecker enforces safety boundaries that prevent the agent from
// performing actions outside its authorized scope.
type BoundaryChecker struct {
	ProjectRoot     string
	AllowedPaths    []string
	BlockedPaths    []string
	AllowedCommands []string
	BlockedCommands []string
	MaxFileSize     int64
	MaxFiles        int
	FilesModified   int

	modifiedFiles map[string]struct{}
	violations    []BoundaryViolation
	mu            sync.RWMutex
}

// BoundaryViolation represents a single boundary violation detected by the checker.
type BoundaryViolation struct {
	Type        string // "path", "command", "size", "count", "network", "env"
	Description string
	Attempted   string
	Allowed     string
	Severity    string // "LOW", "MEDIUM", "HIGH", "CRITICAL"
}

// NewBoundaryChecker creates a new BoundaryChecker with sensible defaults.
func NewBoundaryChecker(projectRoot string) *BoundaryChecker {
	root, err := filepath.Abs(projectRoot)
	if err != nil {
		root = projectRoot
	}
	return &BoundaryChecker{
		ProjectRoot:     root,
		AllowedPaths:    []string{},
		BlockedPaths:    DefaultBlockedPaths(),
		AllowedCommands: []string{},
		BlockedCommands: DefaultBlockedCommands(),
		MaxFileSize:     10 * 1024 * 1024, // 10MB
		MaxFiles:        50,
		FilesModified:   0,
		modifiedFiles:   make(map[string]struct{}),
		violations:      []BoundaryViolation{},
	}
}

// CheckPath verifies that a given path is within the authorized project boundary.
func (bc *BoundaryChecker) CheckPath(path string) *BoundaryViolation {
	bc.mu.RLock()
	defer bc.mu.RUnlock()

	// Resolve to absolute path
	absPath, err := filepath.Abs(path)
	if err != nil {
		v := &BoundaryViolation{
			Type:        "path",
			Description: "failed to resolve path",
			Attempted:   path,
			Allowed:     bc.ProjectRoot,
			Severity:    "HIGH",
		}
		return v
	}

	// Clean the path to remove any traversal components for display
	cleanPath := filepath.Clean(absPath)

	// Check for path traversal attempts (../ in the original path)
	if strings.Contains(path, "..") {
		resolved, err := filepath.Abs(path)
		if err != nil || !strings.HasPrefix(resolved, bc.ProjectRoot) {
			return &BoundaryViolation{
				Type:        "path",
				Description: "path traversal detected",
				Attempted:   path,
				Allowed:     "must be within " + bc.ProjectRoot,
				Severity:    "CRITICAL",
			}
		}
	}

	// Check if path is within project root
	if !strings.HasPrefix(cleanPath, bc.ProjectRoot) {
		return &BoundaryViolation{
			Type:        "path",
			Description: "path outside project root",
			Attempted:   cleanPath,
			Allowed:     "must be within " + bc.ProjectRoot,
			Severity:    "HIGH",
		}
	}

	// Check blocked paths
	for _, blocked := range bc.BlockedPaths {
		expandedBlocked := expandHome(blocked)
		// Check if the clean path matches or is under a blocked path
		if matchesBlockedPath(cleanPath, expandedBlocked) {
			return &BoundaryViolation{
				Type:        "path",
				Description: "access to blocked path",
				Attempted:   cleanPath,
				Allowed:     "path is in blocked list: " + blocked,
				Severity:    "HIGH",
			}
		}
		// Also check relative blocked paths within the project
		if !filepath.IsAbs(blocked) {
			fullBlocked := filepath.Join(bc.ProjectRoot, blocked)
			if matchesBlockedPath(cleanPath, fullBlocked) {
				return &BoundaryViolation{
					Type:        "path",
					Description: "access to blocked path",
					Attempted:   cleanPath,
					Allowed:     "path is in blocked list: " + blocked,
					Severity:    "HIGH",
				}
			}
		}
	}

	// Check symlinks - resolve and verify the target is within project.
	// For non-existent files (e.g. write targets), resolve the parent directory symlinks.
	if target, err := filepath.EvalSymlinks(cleanPath); err == nil {
		if target != cleanPath && !strings.HasPrefix(target, bc.ProjectRoot) {
			return &BoundaryViolation{
				Type:        "path",
				Description: "symlink resolves outside project",
				Attempted:   cleanPath + " -> " + target,
				Allowed:     "symlinks must resolve within " + bc.ProjectRoot,
				Severity:    "CRITICAL",
			}
		}
	} else {
		// File doesn't exist yet — resolve parent directory symlinks to prevent
		// writing through a symlink that points outside the project.
		parent := filepath.Dir(cleanPath)
		if resolved, evalErr := filepath.EvalSymlinks(parent); evalErr == nil {
			resolvedFull := filepath.Join(resolved, filepath.Base(cleanPath))
			if !strings.HasPrefix(resolvedFull, bc.ProjectRoot) {
				return &BoundaryViolation{
					Type:        "path",
					Description: "parent symlink resolves outside project",
					Attempted:   cleanPath + " (parent -> " + resolved + ")",
					Allowed:     "symlinks must resolve within " + bc.ProjectRoot,
					Severity:    "CRITICAL",
				}
			}
		}
	}

	return nil
}

// CheckCommand verifies that a command is not in the blocked list and does not
// attempt privilege escalation or dangerous system operations.
func (bc *BoundaryChecker) CheckCommand(command string) *BoundaryViolation {
	bc.mu.RLock()
	defer bc.mu.RUnlock()

	trimmedCmd := strings.TrimSpace(command)
	lowerCmd := strings.ToLower(trimmedCmd)

	// Check exact match or prefix match against blocked commands
	for _, blocked := range bc.BlockedCommands {
		lowerBlocked := strings.ToLower(blocked)
		if lowerCmd == lowerBlocked || strings.HasPrefix(lowerCmd, lowerBlocked+" ") || strings.HasPrefix(lowerCmd, lowerBlocked+"\t") {
			return &BoundaryViolation{
				Type:        "command",
				Description: "blocked command",
				Attempted:   trimmedCmd,
				Allowed:     "command is in blocked list",
				Severity:    "CRITICAL",
			}
		}
		// Check if the blocked command appears as part of a pipe or chain
		if strings.Contains(lowerCmd, "| "+lowerBlocked) || strings.Contains(lowerCmd, "|"+lowerBlocked) ||
			strings.Contains(lowerCmd, "&& "+lowerBlocked) || strings.Contains(lowerCmd, "&&"+lowerBlocked) ||
			strings.Contains(lowerCmd, "; "+lowerBlocked) || strings.Contains(lowerCmd, ";"+lowerBlocked) {
			return &BoundaryViolation{
				Type:        "command",
				Description: "blocked command in chain",
				Attempted:   trimmedCmd,
				Allowed:     "command contains blocked command: " + blocked,
				Severity:    "CRITICAL",
			}
		}
	}

	// Check privilege escalation
	privEscalation := []string{"sudo", "su", "doas"}
	for _, priv := range privEscalation {
		if strings.HasPrefix(lowerCmd, priv+" ") || lowerCmd == priv {
			return &BoundaryViolation{
				Type:        "command",
				Description: "privilege escalation attempt",
				Attempted:   trimmedCmd,
				Allowed:     "no privilege escalation commands",
				Severity:    "CRITICAL",
			}
		}
	}

	// Check system modification commands
	sysMod := []string{"systemctl", "launchctl", "service", "init.d"}
	for _, sys := range sysMod {
		if strings.HasPrefix(lowerCmd, sys+" ") || lowerCmd == sys {
			return &BoundaryViolation{
				Type:        "command",
				Description: "system modification attempt",
				Attempted:   trimmedCmd,
				Allowed:     "no system modification commands",
				Severity:    "HIGH",
			}
		}
	}

	// Check credential access
	credCmds := []string{"security", "keychain", "pass ", "gpg --export-secret"}
	for _, cred := range credCmds {
		if strings.HasPrefix(lowerCmd, cred) || strings.Contains(lowerCmd, " "+cred) {
			return &BoundaryViolation{
				Type:        "command",
				Description: "credential access attempt",
				Attempted:   trimmedCmd,
				Allowed:     "no credential access commands",
				Severity:    "CRITICAL",
			}
		}
	}

	// Check network exfiltration commands
	netCmds := []string{"curl", "wget", "nc", "ncat", "netcat", "scp", "rsync", "ftp"}
	for _, netCmd := range netCmds {
		if strings.HasPrefix(lowerCmd, netCmd+" ") || lowerCmd == netCmd {
			// If AllowedCommands includes this network command, allow it
			for _, allowed := range bc.AllowedCommands {
				if strings.ToLower(allowed) == netCmd {
					return nil
				}
			}
			return &BoundaryViolation{
				Type:        "command",
				Description: "network exfiltration without approval",
				Attempted:   trimmedCmd,
				Allowed:     "network commands require explicit approval",
				Severity:    "HIGH",
			}
		}
	}

	return nil
}

// CheckFileSize verifies that a file write does not exceed the maximum allowed size.
func (bc *BoundaryChecker) CheckFileSize(path string, size int64) *BoundaryViolation {
	bc.mu.RLock()
	defer bc.mu.RUnlock()

	if size > bc.MaxFileSize {
		return &BoundaryViolation{
			Type:        "size",
			Description: "file size exceeds limit",
			Attempted:   fmt.Sprintf("%s (%d bytes)", path, size),
			Allowed:     fmt.Sprintf("max file size: %d bytes (%dMB)", bc.MaxFileSize, bc.MaxFileSize/(1024*1024)),
			Severity:    "MEDIUM",
		}
	}
	return nil
}

// CheckFileCount verifies that the number of modified files has not exceeded the session limit.
func (bc *BoundaryChecker) CheckFileCount() *BoundaryViolation {
	bc.mu.RLock()
	defer bc.mu.RUnlock()

	if bc.FilesModified >= bc.MaxFiles {
		return &BoundaryViolation{
			Type:        "count",
			Description: "file modification limit reached",
			Attempted:   fmt.Sprintf("modify file #%d", bc.FilesModified+1),
			Allowed:     fmt.Sprintf("max %d files per session", bc.MaxFiles),
			Severity:    "MEDIUM",
		}
	}
	return nil
}

// CheckEnvironment verifies that access to sensitive environment variables is blocked.
func (bc *BoundaryChecker) CheckEnvironment(key string) *BoundaryViolation {
	upperKey := strings.ToUpper(key)

	// Block reading sensitive env vars
	sensitivePatterns := []string{
		"AWS_SECRET",
		"AWS_SESSION_TOKEN",
		"PRIVATE_KEY",
		"SECRET_KEY",
		"API_KEY",
		"API_SECRET",
		"AUTH_TOKEN",
		"ACCESS_TOKEN",
		"REFRESH_TOKEN",
		"DATABASE_PASSWORD",
		"DB_PASSWORD",
		"ENCRYPTION_KEY",
		"SIGNING_KEY",
		"JWT_SECRET",
		"GITHUB_TOKEN",
		"GITLAB_TOKEN",
		"NPM_TOKEN",
		"DOCKER_PASSWORD",
		"REGISTRY_PASSWORD",
	}

	for _, pattern := range sensitivePatterns {
		if strings.Contains(upperKey, pattern) {
			return &BoundaryViolation{
				Type:        "env",
				Description: "access to sensitive environment variable",
				Attempted:   key,
				Allowed:     "sensitive environment variables are blocked",
				Severity:    "CRITICAL",
			}
		}
	}

	// Block setting dangerous env vars
	dangerousSetVars := []string{
		"PATH",
		"LD_PRELOAD",
		"LD_LIBRARY_PATH",
		"DYLD_INSERT_LIBRARIES",
		"DYLD_LIBRARY_PATH",
		"PYTHONPATH",
		"NODE_PATH",
		"RUBYLIB",
		"PERL5LIB",
		"CLASSPATH",
		"HOME",
		"USER",
		"SHELL",
	}

	for _, dangerous := range dangerousSetVars {
		if upperKey == dangerous {
			return &BoundaryViolation{
				Type:        "env",
				Description: "attempt to modify dangerous environment variable",
				Attempted:   key,
				Allowed:     "modification of system environment variables is blocked",
				Severity:    "HIGH",
			}
		}
	}

	return nil
}

// CheckNetwork verifies that network connections are not targeting internal/private networks
// or cloud metadata endpoints.
func (bc *BoundaryChecker) CheckNetwork(host string, port int) *BoundaryViolation {
	// Check cloud metadata endpoint
	if host == "169.254.169.254" || host == "metadata.google.internal" {
		return &BoundaryViolation{
			Type:        "network",
			Description: "access to cloud metadata endpoint",
			Attempted:   fmt.Sprintf("%s:%d", host, port),
			Allowed:     "cloud metadata endpoints are blocked",
			Severity:    "CRITICAL",
		}
	}

	// Parse the host as an IP
	ip := net.ParseIP(host)
	if ip == nil {
		// Try resolving the hostname
		addrs, err := net.DefaultResolver.LookupHost(context.Background(), host)
		if err == nil && len(addrs) > 0 {
			ip = net.ParseIP(addrs[0])
		}
	}

	if ip != nil {
		// Check private network ranges
		privateRanges := []struct {
			network string
			desc    string
		}{
			{"10.0.0.0/8", "10.x.x.x private network"},
			{"192.168.0.0/16", "192.168.x.x private network"},
			{"172.16.0.0/12", "172.16-31.x.x private network"},
			{"127.0.0.0/8", "localhost/loopback"},
			{"169.254.0.0/16", "link-local"},
		}

		for _, pr := range privateRanges {
			_, cidr, err := net.ParseCIDR(pr.network)
			if err != nil {
				continue
			}
			if cidr.Contains(ip) {
				// Allow localhost on common development ports
				if pr.network == "127.0.0.0/8" && isCommonDevPort(port) {
					continue
				}
				return &BoundaryViolation{
					Type:        "network",
					Description: "connection to private/internal network",
					Attempted:   fmt.Sprintf("%s:%d", host, port),
					Allowed:     "connections to internal networks are blocked (" + pr.desc + ")",
					Severity:    "HIGH",
				}
			}
		}
	}

	return nil
}

// IsWithinProject checks whether a path resolves to within the project root.
func (bc *BoundaryChecker) IsWithinProject(path string) bool {
	bc.mu.RLock()
	defer bc.mu.RUnlock()

	absPath, err := filepath.Abs(path)
	if err != nil {
		return false
	}

	cleanPath := filepath.Clean(absPath)

	// Attempt to resolve symlinks
	resolved, err := filepath.EvalSymlinks(cleanPath)
	if err != nil {
		// If the file doesn't exist yet, check the clean path
		return strings.HasPrefix(cleanPath, bc.ProjectRoot)
	}

	return strings.HasPrefix(resolved, bc.ProjectRoot)
}

// RecordModification tracks a file modification for MaxFiles enforcement.
func (bc *BoundaryChecker) RecordModification(path string) {
	bc.mu.Lock()
	defer bc.mu.Unlock()

	absPath, err := filepath.Abs(path)
	if err != nil {
		absPath = path
	}

	if _, exists := bc.modifiedFiles[absPath]; !exists {
		bc.modifiedFiles[absPath] = struct{}{}
		bc.FilesModified++
	}
}

// FormatViolation formats a BoundaryViolation into a human-readable string.
func FormatViolation(v *BoundaryViolation) string {
	if v == nil {
		return ""
	}
	return fmt.Sprintf(
		"[deny]"+" BOUNDARY VIOLATION: %s\nAttempted: %s\nBoundary: %s\nSeverity: %s",
		v.Description,
		v.Attempted,
		v.Allowed,
		v.Severity,
	)
}

// Summary returns a summary of the current session's boundary state.
func (bc *BoundaryChecker) Summary() string {
	bc.mu.RLock()
	defer bc.mu.RUnlock()

	return fmt.Sprintf("Session: %d files modified (limit: %d), %d violations",
		bc.FilesModified, bc.MaxFiles, len(bc.violations))
}

// RecordViolation stores a violation for tracking purposes.
func (bc *BoundaryChecker) RecordViolation(v *BoundaryViolation) {
	if v == nil {
		return
	}
	bc.mu.Lock()
	defer bc.mu.Unlock()
	bc.violations = append(bc.violations, *v)
}

// DefaultBlockedPaths returns the default list of paths that should be blocked.
func DefaultBlockedPaths() []string {
	return []string{
		".git/config",
		".env",
		".env.local",
		".env.production",
		".env.staging",
		"~/.ssh/",
		"~/.aws/",
		"~/.config/gcloud/",
		"~/credentials",
		"~/.netrc",
		"~/.npmrc",
		"~/.docker/config.json",
		"/etc/shadow",
		"/etc/passwd",
		"/etc/sudoers",
		"/etc/hosts",
	}
}

// DefaultBlockedCommands returns the default list of commands that should be blocked.
func DefaultBlockedCommands() []string {
	return []string{
		"sudo",
		"su",
		"doas",
		"chmod 777",
		"chown",
		"mount",
		"umount",
		"systemctl",
		"launchctl",
		"rm -rf /",
		"rm -rf /*",
		"mkfs",
		"dd",
		"iptables",
		"ip6tables",
		"kill -9 1",
		"shutdown",
		"reboot",
		"halt",
		"poweroff",
		"init",
		"telinit",
	}
}

// expandHome expands ~ to the user's home directory.
func expandHome(path string) string {
	if strings.HasPrefix(path, "~/") {
		home, err := os.UserHomeDir()
		if err != nil {
			return path
		}
		return filepath.Join(home, path[2:])
	}
	if path == "~" {
		home, err := os.UserHomeDir()
		if err != nil {
			return path
		}
		return home
	}
	return path
}

// matchesBlockedPath checks if a path matches or is under a blocked path.
func matchesBlockedPath(targetPath, blockedPath string) bool {
	if blockedPath == "" {
		return false
	}

	// If blocked path ends with /, it's a directory - anything under it is blocked
	if strings.HasSuffix(blockedPath, "/") {
		dir := strings.TrimSuffix(blockedPath, "/")
		return strings.HasPrefix(targetPath, dir+"/") || targetPath == dir
	}

	// Exact match
	if targetPath == blockedPath {
		return true
	}

	// Check if target is under blocked path (treat as directory)
	if strings.HasPrefix(targetPath, blockedPath+"/") {
		return true
	}

	return false
}

// isCommonDevPort returns true for ports commonly used in local development.
func isCommonDevPort(port int) bool {
	devPorts := map[int]bool{
		80: true, 443: true, 3000: true, 3001: true, 4000: true, 4200: true,
		4590: true, 5000: true, 5173: true, 5432: true, 5500: true, 6379: true,
		8000: true, 8080: true, 8081: true, 8443: true, 8888: true, 9000: true,
		9090: true, 9200: true, 27017: true,
	}
	return devPorts[port]
}
