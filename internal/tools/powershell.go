package tools

import (
	"context"
	"encoding/json"
)

// powershell_tool — Windows PowerShell execution (separate from the bash tool).
// On non-Windows platforms it falls back to the bash tool's sh execution.

type powershellTool struct{}

func PowerShell() Tool { return &powershellTool{} }

func (t *powershellTool) Name() string { return "powershell" }
func (t *powershellTool) Description() string {
	return "Execute a PowerShell command and return its combined stdout+stderr output. " +
		"Use this for Windows-specific operations: registry access, WMI queries, " +
		"Windows services, environment variables, and PowerShell-idiomatic pipelines. " +
		"On non-Windows systems this falls back to bash."
}
func (t *powershellTool) InputSchema() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"command":         map[string]any{"type": "string", "description": "The PowerShell command or script block to execute."},
			"description":     map[string]any{"type": "string", "description": "Short description of what the command does."},
			"timeout_seconds": map[string]any{"type": "integer", "description": "Optional timeout in seconds (default 30)."},
		},
		"required": []string{"command"},
	}
}
func (t *powershellTool) Execute(ctx context.Context, call Context, input json.RawMessage) Result {
	var p struct {
		Command        string `json:"command"`
		TimeoutSeconds int    `json:"timeout_seconds"`
	}
	if err := json.Unmarshal(input, &p); err != nil {
		return Result{Output: "error: " + err.Error(), IsError: true}
	}
	if p.Command == "" {
		return Result{Output: "error: command is required", IsError: true}
	}
	// runShellCommand already routes through powershell on Windows and sh elsewhere.
	return runShellCommand(ctx, p.Command, p.TimeoutSeconds)
}
