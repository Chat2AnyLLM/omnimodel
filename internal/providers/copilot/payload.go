package copilot

import (
	"encoding/json"
	"fmt"
	"strings"

	"omnillm/internal/cif"
	"omnillm/internal/providers/shared"
)

func sanitizeCopilotUserID(userID string) string {
	trimmed := strings.TrimSpace(userID)
	if len(trimmed) <= copilotMaxUserIDLength {
		return trimmed
	}
	return trimmed[:copilotMaxUserIDLength]
}

func convertCanonicalToolChoiceToOpenAI(toolChoice interface{}, toolNameMapper *copilotToolNameMapper) interface{} {
	switch choice := toolChoice.(type) {
	case string:
		switch choice {
		case "none", "auto", "required":
			return choice
		default:
			return nil
		}
	case map[string]interface{}:
		functionName, _ := choice["functionName"].(string)
		if functionName == "" {
			if function, ok := choice["function"].(map[string]interface{}); ok {
				functionName, _ = function["name"].(string)
			}
		}
		if functionName == "" {
			return nil
		}
		return map[string]interface{}{
			"type": "function",
			"function": map[string]interface{}{
				"name": toolNameMapper.toUpstream(functionName),
			},
		}
	default:
		return nil
	}
}

func copilotModelUsesMaxCompletionTokens(model string) bool {
	lower := strings.ToLower(model)
	return strings.HasPrefix(lower, "o1") ||
		strings.HasPrefix(lower, "o3") ||
		strings.HasPrefix(lower, "o4") ||
		shared.IsGPT5Family(model)
}

func (a *CopilotAdapter) convertCIFToOpenAI(request *cif.CanonicalRequest, toolNameMapper *copilotToolNameMapper) map[string]interface{} {
	payload := map[string]interface{}{
		"model":    a.RemapModel(request.Model),
		"messages": a.convertCIFMessagesToOpenAI(request.Messages, toolNameMapper),
		"stream":   request.Stream,
	}

	if request.Temperature != nil {
		if !shared.IsReasoningModel(payload["model"].(string)) {
			payload["temperature"] = *request.Temperature
		}
	}
	if request.TopP != nil {
		if !shared.IsReasoningModel(payload["model"].(string)) {
			payload["top_p"] = *request.TopP
		}
	}
	if request.MaxTokens != nil {
		if copilotModelUsesMaxCompletionTokens(payload["model"].(string)) {
			payload["max_completion_tokens"] = *request.MaxTokens
		} else {
			payload["max_tokens"] = *request.MaxTokens
		}
	}
	if len(request.Stop) > 0 {
		payload["stop"] = request.Stop
	}
	if request.UserID != nil {
		payload["user"] = sanitizeCopilotUserID(*request.UserID)
	}

	if len(request.Tools) > 0 {
		var tools []map[string]interface{}
		for _, tool := range request.Tools {
			openaiTool := map[string]interface{}{
				"type": "function",
				"function": map[string]interface{}{
					"name":       toolNameMapper.toUpstream(tool.Name),
					"parameters": shared.NormalizeToolParameters(tool.ParametersSchema),
				},
			}
			if tool.Description != nil {
				openaiTool["function"].(map[string]interface{})["description"] = *tool.Description
			}
			tools = append(tools, openaiTool)
		}
		payload["tools"] = tools
	}

	if request.ToolChoice != nil {
		model := payload["model"].(string)
		// Reasoning models (o1, o3, gpt-5 family) reject tool_choice with
		// specific function names. Only pass through simple string choices
		// like "auto" / "required" / "none" for them.
		if toolChoice := convertCanonicalToolChoiceToOpenAI(request.ToolChoice, toolNameMapper); toolChoice != nil {
			if shared.IsReasoningModel(model) {
				if _, isString := toolChoice.(string); isString {
					payload["tool_choice"] = toolChoice
				}
				// else: drop non-string tool_choice for reasoning models
			} else {
				payload["tool_choice"] = toolChoice
			}
		}
	}

	return payload
}

func (a *CopilotAdapter) convertCIFMessagesToOpenAI(messages []cif.CIFMessage, toolNameMapper *copilotToolNameMapper) []map[string]interface{} {
	var openaiMessages []map[string]interface{}

	for _, msg := range messages {
		switch m := msg.(type) {
		case cif.CIFSystemMessage:
			openaiMessages = append(openaiMessages, map[string]interface{}{
				"role":    "system",
				"content": m.Content,
			})

		case cif.CIFUserMessage:
			openaiMsg := map[string]interface{}{
				"role": "user",
			}

			if len(m.Content) == 1 {
				if textPart, ok := m.Content[0].(cif.CIFTextPart); ok {
					openaiMsg["content"] = textPart.Text
					openaiMessages = append(openaiMessages, openaiMsg)
					continue
				}
			}

			var contentParts []map[string]interface{}
			for _, part := range m.Content {
				if toolResult, ok := part.(cif.CIFToolResultPart); ok {
					openaiMessages = append(openaiMessages, map[string]interface{}{
						"role":         "tool",
						"tool_call_id": toolResult.ToolCallID,
						"content":      toolResult.Content,
					})
					continue
				}

				if openaiPart := a.convertCIFPartToOpenAI(part); openaiPart != nil {
					contentParts = append(contentParts, openaiPart)
				}
			}

			if len(contentParts) > 0 {
				openaiMsg["content"] = contentParts
				openaiMessages = append(openaiMessages, openaiMsg)
			}

		case cif.CIFAssistantMessage:
			openaiMsg := map[string]interface{}{
				"role": "assistant",
			}

			var textBuf strings.Builder
			var toolCalls []map[string]interface{}

			for _, part := range m.Content {
				switch p := part.(type) {
				case cif.CIFTextPart:
					textBuf.WriteString(p.Text)
				case cif.CIFThinkingPart:
					continue
				case cif.CIFToolCallPart:
					args, _ := json.Marshal(p.ToolArguments)
					toolCall := map[string]interface{}{
						"id":   p.ToolCallID,
						"type": "function",
						"function": map[string]interface{}{
							"name":      toolNameMapper.toUpstream(p.ToolName),
							"arguments": string(args),
						},
					}
					toolCalls = append(toolCalls, toolCall)
				}
			}

			if textBuf.Len() > 0 {
				openaiMsg["content"] = textBuf.String()
			}
			if len(toolCalls) > 0 {
				if _, ok := openaiMsg["content"]; !ok {
					openaiMsg["content"] = ""
				}
				openaiMsg["tool_calls"] = toolCalls
			}

			openaiMessages = append(openaiMessages, openaiMsg)
		}
	}

	return openaiMessages
}

func (a *CopilotAdapter) convertCIFPartToOpenAI(part cif.CIFContentPart) map[string]interface{} {
	switch p := part.(type) {
	case cif.CIFTextPart:
		return map[string]interface{}{
			"type": "text",
			"text": p.Text,
		}
	case cif.CIFThinkingPart:
		return nil
	case cif.CIFImagePart:
		imageURL := map[string]interface{}{}
		if p.Data != nil {
			imageURL["url"] = fmt.Sprintf("data:%s;base64,%s", p.MediaType, *p.Data)
		} else if p.URL != nil {
			imageURL["url"] = *p.URL
		}
		return map[string]interface{}{
			"type":      "image_url",
			"image_url": imageURL,
		}
	default:
		return map[string]interface{}{
			"type": "text",
			"text": "[Unsupported content type]",
		}
	}
}

func (a *CopilotAdapter) convertOpenAIToCIF(openaiResp map[string]interface{}, toolNameMapper *copilotToolNameMapper) *cif.CanonicalResponse {
	id, _ := openaiResp["id"].(string)
	model, _ := openaiResp["model"].(string)

	response := &cif.CanonicalResponse{
		ID:         id,
		Model:      model,
		StopReason: cif.StopReasonEndTurn,
	}

	if choices, ok := openaiResp["choices"].([]interface{}); ok && len(choices) > 0 {
		if choice, ok := choices[0].(map[string]interface{}); ok {
			if finishReason, ok := choice["finish_reason"].(string); ok {
				response.StopReason = a.convertOpenAIStopReason(finishReason)
			}

			if message, ok := choice["message"].(map[string]interface{}); ok {
				response.Content = a.convertOpenAIMessageToCIF(message, toolNameMapper)
				if response.StopReason != cif.StopReasonToolUse && containsToolCall(response.Content) {
					response.StopReason = cif.StopReasonToolUse
				}
			}
		}
	}

	if usage, ok := openaiResp["usage"].(map[string]interface{}); ok {
		if promptTokens, ok := usage["prompt_tokens"].(float64); ok {
			if completionTokens, ok := usage["completion_tokens"].(float64); ok {
				response.Usage = &cif.CIFUsage{
					InputTokens:  int(promptTokens),
					OutputTokens: int(completionTokens),
				}
			}
		}
	}

	return response
}

func (a *CopilotAdapter) convertOpenAIMessageToCIF(message map[string]interface{}, toolNameMapper *copilotToolNameMapper) []cif.CIFContentPart {
	var parts []cif.CIFContentPart

	if content, ok := message["content"].(string); ok && content != "" {
		parts = append(parts, cif.CIFTextPart{
			Type: "text",
			Text: content,
		})
	}

	if toolCalls, ok := message["tool_calls"].([]interface{}); ok {
		for _, tc := range toolCalls {
			if toolCall, ok := tc.(map[string]interface{}); ok {
				if function, ok := toolCall["function"].(map[string]interface{}); ok {
					id, _ := toolCall["id"].(string)
					if id == "" {
						id, _ = toolCall["call_id"].(string)
					}
					name, _ := function["name"].(string)
					args, _ := function["arguments"].(string)

					var toolArgs map[string]interface{}
					json.Unmarshal([]byte(args), &toolArgs)

					parts = append(parts, cif.CIFToolCallPart{
						Type:          "tool_call",
						ToolCallID:    id,
						ToolName:      toolNameMapper.fromUpstream(name),
						ToolArguments: toolArgs,
					})
				}
			}
		}
	}

	return parts
}

func containsToolCall(parts []cif.CIFContentPart) bool {
	for _, part := range parts {
		if _, ok := part.(cif.CIFToolCallPart); ok {
			return true
		}
	}
	return false
}
