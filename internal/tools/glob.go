package tools

import (
	"context"
	"encoding/json"
	"os"
)

type globTool struct{}

func Glob() Tool { return &globTool{} }

func (t *globTool) Name() string { return "glob" }

func (t *globTool) Description() string {
	return "Find files matching a glob pattern (e.g. **/*.go). Returns matching paths sorted by modification time."
}

func (t *globTool) InputSchema() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"pattern": map[string]any{"type": "string", "description": "Glob pattern, e.g. \"src/**/*.ts\" or \"*.go\"."},
			"path":    map[string]any{"type": "string", "description": "Base directory to search in (default: cwd)."},
		},
		"required": []string{"pattern"},
	}
}

func (t *globTool) Execute(ctx context.Context, call Context, input json.RawMessage) Result {
	var p struct {
		Pattern string `json:"pattern"`
		Path    string `json:"path"`
	}
	if err := json.Unmarshal(input, &p); err != nil {
		return Result{Output: "error: " + err.Error(), IsError: true}
	}
	if p.Pattern == "" {
		return Result{Output: "error: pattern is required", IsError: true}
	}

	base := p.Path
	if base == "" {
		base, _ = os.Getwd()
	}

	matches, err := walkGlob(base, p.Pattern)
	if err != nil {
		return Result{Output: "error: " + err.Error(), IsError: true}
	}
	if len(matches) == 0 {
		return Result{Output: "(no matches)"}
	}
	return Result{Output: joinLines(matches)}
}
