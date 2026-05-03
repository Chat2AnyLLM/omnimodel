package agent

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"

	"omnillm/internal/cif"
	"omnillm/internal/providers/openaicompat"
)

// Client is the minimal transport interface the agent runtime needs from the CLI client.
type Client interface {
	Post(path string, body any) ([]byte, error)
	PostStream(path string, body any) (*http.Response, error)
}

// NewChatCompletionsDispatch returns a DispatchFn backed by OmniLLM's existing /v1/chat/completions path.
func NewChatCompletionsDispatch(c Client, model string) DispatchFn {
	return func(ctx context.Context, req *cif.CanonicalRequest) (<-chan *cif.CanonicalResponse, error) {
		requestModel := strings.TrimSpace(model)
		if requestModel == "" {
			requestModel = strings.TrimSpace(req.Model)
		}
		if requestModel == "" {
			requestModel = "gpt-4"
		}

		chatReq, err := openaicompat.BuildChatRequest(requestModel, req, false, openaicompat.Config{})
		if err != nil {
			return nil, fmt.Errorf("build chat request: %w", err)
		}

		data, err := c.Post("/v1/chat/completions", chatReq)
		if err != nil {
			return nil, fmt.Errorf("failed to create chat completion: %w", err)
		}

		var chatResp openaicompat.ChatResponse
		if err := json.Unmarshal(data, &chatResp); err != nil {
			return nil, fmt.Errorf("decode chat completion: %w", err)
		}

		response := openaicompat.ParseChatResponse(&chatResp)
		ch := make(chan *cif.CanonicalResponse, 1)
		ch <- response
		close(ch)
		return ch, nil
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
func EncodePermissionPrompt(req PermissionRequest) string {
	args, _ := json.Marshal(req.Arguments)
	var buf bytes.Buffer
	buf.WriteString("Allow tool execution?\n")
	buf.WriteString("Tool: ")
	buf.WriteString(req.ToolName)
	buf.WriteString("\nArguments: ")
	buf.Write(args)
	return buf.String()
}
