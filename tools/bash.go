// Bash Tool - Structured command execution with policy enforcement.
//
// Information Hiding:
// - Policy validation logic hidden
// - Argument parsing hidden
// - Security checks abstracted

package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"time"
)

// BashPolicy defines security constraints for command execution.
type BashPolicy struct {
	AllowedCommands          []string
	AllowedSubcommands       []string
	AllowedResources         []string
	ResourceCheckSubcommands []string
	AllowedFlags             []string
	FlagsWithValues          []string
	AllowedEnv               []string
	ArgPattern               *regexp.Regexp
	AllowedCwd               []string
}

// BashTool executes allowlisted commands with structured arguments.
type BashTool struct {
	BaseTool
	timeoutSecs uint64
	policy      BashPolicy
}

// NewBashTool creates a new bash tool with the given timeout.
func NewBashTool(timeoutSecs uint64) *BashTool {
	return &BashTool{
		timeoutSecs: timeoutSecs,
	}
}

// WithPolicy sets the execution policy.
func (t *BashTool) WithPolicy(policy BashPolicy) *BashTool {
	t.policy = policy
	return t
}

// WithAllowedCommands sets allowed commands.
func (t *BashTool) WithAllowedCommands(commands []string) *BashTool {
	t.policy.AllowedCommands = commands
	return t
}

// WithAllowedFlags sets allowed flags.
func (t *BashTool) WithAllowedFlags(flags []string) *BashTool {
	t.policy.AllowedFlags = flags
	return t
}

// WithAllowedSubcommands sets allowed subcommands.
func (t *BashTool) WithAllowedSubcommands(subcommands []string) *BashTool {
	t.policy.AllowedSubcommands = subcommands
	return t
}

// WithAllowedResources sets allowed resource types.
func (t *BashTool) WithAllowedResources(resources []string) *BashTool {
	t.policy.AllowedResources = resources
	return t
}

// WithFlagsWithValues sets flags that take values.
func (t *BashTool) WithFlagsWithValues(flags []string) *BashTool {
	t.policy.FlagsWithValues = flags
	return t
}

// WithAllowedEnv sets allowed environment variables.
func (t *BashTool) WithAllowedEnv(keys []string) *BashTool {
	t.policy.AllowedEnv = keys
	return t
}

// WithArgPattern sets the argument validation pattern.
func (t *BashTool) WithArgPattern(pattern *regexp.Regexp) *BashTool {
	t.policy.ArgPattern = pattern
	return t
}

// WithAllowedCwd sets allowed working directories.
func (t *BashTool) WithAllowedCwd(paths []string) *BashTool {
	t.policy.AllowedCwd = paths
	return t
}

// Metadata returns the tool metadata.
func (t *BashTool) Metadata() ToolMetadata {
	return ToolMetadata{
		Name:        "execute_bash",
		Description: "Execute an allowlisted command with structured arguments and optional environment",
		Parameters: []ToolParameter{
			{Name: "command", ParamType: "string", Description: "The command to execute", Required: true},
			{Name: "argv", ParamType: "array", Description: "Command arguments", Required: false, Items: map[string]interface{}{"type": "string"}},
			{Name: "env", ParamType: "object", Description: "Environment variables", Required: false},
			{Name: "cwd", ParamType: "string", Description: "Working directory", Required: false},
			{Name: "stdout_path", ParamType: "string", Description: "Path to write stdout", Required: false},
			{Name: "stderr_path", ParamType: "string", Description: "Path to write stderr", Required: false},
		},
	}
}

type bashArgs struct {
	Command    string            `json:"command"`
	Argv       []string          `json:"argv"`
	Env        map[string]string `json:"env"`
	Cwd        string            `json:"cwd"`
	StdoutPath string            `json:"stdout_path"`
	StderrPath string            `json:"stderr_path"`
}

// Validate validates the tool arguments.
func (t *BashTool) Validate(args json.RawMessage) error {
	var a bashArgs
	if err := json.Unmarshal(args, &a); err != nil {
		return fmt.Errorf("invalid arguments: %w", err)
	}
	if strings.TrimSpace(a.Command) == "" {
		return fmt.Errorf("command cannot be empty")
	}
	return nil
}

// Execute runs the bash command.
func (t *BashTool) Execute(ctx context.Context, args json.RawMessage) (ToolResult, error) {
	var a bashArgs
	if err := json.Unmarshal(args, &a); err != nil {
		return FailureResult(fmt.Errorf("invalid arguments: %w", err)), nil
	}

	if strings.TrimSpace(a.Command) == "" {
		return FailureResultf("command cannot be empty"), nil
	}

	// Validate command
	if !t.isCommandAllowed(a.Command) {
		return FailureResultf("command '%s' is not allowed", a.Command), nil
	}

	// Validate subcommand if policy requires it
	if len(t.policy.AllowedSubcommands) > 0 {
		subcommand := t.extractSubcommand(a.Argv)
		if subcommand == "" {
			return FailureResultf("subcommand required but not provided"), nil
		}
		if !t.isSubcommandAllowed(subcommand) {
			return FailureResultf("subcommand '%s' is not allowed", subcommand), nil
		}
	}

	// Validate resources if policy requires it
	if len(t.policy.AllowedResources) > 0 {
		subcommand := t.extractSubcommand(a.Argv)
		if t.shouldCheckResource(subcommand) {
			resource := t.extractResource(a.Argv)
			if resource == "" {
				return FailureResultf("resource type required but not provided"), nil
			}
			if !t.isResourceAllowed(resource) {
				return FailureResultf("resource type '%s' is not allowed", resource), nil
			}
		}
	}

	// Validate flags and arguments
	endOfFlags := false
	for _, arg := range a.Argv {
		if arg == "" {
			return FailureResultf("arguments cannot be empty"), nil
		}

		if arg == "--" {
			endOfFlags = true
			continue
		}

		if !endOfFlags && strings.HasPrefix(arg, "-") {
			flag := normalizeFlag(arg)
			if !t.isFlagAllowed(flag) {
				return FailureResultf("flag '%s' is not allowed", flag), nil
			}
			continue
		}

		if !t.isArgAllowed(arg) {
			return FailureResultf("argument '%s' is not allowed", arg), nil
		}
	}

	// Validate environment variables
	for key := range a.Env {
		if !t.isEnvAllowed(key) {
			return FailureResultf("environment variable '%s' is not allowed", key), nil
		}
	}

	// Validate working directory
	if a.Cwd != "" {
		info, err := os.Stat(a.Cwd)
		if err != nil {
			return FailureResultf("working directory does not exist: %s", a.Cwd), nil
		}
		if !info.IsDir() {
			return FailureResultf("working directory is not a directory: %s", a.Cwd), nil
		}
		if !t.isCwdAllowed(a.Cwd) {
			return FailureResultf("working directory '%s' is not allowed", a.Cwd), nil
		}
	}

	// Validate output paths
	if a.StdoutPath != "" {
		if err := t.validateOutputPath(a.StdoutPath); err != nil {
			return FailureResult(err), nil
		}
	}
	if a.StderrPath != "" {
		if err := t.validateOutputPath(a.StderrPath); err != nil {
			return FailureResult(err), nil
		}
	}

	// Execute command
	timeout := time.Duration(t.timeoutSecs) * time.Second
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	cmd := exec.CommandContext(ctx, a.Command, a.Argv...)

	// Set environment
	if len(a.Env) > 0 {
		cmd.Env = os.Environ()
		for k, v := range a.Env {
			cmd.Env = append(cmd.Env, k+"="+v)
		}
	}

	// Set working directory
	if a.Cwd != "" {
		cmd.Dir = a.Cwd
	}

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

	// Write to output files if specified
	result := string(output)
	if a.StdoutPath != "" {
		if err := os.WriteFile(a.StdoutPath, output, 0644); err != nil {
			return FailureResultf("failed to write stdout to %s: %v", a.StdoutPath, err), nil
		}
		result = fmt.Sprintf("stdout saved to %s", a.StdoutPath)
	}

	return SuccessResult(result), nil
}

// Policy validation helpers

func (t *BashTool) isCommandAllowed(command string) bool {
	if len(t.policy.AllowedCommands) == 0 {
		return true
	}
	for _, allowed := range t.policy.AllowedCommands {
		if allowed == command {
			return true
		}
	}
	return false
}

func (t *BashTool) isFlagAllowed(flag string) bool {
	if len(t.policy.AllowedFlags) == 0 {
		return true
	}
	for _, allowed := range t.policy.AllowedFlags {
		if allowed == flag {
			return true
		}
	}
	return false
}

func (t *BashTool) isSubcommandAllowed(subcommand string) bool {
	if len(t.policy.AllowedSubcommands) == 0 {
		return true
	}
	for _, allowed := range t.policy.AllowedSubcommands {
		if allowed == subcommand {
			return true
		}
	}
	return false
}

func (t *BashTool) isResourceAllowed(resource string) bool {
	if len(t.policy.AllowedResources) == 0 {
		return true
	}
	for _, allowed := range t.policy.AllowedResources {
		if allowed == resource {
			return true
		}
	}
	return false
}

func (t *BashTool) shouldCheckResource(subcommand string) bool {
	if len(t.policy.ResourceCheckSubcommands) == 0 {
		return true
	}
	for _, s := range t.policy.ResourceCheckSubcommands {
		if s == subcommand {
			return true
		}
	}
	return false
}

func (t *BashTool) isFlagWithValue(flag string) bool {
	for _, f := range t.policy.FlagsWithValues {
		if f == flag {
			return true
		}
	}
	return false
}

func (t *BashTool) isEnvAllowed(key string) bool {
	if len(t.policy.AllowedEnv) == 0 {
		return true
	}
	for _, allowed := range t.policy.AllowedEnv {
		if allowed == key {
			return true
		}
	}
	return false
}

func (t *BashTool) isArgAllowed(arg string) bool {
	if t.policy.ArgPattern == nil {
		return true
	}
	return t.policy.ArgPattern.MatchString(arg)
}

func (t *BashTool) isCwdAllowed(cwd string) bool {
	if len(t.policy.AllowedCwd) == 0 {
		return true
	}
	absPath, err := filepath.Abs(cwd)
	if err != nil {
		return false
	}
	for _, allowed := range t.policy.AllowedCwd {
		allowedAbs, err := filepath.Abs(allowed)
		if err != nil {
			continue
		}
		if strings.HasPrefix(absPath, allowedAbs) {
			return true
		}
	}
	return false
}

func (t *BashTool) validateOutputPath(path string) error {
	dir := filepath.Dir(path)
	if _, err := os.Stat(dir); err != nil {
		return fmt.Errorf("output path directory does not exist: %s", dir)
	}
	if !t.isPathAllowed(path) {
		return fmt.Errorf("output path '%s' is not allowed", path)
	}
	return nil
}

func (t *BashTool) isPathAllowed(path string) bool {
	if !filepath.IsAbs(path) {
		return true
	}
	if strings.HasPrefix(path, "/tmp") {
		return true
	}
	if len(t.policy.AllowedCwd) == 0 {
		return true
	}
	for _, allowed := range t.policy.AllowedCwd {
		allowedAbs, err := filepath.Abs(allowed)
		if err != nil {
			continue
		}
		if strings.HasPrefix(path, allowedAbs) {
			return true
		}
	}
	return false
}

func normalizeFlag(arg string) string {
	if idx := strings.Index(arg, "="); idx != -1 {
		return arg[:idx]
	}
	return arg
}

func (t *BashTool) extractSubcommand(args []string) string {
	endOfFlags := false
	skipNext := false

	for _, arg := range args {
		if skipNext {
			skipNext = false
			continue
		}
		if arg == "--" {
			endOfFlags = true
			continue
		}
		if !endOfFlags && strings.HasPrefix(arg, "-") {
			flag := normalizeFlag(arg)
			if t.isFlagWithValue(flag) && !strings.Contains(arg, "=") {
				skipNext = true
			}
			continue
		}
		return arg
	}
	return ""
}

func (t *BashTool) extractResource(args []string) string {
	endOfFlags := false
	skipNext := false
	subcommandSeen := false

	for _, arg := range args {
		if skipNext {
			skipNext = false
			continue
		}
		if arg == "--" {
			endOfFlags = true
			continue
		}
		if !endOfFlags && strings.HasPrefix(arg, "-") {
			flag := normalizeFlag(arg)
			if t.isFlagWithValue(flag) && !strings.Contains(arg, "=") {
				skipNext = true
			}
			continue
		}

		if !subcommandSeen {
			subcommandSeen = true
			continue
		}

		return arg
	}
	return ""
}
