package agent

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"omnillm/internal/cif"
)

// TestAnthropicSDKDispatchConvertsRequest verifies that AnthropicSDKDispatch
// sends the correct Anthropic Messages API payload and converts the response
// back to a CIF CanonicalResponse.
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

	req := &cif.CanonicalRequest{
		Model: "claude-opus-4-5",
		Messages: []cif.CIFMessage{
			cif.CIFUserMessage{
				Role: "user",
				Content: []cif.CIFContentPart{
					cif.CIFTextPart{Type: "text", Text: "Hello!"},
				},
			},
		},
	}

	respCh, err := dispatch(context.Background(), req)
	if err != nil {
		t.Fatalf("dispatch error: %v", err)
	}

	var resp *cif.CanonicalResponse
	for r := range respCh {
		resp = r
	}
	if resp == nil {
		t.Fatal("got nil response")
	}
	if resp.ID != "msg_test123" {
		t.Fatalf("id = %q, want msg_test123", resp.ID)
	}
	if resp.StopReason != cif.StopReasonEndTurn {
		t.Fatalf("stop_reason = %v, want EndTurn", resp.StopReason)
	}
	if len(resp.Content) != 1 {
		t.Fatalf("content blocks = %d, want 1", len(resp.Content))
	}
	tp, ok := resp.Content[0].(cif.CIFTextPart)
	if !ok {
		t.Fatalf("content[0] is not CIFTextPart: %T", resp.Content[0])
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
// the response are correctly converted to CIFToolCallPart.
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

	req := &cif.CanonicalRequest{
		Model: "claude-opus-4-5",
		Messages: []cif.CIFMessage{
			cif.CIFUserMessage{
				Role:    "user",
				Content: []cif.CIFContentPart{cif.CIFTextPart{Type: "text", Text: "Run ls"}},
			},
		},
		Tools: []cif.CIFTool{
			{
				Name:             "bash",
				Description:      stringPtr("Execute shell commands"),
				ParametersSchema: map[string]any{"type": "object", "properties": map[string]any{"command": map[string]any{"type": "string"}}},
			},
		},
	}

	respCh, err := dispatch(context.Background(), req)
	if err != nil {
		t.Fatalf("dispatch error: %v", err)
	}

	var resp *cif.CanonicalResponse
	for r := range respCh {
		resp = r
	}
	if resp == nil {
		t.Fatal("got nil response")
	}
	if resp.StopReason != cif.StopReasonToolUse {
		t.Fatalf("stop_reason = %v, want ToolUse", resp.StopReason)
	}
	if len(resp.Content) != 1 {
		t.Fatalf("content blocks = %d, want 1", len(resp.Content))
	}
	tc, ok := resp.Content[0].(cif.CIFToolCallPart)
	if !ok {
		t.Fatalf("content[0] is not CIFToolCallPart: %T", resp.Content[0])
	}
	if tc.ToolCallID != "toolu_01" {
		t.Fatalf("tool call id = %q, want toolu_01", tc.ToolCallID)
	}
	if tc.ToolName != "bash" {
		t.Fatalf("tool name = %q, want bash", tc.ToolName)
	}
}

// TestAnthropicSDKDispatchSystemPrompt verifies that a system prompt in the
// CIF request is forwarded correctly to the Anthropic API.
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
	_, err := dispatch(context.Background(), &cif.CanonicalRequest{
		Model:        "claude-opus-4-5",
		SystemPrompt: &sys,
		Messages: []cif.CIFMessage{
			cif.CIFUserMessage{Role: "user", Content: []cif.CIFContentPart{cif.CIFTextPart{Type: "text", Text: "hi"}}},
		},
	})
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
func TestCifToAnthropicParamsDefaultModel(t *testing.T) {
	req := &cif.CanonicalRequest{
		Model: "", // empty — should default to claude-opus-4-5
		Messages: []cif.CIFMessage{
			cif.CIFUserMessage{Role: "user", Content: []cif.CIFContentPart{cif.CIFTextPart{Type: "text", Text: "hi"}}},
		},
	}
	params, err := cifToAnthropicParams(req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if string(params.Model) != "claude-opus-4-5" {
		t.Fatalf("default model = %q, want claude-opus-4-5", params.Model)
	}
}

// TestCifToAnthropicParamsMaxTokensOverride verifies that MaxTokens is forwarded.
func TestCifToAnthropicParamsMaxTokensOverride(t *testing.T) {
	maxTok := 1234
	req := &cif.CanonicalRequest{
		Model:     "claude-haiku-3-5",
		MaxTokens: &maxTok,
		Messages:  []cif.CIFMessage{},
	}
	params, err := cifToAnthropicParams(req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if params.MaxTokens != 1234 {
		t.Fatalf("max_tokens = %d, want 1234", params.MaxTokens)
	}
}
