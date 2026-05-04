package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

type webSearchTool struct{}

func WebSearch() Tool { return &webSearchTool{} }

func (t *webSearchTool) Name() string { return "web_search" }

func (t *webSearchTool) Description() string {
	return "Search the web for information. Returns a list of search results with titles, URLs, and snippets."
}

func (t *webSearchTool) InputSchema() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"query":      map[string]any{"type": "string", "description": "The search query."},
			"numResults": map[string]any{"type": "integer", "description": "Number of results to return (default 8, max 20)."},
		},
		"required": []string{"query"},
	}
}

func (t *webSearchTool) Execute(ctx context.Context, call Context, input json.RawMessage) Result {
	var p struct {
		Query      string `json:"query"`
		NumResults int    `json:"numResults"`
	}
	if err := json.Unmarshal(input, &p); err != nil {
		return Result{Output: "error: " + err.Error(), IsError: true}
	}
	p.Query = strings.TrimSpace(p.Query)
	if p.Query == "" {
		return Result{Output: "error: query is required", IsError: true}
	}
	if p.NumResults <= 0 {
		p.NumResults = 8
	}
	if p.NumResults > 20 {
		p.NumResults = 20
	}

	requestCtx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()

	searchURL := fmt.Sprintf("https://html.duckduckgo.com/html/?q=%s", url.QueryEscape(p.Query))
	req, err := http.NewRequestWithContext(requestCtx, http.MethodGet, searchURL, nil)
	if err != nil {
		return Result{Output: "error: " + err.Error(), IsError: true}
	}
	req.Header.Set("User-Agent", "Mozilla/5.0 (compatible; OmniLLM/1.0)")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return Result{Output: "error: " + err.Error(), IsError: true}
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(io.LimitReader(resp.Body, 256*1024))
	if err != nil {
		return Result{Output: "error: read response: " + err.Error(), IsError: true}
	}

	results := parseDuckDuckGoResults(string(body), p.NumResults)
	if len(results) == 0 {
		return Result{Output: "No search results found."}
	}
	return Result{Output: strings.Join(results, "\n\n")}
}

// parseDuckDuckGoResults extracts search result snippets from the DuckDuckGo HTML page.
func parseDuckDuckGoResults(html string, max int) []string {
	var results []string
	lines := strings.Split(html, "\n")
	for i := 0; i < len(lines) && len(results) < max; i++ {
		line := lines[i]
		// DuckDuckGo HTML results have result__a class for links and result__snippet for descriptions
		if strings.Contains(line, `class="result__a"`) {
			// Extract title and URL
			title := extractBetween(line, `>`, `</a>`)
			title = stripTags(title)
			title = strings.TrimSpace(title)
			href := extractBetween(line, `href="`, `"`)
			href = strings.ReplaceAll(href, "&amp;", "&")

			// Look ahead for snippet
			var snippet string
			for j := i + 1; j < len(lines) && j < i+10; j++ {
				if strings.Contains(lines[j], `class="result__snippet"`) {
					snippet = extractBetween(lines[j], `>`, `</a>`)
					snippet = stripTags(snippet)
					snippet = strings.TrimSpace(snippet)
					break
				}
			}

			if title != "" {
				result := title
				if href != "" {
					result += "\n" + href
				}
				if snippet != "" {
					result += "\n" + snippet
				}
				results = append(results, result)
			}
		}
	}
	return results
}

func extractBetween(s, start, end string) string {
	_, after, ok := strings.Cut(s, start)
	if !ok {
		return ""
	}
	before, _, _ := strings.Cut(after, end)
	return before
}

func stripTags(s string) string {
	var out strings.Builder
	inTag := false
	for _, r := range s {
		if r == '<' {
			inTag = true
			continue
		}
		if r == '>' {
			inTag = false
			continue
		}
		if !inTag {
			out.WriteRune(r)
		}
	}
	return out.String()
}
