package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
)

type writeTool struct{}

func Write() Tool { return &writeTool{} }

func (t *writeTool) Name() string { return "write" }

func (t *writeTool) Description() string {
	return "Write content to a file, creating or overwriting it. Creates parent directories as needed."
}

func (t *writeTool) InputSchema() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"file_path": map[string]any{"type": "string", "description": "Absolute or relative path to the file."},
			"content":   map[string]any{"type": "string", "description": "The full content to write."},
		},
		"required": []string{"file_path", "content"},
	}
}

func (t *writeTool) Execute(ctx context.Context, call Context, input json.RawMessage) Result {
	var p struct {
		FilePath string `json:"file_path"`
		Content  string `json:"content"`
	}
	if err := json.Unmarshal(input, &p); err != nil {
		return Result{Output: "error: " + err.Error(), IsError: true}
	}
	if p.FilePath == "" {
		return Result{Output: "error: file_path is required", IsError: true}
	}

	if err := os.MkdirAll(filepath.Dir(p.FilePath), 0o755); err != nil {
		return Result{Output: "error: " + err.Error(), IsError: true}
	}
	if err := os.WriteFile(p.FilePath, []byte(p.Content), 0o644); err != nil {
		return Result{Output: "error: " + err.Error(), IsError: true}
	}
	return Result{Output: fmt.Sprintf("wrote %d bytes to %s", len(p.Content), p.FilePath)}
}
