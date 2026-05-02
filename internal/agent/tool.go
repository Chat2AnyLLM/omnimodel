package agent

import (
	"context"
	"encoding/json"
	"omnillm/internal/cif"
	"sync"
)

// Tool defines a callable tool that an agent can use.
type Tool struct {
	Name        string
	Description string
	InputSchema any // JSON Schema object
	Fn          func(ctx context.Context, input json.RawMessage) (string, error)
}

// Registry holds registered tools and provides lookup and conversion.
type Registry struct {
	mu    sync.RWMutex
	tools map[string]*Tool
}

// NewRegistry creates a new empty tool registry.
func NewRegistry() *Registry {
	return &Registry{
		tools: make(map[string]*Tool),
	}
}

// Register adds a tool to the registry.
func (r *Registry) Register(t *Tool) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.tools[t.Name] = t
}

// Get returns a tool by name, or nil if not found.
func (r *Registry) Get(name string) *Tool {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.tools[name]
}

// List returns all registered tools.
func (r *Registry) List() []*Tool {
	r.mu.RLock()
	defer r.mu.RUnlock()
	result := make([]*Tool, 0, len(r.tools))
	for _, t := range r.tools {
		result = append(result, t)
	}
	return result
}

// ToCIFTools converts the registry's tools to the CIF tool format
// used by CanonicalRequest.
func (r *Registry) ToCIFTools() []cif.CIFTool {
	r.mu.RLock()
	defer r.mu.RUnlock()
	result := make([]cif.CIFTool, 0, len(r.tools))
	for _, t := range r.tools {
		var desc *string
		if t.Description != "" {
			d := t.Description
			desc = &d
		}

		var schema map[string]interface{}
		if t.InputSchema != nil {
			// Convert InputSchema to map[string]interface{}
			data, err := json.Marshal(t.InputSchema)
			if err == nil {
				_ = json.Unmarshal(data, &schema)
			}
		}
		if schema == nil {
			schema = map[string]interface{}{
				"type":       "object",
				"properties": map[string]interface{}{},
			}
		}

		result = append(result, cif.CIFTool{
			Name:             t.Name,
			Description:      desc,
			ParametersSchema: schema,
		})
	}
	return result
}

// ToolCallResult holds the result of executing a single tool call.
type ToolCallResult struct {
	ToolCallID string
	ToolName   string
	Content    string
	IsError    bool
}

// ExecuteToolCalls runs multiple tool calls concurrently and returns results.
// Errors in individual tool calls become error result messages, not fatal errors.
func (r *Registry) ExecuteToolCalls(ctx context.Context, calls []cif.CIFToolCallPart) []ToolCallResult {
	results := make([]ToolCallResult, len(calls))
	var wg sync.WaitGroup

	for i, call := range calls {
		wg.Add(1)
		go func(idx int, tc cif.CIFToolCallPart) {
			defer wg.Done()

			tool := r.Get(tc.ToolName)
			if tool == nil {
				results[idx] = ToolCallResult{
					ToolCallID: tc.ToolCallID,
					ToolName:   tc.ToolName,
					Content:    "error: unknown tool " + tc.ToolName,
					IsError:    true,
				}
				return
			}

			// Marshal tool arguments to JSON for the tool function
			inputJSON, err := json.Marshal(tc.ToolArguments)
			if err != nil {
				results[idx] = ToolCallResult{
					ToolCallID: tc.ToolCallID,
					ToolName:   tc.ToolName,
					Content:    "error: failed to marshal tool arguments: " + err.Error(),
					IsError:    true,
				}
				return
			}

			output, err := tool.Fn(ctx, inputJSON)
			if err != nil {
				results[idx] = ToolCallResult{
					ToolCallID: tc.ToolCallID,
					ToolName:   tc.ToolName,
					Content:    "error: " + err.Error(),
					IsError:    true,
				}
				return
			}

			results[idx] = ToolCallResult{
				ToolCallID: tc.ToolCallID,
				ToolName:   tc.ToolName,
				Content:    output,
				IsError:    false,
			}
		}(i, call)
	}

	wg.Wait()
	return results
}
