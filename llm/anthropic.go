// Anthropic Provider implementation using official anthropic-sdk-go.
//
// Information Hiding:
// - API endpoint and authentication
// - Request/response format for Anthropic Messages API
// - Streaming via official SDK

package llm

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/anthropics/anthropic-sdk-go"
	"github.com/anthropics/anthropic-sdk-go/option"
)

// AnthropicProvider implements the Provider interface for Anthropic Claude.
type AnthropicProvider struct {
	client      anthropic.Client
	model       string
	maxTokens   int64
	temperature float64
}

// NewAnthropicProvider creates a new Anthropic provider.
func NewAnthropicProvider(apiKey, model string, maxTokens uint32, temperature float32) *AnthropicProvider {
	client := anthropic.NewClient(
		option.WithAPIKey(apiKey),
	)

	return &AnthropicProvider{
		client:      client,
		model:       model,
		maxTokens:   int64(maxTokens),
		temperature: float64(temperature),
	}
}

// Name returns the provider name.
func (p *AnthropicProvider) Name() string {
	return "anthropic"
}

// Model returns the current model.
func (p *AnthropicProvider) Model() string {
	return p.model
}

// Chat sends a chat completion request.
func (p *AnthropicProvider) Chat(ctx context.Context, messages []ChatMessage) (LLMResponse, error) {
	return p.ChatWithFormat(ctx, messages, nil)
}

// ChatWithFormat sends a chat completion request with optional response format.
func (p *AnthropicProvider) ChatWithFormat(ctx context.Context, messages []ChatMessage, format *ResponseFormat) (LLMResponse, error) {
	anthropicMessages, systemPrompt := convertToAnthropicMessages(messages)

	params := anthropic.MessageNewParams{
		Model:       anthropic.Model(p.model),
		MaxTokens:   p.maxTokens,
		Messages:    anthropicMessages,
		Temperature: anthropic.Float(p.temperature),
	}

	if systemPrompt != "" {
		params.System = []anthropic.TextBlockParam{
			{Text: systemPrompt},
		}
	}

	message, err := p.client.Messages.New(ctx, params)
	if err != nil {
		return LLMResponse{}, fmt.Errorf("chat completion failed: %w", err)
	}

	content := ""
	for _, block := range message.Content {
		switch variant := block.AsAny().(type) {
		case anthropic.TextBlock:
			content += variant.Text
		}
	}

	var usage *TokenUsage
	if message.Usage.InputTokens > 0 || message.Usage.OutputTokens > 0 {
		usage = &TokenUsage{
			PromptTokens:     uint32(message.Usage.InputTokens),
			CompletionTokens: uint32(message.Usage.OutputTokens),
			TotalTokens:      uint32(message.Usage.InputTokens + message.Usage.OutputTokens),
		}
	}

	return LLMResponse{Content: content, Usage: usage}, nil
}

// ChatWithTools sends a chat completion request with tool definitions.
func (p *AnthropicProvider) ChatWithTools(ctx context.Context, messages []ChatMessage, tools []ToolDefinition) (LLMResponse, error) {
	anthropicMessages, systemPrompt := convertToAnthropicMessagesWithTools(messages)

	params := anthropic.MessageNewParams{
		Model:       anthropic.Model(p.model),
		MaxTokens:   p.maxTokens,
		Messages:    anthropicMessages,
		Temperature: anthropic.Float(p.temperature),
		Tools:       convertToAnthropicTools(tools),
	}

	if systemPrompt != "" {
		params.System = []anthropic.TextBlockParam{
			{Text: systemPrompt},
		}
	}

	message, err := p.client.Messages.New(ctx, params)
	if err != nil {
		return LLMResponse{}, fmt.Errorf("chat completion failed: %w", err)
	}

	content := ""
	var toolCalls []ToolCall
	for _, block := range message.Content {
		switch variant := block.AsAny().(type) {
		case anthropic.TextBlock:
			content += variant.Text
		case anthropic.ToolUseBlock:
			// Get raw JSON input from the ToolUseBlock
			inputJSON, _ := json.Marshal(variant.Input)
			toolCalls = append(toolCalls, ToolCall{
				ID:        variant.ID,
				Name:      variant.Name,
				Arguments: inputJSON,
			})
		}
	}

	var usage *TokenUsage
	if message.Usage.InputTokens > 0 || message.Usage.OutputTokens > 0 {
		usage = &TokenUsage{
			PromptTokens:     uint32(message.Usage.InputTokens),
			CompletionTokens: uint32(message.Usage.OutputTokens),
			TotalTokens:      uint32(message.Usage.InputTokens + message.Usage.OutputTokens),
		}
	}

	return LLMResponse{Content: content, ToolCalls: toolCalls, Usage: usage}, nil
}

// convertToAnthropicMessagesWithTools handles tool calls and tool responses.
func convertToAnthropicMessagesWithTools(messages []ChatMessage) ([]anthropic.MessageParam, string) {
	var anthropicMessages []anthropic.MessageParam
	var systemPrompt string

	for _, msg := range messages {
		switch msg.Role {
		case "system":
			systemPrompt = msg.Content
		case "user":
			anthropicMessages = append(anthropicMessages, anthropic.NewUserMessage(
				anthropic.NewTextBlock(msg.Content),
			))
		case "assistant":
			if len(msg.ToolCalls) > 0 {
				// Use message.ToParam() pattern - build from response
				content := &anthropic.MessageParam{
					Role: anthropic.MessageParamRoleAssistant,
				}
				if msg.Content != "" {
					content.Content = append(content.Content, anthropic.NewTextBlock(msg.Content))
				}
				for _, tc := range msg.ToolCalls {
					var input map[string]interface{}
					_ = json.Unmarshal(tc.Arguments, &input)
					content.Content = append(content.Content, anthropic.ContentBlockParamUnion{
						OfToolUse: &anthropic.ToolUseBlockParam{
							ID:    tc.ID,
							Name:  tc.Name,
							Input: input,
						},
					})
				}
				anthropicMessages = append(anthropicMessages, *content)
			} else {
				anthropicMessages = append(anthropicMessages, anthropic.NewAssistantMessage(
					anthropic.NewTextBlock(msg.Content),
				))
			}
		case "tool":
			// Tool result
			anthropicMessages = append(anthropicMessages, anthropic.NewUserMessage(
				anthropic.NewToolResultBlock(msg.ToolCallID, msg.Content, false),
			))
		}
	}

	return anthropicMessages, systemPrompt
}

// convertToAnthropicTools converts tool definitions to Anthropic format.
func convertToAnthropicTools(tools []ToolDefinition) []anthropic.ToolUnionParam {
	result := make([]anthropic.ToolUnionParam, len(tools))
	for i, t := range tools {
		// Extract properties and required from the full schema
		properties, _ := t.Parameters["properties"].(map[string]interface{})
		required, _ := t.Parameters["required"].([]string)

		toolParam := anthropic.ToolParam{
			Name:        t.Name,
			Description: anthropic.String(t.Description),
			InputSchema: anthropic.ToolInputSchemaParam{
				Properties: properties,
				Required:   required,
			},
		}
		result[i] = anthropic.ToolUnionParam{OfTool: &toolParam}
	}
	return result
}

// StreamChat streams a chat completion.
func (p *AnthropicProvider) StreamChat(ctx context.Context, messages []ChatMessage, chunks chan<- string) (*TokenUsage, error) {
	anthropicMessages, systemPrompt := convertToAnthropicMessages(messages)

	params := anthropic.MessageNewParams{
		Model:       anthropic.Model(p.model),
		MaxTokens:   p.maxTokens,
		Messages:    anthropicMessages,
		Temperature: anthropic.Float(p.temperature),
	}

	if systemPrompt != "" {
		params.System = []anthropic.TextBlockParam{
			{Text: systemPrompt},
		}
	}

	stream := p.client.Messages.NewStreaming(ctx, params)

	var usage *TokenUsage
	for stream.Next() {
		event := stream.Current()

		// Handle different event types
		switch eventVariant := event.AsAny().(type) {
		case anthropic.MessageStartEvent:
			// Capture input tokens from message start
			if eventVariant.Message.Usage.InputTokens > 0 {
				usage = &TokenUsage{
					PromptTokens: uint32(eventVariant.Message.Usage.InputTokens),
				}
			}
		case anthropic.ContentBlockDeltaEvent:
			switch deltaVariant := eventVariant.Delta.AsAny().(type) {
			case anthropic.TextDelta:
				if deltaVariant.Text != "" {
					select {
					case chunks <- deltaVariant.Text:
					case <-ctx.Done():
						return usage, ctx.Err()
					}
				}
			}
		case anthropic.MessageDeltaEvent:
			// Capture output tokens from message delta
			if eventVariant.Usage.OutputTokens > 0 {
				if usage == nil {
					usage = &TokenUsage{}
				}
				usage.CompletionTokens = uint32(eventVariant.Usage.OutputTokens)
				usage.TotalTokens = usage.PromptTokens + usage.CompletionTokens
			}
		}
	}

	if stream.Err() != nil {
		return usage, fmt.Errorf("stream error: %w", stream.Err())
	}

	return usage, nil
}

// convertToAnthropicMessages converts our ChatMessage to Anthropic format.
// Extracts system message and returns it separately.
func convertToAnthropicMessages(messages []ChatMessage) ([]anthropic.MessageParam, string) {
	var anthropicMessages []anthropic.MessageParam
	var systemPrompt string

	for _, msg := range messages {
		switch msg.Role {
		case "system":
			systemPrompt = msg.Content
		case "user":
			anthropicMessages = append(anthropicMessages, anthropic.NewUserMessage(
				anthropic.NewTextBlock(msg.Content),
			))
		case "assistant":
			anthropicMessages = append(anthropicMessages, anthropic.NewAssistantMessage(
				anthropic.NewTextBlock(msg.Content),
			))
		}
	}

	return anthropicMessages, systemPrompt
}


// Verify AnthropicProvider implements Provider
var _ Provider = (*AnthropicProvider)(nil)
