package engine

import (
	"strings"
	"testing"
	"time"
)

func TestNewGitProvider(t *testing.T) {
	gp := NewGitProvider("github", "token123", "octocat", "hello-world")

	if gp.Type != "github" {
		t.Errorf("expected Type 'github', got %q", gp.Type)
	}
	if gp.Token != "token123" {
		t.Errorf("expected Token 'token123', got %q", gp.Token)
	}
	if gp.Owner != "octocat" {
		t.Errorf("expected Owner 'octocat', got %q", gp.Owner)
	}
	if gp.Repo != "hello-world" {
		t.Errorf("expected Repo 'hello-world', got %q", gp.Repo)
	}
}

func TestFormatIssues(t *testing.T) {
	issues := []GitIssue{
		{Number: 1, Title: "Bug in login", State: "open", Labels: []string{"bug"}, Author: "alice"},
		{Number: 2, Title: "Add dark mode", State: "closed", Labels: []string{"feature"}, Author: "bob"},
		{Number: 3, Title: "Docs update", State: "open", Author: "carol"},
	}

	result := FormatIssues(issues)

	if !strings.Contains(result, "Issues") {
		t.Error("expected header 'Issues'")
	}
	if !strings.Contains(result, "○ #1 Bug in login [bug] (@alice)") {
		t.Errorf("expected open issue format, got:\n%s", result)
	}
	if !strings.Contains(result, "✓ #2 Add dark mode [feature] (@bob)") {
		t.Errorf("expected closed issue format, got:\n%s", result)
	}
	if !strings.Contains(result, "○ #3 Docs update (@carol)") {
		t.Errorf("expected issue without labels, got:\n%s", result)
	}
}

func TestFormatIssuesEmpty(t *testing.T) {
	result := FormatIssues(nil)
	if result != "No issues found." {
		t.Errorf("expected 'No issues found.', got %q", result)
	}
}

func TestFormatPRs(t *testing.T) {
	prs := []PullRequest{
		{Number: 10, Title: "Add auth", Branch: "feature/auth", BaseBranch: "main", State: "open", Draft: false},
		{Number: 11, Title: "WIP: Refactor", Branch: "refactor/core", BaseBranch: "main", State: "open", Draft: true},
		{Number: 9, Title: "Fix typo", Branch: "fix/typo", BaseBranch: "main", State: "merged"},
		{Number: 8, Title: "Old PR", Branch: "old", BaseBranch: "main", State: "closed"},
	}

	result := FormatPRs(prs)

	if !strings.Contains(result, "Pull Requests") {
		t.Error("expected header 'Pull Requests'")
	}
	if !strings.Contains(result, "○ #10 Add auth (feature/auth → main)") {
		t.Errorf("expected open PR format, got:\n%s", result)
	}
	if !strings.Contains(result, "○ #11 WIP: Refactor [draft] (refactor/core → main)") {
		t.Errorf("expected draft PR format, got:\n%s", result)
	}
	if !strings.Contains(result, "✓ #9 Fix typo (fix/typo → main)") {
		t.Errorf("expected merged PR format, got:\n%s", result)
	}
	if !strings.Contains(result, "✗ #8 Old PR (old → main)") {
		t.Errorf("expected closed PR format, got:\n%s", result)
	}
}

func TestFormatPRsEmpty(t *testing.T) {
	result := FormatPRs(nil)
	if result != "No pull requests found." {
		t.Errorf("expected 'No pull requests found.', got %q", result)
	}
}

func TestFormatCIStatus(t *testing.T) {
	status := &CIStatus{
		State: "failure",
		URL:   "feature/auth",
		Checks: []CICheck{
			{Name: "lint", Status: "success", Duration: 12 * time.Second},
			{Name: "test", Status: "success", Duration: 45 * time.Second},
			{Name: "build", Status: "failure", Duration: 23 * time.Second},
			{Name: "deploy", Status: "pending", Duration: 0},
		},
	}

	result := FormatCIStatus(status)

	if !strings.Contains(result, "CI Status: feature/auth") {
		t.Errorf("expected CI status header, got:\n%s", result)
	}
	if !strings.Contains(result, "✓ lint (12s)") {
		t.Errorf("expected lint check, got:\n%s", result)
	}
	if !strings.Contains(result, "✓ test (45s)") {
		t.Errorf("expected test check, got:\n%s", result)
	}
	if !strings.Contains(result, "✗ build (failed after 23s)") {
		t.Errorf("expected build check, got:\n%s", result)
	}
	if !strings.Contains(result, "○ deploy (pending)") {
		t.Errorf("expected deploy check, got:\n%s", result)
	}
	if !strings.Contains(result, "Overall: FAILING (1/4 checks failed)") {
		t.Errorf("expected overall status, got:\n%s", result)
	}
}

func TestFormatCIStatusAllPassing(t *testing.T) {
	status := &CIStatus{
		State: "success",
		URL:   "main",
		Checks: []CICheck{
			{Name: "lint", Status: "success", Duration: 5 * time.Second},
			{Name: "test", Status: "success", Duration: 30 * time.Second},
		},
	}

	result := FormatCIStatus(status)
	if !strings.Contains(result, "Overall: PASSING (2/2 checks passed)") {
		t.Errorf("expected passing status, got:\n%s", result)
	}
}

func TestFormatCIStatusNil(t *testing.T) {
	result := FormatCIStatus(nil)
	if result != "No CI status available." {
		t.Errorf("expected nil handling, got %q", result)
	}
}

func TestParseRemoteURL(t *testing.T) {
	tests := []struct {
		url      string
		provider string
		owner    string
		repo     string
	}{
		{
			url:      "git@github.com:octocat/hello-world.git",
			provider: "github",
			owner:    "octocat",
			repo:     "hello-world",
		},
		{
			url:      "https://github.com/octocat/hello-world.git",
			provider: "github",
			owner:    "octocat",
			repo:     "hello-world",
		},
		{
			url:      "git@gitlab.com:group/project.git",
			provider: "gitlab",
			owner:    "group",
			repo:     "project",
		},
		{
			url:      "https://gitlab.com/group/project",
			provider: "gitlab",
			owner:    "group",
			repo:     "project",
		},
		{
			url:      "git@bitbucket.org:team/repo.git",
			provider: "bitbucket",
			owner:    "team",
			repo:     "repo",
		},
		{
			url:      "https://bitbucket.org/team/repo.git",
			provider: "bitbucket",
			owner:    "team",
			repo:     "repo",
		},
	}

	for _, tt := range tests {
		t.Run(tt.url, func(t *testing.T) {
			provider, owner, repo := parseRemoteURL(tt.url)
			if provider != tt.provider {
				t.Errorf("parseRemoteURL(%q) provider = %q, want %q", tt.url, provider, tt.provider)
			}
			if owner != tt.owner {
				t.Errorf("parseRemoteURL(%q) owner = %q, want %q", tt.url, owner, tt.owner)
			}
			if repo != tt.repo {
				t.Errorf("parseRemoteURL(%q) repo = %q, want %q", tt.url, repo, tt.repo)
			}
		})
	}
}

func TestParseGitConfig(t *testing.T) {
	config := `[core]
	repositoryformatversion = 0
	filemode = true
[remote "origin"]
	url = git@github.com:myorg/myrepo.git
	fetch = +refs/heads/*:refs/remotes/origin/*
[branch "main"]
	remote = origin
	merge = refs/heads/main
`

	provider, owner, repo := parseGitConfig(config)
	if provider != "github" {
		t.Errorf("expected provider 'github', got %q", provider)
	}
	if owner != "myorg" {
		t.Errorf("expected owner 'myorg', got %q", owner)
	}
	if repo != "myrepo" {
		t.Errorf("expected repo 'myrepo', got %q", repo)
	}
}

func TestParseGitConfigGitLab(t *testing.T) {
	config := `[remote "origin"]
	url = https://gitlab.com/team/project.git
	fetch = +refs/heads/*:refs/remotes/origin/*
`

	provider, owner, repo := parseGitConfig(config)
	if provider != "gitlab" {
		t.Errorf("expected provider 'gitlab', got %q", provider)
	}
	if owner != "team" {
		t.Errorf("expected owner 'team', got %q", owner)
	}
	if repo != "project" {
		t.Errorf("expected repo 'project', got %q", repo)
	}
}

func TestExtractNumberFromURL(t *testing.T) {
	tests := []struct {
		url    string
		number int
	}{
		{"https://github.com/owner/repo/issues/42", 42},
		{"https://github.com/owner/repo/pull/7", 7},
		{"https://gitlab.com/owner/repo/-/issues/123", 123},
		{"", 0},
		{"not-a-url", 0},
	}

	for _, tt := range tests {
		t.Run(tt.url, func(t *testing.T) {
			n := extractNumberFromURL(tt.url)
			if n != tt.number {
				t.Errorf("extractNumberFromURL(%q) = %d, want %d", tt.url, n, tt.number)
			}
		})
	}
}

func TestGitProviderFormatDuration(t *testing.T) {
	tests := []struct {
		d    time.Duration
		want string
	}{
		{0, "0s"},
		{12 * time.Second, "12s"},
		{45 * time.Second, "45s"},
		{2 * time.Minute, "2m"},
		{time.Minute + 30*time.Second, "1m30s"},
		{5*time.Minute + 15*time.Second, "5m15s"},
	}

	for _, tt := range tests {
		t.Run(tt.want, func(t *testing.T) {
			got := formatCIDuration(tt.d)
			if got != tt.want {
				t.Errorf("formatCIDuration(%v) = %q, want %q", tt.d, got, tt.want)
			}
		})
	}
}

func TestSplitJSONObjects(t *testing.T) {
	input := `[{"a": 1}, {"b": 2}, {"c": {"nested": true}}]`
	objects := splitJSONObjects(input)
	if len(objects) != 3 {
		t.Fatalf("expected 3 objects, got %d", len(objects))
	}
	if objects[0] != `{"a": 1}` {
		t.Errorf("object[0] = %q", objects[0])
	}
	if objects[1] != `{"b": 2}` {
		t.Errorf("object[1] = %q", objects[1])
	}
	if !strings.Contains(objects[2], "nested") {
		t.Errorf("object[2] = %q", objects[2])
	}
}

func TestExtractJSONString(t *testing.T) {
	obj := `{"title": "Hello World", "url": "https://example.com", "empty": ""}`

	if v := extractJSONString(obj, "title"); v != "Hello World" {
		t.Errorf("title = %q", v)
	}
	if v := extractJSONString(obj, "url"); v != "https://example.com" {
		t.Errorf("url = %q", v)
	}
	if v := extractJSONString(obj, "empty"); v != "" {
		t.Errorf("empty = %q", v)
	}
	if v := extractJSONString(obj, "missing"); v != "" {
		t.Errorf("missing = %q", v)
	}
}

func TestExtractJSONStringEscaped(t *testing.T) {
	obj := `{"msg": "hello \"world\""}`
	if v := extractJSONString(obj, "msg"); v != `hello "world"` {
		t.Errorf("msg = %q", v)
	}
}

func TestExtractJSONInt(t *testing.T) {
	obj := `{"number": 42, "zero": 0}`

	if v := extractJSONInt(obj, "number"); v != 42 {
		t.Errorf("number = %d", v)
	}
	if v := extractJSONInt(obj, "zero"); v != 0 {
		t.Errorf("zero = %d", v)
	}
	if v := extractJSONInt(obj, "missing"); v != 0 {
		t.Errorf("missing = %d", v)
	}
}

func TestExtractJSONBool(t *testing.T) {
	obj := `{"isDraft": true, "mergeable": false}`

	if v := extractJSONBool(obj, "isDraft"); !v {
		t.Error("isDraft should be true")
	}
	if v := extractJSONBool(obj, "mergeable"); v {
		t.Error("mergeable should be false")
	}
	if v := extractJSONBool(obj, "missing"); v {
		t.Error("missing should be false")
	}
}

func TestExtractNestedJSONString(t *testing.T) {
	obj := `{"author": {"login": "octocat", "id": 1}}`
	if v := extractNestedJSONString(obj, "author", "login"); v != "octocat" {
		t.Errorf("author.login = %q", v)
	}
}

func TestExtractJSONStringArray(t *testing.T) {
	obj := `{"labels": [{"name": "bug"}, {"name": "urgent"}]}`
	labels := extractJSONStringArray(obj, "labels", "name")
	if len(labels) != 2 {
		t.Fatalf("expected 2 labels, got %d", len(labels))
	}
	if labels[0] != "bug" {
		t.Errorf("labels[0] = %q", labels[0])
	}
	if labels[1] != "urgent" {
		t.Errorf("labels[1] = %q", labels[1])
	}
}

func TestExtractJSONStringArraySimple(t *testing.T) {
	obj := `{"tags": ["alpha", "beta"]}`
	tags := extractJSONStringArray(obj, "tags", "")
	if len(tags) != 2 {
		t.Fatalf("expected 2 tags, got %d", len(tags))
	}
	if tags[0] != "alpha" {
		t.Errorf("tags[0] = %q", tags[0])
	}
	if tags[1] != "beta" {
		t.Errorf("tags[1] = %q", tags[1])
	}
}

func TestParseIssuesJSON(t *testing.T) {
	gp := NewGitProvider("github", "", "owner", "repo")
	jsonStr := `[
		{
			"number": 1,
			"title": "Fix login bug",
			"body": "Login fails on Safari",
			"state": "open",
			"url": "https://github.com/owner/repo/issues/1",
			"author": {"login": "alice"},
			"labels": [{"name": "bug"}],
			"createdAt": "2024-01-15T10:00:00Z"
		},
		{
			"number": 2,
			"title": "Add tests",
			"body": "",
			"state": "closed",
			"url": "https://github.com/owner/repo/issues/2",
			"author": {"login": "bob"},
			"labels": [],
			"createdAt": "2024-01-10T08:00:00Z"
		}
	]`

	issues, err := gp.parseIssuesJSON(jsonStr)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(issues) != 2 {
		t.Fatalf("expected 2 issues, got %d", len(issues))
	}

	if issues[0].Number != 1 {
		t.Errorf("issue[0].Number = %d", issues[0].Number)
	}
	if issues[0].Title != "Fix login bug" {
		t.Errorf("issue[0].Title = %q", issues[0].Title)
	}
	if issues[0].State != "open" {
		t.Errorf("issue[0].State = %q", issues[0].State)
	}
	if issues[0].Author != "alice" {
		t.Errorf("issue[0].Author = %q", issues[0].Author)
	}
	if len(issues[0].Labels) != 1 || issues[0].Labels[0] != "bug" {
		t.Errorf("issue[0].Labels = %v", issues[0].Labels)
	}

	if issues[1].Number != 2 {
		t.Errorf("issue[1].Number = %d", issues[1].Number)
	}
	if issues[1].State != "closed" {
		t.Errorf("issue[1].State = %q", issues[1].State)
	}
}

func TestParsePRsJSON(t *testing.T) {
	gp := NewGitProvider("github", "", "owner", "repo")
	jsonStr := `[
		{
			"number": 10,
			"title": "Add authentication",
			"body": "Implements OAuth2",
			"headRefName": "feature/auth",
			"baseRefName": "main",
			"state": "open",
			"isDraft": false,
			"url": "https://github.com/owner/repo/pull/10",
			"mergeable": "MERGEABLE",
			"labels": [{"name": "enhancement"}]
		},
		{
			"number": 11,
			"title": "WIP: Refactoring",
			"body": "",
			"headRefName": "refactor/core",
			"baseRefName": "develop",
			"state": "open",
			"isDraft": true,
			"url": "https://github.com/owner/repo/pull/11",
			"mergeable": "CONFLICTING",
			"labels": []
		}
	]`

	prs, err := gp.parsePRsJSON(jsonStr)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(prs) != 2 {
		t.Fatalf("expected 2 PRs, got %d", len(prs))
	}

	if prs[0].Number != 10 {
		t.Errorf("pr[0].Number = %d", prs[0].Number)
	}
	if prs[0].Branch != "feature/auth" {
		t.Errorf("pr[0].Branch = %q", prs[0].Branch)
	}
	if prs[0].BaseBranch != "main" {
		t.Errorf("pr[0].BaseBranch = %q", prs[0].BaseBranch)
	}
	if !prs[0].Mergeable {
		t.Error("pr[0].Mergeable should be true")
	}
	if prs[0].Draft {
		t.Error("pr[0].Draft should be false")
	}

	if prs[1].Number != 11 {
		t.Errorf("pr[1].Number = %d", prs[1].Number)
	}
	if !prs[1].Draft {
		t.Error("pr[1].Draft should be true")
	}
	if prs[1].Mergeable {
		t.Error("pr[1].Mergeable should be false")
	}
}

func TestParseRunsJSON(t *testing.T) {
	gp := NewGitProvider("github", "", "owner", "repo")
	jsonStr := `[
		{"name": "lint", "status": "completed", "conclusion": "success", "url": "https://example.com/1"},
		{"name": "test", "status": "completed", "conclusion": "success", "url": "https://example.com/2"},
		{"name": "build", "status": "completed", "conclusion": "failure", "url": "https://example.com/3"},
		{"name": "deploy", "status": "in_progress", "conclusion": "", "url": "https://example.com/4"}
	]`

	status, err := gp.parseRunsJSON(jsonStr, "feature/auth")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if status.State != "failure" {
		t.Errorf("expected state 'failure', got %q", status.State)
	}
	if len(status.Checks) != 4 {
		t.Fatalf("expected 4 checks, got %d", len(status.Checks))
	}
	if status.Checks[0].Status != "success" {
		t.Errorf("check[0].Status = %q", status.Checks[0].Status)
	}
	if status.Checks[2].Status != "failure" {
		t.Errorf("check[2].Status = %q", status.Checks[2].Status)
	}
	if status.Checks[3].Status != "pending" {
		t.Errorf("check[3].Status = %q", status.Checks[3].Status)
	}
}

func TestParseRunsJSONAllSuccess(t *testing.T) {
	gp := NewGitProvider("github", "", "owner", "repo")
	jsonStr := `[
		{"name": "lint", "status": "completed", "conclusion": "success", "url": ""},
		{"name": "test", "status": "completed", "conclusion": "success", "url": ""}
	]`

	status, err := gp.parseRunsJSON(jsonStr, "main")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if status.State != "success" {
		t.Errorf("expected state 'success', got %q", status.State)
	}
}

func TestParseRunsJSONEmpty(t *testing.T) {
	gp := NewGitProvider("github", "", "owner", "repo")
	status, err := gp.parseRunsJSON("[]", "main")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if status.State != "pending" {
		t.Errorf("expected state 'pending', got %q", status.State)
	}
}

func TestDetectProviderFromHost(t *testing.T) {
	tests := []struct {
		host     string
		expected string
	}{
		{"github.com", "github"},
		{"gitlab.com", "gitlab"},
		{"gitlab.mycompany.com", "gitlab"},
		{"bitbucket.org", "bitbucket"},
		{"unknown.com", "github"},
	}

	for _, tt := range tests {
		t.Run(tt.host, func(t *testing.T) {
			got := detectProviderFromHost(tt.host)
			if got != tt.expected {
				t.Errorf("detectProviderFromHost(%q) = %q, want %q", tt.host, got, tt.expected)
			}
		})
	}
}

func TestParseGitConfigNoRemote(t *testing.T) {
	config := `[core]
	repositoryformatversion = 0
	filemode = true
`
	provider, owner, repo := parseGitConfig(config)
	if provider != "" || owner != "" || repo != "" {
		t.Errorf("expected empty results, got %q %q %q", provider, owner, repo)
	}
}

func TestParseChecksJSON(t *testing.T) {
	gp := NewGitProvider("github", "", "owner", "repo")
	jsonStr := `[
		{"name": "lint", "state": "SUCCESS", "elapsed": 12, "link": "https://example.com/1"},
		{"name": "test", "state": "FAILURE", "elapsed": 45, "link": "https://example.com/2"},
		{"name": "build", "state": "PENDING", "elapsed": 0, "link": "https://example.com/3"}
	]`

	status, err := gp.parseChecksJSON(jsonStr)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if status.State != "failure" {
		t.Errorf("expected state 'failure', got %q", status.State)
	}
	if len(status.Checks) != 3 {
		t.Fatalf("expected 3 checks, got %d", len(status.Checks))
	}
	if status.Checks[0].Status != "success" {
		t.Errorf("check[0].Status = %q", status.Checks[0].Status)
	}
	if status.Checks[0].Duration != 12*time.Second {
		t.Errorf("check[0].Duration = %v", status.Checks[0].Duration)
	}
	if status.Checks[1].Status != "failure" {
		t.Errorf("check[1].Status = %q", status.Checks[1].Status)
	}
	if status.Checks[2].Status != "pending" {
		t.Errorf("check[2].Status = %q", status.Checks[2].Status)
	}
}
