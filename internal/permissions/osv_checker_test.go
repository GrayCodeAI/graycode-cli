package permissions

import (
	"strings"
	"testing"
	"time"
)

func TestNewOSVChecker(t *testing.T) {
	checker := NewOSVChecker()

	if checker == nil {
		t.Fatal("NewOSVChecker returned nil")
	}
	if len(checker.KnownMalware) < 50 {
		t.Errorf("expected at least 50 malware entries, got %d", len(checker.KnownMalware))
	}
	if checker.Cache == nil {
		t.Fatal("Cache map not initialized")
	}
	if checker.CacheTTL != 1*time.Hour {
		t.Errorf("expected CacheTTL of 1h, got %v", checker.CacheTTL)
	}
}

func TestCheckPackage_KnownMalware(t *testing.T) {
	checker := NewOSVChecker()

	tests := []struct {
		name      string
		ecosystem string
		wantSafe  bool
		severity  string
	}{
		{"event-stream", "npm", false, "CRITICAL"},
		{"ua-parser-js", "npm", false, "CRITICAL"},
		{"colors", "npm", false, "HIGH"},
		{"faker", "npm", false, "HIGH"},
		{"node-ipc", "npm", false, "CRITICAL"},
		{"@pnpm/exe", "npm", false, "CRITICAL"},
		{"crossenv", "npm", false, "CRITICAL"},
		{"ctx", "pypi", false, "CRITICAL"},
		{"jeIlyfish", "pypi", false, "CRITICAL"},
		{"rustdecimal", "crates", false, "CRITICAL"},
		{"github.com/chaselton/xtools", "go", false, "CRITICAL"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := checker.CheckPackage(tt.name, tt.ecosystem)
			if result.Safe != tt.wantSafe {
				t.Errorf("CheckPackage(%q, %q) Safe = %v, want %v", tt.name, tt.ecosystem, result.Safe, tt.wantSafe)
			}
			if result.Severity != tt.severity {
				t.Errorf("CheckPackage(%q, %q) Severity = %q, want %q", tt.name, tt.ecosystem, result.Severity, tt.severity)
			}
			if len(result.Advisories) == 0 {
				t.Errorf("CheckPackage(%q, %q) expected advisories", tt.name, tt.ecosystem)
			}
		})
	}
}

func TestCheckPackage_SafePackages(t *testing.T) {
	checker := NewOSVChecker()

	tests := []struct {
		name      string
		ecosystem string
	}{
		{"express", "npm"},
		{"lodash", "npm"},
		{"requests", "pypi"},
		{"numpy", "pypi"},
		{"github.com/gin-gonic/gin", "go"},
		{"serde", "crates"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := checker.CheckPackage(tt.name, tt.ecosystem)
			if !result.Safe {
				t.Errorf("CheckPackage(%q, %q) Safe = false, expected true", tt.name, tt.ecosystem)
			}
		})
	}
}

func TestCheckPackage_CaseInsensitiveEcosystem(t *testing.T) {
	checker := NewOSVChecker()

	result := checker.CheckPackage("event-stream", "NPM")
	if result.Safe {
		t.Error("expected event-stream to be flagged with uppercase ecosystem")
	}

	result = checker.CheckPackage("ctx", "PyPI")
	if result.Safe {
		t.Error("expected ctx to be flagged with mixed-case ecosystem")
	}
}

func TestCheckPackage_Cache(t *testing.T) {
	checker := NewOSVChecker()
	checker.CacheTTL = 5 * time.Minute

	// First call populates cache.
	result1 := checker.CheckPackage("event-stream", "npm")
	if result1.Safe {
		t.Fatal("expected unsafe")
	}

	// Second call should hit cache.
	result2 := checker.CheckPackage("event-stream", "npm")
	if result2.CheckedAt != result1.CheckedAt {
		t.Error("expected cached result with same CheckedAt timestamp")
	}
}

func TestCheckCommand_NPM(t *testing.T) {
	checker := NewOSVChecker()

	tests := []struct {
		command  string
		wantSafe bool
	}{
		{"npm install event-stream", false},
		{"npm i event-stream", false},
		{"npm install --save event-stream", false},
		{"npm install express", true},
		{"npm install colors", false},
		{"npm install @pnpm/exe", false},
	}

	for _, tt := range tests {
		t.Run(tt.command, func(t *testing.T) {
			result := checker.CheckCommand(tt.command)
			if result.Safe != tt.wantSafe {
				t.Errorf("CheckCommand(%q) Safe = %v, want %v", tt.command, result.Safe, tt.wantSafe)
			}
		})
	}
}

func TestCheckCommand_NPX(t *testing.T) {
	checker := NewOSVChecker()

	result := checker.CheckCommand("npx crossenv")
	if result.Safe {
		t.Error("expected npx crossenv to be unsafe")
	}
}

func TestCheckCommand_Pip(t *testing.T) {
	checker := NewOSVChecker()

	tests := []struct {
		command  string
		wantSafe bool
	}{
		{"pip install ctx", false},
		{"pip3 install ctx", false},
		{"pip install requests", true},
		{"pip install numppy==1.0.0", false},
	}

	for _, tt := range tests {
		t.Run(tt.command, func(t *testing.T) {
			result := checker.CheckCommand(tt.command)
			if result.Safe != tt.wantSafe {
				t.Errorf("CheckCommand(%q) Safe = %v, want %v", tt.command, result.Safe, tt.wantSafe)
			}
		})
	}
}

func TestCheckCommand_GoGet(t *testing.T) {
	checker := NewOSVChecker()

	result := checker.CheckCommand("go get github.com/chaselton/xtools")
	if result.Safe {
		t.Error("expected go get malicious module to be unsafe")
	}

	result = checker.CheckCommand("go get github.com/gin-gonic/gin")
	if !result.Safe {
		t.Errorf("expected github.com/gin-gonic/gin to be safe, got advisories: %v", result.Advisories)
	}
}

func TestCheckCommand_Cargo(t *testing.T) {
	checker := NewOSVChecker()

	result := checker.CheckCommand("cargo add rustdecimal")
	if result.Safe {
		t.Error("expected cargo add rustdecimal to be unsafe")
	}

	result = checker.CheckCommand("cargo add serde")
	if !result.Safe {
		t.Error("expected cargo add serde to be safe")
	}
}

func TestCheckCommand_UnknownCommand(t *testing.T) {
	checker := NewOSVChecker()

	result := checker.CheckCommand("docker run ubuntu")
	if !result.Safe {
		t.Error("expected unknown command to be safe")
	}
}

func TestCheckCommand_VersionStripping(t *testing.T) {
	checker := NewOSVChecker()

	// npm with version
	result := checker.CheckCommand("npm install event-stream@3.3.6")
	if result.Safe {
		t.Error("expected event-stream with version to be flagged")
	}

	// pip with version
	result = checker.CheckCommand("pip install ctx==0.1.2")
	if result.Safe {
		t.Error("expected ctx with version to be flagged")
	}

	// go with version
	result = checker.CheckCommand("go get github.com/chaselton/xtools@v0.1.0")
	if result.Safe {
		t.Error("expected go module with version to be flagged")
	}
}

func TestIsTyposquat(t *testing.T) {
	checker := NewOSVChecker()

	tests := []struct {
		name      string
		ecosystem string
		want      bool
	}{
		// One character off.
		{"reacr", "npm", true},
		{"reavt", "npm", true},
		{"exprss", "npm", true},
		{"requets", "pypi", true},
		{"numpyy", "pypi", true},

		// Extra/missing hyphen.
		{"type-script", "npm", true},
		{"date-fns", "npm", false}, // actual package

		// Legitimate packages should not be flagged.
		{"react", "npm", false},
		{"express", "npm", false},
		{"lodash", "npm", false},
		{"requests", "pypi", false},
		{"numpy", "pypi", false},
		{"flask", "pypi", false},
	}

	for _, tt := range tests {
		t.Run(tt.name+"_"+tt.ecosystem, func(t *testing.T) {
			got := checker.IsTyposquat(tt.name, tt.ecosystem)
			if got != tt.want {
				t.Errorf("IsTyposquat(%q, %q) = %v, want %v", tt.name, tt.ecosystem, got, tt.want)
			}
		})
	}
}

func TestIsTyposquat_Homoglyphs(t *testing.T) {
	checker := NewOSVChecker()

	// 'l' vs '1' substitution.
	result := checker.IsTyposquat("f1ask", "pypi")
	if !result {
		t.Error("expected f1ask to be detected as typosquat of flask")
	}

	// 'o' vs '0' substitution.
	result = checker.IsTyposquat("t0kio", "crates")
	if !result {
		t.Error("expected t0kio to be detected as typosquat of tokio")
	}
}

func TestDetectSuspiciousName(t *testing.T) {
	checker := NewOSVChecker()

	tests := []struct {
		name       string
		wantFlags  bool
		wantSubstr string
	}{
		{"credential-stealer-v2", true, "suspicious keyword"},
		{"keylogger-npm", true, "suspicious keyword"},
		{"data-exfiltrator", true, "suspicious keyword"},
		{"ransomware-helper", true, "suspicious keyword"},
		{"evil-package", true, "suspicious substring"},
		{"hack-tool-utils", true, "suspicious substring"},
		{"express", false, ""},
		{"lodash", false, ""},
		{"react-dom", false, ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			suspicions := checker.DetectSuspiciousName(tt.name)
			if tt.wantFlags && len(suspicions) == 0 {
				t.Errorf("DetectSuspiciousName(%q) returned no flags, expected some", tt.name)
			}
			if !tt.wantFlags && len(suspicions) > 0 {
				t.Errorf("DetectSuspiciousName(%q) returned flags %v, expected none", tt.name, suspicions)
			}
			if tt.wantSubstr != "" && len(suspicions) > 0 {
				found := false
				for _, s := range suspicions {
					if strings.Contains(s, tt.wantSubstr) {
						found = true
						break
					}
				}
				if !found {
					t.Errorf("DetectSuspiciousName(%q) flags %v don't contain %q", tt.name, suspicions, tt.wantSubstr)
				}
			}
		})
	}
}

func TestDetectSuspiciousName_RandomLooking(t *testing.T) {
	checker := NewOSVChecker()

	// A genuinely random-looking long name.
	result := checker.DetectSuspiciousName("a1b2c3d4e5f6g7h8i9j0k1l2m3n4o5p6")
	if len(result) == 0 {
		t.Error("expected random-looking name to be flagged")
	}
}

func TestFormatResult_Safe(t *testing.T) {
	result := &CheckResult{
		Package:   "express",
		Safe:      true,
		CheckedAt: time.Date(2024, 1, 15, 10, 0, 0, 0, time.UTC),
	}

	output := FormatCheckResult(result)
	if !strings.Contains(output, "express") {
		t.Error("output should contain package name")
	}
	if !strings.Contains(output, "SAFE") {
		t.Error("output should indicate SAFE")
	}
}

func TestFormatResult_Unsafe(t *testing.T) {
	result := &CheckResult{
		Package:        "ua-parser-js",
		Safe:           false,
		Advisories:     []string{"MAL-2021-0001"},
		Severity:       "CRITICAL",
		Recommendation: "Use version >= 0.7.30 or switch to alternative",
		CheckedAt:      time.Date(2024, 1, 15, 10, 0, 0, 0, time.UTC),
	}

	output := FormatCheckResult(result)
	if !strings.Contains(output, "ua-parser-js") {
		t.Error("output should contain package name")
	}
	if !strings.Contains(output, "VULNERABLE") {
		t.Error("output should indicate VULNERABLE")
	}
	if !strings.Contains(output, "MAL-2021-0001") {
		t.Error("output should contain advisory ID")
	}
	if !strings.Contains(output, "CRITICAL") {
		t.Error("output should contain severity")
	}
	if !strings.Contains(output, "Recommendation") {
		t.Error("output should contain recommendation")
	}
}

func TestRefreshDatabase(t *testing.T) {
	checker := NewOSVChecker()

	err := checker.RefreshDatabase()
	if err != nil {
		t.Errorf("RefreshDatabase() returned error: %v", err)
	}
}

func TestCheckPackage_CacheExpiration(t *testing.T) {
	checker := NewOSVChecker()
	checker.CacheTTL = 1 * time.Millisecond

	// First call.
	result1 := checker.CheckPackage("express", "npm")
	firstChecked := result1.CheckedAt

	// Wait for cache to expire.
	time.Sleep(5 * time.Millisecond)

	// Second call should not use cache.
	result2 := checker.CheckPackage("express", "npm")
	if result2.CheckedAt.Equal(firstChecked) {
		t.Error("expected fresh result after cache expiration")
	}
}

func TestLevenshteinDistance(t *testing.T) {
	tests := []struct {
		a, b string
		want int
	}{
		{"", "", 0},
		{"a", "", 1},
		{"", "a", 1},
		{"kitten", "sitting", 3},
		{"react", "reacr", 1},
		{"express", "exprss", 1},
		{"lodash", "lodash", 0},
	}

	for _, tt := range tests {
		t.Run(tt.a+"_"+tt.b, func(t *testing.T) {
			got := levenshteinDistance(tt.a, tt.b)
			if got != tt.want {
				t.Errorf("levenshteinDistance(%q, %q) = %d, want %d", tt.a, tt.b, got, tt.want)
			}
		})
	}
}

func TestCheckCommand_EmptyAndEdgeCases(t *testing.T) {
	checker := NewOSVChecker()

	// Empty command.
	result := checker.CheckCommand("")
	if !result.Safe {
		t.Error("expected empty command to be safe")
	}

	// Just whitespace.
	result = checker.CheckCommand("   ")
	if !result.Safe {
		t.Error("expected whitespace command to be safe")
	}

	// Incomplete commands.
	result = checker.CheckCommand("npm install")
	if !result.Safe {
		t.Error("expected incomplete npm install to be safe (no package)")
	}
}

func TestMalwareEntryEcosystems(t *testing.T) {
	checker := NewOSVChecker()

	ecosystems := map[string]int{"npm": 0, "pypi": 0, "go": 0, "crates": 0}

	for _, entry := range checker.KnownMalware {
		ecosystems[entry.Ecosystem]++
	}

	for eco, count := range ecosystems {
		if count == 0 {
			t.Errorf("expected at least one entry for ecosystem %q", eco)
		}
	}
}

func TestCheckPackage_Recommendation(t *testing.T) {
	checker := NewOSVChecker()

	// Compromised supply chain should recommend patched version.
	result := checker.CheckPackage("event-stream", "npm")
	if !strings.Contains(result.Recommendation, "patched") && !strings.Contains(result.Recommendation, "alternative") {
		t.Errorf("expected supply chain recommendation, got: %s", result.Recommendation)
	}

	// Protestware should recommend pinning.
	result = checker.CheckPackage("colors", "npm")
	if !strings.Contains(result.Recommendation, "safe version") && !strings.Contains(result.Recommendation, "fork") {
		t.Errorf("expected protestware recommendation, got: %s", result.Recommendation)
	}

	// Typosquat should recommend removal.
	result = checker.CheckPackage("crossenv", "npm")
	if !strings.Contains(result.Recommendation, "typosquat") && !strings.Contains(result.Recommendation, "Remove") {
		t.Errorf("expected typosquat recommendation, got: %s", result.Recommendation)
	}
}

func TestOSVCheckerConcurrentAccess(t *testing.T) {
	checker := NewOSVChecker()

	done := make(chan struct{})
	for i := 0; i < 50; i++ {
		go func(i int) {
			defer func() { done <- struct{}{} }()
			if i%2 == 0 {
				checker.CheckPackage("event-stream", "npm")
			} else {
				checker.CheckPackage("express", "npm")
			}
		}(i)
	}

	for i := 0; i < 50; i++ {
		<-done
	}
}
