// Package llm provides LLM provider abstractions.
//
// LLM Provider interface - the abstract interface for LLM providers.
// Each provider implementation hides:
// - API client initialization and authentication
// - Request/response format conversion
// - Provider-specific error handling
// - Rate limiting and retry logic

package llm

import (
	"context"
)

// Provider defines the abstract interface for LLM providers.
// Implementations hide provider-specific details while exposing
// a consistent interface for chat completions.
type Provider interface {
	// Name returns the provider name (for logging/debugging).
	Name() string

	// Model returns the current model being used.
	Model() string

	// Chat sends a chat completion request.
	Chat(ctx context.Context, messages []ChatMessage) (LLMResponse, error)

	// ChatWithFormat sends a chat completion request with response format.
	ChatWithFormat(ctx context.Context, messages []ChatMessage, format *ResponseFormat) (LLMResponse, error)

	// ChatWithTools sends a chat completion request with tool definitions.
	// The LLM may respond with tool calls in LLMResponse.ToolCalls.
	ChatWithTools(ctx context.Context, messages []ChatMessage, tools []ToolDefinition) (LLMResponse, error)

	// StreamChat streams a chat completion, sending chunks to the provided channel.
	// Returns token usage (available in final chunk when supported by provider).
	StreamChat(ctx context.Context, messages []ChatMessage, chunks chan<- string) (*TokenUsage, error)
}
