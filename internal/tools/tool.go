// Package tools provides the agent tool system: base interfaces, execution
// context, registry, and built-in tool implementations.
//
// Design draws from claude-code3 (buildTool factory, permission checks),
// opencode (Tool.define + Context pattern), and pi-mono (AgentTool interface,
// pluggable operations).
package tools

import (
	"context"
	"encoding/json"
	"sync"

	"omnillm/internal/cif"
)

// ─── Result ──────────────────────────────────────────────────────────────────

// Result is the output of a tool execution.
type Result struct {
	Output   string
	Title    string
	Metadata map[string]any
	IsError  bool
}

// ─── Context ─────────────────────────────────────────────────────────────────

// PermissionRequest describes a tool execution that may require approval.
type PermissionRequest struct {
	SessionID string
	ToolName  string
	Arguments map[string]any
}

// PermissionChecker decides whether a tool call may run.
type PermissionChecker func(ctx context.Context, req PermissionRequest) (bool, error)

// Context carries the execution environment for a tool call.
type Context struct {
	SessionID string
	Abort     <-chan struct{}
	Checker   PermissionChecker
}

// ─── Tool ────────────────────────────────────────────────────────────────────

// Tool is a callable tool that an agent can invoke.
type Tool interface {
	Name() string
	Description() string
	InputSchema() map[string]any
	Execute(ctx context.Context, call Context, input json.RawMessage) Result
}

// ─── Registry ────────────────────────────────────────────────────────────────

// Registry holds registered tools and provides lookup, conversion, and batch
// execution.
type Registry struct {
	mu        sync.RWMutex
	tools     map[string]Tool
	checker   PermissionChecker
}

// NewRegistry creates an empty tool registry.
func NewRegistry() *Registry {
	return &Registry{tools: make(map[string]Tool)}
}

// Register adds a tool. If a tool with the same name already exists it is replaced.
func (r *Registry) Register(t Tool) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.tools[t.Name()] = t
}

// Get returns the tool with the given name, or nil.
func (r *Registry) Get(name string) Tool {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.tools[name]
}

// SetPermissionChecker configures an approval hook for tool execution.
func (r *Registry) SetPermissionChecker(checker PermissionChecker) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.checker = checker
}

func (r *Registry) permissionChecker() PermissionChecker {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.checker
}

// List returns all registered tools.
func (r *Registry) List() []Tool {
	r.mu.RLock()
	defer r.mu.RUnlock()
	out := make([]Tool, 0, len(r.tools))
	for _, t := range r.tools {
		out = append(out, t)
	}
	return out
}

// ToCIFTools converts the registry's tools to the CIF tool format.
func (r *Registry) ToCIFTools() []cif.CIFTool {
	r.mu.RLock()
	defer r.mu.RUnlock()
	out := make([]cif.CIFTool, 0, len(r.tools))
	for _, t := range r.tools {
		desc := t.Description()
		var descPtr *string
		if desc != "" {
			descPtr = &desc
		}
		schema := t.InputSchema()
		if schema == nil {
			schema = map[string]any{"type": "object", "properties": map[string]any{}}
		}
		out = append(out, cif.CIFTool{
			Name:             t.Name(),
			Description:      descPtr,
			ParametersSchema: schema,
		})
	}
	return out
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
func (r *Registry) ExecuteToolCalls(ctx context.Context, sessionID string, calls []cif.CIFToolCallPart) []ToolCallResult {
	results := make([]ToolCallResult, len(calls))
	var wg sync.WaitGroup
	checker := r.permissionChecker()

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

			// Permission check.
			if checker != nil {
				approved, err := checker(ctx, PermissionRequest{
					SessionID: sessionID,
					ToolName:  tc.ToolName,
					Arguments: tc.ToolArguments,
				})
				if err != nil {
					results[idx] = ToolCallResult{
						ToolCallID: tc.ToolCallID,
						ToolName:   tc.ToolName,
						Content:    "error: permission check failed: " + err.Error(),
						IsError:    true,
					}
					return
				}
				if !approved {
					results[idx] = ToolCallResult{
						ToolCallID: tc.ToolCallID,
						ToolName:   tc.ToolName,
						Content:    "error: tool execution denied by user",
						IsError:    true,
					}
					return
				}
			}

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

			callCtx := Context{
				SessionID: sessionID,
				Abort:     ctx.Done(),
			}
			result := tool.Execute(ctx, callCtx, inputJSON)

			results[idx] = ToolCallResult{
				ToolCallID: tc.ToolCallID,
				ToolName:   tc.ToolName,
				Content:    result.Output,
				IsError:    result.IsError,
			}
		}(i, call)
	}

	wg.Wait()
	return results
}
