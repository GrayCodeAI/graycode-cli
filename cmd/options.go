package cmd

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strings"

	hawkconfig "github.com/GrayCodeAI/hawk/config"
	"github.com/GrayCodeAI/hawk/engine"
	"github.com/GrayCodeAI/hawk/memory"
	hawkmodel "github.com/GrayCodeAI/hawk/routing"
	"github.com/GrayCodeAI/hawk/prompt"
	"github.com/GrayCodeAI/hawk/prompts"
	"github.com/GrayCodeAI/hawk/repomap"
	"github.com/GrayCodeAI/hawk/tool"
	"github.com/GrayCodeAI/eyrie/client"
)

func buildSystemPrompt() (string, error) {
	if systemPromptFlag != "" && systemPromptFile != "" {
		return "", fmt.Errorf("cannot use both --system-prompt and --system-prompt-file")
	}
	if appendSystemPromptFlag != "" && appendSystemPromptFile != "" {
		return "", fmt.Errorf("cannot use both --append-system-prompt and --append-system-prompt-file")
	}

	// Build modular template-based system prompt
	ctx := prompts.DefaultContext()

	// Gather workspace context and inject into prompt context
	cwd, _ := os.Getwd()
	ws := prompts.GatherWorkspaceContext(cwd)
	if ws != nil {
		ctx.GitBranch = ws.GitBranch
		ctx.GitStatus = ws.GitStatus
		if len(ws.RecentCommits) > 0 {
			ctx.RecentCommits = strings.Join(ws.RecentCommits, " / ")
		}
		if len(ws.TopFiles) > 0 {
			ctx.TopFiles = strings.Join(ws.TopFiles, " ")
		}
	}

	// Assemble modular prompt from templates (primary source for tools,
	// practices, communication). prompt.System() provides only the identity
	// preamble and system-level instructions the templates don't cover.
	modularPrompt, err := prompts.BuildSystemPrompt(ctx)
	if err != nil {
		// Fall back to preamble-only if templates fail
		modularPrompt = ""
	}

	base := prompt.System() + "\n\n" + hawkconfig.BuildContextWithDirs(addDirs)
	if modularPrompt != "" {
		base += "\n\n" + modularPrompt
	}
	if ws != nil {
		wsFormatted := ws.Format()
		if wsFormatted != "" {
			base += "\n\n" + wsFormatted
		}
	}

	if systemPromptFile != "" {
		data, err := os.ReadFile(systemPromptFile)
		if err != nil {
			return "", fmt.Errorf("read --system-prompt-file: %w", err)
		}
		base = string(data)
	} else if systemPromptFlag != "" {
		base = systemPromptFlag
	}

	appendPrompt := appendSystemPromptFlag
	if appendSystemPromptFile != "" {
		data, err := os.ReadFile(appendSystemPromptFile)
		if err != nil {
			return "", fmt.Errorf("read --append-system-prompt-file: %w", err)
		}
		appendPrompt = string(data)
	}
	if appendPrompt != "" {
		if base != "" {
			base += "\n\n"
		}
		base += appendPrompt
	}

	// Inject repo map into system prompt if enabled in settings.
	base = injectRepoMap(base)

	return base, nil
}

// injectRepoMap generates a repo map of the current directory and appends
// it to the system prompt when the repo_map setting is enabled.
func injectRepoMap(base string) string {
	settings := hawkconfig.LoadSettings()
	if settings.RepoMap == nil || !*settings.RepoMap {
		return base
	}
	maxTokens := settings.RepoMapMaxTokens
	if maxTokens <= 0 {
		maxTokens = 2000
	}
	cwd, err := os.Getwd()
	if err != nil {
		return base
	}
	rm, err := repomap.Generate(cwd, repomap.Options{
		MaxFiles:  500,
		MaxTokens: maxTokens,
	})
	if err != nil || rm == nil || len(rm.Files) == 0 {
		return base
	}
	formatted := rm.Format(maxTokens)
	if formatted == "" {
		return base
	}
	return base + "\n\n# Repository Map\n" + formatted
}

func loadEffectiveSettings() (hawkconfig.Settings, error) {
	settings, err := hawkconfig.LoadSettingsWithOverride(settingsFlag)
	if err != nil {
		return settings, err
	}
	// Register user-defined custom providers with eyrie and hawk model catalog.
	for _, cp := range settings.CustomProviders {
		if cp.Name == "" || cp.BaseURL == "" {
			continue
		}
		client.RegisterDynamicProvider(cp.Name, cp.BaseURL, cp.APIKeyEnv)
		if cp.Model != "" {
			hawkmodel.RegisterDynamic(hawkmodel.ModelInfo{
				Name:        cp.Model,
				Provider:    cp.Name,
				ContextSize: 128_000,
				Description: "Custom provider: " + cp.Name,
			})
		}
	}
	return settings, nil
}

func effectiveModelAndProvider(settings hawkconfig.Settings) (string, string) {
	effectiveModel := strings.TrimSpace(settings.Model)
	if strings.TrimSpace(model) != "" {
		effectiveModel = strings.TrimSpace(model)
	}
	effectiveProvider := strings.TrimSpace(settings.Provider)
	if strings.TrimSpace(provider) != "" {
		effectiveProvider = strings.TrimSpace(provider)
	}
	// Normalize hawk aliases (xai → grok) to eyrie canonical names
	return effectiveModel, hawkconfig.NormalizeProviderForEngine(effectiveProvider)
}

func configureSession(sess *engine.Session, settings hawkconfig.Settings) error {
	sess.WireAgentTool()
	sess.SetAllowedDirs(addDirs)

	// Initialize yaad memory bridge
	yaadMem := memory.NewYaadBridge()
	if yaadMem.Ready() {
		sess.Memory = yaadMem
		sess.YaadBridge = yaadMem
	}
	// Herm-style: API keys from environment only
	normalizedProvider := hawkconfig.NormalizeProviderForEngine(settings.Provider)
	if normalizedProvider != "" {
		if key := hawkconfig.APIKeyForProvider(normalizedProvider); key != "" {
			sess.SetAPIKey(normalizedProvider, key)
		}
	}
	sess.SetAPIKeys(hawkconfig.LoadAPIKeysFromEnv())

	for _, spec := range settings.AutoAllow {
		sess.Permissions.AllowSpec(spec)
	}
	for _, spec := range settings.AllowedTools {
		sess.Permissions.AllowSpec(spec)
	}
	for _, spec := range settings.DisallowedTools {
		sess.Permissions.DenySpec(spec)
	}
	for _, spec := range parseToolListFromCLI(allowedToolsFlag) {
		sess.Permissions.AllowSpec(spec)
	}
	for _, spec := range parseToolListFromCLI(disallowedToolsFlag) {
		sess.Permissions.DenySpec(spec)
	}

	mode := permissionMode
	if dangerouslySkipPermissions {
		mode = string(engine.PermissionModeBypassPermissions)
	}
	if err := sess.SetPermissionMode(mode); err != nil {
		return err
	}
	if err := sess.SetMaxTurns(maxTurns); err != nil {
		return err
	}

	budget := maxBudgetUSD
	if budget == 0 && settings.MaxBudgetUSD > 0 {
		budget = settings.MaxBudgetUSD
	}
	if err := sess.SetMaxBudgetUSD(budget); err != nil {
		return err
	}

	// Teach mode: augment system prompt with explanation instructions
	if teachMode {
		sess.AppendSystemContext("\n\n## Teaching Mode\n" + engine.TeachPromptAugment(teachDepth))
	}

	// Model cascade router: automatically routes tasks to optimal model tier
	roles := hawkmodel.DefaultRoles(sess.Model())
	if settings.ModelRoles != nil {
		roles = *settings.ModelRoles
	}
	sess.Cascade = engine.NewCascadeRouter(sess.Model(), roles)
	sess.Cascade.Enabled = true
	sess.Cascade.FrugalMode = settings.Frugal

	// Session lifecycle: self-improvement loop (learn from sessions)
	sess.Lifecycle = &engine.SessionLifecycle{
		Memory:     &engine.EvolvingMemoryAdapter{EM: memory.NewEvolvingMemory()},
		SkillStore: &engine.SkillDistillerAdapter{SD: sess.SkillDistiller},
	}

	return nil
}

func validateRootFlags() error {
	if outputFormat != "text" && outputFormat != "json" && outputFormat != "stream-json" {
		return fmt.Errorf("--output-format must be one of: text, json, stream-json")
	}
	if inputFormat != "text" && inputFormat != "stream-json" {
		return fmt.Errorf("--input-format must be one of: text, stream-json")
	}
	if inputFormat == "stream-json" && outputFormat != "stream-json" {
		return fmt.Errorf("--input-format=stream-json requires --output-format=stream-json")
	}
	if continueFlag && resumeID != "" {
		return fmt.Errorf("--continue and --resume cannot be used together")
	}
	if sessionIDFlag != "" && (continueFlag || resumeID != "") && !forkSessionFlag {
		return fmt.Errorf("--session-id can only be used with --continue or --resume when --fork-session is also specified")
	}
	if permissionMode != "" {
		var s engine.Session
		if err := s.SetPermissionMode(permissionMode); err != nil {
			return err
		}
	}
	if maxTurns < 0 {
		return fmt.Errorf("--max-turns must be non-negative")
	}
	if maxBudgetUSD < 0 {
		return fmt.Errorf("--max-budget-usd must be non-negative")
	}
	if systemPromptFlag != "" && systemPromptFile != "" {
		return fmt.Errorf("cannot use both --system-prompt and --system-prompt-file")
	}
	if appendSystemPromptFlag != "" && appendSystemPromptFile != "" {
		return fmt.Errorf("cannot use both --append-system-prompt and --append-system-prompt-file")
	}
	return nil
}

func readPromptFromStdin(format string) (string, error) {
	info, err := os.Stdin.Stat()
	if err != nil {
		return "", err
	}
	if info.Mode()&os.ModeCharDevice != 0 {
		return "", nil
	}
	data, err := io.ReadAll(os.Stdin)
	if err != nil {
		return "", err
	}
	if format == "stream-json" {
		return promptFromStreamJSON(data)
	}
	return strings.TrimRight(string(data), "\r\n"), nil
}

func promptFromStreamJSON(data []byte) (string, error) {
	scanner := bufio.NewScanner(strings.NewReader(string(data)))
	scanner.Buffer(make([]byte, 0, 64*1024), 10*1024*1024)
	var parts []string
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}
		text, err := promptFromStreamJSONLine(line)
		if err != nil {
			return "", err
		}
		if text != "" {
			parts = append(parts, text)
		}
	}
	if err := scanner.Err(); err != nil {
		return "", err
	}
	return strings.Join(parts, "\n"), nil
}

func promptFromStreamJSONLine(line string) (string, error) {
	var obj map[string]json.RawMessage
	if err := json.Unmarshal([]byte(line), &obj); err != nil {
		return "", fmt.Errorf("invalid stream-json input: %w", err)
	}

	eventType := jsonString(obj["type"])
	switch eventType {
	case "", "user", "user_message", "message", "prompt":
	default:
		return "", nil
	}
	for _, key := range []string{"prompt", "content", "text"} {
		if s := jsonString(obj[key]); s != "" {
			return s, nil
		}
	}
	if raw, ok := obj["message"]; ok {
		if s := jsonString(raw); s != "" {
			return s, nil
		}
		var nested map[string]json.RawMessage
		if json.Unmarshal(raw, &nested) == nil {
			for _, key := range []string{"content", "text", "prompt"} {
				if s := jsonString(nested[key]); s != "" {
					return s, nil
				}
			}
		}
	}
	return "", nil
}

func jsonString(raw json.RawMessage) string {
	if len(raw) == 0 {
		return ""
	}
	var s string
	if err := json.Unmarshal(raw, &s); err == nil {
		return s
	}
	return ""
}

func parseToolListFromCLI(values []string) []string {
	if len(values) == 0 {
		return nil
	}
	var result []string
	for _, value := range values {
		if value == "" {
			continue
		}
		var current strings.Builder
		depth := 0
		for _, r := range value {
			switch r {
			case '(':
				depth++
				current.WriteRune(r)
			case ')':
				if depth > 0 {
					depth--
				}
				current.WriteRune(r)
			case ',', ' ', '\t', '\n':
				if depth > 0 {
					current.WriteRune(r)
					continue
				}
				if spec := strings.TrimSpace(current.String()); spec != "" {
					result = append(result, spec)
				}
				current.Reset()
			default:
				current.WriteRune(r)
			}
		}
		if spec := strings.TrimSpace(current.String()); spec != "" {
			result = append(result, spec)
		}
	}
	return result
}

func filterAvailableTools(all []tool.Tool, toolsSpecified bool, toolSpecs []string, disallowedSpecs []string) ([]tool.Tool, error) {
	selected := all
	if toolsSpecified {
		if len(toolSpecs) == 0 {
			return nil, nil
		}
		if len(toolSpecs) == 1 && strings.EqualFold(toolSpecs[0], "default") {
			selected = all
		} else {
			var filtered []tool.Tool
			seen := make(map[string]bool)
			for _, spec := range toolSpecs {
				if strings.EqualFold(spec, "default") {
					for _, t := range all {
						if !seen[t.Name()] {
							filtered = append(filtered, t)
							seen[t.Name()] = true
						}
					}
					continue
				}
				match := findToolBySpec(all, spec)
				if match == nil {
					return nil, fmt.Errorf("unknown tool in --tools: %s", spec)
				}
				if !seen[match.Name()] {
					filtered = append(filtered, match)
					seen[match.Name()] = true
				}
			}
			selected = filtered
		}
	}

	for _, spec := range disallowedSpecs {
		if toolSpecHasPattern(spec) {
			continue
		}
		selected = removeToolBySpec(selected, spec)
	}
	return selected, nil
}

func findToolBySpec(tools []tool.Tool, spec string) tool.Tool {
	name := toolSpecName(spec)
	for _, t := range tools {
		if toolNameMatches(t, name) {
			return t
		}
	}
	return nil
}

func removeToolBySpec(tools []tool.Tool, spec string) []tool.Tool {
	name := toolSpecName(spec)
	out := tools[:0]
	for _, t := range tools {
		if !toolNameMatches(t, name) {
			out = append(out, t)
		}
	}
	return out
}

func toolNameMatches(t tool.Tool, name string) bool {
	if strings.EqualFold(t.Name(), name) {
		return true
	}
	aliased, ok := t.(tool.AliasedTool)
	if !ok {
		return false
	}
	for _, alias := range aliased.Aliases() {
		if strings.EqualFold(alias, name) {
			return true
		}
	}
	return false
}

func toolSpecName(spec string) string {
	spec = strings.TrimSpace(spec)
	if open := strings.Index(spec, "("); open >= 0 {
		return strings.TrimSpace(spec[:open])
	}
	return spec
}

func toolSpecHasPattern(spec string) bool {
	spec = strings.TrimSpace(spec)
	open := strings.Index(spec, "(")
	return open >= 0 && strings.HasSuffix(spec, ")") && strings.TrimSpace(spec[open+1:len(spec)-1]) != ""
}
