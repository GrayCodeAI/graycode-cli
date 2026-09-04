package engine

import (
	"context"
	"errors"

	"github.com/GrayCodeAI/graycode-cli/internal/types"
)

// NewUnavailableChatClient preserves Session construction while surfacing
// GraycodeRouter transport setup failures at the first chat call.
func NewUnavailableChatClient(err error) ChatClient {
	if err == nil {
		err = errors.New("graycode: chat transport unavailable")
	}
	return &unavailableChatClient{err: err}
}

type unavailableChatClient struct {
	err error
}

func (c *unavailableChatClient) Chat(context.Context, []types.GraycodeRouterMessage, types.ChatOptions) (*types.GraycodeRouterResponse, error) {
	return nil, c.err
}

func (c *unavailableChatClient) StreamChatContinue(context.Context, []types.GraycodeRouterMessage, types.ChatOptions, types.ContinuationConfig) (*types.StreamResult, error) {
	return nil, c.err
}
