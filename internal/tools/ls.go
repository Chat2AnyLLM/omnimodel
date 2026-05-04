package tools

import (
	"context"
	"encoding/json"
	"os"
)

type lsTool struct{}

func LS() Tool { return &lsTool{} }

func (t *lsTool) Name() string { return "ls" }

func (t *lsTool) Description() string {
	return "List files and directories at a given path."
}

func (t *lsTool) InputSchema() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"path": map[string]any{"type": "string", "description": "Directory path to list (default: cwd)."},
		},
	}
}

func (t *lsTool) Execute(ctx context.Context, call Context, input json.RawMessage) Result {
	var p struct {
		Path string `json:"path"`
	}
	if err := json.Unmarshal(input, &p); err != nil {
		return Result{Output: "error: " + err.Error(), IsError: true}
	}

	dir := p.Path
	if dir == "" {
		dir, _ = os.Getwd()
	}

	return listDirectory(dir)
}
