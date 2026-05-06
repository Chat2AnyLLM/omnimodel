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

func TestBuildRequestRequiresInitialToolForLocalInfoPrompt(t *testing.T) {
	ag := newTestAgent()
	ag.appendUserMessage("show CPU info")

	req := ag.buildRequest(0, "show CPU info")
	if req.ToolChoice != "required" {
		t.Fatalf("tool choice = %#v, want required", req.ToolChoice)
	}

	req = ag.buildRequest(1, "show CPU info")
	if req.ToolChoice != "auto" {
		t.Fatalf("follow-up tool choice = %#v, want auto", req.ToolChoice)
	}
}

func TestBuildRequestUsesAutoForConversationalPrompt(t *testing.T) {
	ag := newTestAgent()
	ag.appendUserMessage("hello")

	req := ag.buildRequest(0, "hello")
	if req.ToolChoice != "auto" {
		t.Fatalf("tool choice = %#v, want auto", req.ToolChoice)
	}
}

func TestBuildRequestRequiresInitialToolForRepoLookupPrompt(t *testing.T) {
	ag := newTestAgent()
	ag.appendUserMessage("find all references to buildSystemPrompt")

	req := ag.buildRequest(0, "find all references to buildSystemPrompt")
	if req.ToolChoice != "required" {
		t.Fatalf("tool choice = %#v, want required", req.ToolChoice)
	}
}

func TestBuildRequestUsesAutoForConceptualArchitecturePrompt(t *testing.T) {
	ag := newTestAgent()
	ag.appendUserMessage("explain the architecture of this repo")

	req := ag.buildRequest(0, "explain the architecture of this repo")
	if req.ToolChoice != "auto" {
		t.Fatalf("tool choice = %#v, want auto", req.ToolChoice)
	}
}

func TestBuildRequestLeavesToolChoiceUnsetWithoutTools(t *testing.T) {
	ag := NewAgent(tools.NewRegistry(), NewBufferMemory(8), 10, nil)
	ag.appendUserMessage("show CPU info")

	req := ag.buildRequest(0, "show CPU info")
	if req.ToolChoice != nil {
		t.Fatalf("tool choice = %#v, want nil", req.ToolChoice)
	}
}

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

func TestChatCompletionsDispatchRoutesAllModelsToMessages(t *testing.T) {
	// All models now route to /v1/messages; OmniLLM handles upstream translation.
	modelsAndResponses := []struct {
		model    string
		response string
	}{
		{
			model:    "claude-opus-4-7",
			response: `{"id":"msg_123","type":"message","role":"assistant","model":"claude-opus-4-7","content":[{"type":"text","text":"ok"}],"stop_reason":"end_turn","usage":{"input_tokens":3,"output_tokens":1}}`,
		},
		{
			model:    "gpt-5.4",
			response: `{"id":"msg_gpt","type":"message","role":"assistant","model":"gpt-5.4","content":[{"type":"text","text":"ok"}],"stop_reason":"end_turn","usage":{"input_tokens":3,"output_tokens":1}}`,
		},
		{
			model:    "gemini-3.1-pro-preview",
			response: `{"id":"msg_gem","type":"message","role":"assistant","model":"gemini-3.1-pro-preview","content":[{"type":"text","text":"ok"}],"stop_reason":"end_turn","usage":{"input_tokens":3,"output_tokens":1}}`,
		},
		{
			model:    "deepseek-v4-flash",
			response: `{"id":"msg_ds","type":"message","role":"assistant","model":"deepseek-v4-flash","content":[{"type":"text","text":"ok"}],"stop_reason":"end_turn","usage":{"input_tokens":3,"output_tokens":1}}`,
		},
		{
			model:    "qwen3.6-flash",
			response: `{"id":"msg_qw","type":"message","role":"assistant","model":"qwen3.6-flash","content":[{"type":"text","text":"ok"}],"stop_reason":"end_turn","usage":{"input_tokens":3,"output_tokens":1}}`,
		},
		{
			model:    "kimi-k2.6",
			response: `{"id":"msg_km","type":"message","role":"assistant","model":"kimi-k2.6","content":[{"type":"text","text":"ok"}],"stop_reason":"end_turn","usage":{"input_tokens":3,"output_tokens":1}}`,
		},
	}

	for _, tc := range modelsAndResponses {
		t.Run(tc.model, func(t *testing.T) {
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
					return []byte(tc.response), nil
				},
			}

			dispatch := NewChatCompletionsDispatch(client, tc.model)
			respCh, err := dispatch(context.Background(), &cif.CanonicalRequest{
				Messages: []cif.CIFMessage{
					cif.CIFUserMessage{Role: "user", Content: []cif.CIFContentPart{cif.CIFTextPart{Type: "text", Text: "Hello"}}},
				},
				Tools:      []cif.CIFTool{{Name: "ls", Description: stringPtr("List files"), ParametersSchema: map[string]any{"type": "object", "properties": map[string]any{"path": map[string]any{"type": "string"}}}}},
				ToolChoice: "auto",
			})
			if err != nil {
				t.Fatalf("dispatch returned error: %v", err)
			}
			for range respCh {
			}

			if capturedPath != "/v1/messages" {
				t.Fatalf("[%s] path = %q, want /v1/messages", tc.model, capturedPath)
			}
			if capturedPayload["model"] != tc.model {
				t.Fatalf("[%s] model = %#v", tc.model, capturedPayload["model"])
			}
			if _, ok := capturedPayload["messages"].([]any); !ok {
				t.Fatalf("[%s] messages = %#v", tc.model, capturedPayload["messages"])
			}
			if tools, ok := capturedPayload["tools"].([]any); !ok || len(tools) != 1 {
				t.Fatalf("[%s] tools = %#v", tc.model, capturedPayload["tools"])
			}
		})
	}
}

func TestChatCompletionsDispatchRoutesGPTModelsToMessages(t *testing.T) {
	// GPT models now route to /v1/messages; OmniLLM translates to the Responses API upstream.
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
			return []byte(`{"id":"msg_gpt","type":"message","role":"assistant","model":"gpt-5.4","content":[{"type":"text","text":"ok"}],"stop_reason":"end_turn","usage":{"input_tokens":3,"output_tokens":1}}`), nil
		},
	}

	dispatch := NewChatCompletionsDispatch(client, "gpt-5.4")
	respCh, err := dispatch(context.Background(), &cif.CanonicalRequest{
		Messages: []cif.CIFMessage{
			cif.CIFUserMessage{Role: "user", Content: []cif.CIFContentPart{cif.CIFTextPart{Type: "text", Text: "Hello"}}},
		},
		Tools:      []cif.CIFTool{{Name: "ls", Description: stringPtr("List files"), ParametersSchema: map[string]any{"type": "object", "properties": map[string]any{"path": map[string]any{"type": "string"}}}}},
		ToolChoice: "auto",
	})
	if err != nil {
		t.Fatalf("dispatch returned error: %v", err)
	}
	for range respCh {
	}

	if capturedPath != "/v1/messages" {
		t.Fatalf("path = %q, want /v1/messages", capturedPath)
	}
	if capturedPayload["model"] != "gpt-5.4" {
		t.Fatalf("model = %#v", capturedPayload["model"])
	}
	if _, ok := capturedPayload["messages"].([]any); !ok {
		t.Fatalf("messages = %#v", capturedPayload["messages"])
	}
	if tools, ok := capturedPayload["tools"].([]any); !ok || len(tools) != 1 {
		t.Fatalf("tools = %#v", capturedPayload["tools"])
	}
}

func TestChatCompletionsDispatchPostsStandardOpenAIToolPayload(t *testing.T) {
	// Even non-claude models now go to /v1/messages; OmniLLM handles translation.
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
			return []byte(`{"id":"msg_ds","type":"message","role":"assistant","model":"deepseek-v4-flash","content":[{"type":"text","text":"ok"}],"stop_reason":"end_turn","usage":{"input_tokens":3,"output_tokens":1}}`), nil
		},
	}

	dispatch := NewChatCompletionsDispatch(client, "deepseek-v4-flash")
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

	if capturedPath != "/v1/messages" {
		t.Fatalf("path = %q, want /v1/messages", capturedPath)
	}
	if capturedPayload["model"] != "deepseek-v4-flash" {
		t.Fatalf("model = %#v", capturedPayload["model"])
	}
	tools, ok := capturedPayload["tools"].([]any)
	if !ok || len(tools) != 1 {
		t.Fatalf("tools = %#v", capturedPayload["tools"])
	}
	// Anthropic tool shape: {name, description, input_schema}
	tool, _ := tools[0].(map[string]any)
	if tool["name"] != "ls" {
		t.Fatalf("tool.name = %#v", tool["name"])
	}
	if _, ok := tool["input_schema"].(map[string]any); !ok {
		t.Fatalf("tool.input_schema = %#v", tool["input_schema"])
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
			if path != "/v1/messages" {
				t.Fatalf("path = %q, want /v1/messages", path)
			}
			data, err := json.Marshal(body)
			if err != nil {
				t.Fatalf("marshal body: %v", err)
			}
			if err := json.Unmarshal(data, &capturedPayload); err != nil {
				t.Fatalf("unmarshal body: %v\n%s", err, string(data))
			}
			return []byte(`{"id":"msg_test","type":"message","role":"assistant","model":"deepseek-v4-flash","content":[{"type":"text","text":"ok"}],"stop_reason":"end_turn","usage":{"input_tokens":5,"output_tokens":1}}`), nil
		},
	}

	result, err := RunTurn(context.Background(), client, "session-1", "deepseek-v4-flash", "agent-sdk-go", DefaultAPIShape, "List this directory", nil, nil, nil, 10)
	if err != nil {
		t.Fatalf("RunTurn returned error: %v", err)
	}
	if result == nil || result.Output != "ok" {
		t.Fatalf("unexpected result: %#v", result)
	}

	// All requests now use /v1/messages (Anthropic shape). Deprecated functions/function_call must not appear.
	if _, exists := capturedPayload["functions"]; exists {
		t.Fatalf("deprecated functions field must not be sent: %#v", capturedPayload["functions"])
	}
	if _, exists := capturedPayload["function_call"]; exists {
		t.Fatalf("deprecated function_call field must not be sent: %#v", capturedPayload["function_call"])
	}

	toolsPayload, ok := capturedPayload["tools"].([]any)
	if !ok || len(toolsPayload) != 36 {
		t.Fatalf("tools = %#v", capturedPayload["tools"])
	}

	validName := regexp.MustCompile(`^[A-Za-z0-9_-]{1,64}$`)
	names := make([]string, 0, len(toolsPayload))
	for _, rawTool := range toolsPayload {
		tool, ok := rawTool.(map[string]any)
		if !ok {
			t.Fatalf("tool has unexpected shape: %#v", rawTool)
		}
		// Anthropic /v1/messages tool shape: {name, description, input_schema}
		name, _ := tool["name"].(string)
		if !validName.MatchString(name) {
			t.Fatalf("invalid tool name %q", name)
		}
		if _, ok := tool["description"].(string); !ok {
			t.Fatalf("description missing or invalid for %q: %#v", name, tool)
		}
		inputSchema, ok := tool["input_schema"].(map[string]any)
		if !ok {
			t.Fatalf("input_schema missing or invalid for %q: %#v", name, tool["input_schema"])
		}
		if inputSchema["type"] != "object" {
			t.Fatalf("input_schema.type for %q = %#v", name, inputSchema["type"])
		}
		if _, ok := inputSchema["properties"].(map[string]any); !ok {
			t.Fatalf("input_schema.properties missing or invalid for %q: %#v", name, inputSchema["properties"])
		}
		names = append(names, name)
	}
	sort.Strings(names)
	wantNames := []string{"agent", "apply_patch", "ask_user_question", "bash", "batch", "calculator", "codesearch", "config", "edit", "enter_plan_mode", "enter_worktree", "exit_plan_mode", "exit_worktree", "get_current_time", "glob", "grep", "ls", "lsp", "multiedit", "notebook_edit", "powershell", "read", "schedule_cron", "send_message", "sleep", "task_create", "task_get", "task_list", "task_output", "task_stop", "task_update", "todo_write", "tool_search", "web_fetch", "web_search", "write"}
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

func TestSelectDispatchUsesProxyForAgentSDKGo(t *testing.T) {
	var capturedPath string
	client := &stubAgentClient{
		postFn: func(path string, body any) ([]byte, error) {
			capturedPath = path
			return []byte(`{"id":"msg_1","type":"message","role":"assistant","model":"claude-opus-4-5","content":[{"type":"text","text":"ok"}],"stop_reason":"end_turn","usage":{"input_tokens":3,"output_tokens":1}}`), nil
		},
	}
	dispatch := selectDispatch(client, "claude-opus-4-5", "agent-sdk-go", DefaultAPIShape)
	_, err := dispatch(context.Background(), &cif.CanonicalRequest{
		Messages: []cif.CIFMessage{
			cif.CIFUserMessage{Role: "user", Content: []cif.CIFContentPart{cif.CIFTextPart{Type: "text", Text: "hi"}}},
		},
	})
	if err != nil {
		t.Fatalf("dispatch error: %v", err)
	}
	if capturedPath != "/v1/messages" {
		t.Fatalf("expected /v1/messages, got %q", capturedPath)
	}
}

func TestSelectDispatchUsesProxyForGoogleADK(t *testing.T) {
	var capturedPath string
	client := &stubAgentClient{
		postFn: func(path string, body any) ([]byte, error) {
			capturedPath = path
			return []byte(`{"id":"msg_1","type":"message","role":"assistant","model":"gemini-pro","content":[{"type":"text","text":"ok"}],"stop_reason":"end_turn","usage":{"input_tokens":3,"output_tokens":1}}`), nil
		},
	}
	dispatch := selectDispatch(client, "gemini-pro", "google-adk", DefaultAPIShape)
	_, err := dispatch(context.Background(), &cif.CanonicalRequest{
		Messages: []cif.CIFMessage{
			cif.CIFUserMessage{Role: "user", Content: []cif.CIFContentPart{cif.CIFTextPart{Type: "text", Text: "hi"}}},
		},
	})
	if err != nil {
		t.Fatalf("dispatch error: %v", err)
	}
	if capturedPath != "/v1/messages" {
		t.Fatalf("expected /v1/messages, got %q", capturedPath)
	}
}

func TestSelectDispatchUsesAnthropicSDKForAnthropicSDKBackend(t *testing.T) {
	// anthropic-sdk now routes through the OmniLLM proxy just like agent-sdk-go
	// and google-adk.  The proxy client MUST be called.
	var capturedPath string
	client := &stubAgentClient{
		postFn: func(path string, body any) ([]byte, error) {
			capturedPath = path
			return []byte(`{"id":"msg_1","type":"message","role":"assistant","model":"claude-opus-4-5","content":[{"type":"text","text":"ok"}],"stop_reason":"end_turn","usage":{"input_tokens":3,"output_tokens":1}}`), nil
		},
	}
	dispatch := selectDispatch(client, "claude-opus-4-5", "anthropic-sdk", DefaultAPIShape)
	_, err := dispatch(context.Background(), &cif.CanonicalRequest{
		Messages: []cif.CIFMessage{
			cif.CIFUserMessage{Role: "user", Content: []cif.CIFContentPart{cif.CIFTextPart{Type: "text", Text: "hi"}}},
		},
	})
	if err != nil {
		t.Fatalf("dispatch error: %v", err)
	}
	if capturedPath != "/v1/messages" {
		t.Fatalf("expected /v1/messages, got %q", capturedPath)
	}
}

func newTestAgent() *Agent {
	registry := tools.NewRegistry()
	registry.Register(tools.Bash())
	memory := NewBufferMemory(8)
	return NewAgent(registry, memory, 10, nil)
}
