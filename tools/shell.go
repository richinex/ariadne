// Shell Command Executor Tool.
//
// Information Hiding:
// - Shell execution details hidden
// - Command validation hidden
// - Output parsing abstracted

package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"os/exec"
	"strings"
	"time"
)

// ShellTool executes shell commands via sh -c.
type ShellTool struct {
	BaseTool
	timeoutSecs     uint64
	allowedCommands []string
}

// NewShellTool creates a new shell tool with the given timeout.
func NewShellTool(timeoutSecs uint64) *ShellTool {
	return &ShellTool{
		timeoutSecs: timeoutSecs,
	}
}

// WithAllowedCommands sets the allowlist for commands.
func (t *ShellTool) WithAllowedCommands(commands []string) *ShellTool {
	t.allowedCommands = commands
	return t
}

// Metadata returns the tool metadata.
func (t *ShellTool) Metadata() ToolMetadata {
	return ToolMetadata{
		Name:        "execute_shell",
		Description: "Execute a shell command and return its output",
		Parameters: []ToolParameter{
			{
				Name:        "command",
				ParamType:   "string",
				Description: "The shell command to execute",
				Required:    true,
			},
		},
	}
}

type shellArgs struct {
	Command string `json:"command"`
}

// Validate validates the tool arguments.
func (t *ShellTool) Validate(args json.RawMessage) error {
	var a shellArgs
	if err := json.Unmarshal(args, &a); err != nil {
		return fmt.Errorf("invalid arguments: %w", err)
	}
	if a.Command == "" {
		return fmt.Errorf("command cannot be empty")
	}
	return nil
}

// Execute runs the shell command.
func (t *ShellTool) Execute(ctx context.Context, args json.RawMessage) (ToolResult, error) {
	var a shellArgs
	if err := json.Unmarshal(args, &a); err != nil {
		return FailureResult(fmt.Errorf("invalid arguments: %w", err)), nil
	}

	if a.Command == "" {
		return FailureResultf("command cannot be empty"), nil
	}

	// Check command allowlist
	if !t.isCommandAllowed(a.Command) {
		return FailureResultf("command '%s' is not in the allowed list", a.Command), nil
	}

	// Create timeout context
	timeout := time.Duration(t.timeoutSecs) * time.Second
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	// Execute via sh -c
	cmd := exec.CommandContext(ctx, "sh", "-c", a.Command)
	output, err := cmd.CombinedOutput()

	if ctx.Err() == context.DeadlineExceeded {
		return FailureResultf("command timed out after %d seconds", t.timeoutSecs), nil
	}

	if err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			return FailureResultf("command failed with exit code %d\noutput: %s",
				exitErr.ExitCode(), string(output)), nil
		}
		return FailureResult(fmt.Errorf("failed to execute command: %w", err)), nil
	}

	return SuccessResult(string(output)), nil
}

// isCommandAllowed checks if the command is in the allowlist.
func (t *ShellTool) isCommandAllowed(command string) bool {
	if len(t.allowedCommands) == 0 {
		return true
	}

	// Extract base command (first word)
	baseCmd := strings.Fields(command)
	if len(baseCmd) == 0 {
		return false
	}

	for _, allowed := range t.allowedCommands {
		if allowed == baseCmd[0] {
			return true
		}
	}
	return false
}
