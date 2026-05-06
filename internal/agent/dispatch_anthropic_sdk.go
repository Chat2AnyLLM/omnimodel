package agent

// dispatch_anthropic_sdk.go — DispatchFn backed by the official Anthropic Go SDK.
// Unlike NewChatCompletionsDispatch (which routes through the local OmniLLM proxy),
// AnthropicSDKDispatch connects directly to api.anthropic.com. The base URL is
// overridable via ANTHROPIC_BASE_URL so the call can still be proxied when needed.

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	anthropic "github.com/anthropics/anthropic-sdk-go"
	"github.com/anthropics/anthropic-sdk-go/option"

	"omnillm/internal/cif"
)

// AnthropicSDKDispatch returns a DispatchFn that calls the Anthropic Messages
// API directly using the official anthropic-sdk-go client.
//
// apiKey must be a valid Anthropic API key (or "" to let the SDK fall back to
// the ANTHROPIC_API_KEY environment variable).  Pass a non-empty baseURL to
// override the default https://api.anthropic.com endpoint — useful for
// pointing at OmniLLM's /v1/messages proxy or a local test server.
func AnthropicSDKDispatch(apiKey, baseURL string) DispatchFn {
	opts := []option.RequestOption{}
	if apiKey != "" {
		opts = append(opts, option.WithAPIKey(apiKey))
	}
	if baseURL != "" {
		opts = append(opts, option.WithBaseURL(baseURL))
	}
	client := anthropic.NewClient(opts...)

	return func(ctx context.Context, req *cif.CanonicalRequest) (<-chan *cif.CanonicalResponse, error) {
		params, err := cifToAnthropicParams(req)
		if err != nil {
			return nil, fmt.Errorf("anthropic-sdk: build params: %w", err)
		}

		msg, err := client.Messages.New(ctx, params)
		if err != nil {
			return nil, fmt.Errorf("anthropic-sdk: messages.new: %w", err)
		}

		resp := anthropicMsgToCIF(msg)

		ch := make(chan *cif.CanonicalResponse, 1)
		ch <- resp
		close(ch)
		return ch, nil
	}
}

// ─── conversion helpers ───────────────────────────────────────────────────────

const defaultAnthropicMaxTokens = 4096

func cifToAnthropicParams(req *cif.CanonicalRequest) (anthropic.MessageNewParams, error) {
	model := strings.TrimSpace(req.Model)
	if model == "" {
		model = "claude-opus-4-5"
	}

	maxTokens := int64(defaultAnthropicMaxTokens)
	if req.MaxTokens != nil && *req.MaxTokens > 0 {
		maxTokens = int64(*req.MaxTokens)
	}

	params := anthropic.MessageNewParams{
		Model:     anthropic.Model(model),
		MaxTokens: maxTokens,
	}

	if req.SystemPrompt != nil && strings.TrimSpace(*req.SystemPrompt) != "" {
		params.System = []anthropic.TextBlockParam{
			{Text: strings.TrimSpace(*req.SystemPrompt)},
		}
	}

	messages, err := cifMessagesToAnthropicParams(req.Messages)
	if err != nil {
		return params, err
	}
	params.Messages = messages

	if len(req.Tools) > 0 {
		tools := make([]anthropic.ToolUnionParam, 0, len(req.Tools))
		for _, t := range req.Tools {
			desc := ""
			if t.Description != nil {
				desc = *t.Description
			}
			tools = append(tools, anthropic.ToolUnionParam{
				OfTool: &anthropic.ToolParam{
					Name:        t.Name,
					Description: anthropic.String(desc),
					InputSchema: anthropic.ToolInputSchemaParam{
						Properties: t.ParametersSchema,
					},
				},
			})
		}
		params.Tools = tools
		if req.ToolChoice != nil {
			params.ToolChoice = canonicalToolChoiceToAnthropicSDK(req.ToolChoice)
		} else {
			params.ToolChoice = anthropic.ToolChoiceUnionParam{
				OfAuto: &anthropic.ToolChoiceAutoParam{},
			}
		}
	}

	return params, nil
}

func cifMessagesToAnthropicParams(msgs []cif.CIFMessage) ([]anthropic.MessageParam, error) {
	var out []anthropic.MessageParam
	for _, msg := range msgs {
		switch m := msg.(type) {
		case cif.CIFSystemMessage:
			// System messages are handled via params.System; skip inline.
		case cif.CIFUserMessage:
			blocks := cifUserPartsToAnthropicBlocks(m.Content)
			if len(blocks) > 0 {
				out = append(out, anthropic.NewUserMessage(blocks...))
			}
		case cif.CIFAssistantMessage:
			blocks := cifAssistantPartsToAnthropicBlocks(m.Content)
			if len(blocks) > 0 {
				out = append(out, anthropic.NewAssistantMessage(blocks...))
			}
		}
	}
	return out, nil
}

func cifUserPartsToAnthropicBlocks(parts []cif.CIFContentPart) []anthropic.ContentBlockParamUnion {
	var blocks []anthropic.ContentBlockParamUnion
	for _, part := range parts {
		switch p := part.(type) {
		case cif.CIFTextPart:
			blocks = append(blocks, anthropic.NewTextBlock(p.Text))
		case cif.CIFToolResultPart:
			isError := false
			if p.IsError != nil {
				isError = *p.IsError
			}
			blocks = append(blocks, anthropic.NewToolResultBlock(p.ToolCallID, p.Content, isError))
		}
	}
	return blocks
}

func cifAssistantPartsToAnthropicBlocks(parts []cif.CIFContentPart) []anthropic.ContentBlockParamUnion {
	var blocks []anthropic.ContentBlockParamUnion
	for _, part := range parts {
		switch p := part.(type) {
		case cif.CIFTextPart:
			blocks = append(blocks, anthropic.NewTextBlock(p.Text))
		case cif.CIFToolCallPart:
			// NewToolUseBlock(id, input, name) — note: input is any, name is last
			blocks = append(blocks, anthropic.NewToolUseBlock(p.ToolCallID, p.ToolArguments, p.ToolName))
		}
	}
	return blocks
}

func canonicalToolChoiceToAnthropicSDK(choice any) anthropic.ToolChoiceUnionParam {
	switch v := choice.(type) {
	case string:
		switch v {
		case "required":
			return anthropic.ToolChoiceUnionParam{OfAny: &anthropic.ToolChoiceAnyParam{}}
		case "none":
			return anthropic.ToolChoiceUnionParam{OfNone: &anthropic.ToolChoiceNoneParam{}}
		case "auto":
			fallthrough
		default:
			return anthropic.ToolChoiceUnionParam{OfAuto: &anthropic.ToolChoiceAutoParam{}}
		}
	case map[string]any:
		if typ, _ := v["type"].(string); typ == "function" {
			if name, _ := v["functionName"].(string); name != "" {
				return anthropic.ToolChoiceUnionParam{OfTool: &anthropic.ToolChoiceToolParam{Name: name}}
			}
		}
	}
	return anthropic.ToolChoiceUnionParam{OfAuto: &anthropic.ToolChoiceAutoParam{}}
}

func anthropicMsgToCIF(msg *anthropic.Message) *cif.CanonicalResponse {
	resp := &cif.CanonicalResponse{
		ID:    msg.ID,
		Model: string(msg.Model),
	}

	switch msg.StopReason {
	case anthropic.StopReasonToolUse:
		resp.StopReason = cif.StopReasonToolUse
	case anthropic.StopReasonMaxTokens:
		resp.StopReason = cif.StopReasonMaxTokens
	case anthropic.StopReasonStopSequence:
		resp.StopReason = cif.StopReasonStopSequence
	default:
		resp.StopReason = cif.StopReasonEndTurn
	}

	resp.Usage = &cif.CIFUsage{
		InputTokens:  int(msg.Usage.InputTokens),
		OutputTokens: int(msg.Usage.OutputTokens),
	}

	for _, block := range msg.Content {
		switch block.Type {
		case "text":
			resp.Content = append(resp.Content, cif.CIFTextPart{
				Type: "text",
				Text: block.Text,
			})
		case "tool_use":
			args := rawMessageToMap(block.Input)
			resp.Content = append(resp.Content, cif.CIFToolCallPart{
				Type:          "tool_use",
				ToolCallID:    block.ID,
				ToolName:      block.Name,
				ToolArguments: args,
			})
		}
	}

	return resp
}

func rawMessageToMap(raw json.RawMessage) map[string]any {
	if len(raw) == 0 {
		return map[string]any{}
	}
	var m map[string]any
	if err := json.Unmarshal(raw, &m); err != nil {
		return map[string]any{}
	}
	return m
}
