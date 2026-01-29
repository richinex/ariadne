// DeepSeek Provider implementation using go-openai library.
//
// Information Hiding:
// - Uses OpenAI-compatible API with different base URL
// - Supports deepseek-chat and deepseek-reasoner models
// - Streaming via go-openai library

package llm

import (
	"context"
	"errors"
	"fmt"
	"io"

	openai "github.com/sashabaranov/go-openai"
)

const deepseekBaseURL = "https://api.deepseek.com/v1"

// DeepSeekProvider implements the Provider interface for DeepSeek.
type DeepSeekProvider struct {
	client      *openai.Client
	model       string
	maxTokens   int
	temperature float32
}

// NewDeepSeekProvider creates a new DeepSeek provider.
func NewDeepSeekProvider(apiKey, model string, maxTokens uint32, temperature float32) *DeepSeekProvider {
	config := openai.DefaultConfig(apiKey)
	config.BaseURL = deepseekBaseURL

	return &DeepSeekProvider{
		client:      openai.NewClientWithConfig(config),
		model:       model,
		maxTokens:   int(maxTokens),
		temperature: temperature,
	}
}

// Name returns the provider name.
func (p *DeepSeekProvider) Name() string {
	return "deepseek"
}

// Model returns the current model.
func (p *DeepSeekProvider) Model() string {
	return p.model
}

// Chat sends a chat completion request.
func (p *DeepSeekProvider) Chat(ctx context.Context, messages []ChatMessage) (LLMResponse, error) {
	return p.ChatWithFormat(ctx, messages, nil)
}

// ChatWithFormat sends a chat completion request with optional response format.
func (p *DeepSeekProvider) ChatWithFormat(ctx context.Context, messages []ChatMessage, format *ResponseFormat) (LLMResponse, error) {
	req := openai.ChatCompletionRequest{
		Model:       p.model,
		Messages:    convertMessages(messages),
		MaxCompletionTokens:   p.maxTokens,
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

	// DeepSeek returns token usage in the standard OpenAI format
	usage := &TokenUsage{
		PromptTokens:     uint32(resp.Usage.PromptTokens),
		CompletionTokens: uint32(resp.Usage.CompletionTokens),
		TotalTokens:      uint32(resp.Usage.TotalTokens),
	}

	return LLMResponse{Content: content, Usage: usage}, nil
}

// ChatWithTools sends a chat completion request with tool definitions.
func (p *DeepSeekProvider) ChatWithTools(ctx context.Context, messages []ChatMessage, tools []ToolDefinition) (LLMResponse, error) {
	req := openai.ChatCompletionRequest{
		Model:       p.model,
		Messages:    convertMessagesWithTools(messages),
		MaxCompletionTokens:   p.maxTokens,
		Temperature: p.temperature,
		Tools:       convertTools(tools),
	}

	resp, err := p.client.CreateChatCompletion(ctx, req)
	if err != nil {
		return LLMResponse{}, fmt.Errorf("chat completion failed: %w", err)
	}

	content := ""
	var toolCalls []ToolCall
	if len(resp.Choices) > 0 {
		content = resp.Choices[0].Message.Content
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
func (p *DeepSeekProvider) StreamChat(ctx context.Context, messages []ChatMessage, chunks chan<- string) (*TokenUsage, error) {
	req := openai.ChatCompletionRequest{
		Model:       p.model,
		Messages:    convertMessages(messages),
		MaxCompletionTokens:   p.maxTokens,
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

// convertMessages converts our ChatMessage to openai.ChatCompletionMessage
func convertMessages(messages []ChatMessage) []openai.ChatCompletionMessage {
	result := make([]openai.ChatCompletionMessage, len(messages))
	for i, msg := range messages {
		result[i] = openai.ChatCompletionMessage{
			Role:    msg.Role,
			Content: msg.Content,
		}
	}
	return result
}

// convertMessagesWithTools handles tool calls and tool responses.
func convertMessagesWithTools(messages []ChatMessage) []openai.ChatCompletionMessage {
	result := make([]openai.ChatCompletionMessage, len(messages))
	for i, msg := range messages {
		oaiMsg := openai.ChatCompletionMessage{
			Role:    msg.Role,
			Content: msg.Content,
		}
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
		if msg.ToolCallID != "" {
			oaiMsg.ToolCallID = msg.ToolCallID
		}
		result[i] = oaiMsg
	}
	return result
}

// convertTools converts tool definitions to OpenAI format.
func convertTools(tools []ToolDefinition) []openai.Tool {
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

// Verify DeepSeekProvider implements Provider
var _ Provider = (*DeepSeekProvider)(nil)
