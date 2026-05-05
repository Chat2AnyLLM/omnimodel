package shared

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"omnillm/internal/cif"
	"strings"
	"time"

	"github.com/rs/zerolog/log"
)

var (
	openAIHTTPClient = &http.Client{
		Timeout: 120 * time.Second,
		Transport: &http.Transport{
			Proxy:                 http.ProxyFromEnvironment,
			ForceAttemptHTTP2:     true,
			MaxIdleConns:          100,
			MaxIdleConnsPerHost:   20,
			MaxConnsPerHost:       50,
			IdleConnTimeout:       90 * time.Second,
			TLSHandshakeTimeout:   10 * time.Second,
			ExpectContinueTimeout: 1 * time.Second,
		},
	}
	openAIStreamClient = &http.Client{
		Transport: &http.Transport{
			Proxy:                 http.ProxyFromEnvironment,
			ForceAttemptHTTP2:     true,
			MaxIdleConns:          100,
			MaxIdleConnsPerHost:   20,
			MaxConnsPerHost:       50,
			IdleConnTimeout:       90 * time.Second,
			TLSHandshakeTimeout:   10 * time.Second,
			ExpectContinueTimeout: 1 * time.Second,
		},
	}
)

// ExecuteOpenAIWithPayload performs a non-streaming OpenAI-compatible HTTP request.
func ExecuteOpenAIWithPayload(ctx context.Context, url string, headers map[string]string, payload map[string]interface{}) (*cif.CanonicalResponse, error) {
	body, err := json.Marshal(payload)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal request: %w", err)
	}

	log.Trace().Str("url", url).RawJSON("payload", body).Msg("outbound proxy request payload")

	req, err := http.NewRequestWithContext(ctx, "POST", url, bytes.NewBuffer(body))
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}
	for k, v := range headers {
		req.Header.Set(k, v)
	}

	resp, err := openAIHTTPClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("API request failed with status %d: %s", resp.StatusCode, string(b))
	}

	var openaiResp map[string]interface{}
	if err := json.NewDecoder(resp.Body).Decode(&openaiResp); err != nil {
		return nil, fmt.Errorf("failed to decode response: %w", err)
	}

	return OpenAIRespToCIF(openaiResp), nil
}

// StreamOpenAIWithPayload performs a streaming OpenAI-compatible HTTP request.
func StreamOpenAIWithPayload(ctx context.Context, url string, headers map[string]string, payload map[string]interface{}) (<-chan cif.CIFStreamEvent, error) {
	body, err := json.Marshal(payload)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal request: %w", err)
	}

	log.Trace().Str("url", url).RawJSON("payload", body).Msg("outbound proxy request payload")

	req, err := http.NewRequestWithContext(ctx, "POST", url, bytes.NewBuffer(body))
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	req.Header.Set("Accept", "text/event-stream")

	resp, err := openAIStreamClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("streaming request failed: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		return nil, fmt.Errorf("streaming API request failed with status %d: %s", resp.StatusCode, string(b))
	}

	eventCh := make(chan cif.CIFStreamEvent, 64)
	go ParseOpenAISSE(resp.Body, eventCh)
	return eventCh, nil
}

// nonEmptyStringPtr returns a pointer to s if s contains non-whitespace text,
// otherwise nil. This avoids repeating the "trim + take address" pattern
// across provider parsers.
func nonEmptyStringPtr(s string) *string {
	s = strings.TrimSpace(s)
	if s == "" {
		return nil
	}
	return &s
}

// CIFMessagesToOpenAI converts CIF messages to the OpenAI chat completions format.
func CIFMessagesToOpenAI(messages []cif.CIFMessage) []map[string]interface{} {
	var result []map[string]interface{}
	for _, msg := range messages {
		switch m := msg.(type) {
		case cif.CIFSystemMessage:
			result = append(result, map[string]interface{}{
				"role":    "system",
				"content": m.Content,
			})
		case cif.CIFUserMessage:
			openaiMsg := map[string]interface{}{"role": "user"}
			if len(m.Content) == 1 {
				if textPart, ok := m.Content[0].(cif.CIFTextPart); ok {
					openaiMsg["content"] = textPart.Text
					result = append(result, openaiMsg)
					continue
				}
			}
			var parts []map[string]interface{}
			for _, part := range m.Content {
				switch p := part.(type) {
				case cif.CIFTextPart:
					parts = append(parts, map[string]interface{}{"type": "text", "text": p.Text})
				case cif.CIFToolResultPart:
					content := p.Content
					if p.IsError != nil && *p.IsError && content == "" {
						content = "Error: tool call failed"
					}
					result = append(result, map[string]interface{}{
						"role":         "tool",
						"tool_call_id": p.ToolCallID,
						"content":      content,
					})
					continue
				case cif.CIFImagePart:
					imgURL := map[string]interface{}{}
					if p.Data != nil {
						imgURL["url"] = fmt.Sprintf("data:%s;base64,%s", p.MediaType, *p.Data)
					} else if p.URL != nil {
						imgURL["url"] = *p.URL
					}
					parts = append(parts, map[string]interface{}{"type": "image_url", "image_url": imgURL})
				}
			}
			if len(parts) > 0 {
				openaiMsg["content"] = parts
				result = append(result, openaiMsg)
			}
		case cif.CIFAssistantMessage:
			openaiMsg := map[string]interface{}{"role": "assistant"}
			var textBuf strings.Builder
			var reasoningContent string
			var reasoningSignature *string
			var toolCalls []map[string]interface{}
			for _, part := range m.Content {
				switch p := part.(type) {
				case cif.CIFTextPart:
					textBuf.WriteString(p.Text)
				case cif.CIFThinkingPart:
					reasoningContent = p.Thinking
					if p.Signature != nil {
						reasoningSignature = nonEmptyStringPtr(*p.Signature)
					}
				case cif.CIFToolCallPart:
					args, _ := json.Marshal(p.ToolArguments)
					toolCalls = append(toolCalls, map[string]interface{}{
						"id":      p.ToolCallID,
						"call_id": p.ToolCallID,
						"type":    "function",
						"function": map[string]interface{}{
							"name":      p.ToolName,
							"arguments": string(args),
						},
					})
				}
			}
			if textBuf.Len() > 0 {
				openaiMsg["content"] = textBuf.String()
			}
			if reasoningContent != "" {
				openaiMsg["reasoning_content"] = reasoningContent
				if reasoningSignature != nil {
					openaiMsg["reasoning_signature"] = *reasoningSignature
				}
			}
			if len(toolCalls) > 0 {
				openaiMsg["tool_calls"] = toolCalls
			}
			result = append(result, openaiMsg)
		}
	}
	return result
}

// OpenAIRespToCIF converts an OpenAI chat completions response to CIF format.
func OpenAIRespToCIF(resp map[string]interface{}) *cif.CanonicalResponse {
	id, _ := resp["id"].(string)
	model, _ := resp["model"].(string)
	result := &cif.CanonicalResponse{ID: id, Model: model, StopReason: cif.StopReasonEndTurn}

	if choices, ok := resp["choices"].([]interface{}); ok && len(choices) > 0 {
		if choice, ok := choices[0].(map[string]interface{}); ok {
			if fr, ok := choice["finish_reason"].(string); ok {
				result.StopReason = OpenAIStopReason(fr)
			}
			if message, ok := choice["message"].(map[string]interface{}); ok {
				if content, ok := message["content"].(string); ok && content != "" {
					result.Content = append(result.Content, cif.CIFTextPart{Type: "text", Text: content})
				}
				if toolCalls, ok := message["tool_calls"].([]interface{}); ok {
					for _, tc := range toolCalls {
						tcMap, ok := tc.(map[string]interface{})
						if !ok {
							continue
						}
						if function, ok := tcMap["function"].(map[string]interface{}); ok {
							id, _ := tcMap["id"].(string)
							name, _ := function["name"].(string)
							args, _ := function["arguments"].(string)
							var toolArgs map[string]interface{}
							json.Unmarshal([]byte(args), &toolArgs) //nolint:errcheck
							result.Content = append(result.Content, cif.CIFToolCallPart{
								Type:          "tool_call",
								ToolCallID:    id,
								ToolName:      name,
								ToolArguments: toolArgs,
							})
						}
					}
				}
			}
		}
	}

	if usage, ok := resp["usage"].(map[string]interface{}); ok {
		pt, _ := usage["prompt_tokens"].(float64)
		ct, _ := usage["completion_tokens"].(float64)
		result.Usage = &cif.CIFUsage{InputTokens: int(pt), OutputTokens: int(ct)}
	}

	return result
}

// OpenAIStopReason converts an OpenAI finish_reason to a CIF stop reason.
func OpenAIStopReason(reason string) cif.CIFStopReason {
	switch reason {
	case "stop":
		return cif.StopReasonEndTurn
	case "length":
		return cif.StopReasonMaxTokens
	case "tool_calls":
		return cif.StopReasonToolUse
	case "content_filter":
		return cif.StopReasonContentFilter
	default:
		return cif.StopReasonEndTurn
	}
}

// NormalizeOpenAICompatibleAPIFormat canonicalizes supported OpenAI-compatible
// upstream API format aliases.
func NormalizeOpenAICompatibleAPIFormat(raw string) string {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case "responses", "response", "openai-responses", "openai_responses":
		return "responses"
	case "chat", "chat.completions", "chat_completions", "openai-chat", "openai_chat":
		return "chat.completions"
	default:
		return ""
	}
}
