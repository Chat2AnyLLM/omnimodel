package agent

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"

	"omnillm/internal/cif"
	"omnillm/internal/ingestion"
	"omnillm/internal/providers/openaicompat"
	sharedproviders "omnillm/internal/providers/shared"
)

// Client is the minimal transport interface the agent runtime needs from the CLI client.
type Client interface {
	Post(path string, body any) ([]byte, error)
	PostStream(path string, body any) (*http.Response, error)
}

// NewChatCompletionsDispatch returns a DispatchFn backed by OmniLLM's local proxy.
// All models are sent to /v1/messages (Anthropic Messages API shape) and OmniLLM
// translates the request to the appropriate upstream format.
func NewChatCompletionsDispatch(c Client, model string) DispatchFn {
	return func(ctx context.Context, req *cif.CanonicalRequest) (<-chan *cif.CanonicalResponse, error) {
		requestModel := strings.TrimSpace(model)
		if requestModel == "" {
			requestModel = strings.TrimSpace(req.Model)
		}
		if requestModel == "" {
			requestModel = "gpt-4"
		}

		var (
			response *cif.CanonicalResponse
			err      error
		)
		if req != nil && req.Stream {
			response, err = doStreamPost(ctx, c, requestModel, req)
		} else {
			response, err = doPost(c, requestModel, req)
		}
		if err != nil {
			return nil, err
		}

		ch := make(chan *cif.CanonicalResponse, 1)
		ch <- response
		close(ch)
		return ch, nil
	}
}

// doPost always uses /v1/messages. OmniLLM translates to the upstream API shape.
func doPost(c Client, model string, req *cif.CanonicalRequest) (*cif.CanonicalResponse, error) {
	return doAnthropicPost(c, model, req)
}

// doStreamPost always uses /v1/messages with stream=true. OmniLLM translates.
func doStreamPost(ctx context.Context, c Client, model string, req *cif.CanonicalRequest) (*cif.CanonicalResponse, error) {
	return doAnthropicStreamPost(ctx, c, model, req)
}

func doChatCompletionsPost(c Client, model string, req *cif.CanonicalRequest) (*cif.CanonicalResponse, error) {
	chatReq, err := openaicompat.BuildChatRequest(model, req, false, openaicompat.Config{})
	if err != nil {
		return nil, fmt.Errorf("build chat request: %w", err)
	}

	jsonBytes, err := openaicompat.Marshal(chatReq)
	if err != nil {
		return nil, fmt.Errorf("marshal chat request: %w", err)
	}

	data, err := c.Post("/v1/chat/completions", json.RawMessage(jsonBytes))
	if err != nil {
		return nil, fmt.Errorf("failed to create chat completion: %w", err)
	}

	var chatResp openaicompat.ChatResponse
	if err := json.Unmarshal(data, &chatResp); err != nil {
		return nil, fmt.Errorf("decode chat completion: %w", err)
	}

	return openaicompat.ParseChatResponse(&chatResp), nil
}

func doChatCompletionsStreamPost(ctx context.Context, c Client, model string, req *cif.CanonicalRequest) (*cif.CanonicalResponse, error) {
	chatReq, err := openaicompat.BuildChatRequest(model, req, true, openaicompat.Config{})
	if err != nil {
		return nil, fmt.Errorf("build chat request: %w", err)
	}
	ch, err := openaicompat.Stream(ctx, "/v1/chat/completions", nil, chatReq)
	if err == nil {
		return sharedproviders.CollectStream(ch)
	}
	// Fallback through client PostStream because the local agent client already wraps transport.
	jsonBytes, marshalErr := openaicompat.Marshal(chatReq)
	if marshalErr != nil {
		return nil, fmt.Errorf("marshal chat request: %w", marshalErr)
	}
	resp, streamErr := c.PostStream("/v1/chat/completions", json.RawMessage(jsonBytes))
	if streamErr != nil {
		return nil, fmt.Errorf("failed to stream chat completion: %w", err)
	}
	defer resp.Body.Close()
	streamCh := make(chan cif.CIFStreamEvent, 64)
	go openaicompat.ParseSSE(resp.Body, streamCh)
	return sharedproviders.CollectStream(streamCh)
}

func doResponsesPost(c Client, model string, req *cif.CanonicalRequest) (*cif.CanonicalResponse, error) {
	payload := openaicompat.BuildResponsesPayload(model, req, false, openaicompat.ResponsesConfig{})
	data, err := c.Post("/v1/responses", payload)
	if err != nil {
		return nil, fmt.Errorf("failed to create responses request: %w", err)
	}

	var responsesResp openaicompat.ResponsesResponse
	if err := json.Unmarshal(data, &responsesResp); err != nil {
		return nil, fmt.Errorf("decode responses response: %w", err)
	}

	return openaicompat.ParseResponsesResponse(&responsesResp), nil
}

func doResponsesStreamPost(_ context.Context, c Client, model string, req *cif.CanonicalRequest) (*cif.CanonicalResponse, error) {
	payload := openaicompat.BuildResponsesPayload(model, req, true, openaicompat.ResponsesConfig{})
	resp, err := c.PostStream("/v1/responses", payload)
	if err != nil {
		return nil, fmt.Errorf("failed to stream responses request: %w", err)
	}
	defer resp.Body.Close()
	streamCh := make(chan cif.CIFStreamEvent, 64)
	go openaicompat.ParseResponsesSSE(resp.Body, streamCh)
	return sharedproviders.CollectStream(streamCh)
}

func doAnthropicPost(c Client, model string, req *cif.CanonicalRequest) (*cif.CanonicalResponse, error) {
	payload, err := buildAnthropicMessagesRequest(model, req)
	if err != nil {
		return nil, fmt.Errorf("build anthropic messages request: %w", err)
	}

	data, err := c.Post("/v1/messages", payload)
	if err != nil {
		return nil, fmt.Errorf("failed to create anthropic message: %w", err)
	}

	var anthropicResp anthropicMessagesResponse
	if err := json.Unmarshal(data, &anthropicResp); err != nil {
		return nil, fmt.Errorf("decode anthropic message response: %w", err)
	}

	return parseAnthropicResponse(&anthropicResp), nil
}

func doAnthropicStreamPost(_ context.Context, c Client, model string, req *cif.CanonicalRequest) (*cif.CanonicalResponse, error) {
	payload, err := buildAnthropicMessagesRequest(model, req)
	if err != nil {
		return nil, fmt.Errorf("build anthropic messages request: %w", err)
	}
	payload["stream"] = true

	resp, err := c.PostStream("/v1/messages", payload)
	if err != nil {
		return nil, fmt.Errorf("failed to stream anthropic message: %w", err)
	}
	defer resp.Body.Close()

	streamCh := make(chan cif.CIFStreamEvent, 64)
	go parseAnthropicSSE(resp.Body, streamCh)
	return sharedproviders.CollectStream(streamCh)
}

func buildAnthropicMessagesRequest(model string, req *cif.CanonicalRequest) (map[string]any, error) {
	payload := map[string]any{
		"model":      model,
		"max_tokens": 4096,
	}
	if req.MaxTokens != nil && *req.MaxTokens > 0 {
		payload["max_tokens"] = *req.MaxTokens
	}
	if req.SystemPrompt != nil && strings.TrimSpace(*req.SystemPrompt) != "" {
		payload["system"] = *req.SystemPrompt
	}

	messages, err := cifMessagesToAnthropic(req)
	if err != nil {
		return nil, err
	}
	payload["messages"] = messages

	if len(req.Tools) > 0 {
		tools := make([]map[string]any, 0, len(req.Tools))
		for _, t := range req.Tools {
			tool := map[string]any{
				"name":         t.Name,
				"input_schema": t.ParametersSchema,
			}
			if t.Description != nil && *t.Description != "" {
				tool["description"] = *t.Description
			}
			tools = append(tools, tool)
		}
		payload["tools"] = tools
	}
	if req.ToolChoice != nil {
		if toolChoice := canonicalToolChoiceToAnthropic(req.ToolChoice); toolChoice != nil {
			payload["tool_choice"] = toolChoice
		}
	}
	if req.Stream {
		payload["stream"] = true
	}
	return payload, nil
}

func cifMessagesToAnthropic(req *cif.CanonicalRequest) ([]map[string]any, error) {
	var messages []map[string]any
	for _, msg := range req.Messages {
		switch m := msg.(type) {
		case cif.CIFSystemMessage:
			if strings.TrimSpace(m.Content) != "" {
				messages = append(messages, map[string]any{"role": "user", "content": m.Content})
			}
		case cif.CIFUserMessage:
			content := anthropicContentBlocksFromUser(m)
			messages = append(messages, map[string]any{"role": "user", "content": content})
		case cif.CIFAssistantMessage:
			content := anthropicContentBlocksFromAssistant(m)
			messages = append(messages, map[string]any{"role": "assistant", "content": content})
		}
	}
	return messages, nil
}

func anthropicContentBlocksFromUser(m cif.CIFUserMessage) []map[string]any {
	var blocks []map[string]any
	for _, part := range m.Content {
		switch p := part.(type) {
		case cif.CIFTextPart:
			blocks = append(blocks, map[string]any{"type": "text", "text": p.Text})
		case cif.CIFImagePart:
			if p.Data != nil {
				blocks = append(blocks, map[string]any{
					"type": "image",
					"source": map[string]any{
						"type":       "base64",
						"media_type": p.MediaType,
						"data":       *p.Data,
					},
				})
			}
		case cif.CIFToolResultPart:
			block := map[string]any{
				"type":        "tool_result",
				"tool_use_id": p.ToolCallID,
				"content":     p.Content,
			}
			if p.IsError != nil {
				block["is_error"] = *p.IsError
			}
			blocks = append(blocks, block)
		}
	}
	return blocks
}

func anthropicContentBlocksFromAssistant(m cif.CIFAssistantMessage) []map[string]any {
	var blocks []map[string]any
	for _, part := range m.Content {
		switch p := part.(type) {
		case cif.CIFTextPart:
			blocks = append(blocks, map[string]any{"type": "text", "text": p.Text})
		case cif.CIFThinkingPart:
			block := map[string]any{"type": "thinking", "thinking": p.Thinking}
			if p.Signature != nil {
				block["signature"] = *p.Signature
			}
			blocks = append(blocks, block)
		case cif.CIFToolCallPart:
			blocks = append(blocks, map[string]any{
				"type":  "tool_use",
				"id":    p.ToolCallID,
				"name":  p.ToolName,
				"input": p.ToolArguments,
			})
		}
	}
	return blocks
}

func canonicalToolChoiceToAnthropic(choice any) any {
	switch v := choice.(type) {
	case string:
		switch v {
		case "auto":
			return map[string]any{"type": "auto"}
		case "required":
			return map[string]any{"type": "any"}
		case "none":
			return map[string]any{"type": "none"}
		}
	case map[string]any:
		if typ, _ := v["type"].(string); typ == "function" {
			if name, _ := v["functionName"].(string); name != "" {
				return map[string]any{"type": "tool", "name": name}
			}
		}
	}
	return nil
}

type anthropicUsage struct {
	InputTokens  int `json:"input_tokens"`
	OutputTokens int `json:"output_tokens"`
}

type anthropicMessagesResponse struct {
	ID         string                            `json:"id"`
	Model      string                            `json:"model"`
	StopReason *string                           `json:"stop_reason"`
	Usage      *anthropicUsage                   `json:"usage,omitempty"`
	Content    []ingestion.AnthropicContentBlock `json:"content"`
}

func parseAnthropicResponse(resp *anthropicMessagesResponse) *cif.CanonicalResponse {
	result := &cif.CanonicalResponse{
		ID:    resp.ID,
		Model: resp.Model,
	}
	if resp.StopReason != nil {
		switch *resp.StopReason {
		case "tool_use":
			result.StopReason = cif.StopReasonToolUse
		case "max_tokens":
			result.StopReason = cif.StopReasonMaxTokens
		case "stop_sequence":
			result.StopReason = cif.StopReasonStopSequence
		case "content_filter":
			result.StopReason = cif.StopReasonContentFilter
		default:
			result.StopReason = cif.StopReasonEndTurn
		}
	} else {
		result.StopReason = cif.StopReasonEndTurn
	}
	if resp.Usage != nil {
		result.Usage = &cif.CIFUsage{InputTokens: resp.Usage.InputTokens, OutputTokens: resp.Usage.OutputTokens}
	}
	for _, block := range resp.Content {
		part, err := ingestion.ConvertAnthropicContentBlockForAgent(block)
		if err != nil {
			continue
		}
		result.Content = append(result.Content, part)
	}
	return result
}

func parseAnthropicSSE(body io.Reader, ch chan<- cif.CIFStreamEvent) {
	defer close(ch)
	scanner := bufio.NewScanner(body)
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	var eventName string
	var dataLines []string

	emit := func() {
		if len(dataLines) == 0 {
			return
		}
		data := strings.Join(dataLines, "\n")
		dataLines = nil
		if strings.TrimSpace(data) == "" {
			return
		}
		var payload map[string]any
		if err := json.Unmarshal([]byte(data), &payload); err != nil {
			ch <- cif.CIFStreamError{Type: "stream_error", Error: cif.ErrorInfo{Type: "parse_error", Message: err.Error()}}
			return
		}
		switch eventName {
		case "message_start":
			message, _ := payload["message"].(map[string]any)
			id, _ := message["id"].(string)
			model, _ := message["model"].(string)
			ch <- cif.CIFStreamStart{Type: "stream_start", ID: id, Model: model}
		case "content_block_start":
			index := intValue(payload["index"])
			blockMap, _ := payload["content_block"].(map[string]any)
			block := ingestion.AnthropicContentBlock{}
			block.Type, _ = blockMap["type"].(string)
			block.Text, _ = blockMap["text"].(string)
			block.Thinking, _ = blockMap["thinking"].(string)
			block.ID, _ = blockMap["id"].(string)
			block.Name, _ = blockMap["name"].(string)
			block.Input, _ = blockMap["input"].(map[string]any)
			part, err := ingestion.ConvertAnthropicContentBlockForAgent(block)
			if err != nil {
				return
			}
			var delta cif.DeltaContent = cif.TextDelta{Type: "text_delta", Text: ""}
			switch part.(type) {
			case cif.CIFThinkingPart:
				delta = cif.ThinkingDelta{Type: "thinking_delta", Thinking: ""}
			case cif.CIFToolCallPart:
				delta = cif.ToolArgumentsDelta{Type: "tool_arguments_delta", PartialJSON: ""}
			}
			ch <- cif.CIFContentDelta{Type: "content_delta", Index: index, ContentBlock: part, Delta: delta}
		case "content_block_delta":
			index := intValue(payload["index"])
			deltaMap, _ := payload["delta"].(map[string]any)
			deltaType, _ := deltaMap["type"].(string)
			switch deltaType {
			case "text_delta":
				text, _ := deltaMap["text"].(string)
				ch <- cif.CIFContentDelta{Type: "content_delta", Index: index, Delta: cif.TextDelta{Type: "text_delta", Text: text}}
			case "thinking_delta":
				thinking, _ := deltaMap["thinking"].(string)
				ch <- cif.CIFContentDelta{Type: "content_delta", Index: index, Delta: cif.ThinkingDelta{Type: "thinking_delta", Thinking: thinking}}
			case "input_json_delta":
				partial, _ := deltaMap["partial_json"].(string)
				ch <- cif.CIFContentDelta{Type: "content_delta", Index: index, Delta: cif.ToolArgumentsDelta{Type: "tool_arguments_delta", PartialJSON: partial}}
			}
		case "content_block_stop":
			ch <- cif.CIFContentBlockStop{Type: "content_block_stop", Index: intValue(payload["index"])}
		case "message_delta":
			deltaMap, _ := payload["delta"].(map[string]any)
			stopReason, _ := deltaMap["stop_reason"].(string)
			var stop cif.CIFStopReason
			switch stopReason {
			case "tool_use":
				stop = cif.StopReasonToolUse
			case "max_tokens":
				stop = cif.StopReasonMaxTokens
			case "stop_sequence":
				stop = cif.StopReasonStopSequence
			case "content_filter":
				stop = cif.StopReasonContentFilter
			default:
				stop = cif.StopReasonEndTurn
			}
			var usage *cif.CIFUsage
			if usageMap, ok := payload["usage"].(map[string]any); ok {
				usage = &cif.CIFUsage{OutputTokens: intValue(usageMap["output_tokens"])}
			}
			ch <- cif.CIFStreamEnd{Type: "stream_end", StopReason: stop, Usage: usage}
		case "error":
			errMap, _ := payload["error"].(map[string]any)
			errType, _ := errMap["type"].(string)
			message, _ := errMap["message"].(string)
			ch <- cif.CIFStreamError{Type: "stream_error", Error: cif.ErrorInfo{Type: errType, Message: message}}
		}
	}

	for scanner.Scan() {
		line := scanner.Text()
		if line == "" {
			emit()
			eventName = ""
			continue
		}
		if after, ok := strings.CutPrefix(line, "event:"); ok {
			eventName = strings.TrimSpace(after)
			continue
		}
		if after, ok := strings.CutPrefix(line, "data:"); ok {
			dataLines = append(dataLines, strings.TrimSpace(after))
		}
	}
	if err := scanner.Err(); err != nil {
		ch <- cif.CIFStreamError{Type: "stream_error", Error: cif.ErrorInfo{Type: "io_error", Message: err.Error()}}
		return
	}
	emit()
}

func intValue(v any) int {
	switch n := v.(type) {
	case float64:
		return int(n)
	case int:
		return n
	default:
		return 0
	}
}

// ReadStreamText parses OpenAI-compatible SSE output and collects assistant text content.
func ReadStreamText(body io.Reader) (string, error) {
	var out strings.Builder
	streamCh := make(chan cif.CIFStreamEvent, 64)
	go openaicompat.ParseSSE(io.NopCloser(body), streamCh)
	for event := range streamCh {
		switch ev := event.(type) {
		case cif.CIFContentDelta:
			if delta, ok := ev.Delta.(cif.TextDelta); ok {
				out.WriteString(delta.Text)
			}
		case cif.CIFStreamError:
			return out.String(), fmt.Errorf("%s", ev.Error.Message)
		}
	}
	return out.String(), nil
}

// EncodePermissionPrompt formats a tool-call approval prompt for UI layers.
func EncodePermissionPrompt(toolName string, args map[string]any) string {
	encoded, err := json.Marshal(args)
	if err != nil {
		encoded = fmt.Appendf(nil, "<failed to serialize arguments: %v>", err)
	}
	var buf bytes.Buffer
	buf.WriteString("Allow tool execution?\n")
	buf.WriteString("Tool: ")
	buf.WriteString(toolName)
	buf.WriteString("\nArguments: ")
	buf.Write(encoded)
	return buf.String()
}
