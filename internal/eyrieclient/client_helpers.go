package eyrieclient

import "github.com/GrayCodeAI/eyrie/client"

type EyrieMessage = client.EyrieMessage
type EyrieUsage = client.EyrieUsage
type ToolCall = client.ToolCall
type ToolResult = client.ToolResult
type ChatOptions = client.ChatOptions
type StreamResult = client.StreamResult
type EyrieResponse = client.EyrieResponse
type EyrieStreamEvent = client.EyrieStreamEvent
type EyrieTool = client.EyrieTool
type ContinuationConfig = client.ContinuationConfig
type EyrieConfig = client.EyrieConfig

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
