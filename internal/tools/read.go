package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"
)

type readTool struct{}

func Read() Tool { return &readTool{} }

func (t *readTool) Name() string { return "read" }

func (t *readTool) Description() string {
	return "Read a file from the local filesystem. Use offset/limit for line-range slicing (line numbers are prefixed)."
}

func (t *readTool) InputSchema() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"file_path": map[string]any{"type": "string", "description": "Absolute or relative path to the file."},
			"offset":    map[string]any{"type": "integer", "description": "Line number to start reading from (1-based)."},
			"limit":     map[string]any{"type": "integer", "description": "Maximum number of lines to read."},
		},
		"required": []string{"file_path"},
	}
}

func (t *readTool) Execute(ctx context.Context, call Context, input json.RawMessage) Result {
	var p struct {
		FilePath string `json:"file_path"`
		Offset   int    `json:"offset"`
		Limit    int    `json:"limit"`
	}
	if err := json.Unmarshal(input, &p); err != nil {
		return Result{Output: "error: " + err.Error(), IsError: true}
	}
	if p.FilePath == "" {
		return Result{Output: "error: file_path is required", IsError: true}
	}

	data, err := os.ReadFile(p.FilePath)
	if err != nil {
		return Result{Output: "error: " + err.Error(), IsError: true}
	}

	if p.Offset == 0 && p.Limit == 0 {
		return Result{Output: string(data)}
	}

	lines := strings.Split(string(data), "\n")
	start := 0
	if p.Offset > 0 {
		start = p.Offset - 1
	}
	if start >= len(lines) {
		return Result{Output: ""}
	}
	end := len(lines)
	if p.Limit > 0 && start+p.Limit < end {
		end = start + p.Limit
	}

	var buf strings.Builder
	for i, line := range lines[start:end] {
		fmt.Fprintf(&buf, "%d\t%s\n", start+i+1, line)
	}
	return Result{Output: buf.String()}
}
