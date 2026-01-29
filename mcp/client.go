// Package mcp provides Model Context Protocol (MCP) client implementation.
//
// MCP is a protocol for communication between AI models and tool providers.
// This package provides a client that can connect to MCP servers and execute
// tools through JSON-RPC over stdin/stdout.
//
// Information Hiding:
// - Process management hidden
// - JSON-RPC protocol details hidden
// - Request ID tracking hidden

package mcp

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os/exec"
	"sync"
)

// Client communicates with an MCP server via JSON-RPC over stdin/stdout.
type Client struct {
	cmd       *exec.Cmd
	stdin     io.WriteCloser
	stdout    *bufio.Reader
	requestID uint64
	mu        sync.Mutex
}

// mcpRequest is a JSON-RPC request to an MCP server.
type mcpRequest struct {
	JSONRPC string      `json:"jsonrpc"`
	ID      uint64      `json:"id"`
	Method  string      `json:"method"`
	Params  interface{} `json:"params,omitempty"`
}

// mcpResponse is a JSON-RPC response from an MCP server.
type mcpResponse struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      uint64          `json:"id"`
	Result  json.RawMessage `json:"result,omitempty"`
	Error   *mcpError       `json:"error,omitempty"`
}

// mcpError is a JSON-RPC error.
type mcpError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

// ToolInfo describes a tool available on the MCP server.
type ToolInfo struct {
	Name        string          `json:"name"`
	Description *string         `json:"description,omitempty"`
	InputSchema json.RawMessage `json:"inputSchema"`
}

// toolsListResult is the result of tools/list method.
type toolsListResult struct {
	Tools []ToolInfo `json:"tools"`
}

// NewClient creates a new MCP client by starting the given command.
// The command is expected to be an MCP server that communicates via stdin/stdout.
func NewClient(ctx context.Context, command string, args ...string) (*Client, error) {
	cmd := exec.CommandContext(ctx, command, args...)

	stdin, err := cmd.StdinPipe()
	if err != nil {
		return nil, fmt.Errorf("failed to get stdin pipe: %w", err)
	}

	stdout, err := cmd.StdoutPipe()
	if err != nil {
		stdin.Close()
		return nil, fmt.Errorf("failed to get stdout pipe: %w", err)
	}

	if err := cmd.Start(); err != nil {
		stdin.Close()
		return nil, fmt.Errorf("failed to start MCP server: %w", err)
	}

	client := &Client{
		cmd:       cmd,
		stdin:     stdin,
		stdout:    bufio.NewReader(stdout),
		requestID: 0,
	}

	if err := client.initialize(ctx); err != nil {
		client.Close()
		return nil, fmt.Errorf("failed to initialize MCP client: %w", err)
	}

	return client, nil
}

// initialize sends the initialize request to the MCP server.
func (c *Client) initialize(ctx context.Context) error {
	params := map[string]interface{}{
		"protocolVersion": "2024-11-05",
		"capabilities":    map[string]interface{}{},
		"clientInfo": map[string]interface{}{
			"name":    "davingo",
			"version": "0.1.0",
		},
	}

	_, err := c.call(ctx, "initialize", params)
	return err
}

// ListTools returns all tools available on the MCP server.
func (c *Client) ListTools(ctx context.Context) ([]ToolInfo, error) {
	result, err := c.call(ctx, "tools/list", nil)
	if err != nil {
		return nil, err
	}

	var toolsResult toolsListResult
	if err := json.Unmarshal(result, &toolsResult); err != nil {
		return nil, fmt.Errorf("failed to parse tools list: %w", err)
	}

	return toolsResult.Tools, nil
}

// CallTool calls a tool on the MCP server with the given arguments.
func (c *Client) CallTool(ctx context.Context, name string, arguments json.RawMessage) (json.RawMessage, error) {
	params := map[string]interface{}{
		"name":      name,
		"arguments": json.RawMessage(arguments),
	}

	return c.call(ctx, "tools/call", params)
}

// call sends a JSON-RPC request and returns the result.
func (c *Client) call(ctx context.Context, method string, params interface{}) (json.RawMessage, error) {
	c.mu.Lock()
	defer c.mu.Unlock()

	// Check context before sending
	if ctx.Err() != nil {
		return nil, ctx.Err()
	}

	c.requestID++
	request := mcpRequest{
		JSONRPC: "2.0",
		ID:      c.requestID,
		Method:  method,
		Params:  params,
	}

	// Send request
	reqJSON, err := json.Marshal(request)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal request: %w", err)
	}

	if _, err := c.stdin.Write(append(reqJSON, '\n')); err != nil {
		return nil, fmt.Errorf("failed to write request: %w", err)
	}

	// Read response
	line, err := c.stdout.ReadBytes('\n')
	if err != nil {
		return nil, fmt.Errorf("failed to read response: %w", err)
	}

	var response mcpResponse
	if err := json.Unmarshal(line, &response); err != nil {
		return nil, fmt.Errorf("failed to parse response: %w", err)
	}

	if response.Error != nil {
		return nil, fmt.Errorf("MCP error %d: %s", response.Error.Code, response.Error.Message)
	}

	return response.Result, nil
}

// Close stops the MCP server process and releases resources.
func (c *Client) Close() error {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.stdin != nil {
		c.stdin.Close()
	}

	if c.cmd != nil && c.cmd.Process != nil {
		_ = c.cmd.Process.Kill() // Intentionally ignore - cleanup
		_ = c.cmd.Wait()         // Intentionally ignore - cleanup
	}

	return nil
}
