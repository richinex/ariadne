// Security tests for LLM providers to ensure error messages don't leak API keys.
package llm

import (
	"context"
	"strings"
	"testing"
	"time"
)

// TestOpenAIErrorNoAPIKeyLeak verifies OpenAI errors don't contain API keys
func TestOpenAIErrorNoAPIKeyLeak(t *testing.T) {
	// Use intentionally invalid API key
	testKey := "sk-test-invalid-key-12345xyz"
	provider := NewOpenAIProvider(testKey, "gpt-4o", 100, 0.7)

	// Force error with invalid key
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	_, err := provider.Chat(ctx, []ChatMessage{
		{Role: "user", Content: "test"},
	})

	// Should return an error
	if err == nil {
		t.Skip("Expected error with invalid API key, but got success - skipping leak test")
	}

	// Verify error doesn't contain the API key
	errStr := err.Error()
	if strings.Contains(errStr, testKey) {
		t.Errorf("OpenAI error message leaked API key: %v", errStr)
	}

	// Should not contain common auth header patterns
	if strings.Contains(errStr, "Authorization:") {
		t.Errorf("OpenAI error exposed Authorization header: %v", errStr)
	}
}

// TestAnthropicErrorNoAPIKeyLeak verifies Anthropic errors don't contain API keys
func TestAnthropicErrorNoAPIKeyLeak(t *testing.T) {
	testKey := "sk-ant-test-invalid-key-12345xyz"
	provider := NewAnthropicProvider(testKey, "claude-sonnet-4-20250514", 100, 0.7)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	_, err := provider.Chat(ctx, []ChatMessage{
		{Role: "user", Content: "test"},
	})

	if err == nil {
		t.Skip("Expected error with invalid API key, but got success - skipping leak test")
	}

	errStr := err.Error()
	if strings.Contains(errStr, testKey) {
		t.Errorf("Anthropic error message leaked API key: %v", errStr)
	}

	if strings.Contains(errStr, "x-api-key:") || strings.Contains(errStr, "X-API-Key:") {
		t.Errorf("Anthropic error exposed API key header: %v", errStr)
	}
}

// TestDeepSeekErrorNoAPIKeyLeak verifies DeepSeek errors don't contain API keys
func TestDeepSeekErrorNoAPIKeyLeak(t *testing.T) {
	testKey := "sk-test-invalid-key-12345xyz"
	provider := NewDeepSeekProvider(testKey, "deepseek-chat", 100, 0.7)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	_, err := provider.Chat(ctx, []ChatMessage{
		{Role: "user", Content: "test"},
	})

	if err == nil {
		t.Skip("Expected error with invalid API key, but got success - skipping leak test")
	}

	errStr := err.Error()
	if strings.Contains(errStr, testKey) {
		t.Errorf("DeepSeek error message leaked API key: %v", errStr)
	}

	if strings.Contains(errStr, "Authorization:") {
		t.Errorf("DeepSeek error exposed Authorization header: %v", errStr)
	}
}

// TestGeminiErrorNoAPIKeyLeak verifies Gemini errors don't contain API keys
func TestGeminiErrorNoAPIKeyLeak(t *testing.T) {
	testKey := "test-invalid-key-12345xyz"
	provider := NewGeminiProvider(testKey, "gemini-2.5-flash", 100, 0.7)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	_, err := provider.Chat(ctx, []ChatMessage{
		{Role: "user", Content: "test"},
	})

	if err == nil {
		t.Skip("Expected error with invalid API key, but got success - skipping leak test")
	}

	errStr := err.Error()
	if strings.Contains(errStr, testKey) {
		t.Errorf("Gemini error message leaked API key: %v", errStr)
	}

	// Gemini uses x-goog-api-key header
	if strings.Contains(errStr, "x-goog-api-key:") {
		t.Errorf("Gemini error exposed API key header: %v", errStr)
	}
}

// TestGeminiInitErrorPreserved verifies Gemini returns initialization errors
func TestGeminiInitErrorPreserved(t *testing.T) {
	// Use invalid key that should fail during client initialization
	provider := NewGeminiProvider("", "gemini-2.5-flash", 100, 0.7)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	_, err := provider.Chat(ctx, []ChatMessage{
		{Role: "user", Content: "test"},
	})

	// Should return an error
	if err == nil {
		t.Error("Expected initialization error to be returned, got nil")
		return
	}

	// Error should indicate initialization failure
	errStr := err.Error()
	if !strings.Contains(errStr, "failed to initialize") {
		t.Errorf("Expected initialization error, got: %v", errStr)
	}
}

// TestStreamErrorNoAPIKeyLeak verifies streaming errors don't leak API keys
func TestStreamErrorNoAPIKeyLeak(t *testing.T) {
	testKey := "sk-test-invalid-key-12345xyz"
	provider := NewOpenAIProvider(testKey, "gpt-4o", 100, 0.7)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	chunks := make(chan string, 10)
	_, err := provider.StreamChat(ctx, []ChatMessage{
		{Role: "user", Content: "test"},
	}, chunks)

	if err == nil {
		t.Skip("Expected error with invalid API key, but got success - skipping leak test")
	}

	errStr := err.Error()
	if strings.Contains(errStr, testKey) {
		t.Errorf("Stream error message leaked API key: %v", errStr)
	}
}

// TestToolCallErrorNoAPIKeyLeak verifies tool call errors don't leak API keys
func TestToolCallErrorNoAPIKeyLeak(t *testing.T) {
	testKey := "sk-test-invalid-key-12345xyz"
	provider := NewOpenAIProvider(testKey, "gpt-4o", 100, 0.7)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	tools := []ToolDefinition{
		{
			Name:        "test_tool",
			Description: "A test tool",
			Parameters:  map[string]interface{}{"type": "object"},
		},
	}

	_, err := provider.ChatWithTools(ctx, []ChatMessage{
		{Role: "user", Content: "test"},
	}, tools)

	if err == nil {
		t.Skip("Expected error with invalid API key, but got success - skipping leak test")
	}

	errStr := err.Error()
	if strings.Contains(errStr, testKey) {
		t.Errorf("Tool call error message leaked API key: %v", errStr)
	}
}
