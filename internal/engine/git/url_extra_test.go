package git

import (
	"testing"
)

func TestParseRemoteURL_InvalidSSH(t *testing.T) {
	provider, owner, repo := parseRemoteURL("git@github.com")
	if provider != "" || owner != "" || repo != "" {
		t.Errorf("expected empty results for invalid SSH URL, got %q %q %q", provider, owner, repo)
	}
}

func TestParseRemoteURL_InvalidSSHPath(t *testing.T) {
	provider, owner, repo := parseRemoteURL("git@github.com:onlyonepart")
	if provider != "" || owner != "" || repo != "" {
		t.Errorf("expected empty results for invalid SSH path, got %q %q %q", provider, owner, repo)
	}
}

func TestParseRemoteURL_InvalidHTTPS(t *testing.T) {
	provider, owner, repo := parseRemoteURL("https://github.com/onlyonepart")
	if provider != "" || owner != "" || repo != "" {
		t.Errorf("expected empty results for invalid HTTPS URL, got %q %q %q", provider, owner, repo)
	}
}

func TestParseRemoteURL_UnknownFormat(t *testing.T) {
	// For "ftp://github.com/owner/repo", since it has "://", strings.Contains(url, "://") is true.
	// The function will split by "/" which results in parts = ["ftp:", "", "github.com", "owner", "repo"].
	// len(parts) is 5, so it returns provider="github", owner="owner", repo="repo".
	// Let's test a URL with less than 5 parts when split by "/"
	provider, owner, repo := parseRemoteURL("ftp://github.com/owner")
	if provider != "" || owner != "" || repo != "" {
		t.Errorf("expected empty results for unknown format with few parts, got %q %q %q", provider, owner, repo)
	}
}

func TestDetectProviderFromHost_Extra(t *testing.T) {
	tests := []struct {
		host     string
		expected string
	}{
		{"gitlab.com", "gitlab"},
		{"bitbucket.org", "bitbucket"},
		{"github.com", "github"},
		{"custom.com", "github"}, // defaults to github
	}
	for _, tt := range tests {
		result := detectProviderFromHost(tt.host)
		if result != tt.expected {
			t.Errorf("detectProviderFromHost(%q) = %q, want %q", tt.host, result, tt.expected)
		}
	}
}
