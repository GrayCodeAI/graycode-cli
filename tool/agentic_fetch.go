package tool

import (
	"context"
	"encoding/json"
	"fmt"
)

// AgenticFetchTool spawns a sub-agent to fetch, process, and summarize web content.
type AgenticFetchTool struct{}

func (AgenticFetchTool) Name() string      { return "AgenticFetch" }
func (AgenticFetchTool) RiskLevel() string  { return "low" }
func (AgenticFetchTool) Aliases() []string  { return []string{"agentic_fetch", "research"} }
func (AgenticFetchTool) Description() string {
	return "Fetch and intelligently summarize web content using a sub-agent. Better than raw WebFetch for research — the sub-agent extracts only the relevant information."
}
func (AgenticFetchTool) Parameters() map[string]interface{} {
	return map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"url":   map[string]interface{}{"type": "string", "description": "URL to fetch and analyze"},
			"query": map[string]interface{}{"type": "string", "description": "What to look for or extract from the page"},
		},
	}
}

func (AgenticFetchTool) Execute(ctx context.Context, input json.RawMessage) (string, error) {
	var p struct {
		URL   string `json:"url"`
		Query string `json:"query"`
	}
	if err := json.Unmarshal(input, &p); err != nil {
		return "", err
	}
	if p.URL == "" {
		return "", fmt.Errorf("url is required")
	}
	if p.Query == "" {
		p.Query = "Extract the key information from this page."
	}

	tc := GetToolContext(ctx)
	if tc == nil || tc.AgentSpawnFn == nil {
		// Fallback: do a direct fetch if no sub-agent available.
		return (WebFetchTool{}).Execute(ctx, input)
	}

	prompt := fmt.Sprintf(`You are a web research agent. Your task:
1. Fetch the URL: %s
2. Extract information relevant to: %s
3. Return a concise, well-structured summary of the relevant content.
4. Include specific details, code examples, or data points — not vague descriptions.

Use the WebFetch tool to retrieve the page content, then analyze and summarize it.`, p.URL, p.Query)

	return tc.AgentSpawnFn(ctx, prompt)
}
