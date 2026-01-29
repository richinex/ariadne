// Package agent provides the ReAct agent implementation.
//
// Contains all types used by agents for decisions, actions, and responses.
package agent

import (
	"encoding/json"

	"github.com/richinex/ariadne/model"
	"github.com/richinex/ariadne/llm"
)

// Decision represents a decision made by the agent's LLM.
type Decision struct {
	Thought     string  `json:"thought"`
	Action      *Action `json:"action,omitempty"`
	IsFinal     bool    `json:"is_final"`
	FinalAnswer *string `json:"final_answer,omitempty"`
}

// UnmarshalJSON implements custom unmarshaling that accepts either a string or
// JSON value for FinalAnswer.
// Anatomy of Go 1
func (d *Decision) UnmarshalJSON(data []byte) error {
	// Use an alias to avoid infinite recursion
	type DecisionAlias Decision
	aux := &struct {
		FinalAnswer json.RawMessage `json:"final_answer,omitempty"`
		*DecisionAlias
	}{
		DecisionAlias: (*DecisionAlias)(d),
	}

	if err := json.Unmarshal(data, aux); err != nil {
		return err
	}

	if len(aux.FinalAnswer) > 0 {
		// Try to unmarshal as string first
		var s string
		if err := json.Unmarshal(aux.FinalAnswer, &s); err == nil {
			d.FinalAnswer = &s
			return nil
		}

		// If not a string, convert the JSON to a pretty-printed string
		var v interface{}
		if err := json.Unmarshal(aux.FinalAnswer, &v); err == nil {
			pretty, err := json.MarshalIndent(v, "", "  ")
			if err == nil {
				s := string(pretty)
				d.FinalAnswer = &s
			}
		}
	}

	return nil
}

// Action represents an action to execute a tool.
type Action struct {
	Tool  string          `json:"tool"`
	Input json.RawMessage `json:"input"`
}

// Step is an alias for model.Step for agent reasoning steps.
type Step = model.Step

// ToolCall is an alias for model.ToolCall for tool call metadata.
type ToolCall = model.ToolCall

// Metadata contains metadata about agent execution.
type Metadata struct {
	ExecutionTimeMs uint64
	AgentName       *string
	ToolCalls       []ToolCall
	TokenUsage      *llm.TokenUsage
	LLMCalls        int // Number of LLM calls made by this agent
}

// ResponseType indicates the type of agent response.
type ResponseType int

const (
	ResponseSuccess ResponseType = iota
	ResponseFailure
	ResponseTimeout
)

// Response represents a response from an agent execution.
type Response struct {
	Type          ResponseType
	Result        string // For Success
	Error         string // For Failure
	PartialResult string // For Timeout
	Steps         []Step
	Metadata      Metadata
}

// NewSuccessResponse creates a successful response.
func NewSuccessResponse(result string, steps []Step, toolCalls []ToolCall, executionTimeMs uint64, agentName string, tokenUsage *llm.TokenUsage, llmCalls int) Response {
	return Response{
		Type:   ResponseSuccess,
		Result: result,
		Steps:  steps,
		Metadata: Metadata{
			ExecutionTimeMs: executionTimeMs,
			AgentName:       &agentName,
			ToolCalls:       toolCalls,
			TokenUsage:      tokenUsage,
			LLMCalls:        llmCalls,
		},
	}
}

// NewFailureResponse creates a failure response.
func NewFailureResponse(err string, steps []Step, executionTimeMs uint64) Response {
	return Response{
		Type:  ResponseFailure,
		Error: err,
		Steps: steps,
		Metadata: Metadata{
			ExecutionTimeMs: executionTimeMs,
		},
	}
}

// NewTimeoutResponse creates a timeout response.
func NewTimeoutResponse(steps []Step, toolCalls []ToolCall, executionTimeMs uint64, tokenUsage *llm.TokenUsage, llmCalls int) Response {
	return Response{
		Type:          ResponseTimeout,
		PartialResult: "Max iterations reached",
		Steps:         steps,
		Metadata: Metadata{
			ExecutionTimeMs: executionTimeMs,
			ToolCalls:       toolCalls,
			TokenUsage:      tokenUsage,
			LLMCalls:        llmCalls,
		},
	}
}

// ResultText returns the result string (for success) or error (for failure).
func (r Response) ResultText() string {
	switch r.Type {
	case ResponseSuccess:
		return r.Result
	case ResponseFailure:
		return r.Error
	case ResponseTimeout:
		return r.PartialResult
	default:
		return ""
	}
}

// IsSuccess checks if the response was successful.
func (r Response) IsSuccess() bool {
	return r.Type == ResponseSuccess
}
