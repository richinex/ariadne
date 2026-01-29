// Package orchestration provides multi-agent coordination patterns.
//
// Types used by routers, supervisors, and validation.
package orchestration

import (
	"github.com/richinex/ariadne/model"
	"github.com/richinex/ariadne/llm"
)

// OutputSchema defines the schema for structured agent outputs.
type OutputSchema struct {
	SchemaVersion   string            `json:"schema_version"`
	RequiredFields  []string          `json:"required_fields"`
	OptionalFields  []string          `json:"optional_fields"`
	FieldTypes      map[string]string `json:"field_types"`
	ValidationRules []ValidationRule  `json:"validation_rules"`
}

// ValidationRule defines a validation rule for output fields.
type ValidationRule struct {
	Field      string         `json:"field"`
	RuleType   ValidationType `json:"rule_type"`
	Constraint string         `json:"constraint"`
}

// ValidationType represents types of validation.
type ValidationType string

const (
	ValidationMinLength ValidationType = "MinLength"
	ValidationMaxLength ValidationType = "MaxLength"
	ValidationPattern   ValidationType = "Pattern"
	ValidationRange     ValidationType = "Range"
	ValidationEnum      ValidationType = "Enum"
	ValidationCustom    ValidationType = "Custom"
)

// ValidationResult contains the result of validation with detailed feedback.
type ValidationResult struct {
	Valid    bool              `json:"valid"`
	Errors   []ValidationError `json:"errors"`
	Warnings []string          `json:"warnings"`
}

// ValidationError contains validation error details.
type ValidationError struct {
	Field     string  `json:"field"`
	ErrorType string  `json:"error_type"`
	Message   string  `json:"message"`
	Expected  *string `json:"expected,omitempty"`
	Actual    *string `json:"actual,omitempty"`
}

// NewValidationSuccess creates a successful validation result.
func NewValidationSuccess() ValidationResult {
	return ValidationResult{
		Valid:    true,
		Errors:   []ValidationError{},
		Warnings: []string{},
	}
}

// NewValidationFailure creates a failed validation result.
func NewValidationFailure(errors []ValidationError) ValidationResult {
	return ValidationResult{
		Valid:    false,
		Errors:   errors,
		Warnings: []string{},
	}
}

// WithWarnings adds warnings to the validation result.
func (v ValidationResult) WithWarnings(warnings []string) ValidationResult {
	v.Warnings = warnings
	return v
}

// CompletionStatusType represents the type of completion status.
type CompletionStatusType int

const (
	StatusComplete CompletionStatusType = iota
	StatusPartial
	StatusBlocked
	StatusFailed
)

// CompletionStatus contains completion status with additional context.
type CompletionStatus struct {
	Type        CompletionStatusType
	NextSteps   []string // For Partial
	Reason      string   // For Blocked
	Needs       []string // For Blocked
	Error       string   // For Failed
	Recoverable bool     // For Failed
}

// NewCompleteStatus creates a complete status.
func NewCompleteStatus() CompletionStatus {
	return CompletionStatus{Type: StatusComplete}
}

// NewPartialStatus creates a partial status with next steps.
func NewPartialStatus(nextSteps []string) CompletionStatus {
	return CompletionStatus{
		Type:      StatusPartial,
		NextSteps: nextSteps,
	}
}

// NewBlockedStatus creates a blocked status.
func NewBlockedStatus(reason string, needs []string) CompletionStatus {
	return CompletionStatus{
		Type:   StatusBlocked,
		Reason: reason,
		Needs:  needs,
	}
}

// NewFailedStatus creates a failed status.
func NewFailedStatus(err string, recoverable bool) CompletionStatus {
	return CompletionStatus{
		Type:        StatusFailed,
		Error:       err,
		Recoverable: recoverable,
	}
}

// Step is an alias for model.Step for orchestration steps.
type Step = model.Step

// ToolCallInfo is an alias for model.ToolCall for tool call metadata.
type ToolCallInfo = model.ToolCall

// TokenStats tracks token usage across an orchestration.
type TokenStats struct {
	PromptTokens     uint32 `json:"prompt_tokens"`
	CompletionTokens uint32 `json:"completion_tokens"`
	TotalTokens      uint32 `json:"total_tokens"`
	LLMCalls         int    `json:"llm_calls"`
	// Context savings from ResultStore
	BytesSaved    int `json:"bytes_saved,omitempty"`
	ResultsStored int `json:"results_stored,omitempty"`
}

// AddUsage adds token usage from an LLM call.
func (ts *TokenStats) AddUsage(usage *llm.TokenUsage) {
	if usage == nil {
		return
	}
	ts.PromptTokens += usage.PromptTokens
	ts.CompletionTokens += usage.CompletionTokens
	ts.TotalTokens += usage.TotalTokens
}

// Metadata contains metadata about orchestration execution.
type Metadata struct {
	ExecutionTimeMs  uint64            `json:"execution_time_ms"`
	TokensUsed       *uint32           `json:"tokens_used,omitempty"` // Deprecated: use TokenStats
	TokenStats       *TokenStats       `json:"token_stats,omitempty"`
	PartialResults   map[string]string `json:"partial_results"`
	SchemaVersion    *string           `json:"schema_version,omitempty"`
	ValidationResult *ValidationResult `json:"validation_result,omitempty"`
	AgentName        *string           `json:"agent_name,omitempty"`
	ToolCalls        []ToolCallInfo    `json:"tool_calls"`
}

// ResponseType indicates the type of orchestration response.
type ResponseType int

const (
	ResponseSuccess ResponseType = iota
	ResponseFailure
	ResponseTimeout
)

// Response represents a response from orchestrated agent execution.
// Used by routers and supervisors when coordinating agents.
type Response struct {
	Type             ResponseType
	Result           string // For Success
	Error            string // For Failure
	PartialResult    string // For Timeout
	Steps            []Step
	Metadata         *Metadata
	CompletionStatus *CompletionStatus
}

// NewSuccessResponse creates a successful orchestration response.
func NewSuccessResponse(result string, steps []Step, metadata *Metadata, status *CompletionStatus) Response {
	return Response{
		Type:             ResponseSuccess,
		Result:           result,
		Steps:            steps,
		Metadata:         metadata,
		CompletionStatus: status,
	}
}

// NewFailureResponse creates a failure orchestration response.
func NewFailureResponse(err string, steps []Step, metadata *Metadata, status *CompletionStatus) Response {
	return Response{
		Type:             ResponseFailure,
		Error:            err,
		Steps:            steps,
		Metadata:         metadata,
		CompletionStatus: status,
	}
}

// NewTimeoutResponse creates a timeout orchestration response.
func NewTimeoutResponse(partialResult string, steps []Step, metadata *Metadata, status *CompletionStatus) Response {
	return Response{
		Type:             ResponseTimeout,
		PartialResult:    partialResult,
		Steps:            steps,
		Metadata:         metadata,
		CompletionStatus: status,
	}
}
