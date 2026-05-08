package server

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"omnillm/internal/registry"
	"strings"
	"sync"
	"testing"
	"time"
)

type capturedAlibabaChatRequest struct {
	Authorization string
	Accept        string
	Payload       map[string]interface{}
}

type fakeAlibabaQwenUpstream struct {
	server *httptest.Server

	mu           sync.Mutex
	chatRequests []capturedAlibabaChatRequest
}

func newFakeAlibabaQwenUpstream(t *testing.T) *fakeAlibabaQwenUpstream {
	t.Helper()

	upstream := &fakeAlibabaQwenUpstream{}
	upstream.server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/v1/models":
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]interface{}{
				"data": []map[string]interface{}{
					{"id": "qwen3.6-plus"},
				},
			})
			return

		case r.Method == http.MethodPost && r.URL.Path == "/v1/chat/completions":
			var payload map[string]interface{}
			if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
				http.Error(w, fmt.Sprintf("decode payload: %v", err), http.StatusBadRequest)
				return
			}

			upstream.mu.Lock()
			upstream.chatRequests = append(upstream.chatRequests, capturedAlibabaChatRequest{
				Authorization: r.Header.Get("Authorization"),
				Accept:        r.Header.Get("Accept"),
				Payload:       payload,
			})
			upstream.mu.Unlock()

			stream, _ := payload["stream"].(bool)
			hasTools := len(asInterfaceSlice(payload["tools"])) > 0

			if requestContainsToolResult(payload) {
				if stream {
					writeFakeQwenFinalAnswerStream(w)
				} else {
					writeFakeQwenFinalAnswerResponse(w)
				}
				return
			}

			if hasTools {
				if stream {
					if requestHasToolNamed(payload, "Read") {
						writeFakeQwenReadToolStream(w)
					} else {
						writeFakeQwenToolStream(w)
					}
				} else {
					if requestHasToolNamed(payload, "Read") {
						writeFakeQwenReadToolResponse(w)
					} else {
						writeFakeQwenToolResponse(w)
					}
				}
				return
			}

			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]interface{}{
				"id":    "chatcmpl_qwen_text",
				"model": "qwen3.6-plus",
				"choices": []map[string]interface{}{
					{
						"index": 0,
						"message": map[string]interface{}{
							"role":    "assistant",
							"content": "Qwen says hello from Alibaba.",
						},
						"finish_reason": "stop",
					},
				},
				"usage": map[string]interface{}{
					"prompt_tokens":     12,
					"completion_tokens": 6,
					"total_tokens":      18,
				},
			})
			return
		}

		http.NotFound(w, r)
	}))

	t.Cleanup(upstream.server.Close)
	return upstream
}

func (u *fakeAlibabaQwenUpstream) baseURL() string {
	return u.server.URL + "/v1"
}

func (u *fakeAlibabaQwenUpstream) lastChatRequest(t *testing.T) capturedAlibabaChatRequest {
	t.Helper()

	u.mu.Lock()
	defer u.mu.Unlock()

	if len(u.chatRequests) == 0 {
		t.Fatal("expected at least one captured Alibaba chat request")
	}
	return u.chatRequests[len(u.chatRequests)-1]
}

func (u *fakeAlibabaQwenUpstream) chatRequestCount() int {
	u.mu.Lock()
	defer u.mu.Unlock()
	return len(u.chatRequests)
}

func writeFakeQwenToolResponse(w http.ResponseWriter) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]interface{}{
		"id":    "chatcmpl_qwen_tool",
		"model": "qwen3.6-plus",
		"choices": []map[string]interface{}{
			{
				"index": 0,
				"message": map[string]interface{}{
					"role": "assistant",
					"tool_calls": []map[string]interface{}{
						{
							"id":   "call_qwen_weather",
							"type": "function",
							"function": map[string]interface{}{
								"name":      "get_weather",
								"arguments": `{"location":"Shanghai"}`,
							},
						},
					},
				},
				"finish_reason": "tool_calls",
			},
		},
		"usage": map[string]interface{}{
			"prompt_tokens":     21,
			"completion_tokens": 7,
			"total_tokens":      28,
		},
	})
}

func writeFakeQwenReadToolResponse(w http.ResponseWriter) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]interface{}{
		"id":    "chatcmpl_qwen_read_tool",
		"model": "qwen3.6-plus",
		"choices": []map[string]interface{}{
			{
				"index": 0,
				"message": map[string]interface{}{
					"role": "assistant",
					"tool_calls": []map[string]interface{}{
						{
							"id":   "call_qwen_read",
							"type": "function",
							"function": map[string]interface{}{
								"name":      "Read",
								"arguments": `{"file_path":"README.md"}`,
							},
						},
					},
				},
				"finish_reason": "tool_calls",
			},
		},
		"usage": map[string]interface{}{
			"prompt_tokens":     41,
			"completion_tokens": 11,
			"total_tokens":      52,
		},
	})
}

func writeFakeQwenToolStream(w http.ResponseWriter) {
	w.Header().Set("Content-Type", "text/event-stream")
	flusher, _ := w.(http.Flusher)

	chunks := []string{
		`data: {"id":"chatcmpl_qwen_stream","model":"qwen3.6-plus","choices":[{"index":0,"delta":{"role":"assistant","reasoning_content":"Let me think this through first."}}]}` + "\n\n",
		`data: {"id":"chatcmpl_qwen_stream","model":"qwen3.6-plus","choices":[{"index":0,"delta":{"tool_calls":[{"index":0,"id":"call_qwen_weather","type":"function","function":{"name":"get_weather","arguments":"{\"location\":\"Sh"}}]}}]}` + "\n\n",
		`data: {"id":"chatcmpl_qwen_stream","model":"qwen3.6-plus","choices":[{"index":0,"delta":{"tool_calls":[{"index":0,"function":{"arguments":"anghai\"}"}}]}}]}` + "\n\n",
		`data: {"id":"chatcmpl_qwen_stream","model":"qwen3.6-plus","choices":[{"index":0,"finish_reason":"stop"}],"usage":{"prompt_tokens":23,"completion_tokens":9,"total_tokens":32}}` + "\n\n",
		`data: [DONE]` + "\n\n",
	}

	for _, chunk := range chunks {
		_, _ = io.WriteString(w, chunk)
		if flusher != nil {
			flusher.Flush()
		}
	}
}

func writeFakeQwenReadToolStream(w http.ResponseWriter) {
	w.Header().Set("Content-Type", "text/event-stream")
	flusher, _ := w.(http.Flusher)

	chunks := []string{
		`data: {"id":"chatcmpl_qwen_read_stream","model":"qwen3.6-plus","choices":[{"index":0,"delta":{"role":"assistant","reasoning_content":"I should inspect the README before answering."}}]}` + "\n\n",
		`data: {"id":"chatcmpl_qwen_read_stream","model":"qwen3.6-plus","choices":[{"index":0,"delta":{"tool_calls":[{"index":0,"id":"call_qwen_read","type":"function","function":{"name":"Read","arguments":"{\"file_path\":\"READ"}}]}}]}` + "\n\n",
		`data: {"id":"chatcmpl_qwen_read_stream","model":"qwen3.6-plus","choices":[{"index":0,"delta":{"tool_calls":[{"index":0,"function":{"arguments":"ME.md\"}"}}]}}]}` + "\n\n",
		`data: {"id":"chatcmpl_qwen_read_stream","model":"qwen3.6-plus","choices":[{"index":0,"finish_reason":"stop"}],"usage":{"prompt_tokens":41,"completion_tokens":11,"total_tokens":52}}` + "\n\n",
		`data: [DONE]` + "\n\n",
	}

	for _, chunk := range chunks {
		_, _ = io.WriteString(w, chunk)
		if flusher != nil {
			flusher.Flush()
		}
	}
}

func writeFakeQwenFinalAnswerStream(w http.ResponseWriter) {
	w.Header().Set("Content-Type", "text/event-stream")
	flusher, _ := w.(http.Flusher)

	chunks := []string{
		`data: {"id":"chatcmpl_qwen_final_stream","model":"qwen3.6-plus","choices":[{"index":0,"delta":{"role":"assistant","reasoning_content":"Now I can summarize the codebase clearly."}}]}` + "\n\n",
		`data: {"id":"chatcmpl_qwen_final_stream","model":"qwen3.6-plus","choices":[{"index":0,"delta":{"content":"This codebase exposes an OpenAI-compatible and Anthropic-compatible proxy. "}}]}` + "\n\n",
		`data: {"id":"chatcmpl_qwen_final_stream","model":"qwen3.6-plus","choices":[{"index":0,"delta":{"content":"It normalizes requests into CIF, routes by model, and adapts provider-specific upstreams."}}]}` + "\n\n",
		`data: {"id":"chatcmpl_qwen_final_stream","model":"qwen3.6-plus","choices":[{"index":0,"finish_reason":"stop"}],"usage":{"prompt_tokens":67,"completion_tokens":24,"total_tokens":91}}` + "\n\n",
		`data: [DONE]` + "\n\n",
	}

	for _, chunk := range chunks {
		_, _ = io.WriteString(w, chunk)
		if flusher != nil {
			flusher.Flush()
		}
	}
}

func writeFakeQwenFinalAnswerResponse(w http.ResponseWriter) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]interface{}{
		"id":    "chatcmpl_qwen_final",
		"model": "qwen3.6-plus",
		"choices": []map[string]interface{}{
			{
				"index": 0,
				"message": map[string]interface{}{
					"role":    "assistant",
					"content": "This codebase exposes an OpenAI-compatible and Anthropic-compatible proxy. It normalizes requests into CIF, routes by model, and adapts provider-specific upstreams.",
				},
				"finish_reason": "stop",
			},
		},
		"usage": map[string]interface{}{
			"prompt_tokens":     67,
			"completion_tokens": 24,
			"total_tokens":      91,
		},
	})
}

type parsedSSEEvent struct {
	Event string
	Data  map[string]interface{}
}

func parseSSEBody(t *testing.T, body string) []parsedSSEEvent {
	t.Helper()

	blocks := strings.Split(strings.TrimSpace(body), "\n\n")
	events := make([]parsedSSEEvent, 0, len(blocks))

	for _, block := range blocks {
		block = strings.TrimSpace(block)
		if block == "" {
			continue
		}

		var eventName string
		var dataLines []string
		for _, line := range strings.Split(block, "\n") {
			switch {
			case strings.HasPrefix(line, "event: "):
				eventName = strings.TrimSpace(strings.TrimPrefix(line, "event: "))
			case strings.HasPrefix(line, "data: "):
				dataLines = append(dataLines, strings.TrimPrefix(line, "data: "))
			}
		}

		if eventName == "" {
			continue
		}

		var payload map[string]interface{}
		if err := json.Unmarshal([]byte(strings.Join(dataLines, "\n")), &payload); err != nil {
			t.Fatalf("failed to parse SSE event %q: %v\nblock: %s", eventName, err, block)
		}
		events = append(events, parsedSSEEvent{Event: eventName, Data: payload})
	}

	return events
}

func createAndActivateAlibabaProvider(t *testing.T, backendURL, upstreamBaseURL string) (string, string) {
	t.Helper()

	apiKey := fmt.Sprintf("sk-qwen-%d", time.Now().UnixNano())
	body, err := json.Marshal(map[string]string{
		"method":   "api-key",
		"plan":     "standard",
		"region":   "global",
		"apiKey":   apiKey,
		"endpoint": upstreamBaseURL,
	})
	if err != nil {
		t.Fatalf("marshal auth request: %v", err)
	}

	req, err := http.NewRequest(
		http.MethodPost,
		backendURL+"/api/admin/providers/auth-and-create/alibaba",
		bytes.NewReader(body),
	)
	if err != nil {
		t.Fatalf("create Alibaba provider request: %v", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer test-api-key")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("create Alibaba provider: %v", err)
	}
	createBody := readBody(t, resp)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200 creating Alibaba provider, got %d: %s", resp.StatusCode, createBody)
	}

	var createPayload struct {
		Success  bool `json:"success"`
		Provider struct {
			ID string `json:"id"`
		} `json:"provider"`
	}
	if err := json.Unmarshal([]byte(createBody), &createPayload); err != nil {
		t.Fatalf("parse provider create response: %v", err)
	}
	if !createPayload.Success || createPayload.Provider.ID == "" {
		t.Fatalf("unexpected provider create payload: %#v", createPayload)
	}

	activateResp := postJSON(t, backendURL+"/api/admin/providers/"+createPayload.Provider.ID+"/activate", `{}`, nil)
	activateBody := readBody(t, activateResp)
	if activateResp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200 activating Alibaba provider, got %d: %s", activateResp.StatusCode, activateBody)
	}

	reg := registry.GetProviderRegistry()
	t.Cleanup(func() {
		_ = reg.Remove(createPayload.Provider.ID)
	})

	return createPayload.Provider.ID, apiKey
}

func asInterfaceSlice(value interface{}) []interface{} {
	items, _ := value.([]interface{})
	return items
}

func requestContainsToolResult(payload map[string]interface{}) bool {
	for _, item := range asInterfaceSlice(payload["messages"]) {
		messageMap, ok := item.(map[string]interface{})
		if !ok {
			continue
		}
		role, _ := messageMap["role"].(string)
		if role == "tool" {
			return true
		}
	}
	return false
}

func requestHasToolNamed(payload map[string]interface{}, name string) bool {
	for _, item := range asInterfaceSlice(payload["tools"]) {
		toolMap, ok := item.(map[string]interface{})
		if !ok {
			continue
		}
		functionMap, ok := toolMap["function"].(map[string]interface{})
		if !ok {
			continue
		}
		if functionName, _ := functionMap["name"].(string); functionName == name {
			return true
		}
	}
	return false
}

func TestAlibabaQwen36PlusProviderIntegration(t *testing.T) {
	upstream := newFakeAlibabaQwenUpstream(t)
	backend := newTestServer(t)
	defer backend.Close()

	_, apiKey := createAndActivateAlibabaProvider(t, backend.URL, upstream.baseURL())

	t.Run("plain chat completions request works and enables thinking", func(t *testing.T) {
		resp := postJSON(
			t,
			backend.URL+"/v1/chat/completions",
			`{"model":"qwen3.6-plus","messages":[{"role":"user","content":"Say hello"}]}`,
			nil,
		)
		body := readBody(t, resp)
		if resp.StatusCode != http.StatusOK {
			t.Fatalf("expected 200, got %d: %s", resp.StatusCode, body)
		}

		var payload struct {
			Choices []struct {
				FinishReason *string `json:"finish_reason"`
				Message      struct {
					Content string `json:"content"`
				} `json:"message"`
			} `json:"choices"`
		}
		if err := json.Unmarshal([]byte(body), &payload); err != nil {
			t.Fatalf("invalid chat response JSON: %v", err)
		}
		if len(payload.Choices) != 1 || payload.Choices[0].Message.Content != "Qwen says hello from Alibaba." {
			t.Fatalf("unexpected plain chat response: %#v", payload)
		}
		if payload.Choices[0].FinishReason == nil || *payload.Choices[0].FinishReason != "stop" {
			t.Fatalf("unexpected finish reason: %#v", payload.Choices[0].FinishReason)
		}

		lastReq := upstream.lastChatRequest(t)
		if lastReq.Authorization != "Bearer "+apiKey {
			t.Fatalf("expected bearer auth header, got %q", lastReq.Authorization)
		}
		if got, _ := lastReq.Payload["model"].(string); got != "qwen3.6-plus" {
			t.Fatalf("expected upstream model qwen3.6-plus, got %#v", lastReq.Payload["model"])
		}
		enableThinking, ok := lastReq.Payload["enable_thinking"].(bool)
		if !ok || !enableThinking {
			t.Fatalf("expected enable_thinking=true for plain qwen3.6-plus chat, got %#v", lastReq.Payload["enable_thinking"])
		}
		if len(asInterfaceSlice(lastReq.Payload["tools"])) != 0 {
			t.Fatalf("expected no tools in plain chat request, got %#v", lastReq.Payload["tools"])
		}
	})

	t.Run("non-streaming tool call returns complete tool_calls and suppresses thinking upstream flag", func(t *testing.T) {
		resp := postJSON(
			t,
			backend.URL+"/v1/chat/completions",
			`{"model":"qwen3.6-plus","messages":[{"role":"user","content":"Check the weather in Shanghai"}],"tools":[{"type":"function","function":{"name":"get_weather","parameters":{"type":"object","properties":{"location":{"type":"string"}},"required":["location"]}}}]}`,
			nil,
		)
		body := readBody(t, resp)
		if resp.StatusCode != http.StatusOK {
			t.Fatalf("expected 200, got %d: %s", resp.StatusCode, body)
		}

		var payload struct {
			Choices []struct {
				FinishReason *string `json:"finish_reason"`
				Message      struct {
					ToolCalls []struct {
						ID       string `json:"id"`
						Type     string `json:"type"`
						Function struct {
							Name      string `json:"name"`
							Arguments string `json:"arguments"`
						} `json:"function"`
					} `json:"tool_calls"`
				} `json:"message"`
			} `json:"choices"`
		}
		if err := json.Unmarshal([]byte(body), &payload); err != nil {
			t.Fatalf("invalid tool response JSON: %v", err)
		}
		if len(payload.Choices) != 1 || payload.Choices[0].FinishReason == nil || *payload.Choices[0].FinishReason != "tool_calls" {
			t.Fatalf("unexpected tool response: %#v", payload)
		}
		if len(payload.Choices[0].Message.ToolCalls) != 1 {
			t.Fatalf("expected 1 tool call, got %#v", payload.Choices[0].Message.ToolCalls)
		}
		toolCall := payload.Choices[0].Message.ToolCalls[0]
		if toolCall.Function.Name != "get_weather" || toolCall.Function.Arguments != `{"location":"Shanghai"}` {
			t.Fatalf("unexpected tool call payload: %#v", toolCall)
		}

		lastReq := upstream.lastChatRequest(t)
		enableThinking, ok := lastReq.Payload["enable_thinking"].(bool)
		if !ok || !enableThinking {
			t.Fatalf("expected enable_thinking=true when tools are present, got %#v", lastReq.Payload["enable_thinking"])
		}
		if len(asInterfaceSlice(lastReq.Payload["tools"])) != 1 {
			t.Fatalf("expected 1 upstream tool definition, got %#v", lastReq.Payload["tools"])
		}
	})

	t.Run("chat completions tool loop completes after tool message", func(t *testing.T) {
		before := upstream.chatRequestCount()

		firstResp := postJSON(
			t,
			backend.URL+"/v1/chat/completions",
			`{"model":"qwen3.6-plus","messages":[{"role":"user","content":"Explain codebase in detail"}],"tools":[{"type":"function","function":{"name":"Read","parameters":{"type":"object","properties":{"file_path":{"type":"string"}},"required":["file_path"]}}}]}`,
			nil,
		)
		firstBody := readBody(t, firstResp)
		if firstResp.StatusCode != http.StatusOK {
			t.Fatalf("expected first turn 200, got %d: %s", firstResp.StatusCode, firstBody)
		}

		var firstPayload struct {
			Choices []struct {
				FinishReason *string `json:"finish_reason"`
				Message      struct {
					ToolCalls []struct {
						ID       string `json:"id"`
						Type     string `json:"type"`
						Function struct {
							Name      string `json:"name"`
							Arguments string `json:"arguments"`
						} `json:"function"`
					} `json:"tool_calls"`
				} `json:"message"`
			} `json:"choices"`
		}
		if err := json.Unmarshal([]byte(firstBody), &firstPayload); err != nil {
			t.Fatalf("invalid first chat response JSON: %v", err)
		}
		if len(firstPayload.Choices) != 1 || firstPayload.Choices[0].FinishReason == nil || *firstPayload.Choices[0].FinishReason != "tool_calls" {
			t.Fatalf("unexpected first chat tool response: %#v", firstPayload)
		}
		if len(firstPayload.Choices[0].Message.ToolCalls) != 1 {
			t.Fatalf("expected one tool call in first turn, got %#v", firstPayload.Choices[0].Message.ToolCalls)
		}
		firstToolCall := firstPayload.Choices[0].Message.ToolCalls[0]
		if firstToolCall.ID != "call_qwen_read" || firstToolCall.Type != "function" {
			t.Fatalf("unexpected first tool call metadata: %#v", firstToolCall)
		}
		if firstToolCall.Function.Name != "Read" || firstToolCall.Function.Arguments != `{"file_path":"README.md"}` {
			t.Fatalf("unexpected first tool call function payload: %#v", firstToolCall)
		}
		if upstream.chatRequestCount() != before+1 {
			t.Fatalf("expected one upstream request after first chat turn, got before=%d after=%d", before, upstream.chatRequestCount())
		}

		firstUpstreamReq := upstream.lastChatRequest(t)
		enableThinking, ok := firstUpstreamReq.Payload["enable_thinking"].(bool)
		if !ok || !enableThinking {
			t.Fatalf("expected enable_thinking=true on first chat tool-use turn, got %#v", firstUpstreamReq.Payload["enable_thinking"])
		}
		if stream, _ := firstUpstreamReq.Payload["stream"].(bool); stream {
			t.Fatalf("did not expect upstream streaming on buffered chat turn, got payload %#v", firstUpstreamReq.Payload)
		}
		if firstUpstreamReq.Accept != "application/json" {
			t.Fatalf("expected upstream Accept application/json on first chat turn, got %q", firstUpstreamReq.Accept)
		}

		secondResp := postJSON(
			t,
			backend.URL+"/v1/chat/completions",
			`{"model":"qwen3.6-plus","messages":[{"role":"user","content":"Explain codebase in detail"},{"role":"assistant","content":"","tool_calls":[{"id":"call_qwen_read","type":"function","function":{"name":"Read","arguments":"{\"file_path\":\"README.md\"}"}}]},{"role":"tool","tool_call_id":"call_qwen_read","content":"README.md says this project exposes OpenAI and Anthropic compatible endpoints over a shared CIF routing core."},{"role":"user","content":"Finish the explanation briefly."}],"tools":[{"type":"function","function":{"name":"Read","parameters":{"type":"object","properties":{"file_path":{"type":"string"}},"required":["file_path"]}}}]}`,
			nil,
		)
		secondBody := readBody(t, secondResp)
		if secondResp.StatusCode != http.StatusOK {
			t.Fatalf("expected second turn 200, got %d: %s", secondResp.StatusCode, secondBody)
		}

		var secondPayload struct {
			Choices []struct {
				FinishReason *string `json:"finish_reason"`
				Message      struct {
					Content   string `json:"content"`
					ToolCalls []struct {
						ID string `json:"id"`
					} `json:"tool_calls"`
				} `json:"message"`
			} `json:"choices"`
		}
		if err := json.Unmarshal([]byte(secondBody), &secondPayload); err != nil {
			t.Fatalf("invalid second chat response JSON: %v", err)
		}
		if len(secondPayload.Choices) != 1 || secondPayload.Choices[0].FinishReason == nil || *secondPayload.Choices[0].FinishReason != "stop" {
			t.Fatalf("unexpected second chat response: %#v", secondPayload)
		}
		if len(secondPayload.Choices[0].Message.ToolCalls) != 0 {
			t.Fatalf("did not expect tool calls in second chat turn, got %#v", secondPayload.Choices[0].Message.ToolCalls)
		}
		wantFinalText := "This codebase exposes an OpenAI-compatible and Anthropic-compatible proxy. It normalizes requests into CIF, routes by model, and adapts provider-specific upstreams."
		if secondPayload.Choices[0].Message.Content != wantFinalText {
			t.Fatalf("unexpected second chat answer:\nwant: %q\ngot:  %q", wantFinalText, secondPayload.Choices[0].Message.Content)
		}
		if upstream.chatRequestCount() != before+2 {
			t.Fatalf("expected two upstream requests across chat tool loop, got before=%d after=%d", before, upstream.chatRequestCount())
		}

		secondUpstreamReq := upstream.lastChatRequest(t)
		enableThinking, ok = secondUpstreamReq.Payload["enable_thinking"].(bool)
		if !ok || !enableThinking {
			t.Fatalf("expected enable_thinking=true on second chat tool-result turn, got %#v", secondUpstreamReq.Payload["enable_thinking"])
		}
		if stream, _ := secondUpstreamReq.Payload["stream"].(bool); stream {
			t.Fatalf("did not expect upstream streaming on second buffered chat turn, got payload %#v", secondUpstreamReq.Payload)
		}
		if secondUpstreamReq.Accept != "application/json" {
			t.Fatalf("expected upstream Accept application/json on second chat turn, got %q", secondUpstreamReq.Accept)
		}

		upstreamMessages := asInterfaceSlice(secondUpstreamReq.Payload["messages"])
		if len(upstreamMessages) != 4 {
			t.Fatalf("expected 4 upstream messages on second chat turn, got %#v", upstreamMessages)
		}

		firstMsg, _ := upstreamMessages[0].(map[string]interface{})
		if role, _ := firstMsg["role"].(string); role != "user" {
			t.Fatalf("expected first upstream chat message role=user, got %#v", firstMsg)
		}
		if content, _ := firstMsg["content"].(string); content != "Explain codebase in detail" {
			t.Fatalf("unexpected first upstream chat content: %#v", firstMsg)
		}

		assistantMsg, _ := upstreamMessages[1].(map[string]interface{})
		if role, _ := assistantMsg["role"].(string); role != "assistant" {
			t.Fatalf("expected assistant upstream chat message, got %#v", assistantMsg)
		}
		if content, _ := assistantMsg["content"].(string); content != "" {
			t.Fatalf("expected assistant content placeholder to remain empty string, got %#v", assistantMsg["content"])
		}
		assistantToolCalls := asInterfaceSlice(assistantMsg["tool_calls"])
		if len(assistantToolCalls) != 1 {
			t.Fatalf("expected one upstream chat assistant tool_call, got %#v", assistantMsg)
		}
		assistantToolCall, _ := assistantToolCalls[0].(map[string]interface{})
		if id, _ := assistantToolCall["id"].(string); id != "call_qwen_read" {
			t.Fatalf("unexpected upstream chat tool call id: %#v", assistantToolCall)
		}
		assistantFunction, _ := assistantToolCall["function"].(map[string]interface{})
		if name, _ := assistantFunction["name"].(string); name != "Read" {
			t.Fatalf("unexpected upstream chat tool function: %#v", assistantToolCall)
		}
		if args, _ := assistantFunction["arguments"].(string); args != `{"file_path":"README.md"}` {
			t.Fatalf("unexpected upstream chat tool arguments: %#v", assistantToolCall)
		}

		toolMsg, _ := upstreamMessages[2].(map[string]interface{})
		if role, _ := toolMsg["role"].(string); role != "tool" {
			t.Fatalf("expected upstream chat tool message role=tool, got %#v", toolMsg)
		}
		if toolCallID, _ := toolMsg["tool_call_id"].(string); toolCallID != "call_qwen_read" {
			t.Fatalf("unexpected upstream chat tool_call_id: %#v", toolMsg)
		}
		if content, _ := toolMsg["content"].(string); !strings.Contains(content, "shared CIF routing core") {
			t.Fatalf("unexpected upstream chat tool result content: %#v", toolMsg)
		}

		finalUserMsg, _ := upstreamMessages[3].(map[string]interface{})
		if role, _ := finalUserMsg["role"].(string); role != "user" {
			t.Fatalf("expected final upstream chat message role=user, got %#v", finalUserMsg)
		}
		if content, _ := finalUserMsg["content"].(string); content != "Finish the explanation briefly." {
			t.Fatalf("unexpected final upstream chat content: %#v", finalUserMsg)
		}
	})

	t.Run("anthropic streaming tool use survives qwen reasoning content", func(t *testing.T) {
		before := upstream.chatRequestCount()
		resp := postJSON(
			t,
			backend.URL+"/v1/messages",
			`{"model":"qwen3.6-plus","stream":true,"max_tokens":128,"tools":[{"name":"get_weather","input_schema":{"type":"object","properties":{"location":{"type":"string"}},"required":["location"]}}],"messages":[{"role":"user","content":"Check the weather in Shanghai"}]}`,
			map[string]string{"anthropic-version": "2023-06-01"},
		)
		body := readBody(t, resp)
		if resp.StatusCode != http.StatusOK {
			t.Fatalf("expected 200, got %d: %s", resp.StatusCode, body)
		}
		if !strings.Contains(resp.Header.Get("Content-Type"), "text/event-stream") {
			t.Fatalf("expected event-stream content type, got %q", resp.Header.Get("Content-Type"))
		}

		events := parseSSEBody(t, body)
		if len(events) == 0 {
			t.Fatalf("expected Anthropic SSE events, got body: %s", body)
		}
		if got := events[0].Event; got != "message_start" {
			t.Fatalf("expected first SSE event to be message_start, got %q", got)
		}
		if got := events[len(events)-1].Event; got != "message_stop" {
			t.Fatalf("expected final SSE event to be message_stop, got %q", got)
		}

		var sawThinking bool
		var sawToolUseStart bool
		var stopReason string
		var partialJSON strings.Builder

		for _, evt := range events {
			if contentBlock, ok := evt.Data["content_block"].(map[string]interface{}); ok {
				if contentBlock["type"] == "thinking" {
					sawThinking = true
				}
				if contentBlock["type"] == "tool_use" && contentBlock["name"] == "get_weather" {
					sawToolUseStart = true
				}
			}

			if delta, ok := evt.Data["delta"].(map[string]interface{}); ok {
				switch delta["type"] {
				case "thinking_delta":
					sawThinking = true
				case "input_json_delta":
					if text, ok := delta["partial_json"].(string); ok {
						partialJSON.WriteString(text)
					}
				}
				if evt.Event == "message_delta" {
					if reason, ok := delta["stop_reason"].(string); ok {
						stopReason = reason
					}
				}
			}
		}

		if sawThinking {
			t.Fatalf("did not expect thinking blocks in default Anthropic stream: %s", body)
		}
		if !sawToolUseStart {
			t.Fatalf("expected tool_use block in Anthropic stream: %s", body)
		}
		if stopReason != "tool_use" {
			t.Fatalf("expected stop_reason=tool_use, got %q\nbody: %s", stopReason, body)
		}
		if partialJSON.String() != `{"location":"Shanghai"}` {
			t.Fatalf("expected full reconstructed tool arguments, got %q\nbody: %s", partialJSON.String(), body)
		}

		if upstream.chatRequestCount() != before+1 {
			t.Fatalf("expected exactly one upstream chat request for streaming tool call, got before=%d after=%d", before, upstream.chatRequestCount())
		}

		lastReq := upstream.lastChatRequest(t)
		enableThinking, ok := lastReq.Payload["enable_thinking"].(bool)
		if !ok || !enableThinking {
			t.Fatalf("expected enable_thinking=true in streaming tool request, got %#v", lastReq.Payload["enable_thinking"])
		}
		stream, _ := lastReq.Payload["stream"].(bool)
		if stream {
			t.Fatalf("expected buffered upstream request to disable stream, got payload %#v", lastReq.Payload)
		}
		if lastReq.Accept != "application/json" {
			t.Fatalf("expected buffered upstream Accept header, got %q", lastReq.Accept)
		}
	})

	t.Run("claude code style explain-codebase tool loop completes after tool_result", func(t *testing.T) {
		before := upstream.chatRequestCount()

		firstResp := postJSON(
			t,
			backend.URL+"/v1/messages",
			`{"model":"qwen3.6-plus","stream":true,"max_tokens":256,"tools":[{"name":"Read","input_schema":{"type":"object","properties":{"file_path":{"type":"string"}},"required":["file_path"]}}],"messages":[{"role":"user","content":"Explain codebase in detail"}]}`,
			map[string]string{"anthropic-version": "2023-06-01"},
		)
		firstBody := readBody(t, firstResp)
		if firstResp.StatusCode != http.StatusOK {
			t.Fatalf("expected first turn 200, got %d: %s", firstResp.StatusCode, firstBody)
		}

		firstEvents := parseSSEBody(t, firstBody)
		var firstToolUse bool
		var firstStopReason string
		var firstArgs strings.Builder
		for _, evt := range firstEvents {
			if contentBlock, ok := evt.Data["content_block"].(map[string]interface{}); ok {
				if contentBlock["type"] == "tool_use" && contentBlock["name"] == "Read" && contentBlock["id"] == "toolu_call_qwen_read" {
					firstToolUse = true
				}
				if contentBlock["type"] == "thinking" {
					t.Fatalf("did not expect thinking blocks in first Claude Code turn: %s", firstBody)
				}
			}
			if delta, ok := evt.Data["delta"].(map[string]interface{}); ok {
				switch delta["type"] {
				case "input_json_delta":
					if text, ok := delta["partial_json"].(string); ok {
						firstArgs.WriteString(text)
					}
				case "thinking_delta":
					t.Fatalf("did not expect thinking deltas in first Claude Code turn: %s", firstBody)
				}
				if evt.Event == "message_delta" {
					if reason, ok := delta["stop_reason"].(string); ok {
						firstStopReason = reason
					}
				}
			}
		}
		if !firstToolUse {
			t.Fatalf("expected first turn tool_use block, got body: %s", firstBody)
		}
		if firstStopReason != "tool_use" {
			t.Fatalf("expected first turn stop_reason=tool_use, got %q", firstStopReason)
		}
		if firstArgs.String() != `{"file_path":"README.md"}` {
			t.Fatalf("expected first turn tool args for README.md, got %q", firstArgs.String())
		}
		if upstream.chatRequestCount() != before+1 {
			t.Fatalf("expected one upstream request after first turn, got before=%d after=%d", before, upstream.chatRequestCount())
		}

		firstUpstreamReq := upstream.lastChatRequest(t)
		enableThinking, ok := firstUpstreamReq.Payload["enable_thinking"].(bool)
		if !ok || !enableThinking {
			t.Fatalf("expected enable_thinking=true on first tool-use turn, got %#v", firstUpstreamReq.Payload["enable_thinking"])
		}
		if firstUpstreamReq.Accept != "application/json" {
			t.Fatalf("expected buffered upstream Accept header on first turn, got %q", firstUpstreamReq.Accept)
		}
		if stream, _ := firstUpstreamReq.Payload["stream"].(bool); stream {
			t.Fatalf("expected first buffered upstream request to disable stream, got payload %#v", firstUpstreamReq.Payload)
		}

		secondResp := postJSON(
			t,
			backend.URL+"/v1/messages",
			`{"model":"qwen3.6-plus","stream":true,"max_tokens":512,"tools":[{"name":"Read","input_schema":{"type":"object","properties":{"file_path":{"type":"string"}},"required":["file_path"]}}],"messages":[{"role":"user","content":"Explain codebase in detail"},{"role":"assistant","content":[{"type":"tool_use","id":"toolu_call_qwen_read","name":"Read","input":{"file_path":"README.md"}}]},{"role":"user","content":[{"type":"tool_result","tool_use_id":"toolu_call_qwen_read","content":"README.md says this project exposes OpenAI and Anthropic compatible endpoints over a shared CIF routing core."}]}]}`,
			map[string]string{"anthropic-version": "2023-06-01"},
		)
		secondBody := readBody(t, secondResp)
		if secondResp.StatusCode != http.StatusOK {
			t.Fatalf("expected second turn 200, got %d: %s", secondResp.StatusCode, secondBody)
		}

		secondEvents := parseSSEBody(t, secondBody)
		if len(secondEvents) == 0 {
			t.Fatalf("expected second turn SSE events, got body: %s", secondBody)
		}

		var finalText strings.Builder
		var secondStopReason string
		var secondSawThinking bool
		var secondSawToolUse bool
		for _, evt := range secondEvents {
			if contentBlock, ok := evt.Data["content_block"].(map[string]interface{}); ok {
				if contentBlock["type"] == "thinking" {
					secondSawThinking = true
				}
				if contentBlock["type"] == "tool_use" {
					secondSawToolUse = true
				}
			}
			if delta, ok := evt.Data["delta"].(map[string]interface{}); ok {
				switch delta["type"] {
				case "thinking_delta":
					secondSawThinking = true
				case "text_delta":
					if text, ok := delta["text"].(string); ok {
						finalText.WriteString(text)
					}
				}
				if evt.Event == "message_delta" {
					if reason, ok := delta["stop_reason"].(string); ok {
						secondStopReason = reason
					}
				}
			}
		}

		if secondSawThinking {
			t.Fatalf("did not expect thinking blocks in final Claude Code turn: %s", secondBody)
		}
		if secondSawToolUse {
			t.Fatalf("did not expect another tool_use in final Claude Code turn: %s", secondBody)
		}
		if secondStopReason != "end_turn" {
			t.Fatalf("expected second turn stop_reason=end_turn, got %q\nbody: %s", secondStopReason, secondBody)
		}

		wantFinalText := "This codebase exposes an OpenAI-compatible and Anthropic-compatible proxy. It normalizes requests into CIF, routes by model, and adapts provider-specific upstreams."
		if finalText.String() != wantFinalText {
			t.Fatalf("unexpected final Claude Code answer:\nwant: %q\ngot:  %q", wantFinalText, finalText.String())
		}

		if upstream.chatRequestCount() != before+2 {
			t.Fatalf("expected two upstream requests across the tool loop, got before=%d after=%d", before, upstream.chatRequestCount())
		}

		secondUpstreamReq := upstream.lastChatRequest(t)
		enableThinking, ok = secondUpstreamReq.Payload["enable_thinking"].(bool)
		if !ok || !enableThinking {
			t.Fatalf("expected enable_thinking=true on second tool-result turn, got %#v", secondUpstreamReq.Payload["enable_thinking"])
		}
		if secondUpstreamReq.Accept != "application/json" {
			t.Fatalf("expected buffered upstream Accept header on second turn, got %q", secondUpstreamReq.Accept)
		}
		if stream, _ := secondUpstreamReq.Payload["stream"].(bool); stream {
			t.Fatalf("expected second buffered upstream request to disable stream, got payload %#v", secondUpstreamReq.Payload)
		}

		upstreamMessages := asInterfaceSlice(secondUpstreamReq.Payload["messages"])
		if len(upstreamMessages) != 3 {
			t.Fatalf("expected 3 upstream messages on second turn, got %#v", upstreamMessages)
		}

		firstMsg, _ := upstreamMessages[0].(map[string]interface{})
		if role, _ := firstMsg["role"].(string); role != "user" {
			t.Fatalf("expected first upstream message role=user, got %#v", firstMsg)
		}
		if content, _ := firstMsg["content"].(string); content != "Explain codebase in detail" {
			t.Fatalf("unexpected first upstream message content: %#v", firstMsg)
		}

		assistantMsg, _ := upstreamMessages[1].(map[string]interface{})
		if role, _ := assistantMsg["role"].(string); role != "assistant" {
			t.Fatalf("expected assistant upstream message, got %#v", assistantMsg)
		}
		assistantToolCalls := asInterfaceSlice(assistantMsg["tool_calls"])
		if len(assistantToolCalls) != 1 {
			t.Fatalf("expected one upstream assistant tool_call, got %#v", assistantMsg)
		}
		assistantToolCall, _ := assistantToolCalls[0].(map[string]interface{})
		if id, _ := assistantToolCall["id"].(string); id != "toolu_call_qwen_read" {
			t.Fatalf("unexpected upstream tool call id: %#v", assistantToolCall)
		}
		assistantFunction, _ := assistantToolCall["function"].(map[string]interface{})
		if name, _ := assistantFunction["name"].(string); name != "Read" {
			t.Fatalf("unexpected upstream tool function: %#v", assistantToolCall)
		}
		if args, _ := assistantFunction["arguments"].(string); args != `{"file_path":"README.md"}` {
			t.Fatalf("unexpected upstream tool arguments: %#v", assistantToolCall)
		}

		toolMsg, _ := upstreamMessages[2].(map[string]interface{})
		if role, _ := toolMsg["role"].(string); role != "tool" {
			t.Fatalf("expected upstream tool role message, got %#v", toolMsg)
		}
		if toolCallID, _ := toolMsg["tool_call_id"].(string); toolCallID != "toolu_call_qwen_read" {
			t.Fatalf("unexpected upstream tool_call_id: %#v", toolMsg)
		}
		if content, _ := toolMsg["content"].(string); !strings.Contains(content, "shared CIF routing core") {
			t.Fatalf("unexpected upstream tool result content: %#v", toolMsg)
		}
	})
}
