// Package tools provides tool management and registration.
//
// Information Hiding:
// - Tool storage and lookup implementation hidden
// - Tool lifecycle management hidden
// - Registration and discovery mechanisms abstracted

package tools

import (
	"fmt"
	"sort"
	"strings"
	"sync"
)

// Registry manages available tools with dynamic registration.
type Registry struct {
	mu    sync.RWMutex
	tools map[string]Tool
}

// NewRegistry creates a new empty tool registry.
func NewRegistry() *Registry {
	return &Registry{
		tools: make(map[string]Tool),
	}
}

// Register adds a new tool to the registry.
// Returns error if a tool with the same name already exists.
func (r *Registry) Register(tool Tool) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	name := tool.Metadata().Name
	if _, exists := r.tools[name]; exists {
		return fmt.Errorf("tool '%s' already registered", name)
	}
	r.tools[name] = tool
	return nil
}

// Get returns a tool by name.
func (r *Registry) Get(name string) (Tool, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	tool, exists := r.tools[name]
	return tool, exists
}

// Has checks if a tool exists in the registry.
func (r *Registry) Has(name string) bool {
	r.mu.RLock()
	defer r.mu.RUnlock()

	_, exists := r.tools[name]
	return exists
}

// Names returns all registered tool names in sorted order.
func (r *Registry) Names() []string {
	r.mu.RLock()
	defer r.mu.RUnlock()

	names := make([]string, 0, len(r.tools))
	for name := range r.tools {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

// List returns metadata for all registered tools.
func (r *Registry) List() []ToolMetadata {
	r.mu.RLock()
	defer r.mu.RUnlock()

	metadata := make([]ToolMetadata, 0, len(r.tools))
	for _, tool := range r.tools {
		metadata = append(metadata, tool.Metadata())
	}
	return metadata
}

// Description returns a formatted description of all tools for LLM prompts.
func (r *Registry) Description() string {
	r.mu.RLock()
	defer r.mu.RUnlock()

	var descriptions []string
	for _, tool := range r.tools {
		meta := tool.Metadata()
		var params []string
		for _, p := range meta.Parameters {
			required := "optional"
			if p.Required {
				required = "required"
			}
			params = append(params, fmt.Sprintf("  - %s (%s): %s [%s]",
				p.Name, p.ParamType, p.Description, required))
		}

		paramStr := strings.Join(params, "\n")
		descriptions = append(descriptions, fmt.Sprintf(
			"Tool: %s\nDescription: %s\nParameters:\n%s",
			meta.Name, meta.Description, paramStr))
	}

	return strings.Join(descriptions, "\n\n")
}

// Default timeout and file size constants for tools.
const (
	DefaultToolTimeout  = 30            // seconds
	DefaultMaxFileSize = 1024 * 1024    // 1MB
)

// WithDefaults creates a registry with common default tools.
// Returns error if any tool registration fails.
func WithDefaults() (*Registry, error) {
	registry := NewRegistry()

	tools := []Tool{
		NewBashTool(DefaultToolTimeout),
		NewShellTool(DefaultToolTimeout),
		NewReadFileTool(DefaultMaxFileSize),
		NewWriteFileTool(DefaultMaxFileSize),
		NewEditFileTool(DefaultMaxFileSize),
		NewAppendFileTool(DefaultMaxFileSize),
		NewHTTPTool(DefaultToolTimeout),
		NewRipgrepTool(DefaultToolTimeout),
	}

	for _, t := range tools {
		if err := registry.Register(t); err != nil {
			return nil, fmt.Errorf("failed to register default tools: %w", err)
		}
	}

	return registry, nil
}
