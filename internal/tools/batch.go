package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"sync"
)

// batch — execute multiple independent tool calls concurrently in one step.

type batchTool struct{}

func Batch() Tool { return &batchTool{} }

func (t *batchTool) Name() string { return "batch" }
func (t *batchTool) Description() string {
	return "Run multiple independent tool calls concurrently and return all results. " +
		"Use this when several tools can run in parallel — file reads, web fetches, etc. " +
		"Each invocation specifies a tool name and its JSON input."
}
func (t *batchTool) InputSchema() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"invocations": map[string]any{
				"type":        "array",
				"description": "List of tool calls to run in parallel.",
				"items": map[string]any{
					"type": "object",
					"properties": map[string]any{
						"tool_name":   map[string]any{"type": "string", "description": "Name of the tool to call."},
						"description": map[string]any{"type": "string", "description": "Optional label for this call in the output."},
						"input":       map[string]any{"type": "object", "description": "Input arguments for the tool (passed as-is)."},
					},
					"required": []string{"tool_name", "input"},
				},
			},
		},
		"required": []string{"invocations"},
	}
}

type batchInvocation struct {
	ToolName    string          `json:"tool_name"`
	Description string          `json:"description"`
	Input       json.RawMessage `json:"input"`
}

func (t *batchTool) Execute(ctx context.Context, call Context, input json.RawMessage) Result {
	var p struct {
		Invocations []batchInvocation `json:"invocations"`
	}
	if err := json.Unmarshal(input, &p); err != nil {
		return Result{Output: "error: " + err.Error(), IsError: true}
	}
	if len(p.Invocations) == 0 {
		return Result{Output: "error: invocations list is empty", IsError: true}
	}
	if call.Registry == nil {
		return Result{Output: "error: registry not available for batch execution", IsError: true}
	}

	type batchResult struct {
		idx    int
		label  string
		output string
		isErr  bool
	}

	results := make([]batchResult, len(p.Invocations))
	var wg sync.WaitGroup

	for i, inv := range p.Invocations {
		wg.Add(1)
		go func(idx int, inv batchInvocation) {
			defer wg.Done()
			label := inv.Description
			if label == "" {
				label = fmt.Sprintf("%s[%d]", inv.ToolName, idx)
			}
			tool := call.Registry.Get(inv.ToolName)
			if tool == nil {
				results[idx] = batchResult{idx: idx, label: label, output: fmt.Sprintf("error: unknown tool %q", inv.ToolName), isErr: true}
				return
			}
			inputBytes := inv.Input
			if inputBytes == nil {
				inputBytes = json.RawMessage("{}")
			}
			res := tool.Execute(ctx, call, inputBytes)
			results[idx] = batchResult{idx: idx, label: label, output: res.Output, isErr: res.IsError}
		}(i, inv)
	}
	wg.Wait()

	var sb strings.Builder
	hasErr := false
	for _, r := range results {
		prefix := "✓"
		if r.isErr {
			prefix = "✗"
			hasErr = true
		}
		fmt.Fprintf(&sb, "─── %s %s ───\n%s\n\n", prefix, r.label, r.output)
	}
	return Result{Output: strings.TrimRight(sb.String(), "\n"), IsError: hasErr}
}
