// MCP Tool Wrapper - Makes MCP tools usable in the agent system.
//
// Information Hiding:
// - MCP client lifecycle hidden
// - Schema parsing hidden
// - Tool execution coordination hidden

package mcp

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"

	"github.com/richinex/davingo/tools"
)

// ToolManager manages a set of MCP tools sharing a single client.
// The caller must call Close() when done to release resources.
type ToolManager struct {
	client *Client
	tools  []tools.Tool
}

// Tools returns the discovered tools.
func (m *ToolManager) Tools() []tools.Tool {
	return m.tools
}

// Close closes the MCP client and releases resources.
func (m *ToolManager) Close() error {
	if m.client != nil {
		return m.client.Close()
	}
	return nil
}

// sharedClientToolWrapper wraps an MCP tool with a shared client.
type sharedClientToolWrapper struct {
	client      *Client
	toolName    string
	description string
	inputSchema json.RawMessage
}

// ToolWrapper wraps an MCP tool to make it usable in the agent system.
// It implements the tools.Tool interface.
// Note: Each ToolWrapper spawns a new client per execution.
// For better efficiency, use DiscoverTools which shares a client.
type ToolWrapper struct {
	toolName      string
	description   string
	inputSchema   json.RawMessage
	serverCommand string
	serverArgs    []string
}

// NewToolWrapper creates a new MCP tool wrapper.
// Note: Each execution spawns a new MCP server process.
// For better efficiency, use DiscoverTools which shares a client.
func NewToolWrapper(info ToolInfo, serverCommand string, serverArgs []string) *ToolWrapper {
	description := ""
	if info.Description != nil {
		description = *info.Description
	}

	return &ToolWrapper{
		toolName:      info.Name,
		description:   description,
		inputSchema:   info.InputSchema,
		serverCommand: serverCommand,
		serverArgs:    serverArgs,
	}
}

// Metadata returns the tool metadata extracted from the MCP schema.
func (w *ToolWrapper) Metadata() tools.ToolMetadata {
	return tools.ToolMetadata{
		Name:        w.toolName,
		Description: w.description,
		Parameters:  parseParameters(w.inputSchema),
	}
}

// parseParameters extracts tool parameters from the JSON schema.
// Returns parameters in sorted order for deterministic output.
func parseParameters(inputSchema json.RawMessage) []tools.ToolParameter {
	var schema struct {
		Properties map[string]struct {
			Type        string `json:"type"`
			Description string `json:"description"`
		} `json:"properties"`
		Required []string `json:"required"`
	}

	if err := json.Unmarshal(inputSchema, &schema); err != nil {
		return nil
	}

	requiredSet := make(map[string]bool)
	for _, r := range schema.Required {
		requiredSet[r] = true
	}

	// Extract and sort parameter names for deterministic output
	names := make([]string, 0, len(schema.Properties))
	for name := range schema.Properties {
		names = append(names, name)
	}
	sort.Strings(names)

	params := make([]tools.ToolParameter, 0, len(names))
	for _, name := range names {
		prop := schema.Properties[name]
		paramType := prop.Type
		if paramType == "" {
			paramType = "string"
		}

		params = append(params, tools.ToolParameter{
			Name:        name,
			Description: prop.Description,
			ParamType:   paramType,
			Required:    requiredSet[name],
		})
	}

	return params
}

// Execute calls the MCP tool with the given arguments.
// Note: Creates a new MCP client for each execution.
// For better efficiency, use DiscoverTools which shares a client.
func (w *ToolWrapper) Execute(ctx context.Context, args json.RawMessage) (tools.ToolResult, error) {
	client, err := NewClient(ctx, w.serverCommand, w.serverArgs...)
	if err != nil {
		return tools.ToolResult{}, fmt.Errorf("failed to create MCP client: %w", err)
	}
	defer client.Close()

	result, err := client.CallTool(ctx, w.toolName, args)
	if err != nil {
		return tools.ToolResult{}, fmt.Errorf("tool call failed: %w", err)
	}

	return formatResult(result), nil
}

// Validate validates that arguments are valid JSON.
// Note: Schema validation is performed by the MCP server.
func (w *ToolWrapper) Validate(args json.RawMessage) error {
	var v interface{}
	if err := json.Unmarshal(args, &v); err != nil {
		return fmt.Errorf("invalid JSON arguments: %w", err)
	}
	return nil
}

// formatResult formats the result as pretty JSON if possible.
func formatResult(result json.RawMessage) tools.ToolResult {
	var v interface{}
	if err := json.Unmarshal(result, &v); err != nil {
		// Not valid JSON, return as-is
		return tools.SuccessResult(string(result))
	}

	// If unmarshal succeeded, marshal should never fail
	pretty, _ := json.MarshalIndent(v, "", "  ")
	return tools.SuccessResult(string(pretty))
}

// DiscoverTools discovers all tools from an MCP server and returns a ToolManager.
// The ToolManager shares a single client across all tools for efficiency.
// The caller MUST call ToolManager.Close() when done to release resources.
//
// Example:
//
//	manager, err := mcp.DiscoverTools(ctx, "npx", "-y", "@modelcontextprotocol/server-brave-search")
//	if err != nil {
//	    return err
//	}
//	defer manager.Close()
//
//	for _, tool := range manager.Tools() {
//	    // Use tool
//	}
func DiscoverTools(ctx context.Context, serverCommand string, serverArgs ...string) (*ToolManager, error) {
	client, err := NewClient(ctx, serverCommand, serverArgs...)
	if err != nil {
		return nil, fmt.Errorf("failed to connect to MCP server: %w", err)
	}

	toolInfos, err := client.ListTools(ctx)
	if err != nil {
		client.Close()
		return nil, fmt.Errorf("failed to list tools: %w", err)
	}

	result := make([]tools.Tool, len(toolInfos))
	for i, info := range toolInfos {
		result[i] = &sharedClientToolWrapper{
			client:      client,
			toolName:    info.Name,
			description: stringValue(info.Description),
			inputSchema: info.InputSchema,
		}
	}

	return &ToolManager{
		client: client,
		tools:  result,
	}, nil
}

// stringValue returns empty string for nil pointers.
func stringValue(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}

// sharedClientToolWrapper methods

// Metadata returns the tool metadata extracted from the MCP schema.
func (w *sharedClientToolWrapper) Metadata() tools.ToolMetadata {
	return tools.ToolMetadata{
		Name:        w.toolName,
		Description: w.description,
		Parameters:  parseParameters(w.inputSchema),
	}
}

// Execute calls the MCP tool using the shared client.
func (w *sharedClientToolWrapper) Execute(ctx context.Context, args json.RawMessage) (tools.ToolResult, error) {
	result, err := w.client.CallTool(ctx, w.toolName, args)
	if err != nil {
		return tools.ToolResult{}, fmt.Errorf("tool call failed: %w", err)
	}

	return formatResult(result), nil
}

// Validate validates that arguments are valid JSON.
// Note: Schema validation is performed by the MCP server.
func (w *sharedClientToolWrapper) Validate(args json.RawMessage) error {
	var v interface{}
	if err := json.Unmarshal(args, &v); err != nil {
		return fmt.Errorf("invalid JSON arguments: %w", err)
	}
	return nil
}
