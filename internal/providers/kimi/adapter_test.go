package kimi

import (
	"omnillm/internal/cif"
	"testing"
)

func TestBuildOpenAIPayloadThinkingModelDisablesThinkingInsteadOfSendingToolChoice(t *testing.T) {
	desc := "Make an API call"
	payload := BuildOpenAIPayload("kimi-k2.6", []map[string]interface{}{{
		"role":    "user",
		"content": "Call the API",
	}}, &cif.CanonicalRequest{
		Model: "kimi-k2.6",
		Tools: []cif.CIFTool{{
			Name:        "api_call",
			Description: &desc,
			ParametersSchema: map[string]interface{}{
				"type":       "object",
				"properties": map[string]interface{}{"endpoint": map[string]interface{}{"type": "string"}},
			},
		}},
		ToolChoice: "required",
	}, false)
	if val, ok := payload["thinking"]; !ok || val != false {
		t.Fatalf("expected thinking=false for Kimi thinking tool turn, got %v (present=%v)", val, ok)
	}
	if _, ok := payload["tool_choice"]; ok {
		t.Fatalf("expected tool_choice to be omitted for Kimi thinking tool turn, got %#v", payload["tool_choice"])
	}
}

func TestEnsureReasoningContentInMessagesAddsEmptyFieldForToolCalls(t *testing.T) {
	messages := []map[string]interface{}{{
		"role": "assistant",
		"tool_calls": []map[string]interface{}{ {
			"id":   "call_1",
			"type": "function",
		}},
	}}
	EnsureReasoningContentInMessages(messages)
	if got, ok := messages[0]["reasoning_content"]; !ok || got != "" {
		t.Fatalf("expected empty reasoning_content to be injected, got %#v (present=%v)", got, ok)
	}
}
