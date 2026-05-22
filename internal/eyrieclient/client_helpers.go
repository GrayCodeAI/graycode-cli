package eyrieclient

import "github.com/GrayCodeAI/eyrie/client"

type (
	EyrieMessage       = client.EyrieMessage
	EyrieUsage         = client.EyrieUsage
	ToolCall           = client.ToolCall
	ToolResult         = client.ToolResult
	ChatOptions        = client.ChatOptions
	StreamResult       = client.StreamResult
	EyrieResponse      = client.EyrieResponse
	EyrieStreamEvent   = client.EyrieStreamEvent
	EyrieTool          = client.EyrieTool
	ContinuationConfig = client.ContinuationConfig
	EyrieConfig        = client.EyrieConfig
)

func DefaultContinuationConfig() ContinuationConfig {
	return client.DefaultContinuationConfig()
}

func DetectProvider() string {
	return client.DetectProvider()
}

func RegisterDynamicProvider(name, baseURL, apiKeyEnv string) {
	client.RegisterDynamicProvider(name, baseURL, apiKeyEnv)
}

func GetProviderNames() []string {
	return client.Client(nil).GetProviders()
}

func Client(config *EyrieConfig) *client.EyrieClient {
	return client.Client(config)
}
