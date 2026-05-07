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
	"fmt"
	"sync"
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
	// AskUser is an optional callback for tools that need to ask the user
	// a question during execution (e.g. ask_user_question tool).
	AskUser func(ctx context.Context, question string, options []string) (string, error)

	// Shared session-scoped stores (all safe for concurrent access).

	// TodoStore holds the persistent todo/task list for this agent session.
	TodoStore *TodoStore
	// TaskStore holds background sub-agent tasks spawned via task_create.
	TaskStore *TaskStore
	// PlanState tracks whether the agent is currently in plan mode.
	PlanState *PlanState
	// WorktreeState tracks any active git worktree for this session.
	WorktreeState *WorktreeState
	// Registry is a back-reference used by tool_search to list available tools.
	Registry *Registry
	// ConfigStore holds runtime agent config key-value pairs.
	ConfigStore *ConfigStore
	// SendMessageFn delivers a message to a named agent / sub-process (optional).
	SendMessageFn func(ctx context.Context, to, message string) (string, error)
}

// ─── Tool ────────────────────────────────────────────────────────────────────

// Tool is a callable tool that an agent can invoke.
type Tool interface {
	Name() string
	Description() string
	InputSchema() map[string]any
	Execute(ctx context.Context, call Context, input json.RawMessage) Result
}

type ToolDefinition struct {
	Name        string         `json:"name"`
	Description *string        `json:"description,omitempty"`
	InputSchema map[string]any `json:"input_schema"`
}

type ToolCall struct {
	ID        string
	Name      string
	Arguments map[string]any
}

// ─── Registry ────────────────────────────────────────────────────────────────

// Registry holds registered tools and provides lookup, conversion, and batch
// execution.
type Registry struct {
	mu      sync.RWMutex
	tools   map[string]Tool
	checker PermissionChecker
	askUser func(ctx context.Context, question string, options []string) (string, error)

	// Session-scoped stores — set once at construction via SetStores.
	TodoStore     *TodoStore
	TaskStore     *TaskStore
	PlanState     *PlanState
	WorktreeState *WorktreeState
	ConfigStore   *ConfigStore
	SendMessageFn func(ctx context.Context, to, message string) (string, error)
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

// SetAskUserCallback configures a callback for tools that need to ask the user
// a question during execution.
func (r *Registry) SetAskUserCallback(askUser func(ctx context.Context, question string, options []string) (string, error)) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.askUser = askUser
}

func (r *Registry) permissionChecker() PermissionChecker {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.checker
}

func (r *Registry) askUserCallback() func(ctx context.Context, question string, options []string) (string, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.askUser
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

// Definitions converts the registry's tools to Anthropic /v1/messages tool definitions.
func (r *Registry) Definitions() []ToolDefinition {
	r.mu.RLock()
	defer r.mu.RUnlock()
	out := make([]ToolDefinition, 0, len(r.tools))
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
		out = append(out, ToolDefinition{
			Name:        t.Name(),
			Description: descPtr,
			InputSchema: schema,
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

// ExecuteCalls runs multiple tool calls concurrently and returns results.
// Errors in individual tool calls become error result messages, not fatal errors.
func (r *Registry) ExecuteCalls(ctx context.Context, sessionID string, calls []ToolCall) []ToolCallResult {
	results := make([]ToolCallResult, len(calls))
	var wg sync.WaitGroup
	checker := r.permissionChecker()
	askUser := r.askUserCallback()

	for i, call := range calls {
		wg.Add(1)
		go func(idx int, tc ToolCall) {
			defer wg.Done()
			defer func() {
				if r := recover(); r != nil {
					results[idx] = ToolCallResult{
						ToolCallID: tc.ID,
						ToolName:   tc.Name,
						Content:    fmt.Sprintf("error: tool panicked: %v", r),
						IsError:    true,
					}
				}
			}()

			tool := r.Get(tc.Name)
			if tool == nil {
				results[idx] = ToolCallResult{
					ToolCallID: tc.ID,
					ToolName:   tc.Name,
					Content:    "error: unknown tool " + tc.Name,
					IsError:    true,
				}
				return
			}

			if checker != nil {
				approved, err := checker(ctx, PermissionRequest{
					SessionID: sessionID,
					ToolName:  tc.Name,
					Arguments: tc.Arguments,
				})
				if err != nil {
					results[idx] = ToolCallResult{
						ToolCallID: tc.ID,
						ToolName:   tc.Name,
						Content:    "error: permission check failed: " + err.Error(),
						IsError:    true,
					}
					return
				}
				if !approved {
					results[idx] = ToolCallResult{
						ToolCallID: tc.ID,
						ToolName:   tc.Name,
						Content:    "error: tool execution denied by user",
						IsError:    true,
					}
					return
				}
			}

			inputJSON, err := json.Marshal(tc.Arguments)
			if err != nil {
				results[idx] = ToolCallResult{
					ToolCallID: tc.ID,
					ToolName:   tc.Name,
					Content:    "error: failed to marshal tool arguments: " + err.Error(),
					IsError:    true,
				}
				return
			}

			callCtx := Context{
				SessionID:     sessionID,
				Abort:         ctx.Done(),
				Checker:       checker,
				AskUser:       askUser,
				TodoStore:     r.TodoStore,
				TaskStore:     r.TaskStore,
				PlanState:     r.PlanState,
				WorktreeState: r.WorktreeState,
				ConfigStore:   r.ConfigStore,
				Registry:      r,
				SendMessageFn: r.SendMessageFn,
			}
			result := tool.Execute(ctx, callCtx, inputJSON)

			results[idx] = ToolCallResult{
				ToolCallID: tc.ID,
				ToolName:   tc.Name,
				Content:    result.Output,
				IsError:    result.IsError,
			}
		}(i, call)
	}

	wg.Wait()
	return results
}
