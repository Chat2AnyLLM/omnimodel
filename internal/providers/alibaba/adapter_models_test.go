package alibaba

import (
	"encoding/json"
	"omnillm/internal/cif"
	"omnillm/internal/providers/openaicompat"
	"testing"
)

func TestBuildRequestAlibabaToolHistoryIncludesAssistantContent(t *testing.T) {
	tests := []struct {
		name       string
		model      string
		wantCallID bool
	}{
		{name: "deepseek v4 flash", model: "deepseek-v4-flash", wantCallID: true},
		{name: "qwen reasoning", model: "qwen3.6-flash"},
		{name: "prefixed model", model: "alibaba-test/deepseek-v4-flash", wantCallID: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			adapter := &Adapter{provider: NewProvider("alibaba-test", "Alibaba")}
			request := &cif.CanonicalRequest{
				Model: tt.model,
				Tools: []cif.CIFTool{{
					Name:             "get_hardware_info",
					ParametersSchema: map[string]any{"type": "object", "properties": map[string]any{}},
				}},
				Messages: []cif.CIFMessage{
					cif.CIFUserMessage{Role: "user", Content: []cif.CIFContentPart{cif.CIFTextPart{Type: "text", Text: "Show all hardware info for this machine."}}},
					cif.CIFAssistantMessage{Role: "assistant", Content: []cif.CIFContentPart{cif.CIFToolCallPart{Type: "tool_call", ToolCallID: "call_hardware", ToolName: "get_hardware_info", ToolArguments: map[string]interface{}{}}}},
					cif.CIFUserMessage{Role: "user", Content: []cif.CIFContentPart{cif.CIFToolResultPart{Type: "tool_result", ToolCallID: "call_hardware", Content: "CPU: Intel Core i9\nRAM: 64 GB\nGPU: NVIDIA RTX 4090"}}},
				},
			}

			chatReq, err := adapter.buildRequest(request, false)
			if err != nil {
				t.Fatalf("buildRequest() error = %v", err)
			}
			payload, err := openaicompat.Marshal(chatReq)
			if err != nil {
				t.Fatalf("Marshal() error = %v", err)
			}
			var body map[string]any
			if err := json.Unmarshal(payload, &body); err != nil {
				t.Fatalf("unmarshal payload: %v", err)
			}
			messages, ok := body["messages"].([]any)
			if !ok || len(messages) != 3 {
				t.Fatalf("messages = %#v, want 3 messages", body["messages"])
			}
			assistant, ok := messages[1].(map[string]any)
			if !ok {
				t.Fatalf("assistant message = %#v", messages[1])
			}
			if role, _ := assistant["role"].(string); role != "assistant" {
				t.Fatalf("assistant role = %#v", assistant["role"])
			}
			if content, exists := assistant["content"]; !exists || content != "" {
				t.Fatalf("assistant content = %#v, exists=%v; want explicit empty string", content, exists)
			}
			toolCalls, ok := assistant["tool_calls"].([]any)
			if !ok || len(toolCalls) != 1 {
				t.Fatalf("assistant tool_calls = %#v, want one tool call", assistant["tool_calls"])
			}
			toolCall, ok := toolCalls[0].(map[string]any)
			if !ok {
				t.Fatalf("assistant tool call = %#v", toolCalls[0])
			}
			_, hasCallID := toolCall["call_id"]
			if hasCallID != tt.wantCallID {
				t.Fatalf("call_id exists=%v, want %v in tool call %#v", hasCallID, tt.wantCallID, toolCall)
			}
		})
	}
}
func TestBuildRequestToolResultTurnToolRetentionByModel(t *testing.T) {
	tests := []struct {
		name      string
		model     string
		wantTools bool
	}{
		{name: "deepseek omits tools", model: "deepseek-v4-flash"},
		{name: "qwen keeps tools", model: "qwen3.6-flash", wantTools: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			adapter := &Adapter{provider: NewProvider("alibaba-test", "Alibaba")}
			request := &cif.CanonicalRequest{
				Model:      tt.model,
				ToolChoice: "auto",
				Tools: []cif.CIFTool{{
					Name:             "read_file",
					ParametersSchema: map[string]any{"type": "object", "properties": map[string]any{"path": map[string]any{"type": "string"}}},
				}},
				Messages: []cif.CIFMessage{
					cif.CIFUserMessage{Role: "user", Content: []cif.CIFContentPart{cif.CIFTextPart{Type: "text", Text: "Read go.mod"}}},
					cif.CIFAssistantMessage{Role: "assistant", Content: []cif.CIFContentPart{cif.CIFToolCallPart{Type: "tool_call", ToolCallID: "call_read", ToolName: "read_file", ToolArguments: map[string]interface{}{"path": "go.mod"}}}},
					cif.CIFUserMessage{Role: "user", Content: []cif.CIFContentPart{cif.CIFToolResultPart{Type: "tool_result", ToolCallID: "call_read", Content: "module omnillm"}}},
				},
			}

			chatReq, err := adapter.buildRequest(request, false)
			if err != nil {
				t.Fatalf("buildRequest() error = %v", err)
			}
			if hasTools := len(chatReq.Tools) > 0; hasTools != tt.wantTools {
				t.Fatalf("tools present=%v, want %v", hasTools, tt.wantTools)
			}
			if !tt.wantTools && chatReq.ToolChoice != nil {
				t.Fatalf("expected tool_choice to be omitted, got %#v", chatReq.ToolChoice)
			}
		})
	}
}

func TestBuildRequestNonGLMRetainsSystemRole(t *testing.T) {
	adapter := &Adapter{provider: NewProvider("alibaba-test", "Alibaba")}
	request := &cif.CanonicalRequest{
		Model:        "qwen3.5-plus",
		SystemPrompt: stringPtr("Stay as system."),
		Messages: []cif.CIFMessage{
			cif.CIFUserMessage{Role: "user", Content: []cif.CIFContentPart{cif.CIFTextPart{Type: "text", Text: "Hi"}}},
		},
	}

	chatReq, err := adapter.buildRequest(request, false)
	if err != nil {
		t.Fatalf("buildRequest() error = %v", err)
	}
	if len(chatReq.Messages) == 0 {
		t.Fatal("expected messages to be present")
	}
	if chatReq.Messages[0].Role != "system" {
		t.Fatalf("expected non-GLM request to retain system role, got %q", chatReq.Messages[0].Role)
	}
}

func TestBuildRequestDeepSeekV4FlashWithToolsMatchesDefaultReasoningHandling(t *testing.T) {
	adapter := &Adapter{provider: NewProvider("alibaba-test", "Alibaba")}
	request := &cif.CanonicalRequest{
		Model: "deepseek-v4-flash",
		Messages: []cif.CIFMessage{
			cif.CIFUserMessage{Role: "user", Content: []cif.CIFContentPart{cif.CIFTextPart{Type: "text", Text: "Hi"}}},
		},
		ToolChoice: "required",
		Tools: []cif.CIFTool{{
			Name:             "show_disks",
			Description:      stringPtr("Show disks"),
			ParametersSchema: map[string]any{"type": "object"},
		}},
	}

	chatReq, err := adapter.buildRequest(request, false)
	if err != nil {
		t.Fatalf("buildRequest() error = %v", err)
	}
	if _, ok := chatReq.Extras["thinking"]; ok {
		t.Fatal("expected deepseek-v4-flash to avoid DashScope-specific thinking override")
	}
	if _, ok := chatReq.Extras["enable_thinking"]; !ok {
		t.Fatal("expected deepseek-v4-flash to keep the default non-reasoning enable_thinking flag when tools are present")
	}
	if got := chatReq.Extras["enable_thinking"]; got != false {
		t.Fatalf("expected deepseek-v4-flash enable_thinking=false, got %#v", got)
	}
	if chatReq.ToolChoice != "required" {
		t.Fatalf("expected deepseek-v4-flash tool_choice to be preserved like default handling, got %#v", chatReq.ToolChoice)
	}
}
