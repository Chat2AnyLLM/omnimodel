package tools

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

type codeSearchTool struct{}

func CodeSearch() Tool { return &codeSearchTool{} }

func (t *codeSearchTool) Name() string { return "codesearch" }

func (t *codeSearchTool) Description() string {
	return "Search code and documentation context using Exa's code context endpoint. Useful for library APIs, framework examples, and implementation references."
}

func (t *codeSearchTool) InputSchema() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"query": map[string]any{"type": "string", "description": "Search query for code/documentation context."},
			"tokensNum": map[string]any{"type": "integer", "description": "Approximate number of tokens to return (1000-50000, default 5000)."},
		},
		"required": []string{"query"},
	}
}

func (t *codeSearchTool) Execute(ctx context.Context, call Context, input json.RawMessage) Result {
	var p struct {
		Query     string `json:"query"`
		TokensNum int    `json:"tokensNum"`
	}
	if err := json.Unmarshal(input, &p); err != nil {
		return Result{Output: "error: " + err.Error(), IsError: true}
	}
	p.Query = strings.TrimSpace(p.Query)
	if p.Query == "" {
		return Result{Output: "error: query is required", IsError: true}
	}
	if p.TokensNum <= 0 {
		p.TokensNum = 5000
	}
	if p.TokensNum < 1000 {
		p.TokensNum = 1000
	}
	if p.TokensNum > 50000 {
		p.TokensNum = 50000
	}

	requestCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()

	body := map[string]any{
		"jsonrpc": "2.0",
		"id":      1,
		"method":  "tools/call",
		"params": map[string]any{
			"name": "get_code_context_exa",
			"arguments": map[string]any{
				"query":     p.Query,
				"tokensNum": p.TokensNum,
			},
		},
	}
	payload, err := json.Marshal(body)
	if err != nil {
		return Result{Output: "error: marshal request: " + err.Error(), IsError: true}
	}

	req, err := http.NewRequestWithContext(requestCtx, http.MethodPost, "https://mcp.exa.ai/mcp", bytes.NewReader(payload))
	if err != nil {
		return Result{Output: "error: build request: " + err.Error(), IsError: true}
	}
	req.Header.Set("accept", "application/json, text/event-stream")
	req.Header.Set("content-type", "application/json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return Result{Output: "error: " + err.Error(), IsError: true}
	}
	defer resp.Body.Close()

	responseText, err := io.ReadAll(io.LimitReader(resp.Body, 2*1024*1024))
	if err != nil {
		return Result{Output: "error: read response: " + err.Error(), IsError: true}
	}
	if resp.StatusCode >= 400 {
		return Result{Output: fmt.Sprintf("error: code search (%d): %s", resp.StatusCode, strings.TrimSpace(string(responseText))), IsError: true}
	}

	for line := range strings.SplitSeq(string(responseText), "\n") {
		after, ok := strings.CutPrefix(line, "data: ")
		if !ok {
			continue
		}
		var decoded struct {
			Result struct {
				Content []struct {
					Type string `json:"type"`
					Text string `json:"text"`
				} `json:"content"`
			} `json:"result"`
		}
		if err := json.Unmarshal([]byte(after), &decoded); err != nil {
			continue
		}
		if len(decoded.Result.Content) > 0 && decoded.Result.Content[0].Text != "" {
			return Result{Title: "Code search", Output: decoded.Result.Content[0].Text}
		}
	}

	return Result{Output: "No code snippets or documentation found. Try a more specific query."}
}
