// Agent builder for fluent configuration.
//
// Information Hiding:
// - Builder state management hidden
// - Default value application hidden

package agent

import (
	"encoding/json"
	"fmt"

	"github.com/richinex/davingo/tools"
)

// Builder provides fluent configuration for creating agents.
// Usage: agent.NewBuilder("name") - no stutter.
type Builder struct {
	name             string
	description      string
	systemPrompt     string
	tools            []tools.Tool
	responseSchema   json.RawMessage
	returnToolOutput bool
}

// NewBuilder creates a new agent builder with the given name.
func NewBuilder(name string) *Builder {
	return &Builder{
		name:  name,
		tools: []tools.Tool{},
	}
}

// Description sets the agent's description.
func (b *Builder) Description(description string) *Builder {
	b.description = description
	return b
}

// SystemPrompt sets the agent's system prompt.
func (b *Builder) SystemPrompt(prompt string) *Builder {
	b.systemPrompt = prompt
	return b
}

// Tool adds a tool to the agent.
func (b *Builder) Tool(tool tools.Tool) *Builder {
	b.tools = append(b.tools, tool)
	return b
}

// Tools adds multiple tools at once.
func (b *Builder) Tools(toolList []tools.Tool) *Builder {
	b.tools = append(b.tools, toolList...)
	return b
}

// ResponseSchema sets the JSON schema for structured outputs.
func (b *Builder) ResponseSchema(schema json.RawMessage) *Builder {
	b.responseSchema = schema
	return b
}

// ReturnToolOutput configures the agent to return tool output directly.
func (b *Builder) ReturnToolOutput(enabled bool) *Builder {
	b.returnToolOutput = enabled
	return b
}

// Build creates the agent configuration.
func (b *Builder) Build() Config {
	description := b.description
	if description == "" {
		description = fmt.Sprintf("Agent: %s", b.name)
	}

	systemPrompt := b.systemPrompt
	if systemPrompt == "" {
		systemPrompt = fmt.Sprintf(
			"You are an agent named %s. Use available tools to complete tasks.",
			b.name,
		)
	}

	return Config{
		Name:             b.name,
		Description:      description,
		SystemPrompt:     systemPrompt,
		Tools:            b.tools,
		ResponseSchema:   b.responseSchema,
		ReturnToolOutput: b.returnToolOutput,
	}
}

// Name returns the builder's agent name.
func (b *Builder) Name() string {
	return b.name
}

// ToolCount returns the number of tools registered.
func (b *Builder) ToolCount() int {
	return len(b.tools)
}

// Collection manages multiple agent configurations.
type Collection struct {
	configs []Config
}

// NewCollection creates an empty agent collection.
func NewCollection() *Collection {
	return &Collection{
		configs: []Config{},
	}
}

// Add adds an agent from a builder.
func (c *Collection) Add(builder *Builder) *Collection {
	c.configs = append(c.configs, builder.Build())
	return c
}

// AddConfig adds a pre-built config.
func (c *Collection) AddConfig(config Config) *Collection {
	c.configs = append(c.configs, config)
	return c
}

// Build returns all configurations.
func (c *Collection) Build() []Config {
	return c.configs
}

// Len returns the number of agents.
func (c *Collection) Len() int {
	return len(c.configs)
}

// AgentInfo describes an agent's basic information.
type AgentInfo struct {
	Name        string
	Description string
}

// List returns agent names and descriptions.
func (c *Collection) List() []AgentInfo {
	result := make([]AgentInfo, len(c.configs))
	for i, cfg := range c.configs {
		result[i] = AgentInfo{
			Name:        cfg.Name,
			Description: cfg.Description,
		}
	}
	return result
}
