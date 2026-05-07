package alibaba

import (
	"context"
	"omnillm/internal/cif"
	"omnillm/internal/providers/openaicompat"
	"testing"
)

func TestBuildRequestGLMMergesLeadingSystemIntoFirstUser(t *testing.T) {
	adapter := &Adapter{provider: NewProvider("alibaba-test", "Alibaba")}
	request := &cif.CanonicalRequest{
		Model:        "glm-5.1",
		SystemPrompt: stringPtr("Follow the system instructions."),
		Messages: []cif.CIFMessage{
			cif.CIFUserMessage{Role: "user", Content: []cif.CIFContentPart{cif.CIFTextPart{Type: "text", Text: "Hello"}}},
		},
	}

	chatReq, err := adapter.buildRequest(context.Background(), request, false)
	if err != nil {
		t.Fatalf("buildRequest() error = %v", err)
	}
	if len(chatReq.Messages) != 1 {
		t.Fatalf("expected 1 message after system merge, got %d", len(chatReq.Messages))
	}
	if chatReq.Messages[0].Role != "user" {
		t.Fatalf("expected first message role user, got %q", chatReq.Messages[0].Role)
	}
	content, ok := chatReq.Messages[0].Content.(string)
	if !ok {
		t.Fatalf("expected merged user content to be string, got %T", chatReq.Messages[0].Content)
	}
	want := "Follow the system instructions.\n\nHello"
	if content != want {
		t.Fatalf("expected merged content %q, got %q", want, content)
	}
}

func TestBuildRequestGLMWithToolsKeepsToolQuirksAndRemovesSystemRole(t *testing.T) {
	adapter := &Adapter{provider: NewProvider("alibaba-test", "Alibaba")}
	request := &cif.CanonicalRequest{
		Model:        "glm-5.1",
		SystemPrompt: stringPtr("Use tools carefully."),
		Tools: []cif.CIFTool{{
			Name:             "lookup",
			ParametersSchema: map[string]interface{}{"type": "object"},
		}},
		ToolChoice: "required",
		Messages: []cif.CIFMessage{
			cif.CIFUserMessage{Role: "user", Content: []cif.CIFContentPart{cif.CIFTextPart{Type: "text", Text: "Find it"}}},
			cif.CIFAssistantMessage{Role: "assistant", Content: []cif.CIFContentPart{cif.CIFToolCallPart{Type: "tool_call", ToolCallID: "call_1", ToolName: "lookup", ToolArguments: map[string]interface{}{"q": "Find it"}}}},
		},
	}

	chatReq, err := adapter.buildRequest(context.Background(), request, false)
	if err != nil {
		t.Fatalf("buildRequest() error = %v", err)
	}
	if len(chatReq.Messages) != 2 {
		t.Fatalf("expected 2 messages after system merge, got %d", len(chatReq.Messages))
	}
	if chatReq.Messages[0].Role != "user" {
		t.Fatalf("expected first message role user, got %q", chatReq.Messages[0].Role)
	}
	content, ok := chatReq.Messages[0].Content.(string)
	if !ok {
		t.Fatalf("expected merged user content to be string, got %T", chatReq.Messages[0].Content)
	}
	want := "Use tools carefully.\n\nFind it"
	if content != want {
		t.Fatalf("expected merged content %q, got %q", want, content)
	}
	if chatReq.ToolChoice != nil {
		t.Fatalf("expected GLM tool_choice to be stripped, got %#v", chatReq.ToolChoice)
	}
	assistant := chatReq.Messages[1]
	if assistant.Role != "assistant" {
		t.Fatalf("expected second message role assistant, got %q", assistant.Role)
	}
	assistantContent, ok := assistant.Content.(string)
	if !ok || assistantContent != "" {
		t.Fatalf("expected tool-only assistant content to be empty string, got %#v", assistant.Content)
	}
	if len(assistant.ToolCalls) != 1 {
		t.Fatalf("expected 1 tool call, got %d", len(assistant.ToolCalls))
	}
	if assistant.ToolCalls[0].CallID != "call_1" {
		t.Fatalf("expected DashScope call_id alias to match id, got %q", assistant.ToolCalls[0].CallID)
	}
}

func TestBuildRequestGLMSynthesizesUserMessageWhenNoUserTurnExists(t *testing.T) {
	adapter := &Adapter{provider: NewProvider("alibaba-test", "Alibaba")}
	request := &cif.CanonicalRequest{
		Model:        "glm-5.1",
		SystemPrompt: stringPtr("Keep responses concise."),
		Messages: []cif.CIFMessage{
			cif.CIFAssistantMessage{Role: "assistant", Content: []cif.CIFContentPart{cif.CIFToolCallPart{Type: "tool_call", ToolCallID: "call_1", ToolName: "lookup", ToolArguments: map[string]interface{}{"q": "status"}}}},
		},
	}

	chatReq, err := adapter.buildRequest(context.Background(), request, false)
	if err != nil {
		t.Fatalf("buildRequest() error = %v", err)
	}
	if len(chatReq.Messages) != 2 {
		t.Fatalf("expected synthetic user message plus assistant history, got %d messages", len(chatReq.Messages))
	}
	if chatReq.Messages[0].Role != "user" {
		t.Fatalf("expected first message role user, got %q", chatReq.Messages[0].Role)
	}
	content, ok := chatReq.Messages[0].Content.(string)
	if !ok || content != "Keep responses concise." {
		t.Fatalf("expected synthetic user content %q, got %#v", "Keep responses concise.", chatReq.Messages[0].Content)
	}
	for i, message := range chatReq.Messages {
		if message.Role == "system" {
			t.Fatalf("expected GLM payload to omit system role, found at index %d", i)
		}
	}
	assistant := chatReq.Messages[1]
	assistantContent, ok := assistant.Content.(string)
	if !ok || assistantContent != "" {
		t.Fatalf("expected assistant tool-only content to be empty string, got %#v", assistant.Content)
	}
	if len(assistant.ToolCalls) != 1 || assistant.ToolCalls[0].CallID != "call_1" {
		t.Fatalf("expected assistant tool call alias call_id=call_1, got %#v", assistant.ToolCalls)
	}
}

func TestBuildRequestGLMNormalizesToolHistoryWithoutCurrentTools(t *testing.T) {
	adapter := &Adapter{provider: NewProvider("alibaba-test", "Alibaba")}
	request := &cif.CanonicalRequest{
		Model: "glm-5.1",
		Messages: []cif.CIFMessage{
			cif.CIFAssistantMessage{Role: "assistant", Content: []cif.CIFContentPart{
				cif.CIFThinkingPart{Type: "thinking", Thinking: "hidden reasoning"},
				cif.CIFToolCallPart{Type: "tool_call", ToolCallID: "call_2", ToolName: "lookup", ToolArguments: map[string]interface{}{"q": "history"}},
			}},
			cif.CIFUserMessage{Role: "user", Content: []cif.CIFContentPart{cif.CIFTextPart{Type: "text", Text: "Continue"}}},
		},
	}

	chatReq, err := adapter.buildRequest(context.Background(), request, false)
	if err != nil {
		t.Fatalf("buildRequest() error = %v", err)
	}
	if len(chatReq.Messages) != 2 {
		t.Fatalf("expected assistant history and user turn, got %d messages", len(chatReq.Messages))
	}
	assistant := chatReq.Messages[0]
	if assistant.Role != "assistant" {
		t.Fatalf("expected first message role assistant, got %q", assistant.Role)
	}
	if assistant.ReasoningContent != "" {
		t.Fatalf("expected reasoning_content to be stripped, got %q", assistant.ReasoningContent)
	}
	assistantContent, ok := assistant.Content.(string)
	if !ok || assistantContent != "" {
		t.Fatalf("expected assistant tool-only content to be empty string, got %#v", assistant.Content)
	}
	if len(assistant.ToolCalls) != 1 || assistant.ToolCalls[0].CallID != "call_2" {
		t.Fatalf("expected assistant tool call alias call_id=call_2, got %#v", assistant.ToolCalls)
	}
}

func TestEnsureToolAssistantContent(t *testing.T) {
	messages := []openaicompat.Message{
		{Role: "assistant", ToolCalls: []openaicompat.ToolCall{{ID: "call_1"}}},
		{Role: "assistant", Content: "Hello"},
		{Role: "user", Content: "Hi"},
	}
	ensureToolAssistantContent(messages)
	if messages[0].Content != "" {
		t.Fatalf("expected empty string content for tool-only assistant, got %#v", messages[0].Content)
	}
	if messages[1].Content != "Hello" {
		t.Fatalf("expected 'Hello' unchanged, got %#v", messages[1].Content)
	}
	if messages[2].Content != "Hi" {
		t.Fatalf("expected 'Hi' unchanged, got %#v", messages[2].Content)
	}
}

func stringPtr(value string) *string {
	return &value
}
