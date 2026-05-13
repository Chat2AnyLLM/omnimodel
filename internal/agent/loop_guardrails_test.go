package agent

import (
	"context"
	"strings"
	"testing"

	"omnillm/internal/tools"
)

func TestRunAbortsOnRepeatedIdenticalToolCalls(t *testing.T) {
	registry := tools.NewRegistry()
	registry.Register(tools.Bash())

	dispatch := func(ctx context.Context, req *MessagesRequest) (<-chan *MessagesResponse, error) {
		ch := make(chan *MessagesResponse, 1)
		ch <- &MessagesResponse{Content: []ContentBlock{{
			Type: "tool_use",
			ID:   "toolu_repeat",
			Name: "bash",
			Input: map[string]any{
				"command": "echo hello_loop_guard",
			},
		}}}
		close(ch)
		return ch, nil
	}

	ag := NewAgent(registry, NewBufferMemory(16), 10, dispatch)
	_, err := ag.Run(context.Background(), "session-repeated", "run a command")
	if err == nil {
		t.Fatal("expected repeated tool-call guard to abort")
	}
	if !strings.Contains(err.Error(), "repeated identical tool calls") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestRunContinuesAfterToolIntentTextForCodebasePrompt(t *testing.T) {
	registry := tools.NewRegistry()
	registry.Register(tools.Glob())

	step := 0
	dispatch := func(ctx context.Context, req *MessagesRequest) (<-chan *MessagesResponse, error) {
		step++
		ch := make(chan *MessagesResponse, 1)
		switch step {
		case 1:
			ch <- &MessagesResponse{Content: []ContentBlock{{
				Type: "tool_use",
				ID:   "toolu_glob",
				Name: "glob",
				Input: map[string]any{
					"pattern": "*",
				},
			}}, StopReason: StopReasonToolUse}
		case 2:
			ch <- &MessagesResponse{Content: []ContentBlock{TextBlock("Now let me check the main directories:")}, StopReason: StopReasonEndTurn}
		default:
			toolChoice, ok := req.ToolChoice.(map[string]any)
			if !ok || toolChoice["functionName"] != "read" {
				t.Fatalf("third request tool choice = %#v, want read", req.ToolChoice)
			}
			ch <- &MessagesResponse{Content: []ContentBlock{TextBlock("internal/ and frontend/ provider test summary")}, StopReason: StopReasonEndTurn}
		}
		close(ch)
		return ch, nil
	}

	ag := NewAgent(registry, NewBufferMemory(16), 10, dispatch)
	result, err := ag.Run(context.Background(), "session-codebase-intent", "explain codebase")
	if err != nil {
		t.Fatalf("Run returned error: %v", err)
	}
	if result.Output != "internal/ and frontend/ provider test summary" {
		t.Fatalf("output = %q, want final summary", result.Output)
	}
	if step != 3 {
		t.Fatalf("dispatch steps = %d, want 3", step)
	}
}

func TestRunAbortsAfterConsecutiveToolErrorSteps(t *testing.T) {
	registry := tools.NewRegistry()

	step := 0
	dispatch := func(ctx context.Context, req *MessagesRequest) (<-chan *MessagesResponse, error) {
		step++
		ch := make(chan *MessagesResponse, 1)
		ch <- &MessagesResponse{Content: []ContentBlock{{
			Type: "tool_use",
			ID:   "toolu_err",
			Name: "missing_tool",
			Input: map[string]any{
				"n": step,
			},
		}}}
		close(ch)
		return ch, nil
	}

	ag := NewAgent(registry, NewBufferMemory(16), 10, dispatch)
	_, err := ag.Run(context.Background(), "session-errors", "do something")
	if err == nil {
		t.Fatal("expected consecutive tool-error guard to abort")
	}
	if !strings.Contains(err.Error(), "consecutive tool-error steps") {
		t.Fatalf("unexpected error: %v", err)
	}
}
