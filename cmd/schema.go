package cmd

import (
	"encoding/json"
	"fmt"

	"github.com/spf13/cobra"
)

var schemaCmd = &cobra.Command{
	Use:   "schema",
	Short: "Output JSON schema for hawk settings.json",
	Long:  "Prints the JSON schema for hawk's settings.json configuration file. Use with $schema for IDE autocompletion.",
	RunE: func(cmd *cobra.Command, args []string) error {
		schema := map[string]interface{}{
			"$schema":     "http://json-schema.org/draft-07/schema#",
			"title":       "Hawk Settings",
			"description": "Configuration for the Hawk AI coding agent",
			"type":        "object",
			"properties": map[string]interface{}{
				"model":    map[string]interface{}{"type": "string", "description": "Default model (e.g. claude-sonnet-4-20250514)"},
				"provider": map[string]interface{}{"type": "string", "description": "Default provider (anthropic, openai, gemini, etc.)"},
				"theme":    map[string]interface{}{"type": "string", "enum": []string{"dark", "light", "auto"}},
				"auto_allow": map[string]interface{}{
					"type": "array", "items": map[string]interface{}{"type": "string"},
					"description": "Tools to always allow without prompting",
				},
				"allowedTools": map[string]interface{}{
					"type": "array", "items": map[string]interface{}{"type": "string"},
					"description": "Tool permission allow rules (e.g. Bash(git:*))",
				},
				"disallowedTools": map[string]interface{}{
					"type": "array", "items": map[string]interface{}{"type": "string"},
					"description": "Tool permission deny rules",
				},
				"max_budget_usd": map[string]interface{}{"type": "number", "description": "Cost cap per session in USD"},
				"mcp_servers": map[string]interface{}{
					"type": "array",
					"items": map[string]interface{}{
						"type": "object",
						"properties": map[string]interface{}{
							"name":    map[string]interface{}{"type": "string"},
							"command": map[string]interface{}{"type": "string", "description": "Command for stdio transport"},
							"args":    map[string]interface{}{"type": "array", "items": map[string]interface{}{"type": "string"}},
							"type":    map[string]interface{}{"type": "string", "enum": []string{"stdio", "sse", "http"}, "default": "stdio"},
							"url":     map[string]interface{}{"type": "string", "description": "URL for sse/http transport"},
							"headers": map[string]interface{}{"type": "object", "additionalProperties": map[string]interface{}{"type": "string"}},
						},
					},
				},
				"sandbox":    map[string]interface{}{"type": "string", "enum": []string{"strict", "workspace", "off"}},
				"auto_commit": map[string]interface{}{"type": "boolean"},
				"autonomy":   map[string]interface{}{"type": "integer", "minimum": 0, "maximum": 4},
				"attribution": map[string]interface{}{
					"type": "object",
					"properties": map[string]interface{}{
						"trailer_style":  map[string]interface{}{"type": "string", "enum": []string{"none", "co-authored-by", "assisted-by"}, "default": "assisted-by"},
						"generated_with": map[string]interface{}{"type": "boolean", "description": "Append 'Generated with Hawk' to commits"},
					},
				},
				"repo_map":           map[string]interface{}{"type": "boolean"},
				"repo_map_max_tokens": map[string]interface{}{"type": "integer"},
				"auto_compact_threshold_pct": map[string]interface{}{"type": "integer", "default": 85},
			},
		}
		data, _ := json.MarshalIndent(schema, "", "  ")
		fmt.Println(string(data))
		return nil
	},
}

func init() {
	rootCmd.AddCommand(schemaCmd)
}
