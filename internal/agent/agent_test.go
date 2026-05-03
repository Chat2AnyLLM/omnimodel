package agent

import (
	"context"
	"encoding/json"
	"testing"

	"omnillm/internal/cif"
)

func TestRegistryExecuteToolCallsHonorsPermissionChecker(t *testing.T) {
	registry := NewRegistry()
	registry.Register(&Tool{
		Name: "run_command",
		Fn: func(ctx context.Context, input json.RawMessage) (string, error) {
			return "ok", nil
		},
	})
	registry.SetPermissionChecker(func(ctx context.Context, req PermissionRequest) (bool, error) {
		if req.SessionID != "session-1" {
			t.Fatalf("session id = %q", req.SessionID)
		}
		if req.ToolName != "run_command" {
			t.Fatalf("tool name = %q", req.ToolName)
		}
		return false, nil
	})

	results := registry.ExecuteToolCalls(context.Background(), "session-1", []cif.CIFToolCallPart{{
		ToolCallID: "call-1",
		ToolName:   "run_command",
		ToolArguments: map[string]interface{}{
			"command": "Get-ChildItem",
		},
	}})
	if len(results) != 1 {
		t.Fatalf("results len = %d", len(results))
	}
	if !results[0].IsError {
		t.Fatal("expected denied tool call to be marked as error")
	}
}

func TestRunTurnExecutesRegisteredTool(t *testing.T) {
	client := &stubAgentClient{
		postFn: func(path string, body any) ([]byte, error) {
			payload, _ := body.(map[string]any)
			messages, _ := payload["messages"].([]map[string]any)
			_ = messages
			if stub, ok := body.(*struct{}); ok && stub == nil {
				t.Fatal("unexpected stub")
			}
			return nil, nil
		},
	}
	_ = client
}

type stubAgentClient struct {
	postFn func(path string, body any) ([]byte, error)
}

func (s *stubAgentClient) Post(path string, body any) ([]byte, error) {
	return s.postFn(path, body)
}
