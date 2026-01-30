// Google Gemini Provider implementation using official google.golang.org/genai SDK.
//
// Information Hiding:
// - API authentication and client creation
// - Request/response format for Gemini API
// - System instruction handling via config
// - Streaming via official SDK iterator

package llm

import (
	"context"
	"encoding/json"
	"fmt"

	"google.golang.org/genai"
)

// GeminiProvider implements the Provider interface for Google Gemini.
type GeminiProvider struct {
	client      *genai.Client
	model       string
	maxTokens   int32
	temperature float32
	initErr     error // Stores client initialization error for deferred reporting
}

// NewGeminiProvider creates a new Gemini provider.
// If client initialization fails, the error is stored and returned on first use.
func NewGeminiProvider(apiKey, model string, maxTokens uint32, temperature float32) *GeminiProvider {
	ctx := context.Background()
	client, err := genai.NewClient(ctx, &genai.ClientConfig{
		APIKey:  apiKey,
		Backend: genai.BackendGeminiAPI,
	})
	if err != nil {
		// Store initialization error to return on first use - preserves constructor signature
		return &GeminiProvider{
			client:      nil,
			model:       model,
			maxTokens:   int32(maxTokens),
			temperature: temperature,
			initErr:     fmt.Errorf("failed to initialize Gemini client: %w", err),
		}
	}

	return &GeminiProvider{
		client:      client,
		model:       model,
		maxTokens:   int32(maxTokens),
		temperature: temperature,
		initErr:     nil,
	}
}

// Name returns the provider name.
func (p *GeminiProvider) Name() string {
	return "gemini"
}

// Model returns the current model.
func (p *GeminiProvider) Model() string {
	return p.model
}

// Chat sends a chat completion request.
func (p *GeminiProvider) Chat(ctx context.Context, messages []ChatMessage) (LLMResponse, error) {
	return p.ChatWithFormat(ctx, messages, nil)
}

// ChatWithFormat sends a chat completion request with optional response format.
func (p *GeminiProvider) ChatWithFormat(ctx context.Context, messages []ChatMessage, _ *ResponseFormat) (LLMResponse, error) {
	if p.initErr != nil {
		return LLMResponse{}, p.initErr
	}
	if p.client == nil {
		return LLMResponse{}, fmt.Errorf("gemini client not initialized")
	}

	contents, systemInstruction := convertToGeminiMessages(messages)

	config := &genai.GenerateContentConfig{
		Temperature:     genai.Ptr(p.temperature),
		MaxOutputTokens: p.maxTokens,
	}

	if systemInstruction != "" {
		config.SystemInstruction = genai.NewContentFromText(systemInstruction, genai.RoleUser)
	}

	response, err := p.client.Models.GenerateContent(ctx, p.model, contents, config)
	if err != nil {
		return LLMResponse{}, fmt.Errorf("chat completion failed: %w", err)
	}

	content := response.Text()
	if content == "" {
		return LLMResponse{}, fmt.Errorf("empty response from Gemini")
	}

	var usage *TokenUsage
	if response.UsageMetadata != nil {
		usage = &TokenUsage{
			PromptTokens:     uint32(response.UsageMetadata.PromptTokenCount),
			CompletionTokens: uint32(response.UsageMetadata.CandidatesTokenCount),
			TotalTokens:      uint32(response.UsageMetadata.TotalTokenCount),
		}
	}

	return LLMResponse{Content: content, Usage: usage}, nil
}

// ChatWithTools sends a chat completion request with tool definitions.
func (p *GeminiProvider) ChatWithTools(ctx context.Context, messages []ChatMessage, tools []ToolDefinition) (LLMResponse, error) {
	if p.initErr != nil {
		return LLMResponse{}, p.initErr
	}
	if p.client == nil {
		return LLMResponse{}, fmt.Errorf("gemini client not initialized")
	}

	contents, systemInstruction := convertToGeminiMessagesWithTools(messages)

	config := &genai.GenerateContentConfig{
		Temperature:     genai.Ptr(p.temperature),
		MaxOutputTokens: p.maxTokens,
		Tools:           convertToGeminiTools(tools),
	}

	if systemInstruction != "" {
		config.SystemInstruction = genai.NewContentFromText(systemInstruction, genai.RoleUser)
	}

	response, err := p.client.Models.GenerateContent(ctx, p.model, contents, config)
	if err != nil {
		return LLMResponse{}, fmt.Errorf("chat completion failed: %w", err)
	}

	content := ""
	var toolCalls []ToolCall

	// Extract content and tool calls from response
	if len(response.Candidates) > 0 && response.Candidates[0].Content != nil {
		for _, part := range response.Candidates[0].Content.Parts {
			if part.Text != "" {
				content += part.Text
			}
			if part.FunctionCall != nil {
				argsJSON, _ := json.Marshal(part.FunctionCall.Args)
				toolCalls = append(toolCalls, ToolCall{
					ID:        part.FunctionCall.Name, // Gemini uses name as ID
					Name:      part.FunctionCall.Name,
					Arguments: argsJSON,
				})
			}
		}
	}

	var usage *TokenUsage
	if response.UsageMetadata != nil {
		usage = &TokenUsage{
			PromptTokens:     uint32(response.UsageMetadata.PromptTokenCount),
			CompletionTokens: uint32(response.UsageMetadata.CandidatesTokenCount),
			TotalTokens:      uint32(response.UsageMetadata.TotalTokenCount),
		}
	}

	return LLMResponse{Content: content, ToolCalls: toolCalls, Usage: usage}, nil
}

// StreamChat streams a chat completion.
func (p *GeminiProvider) StreamChat(ctx context.Context, messages []ChatMessage, chunks chan<- string) (*TokenUsage, error) {
	if p.initErr != nil {
		return nil, p.initErr
	}
	if p.client == nil {
		return nil, fmt.Errorf("gemini client not initialized")
	}

	contents, systemInstruction := convertToGeminiMessages(messages)

	config := &genai.GenerateContentConfig{
		Temperature:     genai.Ptr(p.temperature),
		MaxOutputTokens: p.maxTokens,
	}

	if systemInstruction != "" {
		config.SystemInstruction = genai.NewContentFromText(systemInstruction, genai.RoleUser)
	}

	var usage *TokenUsage
	// GenerateContentStream returns iter.Seq2[*GenerateContentResponse, error]
	for response, err := range p.client.Models.GenerateContentStream(ctx, p.model, contents, config) {
		if err != nil {
			return usage, fmt.Errorf("stream error: %w", err)
		}

		// Capture usage metadata from response
		if response.UsageMetadata != nil {
			usage = &TokenUsage{
				PromptTokens:     uint32(response.UsageMetadata.PromptTokenCount),
				CompletionTokens: uint32(response.UsageMetadata.CandidatesTokenCount),
				TotalTokens:      uint32(response.UsageMetadata.TotalTokenCount),
			}
		}

		text := response.Text()
		if text != "" {
			select {
			case chunks <- text:
			case <-ctx.Done():
				return usage, ctx.Err()
			}
		}
	}

	return usage, nil
}

// convertToGeminiMessages converts our ChatMessage to Gemini format.
// Extracts system message and returns it separately.
func convertToGeminiMessages(messages []ChatMessage) ([]*genai.Content, string) {
	var contents []*genai.Content
	var systemInstruction string

	for _, msg := range messages {
		switch msg.Role {
		case "system":
			systemInstruction = msg.Content
		case "user":
			contents = append(contents, genai.NewContentFromText(msg.Content, genai.RoleUser))
		case "assistant":
			contents = append(contents, genai.NewContentFromText(msg.Content, genai.RoleModel))
		}
	}

	return contents, systemInstruction
}

// convertToGeminiMessagesWithTools handles tool calls and tool responses.
func convertToGeminiMessagesWithTools(messages []ChatMessage) ([]*genai.Content, string) {
	var contents []*genai.Content
	var systemInstruction string

	for _, msg := range messages {
		switch msg.Role {
		case "system":
			systemInstruction = msg.Content
		case "user":
			contents = append(contents, genai.NewContentFromText(msg.Content, genai.RoleUser))
		case "assistant":
			if len(msg.ToolCalls) > 0 {
				// Assistant with tool calls
				content := &genai.Content{Role: genai.RoleModel}
				if msg.Content != "" {
					content.Parts = append(content.Parts, &genai.Part{Text: msg.Content})
				}
				for _, tc := range msg.ToolCalls {
					var args map[string]any
					_ = json.Unmarshal(tc.Arguments, &args)
					content.Parts = append(content.Parts, &genai.Part{
						FunctionCall: &genai.FunctionCall{
							Name: tc.Name,
							Args: args,
						},
					})
				}
				contents = append(contents, content)
			} else {
				contents = append(contents, genai.NewContentFromText(msg.Content, genai.RoleModel))
			}
		case "tool":
			// Tool response
			var result map[string]any
			_ = json.Unmarshal([]byte(msg.Content), &result)
			if result == nil {
				result = map[string]any{"result": msg.Content}
			}
			content := &genai.Content{
				Role: genai.RoleUser, // Gemini expects tool results as user
				Parts: []*genai.Part{{
					FunctionResponse: &genai.FunctionResponse{
						Name:     msg.ToolCallID,
						Response: result,
					},
				}},
			}
			contents = append(contents, content)
		}
	}

	return contents, systemInstruction
}

// convertToGeminiTools converts tool definitions to Gemini format.
func convertToGeminiTools(tools []ToolDefinition) []*genai.Tool {
	if len(tools) == 0 {
		return nil
	}

	var declarations []*genai.FunctionDeclaration
	for _, t := range tools {
		schema := convertToGeminiSchema(t.Parameters)
		declarations = append(declarations, &genai.FunctionDeclaration{
			Name:        t.Name,
			Description: t.Description,
			Parameters:  schema,
		})
	}

	return []*genai.Tool{{FunctionDeclarations: declarations}}
}

// convertToGeminiSchema recursively converts a parameter schema to Gemini format.
// Handles arrays by adding required 'items' field.
func convertToGeminiSchema(params map[string]interface{}) *genai.Schema {
	schema := &genai.Schema{
		Type: genai.TypeObject,
	}

	// Get type if present
	if t, ok := params["type"].(string); ok {
		schema.Type = mapToGeminiType(t)
	}

	// Get required fields
	if req, ok := params["required"].([]interface{}); ok {
		for _, r := range req {
			if s, ok := r.(string); ok {
				schema.Required = append(schema.Required, s)
			}
		}
	}
	// Also handle []string
	if req, ok := params["required"].([]string); ok {
		schema.Required = req
	}

	// Convert properties
	if props, ok := params["properties"].(map[string]interface{}); ok {
		schema.Properties = make(map[string]*genai.Schema)
		for name, prop := range props {
			propMap, ok := prop.(map[string]interface{})
			if !ok {
				continue
			}
			schema.Properties[name] = convertPropertyToGeminiSchema(propMap)
		}
	}

	return schema
}

// convertPropertyToGeminiSchema converts a single property to Gemini schema.
func convertPropertyToGeminiSchema(prop map[string]interface{}) *genai.Schema {
	schema := &genai.Schema{}

	// Get type
	if t, ok := prop["type"].(string); ok {
		schema.Type = mapToGeminiType(t)
	}

	// Get description
	if d, ok := prop["description"].(string); ok {
		schema.Description = d
	}

	// Handle array items - Gemini requires 'items' for arrays
	if schema.Type == genai.TypeArray {
		if items, ok := prop["items"].(map[string]interface{}); ok {
			schema.Items = convertPropertyToGeminiSchema(items)
		} else {
			// Default to string items if not specified
			schema.Items = &genai.Schema{Type: genai.TypeString}
		}
	}

	// Handle nested object properties
	if schema.Type == genai.TypeObject {
		if props, ok := prop["properties"].(map[string]interface{}); ok {
			schema.Properties = make(map[string]*genai.Schema)
			for name, p := range props {
				if pMap, ok := p.(map[string]interface{}); ok {
					schema.Properties[name] = convertPropertyToGeminiSchema(pMap)
				}
			}
		}
	}

	return schema
}

// mapToGeminiType maps JSON schema type to Gemini type.
func mapToGeminiType(t string) genai.Type {
	switch t {
	case "string":
		return genai.TypeString
	case "integer", "number":
		return genai.TypeNumber
	case "boolean":
		return genai.TypeBoolean
	case "array":
		return genai.TypeArray
	case "object":
		return genai.TypeObject
	default:
		return genai.TypeString
	}
}

// Verify GeminiProvider implements Provider
var _ Provider = (*GeminiProvider)(nil)
