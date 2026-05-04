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
//
// Per the OpenAI chat completions spec, tools and tool_choice are only included
// when tools are registered, and stream_options is only included when stream:
// true.
func NewChatCompletionsDispatch(c Client, model string) DispatchFn {
	return func(ctx context.Context, req *cif.CanonicalRequest) (<-chan *cif.CanonicalResponse, error) {
		requestModel := strings.TrimSpace(model)
		if requestModel == "" {
			requestModel = strings.TrimSpace(req.Model)
		}
		if requestModel == "" {
			requestModel = "gpt-4"
		}

		response, err := doPost(c, requestModel, req)
		if err != nil {
			return nil, err
		}

		ch := make(chan *cif.CanonicalResponse, 1)
		ch <- response
		close(ch)
		return ch, nil
	}
}

// doPost builds a spec-compliant OpenAI chat completions payload and posts it
// to the local proxy.  It uses openaicompat.Marshal (not json.Marshal) so that
// the request is properly serialised — in particular the Extras side-channel is
// merged and the user field is sanitised, exactly as the proxy-side providers do.
func doPost(c Client, model string, req *cif.CanonicalRequest) (*cif.CanonicalResponse, error) {
	chatReq, err := openaicompat.BuildChatRequest(model, req, false, openaicompat.Config{})
	if err != nil {
		return nil, fmt.Errorf("build chat request: %w", err)
	}

	// Marshal once via openaicompat.Marshal to produce correct JSON, then wrap
	// as a rawPayload so that c.Post's internal json.Marshal encodes it as-is
	// (json.Marshal(json.RawMessage) re-encodes the bytes verbatim).
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
	encoded, _ := json.Marshal(args)
	var buf bytes.Buffer
	buf.WriteString("Allow tool execution?\n")
	buf.WriteString("Tool: ")
	buf.WriteString(toolName)
	buf.WriteString("\nArguments: ")
	buf.Write(encoded)
	return buf.String()
}
