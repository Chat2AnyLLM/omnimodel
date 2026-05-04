package openaicompat

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"omnillm/internal/cif"
	"strings"
	"testing"
)

func TestBuildChatRequest_AppliesDefaultsToolsAndSystemPrompt(t *testing.T) {
	defaultTemp := 0.3
	defaultTopP := 0.9
	maxTokens := 64
	userID := "  " + strings.Repeat("u", 80) + "  "
	request := &cif.CanonicalRequest{
		Model:        "gpt-4o",
		SystemPrompt: stringPtr("Be terse."),
		Messages: []cif.CIFMessage{
			cif.CIFUserMessage{Role: "user", Content: []cif.CIFContentPart{cif.CIFTextPart{Type: "text", Text: "Hello"}}},
		},
		MaxTokens: &maxTokens,
		UserID:    &userID,
		Tools: []cif.CIFTool{{
			Name:             "get_weather",
			ParametersSchema: map[string]interface{}{"type": "object"},
		}},
		ToolChoice: "auto",
	}

	out, err := BuildChatRequest("gpt-4o", request, true, Config{
		DefaultTemperature:   &defaultTemp,
		DefaultTopP:          &defaultTopP,
		IncludeUsageInStream: true,
		Extras:               map[string]interface{}{"enable_thinking": true},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if out.Model != "gpt-4o" || !out.Stream {
		t.Fatalf("unexpected request: %#v", out)
	}
	if out.Temperature == nil || *out.Temperature != defaultTemp {
		t.Fatalf("unexpected temperature: %#v", out.Temperature)
	}
	if out.TopP == nil || *out.TopP != defaultTopP {
		t.Fatalf("unexpected top_p: %#v", out.TopP)
	}
	if out.MaxTokens == nil || *out.MaxTokens != maxTokens {
		t.Fatalf("unexpected max tokens: %#v", out.MaxTokens)
	}
	if out.User == nil || len(*out.User) != 64 {
		t.Fatalf("unexpected user id: %#v", out.User)
	}
	if len(out.Messages) != 2 || out.Messages[0].Role != "system" {
		t.Fatalf("unexpected messages: %#v", out.Messages)
	}
	if len(out.Tools) != 1 || out.Tools[0].Function.Name != "get_weather" {
		t.Fatalf("unexpected tools: %#v", out.Tools)
	}
	if out.StreamOptions == nil || !out.StreamOptions.IncludeUsage {
		t.Fatalf("unexpected stream options: %#v", out.StreamOptions)
	}
	if out.Extras["enable_thinking"] != true {
		t.Fatalf("unexpected extras: %#v", out.Extras)
	}
}

func TestMarshal_MergesExtras(t *testing.T) {
	body, err := Marshal(&ChatRequest{
		Model:    "gpt-4o",
		Messages: []Message{{Role: "user", Content: "Hello"}},
		Extras:   map[string]interface{}{"enable_thinking": true},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	text := string(body)
	if !strings.Contains(text, `"enable_thinking":true`) {
		t.Fatalf("expected extras in marshaled body: %s", text)
	}
}

func TestMarshal_ReappliesUserTruncationAfterExtrasMerge(t *testing.T) {
	body, err := Marshal(&ChatRequest{
		Model:    "gpt-4o",
		Messages: []Message{{Role: "user", Content: "Hello"}},
		Extras: map[string]interface{}{
			"user": "  " + strings.Repeat("x", 80) + "  ",
		},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var payload map[string]interface{}
	if err := json.Unmarshal(body, &payload); err != nil {
		t.Fatalf("unexpected json error: %v", err)
	}
	user, _ := payload["user"].(string)
	if len(user) != 64 {
		t.Fatalf("expected truncated user length 64, got %d", len(user))
	}
}

func TestBuildChatRequest_AssistantToolCallsIncludeEmptyContent(t *testing.T) {
	request := &cif.CanonicalRequest{
		Model: "deepseek-v4-flash",
		Messages: []cif.CIFMessage{
			cif.CIFAssistantMessage{
				Role: "assistant",
				Content: []cif.CIFContentPart{
					cif.CIFToolCallPart{
						Type:          "tool_call",
						ToolCallID:    "call_ls",
						ToolName:      "ls",
						ToolArguments: map[string]interface{}{"path": "."},
					},
				},
			},
		},
	}

	out, err := BuildChatRequest("deepseek-v4-flash", request, false, Config{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(out.Messages) != 1 {
		t.Fatalf("unexpected messages: %#v", out.Messages)
	}
	if content, ok := out.Messages[0].Content.(string); !ok || content != "" {
		t.Fatalf("expected assistant tool-call content to be empty string, got %#v", out.Messages[0].Content)
	}
}

func TestExecute_ParsesChatResponse(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"id":"chat_123","model":"gpt-4o","choices":[{"message":{"content":"pong"},"finish_reason":"stop"}],"usage":{"prompt_tokens":3,"completion_tokens":1}}`)
	}))
	defer srv.Close()

	resp, err := Execute(context.Background(), srv.URL, map[string]string{"Authorization": "Bearer test"}, &ChatRequest{
		Model:    "gpt-4o",
		Messages: []Message{{Role: "user", Content: "ping"}},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.ID != "chat_123" || resp.Model != "gpt-4o" {
		t.Fatalf("unexpected response identity: %#v", resp)
	}
	if len(resp.Content) != 1 {
		t.Fatalf("unexpected content: %#v", resp.Content)
	}
}

func TestExecute_ReturnsAPIErrorDetails(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "bad upstream", http.StatusBadGateway)
	}))
	defer srv.Close()

	_, err := Execute(context.Background(), srv.URL, nil, &ChatRequest{Model: "gpt-4o"})
	if err == nil || !strings.Contains(err.Error(), "502") {
		t.Fatalf("expected wrapped API error, got %v", err)
	}
}

func stringPtr(s string) *string { return &s }
