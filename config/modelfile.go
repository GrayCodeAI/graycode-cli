package config

import (
	"fmt"
	"os"
	"strconv"
	"strings"
	"sync"
)

// ModelMessage represents a conversation example in a Modelfile.
type ModelMessage struct {
	Role    string
	Content string
}

// ModelConfig holds the parsed configuration from a Modelfile.
type ModelConfig struct {
	From       string
	Parameters map[string]interface{}
	System     string
	Template   string
	Messages   []ModelMessage
	License    string
	Adapters   []string
}

// ModelfileParser parses Modelfile DSL content into ModelConfig.
type ModelfileParser struct {
	mu sync.Mutex
}

// NewModelfileParser creates a new ModelfileParser instance.
func NewModelfileParser() *ModelfileParser {
	return &ModelfileParser{}
}

// validParameters defines the set of recognized parameter names.
var validParameters = map[string]bool{
	"temperature":       true,
	"top_p":             true,
	"top_k":             true,
	"max_tokens":        true,
	"stop":              true,
	"seed":              true,
	"num_ctx":           true,
	"repeat_penalty":    true,
	"presence_penalty":  true,
	"frequency_penalty": true,
}

// validRoles defines the set of valid message roles.
var validRoles = map[string]bool{
	"system":    true,
	"user":      true,
	"assistant": true,
}

// Parse parses Modelfile DSL content and returns a ModelConfig.
func (p *ModelfileParser) Parse(content string) (*ModelConfig, error) {
	p.mu.Lock()
	defer p.mu.Unlock()

	cfg := &ModelConfig{
		Parameters: make(map[string]interface{}),
	}

	lines := strings.Split(content, "\n")
	i := 0
	for i < len(lines) {
		line := strings.TrimSpace(lines[i])

		// Skip empty lines and comments.
		if line == "" || strings.HasPrefix(line, "#") {
			i++
			continue
		}

		// Extract directive and argument.
		directive, arg := splitDirective(line)
		directive = strings.ToUpper(directive)

		switch directive {
		case "FROM":
			if arg == "" {
				return nil, fmt.Errorf("line %d: FROM requires a model name", i+1)
			}
			cfg.From = arg

		case "PARAMETER":
			name, value, err := parseParameter(arg)
			if err != nil {
				return nil, fmt.Errorf("line %d: %w", i+1, err)
			}
			cfg.Parameters[name] = value

		case "SYSTEM":
			text, advance, err := parseTextValue(arg, lines, i)
			if err != nil {
				return nil, fmt.Errorf("line %d: %w", i+1, err)
			}
			cfg.System = text
			i += advance
			continue

		case "TEMPLATE":
			text, advance, err := parseTextValue(arg, lines, i)
			if err != nil {
				return nil, fmt.Errorf("line %d: %w", i+1, err)
			}
			cfg.Template = text
			i += advance
			continue

		case "MESSAGE":
			role, msgContent, err := parseMessage(arg)
			if err != nil {
				return nil, fmt.Errorf("line %d: %w", i+1, err)
			}
			cfg.Messages = append(cfg.Messages, ModelMessage{Role: role, Content: msgContent})

		case "LICENSE":
			text, advance, err := parseTextValue(arg, lines, i)
			if err != nil {
				return nil, fmt.Errorf("line %d: %w", i+1, err)
			}
			cfg.License = text
			i += advance
			continue

		case "ADAPTER":
			if arg == "" {
				return nil, fmt.Errorf("line %d: ADAPTER requires a path", i+1)
			}
			cfg.Adapters = append(cfg.Adapters, arg)

		default:
			return nil, fmt.Errorf("line %d: unknown directive %q", i+1, directive)
		}

		i++
	}

	return cfg, nil
}

// ParseFile reads a Modelfile from disk and parses it.
func (p *ModelfileParser) ParseFile(path string) (*ModelConfig, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("reading modelfile: %w", err)
	}
	return p.Parse(string(data))
}

// Validate checks the ModelConfig for errors and returns a list of issues.
func (p *ModelfileParser) Validate(config *ModelConfig) []string {
	var issues []string

	if config.From == "" {
		issues = append(issues, "FROM is required")
	}

	if temp, ok := config.Parameters["temperature"]; ok {
		if v, err := toFloat64(temp); err == nil {
			if v < 0 || v > 2 {
				issues = append(issues, "temperature must be between 0 and 2")
			}
		}
	}

	if topP, ok := config.Parameters["top_p"]; ok {
		if v, err := toFloat64(topP); err == nil {
			if v < 0 || v > 1 {
				issues = append(issues, "top_p must be between 0 and 1")
			}
		}
	}

	for name := range config.Parameters {
		if !validParameters[name] {
			issues = append(issues, fmt.Sprintf("unknown parameter %q", name))
		}
	}

	for _, msg := range config.Messages {
		if !validRoles[msg.Role] {
			issues = append(issues, fmt.Sprintf("invalid message role %q", msg.Role))
		}
	}

	return issues
}

// Render converts a ModelConfig back into Modelfile format.
func (p *ModelfileParser) Render(config *ModelConfig) string {
	var b strings.Builder

	if config.From != "" {
		b.WriteString("FROM ")
		b.WriteString(config.From)
		b.WriteString("\n")
	}

	for name, value := range config.Parameters {
		b.WriteString("PARAMETER ")
		b.WriteString(name)
		b.WriteString(" ")
		b.WriteString(formatParamValue(value))
		b.WriteString("\n")
	}

	if config.System != "" {
		b.WriteString("SYSTEM ")
		b.WriteString(quoteMultiline(config.System))
		b.WriteString("\n")
	}

	if config.Template != "" {
		b.WriteString("TEMPLATE ")
		b.WriteString(quoteMultiline(config.Template))
		b.WriteString("\n")
	}

	for _, msg := range config.Messages {
		b.WriteString("MESSAGE ")
		b.WriteString(msg.Role)
		b.WriteString(" ")
		b.WriteString(quoteValue(msg.Content))
		b.WriteString("\n")
	}

	if config.License != "" {
		b.WriteString("LICENSE ")
		b.WriteString(quoteValue(config.License))
		b.WriteString("\n")
	}

	for _, adapter := range config.Adapters {
		b.WriteString("ADAPTER ")
		b.WriteString(adapter)
		b.WriteString("\n")
	}

	return b.String()
}

// ToProviderConfig converts a ModelConfig into an eyrie-compatible provider configuration map.
func (p *ModelfileParser) ToProviderConfig(config *ModelConfig) map[string]interface{} {
	result := map[string]interface{}{
		"model": config.From,
	}

	if config.System != "" {
		result["system_prompt"] = config.System
	}

	if len(config.Parameters) > 0 {
		params := make(map[string]interface{})
		for k, v := range config.Parameters {
			params[k] = v
		}
		result["parameters"] = params
	}

	if len(config.Messages) > 0 {
		msgs := make([]map[string]string, 0, len(config.Messages))
		for _, m := range config.Messages {
			msgs = append(msgs, map[string]string{
				"role":    m.Role,
				"content": m.Content,
			})
		}
		result["messages"] = msgs
	}

	if config.Template != "" {
		result["template"] = config.Template
	}

	if len(config.Adapters) > 0 {
		result["adapters"] = config.Adapters
	}

	return result
}

// MergeConfigs merges override into base. Override takes precedence for same parameters.
func (p *ModelfileParser) MergeConfigs(base, override *ModelConfig) *ModelConfig {
	merged := &ModelConfig{
		From:       base.From,
		Parameters: make(map[string]interface{}),
		System:     base.System,
		Template:   base.Template,
		License:    base.License,
	}

	// Copy base parameters.
	for k, v := range base.Parameters {
		merged.Parameters[k] = v
	}

	// Copy base messages.
	merged.Messages = append(merged.Messages, base.Messages...)

	// Copy base adapters.
	merged.Adapters = append(merged.Adapters, base.Adapters...)

	// Apply overrides.
	if override.From != "" {
		merged.From = override.From
	}
	if override.System != "" {
		merged.System = override.System
	}
	if override.Template != "" {
		merged.Template = override.Template
	}
	if override.License != "" {
		merged.License = override.License
	}

	for k, v := range override.Parameters {
		merged.Parameters[k] = v
	}

	if len(override.Messages) > 0 {
		merged.Messages = override.Messages
	}

	if len(override.Adapters) > 0 {
		merged.Adapters = override.Adapters
	}

	return merged
}

// DefaultModelConfigs returns built-in configurations for common use cases.
func DefaultModelConfigs() map[string]*ModelConfig {
	return map[string]*ModelConfig{
		"coding": {
			From: "claude-sonnet-4-6",
			Parameters: map[string]interface{}{
				"temperature": 0.2,
				"max_tokens":  4096,
			},
			System: "You are a coding assistant. Write clean, efficient, and well-documented code. Follow best practices and established patterns in the codebase.",
		},
		"creative": {
			From: "claude-sonnet-4-6",
			Parameters: map[string]interface{}{
				"temperature": 0.9,
				"max_tokens":  4096,
			},
			System: "You are a creative writing assistant. Be imaginative, expressive, and original in your responses.",
		},
		"precise": {
			From: "claude-sonnet-4-6",
			Parameters: map[string]interface{}{
				"temperature": 0.0,
				"top_p":       0.1,
				"max_tokens":  4096,
			},
			System: "You are a precise assistant. Provide accurate, factual, and concise responses. Avoid speculation.",
		},
	}
}

// FormatConfig produces a human-readable summary of a ModelConfig.
func FormatConfig(config *ModelConfig) string {
	var b strings.Builder

	b.WriteString("Model Configuration:\n")

	if config.From != "" {
		b.WriteString(fmt.Sprintf("  Base: %s\n", config.From))
	}

	if len(config.Parameters) > 0 {
		parts := make([]string, 0, len(config.Parameters))
		for k, v := range config.Parameters {
			parts = append(parts, fmt.Sprintf("%s=%v", k, v))
		}
		b.WriteString(fmt.Sprintf("  Parameters: %s\n", strings.Join(parts, ", ")))
	}

	if config.System != "" {
		sys := config.System
		if len(sys) > 50 {
			sys = sys[:50] + "..."
		}
		b.WriteString(fmt.Sprintf("  System: %q\n", sys))
	}

	if len(config.Messages) > 0 {
		b.WriteString(fmt.Sprintf("  Messages: %d examples\n", len(config.Messages)))
	}

	if len(config.Adapters) > 0 {
		b.WriteString(fmt.Sprintf("  Adapters: %d\n", len(config.Adapters)))
	}

	if config.License != "" {
		b.WriteString(fmt.Sprintf("  License: %s\n", config.License))
	}

	return b.String()
}

// --- internal helpers ---

// splitDirective splits a line into its directive keyword and the remaining argument.
func splitDirective(line string) (string, string) {
	idx := strings.IndexByte(line, ' ')
	if idx < 0 {
		return line, ""
	}
	return line[:idx], strings.TrimSpace(line[idx+1:])
}

// parseParameter parses "name value" from a PARAMETER directive argument.
func parseParameter(arg string) (string, interface{}, error) {
	if arg == "" {
		return "", nil, fmt.Errorf("PARAMETER requires name and value")
	}

	parts := strings.SplitN(arg, " ", 2)
	if len(parts) < 2 {
		return "", nil, fmt.Errorf("PARAMETER requires name and value")
	}

	name := strings.TrimSpace(parts[0])
	rawValue := strings.TrimSpace(parts[1])

	value := parseValue(rawValue)
	return name, value, nil
}

// parseValue attempts to interpret a string as a number, bool, or quoted string.
func parseValue(raw string) interface{} {
	// Quoted string.
	if len(raw) >= 2 && raw[0] == '"' && raw[len(raw)-1] == '"' {
		return raw[1 : len(raw)-1]
	}

	// Integer.
	if i, err := strconv.ParseInt(raw, 10, 64); err == nil {
		return int(i)
	}

	// Float.
	if f, err := strconv.ParseFloat(raw, 64); err == nil {
		return f
	}

	// Bool.
	if b, err := strconv.ParseBool(raw); err == nil {
		return b
	}

	return raw
}

// parseTextValue handles single-line quoted or triple-quoted multi-line text.
func parseTextValue(arg string, lines []string, currentLine int) (string, int, error) {
	// Triple-quoted multi-line.
	if strings.HasPrefix(arg, `"""`) {
		content := strings.TrimPrefix(arg, `"""`)
		// Check if closing triple-quote is on same line.
		if strings.HasSuffix(content, `"""`) {
			return strings.TrimSuffix(content, `"""`), 1, nil
		}

		var parts []string
		if content != "" {
			parts = append(parts, content)
		}

		for j := currentLine + 1; j < len(lines); j++ {
			l := lines[j]
			if strings.Contains(l, `"""`) {
				// Found closing triple quote.
				before := strings.TrimSuffix(strings.TrimSpace(l), `"""`)
				if before != "" {
					parts = append(parts, before)
				}
				return strings.Join(parts, "\n"), j - currentLine + 1, nil
			}
			parts = append(parts, l)
		}
		return "", 0, fmt.Errorf("unterminated triple-quoted string")
	}

	// Single-line quoted.
	if len(arg) >= 2 && arg[0] == '"' && arg[len(arg)-1] == '"' {
		return arg[1 : len(arg)-1], 1, nil
	}

	// Unquoted single-line.
	return arg, 1, nil
}

// parseMessage parses "role content" from a MESSAGE directive argument.
func parseMessage(arg string) (string, string, error) {
	if arg == "" {
		return "", "", fmt.Errorf("MESSAGE requires role and content")
	}

	parts := strings.SplitN(arg, " ", 2)
	if len(parts) < 2 {
		return "", "", fmt.Errorf("MESSAGE requires role and content")
	}

	role := strings.TrimSpace(parts[0])
	rawContent := strings.TrimSpace(parts[1])

	// Strip surrounding quotes if present.
	if len(rawContent) >= 2 && rawContent[0] == '"' && rawContent[len(rawContent)-1] == '"' {
		rawContent = rawContent[1 : len(rawContent)-1]
	}

	return role, rawContent, nil
}

// formatParamValue formats a parameter value for rendering.
func formatParamValue(v interface{}) string {
	switch val := v.(type) {
	case string:
		return fmt.Sprintf("%q", val)
	case float64:
		if val == float64(int64(val)) {
			return strconv.FormatInt(int64(val), 10)
		}
		return strconv.FormatFloat(val, 'f', -1, 64)
	case int:
		return strconv.Itoa(val)
	case int64:
		return strconv.FormatInt(val, 10)
	case bool:
		return strconv.FormatBool(val)
	default:
		return fmt.Sprintf("%v", val)
	}
}

// quoteValue wraps a string in double quotes.
func quoteValue(s string) string {
	return fmt.Sprintf("%q", s)
}

// quoteMultiline uses triple quotes for multi-line strings, regular quotes otherwise.
func quoteMultiline(s string) string {
	if strings.Contains(s, "\n") {
		return `"""` + s + `"""`
	}
	return fmt.Sprintf("%q", s)
}

// toFloat64 converts a numeric interface value to float64.
func toFloat64(v interface{}) (float64, error) {
	switch val := v.(type) {
	case float64:
		return val, nil
	case int:
		return float64(val), nil
	case int64:
		return float64(val), nil
	case string:
		return strconv.ParseFloat(val, 64)
	default:
		return 0, fmt.Errorf("cannot convert %T to float64", v)
	}
}
