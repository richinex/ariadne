// LLMClient - Simple wrapper around providers.

package llm

import (
	"context"
)

// Client wraps a Provider with a simple interface.
type Client struct {
	provider Provider
}

// NewClient creates a new LLM client from a provider.
func NewClient(provider Provider) *Client {
	return &Client{provider: provider}
}

// Chat sends a chat completion request and returns just the content.
func (c *Client) Chat(ctx context.Context, messages []ChatMessage) (string, error) {
	response, err := c.provider.Chat(ctx, messages)
	if err != nil {
		return "", err
	}
	return response.Content, nil
}

// ChatWithUsage sends a chat completion request and returns content with token usage.
func (c *Client) ChatWithUsage(ctx context.Context, messages []ChatMessage) (string, *TokenUsage, error) {
	response, err := c.provider.Chat(ctx, messages)
	if err != nil {
		return "", nil, err
	}
	return response.Content, response.Usage, nil
}

// ChatWithFormat sends a chat completion request with response format
// and returns just the content.
func (c *Client) ChatWithFormat(ctx context.Context, messages []ChatMessage, format *ResponseFormat) (string, error) {
	response, err := c.provider.ChatWithFormat(ctx, messages, format)
	if err != nil {
		return "", err
	}
	return response.Content, nil
}

// StreamChat streams a chat completion.
func (c *Client) StreamChat(ctx context.Context, messages []ChatMessage, chunks chan<- string) (*TokenUsage, error) {
	return c.provider.StreamChat(ctx, messages, chunks)
}

// Provider returns the underlying provider.
func (c *Client) Provider() Provider {
	return c.provider
}
