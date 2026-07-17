package permissions

import (
	"fmt"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"sync"

	"github.com/GrayCodeAI/hawk/internal/ui/icons"
)

// Pre-compiled patterns for exfiltration detection.
var (
	cachedRegex = &sync.Map{} // string(pattern) -> *regexp.Regexp

	// Pre-compiled patterns for exfiltration detection.
	curlPostWithData = regexp.MustCompile(`(?i)curl\s.*-[A-Za-z]*X\s*POST.*-d\s+@`)
	curlDataWithPost = regexp.MustCompile(`(?i)curl\s.*-d\s+@.*-[A-Za-z]*X\s*POST`)
	curlDataBinary   = regexp.MustCompile(`(?i)curl\s.*--data-binary\s+@`)
	pipeToNetwork    = regexp.MustCompile(`\|\s*(curl|wget|nc|netcat|ncat)\b`)
	base64PipeNet    = regexp.MustCompile(`base64.*\|\s*(curl|wget|nc|netcat)`)
	base64Net        = regexp.MustCompile(`\bbase64\b.*\b(curl|wget|nc|netcat)\b`)
	envVarInURL      = regexp.MustCompile(`(curl|wget)\s.*\$[A-Za-z_]`)
	curlFileUpload   = regexp.MustCompile(`(?i)curl\s.*-F\s+["']?[^=]+=@`)
)

// EgressInspector detects and blocks data exfiltration attempts in shell commands
// by checking outbound network destinations before execution.
type EgressInspector struct {
	AllowedDomains   []string
	BlockedDomains   []string
	AllowedProtocols []string
	mu               sync.RWMutex
	compiledPatterns sync.Map // string pattern -> *regexp.Regexp
}

// EgressAttempt represents the result of inspecting a command for egress activity.
type EgressAttempt struct {
	Command      string
	Destinations []Destination
	Allowed      bool
	Reason       string
}

// Destination represents a network destination extracted from a command.
type Destination struct {
	Host       string
	Port       int
	Protocol   string
	Source     string
	Suspicious bool
}

// NewEgressInspector creates an EgressInspector with sensible defaults.
func NewEgressInspector() *EgressInspector {
	return &EgressInspector{
		AllowedDomains: []string{
			"github.com",
			"golang.org",
			"npmjs.org",
			"pypi.org",
			"crates.io",
			"rubygems.org",
		},
		BlockedDomains: []string{
			"pastebin.com",
			"requestbin.*",
			"*.ngrok.io",
			"transfer.sh",
			"file.io",
		},
		AllowedProtocols: []string{
			"https",
			"ssh",
			"git",
		},
	}
}

// compilePattern pre-compiles a wildcard glob pattern to a compiled regexp and
// caches it in the inspector's compiledPatterns map for reuse.
func (e *EgressInspector) compilePattern(pattern string) *regexp.Regexp {
	if cached, ok := e.compiledPatterns.Load(pattern); ok {
		return cached.(*regexp.Regexp)
	}

	regexStr := "^" + regexp.QuoteMeta(pattern) + "$"
	regexStr = strings.ReplaceAll(regexStr, `\*`, `[A-Za-z0-9._-]*`)
	re, err := regexp.Compile(regexStr)
	if err != nil {
		return nil
	}

	if val, loaded := e.compiledPatterns.LoadOrStore(pattern, re); loaded {
		return val.(*regexp.Regexp)
	}
	return re
}

// Inspect analyzes a command for network egress destinations and returns
// an EgressAttempt indicating whether the command is allowed.
func (e *EgressInspector) Inspect(command string) *EgressAttempt {
	attempt := &EgressAttempt{
		Command: command,
		Allowed: true,
	}

	// Extract all destinations from the command
	var destinations []Destination

	// Parse URLs from curl/wget/git etc.
	for _, rawURL := range e.ExtractURLs(command) {
		dest := e.parseURL(rawURL, command)
		if dest != nil {
			destinations = append(destinations, *dest)
		}
	}

	// Parse SSH destinations
	for _, sshDest := range e.ExtractSSHDests(command) {
		dest := e.parseSSHDest(sshDest)
		if dest != nil {
			destinations = append(destinations, *dest)
		}
	}

	// Parse netcat destinations
	for _, ncDest := range e.ExtractNetcat(command) {
		dest := e.parseNetcatDest(ncDest)
		if dest != nil {
			destinations = append(destinations, *dest)
		}
	}

	// Check suspicion
	isSuspicious := e.IsSuspicious(command)

	// Evaluate each destination
	var blockedReasons []string
	for i := range destinations {
		if !e.IsAllowed(destinations[i].Host) {
			attempt.Allowed = false
			blockedReasons = append(blockedReasons, fmt.Sprintf("%s not in allowlist", destinations[i].Host))
		}
		if isSuspicious {
			destinations[i].Suspicious = true
		}
	}

	if !attempt.Allowed {
		attempt.Reason = strings.Join(blockedReasons, "; ")
	}

	attempt.Destinations = destinations
	return attempt
}

// ExtractURLs finds all URLs in the command (http://, https://, git://, ssh://).
func (e *EgressInspector) ExtractURLs(command string) []string {
	re := regexp.MustCompile(`(https?://[^\s"'` + "`" + `;<>|]+|git://[^\s"'` + "`" + `;<>|]+|ssh://[^\s"'` + "`" + `;<>|]+)`)
	matches := re.FindAllString(command, -1)
	var results []string
	for _, m := range matches {
		// Trim trailing punctuation that may have been captured
		m = strings.TrimRight(m, ".,;:)")
		results = append(results, m)
	}
	return results
}

// ExtractSSHDests parses ssh user@host, scp user@host:path patterns.
func (e *EgressInspector) ExtractSSHDests(command string) []string {
	var results []string

	// Match user@host pattern (most reliable for SSH-family commands)
	userHostRe := regexp.MustCompile(`\b(?:ssh|scp|rsync)\b.*?([A-Za-z0-9._-]+)@([A-Za-z0-9._-]+(?:\.[A-Za-z]{2,}))`)
	matches := userHostRe.FindAllStringSubmatch(command, -1)
	for _, m := range matches {
		if len(m) >= 3 {
			host := m[2]
			results = append(results, host)
		}
	}

	// Match ssh host (without user@) - ssh followed by options then a bare hostname
	// Only for ssh command (scp/rsync always have file args that confuse bare host detection)
	sshBareRe := regexp.MustCompile(`\bssh\s+(?:-[A-Za-z]+\s+\S+\s+)*(?:-[A-Za-z]+\s+)*([A-Za-z][A-Za-z0-9._-]*\.[A-Za-z]{2,})`)
	sshBareMatches := sshBareRe.FindAllStringSubmatch(command, -1)
	for _, m := range sshBareMatches {
		if len(m) >= 2 {
			host := m[1]
			// Avoid duplicates
			found := false
			for _, r := range results {
				if r == host {
					found = true
					break
				}
			}
			if !found {
				results = append(results, host)
			}
		}
	}

	return results
}

// ExtractNetcat parses nc/netcat host port patterns.
func (e *EgressInspector) ExtractNetcat(command string) []string {
	var results []string
	ncRe := regexp.MustCompile(`\b(?:nc|netcat|ncat)\s+(?:-[^\s]*\s+)*([A-Za-z0-9._-]+)\s+(\d+)`)
	matches := ncRe.FindAllStringSubmatch(command, -1)
	for _, m := range matches {
		if len(m) >= 3 {
			results = append(results, m[1]+":"+m[2])
		}
	}
	return results
}

// IsAllowed checks whether a host is permitted based on allow/block lists.
// Blocked takes precedence over allowed.
func (e *EgressInspector) IsAllowed(host string) bool {
	e.mu.RLock()
	defer e.mu.RUnlock()

	// Check blocked list first (takes precedence)
	for _, pattern := range e.BlockedDomains {
		if matchDomain(pattern, host) {
			return false
		}
	}

	// Check allowed list
	for _, allowed := range e.AllowedDomains {
		if matchDomain(allowed, host) {
			return true
		}
	}

	// Not in allowlist = not allowed
	return false
}

// IsSuspicious detects patterns commonly associated with data exfiltration.
func (e *EgressInspector) IsSuspicious(command string) bool {
	// POST with file data
	if curlPostWithData.MatchString(command) ||
		curlDataWithPost.MatchString(command) ||
		curlDataBinary.MatchString(command) {
		return true
	}

	// Pipe to network command (cat file | curl, cat file | nc)
	if pipeToNetwork.MatchString(command) {
		return true
	}

	// Base64 encoding combined with network send
	if base64PipeNet.MatchString(command) ||
		base64Net.MatchString(command) {
		return true
	}

	// Environment variable in URL
	if envVarInURL.MatchString(command) {
		return true
	}

	// File upload via curl -F
	if curlFileUpload.MatchString(command) {
		return true
	}

	return false
}

// FormatAttempt produces a human-readable report of an egress inspection.
func (e *EgressInspector) FormatAttempt(attempt *EgressAttempt) string {
	var sb strings.Builder

	status := "ALLOWED"
	if !attempt.Allowed {
		status = "BLOCKED"
	}

	sb.WriteString(fmt.Sprintf("Egress Inspection: %s\n", status))
	sb.WriteString(fmt.Sprintf("Command: %s\n", attempt.Command))

	if len(attempt.Destinations) > 0 {
		sb.WriteString("\nDestinations:\n")
		for _, dest := range attempt.Destinations {
			marker := icons.CheckBold()
			reason := "allowed"
			if !e.IsAllowed(dest.Host) {
				marker = icons.CloseThick()
				reason = "not in allowlist"
			}
			portStr := ""
			if dest.Port > 0 {
				portStr = fmt.Sprintf(":%d", dest.Port)
			}
			protoStr := ""
			if dest.Protocol != "" {
				protoStr = fmt.Sprintf(" (%s)", dest.Protocol)
			}
			sb.WriteString(fmt.Sprintf("  %s %s%s%s — %s\n", marker, dest.Host, portStr, protoStr, reason))
		}
	}

	// Suspicious patterns
	if e.IsSuspicious(attempt.Command) {
		sb.WriteString("\nSuspicious patterns:\n")
		patterns := e.describeSuspicious(attempt.Command)
		for _, p := range patterns {
			sb.WriteString(fmt.Sprintf("  - %s\n", p))
		}
	}

	return sb.String()
}

// AddAllowed adds a domain to the allowed list.
func (e *EgressInspector) AddAllowed(domain string) {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.AllowedDomains = append(e.AllowedDomains, domain)
}

// AddBlocked adds a domain to the blocked list.
func (e *EgressInspector) AddBlocked(domain string) {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.BlockedDomains = append(e.BlockedDomains, domain)
}

// parseURL converts a raw URL string into a Destination.
func (e *EgressInspector) parseURL(rawURL string, command string) *Destination {
	parsed, err := url.Parse(rawURL)
	if err != nil {
		return nil
	}

	host := parsed.Hostname()
	if host == "" {
		return nil
	}

	port := 0
	if parsed.Port() != "" {
		port, _ = strconv.Atoi(parsed.Port())
	} else {
		switch parsed.Scheme {
		case "https":
			port = 443
		case "http":
			port = 80
		case "git":
			port = 9418
		case "ssh":
			port = 22
		}
	}

	return &Destination{
		Host:     host,
		Port:     port,
		Protocol: parsed.Scheme,
		Source:   rawURL,
	}
}

// parseSSHDest converts a host string from SSH extraction into a Destination.
func (e *EgressInspector) parseSSHDest(host string) *Destination {
	return &Destination{
		Host:     host,
		Port:     22,
		Protocol: "ssh",
		Source:   host,
	}
}

// parseNetcatDest converts a host:port string from netcat extraction into a Destination.
func (e *EgressInspector) parseNetcatDest(hostPort string) *Destination {
	parts := strings.SplitN(hostPort, ":", 2)
	if len(parts) != 2 {
		return nil
	}
	port, _ := strconv.Atoi(parts[1])
	return &Destination{
		Host:     parts[0],
		Port:     port,
		Protocol: "tcp",
		Source:   hostPort,
	}
}

// matchDomain checks if a host matches a domain pattern (supports * wildcards).
func matchDomain(pattern, host string) bool {
	// Exact match
	if pattern == host {
		return true
	}

// Wildcard matching
if strings.Contains(pattern, "*") {
		// Convert glob pattern to regex with caching
		regexStr := "^" + regexp.QuoteMeta(pattern) + "$"
		regexStr = strings.ReplaceAll(regexStr, `\*`, `[A-Za-z0-9._-]*`)
		re, loaded := cachedRegex.LoadOrStore(regexStr, regexp.MustCompile(regexStr))
		if loaded {
			return re.(*regexp.Regexp).MatchString(host)
		}
		return re.(*regexp.Regexp).MatchString(host)
	}

	// Subdomain match: pattern "example.com" matches "sub.example.com"
	if strings.HasSuffix(host, "."+pattern) {
		return true
	}

	return false
}

// describeSuspicious returns human-readable descriptions of detected suspicious patterns.
func (e *EgressInspector) describeSuspicious(command string) []string {
	var patterns []string

	if curlPostWithData.MatchString(command) ||
		curlDataWithPost.MatchString(command) {
		// Try to extract the file name
		fileRe := regexp.MustCompile(`-d\s+@([^\s"']+)`)
		if m := fileRe.FindStringSubmatch(command); len(m) >= 2 {
			patterns = append(patterns, fmt.Sprintf("POST with file data (@%s)", m[1]))
		} else {
			patterns = append(patterns, "POST with file data")
		}
	}

	if curlDataBinary.MatchString(command) {
		patterns = append(patterns, "Binary file upload")
	}

	if pipeToNetwork.MatchString(command) {
		patterns = append(patterns, "Pipe to network command")
	}

	if base64PipeNet.MatchString(command) ||
		base64Net.MatchString(command) {
		patterns = append(patterns, "Base64 encoding with network send")
	}

	if envVarInURL.MatchString(command) {
		patterns = append(patterns, "Environment variable in URL")
	}

	if curlFileUpload.MatchString(command) {
		patterns = append(patterns, "File upload via form data")
	}

	// Check for unknown destinations
	for _, rawURL := range e.ExtractURLs(command) {
		parsed, err := url.Parse(rawURL)
		if err == nil && parsed.Hostname() != "" {
			if !e.IsAllowed(parsed.Hostname()) {
				patterns = append(patterns, "Unknown destination")
				break
			}
		}
	}

	return patterns
}
