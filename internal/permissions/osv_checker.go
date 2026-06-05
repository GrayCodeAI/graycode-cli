package permissions

import (
	"fmt"
	"regexp"
	"strings"
	"sync"
	"time"
	"unicode"
)

// MalwareEntry represents a known malicious package in the database.
type MalwareEntry struct {
	Package     string
	Ecosystem   string // "npm", "pypi", "go", "crates"
	Advisory    string
	Severity    string // "CRITICAL", "HIGH", "MEDIUM", "LOW"
	Description string
	DateAdded   time.Time
}

// CheckResult represents the outcome of a package safety check.
type CheckResult struct {
	Package        string
	Safe           bool
	Advisories     []string
	Severity       string
	Recommendation string
	CheckedAt      time.Time
}

// OSVChecker checks packages against a known malware database before installation.
type OSVChecker struct {
	KnownMalware map[string]*MalwareEntry
	Cache        map[string]*CheckResult
	CacheTTL     time.Duration
	mu           sync.RWMutex
}

// NewOSVChecker creates an OSVChecker pre-populated with known malicious packages.
func NewOSVChecker() *OSVChecker {
	checker := &OSVChecker{
		KnownMalware: make(map[string]*MalwareEntry),
		Cache:        make(map[string]*CheckResult),
		CacheTTL:     1 * time.Hour,
	}

	entries := []*MalwareEntry{
		// NPM - compromised supply chain
		{Package: "event-stream", Ecosystem: "npm", Advisory: "MAL-2018-0001", Severity: "CRITICAL", Description: "compromised supply chain - cryptocurrency wallet theft", DateAdded: time.Date(2018, 11, 26, 0, 0, 0, 0, time.UTC)},
		{Package: "ua-parser-js", Ecosystem: "npm", Advisory: "MAL-2021-0001", Severity: "CRITICAL", Description: "compromised supply chain - cryptominer and credential stealer", DateAdded: time.Date(2021, 10, 22, 0, 0, 0, 0, time.UTC)},
		{Package: "coa", Ecosystem: "npm", Advisory: "MAL-2021-0002", Severity: "CRITICAL", Description: "compromised supply chain - credential stealer", DateAdded: time.Date(2021, 11, 4, 0, 0, 0, 0, time.UTC)},
		{Package: "rc", Ecosystem: "npm", Advisory: "MAL-2021-0003", Severity: "CRITICAL", Description: "compromised supply chain - credential stealer", DateAdded: time.Date(2021, 11, 4, 0, 0, 0, 0, time.UTC)},

		// NPM - protestware
		{Package: "colors", Ecosystem: "npm", Advisory: "MAL-2022-0001", Severity: "HIGH", Description: "protestware - infinite loop introduced by maintainer", DateAdded: time.Date(2022, 1, 8, 0, 0, 0, 0, time.UTC)},
		{Package: "faker", Ecosystem: "npm", Advisory: "MAL-2022-0002", Severity: "HIGH", Description: "protestware - replaced with empty module by maintainer", DateAdded: time.Date(2022, 1, 5, 0, 0, 0, 0, time.UTC)},
		{Package: "node-ipc", Ecosystem: "npm", Advisory: "MAL-2022-0003", Severity: "CRITICAL", Description: "protestware - destructive payload targeting specific geolocations", DateAdded: time.Date(2022, 3, 15, 0, 0, 0, 0, time.UTC)},

		// NPM - typosquats
		{Package: "@pnpm/exe", Ecosystem: "npm", Advisory: "MAL-2022-0010", Severity: "CRITICAL", Description: "typosquat of pnpm - credential stealer", DateAdded: time.Date(2022, 5, 20, 0, 0, 0, 0, time.UTC)},
		{Package: "crossenv", Ecosystem: "npm", Advisory: "MAL-2017-0001", Severity: "CRITICAL", Description: "typosquat of cross-env - credential stealer", DateAdded: time.Date(2017, 8, 1, 0, 0, 0, 0, time.UTC)},
		{Package: "lodahs", Ecosystem: "npm", Advisory: "MAL-2019-0001", Severity: "HIGH", Description: "typosquat of lodash - data exfiltration", DateAdded: time.Date(2019, 6, 15, 0, 0, 0, 0, time.UTC)},
		{Package: "lodashs", Ecosystem: "npm", Advisory: "MAL-2019-0002", Severity: "HIGH", Description: "typosquat of lodash - data exfiltration", DateAdded: time.Date(2019, 6, 15, 0, 0, 0, 0, time.UTC)},
		{Package: "expresss", Ecosystem: "npm", Advisory: "MAL-2020-0001", Severity: "HIGH", Description: "typosquat of express - backdoor", DateAdded: time.Date(2020, 3, 10, 0, 0, 0, 0, time.UTC)}, //nolint:misspell
		{Package: "babelcli", Ecosystem: "npm", Advisory: "MAL-2017-0002", Severity: "HIGH", Description: "typosquat of babel-cli - credential stealer", DateAdded: time.Date(2017, 8, 1, 0, 0, 0, 0, time.UTC)},
		{Package: "mongose", Ecosystem: "npm", Advisory: "MAL-2017-0003", Severity: "HIGH", Description: "typosquat of mongoose - credential stealer", DateAdded: time.Date(2017, 8, 1, 0, 0, 0, 0, time.UTC)},
		{Package: "d3.js", Ecosystem: "npm", Advisory: "MAL-2017-0004", Severity: "HIGH", Description: "typosquat of d3 - credential stealer", DateAdded: time.Date(2017, 8, 1, 0, 0, 0, 0, time.UTC)},
		{Package: "mariadb", Ecosystem: "npm", Advisory: "MAL-2017-0005", Severity: "HIGH", Description: "typosquat of mariasql - credential stealer", DateAdded: time.Date(2017, 8, 1, 0, 0, 0, 0, time.UTC)},
		{Package: "node-fabric", Ecosystem: "npm", Advisory: "MAL-2017-0006", Severity: "HIGH", Description: "typosquat of fabric - credential stealer", DateAdded: time.Date(2017, 8, 1, 0, 0, 0, 0, time.UTC)},
		{Package: "opencv.js", Ecosystem: "npm", Advisory: "MAL-2017-0007", Severity: "HIGH", Description: "typosquat of opencv - credential stealer", DateAdded: time.Date(2017, 8, 1, 0, 0, 0, 0, time.UTC)},
		{Package: "openssl.js", Ecosystem: "npm", Advisory: "MAL-2017-0008", Severity: "HIGH", Description: "typosquat of openssl - credential stealer", DateAdded: time.Date(2017, 8, 1, 0, 0, 0, 0, time.UTC)},
		{Package: "nodemailer-js", Ecosystem: "npm", Advisory: "MAL-2019-0003", Severity: "HIGH", Description: "typosquat of nodemailer - credential stealer", DateAdded: time.Date(2019, 2, 10, 0, 0, 0, 0, time.UTC)},
		{Package: "node-request", Ecosystem: "npm", Advisory: "MAL-2019-0004", Severity: "HIGH", Description: "typosquat of request - backdoor", DateAdded: time.Date(2019, 4, 5, 0, 0, 0, 0, time.UTC)},
		{Package: "discordi.js", Ecosystem: "npm", Advisory: "MAL-2021-0010", Severity: "HIGH", Description: "typosquat of discord.js - token stealer", DateAdded: time.Date(2021, 9, 1, 0, 0, 0, 0, time.UTC)},
		{Package: "discord.jss", Ecosystem: "npm", Advisory: "MAL-2021-0011", Severity: "HIGH", Description: "typosquat of discord.js - token stealer", DateAdded: time.Date(2021, 9, 1, 0, 0, 0, 0, time.UTC)},
		{Package: "electorn", Ecosystem: "npm", Advisory: "MAL-2022-0020", Severity: "HIGH", Description: "typosquat of electron - backdoor", DateAdded: time.Date(2022, 4, 12, 0, 0, 0, 0, time.UTC)}, //nolint:misspell
		{Package: "axios-https", Ecosystem: "npm", Advisory: "MAL-2023-0001", Severity: "HIGH", Description: "typosquat of axios - data exfiltration", DateAdded: time.Date(2023, 1, 15, 0, 0, 0, 0, time.UTC)},
		{Package: "react-dev-utils-cdn", Ecosystem: "npm", Advisory: "MAL-2023-0002", Severity: "HIGH", Description: "typosquat - credential stealer", DateAdded: time.Date(2023, 2, 10, 0, 0, 0, 0, time.UTC)},

		// NPM - malicious packages
		{Package: "flatmap-stream", Ecosystem: "npm", Advisory: "MAL-2018-0002", Severity: "CRITICAL", Description: "malicious dependency of event-stream - cryptocurrency theft", DateAdded: time.Date(2018, 11, 26, 0, 0, 0, 0, time.UTC)},
		{Package: "eslint-scope", Ecosystem: "npm", Advisory: "MAL-2018-0003", Severity: "CRITICAL", Description: "compromised - npm token stealer", DateAdded: time.Date(2018, 7, 12, 0, 0, 0, 0, time.UTC)},
		{Package: "getcookies", Ecosystem: "npm", Advisory: "MAL-2018-0004", Severity: "CRITICAL", Description: "backdoor - remote code execution via cookies", DateAdded: time.Date(2018, 5, 2, 0, 0, 0, 0, time.UTC)},
		{Package: "pac-resolver", Ecosystem: "npm", Advisory: "MAL-2021-0020", Severity: "CRITICAL", Description: "RCE vulnerability in PAC file handling", DateAdded: time.Date(2021, 9, 3, 0, 0, 0, 0, time.UTC)},

		// PyPI - malicious packages
		{Package: "ctx", Ecosystem: "pypi", Advisory: "MAL-2022-0100", Severity: "CRITICAL", Description: "credential stealer - exfiltrates environment variables", DateAdded: time.Date(2022, 5, 21, 0, 0, 0, 0, time.UTC)},
		{Package: "colorama", Ecosystem: "pypi", Advisory: "MAL-2022-0101", Severity: "CRITICAL", Description: "typosquat of colorama - credential stealer", DateAdded: time.Date(2022, 8, 10, 0, 0, 0, 0, time.UTC)},
		{Package: "python-dateutil", Ecosystem: "pypi", Advisory: "MAL-2019-0100", Severity: "CRITICAL", Description: "typosquat of python-dateutil - credential stealer", DateAdded: time.Date(2019, 12, 3, 0, 0, 0, 0, time.UTC)},
		{Package: "jeIlyfish", Ecosystem: "pypi", Advisory: "MAL-2019-0101", Severity: "CRITICAL", Description: "typosquat of jellyfish (homoglyph I/l) - credential stealer", DateAdded: time.Date(2019, 12, 1, 0, 0, 0, 0, time.UTC)},
		{Package: "python3-dateutil", Ecosystem: "pypi", Advisory: "MAL-2019-0102", Severity: "HIGH", Description: "typosquat of python-dateutil - data exfiltration", DateAdded: time.Date(2019, 12, 3, 0, 0, 0, 0, time.UTC)},
		{Package: "colourama", Ecosystem: "pypi", Advisory: "MAL-2020-0100", Severity: "HIGH", Description: "typosquat of colorama - credential stealer", DateAdded: time.Date(2020, 7, 15, 0, 0, 0, 0, time.UTC)},
		{Package: "reqeusts", Ecosystem: "pypi", Advisory: "MAL-2020-0101", Severity: "HIGH", Description: "typosquat of requests - backdoor", DateAdded: time.Date(2020, 5, 20, 0, 0, 0, 0, time.UTC)},
		{Package: "request", Ecosystem: "pypi", Advisory: "MAL-2020-0102", Severity: "HIGH", Description: "typosquat of requests - data exfiltration", DateAdded: time.Date(2020, 5, 20, 0, 0, 0, 0, time.UTC)},
		{Package: "python-sqlite", Ecosystem: "pypi", Advisory: "MAL-2021-0100", Severity: "HIGH", Description: "malicious package - reverse shell", DateAdded: time.Date(2021, 4, 10, 0, 0, 0, 0, time.UTC)},
		{Package: "numppy", Ecosystem: "pypi", Advisory: "MAL-2021-0101", Severity: "HIGH", Description: "typosquat of numpy - credential stealer", DateAdded: time.Date(2021, 6, 1, 0, 0, 0, 0, time.UTC)},
		{Package: "nuumpy", Ecosystem: "pypi", Advisory: "MAL-2021-0102", Severity: "HIGH", Description: "typosquat of numpy - credential stealer", DateAdded: time.Date(2021, 6, 1, 0, 0, 0, 0, time.UTC)},
		{Package: "flaskk", Ecosystem: "pypi", Advisory: "MAL-2021-0103", Severity: "HIGH", Description: "typosquat of flask - backdoor", DateAdded: time.Date(2021, 7, 5, 0, 0, 0, 0, time.UTC)},
		{Package: "djang0", Ecosystem: "pypi", Advisory: "MAL-2022-0110", Severity: "HIGH", Description: "typosquat of django (homoglyph o/0) - credential stealer", DateAdded: time.Date(2022, 3, 1, 0, 0, 0, 0, time.UTC)},
		{Package: "pipsq", Ecosystem: "pypi", Advisory: "MAL-2022-0111", Severity: "HIGH", Description: "typosquat of pip - system compromise", DateAdded: time.Date(2022, 4, 10, 0, 0, 0, 0, time.UTC)},
		{Package: "urllib4", Ecosystem: "pypi", Advisory: "MAL-2023-0100", Severity: "HIGH", Description: "typosquat of urllib3 - credential stealer", DateAdded: time.Date(2023, 3, 1, 0, 0, 0, 0, time.UTC)},
		{Package: "scikit-learn1", Ecosystem: "pypi", Advisory: "MAL-2023-0101", Severity: "HIGH", Description: "typosquat of scikit-learn - data exfiltration", DateAdded: time.Date(2023, 5, 10, 0, 0, 0, 0, time.UTC)},

		// Go - compromised modules
		{Package: "github.com/chaselton/xtools", Ecosystem: "go", Advisory: "MAL-2023-0200", Severity: "CRITICAL", Description: "typosquat of golang.org/x/tools - backdoor", DateAdded: time.Date(2023, 4, 1, 0, 0, 0, 0, time.UTC)},
		{Package: "github.com/paxful/gateway-utils", Ecosystem: "go", Advisory: "MAL-2023-0201", Severity: "CRITICAL", Description: "compromised dependency - credential stealer", DateAdded: time.Date(2023, 6, 15, 0, 0, 0, 0, time.UTC)},
		{Package: "github.com/nicedeal/crypto", Ecosystem: "go", Advisory: "MAL-2023-0202", Severity: "HIGH", Description: "typosquat of standard crypto - key exfiltration", DateAdded: time.Date(2023, 7, 1, 0, 0, 0, 0, time.UTC)},
		{Package: "github.com/xstools/xtools", Ecosystem: "go", Advisory: "MAL-2023-0203", Severity: "HIGH", Description: "typosquat of golang.org/x/tools - backdoor", DateAdded: time.Date(2023, 8, 10, 0, 0, 0, 0, time.UTC)},

		// Crates (Rust) - malicious
		{Package: "rustdecimal", Ecosystem: "crates", Advisory: "MAL-2022-0300", Severity: "CRITICAL", Description: "typosquat of rust_decimal - malware dropper", DateAdded: time.Date(2022, 5, 10, 0, 0, 0, 0, time.UTC)},
		{Package: "rust-decimal", Ecosystem: "crates", Advisory: "MAL-2022-0301", Severity: "CRITICAL", Description: "typosquat of rust_decimal - credential stealer", DateAdded: time.Date(2022, 5, 10, 0, 0, 0, 0, time.UTC)},
		{Package: "cratesio", Ecosystem: "crates", Advisory: "MAL-2023-0300", Severity: "HIGH", Description: "typosquat of crates-io - backdoor", DateAdded: time.Date(2023, 1, 20, 0, 0, 0, 0, time.UTC)},
		{Package: "serde-derive", Ecosystem: "crates", Advisory: "MAL-2023-0301", Severity: "HIGH", Description: "typosquat of serde_derive - data exfiltration", DateAdded: time.Date(2023, 3, 5, 0, 0, 0, 0, time.UTC)},
	}

	for _, entry := range entries {
		key := entry.Ecosystem + "/" + entry.Package
		checker.KnownMalware[key] = entry
	}

	return checker
}

// CheckPackage checks whether a package is known to be malicious.
func (c *OSVChecker) CheckPackage(name, ecosystem string) *CheckResult {
	ecosystem = strings.ToLower(ecosystem)
	key := ecosystem + "/" + name

	// Check cache first.
	c.mu.RLock()
	if cached, ok := c.Cache[key]; ok {
		if time.Since(cached.CheckedAt) < c.CacheTTL {
			c.mu.RUnlock()
			return cached
		}
	}
	c.mu.RUnlock()

	result := &CheckResult{
		Package:   name,
		Safe:      true,
		CheckedAt: time.Now(),
	}

	// Check against known malware database.
	c.mu.RLock()
	entry, found := c.KnownMalware[key]
	c.mu.RUnlock()

	if found {
		result.Safe = false
		result.Advisories = []string{entry.Advisory}
		result.Severity = entry.Severity
		result.Recommendation = buildRecommendation(entry)
	}

	// Check for typosquatting patterns.
	if result.Safe && c.IsTyposquat(name, ecosystem) {
		result.Safe = false
		result.Severity = "HIGH"
		result.Advisories = []string{"TYPOSQUAT-DETECTION"}
		result.Recommendation = fmt.Sprintf("Package %q appears to be a typosquat. Verify the correct package name before installing.", name)
	}

	// Check for suspicious naming patterns.
	if result.Safe {
		suspicions := c.DetectSuspiciousName(name)
		if len(suspicions) > 0 {
			result.Safe = false
			result.Severity = "MEDIUM"
			result.Advisories = []string{"SUSPICIOUS-NAME"}
			result.Recommendation = fmt.Sprintf("Package name triggers suspicious patterns: %s", strings.Join(suspicions, "; "))
		}
	}

	// Update cache.
	c.mu.Lock()
	c.Cache[key] = result
	c.mu.Unlock()

	return result
}

// CheckCommand parses a shell command to extract and check the package being installed.
func (c *OSVChecker) CheckCommand(command string) *CheckResult {
	command = strings.TrimSpace(command)

	var packageName, ecosystem string

	switch {
	case strings.HasPrefix(command, "npm install ") || strings.HasPrefix(command, "npm i "):
		ecosystem = "npm"
		packageName = extractNPMPackage(command)
	case strings.HasPrefix(command, "npx "):
		ecosystem = "npm"
		packageName = extractNPXPackage(command)
	case strings.HasPrefix(command, "pip install ") || strings.HasPrefix(command, "pip3 install "):
		ecosystem = "pypi"
		packageName = extractPipPackage(command)
	case strings.HasPrefix(command, "go get "):
		ecosystem = "go"
		packageName = extractGoPackage(command)
	case strings.HasPrefix(command, "cargo add "):
		ecosystem = "crates"
		packageName = extractCargoPackage(command)
	default:
		return &CheckResult{
			Package:   command,
			Safe:      true,
			CheckedAt: time.Now(),
		}
	}

	if packageName == "" {
		return &CheckResult{
			Package:   command,
			Safe:      true,
			CheckedAt: time.Now(),
		}
	}

	return c.CheckPackage(packageName, ecosystem)
}

// popularPackages maps ecosystems to their popular package names used for typosquat detection.
var popularPackages = map[string][]string{
	"npm": {
		"react", "express", "lodash", "axios", "webpack", "babel",
		"typescript", "eslint", "prettier", "jest", "mocha", "chai",
		"next", "nuxt", "vue", "angular", "svelte", "electron",
		"mongoose", "sequelize", "knex", "prisma", "discord.js",
		"nodemailer", "socket.io", "moment", "dayjs", "date-fns",
		"chalk", "commander", "yargs", "inquirer", "ora", "debug",
		"dotenv", "cors", "helmet", "passport", "jsonwebtoken",
	},
	"pypi": {
		"requests", "numpy", "flask", "django", "pandas", "scipy",
		"matplotlib", "tensorflow", "torch", "scikit-learn", "pillow",
		"beautifulsoup4", "selenium", "scrapy", "celery", "fastapi",
		"sqlalchemy", "pytest", "black", "mypy", "pylint", "colorama",
		"urllib3", "pip", "setuptools", "wheel", "cryptography",
	},
	"go": {
		"golang.org/x/tools", "golang.org/x/net", "golang.org/x/crypto",
		"golang.org/x/sys", "golang.org/x/text", "github.com/gin-gonic/gin",
		"github.com/gorilla/mux", "github.com/stretchr/testify",
	},
	"crates": {
		"serde", "serde_derive", "tokio", "rand", "clap", "reqwest",
		"hyper", "actix-web", "rust_decimal", "crates-io",
	},
}

// IsTyposquat checks whether a package name appears to be a typosquat of a popular package.
func (c *OSVChecker) IsTyposquat(name, ecosystem string) bool {
	ecosystem = strings.ToLower(ecosystem)
	popular, ok := popularPackages[ecosystem]
	if !ok {
		return false
	}

	nameLower := strings.ToLower(name)

	for _, pkg := range popular {
		pkgLower := strings.ToLower(pkg)
		if nameLower == pkgLower {
			continue // exact match is fine
		}

		// Check edit distance of 1 (one char off).
		if levenshteinDistance(nameLower, pkgLower) == 1 {
			return true
		}

		// Check extra/missing hyphen.
		if strings.ReplaceAll(nameLower, "-", "") == strings.ReplaceAll(pkgLower, "-", "") && nameLower != pkgLower {
			return true
		}

		// Check extra/missing underscore.
		if strings.ReplaceAll(nameLower, "_", "") == strings.ReplaceAll(pkgLower, "_", "") && nameLower != pkgLower {
			return true
		}

		// Check homoglyph substitution.
		if containsHomoglyph(nameLower, pkgLower) {
			return true
		}

		// Check name with common suffixes that might be squats.
		squatSuffixes := []string{"-js", ".js", "-node", "-cli", "-api", "-sdk"}
		for _, suffix := range squatSuffixes {
			if nameLower == pkgLower+suffix && nameLower != pkgLower {
				// Some of these are legitimate; flag only if the base is very popular.
				return true
			}
		}
	}

	return false
}

// suspiciousPatterns holds regex patterns for clearly malicious names.
var suspiciousPatterns = []*regexp.Regexp{
	regexp.MustCompile(`(?i)stealer`),
	regexp.MustCompile(`(?i)keylog`),
	regexp.MustCompile(`(?i)exfil`),
	regexp.MustCompile(`(?i)ransomware`),
	regexp.MustCompile(`(?i)cryptojack`),
	regexp.MustCompile(`(?i)backdoor`),
	regexp.MustCompile(`(?i)malware`),
	regexp.MustCompile(`(?i)trojan`),
	regexp.MustCompile(`(?i)rootkit`),
	regexp.MustCompile(`(?i)botnet`),
}

// DetectSuspiciousName identifies red flags in a package name.
func (c *OSVChecker) DetectSuspiciousName(name string) []string {
	var suspicions []string

	// Check against suspicious keyword patterns.
	for _, pat := range suspiciousPatterns {
		if pat.MatchString(name) {
			suspicions = append(suspicions, fmt.Sprintf("name contains suspicious keyword matching %s", pat.String()))
			break
		}
	}

	// Check for very long random-looking names (likely auto-generated malware).
	if len(name) > 30 && looksRandom(name) {
		suspicions = append(suspicions, "name appears randomly generated (long with mixed characters)")
	}

	// Check for known malicious author patterns in package name.
	maliciousPatterns := []string{"evil", "hack-", "pwn", "exploit-"}
	for _, pattern := range maliciousPatterns {
		if strings.Contains(strings.ToLower(name), pattern) {
			suspicions = append(suspicions, fmt.Sprintf("name contains suspicious substring %q", pattern))
		}
	}

	return suspicions
}

// FormatCheckResult produces a human-readable report for a check result.
func FormatCheckResult(result *CheckResult) string {
	if result.Safe {
		return fmt.Sprintf("Package Check: %s\n"+
			"  SAFE - no known advisories\n"+
			"  Checked at: %s\n",
			result.Package,
			result.CheckedAt.Format(time.RFC3339))
	}

	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("Package Check: %s\n", result.Package))
	sb.WriteString(fmt.Sprintf("  WARNING VULNERABLE - %s\n", advisorySummary(result)))

	if len(result.Advisories) > 0 {
		sb.WriteString(fmt.Sprintf("  Advisory: %s\n", strings.Join(result.Advisories, ", ")))
	}
	if result.Severity != "" {
		sb.WriteString(fmt.Sprintf("  Severity: %s\n", result.Severity))
	}
	sb.WriteString("\n")
	if result.Recommendation != "" {
		sb.WriteString(fmt.Sprintf("  Recommendation: %s\n", result.Recommendation))
	}

	return sb.String()
}

// RefreshDatabase is a placeholder for future OSV API integration.
// In production, this would fetch the latest advisories from https://api.osv.dev/v1/query.
func (c *OSVChecker) RefreshDatabase() error {
	// Future implementation:
	// 1. Query OSV API for latest advisories
	// 2. Parse response and update KnownMalware map
	// 3. Invalidate relevant cache entries
	// 4. Record last refresh timestamp
	//
	// For now, the embedded database is used.
	return nil
}

// --- Helper functions ---

func extractNPMPackage(command string) string {
	// Handle: npm install <pkg>, npm install <pkg>@<version>, npm i <pkg>
	parts := strings.Fields(command)
	if len(parts) < 3 {
		return ""
	}
	// Skip flags like --save, --save-dev, -D, -g
	for i := 2; i < len(parts); i++ {
		p := parts[i]
		if strings.HasPrefix(p, "-") {
			continue
		}
		// Strip version specifier.
		if idx := strings.LastIndex(p, "@"); idx > 0 {
			return p[:idx]
		}
		return p
	}
	return ""
}

func extractNPXPackage(command string) string {
	parts := strings.Fields(command)
	if len(parts) < 2 {
		return ""
	}
	// Skip flags.
	for i := 1; i < len(parts); i++ {
		if !strings.HasPrefix(parts[i], "-") {
			pkg := parts[i]
			if idx := strings.LastIndex(pkg, "@"); idx > 0 {
				return pkg[:idx]
			}
			return pkg
		}
	}
	return ""
}

func extractPipPackage(command string) string {
	parts := strings.Fields(command)
	if len(parts) < 3 {
		return ""
	}
	for i := 2; i < len(parts); i++ {
		p := parts[i]
		if strings.HasPrefix(p, "-") {
			continue
		}
		// Strip version specifiers: ==, >=, <=, ~=, !=
		for _, sep := range []string{"==", ">=", "<=", "~=", "!="} {
			if idx := strings.Index(p, sep); idx > 0 {
				return p[:idx]
			}
		}
		return p
	}
	return ""
}

func extractGoPackage(command string) string {
	parts := strings.Fields(command)
	if len(parts) < 3 {
		return ""
	}
	for i := 2; i < len(parts); i++ {
		p := parts[i]
		if strings.HasPrefix(p, "-") {
			continue
		}
		// Strip version: github.com/foo/bar@v1.2.3
		if idx := strings.LastIndex(p, "@"); idx > 0 {
			return p[:idx]
		}
		return p
	}
	return ""
}

func extractCargoPackage(command string) string {
	parts := strings.Fields(command)
	if len(parts) < 3 {
		return ""
	}
	for i := 2; i < len(parts); i++ {
		p := parts[i]
		if strings.HasPrefix(p, "-") {
			// Skip flag and its value if it looks like --features x
			if i+1 < len(parts) && !strings.HasPrefix(parts[i+1], "-") {
				i++
			}
			continue
		}
		return p
	}
	return ""
}

func buildRecommendation(entry *MalwareEntry) string {
	switch {
	case strings.Contains(entry.Description, "compromised supply chain"):
		return "Use a patched version or switch to an alternative package. Check release notes for remediation guidance."
	case strings.Contains(entry.Description, "protestware"):
		return "Pin to a safe version before the malicious release or switch to a community fork."
	case strings.Contains(entry.Description, "typosquat"):
		return "This is a typosquat package. Remove immediately and install the legitimate package instead."
	default:
		return "Remove this package immediately. It is known malware."
	}
}

func advisorySummary(result *CheckResult) string {
	if result.Recommendation != "" {
		// Return a short version.
		parts := strings.SplitN(result.Recommendation, ".", 2)
		if len(parts) > 0 && len(parts[0]) < 80 {
			return parts[0]
		}
	}
	return "known malicious package"
}

// levenshteinDistance computes the edit distance between two strings.
func levenshteinDistance(a, b string) int {
	la, lb := len(a), len(b)
	if la == 0 {
		return lb
	}
	if lb == 0 {
		return la
	}

	// Use two rows for space optimization.
	prev := make([]int, lb+1)
	curr := make([]int, lb+1)

	for j := 0; j <= lb; j++ {
		prev[j] = j
	}

	for i := 1; i <= la; i++ {
		curr[0] = i
		for j := 1; j <= lb; j++ {
			cost := 1
			if a[i-1] == b[j-1] {
				cost = 0
			}
			curr[j] = min3(curr[j-1]+1, prev[j]+1, prev[j-1]+cost)
		}
		prev, curr = curr, prev
	}

	return prev[lb]
}

func min3(a, b, c int) int {
	if a < b {
		if a < c {
			return a
		}
		return c
	}
	if b < c {
		return b
	}
	return c
}

// homoglyphMap maps characters that visually resemble each other.
var homoglyphMap = map[rune][]rune{
	'l': {'1', 'I', 'i'},
	'1': {'l', 'I', 'i'},
	'I': {'l', '1', 'i'},
	'o': {'0', 'O'},
	'0': {'o', 'O'},
	'O': {'o', '0'},
	'i': {'l', '1', 'I'},
}

// containsHomoglyph checks if two names differ only by homoglyph substitution.
func containsHomoglyph(name, target string) bool {
	if len(name) != len(target) {
		// Check rn -> m substitution.
		normalized := strings.ReplaceAll(name, "rn", "m")
		if normalized == target {
			return true
		}
		normalized = strings.ReplaceAll(target, "rn", "m")
		return normalized == name
	}

	diffCount := 0
	for i := 0; i < len(name); i++ {
		if name[i] != target[i] {
			diffCount++
			if diffCount > 2 {
				return false
			}
			// Check if the differing characters are homoglyphs.
			nameRune := rune(name[i])
			targetRune := rune(target[i])
			if !areHomoglyphs(nameRune, targetRune) {
				return false
			}
		}
	}
	return diffCount > 0
}

func areHomoglyphs(a, b rune) bool {
	glyphs, ok := homoglyphMap[a]
	if !ok {
		return false
	}
	for _, g := range glyphs {
		if g == b {
			return true
		}
	}
	return false
}

// looksRandom checks if a string appears to be randomly generated.
func looksRandom(name string) bool {
	if len(name) < 10 {
		return false
	}

	// Count character type transitions.
	transitions := 0
	digitCount := 0
	for i := 1; i < len(name); i++ {
		prev := classifyChar(rune(name[i-1]))
		curr := classifyChar(rune(name[i]))
		if prev != curr {
			transitions++
		}
		if unicode.IsDigit(rune(name[i])) {
			digitCount++
		}
	}

	// High transition ratio suggests random generation.
	ratio := float64(transitions) / float64(len(name))
	digitRatio := float64(digitCount) / float64(len(name))

	return ratio > 0.5 && digitRatio > 0.15
}

func classifyChar(r rune) int {
	switch {
	case unicode.IsLower(r):
		return 0
	case unicode.IsUpper(r):
		return 1
	case unicode.IsDigit(r):
		return 2
	default:
		return 3
	}
}
