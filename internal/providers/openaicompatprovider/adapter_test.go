package openaicompatprovider

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"omnillm/internal/cif"
	"omnillm/internal/providers/openaicompat"
	"omnillm/internal/providers/shared"
	"omnillm/internal/providers/types"
	"strings"
	"testing"
)

func TestAdapterUpstreamAPISelection(t *testing.T) {
	t.Run("official openai gpt-4o uses chat completions", func(t *testing.T) {
		// gpt-4o is not in the providermodels Responses table — stays on chat completions.
		p := NewProvider("test-openai", "Test")
		p.baseURL = "https://api.openai.com/v1"
		p.configLoaded = true

		adapter := p.GetAdapter().(*Adapter)
		req := requestWithInboundShape("anthropic")

		if got := adapter.UpstreamAPI(req, "gpt-4o"); got != openAICompatChatCompletionsAPI {
			t.Fatalf("UpstreamAPI() = %q, want %q", got, openAICompatChatCompletionsAPI)
		}
	})

	t.Run("generic anthropic requests stay on chat completions", func(t *testing.T) {
		p := NewProvider("test-generic", "Test")
		p.baseURL = "http://localhost:11434/v1"
		p.configLoaded = true

		adapter := p.GetAdapter().(*Adapter)
		req := requestWithInboundShape("anthropic")

		if got := adapter.UpstreamAPI(req, "llama3"); got != openAICompatChatCompletionsAPI {
			t.Fatalf("UpstreamAPI() = %q, want %q", got, openAICompatChatCompletionsAPI)
		}
	})

	t.Run("explicit api_format forces responses on generic hosts", func(t *testing.T) {
		p := NewProvider("test-explicit", "Test")
		p.baseURL = "http://localhost:11434/v1"
		p.config = map[string]interface{}{"api_format": "responses"}
		p.configLoaded = true

		adapter := p.GetAdapter().(*Adapter)
		req := requestWithInboundShape("openai")

		if got := adapter.UpstreamAPI(req, "gpt-4o-mini"); got != openAICompatResponsesAPI {
			t.Fatalf("UpstreamAPI() = %q, want %q", got, openAICompatResponsesAPI)
		}
	})
}

func TestSetupProviderAuthRejectsDashScopeEndpoints(t *testing.T) {
	_, _, _, _, err := SetupProviderAuth("test-dashscope", &types.AuthOptions{
		Endpoint: "https://dashscope-intl.aliyuncs.com/compatible-mode/v1",
		APIKey:   "test-key",
	})
	if err == nil || !strings.Contains(err.Error(), "DashScope endpoints must use the Alibaba provider") {
		t.Fatalf("expected DashScope rejection error, got %v", err)
	}
}

func TestAdapterExecute_ChatCompletionsTranslatesAnthropicStyleCIF(t *testing.T) {
	var capturedPath string
	var capturedPayload map[string]interface{}

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		capturedPath = r.URL.Path
		body, _ := io.ReadAll(r.Body)
		json.Unmarshal(body, &capturedPayload) //nolint:errcheck
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, `{"id":"chat_123","model":"llama3","choices":[{"index":0,"message":{"role":"assistant","content":"done"},"finish_reason":"stop"}],"usage":{"prompt_tokens":11,"completion_tokens":2,"total_tokens":13}}`)
	}))
	defer srv.Close()

	p := NewProvider("test-chat", "Test")
	p.baseURL = srv.URL + "/v1"
	p.configLoaded = true

	adapter := p.GetAdapter().(*Adapter)
	resp, err := adapter.Execute(context.Background(), sampleToolLoopRequest("anthropic"))
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}

	if capturedPath != "/v1/chat/completions" {
		t.Fatalf("expected /v1/chat/completions, got %q", capturedPath)
	}
	if text, ok := firstResponseText(resp); !ok || text != "done" {
		t.Fatalf("unexpected response content: %#v", resp.Content)
	}

	messages, ok := capturedPayload["messages"].([]interface{})
	if !ok || len(messages) != 4 {
		t.Fatalf("unexpected chat payload messages: %#v", capturedPayload["messages"])
	}
	if role := messageField(messages[0], "role"); role != "system" {
		t.Fatalf("expected first message role=system, got %q", role)
	}
	if content := messageField(messages[0], "content"); content != "Be terse." {
		t.Fatalf("expected system prompt to lead chat payload, got %#v", messages[0])
	}
	if role := messageField(messages[1], "role"); role != "assistant" {
		t.Fatalf("expected second message role=assistant, got %q", role)
	}
	if toolCalls, ok := messageMap(messages[1])["tool_calls"].([]interface{}); !ok || len(toolCalls) != 1 {
		t.Fatalf("expected assistant tool_calls in chat payload, got %#v", messages[1])
	}
	if role := messageField(messages[2], "role"); role != "tool" {
		t.Fatalf("expected tool result to become role=tool, got %#v", messages[2])
	}
	if role := messageField(messages[3], "role"); role != "user" {
		t.Fatalf("expected final user message, got %#v", messages[3])
	}
}

func TestAdapterExecute_ResponsesTranslatesAnthropicStyleCIF(t *testing.T) {
	var capturedPath string
	var capturedPayload map[string]interface{}

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		capturedPath = r.URL.Path
		body, _ := io.ReadAll(r.Body)
		json.Unmarshal(body, &capturedPayload) //nolint:errcheck
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, `{"id":"resp_123","model":"gpt-4o-mini","status":"completed","output":[{"type":"message","id":"msg_1","role":"assistant","content":[{"type":"output_text","text":"done"}]}],"usage":{"input_tokens":11,"output_tokens":2}}`)
	}))
	defer srv.Close()

	p := NewProvider("test-responses", "Test")
	p.baseURL = srv.URL + "/v1"
	p.config = map[string]interface{}{"api_format": "responses"}
	p.configLoaded = true

	adapter := p.GetAdapter().(*Adapter)
	resp, err := adapter.Execute(context.Background(), sampleToolLoopRequest("anthropic"))
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}

	if capturedPath != "/v1/responses" {
		t.Fatalf("expected /v1/responses, got %q", capturedPath)
	}
	if text, ok := firstResponseText(resp); !ok || text != "done" {
		t.Fatalf("unexpected response content: %#v", resp.Content)
	}
	if instructions, _ := capturedPayload["instructions"].(string); instructions != "Be terse." {
		t.Fatalf("expected responses instructions to carry system prompt, got %#v", capturedPayload["instructions"])
	}

	input, ok := capturedPayload["input"].([]interface{})
	if !ok || len(input) != 3 {
		t.Fatalf("unexpected responses input payload: %#v", capturedPayload["input"])
	}
	if itemType := itemField(input[0], "type"); itemType != "function_call" {
		t.Fatalf("expected first input item to be function_call, got %#v", input[0])
	}
	if itemType := itemField(input[1], "type"); itemType != "function_call_output" {
		t.Fatalf("expected second input item to be function_call_output, got %#v", input[1])
	}
	if itemType := itemField(input[2], "type"); itemType != "message" {
		t.Fatalf("expected final input item to be message, got %#v", input[2])
	}
}

func TestAdapterExecute_ResponsesIncludesConfiguredToolExtras(t *testing.T) {
	var capturedPayload map[string]interface{}

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		json.Unmarshal(body, &capturedPayload) //nolint:errcheck
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, `{"id":"resp_qwen","model":"qwen3.6-plus","status":"completed","output":[{"type":"message","id":"msg_1","role":"assistant","content":[{"type":"output_text","text":"done"}]}],"usage":{"input_tokens":11,"output_tokens":2}}`)
	}))
	defer srv.Close()

	request := sampleToolLoopRequest("openai")
	request.Model = "qwen3.6-plus"

	payload := openaicompat.BuildResponsesPayload(
		request.Model,
		request,
		false,
		openaicompat.ResponsesConfig{Extras: map[string]interface{}{"enable_thinking": false}},
	)

	if value, ok := payload["enable_thinking"].(bool); !ok || value {
		t.Fatalf("expected enable_thinking=false in responses payload, got %#v", payload["enable_thinking"])
	}

	if _, err := openaicompat.ExecuteResponses(context.Background(), srv.URL+"/compatible-mode/v1/responses", buildHeaders("", false), payload); err != nil {
		t.Fatalf("ExecuteResponses() error = %v", err)
	}

	if value, ok := capturedPayload["enable_thinking"].(bool); !ok || value {
		t.Fatalf("expected enable_thinking=false in outbound responses payload, got %#v", capturedPayload["enable_thinking"])
	}
}

func TestAdapterExecuteStream_ResponsesParsesSSE(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/responses" {
			http.Error(w, "unexpected path", http.StatusTeapot)
			return
		}
		w.Header().Set("Content-Type", "text/event-stream")
		fmt.Fprint(w, "event: response.created\n")
		fmt.Fprint(w, "data: {\"response\":{\"id\":\"resp_stream\",\"model\":\"gpt-4o-mini\"}}\n\n")
		fmt.Fprint(w, "event: response.output_text.delta\n")
		fmt.Fprint(w, "data: {\"output_index\":0,\"content_index\":0,\"delta\":\"po\"}\n\n")
		fmt.Fprint(w, "event: response.output_text.delta\n")
		fmt.Fprint(w, "data: {\"output_index\":0,\"content_index\":0,\"delta\":\"ng\"}\n\n")
		fmt.Fprint(w, "event: response.completed\n")
		fmt.Fprint(w, "data: {\"response\":{\"id\":\"resp_stream\",\"model\":\"gpt-4o-mini\",\"status\":\"completed\",\"output\":[{\"type\":\"message\",\"id\":\"msg_1\",\"role\":\"assistant\",\"content\":[{\"type\":\"output_text\",\"text\":\"pong\"}]}],\"usage\":{\"input_tokens\":5,\"output_tokens\":1}}}\n\n")
	}))
	defer srv.Close()

	p := NewProvider("test-stream", "Test")
	p.baseURL = srv.URL + "/v1"
	p.config = map[string]interface{}{"api_format": "responses"}
	p.configLoaded = true

	adapter := p.GetAdapter().(*Adapter)
	eventCh, err := adapter.ExecuteStream(context.Background(), sampleToolLoopRequest("responses"))
	if err != nil {
		t.Fatalf("ExecuteStream() error = %v", err)
	}

	resp, err := shared.CollectStream(eventCh)
	if err != nil {
		t.Fatalf("CollectStream() error = %v", err)
	}

	if resp.ID != "resp_stream" {
		t.Fatalf("unexpected stream response id: %q", resp.ID)
	}
	if text, ok := firstResponseText(resp); !ok || text != "pong" {
		t.Fatalf("unexpected stream response content: %#v", resp.Content)
	}
	if resp.Usage == nil || resp.Usage.InputTokens != 5 || resp.Usage.OutputTokens != 1 {
		t.Fatalf("unexpected stream usage: %#v", resp.Usage)
	}
}

func TestAdapterExecuteStream_AnthropicRequestsBufferCompletedResponse(t *testing.T) {
	var capturedPath string
	var capturedAccept string
	var capturedPayload map[string]interface{}

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		capturedPath = r.URL.Path
		capturedAccept = r.Header.Get("Accept")

		body, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(body, &capturedPayload)

		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, `{"id":"chat_buffered","model":"llama3","choices":[{"index":0,"message":{"role":"assistant","tool_calls":[{"id":"call_readme","type":"function","function":{"name":"Read","arguments":"{\"file_path\":\"README.md\"}"}}]},"finish_reason":"tool_calls"}],"usage":{"prompt_tokens":19,"completion_tokens":4,"total_tokens":23}}`)
	}))
	defer srv.Close()

	p := NewProvider("test-buffered", "Test")
	p.baseURL = srv.URL + "/v1"
	p.configLoaded = true

	adapter := p.GetAdapter().(*Adapter)
	eventCh, err := adapter.ExecuteStream(context.Background(), &cif.CanonicalRequest{
		Model: "llama3",
		Messages: []cif.CIFMessage{
			cif.CIFUserMessage{
				Role: "user",
				Content: []cif.CIFContentPart{
					cif.CIFTextPart{Type: "text", Text: "Explain the codebase"},
				},
			},
		},
		Tools: []cif.CIFTool{{
			Name:             "Read",
			ParametersSchema: map[string]interface{}{"type": "object"},
		}},
		Stream: true,
		Extensions: &cif.Extensions{
			InboundAPIShape: stringPtr("anthropic"),
		},
	})
	if err != nil {
		t.Fatalf("ExecuteStream() error = %v", err)
	}

	resp, err := shared.CollectStream(eventCh)
	if err != nil {
		t.Fatalf("CollectStream() error = %v", err)
	}

	if capturedPath != "/v1/chat/completions" {
		t.Fatalf("expected /v1/chat/completions, got %q", capturedPath)
	}
	if capturedAccept != "application/json" {
		t.Fatalf("expected non-streaming Accept header, got %q", capturedAccept)
	}
	if stream, _ := capturedPayload["stream"].(bool); stream {
		t.Fatalf("expected buffered non-stream upstream request, got payload %#v", capturedPayload)
	}
	if resp.StopReason != cif.StopReasonToolUse {
		t.Fatalf("expected stop reason %q, got %q", cif.StopReasonToolUse, resp.StopReason)
	}

	if len(resp.Content) != 1 {
		t.Fatalf("expected one streamed content block, got %#v", resp.Content)
	}
	toolCall, ok := resp.Content[0].(cif.CIFToolCallPart)
	if !ok {
		t.Fatalf("expected tool call content, got %#v", resp.Content[0])
	}
	if toolCall.ToolCallID != "call_readme" || toolCall.ToolName != "Read" {
		t.Fatalf("unexpected tool call metadata: %#v", toolCall)
	}
	if toolCall.ToolArguments["file_path"] != "README.md" {
		t.Fatalf("unexpected tool arguments: %#v", toolCall.ToolArguments)
	}
	if resp.Usage == nil || resp.Usage.InputTokens != 19 || resp.Usage.OutputTokens != 4 {
		t.Fatalf("unexpected usage: %#v", resp.Usage)
	}
}

func requestWithInboundShape(shape string) *cif.CanonicalRequest {
	return &cif.CanonicalRequest{
		Extensions: &cif.Extensions{InboundAPIShape: &shape},
	}
}

func sampleToolLoopRequest(shape string) *cif.CanonicalRequest {
	system := "Be terse."
	return &cif.CanonicalRequest{
		Model:        "gpt-4o-mini",
		SystemPrompt: &system,
		Messages: []cif.CIFMessage{
			cif.CIFAssistantMessage{
				Role: "assistant",
				Content: []cif.CIFContentPart{
					cif.CIFToolCallPart{
						Type:          "tool_call",
						ToolCallID:    "call_weather",
						ToolName:      "get_weather",
						ToolArguments: map[string]interface{}{"location": "Shanghai"},
					},
				},
			},
			cif.CIFUserMessage{
				Role: "user",
				Content: []cif.CIFContentPart{
					cif.CIFToolResultPart{
						Type:       "tool_result",
						ToolCallID: "call_weather",
						ToolName:   "get_weather",
						Content:    "22C",
					},
					cif.CIFTextPart{Type: "text", Text: "Now answer"},
				},
			},
		},
		Tools: []cif.CIFTool{
			{
				Name:             "get_weather",
				ParametersSchema: map[string]interface{}{"type": "object"},
			},
		},
		ToolChoice: "auto",
		Extensions: &cif.Extensions{InboundAPIShape: &shape},
	}
}

func firstResponseText(resp *cif.CanonicalResponse) (string, bool) {
	if resp == nil || len(resp.Content) == 0 {
		return "", false
	}
	text, ok := resp.Content[0].(cif.CIFTextPart)
	if !ok {
		return "", false
	}
	return text.Text, true
}

func messageMap(raw interface{}) map[string]interface{} {
	msg, _ := raw.(map[string]interface{})
	return msg
}

func messageField(raw interface{}, key string) string {
	value, _ := messageMap(raw)[key].(string)
	return value
}

func itemField(raw interface{}, key string) string {
	value, _ := messageMap(raw)[key].(string)
	return value
}

func stringPtr(value string) *string {
	return &value
}
