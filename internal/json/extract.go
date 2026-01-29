// Package json provides JSON extraction utilities for parsing LLM responses.
//
// LLMs often return JSON embedded in text or with additional commentary.
// This package provides utilities to extract and parse JSON from such responses.
package json

import (
	"encoding/json"
	"fmt"
	"strings"
)

// extractJSON finds and returns the JSON portion of a response string.
// It handles common LLM response patterns:
// 1. Pure JSON response - returns the full response
// 2. JSON wrapped in markdown code blocks (```json ... ```)
// 3. JSON object embedded in text - finds first '{' and last '}'
//
// Limitations:
// - Only handles JSON objects, not arrays
// - Uses simple brace matching, not full JSON parsing
// - May fail if braces appear in strings or are unbalanced
func extractJSON(response string) (string, error) {
	// Strip markdown code blocks if present
	response = stripMarkdownCodeBlocks(response)

	// Try full response first
	var test interface{}
	if err := json.Unmarshal([]byte(response), &test); err == nil {
		return response, nil
	}

	// Try to find and extract JSON from the response
	start := strings.Index(response, "{")
	if start != -1 {
		end := strings.LastIndex(response, "}")
		if end != -1 && end > start {
			jsonStr := response[start : end+1]
			var test interface{}
			if err := json.Unmarshal([]byte(jsonStr), &test); err == nil {
				return jsonStr, nil
			}
		}
	}

	// Create a preview for the error message
	preview := response
	if len(preview) > 100 {
		preview = preview[:100] + "..."
	}
	return "", fmt.Errorf("failed to extract valid JSON from response: %q", preview)
}

// stripMarkdownCodeBlocks removes markdown code block markers from a response.
// Handles patterns like ```json\n...\n``` or ```\n...\n```
func stripMarkdownCodeBlocks(response string) string {
	// Check for ```json or ``` at the start
	trimmed := strings.TrimSpace(response)

	// Handle ```json prefix
	if strings.HasPrefix(trimmed, "```json") {
		trimmed = strings.TrimPrefix(trimmed, "```json")
		trimmed = strings.TrimSpace(trimmed)
	} else if strings.HasPrefix(trimmed, "```") {
		trimmed = strings.TrimPrefix(trimmed, "```")
		trimmed = strings.TrimSpace(trimmed)
	}

	// Handle ``` suffix
	if strings.HasSuffix(trimmed, "```") {
		trimmed = strings.TrimSuffix(trimmed, "```")
		trimmed = strings.TrimSpace(trimmed)
	}

	return trimmed
}

// ExtractJSONFromResponse extracts and parses JSON from an LLM response.
//
// This function handles common LLM response patterns:
// 1. Pure JSON response - parses directly
// 2. JSON object embedded in text - finds first '{' and last '}'
//
// Limitations:
// - Only handles JSON objects, not arrays
// - Uses simple brace matching, not full JSON parsing
//
// Returns the parsed value or an error if extraction fails.
func ExtractJSONFromResponse[T any](response string) (T, error) {
	var result T
	jsonStr, err := extractJSON(response)
	if err != nil {
		return result, err
	}
	if err := json.Unmarshal([]byte(jsonStr), &result); err != nil {
		return result, fmt.Errorf("failed to unmarshal JSON: %w", err)
	}
	return result, nil
}

// ExtractJSONFromResponseWithType extracts JSON from a response into a provided pointer.
// This is the non-generic version for cases where generics aren't suitable.
func ExtractJSONFromResponseWithType(response string, result interface{}) error {
	jsonStr, err := extractJSON(response)
	if err != nil {
		return err
	}
	if err := json.Unmarshal([]byte(jsonStr), result); err != nil {
		return fmt.Errorf("failed to unmarshal JSON: %w", err)
	}
	return nil
}

// ExtractJSON extracts the JSON portion from a response string.
// Returns the raw JSON string suitable for further processing.
func ExtractJSON(response string) (string, error) {
	return extractJSON(response)
}
