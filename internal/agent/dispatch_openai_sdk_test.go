package agent

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"omnillm/internal/tools"
)

func TestOpenAISDKDispatchConvertsRequest(t *testing.T) {
	var capturedBody map[string]any
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/chat/completions" {
			t.Fatalf("path = %q, want /chat/completions", r.URL.Path)
		}
		if auth := r.Header.Get("Authorization"); auth != "Bearer test-openai-key" {
			t.Fatalf("Authorization = %q, want Bearer test-openai-key", auth)
		}
		if err := json.NewDecoder(r.Body).Decode(&capturedBody); err != nil {
			t.Fatalf("decode request: %v", err)
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"id":      "chatcmpl_test",
			"object":  "chat.completion",
			"created": 1,
			"model":   "gpt-4o-mini",
			"choices": []map[string]any{{
				"index":         0,
				"finish_reason": "stop",
				"message": map[string]any{
					"role":    "assistant",
					"content": "Hello from OpenAI SDK",
				},
			}},
			"usage": map[string]any{"prompt_tokens": 7, "completion_tokens": 3, "total_tokens": 10},
		})
	}))
	defer server.Close()

	dispatch := OpenAISDKDispatch("test-openai-key", server.URL, "gpt-4o-mini")
	req := testMessagesRequest("gpt-4o-mini", testUserMessage("Hello"))
	req.System = []ContentBlock{TextBlock("You are concise.")}
	req.Tools = []tools.ToolDefinition{testToolDefinition("lookup", stringPtr("Look up data"), map[string]any{
		"type": "object",
		"properties": map[string]any{
			"query": map[string]any{"type": "string"},
		},
		"required": []any{"query"},
	})}
	req.ToolChoice = "auto"

	respCh, err := dispatch(context.Background(), req)
	if err != nil {
		t.Fatalf("dispatch error: %v", err)
	}

	var resp *MessagesResponse
	for item := range respCh {
		resp = item
	}
	if resp == nil {
		t.Fatal("got nil response")
	}
	if resp.ID != "chatcmpl_test" {
		t.Fatalf("id = %q, want chatcmpl_test", resp.ID)
	}
	if resp.StopReason != StopReasonEndTurn {
		t.Fatalf("stop_reason = %q, want end_turn", resp.StopReason)
	}
	if len(resp.Content) != 1 || resp.Content[0].Text != "Hello from OpenAI SDK" {
		t.Fatalf("content = %#v", resp.Content)
	}
	if resp.Usage == nil || resp.Usage.InputTokens != 7 || resp.Usage.OutputTokens != 3 {
		t.Fatalf("usage = %#v", resp.Usage)
	}

	if capturedBody["model"] != "gpt-4o-mini" {
		t.Fatalf("model = %#v", capturedBody["model"])
	}
	messages, ok := capturedBody["messages"].([]any)
	if !ok || len(messages) != 2 {
		t.Fatalf("messages = %#v", capturedBody["messages"])
	}
	firstMessage, _ := messages[0].(map[string]any)
	if firstMessage["role"] != "system" || firstMessage["content"] != "You are concise." {
		t.Fatalf("system message = %#v", firstMessage)
	}
	toolPayloads, ok := capturedBody["tools"].([]any)
	if !ok || len(toolPayloads) != 1 {
		t.Fatalf("tools = %#v", capturedBody["tools"])
	}
	toolPayload, _ := toolPayloads[0].(map[string]any)
	if toolPayload["type"] != "function" {
		t.Fatalf("tool type = %#v", toolPayload["type"])
	}
	functionPayload, _ := toolPayload["function"].(map[string]any)
	if functionPayload["name"] != "lookup" {
		t.Fatalf("function = %#v", functionPayload)
	}
	if capturedBody["tool_choice"] != "auto" {
		t.Fatalf("tool_choice = %#v, want auto", capturedBody["tool_choice"])
	}
}

func TestOpenAISDKDispatchConvertsToolCalls(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"id":      "chatcmpl_tool",
			"object":  "chat.completion",
			"created": 1,
			"model":   "gpt-4o-mini",
			"choices": []map[string]any{{
				"index":         0,
				"finish_reason": "tool_calls",
				"message": map[string]any{
					"role": "assistant",
					"tool_calls": []map[string]any{{
						"id":   "call_123",
						"type": "function",
						"function": map[string]any{
							"name":      "lookup",
							"arguments": `{"query":"repo"}`,
						},
					}},
				},
			}},
		})
	}))
	defer server.Close()

	dispatch := OpenAISDKDispatch("test-openai-key", server.URL, "gpt-4o-mini")
	respCh, err := dispatch(context.Background(), testMessagesRequest("gpt-4o-mini", testUserMessage("call a tool")))
	if err != nil {
		t.Fatalf("dispatch error: %v", err)
	}

	var resp *MessagesResponse
	for item := range respCh {
		resp = item
	}
	if resp == nil {
		t.Fatal("got nil response")
	}
	if resp.StopReason != StopReasonToolUse {
		t.Fatalf("stop_reason = %q, want tool_use", resp.StopReason)
	}
	if len(resp.Content) != 1 {
		t.Fatalf("content = %#v", resp.Content)
	}
	call := resp.Content[0]
	if call.Type != "tool_use" || call.ID != "call_123" || call.Name != "lookup" {
		t.Fatalf("tool call = %#v", call)
	}
	if call.Input["query"] != "repo" {
		t.Fatalf("tool input = %#v", call.Input)
	}
}

func TestOpenAIParamsReturnsToolInputMarshalError(t *testing.T) {
	_, err := openAIParamsFromRequest(&MessagesRequest{
		Model: "gpt-4o-mini",
		Messages: []Message{{
			Role: "assistant",
			Content: []ContentBlock{{
				Type:  "tool_use",
				ID:    "call_bad",
				Name:  "lookup",
				Input: map[string]any{"bad": func() {}},
			}},
		}},
	})
	if err == nil || !strings.Contains(err.Error(), "marshal tool input") {
		t.Fatalf("expected tool input marshal error, got %v", err)
	}
}

type openAIConfigStubClient struct {
	stubAgentClient
	baseURL string
	apiKey  string
}

func (c *openAIConfigStubClient) GetBaseURL() string { return c.baseURL }
func (c *openAIConfigStubClient) GetAPIKey() string  { return c.apiKey }

func TestSelectDispatchUsesOmniLLMEndpointAndAPIKeyForOpenAIShape(t *testing.T) {
	t.Setenv("OPENAI_API_KEY", "")
	t.Setenv("OPENAI_BASE_URL", "")
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/chat/completions" {
			t.Fatalf("path = %q, want /v1/chat/completions", r.URL.Path)
		}
		if auth := r.Header.Get("Authorization"); auth != "Bearer omnillm-test-key" {
			t.Fatalf("Authorization = %q, want Bearer omnillm-test-key", auth)
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"id":      "chatcmpl_omnillm",
			"object":  "chat.completion",
			"created": 1,
			"model":   "gpt-4o-mini",
			"choices": []map[string]any{{"index": 0, "finish_reason": "stop", "message": map[string]any{"role": "assistant", "content": "ok"}}},
		})
	}))
	defer server.Close()

	dispatch := selectDispatch(&openAIConfigStubClient{baseURL: server.URL, apiKey: "omnillm-test-key"}, "gpt-4o-mini", "google-adk", "openai")
	respCh, err := dispatch(context.Background(), testMessagesRequest("", testUserMessage("hi")))
	if err != nil {
		t.Fatalf("dispatch error: %v", err)
	}
	var got strings.Builder
	for resp := range respCh {
		for _, block := range resp.Content {
			got.WriteString(block.Text)
		}
	}
	if got.String() != "ok" {
		t.Fatalf("response text = %q, want ok", got.String())
	}
}
