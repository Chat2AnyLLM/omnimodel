package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

// ─── TaskStore ────────────────────────────────────────────────────────────────

type TaskRunStatus string

const (
	TaskRunPending   TaskRunStatus = "pending"
	TaskRunRunning   TaskRunStatus = "running"
	TaskRunCompleted TaskRunStatus = "completed"
	TaskRunFailed    TaskRunStatus = "failed"
	TaskRunStopped   TaskRunStatus = "stopped"
)

type TaskRun struct {
	ID          string
	Description string
	Status      TaskRunStatus
	Output      string
	cancel      context.CancelFunc
}

// TaskStore is a thread-safe registry of background sub-agent tasks.
type TaskStore struct {
	mu    sync.RWMutex
	tasks map[string]*TaskRun
	seq   atomic.Int64
}

func NewTaskStore() *TaskStore {
	return &TaskStore{tasks: make(map[string]*TaskRun)}
}

func (s *TaskStore) add(t *TaskRun) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.tasks[t.ID] = t
}

func (s *TaskStore) get(id string) (*TaskRun, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	t, ok := s.tasks[id]
	return t, ok
}

func (s *TaskStore) list() []*TaskRun {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]*TaskRun, 0, len(s.tasks))
	for _, t := range s.tasks {
		out = append(out, t)
	}
	return out
}

func (s *TaskStore) nextID() string {
	return fmt.Sprintf("task-%d", s.seq.Add(1))
}

func (s *TaskStore) setOutput(run *TaskRun, output string, status TaskRunStatus) {
	s.mu.Lock()
	defer s.mu.Unlock()
	run.Output = output
	run.Status = status
}

func (s *TaskStore) isDone(run *TaskRun) bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return run.Status == TaskRunCompleted || run.Status == TaskRunFailed || run.Status == TaskRunStopped
}

// ─── task_create ──────────────────────────────────────────────────────────────

type taskCreateTool struct{}

func TaskCreate() Tool { return &taskCreateTool{} }

func (t *taskCreateTool) Name() string { return "task_create" }
func (t *taskCreateTool) Description() string {
	return "Spawn a background shell command as a named task and return its task ID immediately. " +
		"Use task_output to retrieve results and task_stop to cancel."
}
func (t *taskCreateTool) InputSchema() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"command":     map[string]any{"type": "string", "description": "Shell command to run in the background."},
			"description": map[string]any{"type": "string", "description": "Human-readable description of what the task does."},
		},
		"required": []string{"command"},
	}
}
func (t *taskCreateTool) Execute(ctx context.Context, call Context, input json.RawMessage) Result {
	var p struct {
		Command     string `json:"command"`
		Description string `json:"description"`
	}
	if err := json.Unmarshal(input, &p); err != nil {
		return Result{Output: "error: " + err.Error(), IsError: true}
	}
	p.Command = strings.TrimSpace(p.Command)
	if p.Command == "" {
		return Result{Output: "error: command is required", IsError: true}
	}
	store := call.TaskStore
	if store == nil {
		return Result{Output: "error: task store not available", IsError: true}
	}
	id := store.nextID()
	taskCtx, cancel := context.WithCancel(context.Background())
	run := &TaskRun{
		ID:          id,
		Description: p.Description,
		Status:      TaskRunRunning,
		cancel:      cancel,
	}
	store.add(run)

	cmd := p.Command
	go func() {
		res := runShellCommand(taskCtx, cmd, 0)
		if run.Status == TaskRunStopped {
			return
		}
		if res.IsError {
			store.setOutput(run, res.Output, TaskRunFailed)
		} else {
			store.setOutput(run, res.Output, TaskRunCompleted)
		}
	}()

	return Result{Output: fmt.Sprintf("Task created: %s\nDescription: %s\nStatus: running", id, p.Description)}
}

// ─── task_get ─────────────────────────────────────────────────────────────────

type taskGetTool struct{}

func TaskGet() Tool { return &taskGetTool{} }

func (t *taskGetTool) Name() string { return "task_get" }
func (t *taskGetTool) Description() string {
	return "Get the current status and details of a background task by its ID."
}
func (t *taskGetTool) InputSchema() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"task_id": map[string]any{"type": "string", "description": "The task ID returned by task_create."},
		},
		"required": []string{"task_id"},
	}
}
func (t *taskGetTool) Execute(ctx context.Context, call Context, input json.RawMessage) Result {
	var p struct {
		TaskID string `json:"task_id"`
	}
	if err := json.Unmarshal(input, &p); err != nil {
		return Result{Output: "error: " + err.Error(), IsError: true}
	}
	store := call.TaskStore
	if store == nil {
		return Result{Output: "error: task store not available", IsError: true}
	}
	run, ok := store.get(p.TaskID)
	if !ok {
		return Result{Output: fmt.Sprintf("error: task %q not found", p.TaskID), IsError: true}
	}
	store.mu.RLock()
	desc, status := run.Description, run.Status
	store.mu.RUnlock()
	return Result{Output: fmt.Sprintf("ID: %s\nDescription: %s\nStatus: %s", run.ID, desc, status)}
}

// ─── task_list ────────────────────────────────────────────────────────────────

type taskListTool struct{}

func TaskList() Tool { return &taskListTool{} }

func (t *taskListTool) Name() string { return "task_list" }
func (t *taskListTool) Description() string {
	return "List all background tasks for this session with their IDs and statuses."
}
func (t *taskListTool) InputSchema() map[string]any {
	return map[string]any{"type": "object", "properties": map[string]any{}}
}
func (t *taskListTool) Execute(ctx context.Context, call Context, input json.RawMessage) Result {
	store := call.TaskStore
	if store == nil {
		return Result{Output: "error: task store not available", IsError: true}
	}
	runs := store.list()
	if len(runs) == 0 {
		return Result{Output: "No tasks."}
	}
	var sb strings.Builder
	for _, r := range runs {
		store.mu.RLock()
		sb.WriteString(fmt.Sprintf("%s  [%s]  %s\n", r.ID, r.Status, r.Description))
		store.mu.RUnlock()
	}
	return Result{Output: strings.TrimRight(sb.String(), "\n")}
}

// ─── task_output ──────────────────────────────────────────────────────────────

type taskOutputTool struct{}

func TaskOutput() Tool { return &taskOutputTool{} }

func (t *taskOutputTool) Name() string { return "task_output" }
func (t *taskOutputTool) Description() string {
	return "Retrieve the stdout/stderr output of a background task. Optionally block until the task finishes."
}
func (t *taskOutputTool) InputSchema() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"task_id":         map[string]any{"type": "string", "description": "Task ID."},
			"block":           map[string]any{"type": "boolean", "description": "If true, wait for the task to finish (default false)."},
			"timeout_seconds": map[string]any{"type": "integer", "description": "Max seconds to wait when block=true (default 30)."},
		},
		"required": []string{"task_id"},
	}
}
func (t *taskOutputTool) Execute(ctx context.Context, call Context, input json.RawMessage) Result {
	var p struct {
		TaskID         string `json:"task_id"`
		Block          bool   `json:"block"`
		TimeoutSeconds int    `json:"timeout_seconds"`
	}
	if err := json.Unmarshal(input, &p); err != nil {
		return Result{Output: "error: " + err.Error(), IsError: true}
	}
	store := call.TaskStore
	if store == nil {
		return Result{Output: "error: task store not available", IsError: true}
	}
	run, ok := store.get(p.TaskID)
	if !ok {
		return Result{Output: fmt.Sprintf("error: task %q not found", p.TaskID), IsError: true}
	}

	if p.Block {
		timeout := 30
		if p.TimeoutSeconds > 0 {
			timeout = p.TimeoutSeconds
		}
		deadline := time.Now().Add(time.Duration(timeout) * time.Second)
		for !store.isDone(run) {
			if time.Now().After(deadline) {
				break
			}
			select {
			case <-ctx.Done():
				break
			case <-time.After(100 * time.Millisecond):
			}
		}
	}

	store.mu.RLock()
	status, output := run.Status, run.Output
	store.mu.RUnlock()
	return Result{Output: fmt.Sprintf("Status: %s\n\n%s", status, output)}
}

// ─── task_stop ────────────────────────────────────────────────────────────────

type taskStopTool struct{}

func TaskStop() Tool { return &taskStopTool{} }

func (t *taskStopTool) Name() string { return "task_stop" }
func (t *taskStopTool) Description() string {
	return "Cancel a running background task."
}
func (t *taskStopTool) InputSchema() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"task_id": map[string]any{"type": "string", "description": "Task ID to stop."},
		},
		"required": []string{"task_id"},
	}
}
func (t *taskStopTool) Execute(ctx context.Context, call Context, input json.RawMessage) Result {
	var p struct {
		TaskID string `json:"task_id"`
	}
	if err := json.Unmarshal(input, &p); err != nil {
		return Result{Output: "error: " + err.Error(), IsError: true}
	}
	store := call.TaskStore
	if store == nil {
		return Result{Output: "error: task store not available", IsError: true}
	}
	run, ok := store.get(p.TaskID)
	if !ok {
		return Result{Output: fmt.Sprintf("error: task %q not found", p.TaskID), IsError: true}
	}
	store.mu.Lock()
	run.Status = TaskRunStopped
	store.mu.Unlock()
	if run.cancel != nil {
		run.cancel()
	}
	return Result{Output: fmt.Sprintf("Task %s stopped.", p.TaskID)}
}

// ─── task_update ──────────────────────────────────────────────────────────────

type taskUpdateTool struct{}

func TaskUpdate() Tool { return &taskUpdateTool{} }

func (t *taskUpdateTool) Name() string { return "task_update" }
func (t *taskUpdateTool) Description() string {
	return "Update the description or status of an existing background task."
}
func (t *taskUpdateTool) InputSchema() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"task_id":     map[string]any{"type": "string", "description": "Task ID."},
			"description": map[string]any{"type": "string", "description": "New description (optional)."},
			"status": map[string]any{
				"type":        "string",
				"enum":        []string{"pending", "running", "completed", "failed", "stopped"},
				"description": "New status (optional).",
			},
		},
		"required": []string{"task_id"},
	}
}
func (t *taskUpdateTool) Execute(ctx context.Context, call Context, input json.RawMessage) Result {
	var p struct {
		TaskID      string `json:"task_id"`
		Description string `json:"description"`
		Status      string `json:"status"`
	}
	if err := json.Unmarshal(input, &p); err != nil {
		return Result{Output: "error: " + err.Error(), IsError: true}
	}
	store := call.TaskStore
	if store == nil {
		return Result{Output: "error: task store not available", IsError: true}
	}
	run, ok := store.get(p.TaskID)
	if !ok {
		return Result{Output: fmt.Sprintf("error: task %q not found", p.TaskID), IsError: true}
	}
	store.mu.Lock()
	if p.Description != "" {
		run.Description = p.Description
	}
	if p.Status != "" {
		run.Status = TaskRunStatus(p.Status)
	}
	store.mu.Unlock()
	return Result{Output: fmt.Sprintf("Task %s updated.", p.TaskID)}
}
