package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"sync"
)

// ─── TodoStore ────────────────────────────────────────────────────────────────

type TodoStatus string

const (
	TodoPending    TodoStatus = "pending"
	TodoInProgress TodoStatus = "in_progress"
	TodoCompleted  TodoStatus = "completed"
)

type TodoItem struct {
	ID      string     `json:"id"`
	Content string     `json:"content"`
	Status  TodoStatus `json:"status"`
	// Priority: high / medium / low
	Priority string `json:"priority"`
}

// TodoStore is a thread-safe in-memory todo list for one agent session.
type TodoStore struct {
	mu    sync.Mutex
	items []TodoItem
	next  int
}

func NewTodoStore() *TodoStore { return &TodoStore{} }

func (s *TodoStore) Write(items []TodoItem) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.items = items
}

func (s *TodoStore) Read() []TodoItem {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]TodoItem, len(s.items))
	copy(out, s.items)
	return out
}

// ─── todo_write tool ──────────────────────────────────────────────────────────

type todoWriteTool struct{}

func TodoWrite() Tool { return &todoWriteTool{} }

func (t *todoWriteTool) Name() string { return "todo_write" }

func (t *todoWriteTool) Description() string {
	return "Create or fully replace the session-scoped todo list. " +
		"Pass the complete desired list; the previous list is discarded. " +
		"Use this to track tasks, subtasks, and progress across a multi-step agent run."
}

func (t *todoWriteTool) InputSchema() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"todos": map[string]any{
				"type":        "array",
				"description": "The complete list of todo items.",
				"items": map[string]any{
					"type": "object",
					"properties": map[string]any{
						"id":       map[string]any{"type": "string", "description": "Unique identifier for the item."},
						"content":  map[string]any{"type": "string", "description": "What needs to be done."},
						"status":   map[string]any{"type": "string", "enum": []string{"pending", "in_progress", "completed"}, "description": "Current status."},
						"priority": map[string]any{"type": "string", "enum": []string{"high", "medium", "low"}, "description": "Priority level."},
					},
					"required": []string{"id", "content", "status"},
				},
			},
		},
		"required": []string{"todos"},
	}
}

func (t *todoWriteTool) Execute(ctx context.Context, call Context, input json.RawMessage) Result {
	var p struct {
		Todos []TodoItem `json:"todos"`
	}
	if err := json.Unmarshal(input, &p); err != nil {
		return Result{Output: "error: " + err.Error(), IsError: true}
	}
	store := call.TodoStore
	if store == nil {
		return Result{Output: "error: todo store not available in this context", IsError: true}
	}
	store.Write(p.Todos)

	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("Todo list updated (%d items):\n", len(p.Todos)))
	for _, item := range p.Todos {
		mark := "○"
		switch item.Status {
		case TodoInProgress:
			mark = "◑"
		case TodoCompleted:
			mark = "✓"
		}
		sb.WriteString(fmt.Sprintf("  %s [%s] %s\n", mark, item.ID, item.Content))
	}
	return Result{Output: strings.TrimRight(sb.String(), "\n")}
}
