package config

import (
	"os"
	"testing"
)

func TestNewValidProvider(t *testing.T) {
	settings, err := New("openai")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if settings.LLM.Provider != "openai" {
		t.Errorf("expected provider 'openai', got %q", settings.LLM.Provider)
	}
}

func TestNewWithAlias(t *testing.T) {
	settings, err := New("claude")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if settings.LLM.Provider != "anthropic" {
		t.Errorf("expected provider 'anthropic' (normalized from 'claude'), got %q", settings.LLM.Provider)
	}
}

func TestNewUnknownProvider(t *testing.T) {
	_, err := New("unknown_provider")
	if err == nil {
		t.Error("expected error for unknown provider")
	}
}

func TestAPIKeyForValidProvider(t *testing.T) {
	original := os.Getenv("OPENAI_API_KEY")
	os.Setenv("OPENAI_API_KEY", "test-key")
	defer os.Setenv("OPENAI_API_KEY", original)

	key, err := APIKeyFor("openai")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if key != "test-key" {
		t.Errorf("expected 'test-key', got %q", key)
	}
}

func TestAPIKeyForMissing(t *testing.T) {
	original := os.Getenv("OPENAI_API_KEY")
	os.Unsetenv("OPENAI_API_KEY")
	defer os.Setenv("OPENAI_API_KEY", original)

	_, err := APIKeyFor("openai")
	if err == nil {
		t.Error("expected error for missing API key")
	}
}

func TestAPIKeyForUnknownProvider(t *testing.T) {
	_, err := APIKeyFor("unknown")
	if err == nil {
		t.Error("expected error for unknown provider")
	}
}

func TestModelFor(t *testing.T) {
	model, err := ModelFor("openai")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if model == "" {
		t.Error("expected non-empty model")
	}
}

func TestNewWithInvalidEnvVar(t *testing.T) {
	original := os.Getenv("LLM_MAX_TOKENS")
	os.Setenv("LLM_MAX_TOKENS", "not-a-number")
	defer os.Setenv("LLM_MAX_TOKENS", original)

	_, err := New("openai")
	if err == nil {
		t.Error("expected error for invalid LLM_MAX_TOKENS")
	}
}

func TestMustNewPanics(t *testing.T) {
	defer func() {
		if r := recover(); r == nil {
			t.Error("expected panic for unknown provider")
		}
	}()
	MustNew("unknown_provider")
}

func TestSupportedProviders(t *testing.T) {
	providers := SupportedProviders()
	if len(providers) == 0 {
		t.Error("expected at least one supported provider")
	}
}
