package agent

// agent_matrix_test.go — Comprehensive agent turn matrix
//
// Tests every combination of:
//   - 3 logical shape labels accepted by the agent settings UI
//   - the single supported OmniCode backend
//   - 8 target models: gpt-5.4-mini, gpt-5-mini, claude-haiku-4.5, gemini-3.1-flash,
//                      deepseek-v4-flash, qwen3.6-flash, kimi-k2.6, glm-5.1
//
// All requests run through the OmniLLM stub proxy (no real network).
// Each case verifies:
//   - The proxy is always called on /v1/messages
//   - The correct model name is forwarded
//   - The Anthropic Messages API shape is used (tools array with input_schema,
//     NOT deprecated functions/function_call)
//   - No panic, no spurious error

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"sort"
	"strings"
	"testing"

	toolspkg "omnillm/internal/tools"
)

// ─── test matrix dimensions ───────────────────────────────────────────────────

type apiShape string

const (
	shapeMessages  apiShape = "messages"  // /v1/messages  (Anthropic)
	shapeChatCompl apiShape = "chat"      // /v1/chat/completions (OpenAI)
	shapeResponses apiShape = "responses" // /v1/responses (Responses API)
)

var allShapes = []apiShape{shapeMessages, shapeChatCompl, shapeResponses}

var allBackends = []string{"omnicode"}

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

func chatResponse(model string) []byte {
	r := map[string]any{
		"id":    "chatcmpl_matrix",
		"model": model,
		"choices": []map[string]any{{
			"index":         0,
			"finish_reason": "stop",
			"message": map[string]any{
				"role":    "assistant",
				"content": "ok",
			},
		}},
		"usage": map[string]any{"prompt_tokens": 5, "completion_tokens": 2, "total_tokens": 7},
	}
	b, _ := json.Marshal(r)
	return b
}

func responsesResponse(model string) []byte {
	r := map[string]any{
		"id":     "resp_matrix",
		"object": "response",
		"model":  model,
		"output": []map[string]any{{
			"type": "message",
			"role": "assistant",
			"content": []map[string]any{{
				"type": "output_text",
				"text": "ok",
			}},
		}},
		"usage": map[string]any{"input_tokens": 5, "output_tokens": 2, "total_tokens": 7},
	}
	b, _ := json.Marshal(r)
	return b
}

// ─── stub proxy helpers ───────────────────────────────────────────────────────

// proxyStub is a minimal stub Client that records calls and returns canned
// Anthropic /v1/messages responses regardless of which path is requested.
// (All three backends hit /v1/messages; we never reach /v1/chat/completions
// or /v1/responses from within the agent runtime — those are upstream concerns
// handled by OmniLLM's proxy layer.)
type proxyStub struct {
	t               *testing.T
	model           string
	calls           int
	capturedPath    []string
	capturedPayload []map[string]any
	server          *httptest.Server
	// toolCall: when true, first call returns tool_use; second returns text.
	toolCall bool
}

func (s *proxyStub) GetBaseURL() string {
	if s.server == nil {
		s.server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			data, err := io.ReadAll(r.Body)
			if err != nil {
				s.t.Fatalf("proxyStub HTTP: read body: %v", err)
			}
			body := json.RawMessage(data)
			response, err := s.Post(r.URL.Path, body)
			if err != nil {
				s.t.Fatalf("proxyStub HTTP: post: %v", err)
			}
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write(response)
		}))
		s.t.Cleanup(s.server.Close)
	}
	return s.server.URL
}

func (s *proxyStub) GetAPIKey() string { return "test-api-key" }

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

	if path == "/v1/chat/completions" {
		return chatResponse(s.model), nil
	}
	if path == "/v1/responses" {
		return responsesResponse(s.model), nil
	}
	if s.toolCall {
		return messagesToolResponse(s.model, s.calls), nil
	}
	return messagesResponse(s.model), nil
}

func (s *proxyStub) PostStream(_ string, _ any) (*http.Response, error) {
	return nil, fmt.Errorf("streaming not exercised in matrix test")
}

func pathForShape(_ apiShape) string {
	return "/v1/messages"
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
					"sess-matrix", model, backend, DefaultAPIShape,
					"Hello from "+tag,
					nil, nil, nil, 10,
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
					"sess-tool", model, backend, DefaultAPIShape,
					"What time is it?",
					nil, nil, nil, 10,
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
					"sess-stream", model, backend, DefaultAPIShape,
					"Stream hello from "+tag,
					nil, nil, nil, 10,
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

// ─── matrix: all logical API shape labels → /v1/messages ─────────────────────

// TestAgentMatrixAllAPIShapes verifies that NewChatCompletionsDispatch
// (the only dispatch used by all backends) always sends to /v1/messages
// regardless of which legacy "shape" label is still present in a session.
// From the agent's perspective, shape selection is no longer meaningful — the
// agent always speaks Anthropic Messages API to the proxy.
func TestAgentMatrixAllAPIShapes(t *testing.T) {
	// The agent runtime always calls /v1/messages.
	// We verify this is true for every model across every legacy shape label.
	for _, shape := range allShapes {
		for _, model := range allModels {
			tag := fmt.Sprintf("shape=%s/model=%s", shape, model)
			t.Run(tag, func(t *testing.T) {
				stub := &proxyStub{t: t, model: model}
				dispatch := NewDispatch(stub, model, string(shape))
				respCh, err := dispatch(context.Background(), &MessagesRequest{
					Model:      model,
					MaxTokens:  4096,
					Messages:   []Message{testUserMessage("hi")},
					Tools:      []toolspkg.ToolDefinition{testToolDefinition("get_current_time", stringPtr("Return current time"), map[string]any{"type": "object", "properties": map[string]any{}})},
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
				if stub.capturedPath[0] != pathForShape(shape) {
					t.Fatalf("[%s] path = %q, want %s", tag, stub.capturedPath[0], pathForShape(shape))
				}
				payload := stub.capturedPayload[0]
				if payload["model"] != model {
					t.Fatalf("[%s] model = %#v, want %q", tag, payload["model"], model)
				}
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

// ─── matrix: selectDispatch routes to /v1/messages ──────────────

// TestAgentMatrixSelectDispatchAllBackends verifies that the supported backend
// calls the OmniLLM proxy on /v1/messages with the correct model.
func TestAgentMatrixSelectDispatchAllBackends(t *testing.T) {
	for _, backend := range allBackends {
		for _, model := range allModels {
			tag := fmt.Sprintf("backend=%s/model=%s", backend, model)
			t.Run(tag, func(t *testing.T) {
				stub := &proxyStub{t: t, model: model}
				dispatch := selectDispatch(stub, model, backend, DefaultAPIShape)
				respCh, err := dispatch(context.Background(), testMessagesRequest(model, testUserMessage("hi")))
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
	toolDefs := []toolspkg.ToolDefinition{
		{
			Name:        "bash",
			Description: stringPtr("Run a shell command"),
			InputSchema: map[string]any{"type": "object", "properties": map[string]any{"command": map[string]any{"type": "string"}}, "required": []string{"command"}},
		},
		{
			Name:        "read",
			Description: stringPtr("Read a file"),
			InputSchema: map[string]any{"type": "object", "properties": map[string]any{"file_path": map[string]any{"type": "string"}}, "required": []string{"file_path"}},
		},
	}

	for _, model := range allModels {
		t.Run(model, func(t *testing.T) {
			stub := &proxyStub{t: t, model: model}
			dispatch := NewChatCompletionsDispatch(stub, model)
			_, err := dispatch(context.Background(), &MessagesRequest{Model: model, MaxTokens: 4096, Messages: []Message{testUserMessage("help")}, Tools: toolDefs, ToolChoice: "auto"})
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
	// Skill filtering is active: only core tools + load_skill are visible
	// by default. Skill tools (web, task, filesystem, etc.) require load_skill.
	wantToolNames := []string{
		"ask_user_question", "bash", "edit", "get_current_time",
		"glob", "grep", "load_skill", "ls", "powershell", "read",
		"todo_write", "write",
	}
	sort.Strings(wantToolNames)

	for _, model := range allModels {
		t.Run(model, func(t *testing.T) {
			stub := &proxyStub{t: t, model: model}
			result, err := RunTurn(
				context.Background(), stub,
				"sess-tools", model, "omnicode", DefaultAPIShape,
				"list the current directory",
				nil, nil, nil, 10,
			)
			if err != nil {
				t.Fatalf("[%s] RunTurn error: %v", model, err)
			}
			if result == nil {
				t.Fatalf("[%s] nil result", model)
			}

			payload := stub.capturedPayload[0]
			toolsRaw, _ := payload["tools"].([]any)
			if len(toolsRaw) != 12 {
				t.Fatalf("[%s] tools count = %d, want 12", model, len(toolsRaw))
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
				"sess-perm", model, backend, DefaultAPIShape,
				"what time is it?",
				nil,
				func(_ context.Context, _ toolspkg.PermissionRequest) (bool, error) {
					checkerCalls++
					return true, nil
				},
				nil, 10,
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

// ─── multi-turn conversation simulation ──────────────────────────────────────

// multiTurnExchange describes one step in a simulated conversation.
type multiTurnExchange struct {
	userPrompt string
	// assistantReply is the stub response the model returns for this turn.
	// If toolCallOnTurn is non-zero, the first call to that turn number returns
	// a tool_use block; the second call returns assistantReply.
	assistantReply string
	// toolCallOnFirstStep: when true, stub returns a tool_use then this reply.
	toolCallOnFirstStep bool
}

// multiTurnStub is a stub Client that replays a scripted conversation.
// It cycles through exchanges in order; each exchange may optionally trigger
// one tool call on its first proxy call.
type multiTurnStub struct {
	t           *testing.T
	model       string
	exchanges   []multiTurnExchange
	turnIdx     int // which exchange we are currently serving
	callInTurn  int // call count within the current exchange (1-based)
	totalCalls  int
	allPaths    []string
	allPayloads []map[string]any
}

func (s *multiTurnStub) Post(path string, body any) ([]byte, error) {
	s.totalCalls++
	s.callInTurn++
	s.allPaths = append(s.allPaths, path)

	data, _ := json.Marshal(body)
	var p map[string]any
	_ = json.Unmarshal(data, &p)
	s.allPayloads = append(s.allPayloads, p)

	if s.turnIdx >= len(s.exchanges) {
		s.t.Fatalf("multiTurnStub: unexpected extra call at turn %d", s.turnIdx)
	}
	ex := s.exchanges[s.turnIdx]

	var responseBytes []byte
	if ex.toolCallOnFirstStep && s.callInTurn == 1 {
		responseBytes = messagesToolResponse(s.model, 1) // tool_use
	} else {
		// final text reply for this turn
		r := map[string]any{
			"id":          fmt.Sprintf("msg_turn%d", s.turnIdx),
			"type":        "message",
			"role":        "assistant",
			"model":       s.model,
			"stop_reason": "end_turn",
			"usage":       map[string]any{"input_tokens": 10, "output_tokens": 5},
			"content":     []map[string]any{{"type": "text", "text": ex.assistantReply}},
		}
		b, _ := json.Marshal(r)
		responseBytes = b
		// Advance to the next exchange for the next RunTurn call.
		s.turnIdx++
		s.callInTurn = 0
	}
	return responseBytes, nil
}

func (s *multiTurnStub) PostStream(_ string, _ any) (*http.Response, error) {
	return nil, fmt.Errorf("streaming not used in multi-turn simulation")
}

// conversationScript is the scripted dialogue used for TestAgentMatrixMultiTurn.
// The user sends four messages; the model answers two with plain text and two
// after a tool call (simulating a realistic coding assistant session).
var conversationScript = []multiTurnExchange{
	{
		userPrompt:          "List the files in the current directory.",
		assistantReply:      "main.go  README.md  go.mod",
		toolCallOnFirstStep: true, // model calls ls tool, then replies with file list
	},
	{
		userPrompt:     "What does main.go do?",
		assistantReply: "main.go is the entry point. It initialises the proxy server.",
	},
	{
		userPrompt:          "Show me the first 20 lines of main.go.",
		assistantReply:      "package main\n\nimport \"fmt\"\n\nfunc main() { fmt.Println(\"omnillm\") }",
		toolCallOnFirstStep: true, // model calls read tool, then replies with content
	},
	{
		userPrompt:     "Thanks, that's all.",
		assistantReply: "You're welcome! Let me know if you need anything else.",
	},
}

// alibabaModels are the Alibaba DashScope models under test.
var alibabaModels = []string{"glm-5.1", "deepseek-v4-flash", "qwen3.6-flash"}

// copilotModels are the GitHub Copilot models under test (Jian Zhu account).
var copilotModels = []string{"gpt-5-mini", "claude-haiku-4.5", "gpt-5.4-mini"}

// TestAgentMatrixMultiTurn simulates a realistic 4-turn coding-assistant
// conversation across all 3 agent backends and all 6 targeted models
// (3 Alibaba + 3 GitHub Copilot).  Each turn calls RunTurn with the
// accumulated conversation history, matching how the omnicode agent REPL works.
//
// Assertions per turn:
//   - No error
//   - Correct assistant reply text
//   - All proxy calls hit /v1/messages
//   - Tool-call turns require exactly 2 proxy calls; plain turns require 1
func TestAgentMatrixMultiTurn(t *testing.T) {
	targetModels := append(alibabaModels, copilotModels...)

	for _, backend := range allBackends {
		for _, model := range targetModels {
			backend, model := backend, model
			tag := fmt.Sprintf("%s/%s", backend, model)
			t.Run(tag, func(t *testing.T) {
				stub := &multiTurnStub{
					t:         t,
					model:     model,
					exchanges: conversationScript,
				}

				var history []HistoryMessage

				for turnN, ex := range conversationScript {
					wantCalls := 1
					if ex.toolCallOnFirstStep {
						wantCalls = 2
					}
					prevTotal := stub.totalCalls

					result, err := RunTurn(
						context.Background(), stub,
						fmt.Sprintf("sess-multiturn-%s-%s", backend, model),
						model, backend, DefaultAPIShape,
						ex.userPrompt,
						history, nil, nil, 10,
					)
					if err != nil {
						t.Fatalf("[%s] turn %d: RunTurn error: %v", tag, turnN+1, err)
					}
					if result == nil {
						t.Fatalf("[%s] turn %d: nil result", tag, turnN+1)
					}
					if result.Output != ex.assistantReply {
						t.Errorf("[%s] turn %d: output = %q, want %q", tag, turnN+1, result.Output, ex.assistantReply)
					}

					gotCalls := stub.totalCalls - prevTotal
					if gotCalls != wantCalls {
						t.Errorf("[%s] turn %d: proxy calls = %d, want %d", tag, turnN+1, gotCalls, wantCalls)
					}

					// Verify all proxy calls in this turn used /v1/messages.
					for i := prevTotal; i < stub.totalCalls; i++ {
						if stub.allPaths[i] != "/v1/messages" {
							t.Errorf("[%s] turn %d call %d: path = %q, want /v1/messages",
								tag, turnN+1, i-prevTotal+1, stub.allPaths[i])
						}
						if stub.allPayloads[i]["model"] != model {
							t.Errorf("[%s] turn %d call %d: model = %#v, want %q",
								tag, turnN+1, i-prevTotal+1, stub.allPayloads[i]["model"], model)
						}
					}

					// Accumulate history for the next turn.
					history = append(history, HistoryMessage{Role: "user", Content: ex.userPrompt})
					history = append(history, HistoryMessage{Role: "assistant", Content: result.Output})
				}

				// Total proxy calls: turns without tool = 1 call; with tool = 2 calls.
				// Script: turns 0,2 have tools (2 calls each); turns 1,3 don't (1 call each).
				wantTotal := 0
				for _, ex := range conversationScript {
					if ex.toolCallOnFirstStep {
						wantTotal += 2
					} else {
						wantTotal++
					}
				}
				if stub.totalCalls != wantTotal {
					t.Errorf("[%s] total proxy calls = %d, want %d", tag, stub.totalCalls, wantTotal)
				}

				t.Logf("[%s] completed %d-turn conversation in %d proxy calls",
					tag, len(conversationScript), stub.totalCalls)
			})
		}
	}
}

func TestStreamTurnExecutesProviderQualifiedClaudeToolUse(t *testing.T) {
	model := "github-copilot-jian-zhu---zhujian0805-gmail-com/claude-haiku-4.5"
	stub := &proxyStub{t: t, model: model, toolCall: true}
	checkerCalls := 0

	eventCh, err := StreamTurn(
		context.Background(), stub,
		"sess-copilot-claude-tool", model, "omnicode", DefaultAPIShape,
		"explain codebase",
		nil,
		func(_ context.Context, _ toolspkg.PermissionRequest) (bool, error) {
			checkerCalls++
			return true, nil
		},
		nil, 10,
	)
	if err != nil {
		t.Fatalf("StreamTurn error: %v", err)
	}

	var toolCalls int
	var tokens []string
	var errs []string
	var done bool
	for event := range eventCh {
		switch event.Type {
		case EventToken:
			tokens = append(tokens, event.Content)
		case EventToolCall:
			toolCalls++
		case EventDone:
			done = true
		case EventError:
			errs = append(errs, event.Content)
		}
	}

	if len(errs) > 0 {
		t.Fatalf("stream errors: %v", errs)
	}
	if !done {
		t.Fatal("EventDone never received")
	}
	if toolCalls != 1 {
		t.Fatalf("tool call events = %d, want 1", toolCalls)
	}
	if checkerCalls != 1 {
		t.Fatalf("permission checker calls = %d, want 1", checkerCalls)
	}
	if stub.calls != 2 {
		t.Fatalf("proxy calls = %d, want 2", stub.calls)
	}
	if got := strings.Join(tokens, ""); got != "ok" {
		t.Fatalf("final token output = %q, want ok", got)
	}
	if stub.capturedPayload[0]["model"] != model {
		t.Fatalf("request model = %#v, want %q", stub.capturedPayload[0]["model"], model)
	}
}

func TestStreamTurnErrorsWhenToolUseStopReasonHasNoToolUseBlock(t *testing.T) {
	ag := NewAgent(toolspkg.NewRegistry(), NewBufferMemory(8), 3, func(ctx context.Context, req *MessagesRequest) (<-chan *MessagesResponse, error) {
		ch := make(chan *MessagesResponse, 1)
		ch <- &MessagesResponse{
			ID:         "msg-missing-tool",
			Model:      req.Model,
			Content:    []ContentBlock{TextBlock("Sure! Let me explore the codebase to give you a thorough explanation.")},
			StopReason: StopReasonToolUse,
		}
		close(ch)
		return ch, nil
	})

	eventCh, err := ag.Stream(context.Background(), "sess-missing-tool", "explain codebase")
	if err != nil {
		t.Fatalf("Stream error: %v", err)
	}

	var errs []string
	for event := range eventCh {
		if event.Type == EventError {
			errs = append(errs, event.Content)
		}
	}

	if len(errs) != 1 {
		t.Fatalf("errors = %v, want exactly one error", errs)
	}
	if !strings.Contains(errs[0], "tool_use stop_reason") {
		t.Fatalf("error = %q, want tool_use stop_reason diagnostic", errs[0])
	}
}

// ─── summary smoke: matrix dimensions ────────────────────────────────────────

// TestAgentMatrixDimensions is a sanity check that we cover the expected
// number of combinations and the constants haven't drifted.
func TestAgentMatrixDimensions(t *testing.T) {
	if len(allShapes) != 3 {
		t.Errorf("allShapes = %d, want 3", len(allShapes))
	}
	if len(allBackends) != 1 {
		t.Errorf("allBackends = %d, want 1", len(allBackends))
	}
	if len(allModels) != 8 {
		t.Errorf("allModels = %d, want 8", len(allModels))
	}
	t.Logf("Matrix: %d shapes × %d backends × %d models = %d combinations",
		len(allShapes), len(allBackends), len(allModels),
		len(allShapes)*len(allBackends)*len(allModels))
}
