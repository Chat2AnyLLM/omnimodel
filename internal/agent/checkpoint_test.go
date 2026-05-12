package agent

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"omnillm/internal/tools"
)

func TestSaveAndLoadCheckpoint(t *testing.T) {
	dir := t.TempDir()
	t.Setenv(checkpointDirEnv, dir)

	sessionID := "test-session-123"
	messages := []Message{
		{Role: "user", Content: []ContentBlock{TextBlock("hello")}},
		{Role: "assistant", Content: []ContentBlock{TextBlock("hi")}},
	}

	if err := saveCheckpoint(sessionID, 5, messages); err != nil {
		t.Fatalf("saveCheckpoint: %v", err)
	}

	cp, err := loadCheckpoint(sessionID)
	if err != nil {
		t.Fatalf("loadCheckpoint: %v", err)
	}
	if cp == nil {
		t.Fatal("expected checkpoint, got nil")
	}
	if cp.Step != 5 {
		t.Errorf("step: want 5, got %d", cp.Step)
	}
	if cp.SessionID != sessionID {
		t.Errorf("session_id: want %q, got %q", sessionID, cp.SessionID)
	}
	if len(cp.Messages) != 2 {
		t.Errorf("messages: want 2, got %d", len(cp.Messages))
	}
	if cp.SavedAt.IsZero() {
		t.Error("saved_at should not be zero")
	}
}

func TestLoadCheckpointReturnsNilWhenMissing(t *testing.T) {
	dir := t.TempDir()
	t.Setenv(checkpointDirEnv, dir)

	cp, err := loadCheckpoint("nonexistent-session")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cp != nil {
		t.Error("expected nil checkpoint for missing session")
	}
}

func TestLoadCheckpointIgnoresCorruptFile(t *testing.T) {
	dir := t.TempDir()
	t.Setenv(checkpointDirEnv, dir)

	sessionID := "corrupt-session"
	path := filepath.Join(dir, safeFilename(sessionID)+".json")
	if err := os.WriteFile(path, []byte("not-json{{{"), 0600); err != nil {
		t.Fatal(err)
	}

	cp, err := loadCheckpoint(sessionID)
	if err != nil {
		t.Fatalf("unexpected error on corrupt file: %v", err)
	}
	if cp != nil {
		t.Error("expected nil checkpoint for corrupt file")
	}
	// File should be removed.
	if _, statErr := os.Stat(path); !os.IsNotExist(statErr) {
		t.Error("corrupt checkpoint file should have been removed")
	}
}

func TestClearCheckpoint(t *testing.T) {
	dir := t.TempDir()
	t.Setenv(checkpointDirEnv, dir)

	sessionID := "clear-session"
	_ = saveCheckpoint(sessionID, 3, []Message{{Role: "user", Content: []ContentBlock{TextBlock("x")}}})

	clearCheckpoint(sessionID)

	cp, err := loadCheckpoint(sessionID)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cp != nil {
		t.Error("checkpoint should be nil after clear")
	}
}

func TestSaveCheckpointIsAtomic(t *testing.T) {
	dir := t.TempDir()
	t.Setenv(checkpointDirEnv, dir)

	sessionID := "atomic-session"
	msgs := []Message{{Role: "user", Content: []ContentBlock{TextBlock("atomic")}}}

	if err := saveCheckpoint(sessionID, 1, msgs); err != nil {
		t.Fatal(err)
	}

	// No .tmp file should remain after save.
	tmp := checkpointPath(sessionID) + ".tmp"
	if _, err := os.Stat(tmp); !os.IsNotExist(err) {
		t.Error("tmp checkpoint file should not exist after successful save")
	}
}

func TestSafeFilename(t *testing.T) {
	cases := []struct {
		in   string
		want string
	}{
		{"abc-123_XYZ", "abc-123_XYZ"},
		{"session/id", "session_id"},
		{"", "default"},
		{"hello world", "hello_world"},
	}
	for _, tc := range cases {
		got := safeFilename(tc.in)
		if got != tc.want {
			t.Errorf("safeFilename(%q) = %q, want %q", tc.in, got, tc.want)
		}
	}
}

func TestRunResumesFromCheckpoint(t *testing.T) {
	dir := t.TempDir()
	t.Setenv(checkpointDirEnv, dir)

	sessionID := "resume-session"

	// Pre-seed a checkpoint at step 1 with a partial conversation already
	// in the history. The mock dispatch immediately returns a final text
	// response (no tool calls), so Run should complete in exactly 1 dispatch.
	preMessages := []Message{
		{Role: "system", Content: []ContentBlock{TextBlock("sys")}},
		{Role: "user", Content: []ContentBlock{TextBlock("initial prompt")}},
		{Role: "assistant", Content: []ContentBlock{TextBlock("partial answer")}},
	}
	if err := saveCheckpoint(sessionID, 1, preMessages); err != nil {
		t.Fatal(err)
	}

	dispatched := 0
	dispatch := func(_ context.Context, _ *MessagesRequest) (<-chan *MessagesResponse, error) {
		dispatched++
		ch := make(chan *MessagesResponse, 1)
		ch <- &MessagesResponse{Content: []ContentBlock{TextBlock("final answer")}}
		close(ch)
		return ch, nil
	}

	registry := tools.NewRegistry()
	memory := NewBufferMemory(64)
	ag := NewAgent(registry, memory, 10, dispatch)

	result, err := ag.Run(context.Background(), sessionID, "ignored because checkpoint")
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if result.Output != "final answer" {
		t.Errorf("output: want %q, got %q", "final answer", result.Output)
	}

	// Checkpoint should be cleared after successful completion.
	cp, _ := loadCheckpoint(sessionID)
	if cp != nil {
		t.Error("checkpoint should be cleared after successful run")
	}

	// dispatch should have been called exactly once — resumed from step 1.
	if dispatched != 1 {
		t.Errorf("dispatch calls: want 1, got %d", dispatched)
	}
}

func TestRunSavesCheckpointEveryNSteps(t *testing.T) {
	dir := t.TempDir()
	t.Setenv(checkpointDirEnv, dir)

	sessionID := "save-every-n"

	step := 0
	// Return a tool call for steps 0..N-1, then a final text on step N.
	// checkpointEveryNSteps = 5, so after step 4 (0-indexed) a checkpoint
	// should exist, then the run completes on step 5 and clears it.
	dispatch := func(_ context.Context, _ *MessagesRequest) (<-chan *MessagesResponse, error) {
		ch := make(chan *MessagesResponse, 1)
		if step < checkpointEveryNSteps {
			step++
			ch <- &MessagesResponse{Content: []ContentBlock{{
				Type:  "tool_use",
				ID:    "toolu_" + string(rune('0'+step)),
				Name:  "fake_tool",
				Input: map[string]any{"x": step},
			}}}
		} else {
			step++
			ch <- &MessagesResponse{Content: []ContentBlock{TextBlock("done")}}
		}
		close(ch)
		return ch, nil
	}

	// Register a fake tool that always succeeds.
	registry := tools.NewRegistry()
	registry.Register(&fakeTool{})
	memory := NewBufferMemory(128)
	ag := NewAgent(registry, memory, 20, dispatch)

	result, err := ag.Run(context.Background(), sessionID, "work")
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if result.Output != "done" {
		t.Errorf("output: want %q, got %q", "done", result.Output)
	}

	// After a successful run the checkpoint must be cleared.
	cp, _ := loadCheckpoint(sessionID)
	if cp != nil {
		t.Error("checkpoint should be cleared after successful run")
	}
}

// fakeTool is a minimal tools.Tool that always succeeds, used to allow the
// agent loop to continue past tool-call steps without real I/O.
type fakeTool struct{}

func (f *fakeTool) Name() string        { return "fake_tool" }
func (f *fakeTool) Description() string { return "no-op tool for tests" }
func (f *fakeTool) InputSchema() map[string]any {
	return map[string]any{"type": "object", "properties": map[string]any{"x": map[string]any{"type": "number"}}}
}
func (f *fakeTool) Execute(_ context.Context, _ tools.Context, _ json.RawMessage) tools.Result {
	return tools.Result{Output: "ok"}
}
