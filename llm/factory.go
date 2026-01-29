// LLM Provider Factory - Ergonomic builder-first API for creating LLM providers.
//
// Quick Start:
//
//	// Simplest: use defaults, read API key from environment
//	openai, err := llm.ProviderOpenAI.FromEnv()  // Uses gpt-5.2
//	claude, err := llm.ProviderAnthropic.FromEnv()  // Uses claude-opus-4-5
//
//	// With custom model
//	gpt5Codex, err := llm.ProviderOpenAI.Model(llm.ModelOpenAIGPT52Codex).FromEnv()
//
//	// Full configuration
//	custom, err := llm.ProviderAnthropic.
//	    Model(llm.ModelAnthropicClaudeSonnet4).
//	    MaxTokens(8192).
//	    Temperature(0.3).
//	    FromEnv()
//
//	// With explicit API key
//	provider, err := llm.ProviderOpenAI.Model(llm.ModelOpenAIGPT52).APIKey("sk-...")

package llm

import (
	"fmt"
	"os"
	"strings"
)

// ProviderType represents supported LLM providers.
type ProviderType int

const (
	// ProviderOpenAI is the OpenAI provider (GPT models).
	ProviderOpenAI ProviderType = iota
	// ProviderAnthropic is the Anthropic provider (Claude models).
	ProviderAnthropic
	// ProviderDeepSeek is the DeepSeek provider.
	ProviderDeepSeek
	// ProviderGemini is the Google Gemini provider.
	ProviderGemini
)

// String returns the string representation of the provider type.
func (p ProviderType) String() string {
	switch p {
	case ProviderOpenAI:
		return "openai"
	case ProviderAnthropic:
		return "anthropic"
	case ProviderDeepSeek:
		return "deepseek"
	case ProviderGemini:
		return "gemini"
	default:
		return "unknown"
	}
}

// EnvVar returns the environment variable name for this provider's API key.
func (p ProviderType) EnvVar() string {
	switch p {
	case ProviderOpenAI:
		return "OPENAI_API_KEY"
	case ProviderAnthropic:
		return "ANTHROPIC_API_KEY"
	case ProviderDeepSeek:
		return "DEEPSEEK_API_KEY"
	case ProviderGemini:
		return "GEMINI_API_KEY"
	default:
		return ""
	}
}

// DefaultModel returns the default model for this provider.
func (p ProviderType) DefaultModel() string {
	switch p {
	case ProviderOpenAI:
		return ModelOpenAIGPT52
	case ProviderAnthropic:
		return ModelAnthropicClaudeOpus45
	case ProviderDeepSeek:
		return ModelDeepSeekV32
	case ProviderGemini:
		return ModelGeminiFlash3
	default:
		return ""
	}
}

// ParseProviderType parses a provider from string (case-insensitive).
func ParseProviderType(s string) (ProviderType, error) {
	switch strings.ToLower(s) {
	case "openai", "gpt":
		return ProviderOpenAI, nil
	case "anthropic", "claude":
		return ProviderAnthropic, nil
	case "deepseek":
		return ProviderDeepSeek, nil
	case "gemini", "google":
		return ProviderGemini, nil
	default:
		return 0, fmt.Errorf("unknown provider: %s", s)
	}
}

// FromEnv creates a provider with defaults, reading API key from environment.
func (p ProviderType) FromEnv() (Provider, error) {
	return NewProviderBuilder(p).FromEnv()
}

// Model starts configuring this provider with a specific model.
func (p ProviderType) Model(model string) *ProviderBuilder {
	return NewProviderBuilder(p).Model(model)
}

// APIKey creates a provider with an explicit API key (uses defaults for everything else).
func (p ProviderType) APIKey(key string) (Provider, error) {
	return NewProviderBuilder(p).APIKey(key)
}

// ProviderBuilder is a builder for configuring LLM providers.
type ProviderBuilder struct {
	providerType ProviderType
	model        string
	maxTokens    uint32
	temperature  *float32
}

// NewProviderBuilder creates a new builder for the given provider.
func NewProviderBuilder(providerType ProviderType) *ProviderBuilder {
	return &ProviderBuilder{
		providerType: providerType,
	}
}

// Model sets the model to use.
func (b *ProviderBuilder) Model(model string) *ProviderBuilder {
	b.model = model
	return b
}

// MaxTokens sets maximum tokens for responses.
func (b *ProviderBuilder) MaxTokens(tokens uint32) *ProviderBuilder {
	b.maxTokens = tokens
	return b
}

// Temperature sets temperature (0.0 = deterministic, 1.0 = creative).
func (b *ProviderBuilder) Temperature(temp float32) *ProviderBuilder {
	b.temperature = &temp
	return b
}

// FromEnv builds the provider, reading API key from environment.
func (b *ProviderBuilder) FromEnv() (Provider, error) {
	envVar := b.providerType.EnvVar()
	apiKey := os.Getenv(envVar)
	if apiKey == "" {
		return nil, fmt.Errorf("%s: %s environment variable not set", b.providerType, envVar)
	}
	return b.build(apiKey)
}

// APIKey builds the provider with an explicit API key.
func (b *ProviderBuilder) APIKey(key string) (Provider, error) {
	return b.build(key)
}

func (b *ProviderBuilder) build(apiKey string) (Provider, error) {
	model := b.model
	if model == "" {
		model = b.providerType.DefaultModel()
	}

	maxTokens := b.maxTokens
	if maxTokens == 0 {
		maxTokens = 4096
	}

	temperature := float32(0.7) // default
	if b.temperature != nil {
		temperature = *b.temperature
	}

	switch b.providerType {
	case ProviderOpenAI:
		return NewOpenAIProvider(apiKey, model, maxTokens, temperature), nil
	case ProviderAnthropic:
		return NewAnthropicProvider(apiKey, model, maxTokens, temperature), nil
	case ProviderDeepSeek:
		return NewDeepSeekProvider(apiKey, model, maxTokens, temperature), nil
	case ProviderGemini:
		return NewGeminiProvider(apiKey, model, maxTokens, temperature), nil
	default:
		return nil, fmt.Errorf("unknown provider type: %v", b.providerType)
	}
}

// Model identifier constants for all supported providers.

// OpenAI model identifiers (January 2026)
const (
	// ModelOpenAIGPT52 is GPT-5.2: Latest flagship model (December 2025).
	ModelOpenAIGPT52 = "gpt-5.2"
	// ModelOpenAIGPT52Codex is GPT-5.2-Codex: Agentic coding specialist.
	ModelOpenAIGPT52Codex = "gpt-5.2-codex"
	// ModelOpenAIGPT5 is GPT-5: Previous flagship (August 2025).
	ModelOpenAIGPT5 = "gpt-5"
	// ModelOpenAIO3Mini is O3-mini: Efficient reasoning model.
	ModelOpenAIO3Mini = "o3-mini"
	// ModelOpenAIO1 is O1: Original reasoning model.
	ModelOpenAIO1 = "o1"
	// ModelOpenAIGPT4o is GPT-4o: Legacy model.
	ModelOpenAIGPT4o = "gpt-4o"
	// ModelOpenAIGPT4oMini is GPT-4o-mini: Legacy model.
	ModelOpenAIGPT4oMini = "gpt-4o-mini"
)

// Anthropic model identifiers (January 2026)
const (
	// ModelAnthropicClaudeOpus45 is Claude Opus 4.5: Latest flagship, best for coding/agents.
	ModelAnthropicClaudeOpus45 = "claude-opus-4-5-20251101"
	// ModelAnthropicClaudeSonnet4 is Claude Sonnet 4: Balanced performance.
	ModelAnthropicClaudeSonnet4 = "claude-sonnet-4-20250514"
	// ModelAnthropicClaudeHaiku4 is Claude Haiku 4: Fast and efficient.
	ModelAnthropicClaudeHaiku4 = "claude-haiku-4-20250514"
)

// DeepSeek model identifiers (January 2026)
const (
	// ModelDeepSeekV32 is V3.2: Latest general model, GPT-5 equivalent.
	ModelDeepSeekV32 = "deepseek-v3.2"
	// ModelDeepSeekV31 is V3.1: Previous version.
	ModelDeepSeekV31 = "deepseek-v3.1"
	// ModelDeepSeekR1 is R1: Reasoning model with chain-of-thought.
	ModelDeepSeekR1 = "deepseek-r1"
)

// Gemini model identifiers (January 2026)
const (
	// ModelGeminiPro3 is Gemini 3 Pro: Advanced reasoning, 1M context window.
	ModelGeminiPro3 = "gemini-3-pro"
	// ModelGeminiFlash3 is Gemini 3 Flash: Speed optimized with frontier intelligence.
	ModelGeminiFlash3 = "gemini-3-flash"
	// ModelGeminiDeepThink3 is Gemini 3 Deep Think: Advanced reasoning mode.
	ModelGeminiDeepThink3 = "gemini-3-deep-think"
	// ModelGeminiFlash2 is Gemini 2.0 Flash: Legacy model.
	ModelGeminiFlash2 = "gemini-2.0-flash"
	// ModelGeminiPro2 is Gemini 2.0 Pro: Legacy model.
	ModelGeminiPro2 = "gemini-2.0-pro"
)
