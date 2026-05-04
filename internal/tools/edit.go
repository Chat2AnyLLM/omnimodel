package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"
)

type editTool struct{}

func Edit() Tool { return &editTool{} }

func (t *editTool) Name() string { return "edit" }

func (t *editTool) Description() string {
	return "Perform an exact string replacement in a file. old_string must match exactly (including whitespace). Set replace_all to replace every occurrence."
}

func (t *editTool) InputSchema() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"file_path":   map[string]any{"type": "string", "description": "Absolute or relative path to the file."},
			"old_string":  map[string]any{"type": "string", "description": "The exact text to find and replace."},
			"new_string":  map[string]any{"type": "string", "description": "The replacement text."},
			"replace_all": map[string]any{"type": "boolean", "description": "Replace every occurrence (default false)."},
		},
		"required": []string{"file_path", "old_string", "new_string"},
	}
}

func (t *editTool) Execute(ctx context.Context, call Context, input json.RawMessage) Result {
	var p struct {
		FilePath   string `json:"file_path"`
		OldString  string `json:"old_string"`
		NewString  string `json:"new_string"`
		ReplaceAll bool   `json:"replace_all"`
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
	content := string(data)

	if !strings.Contains(content, p.OldString) {
		return Result{Output: fmt.Sprintf("error: old_string not found in %s", p.FilePath), IsError: true}
	}

	count := strings.Count(content, p.OldString)
	var updated string
	if p.ReplaceAll {
		updated = strings.ReplaceAll(content, p.OldString, p.NewString)
	} else {
		updated = strings.Replace(content, p.OldString, p.NewString, 1)
		count = 1
	}

	if err := os.WriteFile(p.FilePath, []byte(updated), 0o644); err != nil {
		return Result{Output: "error: " + err.Error(), IsError: true}
	}
	return Result{Output: fmt.Sprintf("replaced %d occurrence(s) in %s", count, p.FilePath)}
}
