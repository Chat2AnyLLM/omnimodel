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

// TestAnthropicSDKDispatchConvertsRequest verifies that AnthropicSDKDispatch
// sends the correct Anthropic Messages API payload and converts the response
// back to an agent-native messages response.
func TestAnthropicSDKDispatchConvertsRequest(t *testing.T) {
	// Minimal Anthropic Messages API response.
	responseBody := map[string]any{
		"id":          "msg_test123",
		"type":        "message",
		"role":        "assistant",
		"model":       "claude-opus-4-5",
		"stop_reason": "end_turn",
		"usage":       map[string]any{"input_tokens": 10, "output_tokens": 5},
		"content": []map[string]any{
			{"type": "text", "text": "Hello from Anthropic SDK!"},
		},
	}

	var capturedBody map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/messages" {
			http.NotFound(w, r)
			return
		}
		if err := json.NewDecoder(r.Body).Decode(&capturedBody); err != nil {
			http.Error(w, "bad request", http.StatusBadRequest)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(responseBody)
	}))
	defer srv.Close()

	dispatch := AnthropicSDKDispatch("test-api-key", srv.URL)

	req := testMessagesRequest("claude-opus-4-5", testUserMessage("Hello!"))

	respCh, err := dispatch(context.Background(), req)
	if err != nil {
		t.Fatalf("dispatch error: %v", err)
	}

	var resp *MessagesResponse
	for r := range respCh {
		resp = r
	}
	if resp == nil {
		t.Fatal("got nil response")
	}
	if resp.ID != "msg_test123" {
		t.Fatalf("id = %q, want msg_test123", resp.ID)
	}
	if resp.StopReason != StopReasonEndTurn {
		t.Fatalf("stop_reason = %v, want EndTurn", resp.StopReason)
	}
	if len(resp.Content) != 1 {
		t.Fatalf("content blocks = %d, want 1", len(resp.Content))
	}
	tp := resp.Content[0]
	if tp.Type != "text" {
		t.Fatalf("content[0] type = %q, want text", tp.Type)
	}
	if !strings.Contains(tp.Text, "Anthropic SDK") {
		t.Fatalf("text = %q", tp.Text)
	}

	// Verify the request payload
	if capturedBody["model"] != "claude-opus-4-5" {
		t.Fatalf("request model = %#v", capturedBody["model"])
	}
	msgs, _ := capturedBody["messages"].([]any)
	if len(msgs) != 1 {
		t.Fatalf("request messages count = %d, want 1", len(msgs))
	}
}

// TestAnthropicSDKDispatchToolUseRoundTrip verifies that tool_use blocks in
// the response is correctly converted to a tool_use content block.
func TestAnthropicSDKDispatchToolUseRoundTrip(t *testing.T) {
	responseBody := map[string]any{
		"id":          "msg_tool",
		"type":        "message",
		"role":        "assistant",
		"model":       "claude-opus-4-5",
		"stop_reason": "tool_use",
		"usage":       map[string]any{"input_tokens": 15, "output_tokens": 8},
		"content": []map[string]any{
			{
				"type":  "tool_use",
				"id":    "toolu_01",
				"name":  "bash",
				"input": map[string]any{"command": "ls -la"},
			},
		},
	}

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(responseBody)
	}))
	defer srv.Close()

	dispatch := AnthropicSDKDispatch("test-api-key", srv.URL)

	req := testMessagesRequest("claude-opus-4-5", testUserMessage("Run ls"))
	req.Tools = []tools.ToolDefinition{testToolDefinition("bash", stringPtr("Execute shell commands"), map[string]any{"type": "object", "properties": map[string]any{"command": map[string]any{"type": "string"}}})}

	respCh, err := dispatch(context.Background(), req)
	if err != nil {
		t.Fatalf("dispatch error: %v", err)
	}

	var resp *MessagesResponse
	for r := range respCh {
		resp = r
	}
	if resp == nil {
		t.Fatal("got nil response")
	}
	if resp.StopReason != StopReasonToolUse {
		t.Fatalf("stop_reason = %v, want ToolUse", resp.StopReason)
	}
	if len(resp.Content) != 1 {
		t.Fatalf("content blocks = %d, want 1", len(resp.Content))
	}
	tc := resp.Content[0]
	if tc.Type != "tool_use" {
		t.Fatalf("content[0] type = %q, want tool_use", tc.Type)
	}
	if tc.ID != "toolu_01" {
		t.Fatalf("tool call id = %q, want toolu_01", tc.ID)
	}
	if tc.Name != "bash" {
		t.Fatalf("tool name = %q, want bash", tc.Name)
	}
}

// TestAnthropicSDKDispatchSystemPrompt verifies that a system prompt in the
// agent-native request is forwarded correctly to the Anthropic API.
func TestAnthropicSDKDispatchSystemPrompt(t *testing.T) {
	var capturedBody map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewDecoder(r.Body).Decode(&capturedBody)
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"id": "msg_sys", "type": "message", "role": "assistant", "model": "claude-opus-4-5",
			"stop_reason": "end_turn", "usage": map[string]any{"input_tokens": 5, "output_tokens": 2},
			"content": []map[string]any{{"type": "text", "text": "ok"}},
		})
	}))
	defer srv.Close()

	sys := "You are a helpful coding assistant."
	dispatch := AnthropicSDKDispatch("test-api-key", srv.URL)
	_, err := dispatch(context.Background(), &MessagesRequest{Model: "claude-opus-4-5", MaxTokens: 4096, System: []ContentBlock{TextBlock(sys)}, Messages: []Message{testUserMessage("hi")}})
	if err != nil {
		t.Fatalf("dispatch error: %v", err)
	}

	// Anthropic sends system as an array of text blocks
	systemField := capturedBody["system"]
	if systemField == nil {
		t.Fatal("system field missing from request body")
	}
}

// TestCifToAnthropicParamsDefaultModel verifies the default model fallback.
func TestAnthropicParamsDefaultModel(t *testing.T) {
	params, err := anthropicParamsFromRequest(testMessagesRequest("", testUserMessage("hi")))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if string(params.Model) != "claude-opus-4-5" {
		t.Fatalf("default model = %q, want claude-opus-4-5", params.Model)
	}
}

// TestCifToAnthropicParamsMaxTokensOverride verifies that MaxTokens is forwarded.
func TestAnthropicParamsMaxTokensOverride(t *testing.T) {
	params, err := anthropicParamsFromRequest(&MessagesRequest{Model: "claude-haiku-3-5", MaxTokens: 1234})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if params.MaxTokens != 1234 {
		t.Fatalf("max_tokens = %d, want 1234", params.MaxTokens)
	}
}

func TestAnthropicParamsRespectsToolChoiceRequired(t *testing.T) {
	req := testMessagesRequest("claude-opus-4-5", testUserMessage("list files"))
	req.Tools = []tools.ToolDefinition{testToolDefinition("bash", stringPtr("Execute shell commands"), map[string]any{"type": "object", "properties": map[string]any{"command": map[string]any{"type": "string"}}})}
	req.ToolChoice = "required"
	params, err := anthropicParamsFromRequest(req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if params.ToolChoice.OfAny == nil {
		t.Fatalf("expected required tool choice to map to OfAny, got %#v", params.ToolChoice)
	}
}

func TestAnthropicParamsRespectsToolChoiceNone(t *testing.T) {
	req := testMessagesRequest("claude-opus-4-5", testUserMessage("answer directly"))
	req.Tools = []tools.ToolDefinition{testToolDefinition("bash", stringPtr("Execute shell commands"), map[string]any{"type": "object", "properties": map[string]any{"command": map[string]any{"type": "string"}}})}
	req.ToolChoice = "none"
	params, err := anthropicParamsFromRequest(req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if params.ToolChoice.OfNone == nil {
		t.Fatalf("expected none tool choice to map to OfNone, got %#v", params.ToolChoice)
	}
}

func TestAnthropicParamsDefaultsToolChoiceToAutoWhenUnset(t *testing.T) {
	req := testMessagesRequest("claude-opus-4-5", testUserMessage("list files"))
	req.Tools = []tools.ToolDefinition{testToolDefinition("bash", stringPtr("Execute shell commands"), map[string]any{"type": "object", "properties": map[string]any{"command": map[string]any{"type": "string"}}})}
	params, err := anthropicParamsFromRequest(req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if params.ToolChoice.OfAuto == nil {
		t.Fatalf("expected unset tool choice to default to OfAuto, got %#v", params.ToolChoice)
	}
}

func TestAnthropicParamsSupportsSpecificToolChoice(t *testing.T) {
	req := testMessagesRequest("claude-opus-4-5", testUserMessage("use bash"))
	req.Tools = []tools.ToolDefinition{testToolDefinition("bash", stringPtr("Execute shell commands"), map[string]any{"type": "object", "properties": map[string]any{"command": map[string]any{"type": "string"}}})}
	req.ToolChoice = map[string]any{"type": "function", "functionName": "bash"}
	params, err := anthropicParamsFromRequest(req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if params.ToolChoice.OfTool == nil || params.ToolChoice.OfTool.Name != "bash" {
		t.Fatalf("expected specific tool choice to map to bash, got %#v", params.ToolChoice)
	}
}

func TestAnthropicParamsToolSchemaUsesPropertiesFieldOnly(t *testing.T) {
	req := testMessagesRequest("claude-opus-4-5", testUserMessage("show config"))
	req.Tools = []tools.ToolDefinition{testToolDefinition("config", stringPtr("Read config"), map[string]any{
		"type": "object",
		"properties": map[string]any{
			"action": map[string]any{
				"type": "string",
				"enum": []string{"get", "set", "list"},
			},
			"key": map[string]any{"type": "string"},
		},
		"required": []string{"action"},
	})}

	payload, err := buildAnthropicMessagesJSON("claude-opus-4-5", req, false)
	if err != nil {
		t.Fatalf("buildAnthropicMessagesJSON() error = %v", err)
	}

	var body map[string]any
	if err := json.Unmarshal(payload, &body); err != nil {
		t.Fatalf("unmarshal payload: %v", err)
	}
	toolsRaw, _ := body["tools"].([]any)
	if len(toolsRaw) != 1 {
		t.Fatalf("tools = %#v", body["tools"])
	}
	tool, _ := toolsRaw[0].(map[string]any)
	inputSchema, _ := tool["input_schema"].(map[string]any)
	properties, _ := inputSchema["properties"].(map[string]any)
	if _, ok := properties["action"].(map[string]any); !ok {
		t.Fatalf("input_schema.properties.action = %#v, want object schema", properties["action"])
	}
	if nested, ok := properties["properties"]; ok {
		t.Fatalf("input_schema.properties must not contain nested full schema: %#v", nested)
	}
	required, _ := inputSchema["required"].([]any)
	if len(required) != 1 || required[0] != "action" {
		t.Fatalf("input_schema.required = %#v, want [\"action\"]", inputSchema["required"])
	}
}
