// MCP server configuration file support.
//
// Supports Anthropic-style MCP configuration format:
//
//	{
//	  "mcpServers": {
//	    "filesystem": {
//	      "command": "npx",
//	      "args": ["-y", "@modelcontextprotocol/server-filesystem", "/tmp"]
//	    },
//	    "memory": {
//	      "command": "npx",
//	      "args": ["-y", "@modelcontextprotocol/server-memory"]
//	    }
//	  }
//	}
package mcp

import (
	"encoding/json"
	"fmt"
	"os"
)

// Config represents the MCP configuration file format.
type Config struct {
	MCPServers map[string]ServerConfig `json:"mcpServers"`
}

// ServerConfig represents a single MCP server configuration.
type ServerConfig struct {
	Command string            `json:"command"`
	Args    []string          `json:"args"`
	Env     map[string]string `json:"env,omitempty"`
}

// LoadConfig loads MCP configuration from a JSON file.
func LoadConfig(path string) (*Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("failed to read config file: %w", err)
	}

	var config Config
	if err := json.Unmarshal(data, &config); err != nil {
		return nil, fmt.Errorf("failed to parse config file: %w", err)
	}

	return &config, nil
}

// ServerCommands returns a list of server command strings for each configured server.
// Each command string is in the format "command arg1 arg2 ...".
func (c *Config) ServerCommands() []string {
	var commands []string
	for _, server := range c.MCPServers {
		cmd := server.Command
		for _, arg := range server.Args {
			cmd += " " + arg
		}
		commands = append(commands, cmd)
	}
	return commands
}
