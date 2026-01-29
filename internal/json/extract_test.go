package json

import (
	"strings"
	"testing"
)

type TestStruct struct {
	Name  string `json:"name"`
	Value int    `json:"value"`
}

func TestPureJSON(t *testing.T) {
	response := `{"name": "test", "value": 42}`
	result, err := ExtractJSONFromResponse[TestStruct](response)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Name != "test" {
		t.Errorf("expected name 'test', got '%s'", result.Name)
	}
	if result.Value != 42 {
		t.Errorf("expected value 42, got %d", result.Value)
	}
}

func TestJSONWithPrefix(t *testing.T) {
	response := `Here is the result: {"name": "test", "value": 42}`
	result, err := ExtractJSONFromResponse[TestStruct](response)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Name != "test" {
		t.Errorf("expected name 'test', got '%s'", result.Name)
	}
	if result.Value != 42 {
		t.Errorf("expected value 42, got %d", result.Value)
	}
}

func TestJSONWithSuffix(t *testing.T) {
	response := `{"name": "test", "value": 42} That's the output.`
	result, err := ExtractJSONFromResponse[TestStruct](response)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Name != "test" {
		t.Errorf("expected name 'test', got '%s'", result.Name)
	}
	if result.Value != 42 {
		t.Errorf("expected value 42, got %d", result.Value)
	}
}

func TestJSONWithBoth(t *testing.T) {
	response := `Let me think... {"name": "test", "value": 42} Done!`
	result, err := ExtractJSONFromResponse[TestStruct](response)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Name != "test" {
		t.Errorf("expected name 'test', got '%s'", result.Name)
	}
	if result.Value != 42 {
		t.Errorf("expected value 42, got %d", result.Value)
	}
}

func TestNoJSON(t *testing.T) {
	response := "This is just plain text without any JSON."
	_, err := ExtractJSONFromResponse[TestStruct](response)
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	// Error should contain a preview of the response
	if !strings.Contains(err.Error(), "failed to extract valid JSON") {
		t.Errorf("expected 'failed to extract valid JSON' in error, got: %v", err)
	}
}

func TestInvalidJSON(t *testing.T) {
	response := `{"name": "test", value: }`
	_, err := ExtractJSONFromResponse[TestStruct](response)
	if err == nil {
		t.Fatal("expected error, got nil")
	}
}
