package alibaba

import (
	"omnillm/internal/cif"
	"testing"
)

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
