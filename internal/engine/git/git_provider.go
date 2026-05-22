package git

import (
	"bufio"
	"bytes"
	"context"
	"fmt"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"
)

// GitProvider integrates with GitHub/GitLab/Bitbucket APIs for issue management,
// PR creation, and CI status checking via CLI tools.
type GitProvider struct {
	Type    string // "github", "gitlab", "bitbucket"
	Token   string
	Owner   string
	Repo    string
	BaseURL string
	mu      sync.RWMutex
}

// GitIssue represents a git provider issue.
type GitIssue struct {
	Number    int
	Title     string
	Body      string
	Labels    []string
	State     string
	Author    string
	CreatedAt time.Time
	URL       string
}

// PullRequest represents a pull/merge request.
type PullRequest struct {
	Number     int
	Title      string
	Body       string
	Branch     string
	BaseBranch string
	State      string
	Draft      bool
	Labels     []string
	URL        string
	Mergeable  bool
}

// CIStatus represents the overall CI/CD status for a branch.
type CIStatus struct {
	State  string // "success", "failure", "pending"
	Checks []CICheck
	URL    string
}

// CICheck represents a single CI check/job.
type CICheck struct {
	Name     string
	Status   string
	Duration time.Duration
	URL      string
}

// NewGitProvider creates a new GitProvider with the given configuration.
func NewGitProvider(providerType, token, owner, repo string) *GitProvider {
	return &GitProvider{
		Type:  providerType,
		Token: token,
		Owner: owner,
		Repo:  repo,
	}
}

// ListIssues returns issues matching the given state and limit.
func (gp *GitProvider) ListIssues(state string, limit int) ([]GitIssue, error) {
	gp.mu.RLock()
	defer gp.mu.RUnlock()

	args := []string{"issue", "list", "--state", state}
	if limit > 0 {
		args = append(args, "--limit", strconv.Itoa(limit))
	}
	args = append(args, "--json", "number,title,body,labels,state,author,createdAt,url")

	out, err := gp.runGH(args...)
	if err != nil {
		return nil, fmt.Errorf("list issues: %w", err)
	}

	return gp.parseIssuesJSON(out)
}

// GetIssue returns a single issue by number.
func (gp *GitProvider) GetIssue(number int) (*GitIssue, error) {
	gp.mu.RLock()
	defer gp.mu.RUnlock()

	args := []string{"issue", "view", strconv.Itoa(number), "--json", "number,title,body,labels,state,author,createdAt,url"}

	out, err := gp.runGH(args...)
	if err != nil {
		return nil, fmt.Errorf("get issue #%d: %w", number, err)
	}

	issues, err := gp.parseIssuesJSON("[" + out + "]")
	if err != nil {
		return nil, err
	}
	if len(issues) == 0 {
		return nil, fmt.Errorf("issue #%d not found", number)
	}
	return &issues[0], nil
}

// CreateIssue creates a new issue with the given title, body, and labels.
func (gp *GitProvider) CreateIssue(title, body string, labels []string) (*GitIssue, error) {
	gp.mu.Lock()
	defer gp.mu.Unlock()

	args := []string{"issue", "create", "--title", title, "--body", body}
	for _, label := range labels {
		args = append(args, "--label", label)
	}

	out, err := gp.runGH(args...)
	if err != nil {
		return nil, fmt.Errorf("create issue: %w", err)
	}

	// gh issue create outputs the URL of the new issue
	url := strings.TrimSpace(out)
	number := extractNumberFromURL(url)

	return &GitIssue{
		Number:    number,
		Title:     title,
		Body:      body,
		Labels:    labels,
		State:     "open",
		URL:       url,
		CreatedAt: time.Now(),
	}, nil
}

// ListPRs returns pull requests matching the given state and limit.
func (gp *GitProvider) ListPRs(state string, limit int) ([]PullRequest, error) {
	gp.mu.RLock()
	defer gp.mu.RUnlock()

	args := []string{"pr", "list", "--state", state}
	if limit > 0 {
		args = append(args, "--limit", strconv.Itoa(limit))
	}
	args = append(args, "--json", "number,title,body,headRefName,baseRefName,state,isDraft,labels,url,mergeable")

	out, err := gp.runGH(args...)
	if err != nil {
		return nil, fmt.Errorf("list PRs: %w", err)
	}

	return gp.parsePRsJSON(out)
}

// CreatePR creates a new pull request.
func (gp *GitProvider) CreatePR(title, body, branch, baseBranch string) (*PullRequest, error) {
	gp.mu.Lock()
	defer gp.mu.Unlock()

	args := []string{"pr", "create", "--title", title, "--body", body, "--head", branch, "--base", baseBranch}

	out, err := gp.runGH(args...)
	if err != nil {
		return nil, fmt.Errorf("create PR: %w", err)
	}

	url := strings.TrimSpace(out)
	number := extractNumberFromURL(url)

	return &PullRequest{
		Number:     number,
		Title:      title,
		Body:       body,
		Branch:     branch,
		BaseBranch: baseBranch,
		State:      "open",
		URL:        url,
	}, nil
}

// GetCIStatus returns the CI status for the given branch.
func (gp *GitProvider) GetCIStatus(branch string) (*CIStatus, error) {
	gp.mu.RLock()
	defer gp.mu.RUnlock()

	args := []string{"run", "list", "--branch", branch, "--limit", "1", "--json", "status,conclusion,name,url"}

	out, err := gp.runGH(args...)
	if err != nil {
		// Fallback to pr checks
		return gp.getCIStatusFromPRChecks(branch)
	}

	return gp.parseRunsJSON(out, branch)
}

// getCIStatusFromPRChecks uses gh pr checks as a fallback for CI status.
func (gp *GitProvider) getCIStatusFromPRChecks(branch string) (*CIStatus, error) {
	args := []string{"pr", "checks", "--json", "name,state,elapsed,link"}
	out, err := gp.runGH(args...)
	if err != nil {
		return nil, fmt.Errorf("get CI status for branch %s: %w", branch, err)
	}

	return gp.parseChecksJSON(out)
}

// GetReviewComments returns review comments for a PR.
func (gp *GitProvider) GetReviewComments(prNumber int) ([]string, error) {
	gp.mu.RLock()
	defer gp.mu.RUnlock()

	args := []string{"pr", "view", strconv.Itoa(prNumber), "--json", "reviews", "--jq", ".reviews[].body"}

	out, err := gp.runGH(args...)
	if err != nil {
		return nil, fmt.Errorf("get review comments for PR #%d: %w", prNumber, err)
	}

	var comments []string
	scanner := bufio.NewScanner(strings.NewReader(out))
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line != "" {
			comments = append(comments, line)
		}
	}
	return comments, nil
}

// FormatIssues formats a slice of issues for terminal display.
func FormatIssues(issues []GitIssue) string {
	if len(issues) == 0 {
		return "No issues found."
	}

	var b strings.Builder
	b.WriteString("Issues\n")
	b.WriteString(strings.Repeat("─", 40))
	b.WriteString("\n")

	for _, issue := range issues {
		stateIcon := "○" // open
		if issue.State == "closed" {
			stateIcon = "✓"
		}
		b.WriteString(fmt.Sprintf("%s #%d %s", stateIcon, issue.Number, issue.Title))
		if len(issue.Labels) > 0 {
			b.WriteString(fmt.Sprintf(" [%s]", strings.Join(issue.Labels, ", ")))
		}
		b.WriteString(fmt.Sprintf(" (@%s)\n", issue.Author))
	}
	return b.String()
}

// FormatPRs formats a slice of pull requests for terminal display.
func FormatPRs(prs []PullRequest) string {
	if len(prs) == 0 {
		return "No pull requests found."
	}

	var b strings.Builder
	b.WriteString("Pull Requests\n")
	b.WriteString(strings.Repeat("─", 40))
	b.WriteString("\n")

	for _, pr := range prs {
		stateIcon := "○" // open
		if pr.State == "merged" {
			stateIcon = "✓"
		} else if pr.State == "closed" {
			stateIcon = "✗"
		}
		draft := ""
		if pr.Draft {
			draft = " [draft]"
		}
		b.WriteString(fmt.Sprintf("%s #%d %s%s (%s → %s)\n",
			stateIcon, pr.Number, pr.Title, draft, pr.Branch, pr.BaseBranch))
	}
	return b.String()
}

// FormatCIStatus formats a CIStatus for terminal display.
func FormatCIStatus(status *CIStatus) string {
	if status == nil {
		return "No CI status available."
	}

	var b strings.Builder
	b.WriteString(fmt.Sprintf("CI Status: %s\n", status.URL))
	b.WriteString(strings.Repeat("─", 25))
	b.WriteString("\n")

	failed := 0
	total := len(status.Checks)

	for _, check := range status.Checks {
		var icon string
		var detail string

		switch check.Status {
		case "success":
			icon = "✓"
			detail = fmt.Sprintf("(%s)", formatCIDuration(check.Duration))
		case "failure":
			icon = "✗"
			detail = fmt.Sprintf("(failed after %s)", formatCIDuration(check.Duration))
			failed++
		case "pending":
			icon = "○"
			detail = "(pending)"
		default:
			icon = "?"
			detail = fmt.Sprintf("(%s)", check.Status)
		}

		b.WriteString(fmt.Sprintf("%s %s %s\n", icon, check.Name, detail))
	}

	b.WriteString("\n")
	if failed == 0 {
		b.WriteString(fmt.Sprintf("Overall: PASSING (%d/%d checks passed)\n", total, total))
	} else {
		b.WriteString(fmt.Sprintf("Overall: FAILING (%d/%d checks failed)\n", failed, total))
	}

	return b.String()
}

// DetectProvider parses .git/config or uses gh to detect provider/owner/repo.
func DetectProvider(projectDir string) (string, string, string) {
	// Try gh repo view first
	cmd := exec.CommandContext(context.Background(), "gh", "repo", "view", "--json", "owner,name,url")
	cmd.Dir = projectDir
	out, err := cmd.Output()
	if err == nil {
		owner, repo, provider := parseGHRepoView(string(out))
		if owner != "" && repo != "" {
			return provider, owner, repo
		}
	}

	// Fallback: parse .git/config
	gitConfigPath := filepath.Join(projectDir, ".git", "config")
	cmd = exec.CommandContext(context.Background(), "cat", gitConfigPath)
	out, err = cmd.Output()
	if err != nil {
		return "", "", ""
	}

	return parseGitConfig(string(out))
}

// runGH executes a gh CLI command and returns stdout.
func (gp *GitProvider) runGH(args ...string) (string, error) {
	repoFlag := fmt.Sprintf("%s/%s", gp.Owner, gp.Repo)
	fullArgs := append(args, "--repo", repoFlag)

	cmd := exec.CommandContext(context.Background(), "gh", fullArgs...)
	if gp.Token != "" {
		cmd.Env = append(cmd.Environ(), "GH_TOKEN="+gp.Token)
	}

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		return "", fmt.Errorf("%s: %s", err, stderr.String())
	}

	return stdout.String(), nil
}

// parseIssuesJSON parses the JSON output from gh issue list.
func (gp *GitProvider) parseIssuesJSON(jsonStr string) ([]GitIssue, error) {
	// Simple JSON parsing without encoding/json for stdlib-only constraint
	// Parse array of issue objects
	var issues []GitIssue
	jsonStr = strings.TrimSpace(jsonStr)
	if jsonStr == "" || jsonStr == "[]" {
		return issues, nil
	}

	// Split by objects (simplified parsing)
	objects := splitJSONObjects(jsonStr)
	for _, obj := range objects {
		issue := GitIssue{}
		issue.Number = extractJSONInt(obj, "number")
		issue.Title = extractJSONString(obj, "title")
		issue.Body = extractJSONString(obj, "body")
		issue.State = extractJSONString(obj, "state")
		issue.URL = extractJSONString(obj, "url")

		// Parse author - can be nested object or string
		authorStr := extractJSONString(obj, "login")
		if authorStr == "" {
			authorStr = extractNestedJSONString(obj, "author", "login")
		}
		issue.Author = authorStr

		// Parse labels
		issue.Labels = extractJSONStringArray(obj, "labels", "name")

		// Parse createdAt
		createdStr := extractJSONString(obj, "createdAt")
		if createdStr != "" {
			t, err := time.Parse(time.RFC3339, createdStr)
			if err == nil {
				issue.CreatedAt = t
			}
		}

		issues = append(issues, issue)
	}

	return issues, nil
}

// parsePRsJSON parses the JSON output from gh pr list.
func (gp *GitProvider) parsePRsJSON(jsonStr string) ([]PullRequest, error) {
	var prs []PullRequest
	jsonStr = strings.TrimSpace(jsonStr)
	if jsonStr == "" || jsonStr == "[]" {
		return prs, nil
	}

	objects := splitJSONObjects(jsonStr)
	for _, obj := range objects {
		pr := PullRequest{}
		pr.Number = extractJSONInt(obj, "number")
		pr.Title = extractJSONString(obj, "title")
		pr.Body = extractJSONString(obj, "body")
		pr.Branch = extractJSONString(obj, "headRefName")
		pr.BaseBranch = extractJSONString(obj, "baseRefName")
		pr.State = extractJSONString(obj, "state")
		pr.URL = extractJSONString(obj, "url")
		pr.Draft = extractJSONBool(obj, "isDraft")
		pr.Mergeable = extractJSONString(obj, "mergeable") == "MERGEABLE"
		pr.Labels = extractJSONStringArray(obj, "labels", "name")

		prs = append(prs, pr)
	}

	return prs, nil
}

// parseRunsJSON parses the JSON output from gh run list.
func (gp *GitProvider) parseRunsJSON(jsonStr string, branch string) (*CIStatus, error) {
	jsonStr = strings.TrimSpace(jsonStr)
	if jsonStr == "" || jsonStr == "[]" {
		return &CIStatus{State: "pending", URL: branch}, nil
	}

	status := &CIStatus{URL: branch}
	objects := splitJSONObjects(jsonStr)

	hasFailure := false
	hasPending := false

	for _, obj := range objects {
		check := CICheck{}
		check.Name = extractJSONString(obj, "name")
		check.URL = extractJSONString(obj, "url")

		conclusion := extractJSONString(obj, "conclusion")
		runStatus := extractJSONString(obj, "status")

		if runStatus == "completed" {
			if conclusion == "success" {
				check.Status = "success"
			} else {
				check.Status = "failure"
				hasFailure = true
			}
		} else {
			check.Status = "pending"
			hasPending = true
		}

		status.Checks = append(status.Checks, check)
	}

	if hasFailure {
		status.State = "failure"
	} else if hasPending {
		status.State = "pending"
	} else {
		status.State = "success"
	}

	return status, nil
}

// parseChecksJSON parses the JSON output from gh pr checks.
func (gp *GitProvider) parseChecksJSON(jsonStr string) (*CIStatus, error) {
	jsonStr = strings.TrimSpace(jsonStr)
	if jsonStr == "" || jsonStr == "[]" {
		return &CIStatus{State: "pending"}, nil
	}

	status := &CIStatus{}
	objects := splitJSONObjects(jsonStr)

	hasFailure := false
	hasPending := false

	for _, obj := range objects {
		check := CICheck{}
		check.Name = extractJSONString(obj, "name")
		check.URL = extractJSONString(obj, "link")

		state := extractJSONString(obj, "state")
		switch state {
		case "SUCCESS":
			check.Status = "success"
		case "FAILURE":
			check.Status = "failure"
			hasFailure = true
		default:
			check.Status = "pending"
			hasPending = true
		}

		// Parse elapsed time
		elapsed := extractJSONInt(obj, "elapsed")
		if elapsed > 0 {
			check.Duration = time.Duration(elapsed) * time.Second
		}

		status.Checks = append(status.Checks, check)
	}

	if hasFailure {
		status.State = "failure"
	} else if hasPending {
		status.State = "pending"
	} else {
		status.State = "success"
	}

	return status, nil
}

// parseGHRepoView parses the output of gh repo view --json.
func parseGHRepoView(jsonStr string) (string, string, string) {
	owner := extractJSONString(jsonStr, "owner")
	// owner might be nested
	if owner == "" {
		owner = extractNestedJSONString(jsonStr, "owner", "login")
	}
	name := extractJSONString(jsonStr, "name")
	url := extractJSONString(jsonStr, "url")

	provider := "github"
	if strings.Contains(url, "gitlab") {
		provider = "gitlab"
	} else if strings.Contains(url, "bitbucket") {
		provider = "bitbucket"
	}

	return owner, name, provider
}

// parseGitConfig extracts remote origin URL from .git/config content.
func parseGitConfig(content string) (string, string, string) {
	scanner := bufio.NewScanner(strings.NewReader(content))
	inRemoteOrigin := false

	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())

		if line == "[remote \"origin\"]" {
			inRemoteOrigin = true
			continue
		}
		if strings.HasPrefix(line, "[") {
			inRemoteOrigin = false
			continue
		}

		if inRemoteOrigin && strings.HasPrefix(line, "url =") {
			url := strings.TrimSpace(strings.TrimPrefix(line, "url ="))
			return parseRemoteURL(url)
		}
	}

	return "", "", ""
}

// parseRemoteURL extracts provider, owner, and repo from a git remote URL.
func parseRemoteURL(url string) (string, string, string) {
	var provider string

	// Normalize URL
	url = strings.TrimSuffix(url, ".git")

	// SSH format: git@github.com:owner/repo
	if strings.HasPrefix(url, "git@") {
		parts := strings.SplitN(url, ":", 2)
		if len(parts) != 2 {
			return "", "", ""
		}
		host := strings.TrimPrefix(parts[0], "git@")
		provider = detectProviderFromHost(host)

		pathParts := strings.Split(parts[1], "/")
		if len(pathParts) >= 2 {
			return provider, pathParts[len(pathParts)-2], pathParts[len(pathParts)-1]
		}
		return "", "", ""
	}

	// HTTPS format: https://github.com/owner/repo
	if strings.Contains(url, "://") {
		parts := strings.Split(url, "/")
		if len(parts) < 5 {
			return "", "", ""
		}
		host := parts[2]
		provider = detectProviderFromHost(host)
		return provider, parts[3], parts[4]
	}

	return "", "", ""
}

// detectProviderFromHost determines the git provider from a hostname.
func detectProviderFromHost(host string) string {
	switch {
	case strings.Contains(host, "gitlab"):
		return "gitlab"
	case strings.Contains(host, "bitbucket"):
		return "bitbucket"
	default:
		return "github"
	}
}

// extractNumberFromURL extracts an issue/PR number from a URL like
// https://github.com/owner/repo/issues/42
func extractNumberFromURL(url string) int {
	parts := strings.Split(strings.TrimSpace(url), "/")
	if len(parts) == 0 {
		return 0
	}
	n, _ := strconv.Atoi(parts[len(parts)-1])
	return n
}

// formatCIDuration formats a duration for display (e.g. "12s", "2m30s").
func formatCIDuration(d time.Duration) string {
	if d == 0 {
		return "0s"
	}
	if d < time.Minute {
		return fmt.Sprintf("%ds", int(d.Seconds()))
	}
	mins := int(d.Minutes())
	secs := int(d.Seconds()) % 60
	if secs == 0 {
		return fmt.Sprintf("%dm", mins)
	}
	return fmt.Sprintf("%dm%ds", mins, secs)
}

// --- Simple JSON parsing helpers (stdlib only, no encoding/json) ---

// splitJSONObjects splits a JSON array string into individual object strings.
func splitJSONObjects(jsonStr string) []string {
	var objects []string
	depth := 0
	start := -1

	for i, ch := range jsonStr {
		switch ch {
		case '{':
			if depth == 0 {
				start = i
			}
			depth++
		case '}':
			depth--
			if depth == 0 && start >= 0 {
				objects = append(objects, jsonStr[start:i+1])
				start = -1
			}
		}
	}

	return objects
}

// extractJSONString extracts a string value for the given key from a JSON object string.
func extractJSONString(obj, key string) string {
	search := fmt.Sprintf(`"%s"`, key)
	idx := strings.Index(obj, search)
	if idx < 0 {
		return ""
	}

	// Find the colon after the key
	rest := obj[idx+len(search):]
	colonIdx := strings.Index(rest, ":")
	if colonIdx < 0 {
		return ""
	}
	rest = strings.TrimSpace(rest[colonIdx+1:])

	if len(rest) == 0 {
		return ""
	}

	// Check if value is a string (starts with quote)
	if rest[0] != '"' {
		return ""
	}

	// Find the closing quote (handle escaped quotes)
	var b strings.Builder
	escaped := false
	for i := 1; i < len(rest); i++ {
		if escaped {
			b.WriteByte(rest[i])
			escaped = false
			continue
		}
		if rest[i] == '\\' {
			escaped = true
			continue
		}
		if rest[i] == '"' {
			return b.String()
		}
		b.WriteByte(rest[i])
	}
	return b.String()
}

// extractJSONInt extracts an integer value for the given key from a JSON object string.
func extractJSONInt(obj, key string) int {
	search := fmt.Sprintf(`"%s"`, key)
	idx := strings.Index(obj, search)
	if idx < 0 {
		return 0
	}

	rest := obj[idx+len(search):]
	colonIdx := strings.Index(rest, ":")
	if colonIdx < 0 {
		return 0
	}
	rest = strings.TrimSpace(rest[colonIdx+1:])

	// Read digits
	var numStr strings.Builder
	for _, ch := range rest {
		if ch >= '0' && ch <= '9' {
			numStr.WriteRune(ch)
		} else if numStr.Len() > 0 {
			break
		} else if ch != ' ' && ch != '\t' && ch != '\n' {
			break
		}
	}

	n, _ := strconv.Atoi(numStr.String())
	return n
}

// extractJSONBool extracts a boolean value for the given key from a JSON object string.
func extractJSONBool(obj, key string) bool {
	search := fmt.Sprintf(`"%s"`, key)
	idx := strings.Index(obj, search)
	if idx < 0 {
		return false
	}

	rest := obj[idx+len(search):]
	colonIdx := strings.Index(rest, ":")
	if colonIdx < 0 {
		return false
	}
	rest = strings.TrimSpace(rest[colonIdx+1:])

	return strings.HasPrefix(rest, "true")
}

// extractNestedJSONString extracts a string from a nested object, e.g. "author": {"login": "foo"}.
func extractNestedJSONString(obj, parentKey, childKey string) string {
	search := fmt.Sprintf(`"%s"`, parentKey)
	idx := strings.Index(obj, search)
	if idx < 0 {
		return ""
	}

	rest := obj[idx+len(search):]
	colonIdx := strings.Index(rest, ":")
	if colonIdx < 0 {
		return ""
	}
	rest = strings.TrimSpace(rest[colonIdx+1:])

	// Find the nested object
	braceIdx := strings.Index(rest, "{")
	if braceIdx < 0 {
		return ""
	}

	// Find closing brace
	depth := 0
	for i := braceIdx; i < len(rest); i++ {
		if rest[i] == '{' {
			depth++
		} else if rest[i] == '}' {
			depth--
			if depth == 0 {
				nestedObj := rest[braceIdx : i+1]
				return extractJSONString(nestedObj, childKey)
			}
		}
	}

	return ""
}

// extractJSONStringArray extracts string values from an array of objects.
// For example, "labels": [{"name": "bug"}, {"name": "help"}] -> ["bug", "help"]
func extractJSONStringArray(obj, arrayKey, itemKey string) []string {
	search := fmt.Sprintf(`"%s"`, arrayKey)
	idx := strings.Index(obj, search)
	if idx < 0 {
		return nil
	}

	rest := obj[idx+len(search):]
	colonIdx := strings.Index(rest, ":")
	if colonIdx < 0 {
		return nil
	}
	rest = strings.TrimSpace(rest[colonIdx+1:])

	// Find array bounds
	bracketIdx := strings.Index(rest, "[")
	if bracketIdx < 0 {
		return nil
	}

	depth := 0
	var arrayContent string
	for i := bracketIdx; i < len(rest); i++ {
		if rest[i] == '[' {
			depth++
		} else if rest[i] == ']' {
			depth--
			if depth == 0 {
				arrayContent = rest[bracketIdx : i+1]
				break
			}
		}
	}

	if arrayContent == "" {
		return nil
	}

	// Extract items from array
	var result []string
	objects := splitJSONObjects(arrayContent)
	for _, item := range objects {
		val := extractJSONString(item, itemKey)
		if val != "" {
			result = append(result, val)
		}
	}

	// Also try simple string array: ["foo", "bar"]
	if len(result) == 0 {
		inner := arrayContent[1 : len(arrayContent)-1]
		parts := strings.Split(inner, ",")
		for _, part := range parts {
			part = strings.TrimSpace(part)
			if strings.HasPrefix(part, `"`) && strings.HasSuffix(part, `"`) {
				result = append(result, part[1:len(part)-1])
			}
		}
	}

	return result
}
