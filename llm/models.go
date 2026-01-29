// Package llm provides shared data models for LLM providers.
package llm

import "encoding/json"

// ChatMessage represents a chat message with role and content.
type ChatMessage struct {
	Role       string     `json:"role"`
	Content    string     `json:"content"`
	ToolCalls  []ToolCall `json:"tool_calls,omitempty"`  // For assistant messages with tool calls
	ToolCallID string     `json:"tool_call_id,omitempty"` // For tool result messages
}

// ToolCall represents a tool call from the LLM.
type ToolCall struct {
	ID        string          `json:"id"`
	Name      string          `json:"name"`
	Arguments json.RawMessage `json:"arguments"`
}

// ToolDefinition defines a tool that the LLM can call.
type ToolDefinition struct {
	Name        string                 `json:"name"`
	Description string                 `json:"description"`
	Parameters  map[string]interface{} `json:"parameters"` // JSON Schema
}

// SystemMessage creates a system message.
func SystemMessage(content string) ChatMessage {
	return ChatMessage{
		Role:    "system",
		Content: content,
	}
}

// UserMessage creates a user message.
func UserMessage(content string) ChatMessage {
	return ChatMessage{
		Role:    "user",
		Content: content,
	}
}

// AssistantMessage creates an assistant message.
func AssistantMessage(content string) ChatMessage {
	return ChatMessage{
		Role:    "assistant",
		Content: content,
	}
}

// LLMResponse represents a response from an LLM provider.
type LLMResponse struct {
	Content   string
	ToolCalls []ToolCall // Tool calls requested by the LLM
	Usage     *TokenUsage
}

// TokenUsage contains token usage statistics.
type TokenUsage struct {
	PromptTokens     uint32
	CompletionTokens uint32
	TotalTokens      uint32
}

// ResponseFormatType defines the type of response format.
type ResponseFormatType string

const (
	ResponseFormatText       ResponseFormatType = "text"
	ResponseFormatJSONObject ResponseFormatType = "json_object"
	ResponseFormatJSONSchema ResponseFormatType = "json_schema"
)

// ResponseFormat specifies how the LLM should format its response.
type ResponseFormat struct {
	Type       ResponseFormatType `json:"type"`
	JSONSchema *JSONSchemaFormat  `json:"json_schema,omitempty"`
}

// JSONSchemaFormat defines a JSON schema for structured outputs.
type JSONSchemaFormat struct {
	Name        string          `json:"name"`
	Description string          `json:"description,omitempty"`
	Schema      json.RawMessage `json:"schema"`
	Strict      bool            `json:"strict"`
}

// NewTextFormat creates a text response format.
func NewTextFormat() *ResponseFormat {
	return &ResponseFormat{Type: ResponseFormatText}
}

// NewJSONObjectFormat creates a JSON object response format.
func NewJSONObjectFormat() *ResponseFormat {
	return &ResponseFormat{Type: ResponseFormatJSONObject}
}

// NewJSONSchemaFormat creates a JSON schema response format.
func NewJSONSchemaFormat(name string, schema json.RawMessage) *ResponseFormat {
	return &ResponseFormat{
		Type: ResponseFormatJSONSchema,
		JSONSchema: &JSONSchemaFormat{
			Name:   name,
			Schema: schema,
			Strict: true,
		},
	}
}
