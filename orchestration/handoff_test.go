package orchestration

import (
	"testing"
)

func TestHandoffValidationSuccess(t *testing.T) {
	coordinator := NewCoordinator()

	maxTime := uint64(5000)
	coordinator.RegisterContract("test_contract", Contract{
		FromAgent: "agent_a",
		ToAgent:   stringPtr("agent_b"),
		Schema: OutputSchema{
			SchemaVersion:  "1.0",
			RequiredFields: []string{"result"},
		},
		MaxExecutionTimeMs: &maxTime,
	})

	response := NewSuccessResponse(
		`{"result": "success"}`,
		nil,
		&Metadata{ExecutionTimeMs: 1000},
		&CompletionStatus{Type: StatusComplete},
	)

	validation := coordinator.Validate("test_contract", &response)

	if !validation.Valid {
		t.Errorf("expected validation to pass, got errors: %v", validation.Errors)
	}
}

func TestHandoffValidationTimeoutWarning(t *testing.T) {
	coordinator := NewCoordinator()

	maxTime := uint64(1000)
	coordinator.RegisterContract("test_contract", Contract{
		FromAgent: "agent_a",
		ToAgent:   stringPtr("agent_b"),
		Schema: OutputSchema{
			SchemaVersion: "1.0",
		},
		MaxExecutionTimeMs: &maxTime,
	})

	response := NewSuccessResponse(
		"success",
		nil,
		&Metadata{ExecutionTimeMs: 2000},
		&CompletionStatus{Type: StatusComplete},
	)

	validation := coordinator.Validate("test_contract", &response)

	if !validation.Valid {
		t.Errorf("expected validation to pass (with warnings), got errors: %v", validation.Errors)
	}

	if len(validation.Warnings) == 0 {
		t.Error("expected warnings about execution time")
	}
}

func TestHandoffValidationContractNotFound(t *testing.T) {
	coordinator := NewCoordinator()

	response := NewSuccessResponse(
		"success",
		nil,
		nil,
		nil,
	)

	validation := coordinator.Validate("nonexistent", &response)

	if validation.Valid {
		t.Error("expected validation to fail for nonexistent contract")
	}

	if len(validation.Errors) == 0 {
		t.Error("expected error about contract not found")
	}

	if validation.Errors[0].ErrorType != "ContractNotFound" {
		t.Errorf("expected ContractNotFound error, got: %s", validation.Errors[0].ErrorType)
	}
}

func TestHandoffValidationMissingRequiredField(t *testing.T) {
	coordinator := NewCoordinator()

	coordinator.RegisterContract("test_contract", Contract{
		FromAgent: "agent_a",
		Schema: OutputSchema{
			SchemaVersion:  "1.0",
			RequiredFields: []string{"name", "value"},
		},
	})

	response := NewSuccessResponse(
		`{"name": "test"}`,
		nil,
		nil,
		nil,
	)

	validation := coordinator.Validate("test_contract", &response)

	if validation.Valid {
		t.Error("expected validation to fail for missing required field")
	}

	// Check that we have an error for the missing "value" field
	found := false
	for _, err := range validation.Errors {
		if err.Field == "value" && err.ErrorType == "MissingRequired" {
			found = true
			break
		}
	}

	if !found {
		t.Errorf("expected MissingRequired error for 'value' field, got: %v", validation.Errors)
	}
}

func TestHandoffValidationFailureResponse(t *testing.T) {
	coordinator := NewCoordinator()

	coordinator.RegisterContract("test_contract", Contract{
		FromAgent: "agent_a",
		Schema:    OutputSchema{SchemaVersion: "1.0"},
	})

	response := NewFailureResponse(
		"agent failed",
		nil,
		nil,
		nil,
	)

	validation := coordinator.Validate("test_contract", &response)

	if validation.Valid {
		t.Error("expected validation to fail for failure response")
	}

	if len(validation.Errors) == 0 || validation.Errors[0].ErrorType != "AgentFailure" {
		t.Errorf("expected AgentFailure error, got: %v", validation.Errors)
	}
}

func TestHandoffValidationTimeoutResponse(t *testing.T) {
	coordinator := NewCoordinator()

	coordinator.RegisterContract("test_contract", Contract{
		FromAgent: "agent_a",
		Schema:    OutputSchema{SchemaVersion: "1.0"},
	})

	response := NewTimeoutResponse(
		"partial result",
		nil,
		nil,
		nil,
	)

	validation := coordinator.Validate("test_contract", &response)

	if validation.Valid {
		t.Error("expected validation to fail for timeout response")
	}

	if len(validation.Errors) == 0 || validation.Errors[0].ErrorType != "AgentTimeout" {
		t.Errorf("expected AgentTimeout error, got: %v", validation.Errors)
	}
}

func stringPtr(s string) *string {
	return &s
}
