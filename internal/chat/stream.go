package chat

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/charmbracelet/glamour"
)

func showLoading(w io.Writer) func() {
	phrases := []string{"thinking", "processing", "reasoning", "computing", "generating"}
	stop := make(chan struct{})
	go func() {
		i := 0
		for {
			select {
			case <-stop:
				return
			default:
				fmt.Fprintf(w, "\r  %s... ", phrases[i%len(phrases)])
				i++
				time.Sleep(400 * time.Millisecond)
			}
		}
	}()
	return func() {
		close(stop)
		fmt.Fprint(w, "\r"+strings.Repeat(" ", 20)+"\r")
	}
}

func parseSSEStreams(body io.Reader, onEvent func(sseEvent) error) error {
	scanner := bufio.NewScanner(body)
	scanner.Buffer(make([]byte, 0, 64*1024), 256*1024)
	var buf bytes.Buffer
	for scanner.Scan() {
		line := scanner.Bytes()
		if len(line) == 0 {
			if buf.Len() > 0 {
				if err := onEvent(sseEvent{data: buf.Bytes()}); err != nil {
					return err
				}
				buf.Reset()
			}
			continue
		}
		if bytes.HasPrefix(line, []byte("data: ")) {
			buf.Write(bytes.TrimPrefix(line, []byte("data: ")))
		} else if bytes.HasPrefix(line, []byte("data:")) {
			buf.Write(bytes.TrimPrefix(line, []byte("data:")))
		}
	}
	if buf.Len() > 0 {
		if err := onEvent(sseEvent{data: buf.Bytes()}); err != nil {
			return err
		}
	}
	return scanner.Err()
}

func StreamCompletion(c Client, model string, messages []Message, w io.Writer, tty bool) (string, error) {
	reqModel := model
	if reqModel == "" {
		reqModel = "gpt-4"
	}

	completionBody := map[string]any{
		"model":    reqModel,
		"messages": messages,
		"stream":   true,
	}

	resp, err := c.PostStream("/v1/chat/completions", completionBody)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		data, _ := io.ReadAll(resp.Body)
		return "", fmt.Errorf("server error (%d): %s", resp.StatusCode, strings.TrimSpace(string(data)))
	}

	stop := showLoading(w)
	var fullContent strings.Builder
	firstChunk := true

	err = parseSSEStreams(resp.Body, func(ev sseEvent) error {
		trimmed := bytes.TrimSpace(ev.data)
		if len(trimmed) == 0 || string(trimmed) == "[DONE]" {
			return nil
		}

		var chunk map[string]any
		if err := json.Unmarshal(trimmed, &chunk); err != nil {
			return nil
		}

		choices, _ := chunk["choices"].([]any)
		if len(choices) == 0 {
			return nil
		}
		choice, _ := choices[0].(map[string]any)
		delta, _ := choice["delta"].(map[string]any)
		content, _ := delta["content"].(string)
		if content == "" {
			return nil
		}

		if firstChunk {
			stop()
			firstChunk = false
		}

		fullContent.WriteString(content)
		fmt.Fprint(w, content)
		return nil
	})
	if err != nil {
		if !firstChunk {
			fmt.Fprintln(w)
		}
		return fullContent.String(), fmt.Errorf("stream error: %w", err)
	}

	if !firstChunk {
		fmt.Fprint(w, "\n\n")
	} else {
		stop()
	}

	if tty && fullContent.Len() > 0 {
		rendered := RenderMarkdown(fullContent.String())
		if rendered != "" {
			clearLinesUp(w, fullContent.String())
			fmt.Fprint(w, rendered)
		}
	}

	return fullContent.String(), nil
}

func clearLinesUp(w io.Writer, content string) {
	lines := strings.Count(content, "\n") + 2
	for i := 0; i < lines; i++ {
		fmt.Fprint(w, "\033[1A\033[2K")
	}
}

func RenderMarkdown(md string) string {
	r, err := glamour.NewTermRenderer(
		glamour.WithAutoStyle(),
		glamour.WithWordWrap(0),
	)
	if err != nil {
		return ""
	}
	out, err := r.Render(md)
	if err != nil {
		return ""
	}
	return out
}
