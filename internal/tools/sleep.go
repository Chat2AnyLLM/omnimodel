package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"time"
)

type sleepTool struct{}

func Sleep() Tool { return &sleepTool{} }

func (t *sleepTool) Name() string { return "sleep" }

func (t *sleepTool) Description() string {
	return "Wait for a specified number of seconds before continuing. Useful when a process needs time to settle before the next tool call."
}

func (t *sleepTool) InputSchema() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"seconds": map[string]any{"type": "integer", "description": "Number of seconds to wait (1-3600)."},
		},
		"required": []string{"seconds"},
	}
}

func (t *sleepTool) Execute(ctx context.Context, call Context, input json.RawMessage) Result {
	var p struct {
		Seconds int `json:"seconds"`
	}
	if err := json.Unmarshal(input, &p); err != nil {
		return Result{Output: "error: " + err.Error(), IsError: true}
	}
	if p.Seconds < 1 || p.Seconds > 3600 {
		return Result{Output: "error: seconds must be between 1 and 3600", IsError: true}
	}

	select {
	case <-ctx.Done():
		return Result{Output: "error: sleep interrupted: " + ctx.Err().Error(), IsError: true}
	case <-time.After(time.Duration(p.Seconds) * time.Second):
		return Result{Output: fmt.Sprintf("Slept for %d second(s)", p.Seconds)}
	}
}
