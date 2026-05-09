package agent

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"sort"
	"strings"
)

// Client is the minimal transport interface the agent runtime needs from the CLI client.
type Client interface {
	Post(path string, body any) ([]byte, error)
	PostStream(path string, body any) (*http.Response, error)
}

type OmniLLMClientConfig interface {
	GetBaseURL() string
	GetAPIKey() string
}

// NewChatCompletionsDispatch returns a DispatchFn backed by OmniLLM's local proxy.
// All models are sent to /v1/messages and OmniLLM translates the request upstream.
func NewChatCompletionsDispatch(c Client, model string) DispatchFn {
	return NewDispatch(c, model, "anthropic")
}

func NewDispatch(c Client, model, _ string) DispatchFn {
	return func(ctx context.Context, req *MessagesRequest) (<-chan *MessagesResponse, error) {
		request := cloneMessagesRequest(req)
		requestModel := strings.TrimSpace(model)
		if requestModel == "" {
			requestModel = strings.TrimSpace(request.Model)
		}
		if requestModel == "" {
			requestModel = "gpt-4"
		}
		request.Model = requestModel

		var (
			response *MessagesResponse
			err      error
		)
		if request.Stream {
			response, err = doAnthropicStreamPost(ctx, c, request)
		} else {
			response, err = doAnthropicPost(c, request)
		}
		if err != nil {
			return nil, err
		}

		ch := make(chan *MessagesResponse, 1)
		ch <- response
		close(ch)
		return ch, nil
	}
}

func cloneMessagesRequest(req *MessagesRequest) *MessagesRequest {
	if req == nil {
		return &MessagesRequest{MaxTokens: 4096}
	}
	cloned := *req
	if cloned.MaxTokens <= 0 {
		cloned.MaxTokens = 4096
	}
	if req.System != nil {
		cloned.System = append([]ContentBlock(nil), req.System...)
	}
	if req.Messages != nil {
		cloned.Messages = append([]Message(nil), req.Messages...)
	}
	if req.Tools != nil {
		cloned.Tools = append(cloned.Tools[:0:0], req.Tools...)
	}
	return &cloned
}

func doAnthropicPost(c Client, req *MessagesRequest) (*MessagesResponse, error) {
	payload, err := buildAnthropicMessagesJSON(req.Model, req, false)
	if err != nil {
		return nil, fmt.Errorf("build anthropic messages request: %w", err)
	}

	data, err := c.Post("/v1/messages", payload)
	if err != nil {
		return nil, fmt.Errorf("failed to create anthropic message: %w", err)
	}

	var response MessagesResponse
	if err := json.Unmarshal(data, &response); err != nil {
		return nil, fmt.Errorf("decode anthropic message response: %w", err)
	}
	if response.StopReason == "" {
		response.StopReason = StopReasonEndTurn
	}
	return &response, nil
}

func doAnthropicStreamPost(_ context.Context, c Client, req *MessagesRequest) (*MessagesResponse, error) {
	payload, err := buildAnthropicMessagesJSON(req.Model, req, true)
	if err != nil {
		return nil, fmt.Errorf("build anthropic messages request: %w", err)
	}

	resp, err := c.PostStream("/v1/messages", payload)
	if err != nil {
		return nil, fmt.Errorf("failed to stream anthropic message: %w", err)
	}
	defer resp.Body.Close()

	return collectAnthropicStream(resp.Body)
}

func collectAnthropicStream(body io.Reader) (*MessagesResponse, error) {
	scanner := bufio.NewScanner(body)
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)

	response := &MessagesResponse{}
	type blockState struct {
		block       ContentBlock
		partialJSON strings.Builder
	}
	blocks := map[int]*blockState{}
	orderedIndexes := map[int]struct{}{}

	var eventName string
	var dataLines []string

	applyPayload := func(name string, payload map[string]any) error {
		switch name {
		case "message_start":
			message, _ := payload["message"].(map[string]any)
			response.ID, _ = message["id"].(string)
			response.Model, _ = message["model"].(string)
		case "content_block_start":
			index := intValue(payload["index"])
			blockMap, _ := payload["content_block"].(map[string]any)
			blocks[index] = &blockState{block: anthropicBlockFromMap(blockMap)}
			orderedIndexes[index] = struct{}{}
		case "content_block_delta":
			index := intValue(payload["index"])
			state, ok := blocks[index]
			if !ok {
				state = &blockState{}
				blocks[index] = state
				orderedIndexes[index] = struct{}{}
			}
			deltaMap, _ := payload["delta"].(map[string]any)
			switch deltaMap["type"] {
			case "text_delta":
				text, _ := deltaMap["text"].(string)
				state.block.Text += text
			case "thinking_delta":
				thinking, _ := deltaMap["thinking"].(string)
				state.block.Thinking += thinking
			case "input_json_delta":
				partial, _ := deltaMap["partial_json"].(string)
				state.partialJSON.WriteString(partial)
			}
		case "message_delta":
			deltaMap, _ := payload["delta"].(map[string]any)
			if reason, _ := deltaMap["stop_reason"].(string); reason != "" {
				response.StopReason = StopReason(reason)
			}
			if usageMap, ok := payload["usage"].(map[string]any); ok {
				if response.Usage == nil {
					response.Usage = &Usage{}
				}
				response.Usage.OutputTokens = intValue(usageMap["output_tokens"])
			}
		case "error":
			errMap, _ := payload["error"].(map[string]any)
			message, _ := errMap["message"].(string)
			if message == "" {
				message = "anthropic stream error"
			}
			return fmt.Errorf("%s", message)
		}
		return nil
	}

	emit := func() error {
		if len(dataLines) == 0 {
			return nil
		}
		data := strings.Join(dataLines, "\n")
		dataLines = nil
		if strings.TrimSpace(data) == "" {
			return nil
		}
		var payload map[string]any
		if err := json.Unmarshal([]byte(data), &payload); err != nil {
			return err
		}
		return applyPayload(eventName, payload)
	}

	for scanner.Scan() {
		line := scanner.Text()
		if line == "" {
			if err := emit(); err != nil {
				return nil, err
			}
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
		return nil, err
	}
	if err := emit(); err != nil {
		return nil, err
	}

	indexes := make([]int, 0, len(orderedIndexes))
	for index := range orderedIndexes {
		indexes = append(indexes, index)
	}
	sort.Ints(indexes)
	response.Content = make([]ContentBlock, 0, len(indexes))
	for _, index := range indexes {
		state := blocks[index]
		if state == nil {
			continue
		}
		if state.partialJSON.Len() > 0 {
			state.block.Input = map[string]any{}
			if err := json.Unmarshal([]byte(state.partialJSON.String()), &state.block.Input); err != nil {
				return nil, fmt.Errorf("decode tool input json: %w", err)
			}
		}
		response.Content = append(response.Content, state.block)
	}
	if response.StopReason == "" {
		response.StopReason = StopReasonEndTurn
	}
	return response, nil
}

func anthropicBlockFromMap(m map[string]any) ContentBlock {
	block := ContentBlock{}
	block.Type, _ = m["type"].(string)
	block.Text, _ = m["text"].(string)
	block.Thinking, _ = m["thinking"].(string)
	block.ID, _ = m["id"].(string)
	block.ToolUseID, _ = m["tool_use_id"].(string)
	block.Name, _ = m["name"].(string)
	block.Content, _ = m["content"].(string)
	block.Input, _ = m["input"].(map[string]any)
	if isErr, ok := m["is_error"].(bool); ok {
		block.IsError = &isErr
	}
	if signature, ok := m["signature"].(string); ok && signature != "" {
		block.Signature = &signature
	}
	return block
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
	scanner := bufio.NewScanner(body)
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	for scanner.Scan() {
		line := scanner.Text()
		if !strings.HasPrefix(line, "data:") {
			continue
		}
		payload := strings.TrimSpace(strings.TrimPrefix(line, "data:"))
		if payload == "" || payload == "[DONE]" {
			continue
		}
		var chunk map[string]any
		if err := json.Unmarshal([]byte(payload), &chunk); err != nil {
			return out.String(), err
		}
		if errRaw, ok := chunk["error"].(map[string]any); ok {
			message, _ := errRaw["message"].(string)
			if message == "" {
				message = "stream error"
			}
			return out.String(), fmt.Errorf("%s", message)
		}
		choices, _ := chunk["choices"].([]any)
		for _, rawChoice := range choices {
			choice, _ := rawChoice.(map[string]any)
			delta, _ := choice["delta"].(map[string]any)
			content, _ := delta["content"].(string)
			out.WriteString(content)
		}
	}
	if err := scanner.Err(); err != nil {
		return out.String(), err
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
