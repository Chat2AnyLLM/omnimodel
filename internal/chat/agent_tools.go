package chat

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

	"github.com/Ingenimax/agent-sdk-go/pkg/interfaces"
)

// buildTools returns the set of tools available to the agent when in /mode agent.
func buildTools() []interfaces.Tool {
	return []interfaces.Tool{
		&timeTool{},
		&httpGetTool{},
		&calculatorTool{},
	}
}

// --------------------------------------------------------------------------
// get_current_time
// --------------------------------------------------------------------------

type timeTool struct{}

func (t *timeTool) Name() string        { return "get_current_time" }
func (t *timeTool) Description() string { return "Returns the current date and time in RFC3339 format." }
func (t *timeTool) Parameters() map[string]interfaces.ParameterSpec {
	return map[string]interfaces.ParameterSpec{}
}
func (t *timeTool) Run(ctx context.Context, input string) (string, error) {
	return time.Now().Format(time.RFC3339), nil
}
func (t *timeTool) Execute(ctx context.Context, args string) (string, error) {
	return t.Run(ctx, args)
}

// --------------------------------------------------------------------------
// http_get
// --------------------------------------------------------------------------

type httpGetTool struct{}

func (h *httpGetTool) Name() string        { return "http_get" }
func (h *httpGetTool) Description() string { return "Fetches a URL via HTTP GET and returns the response body (truncated to 4 KB)." }
func (h *httpGetTool) Parameters() map[string]interfaces.ParameterSpec {
	return map[string]interfaces.ParameterSpec{
		"url": {
			Type:        "string",
			Description: "The URL to fetch.",
			Required:    true,
		},
	}
}

func (h *httpGetTool) Run(ctx context.Context, input string) (string, error) {
	// input may be plain URL or JSON {"url":"..."}
	url := strings.TrimSpace(input)
	if strings.HasPrefix(url, "{") {
		var m map[string]string
		if err := json.Unmarshal([]byte(input), &m); err == nil {
			if u, ok := m["url"]; ok {
				url = u
			}
		}
	}
	if url == "" {
		return "", fmt.Errorf("http_get: url is required")
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return "", fmt.Errorf("http_get: build request: %w", err)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("http_get: %w", err)
	}
	defer resp.Body.Close()

	const maxBytes = 4096
	body, err := io.ReadAll(io.LimitReader(resp.Body, maxBytes))
	if err != nil {
		return "", fmt.Errorf("http_get: read body: %w", err)
	}
	return string(body), nil
}

func (h *httpGetTool) Execute(ctx context.Context, args string) (string, error) {
	return h.Run(ctx, args)
}

// --------------------------------------------------------------------------
// calculator
// --------------------------------------------------------------------------

type calculatorTool struct{}

func (c *calculatorTool) Name() string        { return "calculator" }
func (c *calculatorTool) Description() string { return "Evaluates a simple arithmetic expression (+, -, *, /) and returns the result." }
func (c *calculatorTool) Parameters() map[string]interfaces.ParameterSpec {
	return map[string]interfaces.ParameterSpec{
		"expression": {
			Type:        "string",
			Description: "The arithmetic expression to evaluate, e.g. \"17 * 23\" or \"(10 + 5) / 3\".",
			Required:    true,
		},
	}
}

func (c *calculatorTool) Run(ctx context.Context, input string) (string, error) {
	expr := strings.TrimSpace(input)
	if strings.HasPrefix(expr, "{") {
		var m map[string]string
		if err := json.Unmarshal([]byte(input), &m); err == nil {
			if e, ok := m["expression"]; ok {
				expr = e
			}
		}
	}
	if expr == "" {
		return "", fmt.Errorf("calculator: expression is required")
	}

	val, err := evalExpr(expr)
	if err != nil {
		return "", fmt.Errorf("calculator: %w", err)
	}
	// Format without trailing zeros when possible.
	result := strconv.FormatFloat(val, 'f', -1, 64)
	return result, nil
}

func (c *calculatorTool) Execute(ctx context.Context, args string) (string, error) {
	return c.Run(ctx, args)
}

// --------------------------------------------------------------------------
// Recursive descent parser for arithmetic expressions.
// Supports: +, -, *, /, unary minus, parentheses, integers and floats.
// --------------------------------------------------------------------------

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
	// Consume trailing whitespace.
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

// parseExpr handles + and - (lowest precedence).
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

// parseTerm handles * and / (higher precedence).
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

// parseFactor handles unary minus, parentheses, and numbers.
func (p *parser) parseFactor() (float64, error) {
	p.skipSpace()
	if p.pos >= len(p.input) {
		return 0, fmt.Errorf("unexpected end of expression")
	}

	ch := p.input[p.pos]

	// Unary minus.
	if ch == '-' {
		p.pos++
		val, err := p.parseFactor()
		return -val, err
	}

	// Parenthesised sub-expression.
	if ch == '(' {
		p.pos++ // consume '('
		val, err := p.parseExpr()
		if err != nil {
			return 0, err
		}
		p.skipSpace()
		if p.pos >= len(p.input) || p.input[p.pos] != ')' {
			return 0, fmt.Errorf("missing closing parenthesis")
		}
		p.pos++ // consume ')'
		return val, nil
	}

	// Number.
	return p.parseNumber()
}

func (p *parser) parseNumber() (float64, error) {
	start := p.pos
	for p.pos < len(p.input) && (unicode.IsDigit(p.input[p.pos]) || p.input[p.pos] == '.') {
		p.pos++
	}
	if p.pos == start {
		return 0, fmt.Errorf("expected number at position %d, got %q", p.pos, string(p.input[p.pos:]))
	}
	return strconv.ParseFloat(string(p.input[start:p.pos]), 64)
}
