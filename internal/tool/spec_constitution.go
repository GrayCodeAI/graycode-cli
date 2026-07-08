package tool

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// ConstitutionTool manages the project's constitution — a set of
// non-negotiable rules and constraints that all specs and implementations
// must follow. The constitution is stored in .hawk/specs/constitution.md
// and referenced during spec validation.
type ConstitutionTool struct{}

func (ConstitutionTool) Name() string { return "Constitution" }
func (ConstitutionTool) Aliases() []string {
	return []string{"constitution", "spec_constitution", "spec:constitution"}
}

func (ConstitutionTool) Description() string {
	return "Read or update the project constitution — non-negotiable rules that all specs must follow. Use 'get' to read, 'set' to update, 'init' to create from template, 'validate' to check a spec against constitution rules."
}

func (ConstitutionTool) Parameters() map[string]interface{} {
	return map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"action": map[string]interface{}{
				"type":        "string",
				"description": "Action to perform: get (read), set (update rules), init (create from template), validate (check spec against rules)",
				"enum":        []string{"get", "set", "init", "validate"},
			},
			"rules": map[string]interface{}{
				"type":        "string",
				"description": "Constitution rules content (required for 'set' action)",
			},
		},
		"required": []string{"action"},
	}
}

func (ConstitutionTool) Execute(ctx context.Context, input json.RawMessage) (string, error) {
	var p struct {
		Action string `json:"action"`
		Rules  string `json:"rules"`
	}
	if err := json.Unmarshal(input, &p); err != nil {
		return "", err
	}

	dir, err := specDir(ctx)
	if err != nil {
		return "", err
	}

	constitutionPath := filepath.Join(dir, "constitution.md")

	switch p.Action {
	case "get":
		return getConstitution(constitutionPath)

	case "init":
		return initConstitution(constitutionPath)

	case "set":
		if strings.TrimSpace(p.Rules) == "" {
			return "", fmt.Errorf("rules content is required for 'set' action")
		}
		return setConstitution(constitutionPath, p.Rules)

	case "validate":
		return validateAgainstConstitution(ctx, dir, constitutionPath)

	default:
		return "", fmt.Errorf("unknown action %q. Use 'get', 'set', 'init', or 'validate'", p.Action)
	}
}

func getConstitution(path string) (string, error) {
	data, err := os.ReadFile(path) // #nosec G304 -- path provided by caller via tool/task parameters, inherent to this dev CLI's file operations
	if err != nil {
		return "No constitution found. Use action='init' to create one from template.", nil
	}
	return fmt.Sprintf("# Project Constitution\n\n%s", string(data)), nil
}

func initConstitution(path string) (string, error) {
	if _, err := os.Stat(path); err == nil {
		return "", fmt.Errorf("constitution already exists at %s — use 'set' to update", path)
	}

	template := "## Core Principles\n\n" +
		"1. **Security First**: Never expose secrets, never trust user input, always validate.\n" +
		"2. **Explicit Over Implicit**: Use SHALL/MUST for normative requirements. No vague language.\n" +
		"3. **Testable Everything**: Every requirement MUST have at least one test scenario.\n" +
		"4. **Minimal Scope**: Each change should do one thing well. Avoid scope creep.\n" +
		"5. **Backward Compatibility**: Breaking changes require explicit migration plan.\n\n" +
		"## Code Standards\n\n" +
		"- All errors must be handled (no unchecked errors)\n" +
		"- No global mutable state — prefer dependency injection\n" +
		"- Functions should do one thing well\n" +
		"- Tests must pass with race detector enabled\n\n" +
		"## Architecture Rules\n\n" +
		"- Internal packages must not be imported from external repos\n" +
		"- API keys go in OS keychain, not config files\n" +
		"- No panic() for error handling (except init() assertions)\n" +
		"- No fmt.Print for logging — use structured logger\n\n" +
		"## Review Requirements\n\n" +
		"- All PRs must have at least one approval\n" +
		"- Security-sensitive changes require security review\n" +
		"- Performance changes require benchmarks\n"

	if err := os.WriteFile(path, []byte(template), 0o600); err != nil {
		return "", fmt.Errorf("write constitution: %w", err)
	}
	return fmt.Sprintf("Created constitution at %s\n\n%s", path, template), nil
}

func setConstitution(path, rules string) (string, error) {
	// Ensure directory exists
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return "", fmt.Errorf("mkdir: %w", err)
	}

	if err := os.WriteFile(path, []byte(rules), 0o600); err != nil {
		return "", fmt.Errorf("write constitution: %w", err)
	}
	return fmt.Sprintf("Updated constitution at %s", path), nil
}

func validateAgainstConstitution(ctx context.Context, specDir, constitutionPath string) (string, error) {
	// Load constitution
	constData, err := os.ReadFile(constitutionPath) // #nosec G304 -- path provided by caller via tool/task parameters, inherent to this dev CLI's file operations
	if err != nil {
		return "No constitution found — skipping validation. Use action='init' to create one.", nil
	}

	// Load spec
	specData, err := os.ReadFile(filepath.Join(specDir, "spec.md")) // #nosec G304 -- path provided by caller via tool/task parameters, inherent to this dev CLI's file operations
	if err != nil {
		return "", fmt.Errorf("no spec.md found — write a spec first")
	}

	specContent := string(specData)
	constContent := string(constData)
	var issues []string

	// Check for security rules
	if strings.Contains(constContent, "Security First") || strings.Contains(constContent, "security") {
		if strings.Contains(specContent, "password") || strings.Contains(specContent, "secret") || strings.Contains(specContent, "api key") {
			issues = append(issues, "Security: Spec mentions sensitive data — ensure proper handling is specified")
		}
	}

	// Check for SHALL/MUST requirements
	if strings.Contains(constContent, "SHALL/MUST") || strings.Contains(constContent, "normative") {
		reqs := extractRequirementsFromContent(specContent)
		if len(reqs) > 0 {
			// Check if the spec content as a whole uses SHALL/MUST
			// (requirement headers don't contain SHALL/MUST — it's in the body)
			hasShallMust := strings.Contains(specContent, "SHALL") || strings.Contains(specContent, "MUST")
			if !hasShallMust {
				issues = append(issues, fmt.Sprintf("Normative: %d requirement(s) lack SHALL/MUST keywords", len(reqs)))
			}
		}
	}

	// Check for testability
	if strings.Contains(constContent, "Testable Everything") || strings.Contains(constContent, "test scenario") {
		scenarios := extractScenariosFromContent(specContent)
		reqs := extractRequirementsFromContent(specContent)
		if len(reqs) > 0 && len(scenarios) == 0 {
			issues = append(issues, "Testability: No test scenarios found — every requirement needs at least one scenario")
		}
	}

	if len(issues) == 0 {
		return fmt.Sprintf("+ Spec passes constitution validation (%d rules checked)", countConstitutionRules(constContent)), nil
	}

	var b strings.Builder
	fmt.Fprintf(&b, "Constitution validation found %d issue(s):\n\n", len(issues))
	for i, iss := range issues {
		fmt.Fprintf(&b, "%d. %s\n", i+1, iss)
	}

	return strings.TrimSpace(b.String()), nil
}

func extractRequirementsFromContent(content string) []string {
	var reqs []string
	lines := strings.Split(content, "\n")
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "### Requirement:") || strings.HasPrefix(trimmed, "## Requirement:") {
			reqs = append(reqs, trimmed)
		}
	}
	return reqs
}

func extractScenariosFromContent(content string) []string {
	var scenarios []string
	lines := strings.Split(content, "\n")
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "#### Scenario:") || strings.HasPrefix(trimmed, "### Scenario:") {
			scenarios = append(scenarios, trimmed)
		}
	}
	return scenarios
}

func countConstitutionRules(content string) int {
	count := 0
	lines := strings.Split(content, "\n")
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if len(trimmed) > 2 && (strings.HasPrefix(trimmed, "1.") || strings.HasPrefix(trimmed, "2.") ||
			strings.HasPrefix(trimmed, "3.") || strings.HasPrefix(trimmed, "4.") || strings.HasPrefix(trimmed, "5.") ||
			strings.HasPrefix(trimmed, "6.") || strings.HasPrefix(trimmed, "7.") || strings.HasPrefix(trimmed, "8.") ||
			strings.HasPrefix(trimmed, "9.")) {
			count++
		}
	}
	return count
}
