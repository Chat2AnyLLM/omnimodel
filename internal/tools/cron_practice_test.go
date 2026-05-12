package tools

import (
	"context"
	"encoding/json"
	"sync/atomic"
	"strings"
	"testing"
	"time"
)

func TestScheduleHeartbeatCreatesTaskEntry(t *testing.T) {
	tool := ScheduleHeartbeat()
	store := NewTaskStore()
	input, _ := json.Marshal(map[string]any{
		"interval_seconds": 30,
		"prompt":           "check queue depth",
		"target":           "ops-worker",
	})
	res := tool.Execute(context.Background(), Context{TaskStore: store}, input)
	if res.IsError {
		t.Fatalf("unexpected error: %s", res.Output)
	}
	if !strings.Contains(res.Output, "Job ID:") {
		t.Fatalf("expected job id in output: %q", res.Output)
	}
	if len(store.list()) != 1 {
		t.Fatalf("expected one task entry, got %d", len(store.list()))
	}
}

func TestTriggerEventDispatchesToWorkerWhenAvailable(t *testing.T) {
	tool := TriggerEvent()
	var targetSeen, messageSeen string
	ctx := Context{
		SendMessageFn: func(_ context.Context, to, message string) (string, error) {
			targetSeen = to
			messageSeen = message
			return "ack", nil
		},
	}
	input, _ := json.Marshal(map[string]any{
		"event_type": "build.failed",
		"payload":    "service=api",
		"target":     "incident-worker",
	})
	res := tool.Execute(context.Background(), ctx, input)
	if res.IsError {
		t.Fatalf("unexpected error: %s", res.Output)
	}
	if targetSeen != "incident-worker" {
		t.Fatalf("unexpected dispatch target: %q", targetSeen)
	}
	if !strings.Contains(messageSeen, "build.failed") {
		t.Fatalf("unexpected dispatched message: %q", messageSeen)
	}
}

func TestTriggerEventFallsBackToTaskStore(t *testing.T) {
	tool := TriggerEvent()
	store := NewTaskStore()
	input, _ := json.Marshal(map[string]any{"event_type": "deploy.completed"})
	res := tool.Execute(context.Background(), Context{TaskStore: store}, input)
	if res.IsError {
		t.Fatalf("unexpected error: %s", res.Output)
	}
	if len(store.list()) != 1 {
		t.Fatalf("expected one queued event task, got %d", len(store.list()))
	}
}

func TestScheduleHeartbeatRunsAndCanBeStopped(t *testing.T) {
	tool := ScheduleHeartbeat()
	store := NewTaskStore()
	var calls atomic.Int32
	ctx := Context{
		TaskStore: store,
		SendMessageFn: func(_ context.Context, to, message string) (string, error) {
			calls.Add(1)
			return "heartbeat ok", nil
		},
	}
	input, _ := json.Marshal(map[string]any{
		"interval_seconds": 1,
		"prompt":           "check queue depth",
		"target":           "ops-worker",
	})
	res := tool.Execute(context.Background(), ctx, input)
	if res.IsError {
		t.Fatalf("unexpected error: %s", res.Output)
	}

	if len(store.list()) != 1 {
		t.Fatalf("expected one task entry, got %d", len(store.list()))
	}
	taskID := store.list()[0].ID

	deadline := time.Now().Add(2500 * time.Millisecond)
	for calls.Load() == 0 && time.Now().Before(deadline) {
		time.Sleep(50 * time.Millisecond)
	}
	if calls.Load() == 0 {
		t.Fatal("expected heartbeat scheduler to fire at least once")
	}

	stopTool := TaskStop()
	stopInput, _ := json.Marshal(map[string]any{"task_id": taskID})
	stopRes := stopTool.Execute(context.Background(), Context{TaskStore: store}, stopInput)
	if stopRes.IsError {
		t.Fatalf("unexpected stop error: %s", stopRes.Output)
	}

	run, ok := store.get(taskID)
	if !ok {
		t.Fatalf("task %s not found", taskID)
	}
	if run.Status != TaskRunStopped {
		t.Fatalf("expected task status stopped, got %s", run.Status)
	}
}

func TestScheduleCronRunsOnceWithDescriptor(t *testing.T) {
	tool := ScheduleCron()
	store := NewTaskStore()
	var calls atomic.Int32
	ctx := Context{
		TaskStore: store,
		SendMessageFn: func(_ context.Context, to, message string) (string, error) {
			calls.Add(1)
			return "cron ok", nil
		},
	}
	input, _ := json.Marshal(map[string]any{
		"cron":      "@every 1s",
		"prompt":    "run scheduled check",
		"recurring": false,
		"target":    "scheduler-worker",
	})
	res := tool.Execute(context.Background(), ctx, input)
	if res.IsError {
		t.Fatalf("unexpected error: %s", res.Output)
	}

	if len(store.list()) != 1 {
		t.Fatalf("expected one task entry, got %d", len(store.list()))
	}
	taskID := store.list()[0].ID

	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		run, ok := store.get(taskID)
		if ok && run.Status == TaskRunCompleted {
			break
		}
		time.Sleep(50 * time.Millisecond)
	}

	run, ok := store.get(taskID)
	if !ok {
		t.Fatalf("task %s not found", taskID)
	}
	if run.Status != TaskRunCompleted {
		t.Fatalf("expected completed status, got %s", run.Status)
	}
	if calls.Load() != 1 {
		t.Fatalf("expected exactly one cron dispatch, got %d", calls.Load())
	}
}
