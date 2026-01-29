// Package config provides application settings loaded from environment variables.
//
// Settings are created via New() which handles:
// - Environment variable parsing with validation
// - Default value application
// - Provider-specific configuration lookup

package config

import (
	"fmt"
	"os"
	"strconv"
	"strings"
)

// Settings holds all application configuration.
type Settings struct {
	LLM   LLMConfig
	Agent AgentConfig
}

// LLMConfig holds LLM provider configuration.
type LLMConfig struct {
	Provider    string
	Model       string
	MaxTokens   uint32
	Temperature float64
}

// AgentConfig holds agent execution configuration.
type AgentConfig struct {
	MaxIterations         int
	MaxOrchestrationSteps int
	MaxSubGoals           int
}

// providerInfo holds configuration for a specific LLM provider.
type providerInfo struct {
	modelEnv     string
	defaultModel string
	apiKeyEnv    string
}

// Supported providers and their configuration.
var providers = map[string]providerInfo{
	"openai":    {"OPENAI_MODEL", "gpt-4o", "OPENAI_API_KEY"},
	"anthropic": {"ANTHROPIC_MODEL", "claude-sonnet-4-20250514", "ANTHROPIC_API_KEY"},
	"deepseek":  {"DEEPSEEK_MODEL", "deepseek-chat", "DEEPSEEK_API_KEY"},
	"gemini":    {"GEMINI_MODEL", "gemini-2.5-flash", "GEMINI_API_KEY"},
}

// Provider aliases map to canonical names.
var providerAliases = map[string]string{
	"claude": "anthropic",
	"google": "gemini",
	"gpt":    "openai",
}

// New creates settings for the specified provider, loading values from environment variables.
// Returns an error if the provider is unknown or environment variables contain invalid values.
func New(provider string) (Settings, error) {
	provider = normalizeProvider(provider)

	info, err := getProviderInfo(provider)
	if err != nil {
		return Settings{}, err
	}

	maxTokens, err := getEnvUint32("LLM_MAX_TOKENS", 4096)
	if err != nil {
		return Settings{}, err
	}

	temperature, err := getEnvFloat64("LLM_TEMPERATURE", 0.7)
	if err != nil {
		return Settings{}, err
	}

	maxIterations, err := getEnvInt("AGENT_MAX_ITERATIONS", 10)
	if err != nil {
		return Settings{}, err
	}

	maxOrchestrationSteps, err := getEnvInt("AGENT_MAX_ORCHESTRATION_STEPS", 8)
	if err != nil {
		return Settings{}, err
	}

	maxSubGoals, err := getEnvInt("AGENT_MAX_SUB_GOALS", 5)
	if err != nil {
		return Settings{}, err
	}

	// Get model from environment or use default
	model := os.Getenv(info.modelEnv)
	if model == "" {
		model = info.defaultModel
	}

	return Settings{
		LLM: LLMConfig{
			Provider:    provider,
			Model:       model,
			MaxTokens:   maxTokens,
			Temperature: temperature,
		},
		Agent: AgentConfig{
			MaxIterations:         maxIterations,
			MaxOrchestrationSteps: maxOrchestrationSteps,
			MaxSubGoals:           maxSubGoals,
		},
	}, nil
}

// MustNew creates settings for the specified provider.
// Panics if the provider is unknown or environment variables are invalid.
// Use this only when configuration errors should be fatal.
func MustNew(provider string) Settings {
	settings, err := New(provider)
	if err != nil {
		panic(fmt.Sprintf("config: %v", err))
	}
	return settings
}

// normalizeProvider converts provider aliases to canonical names.
func normalizeProvider(provider string) string {
	provider = strings.ToLower(provider)
	if canonical, ok := providerAliases[provider]; ok {
		return canonical
	}
	return provider
}

// getProviderInfo returns configuration for a provider.
func getProviderInfo(provider string) (providerInfo, error) {
	info, ok := providers[provider]
	if !ok {
		return providerInfo{}, fmt.Errorf("unknown provider: %q", provider)
	}
	return info, nil
}

// APIKeyFor returns the API key for a provider from environment variables.
func APIKeyFor(provider string) (string, error) {
	provider = normalizeProvider(provider)

	info, err := getProviderInfo(provider)
	if err != nil {
		return "", err
	}

	key := os.Getenv(info.apiKeyEnv)
	if key == "" {
		return "", fmt.Errorf("%s environment variable not set", info.apiKeyEnv)
	}
	return key, nil
}

// ModelFor returns the model for a provider, checking environment first.
func ModelFor(provider string) (string, error) {
	provider = normalizeProvider(provider)

	info, err := getProviderInfo(provider)
	if err != nil {
		return "", err
	}

	if val := os.Getenv(info.modelEnv); val != "" {
		return val, nil
	}
	return info.defaultModel, nil
}

// SupportedProviders returns the list of supported provider names.
func SupportedProviders() []string {
	result := make([]string, 0, len(providers))
	for name := range providers {
		result = append(result, name)
	}
	return result
}

// Environment variable helpers with proper error handling

func getEnvInt(key string, defaultVal int) (int, error) {
	val := os.Getenv(key)
	if val == "" {
		return defaultVal, nil
	}
	i, err := strconv.Atoi(val)
	if err != nil {
		return 0, fmt.Errorf("invalid value for %s: %q: %w", key, val, err)
	}
	return i, nil
}

func getEnvUint32(key string, defaultVal uint32) (uint32, error) {
	val := os.Getenv(key)
	if val == "" {
		return defaultVal, nil
	}
	i, err := strconv.ParseUint(val, 10, 32)
	if err != nil {
		return 0, fmt.Errorf("invalid value for %s: %q: %w", key, val, err)
	}
	return uint32(i), nil
}

func getEnvFloat64(key string, defaultVal float64) (float64, error) {
	val := os.Getenv(key)
	if val == "" {
		return defaultVal, nil
	}
	f, err := strconv.ParseFloat(val, 64)
	if err != nil {
		return 0, fmt.Errorf("invalid value for %s: %q: %w", key, val, err)
	}
	return f, nil
}
