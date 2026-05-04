package tools

import (
	"context"
	"encoding/json"
	"strings"
)

type bashTool struct{}

func Bash() Tool { return &bashTool{} }

func (t *bashTool) Name() string { return "bash" }

func (t *bashTool) Description() string {
	return "Execute a shell command and return its combined stdout+stderr output."
}

func (t *bashTool) InputSchema() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"command":           map[string]any{"type": "string", "description": "The shell command to execute."},
			"description":       map[string]any{"type": "string", "description": "Short description of what the command does."},
			"timeout_seconds":   map[string]any{"type": "integer", "description": "Optional timeout in seconds (default 30)."},
			"run_in_background": map[string]any{"type": "boolean", "description": "Run the command in the background."},
		},
		"required": []string{"command"},
	}
}

func (t *bashTool) Execute(ctx context.Context, call Context, input json.RawMessage) Result {
	var p struct {
		Command         string `json:"command"`
		TimeoutSeconds  int    `json:"timeout_seconds"`
		RunInBackground bool   `json:"run_in_background"`
	}
	if err := json.Unmarshal(input, &p); err != nil {
		return Result{Output: "error: " + err.Error(), IsError: true}
	}
	p.Command = strings.TrimSpace(p.Command)
	if p.Command == "" {
		return Result{Output: "error: command is required", IsError: true}
	}

	return runShellCommand(ctx, p.Command, p.TimeoutSeconds)
}
