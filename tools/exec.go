// Exec Tool - run shell commands
package tools

import (
	"bytes"
	"context"
	"os/exec"
	"time"
)

type ExecTool struct{}

func (t *ExecTool) Name() string {
	return "exec"
}

func (t *ExecTool) Description() string {
	return "Execute shell commands with timeout control and error handling."
}

func (t *ExecTool) Parameters() map[string]interface{} {
	return map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"command": map[string]interface{}{
				"type":        "string",
				"description": "Command to execute",
			},
			"timeout": map[string]interface{}{
				"type":        "integer",
				"description": "Timeout in seconds (default 30, max 300)",
				"default":     30,
			},
			"workdir": map[string]interface{}{
				"type":        "string",
				"description": "Working directory (default: current)",
			},
		},
		"required": []string{"command"},
	}
}

func (t *ExecTool) Execute(args map[string]interface{}) (interface{}, error) {
	command := GetString(args, "command")
	timeout := GetInt(args, "timeout")
	workdir := GetString(args, "workdir")

	if command == "" {
		return nil, &ExecError{Message: "command is required"}
	}

	if timeout <= 0 {
		timeout = 30
	}
	if timeout > 300 {
		return nil, &ExecError{Message: "timeout cannot exceed 300 seconds"}
	}

	ctx, cancel := context.WithTimeout(context.Background(), time.Duration(timeout)*time.Second)
	defer cancel()

	// Use shell parsing to keep quotes/pipes
	cmd := exec.CommandContext(ctx, "/bin/sh", "-c", command)

	// Set working directory
	if workdir != "" {
		cmd.Dir = workdir
	}

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	err := cmd.Run()

	result := ExecResult{
		Command:  command,
		Timeout:  timeout,
		Workdir:  workdir,
		Success:  err == nil,
		ExitCode: -1,
	}
	if cmd.ProcessState != nil {
		result.ExitCode = cmd.ProcessState.ExitCode()
	}

	result.Stdout = Truncate(stdout.String(), 10000)
	result.Stderr = Truncate(stderr.String(), 2000)

	if ctx.Err() == context.DeadlineExceeded {
		return nil, &ExecError{
			Message:  "command timed out",
			Metadata: map[string]interface{}{"command": command, "timeout": timeout},
		}
	}

	if err != nil {
		result.Error = err.Error()
	}

	return result, nil
}

type ExecResult struct {
	Command  string `json:"command"`
	Timeout  int    `json:"timeout"`
	Workdir  string `json:"workdir,omitempty"`
	Success  bool   `json:"success"`
	ExitCode int    `json:"exit_code"`
	Stdout   string `json:"stdout"`
	Stderr   string `json:"stderr"`
	Error    string `json:"error,omitempty"`
}

type ExecError struct {
	Message  string
	Metadata map[string]interface{}
}

func (e *ExecError) Error() string {
	return e.Message
}
