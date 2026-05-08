package agent

import toolspkg "omnillm/internal/tools"

func testUserMessage(text string) Message {
	return Message{Role: "user", Content: []ContentBlock{TextBlock(text)}}
}

func testAssistantMessage(text string) Message {
	return Message{Role: "assistant", Content: []ContentBlock{TextBlock(text)}}
}

func testMessagesRequest(model string, messages ...Message) *MessagesRequest {
	return &MessagesRequest{Model: model, MaxTokens: 4096, Messages: messages}
}

func testToolDefinition(name string, description *string, schema map[string]any) toolspkg.ToolDefinition {
	if schema == nil {
		schema = map[string]any{"type": "object", "properties": map[string]any{}}
	}
	return toolspkg.ToolDefinition{Name: name, Description: description, InputSchema: schema}
}