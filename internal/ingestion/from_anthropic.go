package ingestion

import (
	"encoding/json"
	"fmt"
	"omnillm/internal/cif"
	"strings"

	"github.com/rs/zerolog/log"
)

// Anthropic Messages format
type AnthropicMessage struct {
	Role    string      `json:"role"`
	Content interface{} `json:"content"` // string or array of content blocks
}

type AnthropicContentBlock struct {
	Type      string                 `json:"type"`
	Text      string                 `json:"text,omitempty"`
	Thinking  string                 `json:"thinking,omitempty"`
	Signature string                 `json:"signature,omitempty"`
	Source    *AnthropicImageSource  `json:"source,omitempty"`
	ID        string                 `json:"id,omitempty"`
	ToolUseID string                 `json:"tool_use_id,omitempty"`
	Name      string                 `json:"name,omitempty"`
	Input     map[string]interface{} `json:"input,omitempty"`
	Content   interface{}            `json:"content,omitempty"`
	IsError   *bool                  `json:"is_error,omitempty"`
}

type AnthropicImageSource struct {
	Type      string `json:"type"`
	MediaType string `json:"media_type"`
	Data      string `json:"data"`
}

type AnthropicTool struct {
	Name        string                 `json:"name"`
	Description string                 `json:"description,omitempty"`
	InputSchema map[string]interface{} `json:"input_schema"`
}

type AnthropicMessagesRequest struct {
	Model       string             `json:"model"`
	System      interface{}        `json:"system,omitempty"`
	Messages    []AnthropicMessage `json:"messages"`
	Tools       []AnthropicTool    `json:"tools,omitempty"`
	ToolChoice  interface{}        `json:"tool_choice,omitempty"`
	Temperature *float64           `json:"temperature,omitempty"`
	TopP        *float64           `json:"top_p,omitempty"`
	MaxTokens   *int               `json:"max_tokens,omitempty"`
	Stop        []string           `json:"stop_sequences,omitempty"`
	Stream      *bool              `json:"stream,omitempty"`
	Metadata    *struct {
		UserID string `json:"user_id,omitempty"`
	} `json:"metadata,omitempty"`
}

// ParseAnthropicMessages converts Anthropic messages format to CIF
func ParseAnthropicMessages(raw json.RawMessage) (*cif.CanonicalRequest, error) {
	var req AnthropicMessagesRequest
	if err := json.Unmarshal(raw, &req); err != nil {
		return nil, fmt.Errorf("failed to unmarshal Anthropic request: %w", err)
	}

	canonical := &cif.CanonicalRequest{
		Model:       req.Model,
		Temperature: req.Temperature,
		TopP:        req.TopP,
		MaxTokens:   req.MaxTokens,
		Stop:        req.Stop,
		Stream:      req.Stream != nil && *req.Stream,
	}

	if systemPrompt := normalizeAnthropicSystem(req.System); systemPrompt != nil {
		canonical.SystemPrompt = systemPrompt
	}
	if req.Metadata != nil && req.Metadata.UserID != "" {
		canonical.UserID = &req.Metadata.UserID
	}

	for _, msg := range req.Messages {
		cifMsg, err := convertAnthropicMessage(msg)
		if err != nil {
			return nil, fmt.Errorf("failed to convert message: %w", err)
		}
		canonical.Messages = append(canonical.Messages, cifMsg)
	}

	for _, tool := range req.Tools {
		var description *string
		if tool.Description != "" {
			description = &tool.Description
		}
		cifTool := cif.CIFTool{
			Name:             tool.Name,
			Description:      description,
			ParametersSchema: normalizeSchema(tool.InputSchema),
		}
		canonical.Tools = append(canonical.Tools, cifTool)
	}

	canonical.ToolChoice = normalizeAnthropicToolChoice(req.ToolChoice)

	log.Debug().
		Str("model", canonical.Model).
		Int("messages", len(canonical.Messages)).
		Int("tools", len(canonical.Tools)).
		Bool("stream", canonical.Stream).
		Msg("Converted Anthropic request to CIF")

	return canonical, nil
}

func convertAnthropicMessage(msg AnthropicMessage) (cif.CIFMessage, error) {
	contentParts, err := convertAnthropicMessageContent(msg.Content)
	if err != nil {
		return nil, err
	}

	switch msg.Role {
	case "user":
		return cif.CIFUserMessage{
			Role:    "user",
			Content: contentParts,
		}, nil

	case "assistant":
		return cif.CIFAssistantMessage{
			Role:    "assistant",
			Content: contentParts,
		}, nil

	default:
		return nil, fmt.Errorf("unknown message role: %s", msg.Role)
	}
}

func convertAnthropicMessageContent(content interface{}) ([]cif.CIFContentPart, error) {
	switch raw := content.(type) {
	case string:
		return []cif.CIFContentPart{
			cif.CIFTextPart{Type: "text", Text: raw},
		}, nil
	case []interface{}:
		contentParts := make([]cif.CIFContentPart, 0, len(raw))
		for _, blockRaw := range raw {
			blockMap, ok := blockRaw.(map[string]interface{})
			if !ok {
				return nil, fmt.Errorf("invalid Anthropic content block type: %T", blockRaw)
			}
			// Build typed block directly from the map to avoid a
			// marshal→unmarshal roundtrip for each content block.
			block := anthropicBlockFromMap(blockMap)
			cifPart, err := convertAnthropicContentBlock(block)
			if err != nil {
				return nil, err
			}
			contentParts = append(contentParts, cifPart)
		}
		return contentParts, nil
	case nil:
		return []cif.CIFContentPart{}, nil
	default:
		return nil, fmt.Errorf("unsupported Anthropic message content type: %T", content)
	}
}

// anthropicBlockFromMap extracts AnthropicContentBlock fields directly from a
// map[string]interface{} produced by the top-level json.Unmarshal call.  This
// avoids a second marshal+unmarshal cycle for every content block.
func anthropicBlockFromMap(m map[string]interface{}) AnthropicContentBlock {
	block := AnthropicContentBlock{}
	block.Type, _ = m["type"].(string)
	block.Text, _ = m["text"].(string)
	block.Thinking, _ = m["thinking"].(string)
	block.Signature, _ = m["signature"].(string)
	block.ID, _ = m["id"].(string)
	block.ToolUseID, _ = m["tool_use_id"].(string)
	block.Name, _ = m["name"].(string)
	block.Content = m["content"]
	block.Input, _ = m["input"].(map[string]interface{})

	if srcRaw, ok := m["source"].(map[string]interface{}); ok {
		src := &AnthropicImageSource{}
		src.Type, _ = srcRaw["type"].(string)
		src.MediaType, _ = srcRaw["media_type"].(string)
		src.Data, _ = srcRaw["data"].(string)
		block.Source = src
	}

	if isErr, ok := m["is_error"].(bool); ok {
		block.IsError = &isErr
	}

	return block
}

func convertAnthropicContentBlock(block AnthropicContentBlock) (cif.CIFContentPart, error) {
	switch block.Type {
	case "text":
		return cif.CIFTextPart{
			Type: "text",
			Text: block.Text,
		}, nil

	case "image":
		if block.Source != nil {
			part := cif.CIFImagePart{
				Type:      "image",
				MediaType: block.Source.MediaType,
			}
			if block.Source.Type == "base64" {
				part.Data = &block.Source.Data
			}
			return part, nil
		}
		return nil, fmt.Errorf("image block missing source")

	case "thinking":
		part := cif.CIFThinkingPart{
			Type:     "thinking",
			Thinking: block.Thinking,
		}
		if block.Signature != "" {
			signature := block.Signature
			part.Signature = &signature
		}
		return part, nil

	case "tool_use":
		toolCallID := block.ToolUseID
		if toolCallID == "" {
			toolCallID = block.ID
		}
		input := block.Input
		if input == nil {
			input = map[string]interface{}{}
		}
		return cif.CIFToolCallPart{
			Type:          "tool_call",
			ToolCallID:    toolCallID,
			ToolName:      block.Name,
			ToolArguments: input,
		}, nil

	case "tool_result":
		toolCallID := block.ToolUseID
		if toolCallID == "" {
			toolCallID = block.ID
		}
		return cif.CIFToolResultPart{
			Type:       "tool_result",
			ToolCallID: toolCallID,
			ToolName:   block.Name,
			Content:    normalizeAnthropicToolResultContent(block.Content),
			IsError:    block.IsError,
		}, nil

	default:
		return nil, fmt.Errorf("unknown content block type: %s", block.Type)
	}
}

func normalizeAnthropicSystem(system interface{}) *string {
	switch value := system.(type) {
	case string:
		if value == "" {
			return nil
		}
		return &value
	case []interface{}:
		parts := make([]string, 0, len(value))
		for _, rawPart := range value {
			partMap, ok := rawPart.(map[string]interface{})
			if !ok {
				continue
			}
			if partType, _ := partMap["type"].(string); partType != "text" {
				continue
			}
			if text, ok := partMap["text"].(string); ok {
				parts = append(parts, text)
			}
		}
		if len(parts) == 0 {
			return nil
		}
		result := strings.Join(parts, "\n\n")
		return &result
	default:
		return nil
	}
}

func normalizeAnthropicToolChoice(choice interface{}) interface{} {
	choiceMap, ok := choice.(map[string]interface{})
	if !ok {
		return nil
	}

	switch choiceType, _ := choiceMap["type"].(string); choiceType {
	case "auto":
		return "auto"
	case "any":
		return "required"
	case "none":
		return "none"
	case "tool":
		if name, ok := choiceMap["name"].(string); ok && name != "" {
			return map[string]interface{}{
				"type":         "function",
				"functionName": name,
			}
		}
	}

	return nil
}

func normalizeAnthropicToolResultContent(content interface{}) string {
	switch value := content.(type) {
	case string:
		return value
	case []interface{}:
		parts := make([]string, 0, len(value))
		for _, rawPart := range value {
			partMap, ok := rawPart.(map[string]interface{})
			if !ok {
				continue
			}
			if text, ok := partMap["text"].(string); ok {
				parts = append(parts, text)
			}
		}
		if len(parts) > 0 {
			return strings.Join(parts, "\n\n")
		}
	case nil:
		return ""
	}

	if jsonBytes, err := json.Marshal(content); err == nil {
		return string(jsonBytes)
	}
	return ""
}

// ConvertAnthropicContentBlockForAgent exposes the existing Anthropic content
// block conversion logic for the agent runtime.
func ConvertAnthropicContentBlockForAgent(block AnthropicContentBlock) (cif.CIFContentPart, error) {
	return convertAnthropicContentBlock(block)
}

func normalizeSchema(schema interface{}) map[string]interface{} {
	normalized, ok := normalizeSchemaValue(schema).(map[string]interface{})
	if !ok {
		return nil
	}
	return normalized
}

func normalizeSchemaValue(value interface{}) interface{} {
	switch typed := value.(type) {
	case map[string]interface{}:
		result := make(map[string]interface{}, len(typed))
		for key, item := range typed {
			if key == "$schema" || key == "patternProperties" {
				continue
			}
			result[key] = normalizeSchemaValue(item)
		}

		if nullable, ok := result["nullable"].(bool); ok && nullable {
			delete(result, "nullable")
			switch currentType := result["type"].(type) {
			case string:
				result["type"] = []interface{}{currentType, "null"}
			case []interface{}:
				hasNull := false
				for _, item := range currentType {
					if text, ok := item.(string); ok && text == "null" {
						hasNull = true
						break
					}
				}
				if !hasNull {
					result["type"] = append(currentType, "null")
				}
			}
		}

		return result
	case []interface{}:
		result := make([]interface{}, 0, len(typed))
		for _, item := range typed {
			result = append(result, normalizeSchemaValue(item))
		}
		return result
	default:
		return value
	}
}
