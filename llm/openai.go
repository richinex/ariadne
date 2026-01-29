// OpenAI Provider implementation using go-openai library.
//
// Information Hiding:
// - API endpoint and authentication
// - Request/response format for OpenAI Chat Completions API
// - Streaming via go-openai library

package llm

import (
	"context"
	"errors"
	"fmt"
	"io"

	openai "github.com/sashabaranov/go-openai"
)

// OpenAIProvider implements the Provider interface for OpenAI.
type OpenAIProvider struct {
	client      *openai.Client
	model       string
	maxTokens   int
	temperature float32
}

// NewOpenAIProvider creates a new OpenAI provider.
func NewOpenAIProvider(apiKey, model string, maxTokens uint32, temperature float32) *OpenAIProvider {
	return &OpenAIProvider{
		client:      openai.NewClient(apiKey),
		model:       model,
		maxTokens:   int(maxTokens),
		temperature: temperature,
	}
}

// Name returns the provider name.
func (p *OpenAIProvider) Name() string {
	return "openai"
}

// Model returns the current model.
func (p *OpenAIProvider) Model() string {
	return p.model
}

// Chat sends a chat completion request.
func (p *OpenAIProvider) Chat(ctx context.Context, messages []ChatMessage) (LLMResponse, error) {
	return p.ChatWithFormat(ctx, messages, nil)
}

// ChatWithFormat sends a chat completion request with optional response format.
func (p *OpenAIProvider) ChatWithFormat(ctx context.Context, messages []ChatMessage, format *ResponseFormat) (LLMResponse, error) {
	req := openai.ChatCompletionRequest{
		Model:       p.model,
		Messages:    convertToOpenAIMessages(messages),
		MaxTokens:   p.maxTokens,
		Temperature: p.temperature,
	}

	if format != nil {
		req.ResponseFormat = &openai.ChatCompletionResponseFormat{
			Type: openai.ChatCompletionResponseFormatType(format.Type),
		}
	}

	resp, err := p.client.CreateChatCompletion(ctx, req)
	if err != nil {
		return LLMResponse{}, fmt.Errorf("chat completion failed: %w", err)
	}

	content := ""
	if len(resp.Choices) > 0 {
		content = resp.Choices[0].Message.Content
	}

	usage := &TokenUsage{
		PromptTokens:     uint32(resp.Usage.PromptTokens),
		CompletionTokens: uint32(resp.Usage.CompletionTokens),
		TotalTokens:      uint32(resp.Usage.TotalTokens),
	}

	return LLMResponse{Content: content, Usage: usage}, nil
}

// ChatWithTools sends a chat completion request with tool definitions.
func (p *OpenAIProvider) ChatWithTools(ctx context.Context, messages []ChatMessage, tools []ToolDefinition) (LLMResponse, error) {
	req := openai.ChatCompletionRequest{
		Model:       p.model,
		Messages:    convertToOpenAIMessagesWithTools(messages),
		MaxTokens:   p.maxTokens,
		Temperature: p.temperature,
		Tools:       convertToOpenAITools(tools),
	}

	resp, err := p.client.CreateChatCompletion(ctx, req)
	if err != nil {
		return LLMResponse{}, fmt.Errorf("chat completion failed: %w", err)
	}

	content := ""
	var toolCalls []ToolCall
	if len(resp.Choices) > 0 {
		content = resp.Choices[0].Message.Content
		// Convert OpenAI tool calls to our format
		for _, tc := range resp.Choices[0].Message.ToolCalls {
			toolCalls = append(toolCalls, ToolCall{
				ID:        tc.ID,
				Name:      tc.Function.Name,
				Arguments: []byte(tc.Function.Arguments),
			})
		}
	}

	usage := &TokenUsage{
		PromptTokens:     uint32(resp.Usage.PromptTokens),
		CompletionTokens: uint32(resp.Usage.CompletionTokens),
		TotalTokens:      uint32(resp.Usage.TotalTokens),
	}

	return LLMResponse{Content: content, ToolCalls: toolCalls, Usage: usage}, nil
}

// StreamChat streams a chat completion.
func (p *OpenAIProvider) StreamChat(ctx context.Context, messages []ChatMessage, chunks chan<- string) (*TokenUsage, error) {
	req := openai.ChatCompletionRequest{
		Model:       p.model,
		Messages:    convertToOpenAIMessages(messages),
		MaxTokens:   p.maxTokens,
		Temperature: p.temperature,
		Stream:      true,
		StreamOptions: &openai.StreamOptions{
			IncludeUsage: true,
		},
	}

	stream, err := p.client.CreateChatCompletionStream(ctx, req)
	if err != nil {
		return nil, fmt.Errorf("stream creation failed: %w", err)
	}
	defer stream.Close()

	var usage *TokenUsage
	for {
		response, err := stream.Recv()
		if errors.Is(err, io.EOF) {
			return usage, nil
		}
		if err != nil {
			return usage, fmt.Errorf("stream recv failed: %w", err)
		}

		// Capture token usage from final chunk
		if response.Usage != nil {
			usage = &TokenUsage{
				PromptTokens:     uint32(response.Usage.PromptTokens),
				CompletionTokens: uint32(response.Usage.CompletionTokens),
				TotalTokens:      uint32(response.Usage.TotalTokens),
			}
		}

		if len(response.Choices) > 0 {
			content := response.Choices[0].Delta.Content
			if content != "" {
				select {
				case chunks <- content:
				case <-ctx.Done():
					return usage, ctx.Err()
				}
			}
		}
	}
}

// convertToOpenAIMessages converts our ChatMessage to openai.ChatCompletionMessage
func convertToOpenAIMessages(messages []ChatMessage) []openai.ChatCompletionMessage {
	result := make([]openai.ChatCompletionMessage, len(messages))
	for i, msg := range messages {
		result[i] = openai.ChatCompletionMessage{
			Role:    msg.Role,
			Content: msg.Content,
		}
	}
	return result
}

// convertToOpenAIMessagesWithTools handles tool calls and tool responses.
func convertToOpenAIMessagesWithTools(messages []ChatMessage) []openai.ChatCompletionMessage {
	result := make([]openai.ChatCompletionMessage, len(messages))
	for i, msg := range messages {
		oaiMsg := openai.ChatCompletionMessage{
			Role:    msg.Role,
			Content: msg.Content,
		}

		// Handle tool calls from assistant
		if len(msg.ToolCalls) > 0 {
			for _, tc := range msg.ToolCalls {
				oaiMsg.ToolCalls = append(oaiMsg.ToolCalls, openai.ToolCall{
					ID:   tc.ID,
					Type: openai.ToolTypeFunction,
					Function: openai.FunctionCall{
						Name:      tc.Name,
						Arguments: string(tc.Arguments),
					},
				})
			}
		}

		// Handle tool response
		if msg.ToolCallID != "" {
			oaiMsg.ToolCallID = msg.ToolCallID
		}

		result[i] = oaiMsg
	}
	return result
}

// convertToOpenAITools converts tool definitions to OpenAI format.
func convertToOpenAITools(tools []ToolDefinition) []openai.Tool {
	result := make([]openai.Tool, len(tools))
	for i, t := range tools {
		result[i] = openai.Tool{
			Type: openai.ToolTypeFunction,
			Function: &openai.FunctionDefinition{
				Name:        t.Name,
				Description: t.Description,
				Parameters:  t.Parameters,
			},
		}
	}
	return result
}

// Verify OpenAIProvider implements Provider
var _ Provider = (*OpenAIProvider)(nil)
