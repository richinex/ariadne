// Handoff Protocol for Multi-Agent Coordination.
//
// Structured handoff protocols between agents,
// ensuring data quality and contract compliance.
//
// Information Hiding:
// - Contract storage and lookup hidden
// - Validation logic hidden

package orchestration

import (
	"encoding/json"
	"fmt"
	"sync"
)

// Contract defines expected output from an agent.
type Contract struct {
	FromAgent          string
	ToAgent            *string // nil if no specific target
	Schema             OutputSchema
	MaxExecutionTimeMs *uint64
}

// Coordinator manages handoff contracts between agents.
type Coordinator struct {
	mu        sync.RWMutex
	contracts map[string]Contract
}

// NewCoordinator creates a new handoff coordinator.
func NewCoordinator() *Coordinator {
	return &Coordinator{
		contracts: make(map[string]Contract),
	}
}

// RegisterContract registers a handoff contract between agents.
func (c *Coordinator) RegisterContract(name string, contract Contract) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.contracts[name] = contract
}

// Validate validates agent output against a handoff contract.
func (c *Coordinator) Validate(contractName string, response *Response) ValidationResult {
	c.mu.RLock()
	contract, exists := c.contracts[contractName]
	c.mu.RUnlock()

	if !exists {
		return NewValidationFailure([]ValidationError{{
			Field:     "contract",
			ErrorType: "ContractNotFound",
			Message:   fmt.Sprintf("Handoff contract '%s' not registered", contractName),
		}})
	}

	var errors []ValidationError
	var warnings []string

	// Check response type
	switch response.Type {
	case ResponseFailure:
		expected := "Success"
		actual := "Failure"
		errors = append(errors, ValidationError{
			Field:     "response",
			ErrorType: "AgentFailure",
			Message:   "Agent failed to complete task",
			Expected:  &expected,
			Actual:    &actual,
		})
		return NewValidationFailure(errors)

	case ResponseTimeout:
		expected := "Success"
		actual := "Timeout"
		errors = append(errors, ValidationError{
			Field:     "response",
			ErrorType: "AgentTimeout",
			Message:   "Agent timed out before completing task",
			Expected:  &expected,
			Actual:    &actual,
		})
		return NewValidationFailure(errors)

	case ResponseSuccess:
		// Continue with validation
	}

	// Check execution time limit
	if response.Metadata != nil && contract.MaxExecutionTimeMs != nil {
		if response.Metadata.ExecutionTimeMs > *contract.MaxExecutionTimeMs {
			warnings = append(warnings, fmt.Sprintf(
				"Execution time (%dms) exceeded limit (%dms)",
				response.Metadata.ExecutionTimeMs,
				*contract.MaxExecutionTimeMs,
			))
		}
	}

	// Validate required fields if result is JSON
	var jsonValue map[string]interface{}
	if err := json.Unmarshal([]byte(response.Result), &jsonValue); err == nil {
		for _, field := range contract.Schema.RequiredFields {
			if _, exists := jsonValue[field]; !exists {
				expected := "present"
				actual := "missing"
				errors = append(errors, ValidationError{
					Field:     field,
					ErrorType: "MissingRequired",
					Message:   fmt.Sprintf("Required field '%s' is missing", field),
					Expected:  &expected,
					Actual:    &actual,
				})
			}
		}
	}

	if len(errors) == 0 {
		return NewValidationSuccess().WithWarnings(warnings)
	}
	return NewValidationFailure(errors).WithWarnings(warnings)
}

// ContractNames returns all registered contract names.
func (c *Coordinator) ContractNames() []string {
	c.mu.RLock()
	defer c.mu.RUnlock()

	names := make([]string, 0, len(c.contracts))
	for name := range c.contracts {
		names = append(names, name)
	}
	return names
}

// GetContract retrieves a contract by name.
func (c *Coordinator) GetContract(name string) (Contract, bool) {
	c.mu.RLock()
	defer c.mu.RUnlock()
	contract, exists := c.contracts[name]
	return contract, exists
}
