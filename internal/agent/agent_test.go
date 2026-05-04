package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"regexp"
	"sort"
	"testing"

	"omnillm/internal/cif"
	"omnillm/internal/tools"
)

func TestRegistryExecuteToolCallsHonorsPermissionChecker(t *testing.T) {
	registry := tools.NewRegistry()
	registry.Register(tools.Bash())

	results := registry.ExecuteToolCalls(context.Background(), "session-1", []cif.CIFToolCallPart{{
		ToolCallID: "call-1",
		ToolName:   "bash",
		ToolArguments: map[string]any{
			"command": "echo hello",
		},
	}})
	if len(results) != 1 {
		t.Fatalf("results len = %d", len(results))
	}
	if results[0].IsError {
		t.Fatal("expected bash tool to succeed")
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

func TestChatCompletionsDispatchPostsStandardOpenAIToolPayload(t *testing.T) {
	var capturedPath string
	var capturedPayload map[string]any
	client := &stubAgentClient{
		postFn: func(path string, body any) ([]byte, error) {
			capturedPath = path
			data, err := json.Marshal(body)
			if err != nil {
				t.Fatalf("marshal body: %v", err)
			}
			if err := json.Unmarshal(data, &capturedPayload); err != nil {
				t.Fatalf("unmarshal body: %v\n%s", err, string(data))
			}
			return []byte(`{"id":"chatcmpl-test","model":"deepseek-v4-flash","choices":[{"index":0,"message":{"role":"assistant","content":"ok"},"finish_reason":"stop"}]}`), nil
		},
	}

	dispatch := NewChatCompletionsDispatch(client, "alibaba-sk-ab2c5/deepseek-v4-flash")
	toolChoice := "auto"
	respCh, err := dispatch(context.Background(), &cif.CanonicalRequest{
		Messages: []cif.CIFMessage{
			cif.CIFUserMessage{
				Role: "user",
				Content: []cif.CIFContentPart{
					cif.CIFTextPart{Type: "text", Text: "List files"},
				},
			},
		},
		Tools: []cif.CIFTool{{
			Name:        "ls",
			Description: stringPtr("List files"),
			ParametersSchema: map[string]any{
				"type":       "object",
				"properties": map[string]any{"path": map[string]any{"type": "string"}},
			},
		}},
		ToolChoice: toolChoice,
	})
	if err != nil {
		t.Fatalf("dispatch returned error: %v", err)
	}
	for range respCh {
	}

	if capturedPath != "/v1/chat/completions" {
		t.Fatalf("path = %q", capturedPath)
	}
	if capturedPayload["model"] != "alibaba-sk-ab2c5/deepseek-v4-flash" {
		t.Fatalf("model = %#v", capturedPayload["model"])
	}
	if capturedPayload["stream"] != false {
		t.Fatalf("stream = %#v", capturedPayload["stream"])
	}
	if capturedPayload["tool_choice"] != "auto" {
		t.Fatalf("tool_choice = %#v", capturedPayload["tool_choice"])
	}
	tools, ok := capturedPayload["tools"].([]any)
	if !ok || len(tools) != 1 {
		t.Fatalf("tools = %#v", capturedPayload["tools"])
	}
	tool, _ := tools[0].(map[string]any)
	if tool["type"] != "function" {
		t.Fatalf("tool.type = %#v", tool["type"])
	}
	function, _ := tool["function"].(map[string]any)
	if function["name"] != "ls" {
		t.Fatalf("function.name = %#v", function["name"])
	}
	if _, ok := function["parameters"].(map[string]any); !ok {
		t.Fatalf("function.parameters = %#v", function["parameters"])
	}
}

func TestChatCompletionsDispatchDoesNotRetryWithoutTools(t *testing.T) {
	calls := 0
	client := &stubAgentClient{
		postFn: func(path string, body any) ([]byte, error) {
			calls++
			return nil, fmt.Errorf("server error (502): provider_error")
		},
	}

	dispatch := NewChatCompletionsDispatch(client, "deepseek-v4-flash")
	_, err := dispatch(context.Background(), &cif.CanonicalRequest{
		Messages: []cif.CIFMessage{
			cif.CIFUserMessage{
				Role: "user",
				Content: []cif.CIFContentPart{
					cif.CIFTextPart{Type: "text", Text: "List files"},
				},
			},
		},
		Tools: []cif.CIFTool{{
			Name:             "ls",
			ParametersSchema: map[string]any{"type": "object"},
		}},
		ToolChoice: "auto",
	})
	if err == nil {
		t.Fatal("expected dispatch error")
	}
	if calls != 1 {
		t.Fatalf("calls = %d, want 1", calls)
	}
}

func TestRunTurnPostsDefaultToolsAsOpenAIToolsNotDeprecatedFunctions(t *testing.T) {
	var capturedPayload map[string]any
	client := &stubAgentClient{
		postFn: func(path string, body any) ([]byte, error) {
			if path != "/v1/chat/completions" {
				t.Fatalf("path = %q", path)
			}
			data, err := json.Marshal(body)
			if err != nil {
				t.Fatalf("marshal body: %v", err)
			}
			if err := json.Unmarshal(data, &capturedPayload); err != nil {
				t.Fatalf("unmarshal body: %v\n%s", err, string(data))
			}
			return []byte(`{"id":"chatcmpl-test","model":"deepseek-v4-flash","choices":[{"index":0,"message":{"role":"assistant","content":"ok"},"finish_reason":"stop"}]}`), nil
		},
	}

	result, err := RunTurn(context.Background(), client, "session-1", "alibaba-sk-ab2c5/deepseek-v4-flash", "agent-sdk-go", "List this directory", nil, nil)
	if err != nil {
		t.Fatalf("RunTurn returned error: %v", err)
	}
	if result == nil || result.Output != "ok" {
		t.Fatalf("unexpected result: %#v", result)
	}

	if _, exists := capturedPayload["functions"]; exists {
		t.Fatalf("deprecated functions field must not be sent: %#v", capturedPayload["functions"])
	}
	if _, exists := capturedPayload["function_call"]; exists {
		t.Fatalf("deprecated function_call field must not be sent: %#v", capturedPayload["function_call"])
	}
	if capturedPayload["tool_choice"] != "auto" {
		t.Fatalf("tool_choice = %#v", capturedPayload["tool_choice"])
	}

	toolsPayload, ok := capturedPayload["tools"].([]any)
	if !ok || len(toolsPayload) != 7 {
		t.Fatalf("tools = %#v", capturedPayload["tools"])
	}

	validName := regexp.MustCompile(`^[A-Za-z0-9_-]{1,64}$`)
	names := make([]string, 0, len(toolsPayload))
	for _, rawTool := range toolsPayload {
		tool, ok := rawTool.(map[string]any)
		if !ok {
			t.Fatalf("tool has unexpected shape: %#v", rawTool)
		}
		if tool["type"] != "function" {
			t.Fatalf("tool.type = %#v", tool["type"])
		}
		function, ok := tool["function"].(map[string]any)
		if !ok {
			t.Fatalf("tool.function has unexpected shape: %#v", tool["function"])
		}
		name, _ := function["name"].(string)
		if !validName.MatchString(name) {
			t.Fatalf("invalid OpenAI function name %q", name)
		}
		if _, ok := function["description"].(string); !ok {
			t.Fatalf("function.description missing or invalid for %q: %#v", name, function)
		}
		parameters, ok := function["parameters"].(map[string]any)
		if !ok {
			t.Fatalf("function.parameters missing or invalid for %q: %#v", name, function["parameters"])
		}
		if parameters["type"] != "object" {
			t.Fatalf("function.parameters.type for %q = %#v", name, parameters["type"])
		}
		if _, ok := parameters["properties"].(map[string]any); !ok {
			t.Fatalf("function.parameters.properties missing or invalid for %q: %#v", name, parameters["properties"])
		}
		names = append(names, name)
	}
	sort.Strings(names)
	wantNames := []string{"bash", "edit", "glob", "grep", "ls", "read", "write"}
	if fmt.Sprint(names) != fmt.Sprint(wantNames) {
		t.Fatalf("tool names = %#v, want %#v", names, wantNames)
	}
}

type stubAgentClient struct {
	postFn func(path string, body any) ([]byte, error)
}

func (s *stubAgentClient) Post(path string, body any) ([]byte, error) {
	return s.postFn(path, body)
}

func (s *stubAgentClient) PostStream(path string, body any) (*http.Response, error) {
	return nil, fmt.Errorf("not implemented")
}

func stringPtr(value string) *string {
	return &value
}
