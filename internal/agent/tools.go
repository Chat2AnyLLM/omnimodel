package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"os/exec"
	"runtime"
	"strings"
	"time"
)

const defaultCommandTimeout = 30 * time.Second

func RegisterDefaultTools(registry *Registry) {
	registry.Register(&Tool{
		Name:        "run_command",
		Description: "Run a local shell command and return its combined output.",
		InputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"command": map[string]any{"type": "string", "description": "The command to execute."},
			},
			"required": []string{"command"},
		},
		Fn: runCommandTool,
	})
}

func runCommandTool(ctx context.Context, input json.RawMessage) (string, error) {
	var payload struct {
		Command string `json:"command"`
	}
	if err := json.Unmarshal(input, &payload); err != nil {
		return "", fmt.Errorf("parse command input: %w", err)
	}
	command := strings.TrimSpace(payload.Command)
	if command == "" {
		return "", fmt.Errorf("command is required")
	}

	cmdCtx, cancel := context.WithTimeout(ctx, defaultCommandTimeout)
	defer cancel()

	var cmd *exec.Cmd
	switch runtime.GOOS {
	case "windows":
		cmd = exec.CommandContext(cmdCtx, "powershell", "-NoProfile", "-NonInteractive", "-Command", command)
	default:
		cmd = exec.CommandContext(cmdCtx, "sh", "-lc", command)
	}

	output, err := cmd.CombinedOutput()
	text := strings.TrimSpace(string(output))
	if err != nil {
		if text == "" {
			return "", fmt.Errorf("command failed: %w", err)
		}
		return text, fmt.Errorf("command failed: %w", err)
	}
	if text == "" {
		return "(no output)", nil
	}
	return text, nil
}
