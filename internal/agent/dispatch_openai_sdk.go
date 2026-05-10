package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"omnillm/internal/tools"

	openai "github.com/openai/openai-go/v3"
	"github.com/openai/openai-go/v3/option"
	"github.com/openai/openai-go/v3/shared"
)

func OpenAISDKDispatch(apiKey, baseURL, model string) DispatchFn {
	opts := []option.RequestOption{}
	if apiKey != "" {
		opts = append(opts, option.WithAPIKey(apiKey))
	}
	if baseURL != "" {
		opts = append(opts, option.WithBaseURL(baseURL))
	}
	client := openai.NewClient(opts...)

	return func(ctx context.Context, req *MessagesRequest) (<-chan *MessagesResponse, error) {
		if req.Model == "" {
			req.Model = model
		}
		params, err := openAIParamsFromRequest(req)
		if err != nil {
			return nil, fmt.Errorf("openai-sdk: build params: %w", err)
		}

		completion, err := client.Chat.Completions.New(ctx, params)
		if err != nil {
			return nil, fmt.Errorf("openai-sdk: chat.completions.new: %w", err)
		}

		resp := openAICompletionToResponse(completion)
		ch := make(chan *MessagesResponse, 1)
		ch <- resp
		close(ch)
		return ch, nil
	}
}

func openAIParamsFromRequest(req *MessagesRequest) (openai.ChatCompletionNewParams, error) {
	if req == nil {
		req = &MessagesRequest{}
	}
	model := strings.TrimSpace(req.Model)
	if model == "" {
		model = "gpt-4o-mini"
	}
	messages, err := messagesToOpenAIParams(req.System, req.Messages)
	if err != nil {
		return openai.ChatCompletionNewParams{}, err
	}
	params := openai.ChatCompletionNewParams{
		Model:    shared.ChatModel(model),
		Messages: messages,
	}
	if req.MaxTokens > 0 {
		params.MaxCompletionTokens = openai.Int(int64(req.MaxTokens))
	}
	if len(req.Tools) > 0 {
		params.Tools = toolsToOpenAIParams(req.Tools)
		params.ToolChoice = toolChoiceToOpenAIParam(req.ToolChoice)
	}
	return params, nil
}

func messagesToOpenAIParams(system []ContentBlock, messages []Message) ([]openai.ChatCompletionMessageParamUnion, error) {
	out := make([]openai.ChatCompletionMessageParamUnion, 0, len(system)+len(messages))
	if text := joinTextBlocks(system); text != "" {
		out = append(out, openai.ChatCompletionMessageParamUnion{OfSystem: &openai.ChatCompletionSystemMessageParam{
			Content: openai.ChatCompletionSystemMessageParamContentUnion{OfString: openai.String(text)},
		}})
	}
	for _, msg := range messages {
		text := joinTextBlocks(msg.Content)
		switch msg.Role {
		case "system":
			if text != "" {
				out = append(out, openai.ChatCompletionMessageParamUnion{OfSystem: &openai.ChatCompletionSystemMessageParam{Content: openai.ChatCompletionSystemMessageParamContentUnion{OfString: openai.String(text)}}})
			}
		case "assistant":
			assistant := openai.ChatCompletionAssistantMessageParam{}
			if text != "" {
				assistant.Content = openai.ChatCompletionAssistantMessageParamContentUnion{OfString: openai.String(text)}
			}
			for _, block := range msg.Content {
				if block.Type != "tool_use" {
					continue
				}
				arguments, err := json.Marshal(block.Input)
				if err != nil {
					return nil, fmt.Errorf("marshal tool input %q: %w", block.ID, err)
				}
				assistant.ToolCalls = append(assistant.ToolCalls, openai.ChatCompletionMessageToolCallUnionParam{OfFunction: &openai.ChatCompletionMessageFunctionToolCallParam{
					ID: block.ID,
					Function: openai.ChatCompletionMessageFunctionToolCallFunctionParam{
						Name:      block.Name,
						Arguments: string(arguments),
					},
				}})
			}
			if text != "" || len(assistant.ToolCalls) > 0 {
				out = append(out, openai.ChatCompletionMessageParamUnion{OfAssistant: &assistant})
			}
		default:
			if toolResults := toolResultBlocksToOpenAIParams(msg.Content); len(toolResults) > 0 {
				out = append(out, toolResults...)
			} else if text != "" {
				out = append(out, openai.ChatCompletionMessageParamUnion{OfUser: &openai.ChatCompletionUserMessageParam{Content: openai.ChatCompletionUserMessageParamContentUnion{OfString: openai.String(text)}}})
			}
		}
	}
	return out, nil
}

func joinTextBlocks(blocks []ContentBlock) string {
	var parts []string
	for _, block := range blocks {
		if block.Type == "text" && strings.TrimSpace(block.Text) != "" {
			parts = append(parts, block.Text)
		}
	}
	return strings.Join(parts, "\n")
}

func toolResultBlocksToOpenAIParams(blocks []ContentBlock) []openai.ChatCompletionMessageParamUnion {
	out := make([]openai.ChatCompletionMessageParamUnion, 0, len(blocks))
	for _, block := range blocks {
		if block.Type != "tool_result" {
			continue
		}
		toolCallID := block.ToolUseID
		if toolCallID == "" {
			toolCallID = block.ID
		}
		out = append(out, openai.ChatCompletionMessageParamUnion{OfTool: &openai.ChatCompletionToolMessageParam{
			ToolCallID: toolCallID,
			Content:    openai.ChatCompletionToolMessageParamContentUnion{OfString: openai.String(block.Content)},
		}})
	}
	return out
}

func toolsToOpenAIParams(toolDefs []tools.ToolDefinition) []openai.ChatCompletionToolUnionParam {
	out := make([]openai.ChatCompletionToolUnionParam, 0, len(toolDefs))
	for _, tool := range toolDefs {
		description := ""
		if tool.Description != nil {
			description = *tool.Description
		}
		out = append(out, openai.ChatCompletionToolUnionParam{OfFunction: &openai.ChatCompletionFunctionToolParam{
			Function: shared.FunctionDefinitionParam{
				Name:        tool.Name,
				Description: openai.String(description),
				Parameters:  shared.FunctionParameters(tool.InputSchema),
			},
		}})
	}
	return out
}

func toolChoiceToOpenAIParam(choice any) openai.ChatCompletionToolChoiceOptionUnionParam {
	switch v := choice.(type) {
	case string:
		switch v {
		case "required":
			return openai.ChatCompletionToolChoiceOptionUnionParam{OfAuto: openai.String("required")}
		case "none":
			return openai.ChatCompletionToolChoiceOptionUnionParam{OfAuto: openai.String("none")}
		case "auto":
			fallthrough
		default:
			return openai.ChatCompletionToolChoiceOptionUnionParam{OfAuto: openai.String("auto")}
		}
	case map[string]any:
		if typ, _ := v["type"].(string); typ == "function" {
			name, _ := v["functionName"].(string)
			if name == "" {
				name, _ = v["name"].(string)
			}
			if name != "" {
				return openai.ChatCompletionToolChoiceOptionUnionParam{OfFunctionToolChoice: &openai.ChatCompletionNamedToolChoiceParam{Function: openai.ChatCompletionNamedToolChoiceFunctionParam{Name: name}}}
			}
		}
	}
	return openai.ChatCompletionToolChoiceOptionUnionParam{OfAuto: openai.String("auto")}
}

func openAICompletionToResponse(completion *openai.ChatCompletion) *MessagesResponse {
	resp := &MessagesResponse{StopReason: StopReasonEndTurn}
	if completion == nil {
		return resp
	}
	resp.ID = completion.ID
	resp.Model = completion.Model
	resp.Usage = &Usage{InputTokens: int(completion.Usage.PromptTokens), OutputTokens: int(completion.Usage.CompletionTokens)}
	if len(completion.Choices) == 0 {
		return resp
	}
	choice := completion.Choices[0]
	resp.StopReason = openAIFinishReason(choice.FinishReason)
	if choice.Message.Content != "" {
		resp.Content = append(resp.Content, TextBlock(choice.Message.Content))
	}
	for _, toolCall := range choice.Message.ToolCalls {
		if toolCall.Function.Name == "" {
			continue
		}
		input := map[string]any{}
		if strings.TrimSpace(toolCall.Function.Arguments) != "" {
			_ = json.Unmarshal([]byte(toolCall.Function.Arguments), &input)
		}
		resp.Content = append(resp.Content, ContentBlock{Type: "tool_use", ID: toolCall.ID, Name: toolCall.Function.Name, Input: input})
	}
	return resp
}

func openAIFinishReason(reason string) StopReason {
	switch reason {
	case "tool_calls", "function_call":
		return StopReasonToolUse
	case "length":
		return StopReasonMaxTokens
	case "content_filter":
		return StopReasonContentFilter
	default:
		return StopReasonEndTurn
	}
}
