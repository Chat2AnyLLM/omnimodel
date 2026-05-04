package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"
	"unicode"
)

type currentTimeTool struct{}

func CurrentTime() Tool { return &currentTimeTool{} }

func (t *currentTimeTool) Name() string { return "get_current_time" }

func (t *currentTimeTool) Description() string {
	return "Returns the current date and time in RFC3339 format."
}

func (t *currentTimeTool) InputSchema() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"timezone": map[string]any{"type": "string", "description": "Optional IANA timezone name, e.g. Asia/Shanghai."},
		},
	}
}

func (t *currentTimeTool) Execute(ctx context.Context, call Context, input json.RawMessage) Result {
	var p struct {
		Timezone string `json:"timezone"`
	}
	if err := json.Unmarshal(input, &p); err != nil && len(strings.TrimSpace(string(input))) > 0 && string(input) != "null" {
		return Result{Output: "error: " + err.Error(), IsError: true}
	}

	now := time.Now()
	if strings.TrimSpace(p.Timezone) != "" {
		loc, err := time.LoadLocation(strings.TrimSpace(p.Timezone))
		if err != nil {
			return Result{Output: "error: invalid timezone: " + err.Error(), IsError: true}
		}
		now = now.In(loc)
	}
	return Result{Output: now.Format(time.RFC3339)}
}

type webFetchTool struct{}

func WebFetch() Tool { return &webFetchTool{} }

func (t *webFetchTool) Name() string { return "web_fetch" }

func (t *webFetchTool) Description() string {
	return "Fetch a URL via HTTP GET and return the response body as text, truncated to a safe size."
}

func (t *webFetchTool) InputSchema() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"url":             map[string]any{"type": "string", "description": "The URL to fetch."},
			"timeout_seconds": map[string]any{"type": "integer", "description": "Optional timeout in seconds (default 30)."},
			"max_bytes":       map[string]any{"type": "integer", "description": "Optional maximum response bytes to read (default 4096)."},
		},
		"required": []string{"url"},
	}
}

func (t *webFetchTool) Execute(ctx context.Context, call Context, input json.RawMessage) Result {
	var p struct {
		URL            string `json:"url"`
		TimeoutSeconds int    `json:"timeout_seconds"`
		MaxBytes       int64  `json:"max_bytes"`
	}
	if err := json.Unmarshal(input, &p); err != nil {
		return Result{Output: "error: " + err.Error(), IsError: true}
	}
	p.URL = strings.TrimSpace(p.URL)
	if p.URL == "" {
		return Result{Output: "error: url is required", IsError: true}
	}
	if p.MaxBytes <= 0 {
		p.MaxBytes = 4096
	}

	timeout := defaultTimeout
	if p.TimeoutSeconds > 0 {
		timeout = time.Duration(p.TimeoutSeconds) * time.Second
	}
	requestCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	req, err := http.NewRequestWithContext(requestCtx, http.MethodGet, p.URL, nil)
	if err != nil {
		return Result{Output: "error: build request: " + err.Error(), IsError: true}
	}
	req.Header.Set("User-Agent", "omnillm-agent/1.0")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return Result{Output: "error: " + err.Error(), IsError: true}
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(io.LimitReader(resp.Body, p.MaxBytes))
	if err != nil {
		return Result{Output: "error: read body: " + err.Error(), IsError: true}
	}

	prefix := fmt.Sprintf("status: %s\n", resp.Status)
	if len(body) == 0 {
		return Result{Output: prefix + "(empty body)"}
	}
	return Result{Output: prefix + string(body)}
}

type calculatorTool struct{}

func Calculator() Tool { return &calculatorTool{} }

func (c *calculatorTool) Name() string { return "calculator" }

func (c *calculatorTool) Description() string {
	return "Evaluates a simple arithmetic expression (+, -, *, /) and returns the result."
}

func (c *calculatorTool) InputSchema() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"expression": map[string]any{"type": "string", "description": "The arithmetic expression to evaluate, e.g. \"17 * 23\" or \"(10 + 5) / 3\"."},
		},
		"required": []string{"expression"},
	}
}

func (c *calculatorTool) Execute(ctx context.Context, call Context, input json.RawMessage) Result {
	var p struct {
		Expression string `json:"expression"`
	}
	if err := json.Unmarshal(input, &p); err != nil {
		return Result{Output: "error: " + err.Error(), IsError: true}
	}
	p.Expression = strings.TrimSpace(p.Expression)
	if p.Expression == "" {
		return Result{Output: "error: expression is required", IsError: true}
	}

	val, err := evalExpr(p.Expression)
	if err != nil {
		return Result{Output: "error: " + err.Error(), IsError: true}
	}
	return Result{Output: strconv.FormatFloat(val, 'f', -1, 64)}
}

type parser struct {
	input []rune
	pos   int
}

func evalExpr(s string) (float64, error) {
	p := &parser{input: []rune(strings.TrimSpace(s))}
	val, err := p.parseExpr()
	if err != nil {
		return 0, err
	}
	for p.pos < len(p.input) && unicode.IsSpace(p.input[p.pos]) {
		p.pos++
	}
	if p.pos != len(p.input) {
		return 0, fmt.Errorf("unexpected character %q at position %d", string(p.input[p.pos:]), p.pos)
	}
	return val, nil
}

func (p *parser) skipSpace() {
	for p.pos < len(p.input) && unicode.IsSpace(p.input[p.pos]) {
		p.pos++
	}
}

func (p *parser) parseExpr() (float64, error) {
	left, err := p.parseTerm()
	if err != nil {
		return 0, err
	}
	for {
		p.skipSpace()
		if p.pos >= len(p.input) {
			break
		}
		op := p.input[p.pos]
		if op != '+' && op != '-' {
			break
		}
		p.pos++
		right, err := p.parseTerm()
		if err != nil {
			return 0, err
		}
		if op == '+' {
			left += right
		} else {
			left -= right
		}
	}
	return left, nil
}

func (p *parser) parseTerm() (float64, error) {
	left, err := p.parseFactor()
	if err != nil {
		return 0, err
	}
	for {
		p.skipSpace()
		if p.pos >= len(p.input) {
			break
		}
		op := p.input[p.pos]
		if op != '*' && op != '/' {
			break
		}
		p.pos++
		right, err := p.parseFactor()
		if err != nil {
			return 0, err
		}
		if op == '*' {
			left *= right
		} else {
			if right == 0 {
				return 0, fmt.Errorf("division by zero")
			}
			left /= right
		}
	}
	return left, nil
}

func (p *parser) parseFactor() (float64, error) {
	p.skipSpace()
	if p.pos >= len(p.input) {
		return 0, fmt.Errorf("unexpected end of expression")
	}

	ch := p.input[p.pos]
	if ch == '-' {
		p.pos++
		val, err := p.parseFactor()
		return -val, err
	}
	if ch == '(' {
		p.pos++
		val, err := p.parseExpr()
		if err != nil {
			return 0, err
		}
		p.skipSpace()
		if p.pos >= len(p.input) || p.input[p.pos] != ')' {
			return 0, fmt.Errorf("missing closing parenthesis")
		}
		p.pos++
		return val, nil
	}
	return p.parseNumber()
}

func (p *parser) parseNumber() (float64, error) {
	p.skipSpace()
	start := p.pos
	dotSeen := false
	for p.pos < len(p.input) {
		ch := p.input[p.pos]
		if ch >= '0' && ch <= '9' {
			p.pos++
			continue
		}
		if ch == '.' && !dotSeen {
			dotSeen = true
			p.pos++
			continue
		}
		break
	}
	if start == p.pos {
		return 0, fmt.Errorf("expected number at position %d", p.pos)
	}
	return strconv.ParseFloat(string(p.input[start:p.pos]), 64)
}
