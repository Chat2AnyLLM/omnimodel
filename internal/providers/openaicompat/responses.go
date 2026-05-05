package openaicompat

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"omnillm/internal/cif"
	"omnillm/internal/providers/shared"
	"strings"

	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
)

type ResponsesConfig struct {
	DefaultTemperature *float64
	DefaultTopP        *float64
	Extras             map[string]interface{}
}

type ResponsesResponse struct {
	ID                string                `json:"id"`
	Model             string                `json:"model"`
	Status            string                `json:"status"`
	Output            []ResponsesOutputItem `json:"output"`
	OutputText        interface{}           `json:"output_text,omitempty"`
	IncompleteDetails *struct {
		Reason string `json:"reason"`
	} `json:"incomplete_details,omitempty"`
	Usage *ResponsesUsage `json:"usage,omitempty"`
}

type ResponsesOutputItem struct {
	Type      string                  `json:"type"`
	ID        string                  `json:"id"`
	CallID    string                  `json:"call_id,omitempty"`
	Role      string                  `json:"role,omitempty"`
	Name      string                  `json:"name,omitempty"`
	Arguments string                  `json:"arguments,omitempty"`
	Content   []ResponsesContentBlock `json:"content,omitempty"`
	Status    string                  `json:"status,omitempty"`
}

type ResponsesContentBlock struct {
	Type string `json:"type"`
	Text string `json:"text,omitempty"`
}

type ResponsesUsage struct {
	InputTokens  int `json:"input_tokens"`
	OutputTokens int `json:"output_tokens"`
}

type responsesStreamState struct {
	streamStarted     bool
	responseID        string
	model             string
	nextContentIndex  int
	textBlockIndices  map[string]int
	textBlockHasDelta map[string]bool
	toolCallIndices   map[int]int
	toolCallHasDelta  map[int]bool
}

func BuildResponsesPayload(model string, request *cif.CanonicalRequest, stream bool, cfg ResponsesConfig) map[string]interface{} {
	payload := map[string]interface{}{
		"model":  model,
		"input":  CIFMessagesToResponsesInput(request),
		"stream": stream,
	}

	if request.SystemPrompt != nil && strings.TrimSpace(*request.SystemPrompt) != "" {
		payload["instructions"] = *request.SystemPrompt
	}

	if request.Temperature != nil {
		payload["temperature"] = *request.Temperature
	} else if cfg.DefaultTemperature != nil {
		payload["temperature"] = *cfg.DefaultTemperature
	}
	if request.TopP != nil {
		payload["top_p"] = *request.TopP
	} else if cfg.DefaultTopP != nil {
		payload["top_p"] = *cfg.DefaultTopP
	}
	if request.MaxTokens != nil && *request.MaxTokens > 0 {
		payload["max_output_tokens"] = *request.MaxTokens
	}
	if request.UserID != nil {
		payload["user"] = shared.TruncateOpenAIUserID(*request.UserID)
	}

	if len(request.Tools) > 0 {
		tools := make([]map[string]interface{}, 0, len(request.Tools))
		for _, tool := range request.Tools {
			item := map[string]interface{}{
				"type":       "function",
				"name":       tool.Name,
				"parameters": shared.NormalizeToolParameters(tool.ParametersSchema),
			}
			if tool.Description != nil {
				item["description"] = *tool.Description
			}
			tools = append(tools, item)
		}
		payload["tools"] = tools
	}

	if request.ToolChoice != nil {
		if toolChoice := shared.ConvertCanonicalToolChoiceToOpenAI(request.ToolChoice); toolChoice != nil {
			payload["tool_choice"] = toolChoice
		}
	}

	for key, value := range cfg.Extras {
		payload[key] = value
	}

	// Re-apply user ID truncation in case Extras overwrote it with an oversize value.
	if userVal, ok := payload["user"].(string); ok {
		payload["user"] = shared.TruncateOpenAIUserID(userVal)
	}

	return payload
}

func CIFMessagesToResponsesInput(request *cif.CanonicalRequest) []map[string]interface{} {
	if request == nil {
		return nil
	}

	var input []map[string]interface{}
	for _, message := range request.Messages {
		switch m := message.(type) {
		case cif.CIFSystemMessage:
			input = append(input, map[string]interface{}{
				"type": "message",
				"role": "system",
				"content": []map[string]interface{}{
					{"type": "input_text", "text": m.Content},
				},
			})
		case cif.CIFUserMessage:
			input = append(input, responsesUserMessageItems(m)...)
		case cif.CIFAssistantMessage:
			input = append(input, responsesAssistantMessageItems(m)...)
		}
	}

	return input
}

func responsesUserMessageItems(message cif.CIFUserMessage) []map[string]interface{} {
	var items []map[string]interface{}
	var content []map[string]interface{}

	flushContent := func() {
		if len(content) == 0 {
			return
		}
		items = append(items, map[string]interface{}{
			"type":    "message",
			"role":    "user",
			"content": content,
		})
		content = nil
	}

	for _, part := range message.Content {
		switch p := part.(type) {
		case cif.CIFTextPart:
			content = append(content, map[string]interface{}{
				"type": "input_text",
				"text": p.Text,
			})
		case cif.CIFImagePart:
			imageURL := responsesImageURL(p)
			if imageURL == "" {
				continue
			}
			content = append(content, map[string]interface{}{
				"type":      "input_image",
				"image_url": imageURL,
			})
		case cif.CIFToolResultPart:
			flushContent()
			output := p.Content
			if p.IsError != nil && *p.IsError && output == "" {
				output = "Error: tool call failed"
			}
			items = append(items, map[string]interface{}{
				"type":    "function_call_output",
				"call_id": p.ToolCallID,
				"output":  output,
			})
		}
	}

	flushContent()
	return items
}

func responsesAssistantMessageItems(message cif.CIFAssistantMessage) []map[string]interface{} {
	var items []map[string]interface{}
	var content []map[string]interface{}

	flushContent := func() {
		if len(content) == 0 {
			return
		}
		items = append(items, map[string]interface{}{
			"type":    "message",
			"role":    "assistant",
			"content": content,
		})
		content = nil
	}

	for _, part := range message.Content {
		switch p := part.(type) {
		case cif.CIFTextPart:
			content = append(content, map[string]interface{}{
				"type": "output_text",
				"text": p.Text,
			})
		case cif.CIFThinkingPart:
			content = append(content, map[string]interface{}{
				"type": "output_text",
				"text": fmt.Sprintf("<thinking>\n%s\n</thinking>", p.Thinking),
			})
		case cif.CIFToolCallPart:
			flushContent()
			argsBytes, _ := json.Marshal(p.ToolArguments)
			items = append(items, map[string]interface{}{
				"type":      "function_call",
				"call_id":   p.ToolCallID,
				"name":      p.ToolName,
				"arguments": string(argsBytes),
			})
		}
	}

	flushContent()
	return items
}

func responsesImageURL(part cif.CIFImagePart) string {
	if part.Data != nil {
		return fmt.Sprintf("data:%s;base64,%s", part.MediaType, *part.Data)
	}
	if part.URL != nil {
		return *part.URL
	}
	return ""
}

func ParseResponsesResponse(resp *ResponsesResponse) *cif.CanonicalResponse {
	if resp == nil {
		return &cif.CanonicalResponse{StopReason: cif.StopReasonEndTurn}
	}

	result := &cif.CanonicalResponse{
		ID:         resp.ID,
		Model:      resp.Model,
		StopReason: ResponsesStopReason(resp),
		Usage:      responsesUsageToCIF(resp.Usage),
	}

	for _, item := range resp.Output {
		switch item.Type {
		case "message":
			for _, block := range item.Content {
				if block.Type != "output_text" && block.Type != "text" {
					continue
				}
				if block.Text == "" {
					continue
				}
				result.Content = append(result.Content, cif.CIFTextPart{
					Type: "text",
					Text: block.Text,
				})
			}
		case "function_call":
			var args map[string]interface{}
			json.Unmarshal([]byte(item.Arguments), &args) //nolint:errcheck
			if args == nil {
				args = map[string]interface{}{}
			}
			result.Content = append(result.Content, cif.CIFToolCallPart{
				Type:          "tool_call",
				ToolCallID:    firstNonEmpty(item.CallID, item.ID),
				ToolName:      item.Name,
				ToolArguments: args,
			})
		}
	}

	return result
}

func ResponsesStopReason(resp *ResponsesResponse) cif.CIFStopReason {
	if resp == nil {
		return cif.StopReasonEndTurn
	}

	for _, item := range resp.Output {
		if item.Type == "function_call" {
			return cif.StopReasonToolUse
		}
	}

	if resp.IncompleteDetails != nil {
		switch resp.IncompleteDetails.Reason {
		case "max_output_tokens":
			return cif.StopReasonMaxTokens
		case "content_filter":
			return cif.StopReasonContentFilter
		}
	}

	if strings.EqualFold(resp.Status, "incomplete") {
		return cif.StopReasonMaxTokens
	}

	return cif.StopReasonEndTurn
}

func responsesUsageToCIF(usage *ResponsesUsage) *cif.CIFUsage {
	if usage == nil {
		return nil
	}
	return &cif.CIFUsage{
		InputTokens:  usage.InputTokens,
		OutputTokens: usage.OutputTokens,
	}
}

func ExecuteResponses(ctx context.Context, url string, headers map[string]string, payload map[string]interface{}) (*cif.CanonicalResponse, error) {
	body, err := json.Marshal(payload)
	if err != nil {
		return nil, fmt.Errorf("openaicompat: marshal responses request: %w", err)
	}

	if log.Logger.GetLevel() <= zerolog.TraceLevel {
		log.Trace().Str("url", url).RawJSON("payload", cappedBody(body)).Msg("outbound openaicompat responses request")
	}

	req, err := newPOSTRequest(ctx, url, headers, body, false)
	if err != nil {
		return nil, fmt.Errorf("openaicompat: create responses request: %w", err)
	}

	resp, err := doPOST(req, false)
	if err != nil {
		return nil, fmt.Errorf("openaicompat: responses request failed: %w", err)
	}
	defer resp.Body.Close()

	var responsesResp ResponsesResponse
	if err := json.NewDecoder(resp.Body).Decode(&responsesResp); err != nil {
		return nil, fmt.Errorf("openaicompat: decode responses response: %w", err)
	}

	return ParseResponsesResponse(&responsesResp), nil
}

func StreamResponses(ctx context.Context, url string, headers map[string]string, payload map[string]interface{}) (<-chan cif.CIFStreamEvent, error) {
	body, err := json.Marshal(payload)
	if err != nil {
		return nil, fmt.Errorf("openaicompat: marshal responses stream request: %w", err)
	}

	if log.Logger.GetLevel() <= zerolog.TraceLevel {
		log.Trace().Str("url", url).RawJSON("payload", cappedBody(body)).Msg("outbound openaicompat responses stream request")
	}

	req, err := newPOSTRequest(ctx, url, headers, body, true)
	if err != nil {
		return nil, fmt.Errorf("openaicompat: create responses stream request: %w", err)
	}

	resp, err := doPOST(req, true)
	if err != nil {
		return nil, fmt.Errorf("openaicompat: responses stream request failed: %w", err)
	}

	return startSSEStream(resp.Body, ParseResponsesSSE), nil
}

func ParseResponsesSSE(body io.ReadCloser, eventCh chan cif.CIFStreamEvent) {
	defer body.Close()
	defer close(eventCh)

	scanner := bufio.NewScanner(body)
	scanner.Buffer(make([]byte, 0, 4*1024), 1024*1024)

	state := &responsesStreamState{
		textBlockIndices:  make(map[string]int),
		textBlockHasDelta: make(map[string]bool),
		toolCallIndices:   make(map[int]int),
		toolCallHasDelta:  make(map[int]bool),
	}

	var eventType string
	var dataLines []string

	flushEvent := func() {
		if eventType == "" || len(dataLines) == 0 {
			eventType = ""
			dataLines = nil
			return
		}
		handleResponsesSSEEvent(eventType, strings.Join(dataLines, "\n"), state, eventCh)
		eventType = ""
		dataLines = nil
	}

	for scanner.Scan() {
		line := scanner.Text()
		if line == "" {
			flushEvent()
			continue
		}
		if strings.HasPrefix(line, ":") {
			continue
		}
		if strings.HasPrefix(line, "event: ") {
			eventType = strings.TrimSpace(strings.TrimPrefix(line, "event: "))
			continue
		}
		if strings.HasPrefix(line, "data: ") {
			dataLines = append(dataLines, strings.TrimPrefix(line, "data: "))
		}
	}

	flushEvent()

	if err := scanner.Err(); err != nil {
		log.Error().Err(err).Str("provider", "openaicompat").Msg("Responses SSE scanner error")
		eventCh <- cif.CIFStreamError{
			Type:  "stream_error",
			Error: cif.ErrorInfo{Type: "stream_error", Message: err.Error()},
		}
	}
}

//nolint:gocyclo,funlen // SSE event dispatch handles many event types from upstream API
func handleResponsesSSEEvent(eventType, data string, state *responsesStreamState, eventCh chan cif.CIFStreamEvent) {
	if data == "" {
		return
	}

	switch eventType {
	case "response.created":
		var payload struct {
			Response *ResponsesResponse `json:"response"`
		}
		if err := json.Unmarshal([]byte(data), &payload); err != nil || payload.Response == nil {
			if err != nil {
				log.Warn().Err(err).Msg("openaicompat: failed to parse response.created event")
			}
			return
		}
		state.responseID = payload.Response.ID
		state.model = payload.Response.Model
		state.streamStarted = true
		eventCh <- cif.CIFStreamStart{
			Type:  "stream_start",
			ID:    payload.Response.ID,
			Model: payload.Response.Model,
		}

	case "response.output_item.added", "response.output_item.done":
		var payload struct {
			Item        ResponsesOutputItem `json:"item"`
			OutputIndex int                 `json:"output_index"`
		}
		if err := json.Unmarshal([]byte(data), &payload); err != nil {
			log.Warn().Err(err).Msg("openaicompat: failed to parse response.output_item event")
			return
		}
		if payload.Item.Type != "function_call" {
			return
		}
		index, exists := state.toolCallIndices[payload.OutputIndex]
		if !exists {
			index = state.nextContentIndex
			state.nextContentIndex++
			state.toolCallIndices[payload.OutputIndex] = index
			eventCh <- cif.CIFContentDelta{
				Type:  "content_delta",
				Index: index,
				ContentBlock: cif.CIFToolCallPart{
					Type:          "tool_call",
					ToolCallID:    firstNonEmpty(payload.Item.CallID, payload.Item.ID),
					ToolName:      payload.Item.Name,
					ToolArguments: map[string]interface{}{},
				},
				Delta: cif.ToolArgumentsDelta{
					Type:        "tool_arguments_delta",
					PartialJSON: "",
				},
			}
		}
		if payload.Item.Arguments == "" || state.toolCallHasDelta[payload.OutputIndex] {
			return
		}
		eventCh <- cif.CIFContentDelta{
			Type:  "content_delta",
			Index: index,
			Delta: cif.ToolArgumentsDelta{
				Type:        "tool_arguments_delta",
				PartialJSON: payload.Item.Arguments,
			},
		}
		state.toolCallHasDelta[payload.OutputIndex] = true

	case "response.function_call_arguments.delta", "response.function_call_arguments.done":
		var payload struct {
			OutputIndex int    `json:"output_index"`
			Delta       string `json:"delta"`
			Arguments   string `json:"arguments"`
		}
		if err := json.Unmarshal([]byte(data), &payload); err != nil {
			log.Warn().Err(err).Msg("openaicompat: failed to parse function_call_arguments event")
			return
		}
		index, ok := state.toolCallIndices[payload.OutputIndex]
		if !ok {
			return
		}
		partial := payload.Delta
		if partial == "" {
			partial = payload.Arguments
		}
		if partial == "" {
			return
		}
		if eventType == "response.function_call_arguments.done" && state.toolCallHasDelta[payload.OutputIndex] {
			return
		}
		eventCh <- cif.CIFContentDelta{
			Type:  "content_delta",
			Index: index,
			Delta: cif.ToolArgumentsDelta{
				Type:        "tool_arguments_delta",
				PartialJSON: partial,
			},
		}
		state.toolCallHasDelta[payload.OutputIndex] = true

	case "response.output_text.delta", "response.output_text.done":
		var payload struct {
			OutputIndex  int    `json:"output_index"`
			ContentIndex int    `json:"content_index"`
			Delta        string `json:"delta"`
			Text         string `json:"text"`
		}
		if err := json.Unmarshal([]byte(data), &payload); err != nil {
			log.Warn().Err(err).Msg("openaicompat: failed to parse output_text event")
			return
		}
		index, isNew := state.ensureTextBlockIndex(payload.OutputIndex, payload.ContentIndex)
		key := state.textBlockKey(payload.OutputIndex, payload.ContentIndex)
		text := payload.Delta
		if text == "" {
			text = payload.Text
		}
		if text == "" {
			return
		}
		if eventType == "response.output_text.done" && state.textBlockHasDelta[key] {
			return
		}
		contentDelta := cif.CIFContentDelta{
			Type:  "content_delta",
			Index: index,
			Delta: cif.TextDelta{
				Type: "text_delta",
				Text: text,
			},
		}
		if isNew {
			contentDelta.ContentBlock = cif.CIFTextPart{Type: "text", Text: ""}
		}
		eventCh <- contentDelta
		state.textBlockHasDelta[key] = true

	case "response.completed":
		var payload struct {
			Response *ResponsesResponse `json:"response"`
		}
		if err := json.Unmarshal([]byte(data), &payload); err != nil || payload.Response == nil {
			if err != nil {
				log.Warn().Err(err).Msg("openaicompat: failed to parse response.completed event")
			}
			return
		}
		eventCh <- cif.CIFStreamEnd{
			Type:       "stream_end",
			StopReason: ResponsesStopReason(payload.Response),
			Usage:      responsesUsageToCIF(payload.Response.Usage),
		}

	case "error", "response.failed":
		var payload struct {
			Error *struct {
				Message string `json:"message"`
				Type    string `json:"type"`
			} `json:"error"`
			Response *struct {
				Error *struct {
					Message string `json:"message"`
					Type    string `json:"type"`
				} `json:"error"`
			} `json:"response"`
		}
		if err := json.Unmarshal([]byte(data), &payload); err != nil {
			log.Warn().Err(err).Msg("openaicompat: failed to parse responses error event")
			return
		}
		errInfo := payload.Error
		if errInfo == nil && payload.Response != nil {
			errInfo = payload.Response.Error
		}
		if errInfo == nil {
			return
		}
		eventCh <- cif.CIFStreamError{
			Type: "stream_error",
			Error: cif.ErrorInfo{
				Type:    firstNonEmpty(errInfo.Type, "stream_error"),
				Message: errInfo.Message,
			},
		}
	}
}

func (s *responsesStreamState) ensureTextBlockIndex(outputIndex, contentIndex int) (int, bool) {
	key := s.textBlockKey(outputIndex, contentIndex)
	if index, ok := s.textBlockIndices[key]; ok {
		return index, false
	}
	index := s.nextContentIndex
	s.nextContentIndex++
	s.textBlockIndices[key] = index
	return index, true
}

func (s *responsesStreamState) textBlockKey(outputIndex, contentIndex int) string {
	return fmt.Sprintf("%d:%d", outputIndex, contentIndex)
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return value
		}
	}
	return ""
}
