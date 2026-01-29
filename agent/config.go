// Agent configuration types.
//
// Information Hiding:
// - Configuration validation logic hidden
// - Default values hidden

package agent

import (
	"encoding/json"

	"github.com/richinex/ariadne/tools"
)

// Config holds agent configuration.
// Following Dave's naming advice: use agent.Config, not agent.AgentConfig.
type Config struct {
	// Name is a unique identifier for the agent.
	Name string

	// Description explains what this agent does (used by routers/supervisors).
	Description string

	// SystemPrompt guides the agent's behavior.
	SystemPrompt string

	// Tools available to this agent.
	Tools []tools.Tool

	// ResponseSchema is an optional JSON schema for structured outputs.
	ResponseSchema json.RawMessage

	// ReturnToolOutput returns the last tool output instead of final_answer.
	ReturnToolOutput bool
}

// DefaultConfig returns a basic agent configuration.
func DefaultConfig() Config {
	return Config{
		Name:         "agent",
		Description:  "A general-purpose agent",
		SystemPrompt: "You are a helpful assistant.",
		Tools:        []tools.Tool{},
	}
}

// HasTools returns true if the agent has tools configured.
func (c *Config) HasTools() bool {
	return len(c.Tools) > 0
}

// HasResponseSchema returns true if a response schema is configured.
func (c *Config) HasResponseSchema() bool {
	return len(c.ResponseSchema) > 0
}
