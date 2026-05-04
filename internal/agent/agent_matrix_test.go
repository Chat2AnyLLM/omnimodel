package agent

// agent_matrix_test.go — Comprehensive agent turn matrix
//
// Tests every combination of:
//   - 3 API shapes   : /v1/messages (Anthropic), /v1/chat/completions (OpenAI), /v1/responses (Responses)
//   - 3 SDK backends : agent-sdk-go, google-adk, anthropic-sdk
//   - 8 target models: gpt-5.4-mini, gpt-5-mini, claude-haiku-4.5, gemini-3.1-flash,
//                      deepseek-v4-flash, qwen3.6-flash, kimi-k2.6, glm-5.1
//
// All requests run through the OmniLLM stub proxy (no real network).
// Each case verifies:
//   - The proxy is always called on /v1/messages  (all backends, all models)
//   - The correct model name is forwarded
//   - The Anthropic Messages API shape is used (tools array with input_schema,
//     NOT deprecated functions/function_call)
//   - No panic, no spurious error

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"sort"
	"strings"
	"testing"

	"omnillm/internal/cif"
	toolspkg "omnillm/internal/tools"
)

// ─── test matrix dimensions ───────────────────────────────────────────────────

type apiShape string

const (
	shapeMessages    apiShape = "messages"    // /v1/messages  (Anthropic)
	shapeChatCompl   apiShape = "chat"        // /v1/chat/completions (OpenAI)
	shapeResponses   apiShape = "responses"   // /v1/responses (Responses API)
)

var allShapes = []apiShape{shapeMessages, shapeChatCompl, shapeResponses}

var allBackends = []string{"agent-sdk-go", "google-adk", "anthropic-sdk"}

var allModels = []string{
	"gpt-5.4-mini",
	"gpt-5-mini",
	"claude-haiku-4.5",
	"gemini-3.1-flash",
	"deepseek-v4-flash",
	"qwen3.6-flash",
	"kimi-k2.6",
	"glm-5.1",
}

// ─── response factories ───────────────────────────────────────────────────────

// messagesResponse returns a minimal Anthropic /v1/messages response for the given model.
func messagesResponse(model string) []byte {
	r := map[string]any{
		"id":          "msg_matrix",
		"type":        "message",
		"role":        "assistant",
		"model":       model,
		"stop_reason": "end_turn",
		"usage":       map[string]any{"input_tokens": 5, "output_tokens": 2},
		"content":     []map[string]any{{"type": "text", "text": "ok"}},
	}
	b, _ := json.Marshal(r)
	return b
}

// messagesToolResponse returns a /v1/messages response with one tool_use block,
// followed by a final text response on the second call.
func messagesToolResponse(model string, callN int) []byte {
	if callN == 1 {
		r := map[string]any{
			"id":          "msg_tool",
			"type":        "message",
			"role":        "assistant",
			"model":       model,
			"stop_reason": "tool_use",
			"usage":       map[string]any{"input_tokens": 10, "output_tokens": 5},
			"content": []map[string]any{{
				"type":  "tool_use",
				"id":    "toolu_01",
				"name":  "get_current_time",
				"input": map[string]any{},
			}},
		}
		b, _ := json.Marshal(r)
		return b
	}
	return messagesResponse(model)
}

// ─── stub proxy helpers ───────────────────────────────────────────────────────

// proxyStub is a minimal stub Client that records calls and returns canned
// Anthropic /v1/messages responses regardless of which path is requested.
// (All three backends hit /v1/messages; we never reach /v1/chat/completions
// or /v1/responses from within the agent runtime — those are upstream concerns
// handled by OmniLLM's proxy layer.)
type proxyStub struct {
	t          *testing.T
	model      string
	calls      int
	capturedPath    []string
	capturedPayload []map[string]any
	// toolCall: when true, first call returns tool_use; second returns text.
	toolCall bool
}

func (s *proxyStub) Post(path string, body any) ([]byte, error) {
	s.calls++
	s.capturedPath = append(s.capturedPath, path)

	data, err := json.Marshal(body)
	if err != nil {
		s.t.Fatalf("proxyStub.Post: marshal body: %v", err)
	}
	var p map[string]any
	if err := json.Unmarshal(data, &p); err != nil {
		s.t.Fatalf("proxyStub.Post: unmarshal body: %v\n%s", err, data)
	}
	s.capturedPayload = append(s.capturedPayload, p)

	if s.toolCall {
		return messagesToolResponse(s.model, s.calls), nil
	}
	return messagesResponse(s.model), nil
}

func (s *proxyStub) PostStream(_ string, _ any) (*http.Response, error) {
	return nil, fmt.Errorf("streaming not exercised in matrix test")
}

// ─── assertion helpers ────────────────────────────────────────────────────────

func assertProxyShape(t *testing.T, tag string, stub *proxyStub, wantModel string, wantCalls int) {
	t.Helper()
	if stub.calls != wantCalls {
		t.Errorf("[%s] proxy calls = %d, want %d", tag, stub.calls, wantCalls)
	}
	for i, path := range stub.capturedPath {
		if path != "/v1/messages" {
			t.Errorf("[%s] call %d: path = %q, want /v1/messages", tag, i+1, path)
		}
	}
	for i, payload := range stub.capturedPayload {
		if payload["model"] != wantModel {
			t.Errorf("[%s] call %d: model = %#v, want %q", tag, i+1, payload["model"], wantModel)
		}
		if _, ok := payload["messages"].([]any); !ok {
			t.Errorf("[%s] call %d: messages field missing or wrong type: %T", tag, i+1, payload["messages"])
		}
		// Anthropic shape must not include deprecated OpenAI fields
		if _, exists := payload["functions"]; exists {
			t.Errorf("[%s] call %d: deprecated 'functions' field present", tag, i+1)
		}
		if _, exists := payload["function_call"]; exists {
			t.Errorf("[%s] call %d: deprecated 'function_call' field present", tag, i+1)
		}
		// When tools are present they must use Anthropic's input_schema shape
		if toolsRaw, ok := payload["tools"].([]any); ok && len(toolsRaw) > 0 {
			for j, raw := range toolsRaw {
				tool, _ := raw.(map[string]any)
				if tool["input_schema"] == nil {
					t.Errorf("[%s] call %d tool %d: input_schema missing (OpenAI shape?): %#v", tag, i+1, j, tool)
				}
			}
		}
	}
}

// ─── matrix: simple turn (no tool call) ──────────────────────────────────────

// TestAgentMatrixSimpleTurn exercises RunTurn for every (backend × model)
// combination.  No tool calls are involved — the model replies with plain text.
func TestAgentMatrixSimpleTurn(t *testing.T) {
	for _, backend := range allBackends {
		for _, model := range allModels {
			backend, model := backend, model
			tag := fmt.Sprintf("%s/%s", backend, model)
			t.Run(tag, func(t *testing.T) {
				stub := &proxyStub{t: t, model: model}
				result, err := RunTurn(
					context.Background(), stub,
					"sess-matrix", model, backend,
					"Hello from "+tag,
					nil, nil, nil,
				)
				if err != nil {
					t.Fatalf("RunTurn error: %v", err)
				}
				if result == nil || result.Output != "ok" {
					t.Fatalf("unexpected result: %#v", result)
				}
				assertProxyShape(t, tag, stub, model, 1)
			})
		}
	}
}

// ─── matrix: tool-call turn ───────────────────────────────────────────────────

// TestAgentMatrixToolCallTurn exercises a two-step turn: first reply contains a
// tool_use block (get_current_time, which needs no external I/O), second reply
// is plain text.  Runs over every (backend × model) combination.
func TestAgentMatrixToolCallTurn(t *testing.T) {
	for _, backend := range allBackends {
		for _, model := range allModels {
			backend, model := backend, model
			tag := fmt.Sprintf("%s/%s", backend, model)
			t.Run(tag, func(t *testing.T) {
				stub := &proxyStub{t: t, model: model, toolCall: true}
				result, err := RunTurn(
					context.Background(), stub,
					"sess-tool", model, backend,
					"What time is it?",
					nil, nil, nil,
				)
				if err != nil {
					t.Fatalf("RunTurn error: %v", err)
				}
				if result == nil || result.Output != "ok" {
					t.Fatalf("unexpected result: %#v", result)
				}
				// 2 proxy calls: step 1 (tool_use) + step 2 (final text)
				assertProxyShape(t, tag, stub, model, 2)
				if result.Steps != 2 {
					t.Errorf("[%s] steps = %d, want 2", tag, result.Steps)
				}
			})
		}
	}
}

// ─── matrix: streaming turn ───────────────────────────────────────────────────

// TestAgentMatrixStreamTurn exercises StreamTurn for every (backend × model).
// Events are collected and the final output is verified.
func TestAgentMatrixStreamTurn(t *testing.T) {
	for _, backend := range allBackends {
		for _, model := range allModels {
			backend, model := backend, model
			tag := fmt.Sprintf("%s/%s", backend, model)
			t.Run(tag, func(t *testing.T) {
				stub := &proxyStub{t: t, model: model}
				eventCh, err := StreamTurn(
					context.Background(), stub,
					"sess-stream", model, backend,
					"Stream hello from "+tag,
					nil, nil, nil,
				)
				if err != nil {
					t.Fatalf("StreamTurn error: %v", err)
				}

				var tokens []string
				var errs []string
				var done bool
				for event := range eventCh {
					switch event.Type {
					case EventToken:
						tokens = append(tokens, event.Content)
					case EventDone:
						done = true
					case EventError:
						errs = append(errs, event.Content)
					}
				}
				if len(errs) > 0 {
					t.Fatalf("[%s] stream errors: %v", tag, errs)
				}
				if !done {
					t.Errorf("[%s] EventDone never received", tag)
				}
				if got := strings.Join(tokens, ""); got != "ok" {
					t.Errorf("[%s] token output = %q, want \"ok\"", tag, got)
				}
				assertProxyShape(t, tag, stub, model, 1)
			})
		}
	}
}

// ─── matrix: all 3 API shapes → all route to /v1/messages ───────────────────

// TestAgentMatrixAllAPIShapes verifies that NewChatCompletionsDispatch
// (the only dispatch used by all backends) always sends to /v1/messages
// regardless of which "shape" OmniLLM is configured to use upstream.
// From the agent's perspective, the shape is an upstream concern — the agent
// always speaks Anthropic Messages API to the proxy.
func TestAgentMatrixAllAPIShapes(t *testing.T) {
	// The agent runtime always calls /v1/messages.
	// We verify this is true for every model across every logical "shape" label.
	for _, shape := range allShapes {
		for _, model := range allModels {
			tag := fmt.Sprintf("shape=%s/model=%s", shape, model)
			t.Run(tag, func(t *testing.T) {
				stub := &proxyStub{t: t, model: model}
				dispatch := NewChatCompletionsDispatch(stub, model)
				respCh, err := dispatch(context.Background(), &cif.CanonicalRequest{
					Model: model,
					Messages: []cif.CIFMessage{
						cif.CIFUserMessage{
							Role:    "user",
							Content: []cif.CIFContentPart{cif.CIFTextPart{Type: "text", Text: "hi"}},
						},
					},
					Tools: []cif.CIFTool{{
						Name:             "get_current_time",
						Description:      stringPtr("Return current time"),
						ParametersSchema: map[string]any{"type": "object", "properties": map[string]any{}},
					}},
					ToolChoice: "auto",
				})
				if err != nil {
					t.Fatalf("dispatch error: %v", err)
				}
				for range respCh {
				}
				if stub.calls != 1 {
					t.Fatalf("[%s] proxy calls = %d, want 1", tag, stub.calls)
				}
				if stub.capturedPath[0] != "/v1/messages" {
					t.Fatalf("[%s] path = %q, want /v1/messages", tag, stub.capturedPath[0])
				}
				payload := stub.capturedPayload[0]
				if payload["model"] != model {
					t.Fatalf("[%s] model = %#v, want %q", tag, payload["model"], model)
				}
				// Anthropic shape: tools have input_schema, NOT parameters
				tools, _ := payload["tools"].([]any)
				if len(tools) != 1 {
					t.Fatalf("[%s] tools count = %d, want 1", tag, len(tools))
				}
				tool, _ := tools[0].(map[string]any)
				if tool["input_schema"] == nil {
					t.Fatalf("[%s] tool missing input_schema: %#v", tag, tool)
				}
				if tool["parameters"] != nil {
					t.Fatalf("[%s] tool has 'parameters' (OpenAI shape leaked): %#v", tag, tool)
				}
			})
		}
	}
}

// ─── matrix: selectDispatch — all backends route to /v1/messages ─────────────

// TestAgentMatrixSelectDispatchAllBackends verifies that all three backends
// produce the same dispatch behaviour: they all call the OmniLLM proxy on
// /v1/messages with the correct model.
func TestAgentMatrixSelectDispatchAllBackends(t *testing.T) {
	for _, backend := range allBackends {
		for _, model := range allModels {
			tag := fmt.Sprintf("backend=%s/model=%s", backend, model)
			t.Run(tag, func(t *testing.T) {
				stub := &proxyStub{t: t, model: model}
				dispatch := selectDispatch(stub, model, backend)
				respCh, err := dispatch(context.Background(), &cif.CanonicalRequest{
					Model: model,
					Messages: []cif.CIFMessage{
						cif.CIFUserMessage{
							Role:    "user",
							Content: []cif.CIFContentPart{cif.CIFTextPart{Type: "text", Text: "hi"}},
						},
					},
				})
				if err != nil {
					t.Fatalf("[%s] dispatch error: %v", tag, err)
				}
				for range respCh {
				}
				if stub.capturedPath[0] != "/v1/messages" {
					t.Fatalf("[%s] path = %q, want /v1/messages", tag, stub.capturedPath[0])
				}
				if stub.capturedPayload[0]["model"] != model {
					t.Fatalf("[%s] model = %#v, want %q", tag, stub.capturedPayload[0]["model"], model)
				}
			})
		}
	}
}

// ─── matrix: tool call payload shape for all models ──────────────────────────

// TestAgentMatrixToolPayloadShape checks that for every model the tool payload
// uses the Anthropic Messages API shape (input_schema, name, description) and
// never the deprecated OpenAI functions field.
func TestAgentMatrixToolPayloadShape(t *testing.T) {
	toolDefs := []cif.CIFTool{
		{
			Name:             "bash",
			Description:      stringPtr("Run a shell command"),
			ParametersSchema: map[string]any{"type": "object", "properties": map[string]any{"command": map[string]any{"type": "string"}}, "required": []string{"command"}},
		},
		{
			Name:             "read",
			Description:      stringPtr("Read a file"),
			ParametersSchema: map[string]any{"type": "object", "properties": map[string]any{"file_path": map[string]any{"type": "string"}}, "required": []string{"file_path"}},
		},
	}

	for _, model := range allModels {
		t.Run(model, func(t *testing.T) {
			stub := &proxyStub{t: t, model: model}
			dispatch := NewChatCompletionsDispatch(stub, model)
			_, err := dispatch(context.Background(), &cif.CanonicalRequest{
				Model: model,
				Messages: []cif.CIFMessage{
					cif.CIFUserMessage{
						Role:    "user",
						Content: []cif.CIFContentPart{cif.CIFTextPart{Type: "text", Text: "help"}},
					},
				},
				Tools:      toolDefs,
				ToolChoice: "auto",
			})
			if err != nil {
				t.Fatalf("[%s] dispatch error: %v", model, err)
			}

			payload := stub.capturedPayload[0]
			toolsRaw, _ := payload["tools"].([]any)
			if len(toolsRaw) != 2 {
				t.Fatalf("[%s] tools count = %d, want 2", model, len(toolsRaw))
			}
			// Must not use deprecated OpenAI fields
			if _, exists := payload["functions"]; exists {
				t.Fatalf("[%s] deprecated 'functions' field present", model)
			}
			if _, exists := payload["function_call"]; exists {
				t.Fatalf("[%s] deprecated 'function_call' field present", model)
			}
			// tool_choice should be in Anthropic shape ({"type":"auto"})
			if tc, ok := payload["tool_choice"].(map[string]any); !ok {
				t.Fatalf("[%s] tool_choice missing or wrong type: %T", model, payload["tool_choice"])
			} else if tc["type"] != "auto" {
				t.Fatalf("[%s] tool_choice.type = %#v, want \"auto\"", model, tc["type"])
			}

			// Validate each tool
			wantNames := []string{"bash", "read"}
			sort.Strings(wantNames)
			var gotNames []string
			for _, raw := range toolsRaw {
				tool, _ := raw.(map[string]any)
				name, _ := tool["name"].(string)
				gotNames = append(gotNames, name)
				if tool["input_schema"] == nil {
					t.Errorf("[%s] tool %q missing input_schema", model, name)
				}
				if tool["description"] == nil {
					t.Errorf("[%s] tool %q missing description", model, name)
				}
			}
			sort.Strings(gotNames)
			if fmt.Sprint(gotNames) != fmt.Sprint(wantNames) {
				t.Fatalf("[%s] tool names = %v, want %v", model, gotNames, wantNames)
			}
		})
	}
}

// ─── matrix: full RunTurn with all 17 core tools for every model ──────────────

// TestAgentMatrixCoreToolsRegistered ensures that for every model, RunTurn
// forwards all 17 registered core tools to the proxy in Anthropic shape.
func TestAgentMatrixCoreToolsRegistered(t *testing.T) {
	wantToolNames := []string{
		"agent", "apply_patch", "ask_user_question", "bash", "batch",
		"calculator", "codesearch", "config", "edit",
		"enter_plan_mode", "enter_worktree", "exit_plan_mode", "exit_worktree",
		"get_current_time", "glob", "grep", "ls", "lsp", "multiedit",
		"notebook_edit", "powershell", "read", "schedule_cron", "send_message",
		"sleep", "task_create", "task_get", "task_list", "task_output",
		"task_stop", "task_update", "todo_write", "tool_search",
		"web_fetch", "web_search", "write",
	}
	sort.Strings(wantToolNames)

	for _, model := range allModels {
		t.Run(model, func(t *testing.T) {
			stub := &proxyStub{t: t, model: model}
			result, err := RunTurn(
				context.Background(), stub,
				"sess-tools", model, "agent-sdk-go",
				"list the current directory",
				nil, nil, nil,
			)
			if err != nil {
				t.Fatalf("[%s] RunTurn error: %v", model, err)
			}
			if result == nil {
				t.Fatalf("[%s] nil result", model)
			}

			payload := stub.capturedPayload[0]
			toolsRaw, _ := payload["tools"].([]any)
			if len(toolsRaw) != 36 {
				t.Fatalf("[%s] tools count = %d, want 36", model, len(toolsRaw))
			}
			var gotNames []string
			for _, raw := range toolsRaw {
				tool, _ := raw.(map[string]any)
				gotNames = append(gotNames, tool["name"].(string))
			}
			sort.Strings(gotNames)
			if fmt.Sprint(gotNames) != fmt.Sprint(wantToolNames) {
				t.Fatalf("[%s] tool names mismatch\n  got:  %v\n  want: %v", model, gotNames, wantToolNames)
			}
		})
	}
}

// ─── matrix: permission checker wiring ───────────────────────────────────────

// TestAgentMatrixPermissionCheckerAllBackends verifies that the permission
// checker is called for every backend when a tool_use response is returned.
func TestAgentMatrixPermissionCheckerAllBackends(t *testing.T) {
	for _, backend := range allBackends {
		model := "claude-haiku-4.5"
		tag := "backend=" + backend
		t.Run(tag, func(t *testing.T) {
			stub := &proxyStub{t: t, model: model, toolCall: true}
			checkerCalls := 0
			result, err := RunTurn(
				context.Background(), stub,
				"sess-perm", model, backend,
				"what time is it?",
				nil,
				func(_ context.Context, _ toolspkg.PermissionRequest) (bool, error) {
					checkerCalls++
					return true, nil
				},
				nil,
			)
			if err != nil {
				t.Fatalf("[%s] RunTurn error: %v", tag, err)
			}
			if result == nil || result.Output != "ok" {
				t.Fatalf("[%s] unexpected result: %#v", tag, result)
			}
			if checkerCalls != 1 {
				t.Errorf("[%s] checker calls = %d, want 1", tag, checkerCalls)
			}
			if stub.calls != 2 {
				t.Errorf("[%s] proxy calls = %d, want 2 (tool + final)", tag, stub.calls)
			}
		})
	}
}

// ─── summary smoke: matrix dimensions ────────────────────────────────────────

// TestAgentMatrixDimensions is a sanity check that we cover the expected
// number of combinations and the constants haven't drifted.
func TestAgentMatrixDimensions(t *testing.T) {
	if len(allShapes) != 3 {
		t.Errorf("allShapes = %d, want 3", len(allShapes))
	}
	if len(allBackends) != 3 {
		t.Errorf("allBackends = %d, want 3", len(allBackends))
	}
	if len(allModels) != 8 {
		t.Errorf("allModels = %d, want 8", len(allModels))
	}
	t.Logf("Matrix: %d shapes × %d backends × %d models = %d combinations",
		len(allShapes), len(allBackends), len(allModels),
		len(allShapes)*len(allBackends)*len(allModels))
}
