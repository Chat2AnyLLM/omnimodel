package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"omnillm/internal/tools"
)

func TestRegisterSubAgentToolsExcludesRecursiveAndWorktreeTools(t *testing.T) {
	r := tools.NewRegistry()
	registerSubAgentTools(r)

	forbidden := []string{"agent", "send_message", "orchestrate_agents", "enter_worktree", "exit_worktree"}
	for _, name := range forbidden {
		if tool := r.Get(name); tool != nil {
			t.Fatalf("expected %q to be excluded from sub-agent tools", name)
		}
	}

	if r.Get("bash") == nil {
		t.Fatal("expected core tool bash to be present in sub-agent tools")
	}
}

func TestSessionOrchestratorPersistsWorkerHistoryAndMailboxes(t *testing.T) {
	root := t.TempDir()
	t.Setenv(workspaceDirEnv, root)

	callCount := 0
	var payloads []map[string]any
	client := &stubAgentClient{
		postFn: func(_ string, body any) ([]byte, error) {
			callCount++
			payload := decodePayload(t, body)
			payloads = append(payloads, payload)
			text := "worker-reply-1"
			if callCount > 1 {
				text = "worker-reply-2"
			}
			return []byte(fmt.Sprintf(`{"id":"msg_%d","type":"message","role":"assistant","model":"deepseek-v4-flash","content":[{"type":"text","text":%q}],"stop_reason":"end_turn","usage":{"input_tokens":5,"output_tokens":2}}`, callCount, text)), nil
		},
	}

	orch := SessionOrchestrator("main-session", SubAgentOptions{
		Client:   client,
		Model:    "deepseek-v4-flash",
		Backend:  "google-adk",
		APIShape: DefaultAPIShape,
		MaxTurns: 8,
	})

	out1, err := orch.SendMessage(context.Background(), "planner", "first task")
	if err != nil {
		t.Fatalf("SendMessage #1 returned error: %v", err)
	}
	if out1 != "worker-reply-1" {
		t.Fatalf("unexpected first reply: %q", out1)
	}

	out2, err := orch.SendMessage(context.Background(), "planner", "second task")
	if err != nil {
		t.Fatalf("SendMessage #2 returned error: %v", err)
	}
	if out2 != "worker-reply-2" {
		t.Fatalf("unexpected second reply: %q", out2)
	}

	if len(payloads) != 2 {
		t.Fatalf("expected 2 upstream payloads, got %d", len(payloads))
	}

	secondTexts := messageTexts(payloads[1])
	joined := strings.Join(secondTexts, "\n")
	for _, want := range []string{"first task", "worker-reply-1", "second task"} {
		if !strings.Contains(joined, want) {
			t.Fatalf("second worker request missing %q in messages: %q", want, joined)
		}
	}

	sessionDir := filepath.Join(root, multiAgentRootDir, "sessions", "main-session", "workers", "planner")
	inboxFiles, err := os.ReadDir(filepath.Join(sessionDir, "inbox"))
	if err != nil {
		t.Fatalf("read inbox dir: %v", err)
	}
	outboxFiles, err := os.ReadDir(filepath.Join(sessionDir, "outbox"))
	if err != nil {
		t.Fatalf("read outbox dir: %v", err)
	}
	if len(inboxFiles) != 2 || len(outboxFiles) != 2 {
		t.Fatalf("expected 2 inbox and 2 outbox files, got inbox=%d outbox=%d", len(inboxFiles), len(outboxFiles))
	}
}

func TestSessionOrchestratorReusesInstancePerSession(t *testing.T) {
	first := SessionOrchestrator("same-session", SubAgentOptions{})
	second := SessionOrchestrator("same-session", SubAgentOptions{})
	if first != second {
		t.Fatal("expected session orchestrator to be reused for the same session")
	}
}

func decodePayload(t *testing.T, body any) map[string]any {
	t.Helper()
	data, err := json.Marshal(body)
	if err != nil {
		t.Fatalf("marshal body: %v", err)
	}
	var payload map[string]any
	if err := json.Unmarshal(data, &payload); err != nil {
		t.Fatalf("unmarshal payload: %v", err)
	}
	return payload
}

func messageTexts(payload map[string]any) []string {
	msgs, _ := payload["messages"].([]any)
	out := make([]string, 0, len(msgs))
	for _, raw := range msgs {
		msg, _ := raw.(map[string]any)
		content, _ := msg["content"].([]any)
		for _, blockRaw := range content {
			block, _ := blockRaw.(map[string]any)
			if text, ok := block["text"].(string); ok && strings.TrimSpace(text) != "" {
				out = append(out, text)
			}
		}
	}
	return out
}
