package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"sort"
	"strings"
	"testing"
	"time"

	"omnillm/internal/tools"
)

func TestBuildRequestRequiresInitialToolForLocalInfoPrompt(t *testing.T) {
	ag := newTestAgent()
	ag.appendUserMessage("show CPU info")

	req := ag.buildRequest(0, "show CPU info")
	if req.ToolChoice != "required" {
		t.Fatalf("tool choice = %#v, want required", req.ToolChoice)
	}

	req = ag.buildRequest(1, "show CPU info")
	if req.ToolChoice != "auto" {
		t.Fatalf("follow-up tool choice = %#v, want auto", req.ToolChoice)
	}
}

func TestBuildRequestUsesAutoForConversationalPrompt(t *testing.T) {
	ag := newTestAgent()
	ag.appendUserMessage("hello")

	req := ag.buildRequest(0, "hello")
	if req.ToolChoice != "auto" {
		t.Fatalf("tool choice = %#v, want auto", req.ToolChoice)
	}
}

func TestBuildRequestRequiresInitialToolForRepoLookupPrompt(t *testing.T) {
	ag := newTestAgent()
	ag.appendUserMessage("find all references to buildSystemPrompt")

	req := ag.buildRequest(0, "find all references to buildSystemPrompt")
	if req.ToolChoice != "required" {
		t.Fatalf("tool choice = %#v, want required", req.ToolChoice)
	}
}

func TestBuildRequestUsesAutoForConceptualArchitecturePrompt(t *testing.T) {
	ag := newTestAgent()
	ag.appendUserMessage("explain the architecture of this repo")

	req := ag.buildRequest(0, "explain the architecture of this repo")
	if req.ToolChoice != "auto" {
		t.Fatalf("tool choice = %#v, want auto", req.ToolChoice)
	}
}

func TestBuildRequestLeavesToolChoiceUnsetWithoutTools(t *testing.T) {
	ag := NewAgent(tools.NewRegistry(), NewBufferMemory(8), 10, nil)
	ag.appendUserMessage("show CPU info")

	req := ag.buildRequest(0, "show CPU info")
	if req.ToolChoice != nil {
		t.Fatalf("tool choice = %#v, want nil", req.ToolChoice)
	}
}

func TestRegistryExecuteToolCallsHonorsPermissionChecker(t *testing.T) {
	registry := tools.NewRegistry()
	registry.Register(tools.Bash())

	results := registry.ExecuteCalls(context.Background(), "session-1", []tools.ToolCall{{
		ID:   "call-1",
		Name: "bash",
		Arguments: map[string]any{
			"command": "echo hello",
		},
	}})
	if len(results) != 1 {
		t.Fatalf("results len = %d", len(results))
	}
	if results[0].IsError {
		t.Fatal("expected bash tool to succeed")
	}
}

func TestChatCompletionsDispatchRoutesAllModelsToMessages(t *testing.T) {
	// All models now route to /v1/messages; OmniLLM handles upstream translation.
	modelsAndResponses := []struct {
		model    string
		response string
	}{
		{
			model:    "claude-opus-4-7",
			response: `{"id":"msg_123","type":"message","role":"assistant","model":"claude-opus-4-7","content":[{"type":"text","text":"ok"}],"stop_reason":"end_turn","usage":{"input_tokens":3,"output_tokens":1}}`,
		},
		{
			model:    "gpt-5.4",
			response: `{"id":"msg_gpt","type":"message","role":"assistant","model":"gpt-5.4","content":[{"type":"text","text":"ok"}],"stop_reason":"end_turn","usage":{"input_tokens":3,"output_tokens":1}}`,
		},
		{
			model:    "gemini-3.1-pro-preview",
			response: `{"id":"msg_gem","type":"message","role":"assistant","model":"gemini-3.1-pro-preview","content":[{"type":"text","text":"ok"}],"stop_reason":"end_turn","usage":{"input_tokens":3,"output_tokens":1}}`,
		},
		{
			model:    "deepseek-v4-flash",
			response: `{"id":"msg_ds","type":"message","role":"assistant","model":"deepseek-v4-flash","content":[{"type":"text","text":"ok"}],"stop_reason":"end_turn","usage":{"input_tokens":3,"output_tokens":1}}`,
		},
		{
			model:    "qwen3.6-flash",
			response: `{"id":"msg_qw","type":"message","role":"assistant","model":"qwen3.6-flash","content":[{"type":"text","text":"ok"}],"stop_reason":"end_turn","usage":{"input_tokens":3,"output_tokens":1}}`,
		},
		{
			model:    "kimi-k2.6",
			response: `{"id":"msg_km","type":"message","role":"assistant","model":"kimi-k2.6","content":[{"type":"text","text":"ok"}],"stop_reason":"end_turn","usage":{"input_tokens":3,"output_tokens":1}}`,
		},
	}

	for _, tc := range modelsAndResponses {
		t.Run(tc.model, func(t *testing.T) {
			var capturedPath string
			var capturedPayload map[string]any
			client := &stubAgentClient{
				postFn: func(path string, body any) ([]byte, error) {
					capturedPath = path
					data, err := json.Marshal(body)
					if err != nil {
						t.Fatalf("marshal body: %v", err)
					}
					if err := json.Unmarshal(data, &capturedPayload); err != nil {
						t.Fatalf("unmarshal body: %v\n%s", err, string(data))
					}
					return []byte(tc.response), nil
				},
			}

			dispatch := NewChatCompletionsDispatch(client, tc.model)
			respCh, err := dispatch(context.Background(), &MessagesRequest{
				MaxTokens:  4096,
				Messages:   []Message{testUserMessage("Hello")},
				Tools:      []tools.ToolDefinition{testToolDefinition("ls", stringPtr("List files"), map[string]any{"type": "object", "properties": map[string]any{"path": map[string]any{"type": "string"}}})},
				ToolChoice: "auto",
			})
			if err != nil {
				t.Fatalf("dispatch returned error: %v", err)
			}
			for range respCh {
			}

			if capturedPath != "/v1/messages" {
				t.Fatalf("[%s] path = %q, want /v1/messages", tc.model, capturedPath)
			}
			if capturedPayload["model"] != tc.model {
				t.Fatalf("[%s] model = %#v", tc.model, capturedPayload["model"])
			}
			if _, ok := capturedPayload["messages"].([]any); !ok {
				t.Fatalf("[%s] messages = %#v", tc.model, capturedPayload["messages"])
			}
			if tools, ok := capturedPayload["tools"].([]any); !ok || len(tools) != 1 {
				t.Fatalf("[%s] tools = %#v", tc.model, capturedPayload["tools"])
			}
		})
	}
}

func TestChatCompletionsDispatchRoutesGPTModelsToMessages(t *testing.T) {
	// GPT models now route to /v1/messages; OmniLLM translates to the Responses API upstream.
	var capturedPath string
	var capturedPayload map[string]any
	client := &stubAgentClient{
		postFn: func(path string, body any) ([]byte, error) {
			capturedPath = path
			data, err := json.Marshal(body)
			if err != nil {
				t.Fatalf("marshal body: %v", err)
			}
			if err := json.Unmarshal(data, &capturedPayload); err != nil {
				t.Fatalf("unmarshal body: %v\n%s", err, string(data))
			}
			return []byte(`{"id":"msg_gpt","type":"message","role":"assistant","model":"gpt-5.4","content":[{"type":"text","text":"ok"}],"stop_reason":"end_turn","usage":{"input_tokens":3,"output_tokens":1}}`), nil
		},
	}

	dispatch := NewChatCompletionsDispatch(client, "gpt-5.4")
	respCh, err := dispatch(context.Background(), &MessagesRequest{
		MaxTokens:  4096,
		Messages:   []Message{testUserMessage("Hello")},
		Tools:      []tools.ToolDefinition{testToolDefinition("ls", stringPtr("List files"), map[string]any{"type": "object", "properties": map[string]any{"path": map[string]any{"type": "string"}}})},
		ToolChoice: "auto",
	})
	if err != nil {
		t.Fatalf("dispatch returned error: %v", err)
	}
	for range respCh {
	}

	if capturedPath != "/v1/messages" {
		t.Fatalf("path = %q, want /v1/messages", capturedPath)
	}
	if capturedPayload["model"] != "gpt-5.4" {
		t.Fatalf("model = %#v", capturedPayload["model"])
	}
	if _, ok := capturedPayload["messages"].([]any); !ok {
		t.Fatalf("messages = %#v", capturedPayload["messages"])
	}
	if tools, ok := capturedPayload["tools"].([]any); !ok || len(tools) != 1 {
		t.Fatalf("tools = %#v", capturedPayload["tools"])
	}
}

func TestChatCompletionsDispatchPostsStandardOpenAIToolPayload(t *testing.T) {
	// Even non-claude models now go to /v1/messages; OmniLLM handles translation.
	var capturedPath string
	var capturedPayload map[string]any
	client := &stubAgentClient{
		postFn: func(path string, body any) ([]byte, error) {
			capturedPath = path
			data, err := json.Marshal(body)
			if err != nil {
				t.Fatalf("marshal body: %v", err)
			}
			if err := json.Unmarshal(data, &capturedPayload); err != nil {
				t.Fatalf("unmarshal body: %v\n%s", err, string(data))
			}
			return []byte(`{"id":"msg_ds","type":"message","role":"assistant","model":"deepseek-v4-flash","content":[{"type":"text","text":"ok"}],"stop_reason":"end_turn","usage":{"input_tokens":3,"output_tokens":1}}`), nil
		},
	}

	dispatch := NewChatCompletionsDispatch(client, "deepseek-v4-flash")
	toolChoice := "auto"
	respCh, err := dispatch(context.Background(), &MessagesRequest{
		MaxTokens:  4096,
		Messages:   []Message{testUserMessage("List files")},
		Tools:      []tools.ToolDefinition{testToolDefinition("ls", stringPtr("List files"), map[string]any{"type": "object", "properties": map[string]any{"path": map[string]any{"type": "string"}}})},
		ToolChoice: toolChoice,
	})
	if err != nil {
		t.Fatalf("dispatch returned error: %v", err)
	}
	for range respCh {
	}

	if capturedPath != "/v1/messages" {
		t.Fatalf("path = %q, want /v1/messages", capturedPath)
	}
	if capturedPayload["model"] != "deepseek-v4-flash" {
		t.Fatalf("model = %#v", capturedPayload["model"])
	}
	tools, ok := capturedPayload["tools"].([]any)
	if !ok || len(tools) != 1 {
		t.Fatalf("tools = %#v", capturedPayload["tools"])
	}
	// Anthropic tool shape: {name, description, input_schema}
	tool, _ := tools[0].(map[string]any)
	if tool["name"] != "ls" {
		t.Fatalf("tool.name = %#v", tool["name"])
	}
	if _, ok := tool["input_schema"].(map[string]any); !ok {
		t.Fatalf("tool.input_schema = %#v", tool["input_schema"])
	}
}

func TestChatCompletionsDispatchDoesNotRetryWithoutTools(t *testing.T) {
	calls := 0
	client := &stubAgentClient{
		postFn: func(path string, body any) ([]byte, error) {
			calls++
			return nil, fmt.Errorf("server error (502): provider_error")
		},
	}

	dispatch := NewChatCompletionsDispatch(client, "deepseek-v4-flash")
	_, err := dispatch(context.Background(), &MessagesRequest{
		MaxTokens:  4096,
		Messages:   []Message{testUserMessage("List files")},
		Tools:      []tools.ToolDefinition{testToolDefinition("ls", nil, map[string]any{"type": "object"})},
		ToolChoice: "auto",
	})
	if err == nil {
		t.Fatal("expected dispatch error")
	}
	if calls != 1 {
		t.Fatalf("calls = %d, want 1", calls)
	}
}

func TestRunTurnPostsDefaultToolsAsOpenAIToolsNotDeprecatedFunctions(t *testing.T) {
	var capturedPayload map[string]any
	client := &stubAgentClient{
		postFn: func(path string, body any) ([]byte, error) {
			if path != "/v1/messages" {
				t.Fatalf("path = %q, want /v1/messages", path)
			}
			data, err := json.Marshal(body)
			if err != nil {
				t.Fatalf("marshal body: %v", err)
			}
			if err := json.Unmarshal(data, &capturedPayload); err != nil {
				t.Fatalf("unmarshal body: %v\n%s", err, string(data))
			}
			return []byte(`{"id":"msg_test","type":"message","role":"assistant","model":"deepseek-v4-flash","content":[{"type":"text","text":"ok"}],"stop_reason":"end_turn","usage":{"input_tokens":5,"output_tokens":1}}`), nil
		},
	}

	result, err := RunTurn(context.Background(), client, "session-1", "deepseek-v4-flash", "google-adk", DefaultAPIShape, "List this directory", nil, nil, nil, 10)
	if err != nil {
		t.Fatalf("RunTurn returned error: %v", err)
	}
	if result == nil || result.Output != "ok" {
		t.Fatalf("unexpected result: %#v", result)
	}

	// All requests now use /v1/messages (Anthropic shape). Deprecated functions/function_call must not appear.
	if _, exists := capturedPayload["functions"]; exists {
		t.Fatalf("deprecated functions field must not be sent: %#v", capturedPayload["functions"])
	}
	if _, exists := capturedPayload["function_call"]; exists {
		t.Fatalf("deprecated function_call field must not be sent: %#v", capturedPayload["function_call"])
	}

	toolsPayload, ok := capturedPayload["tools"].([]any)
	if !ok || len(toolsPayload) != 12 {
		t.Fatalf("tools = %#v", capturedPayload["tools"])
	}

	validName := regexp.MustCompile(`^[A-Za-z0-9_-]{1,64}$`)
	names := make([]string, 0, len(toolsPayload))
	for _, rawTool := range toolsPayload {
		tool, ok := rawTool.(map[string]any)
		if !ok {
			t.Fatalf("tool has unexpected shape: %#v", rawTool)
		}
		// Anthropic /v1/messages tool shape: {name, description, input_schema}
		name, _ := tool["name"].(string)
		if !validName.MatchString(name) {
			t.Fatalf("invalid tool name %q", name)
		}
		if _, ok := tool["description"].(string); !ok {
			t.Fatalf("description missing or invalid for %q: %#v", name, tool)
		}
		inputSchema, ok := tool["input_schema"].(map[string]any)
		if !ok {
			t.Fatalf("input_schema missing or invalid for %q: %#v", name, tool["input_schema"])
		}
		if inputSchema["type"] != "object" {
			t.Fatalf("input_schema.type for %q = %#v", name, inputSchema["type"])
		}
		if _, ok := inputSchema["properties"].(map[string]any); !ok {
			t.Fatalf("input_schema.properties missing or invalid for %q: %#v", name, inputSchema["properties"])
		}
		names = append(names, name)
	}
	sort.Strings(names)
	wantNames := []string{"ask_user_question", "bash", "edit", "get_current_time", "glob", "grep", "load_skill", "ls", "powershell", "read", "todo_write", "write"}
	if fmt.Sprint(names) != fmt.Sprint(wantNames) {
		t.Fatalf("tool names = %#v, want %#v", names, wantNames)
	}

	systemBlocks, ok := capturedPayload["system"].([]any)
	if !ok || len(systemBlocks) == 0 {
		t.Fatalf("system = %#v, want Anthropic text blocks", capturedPayload["system"])
	}
	firstSystemBlock, ok := systemBlocks[0].(map[string]any)
	if !ok || firstSystemBlock["text"] == nil {
		t.Fatalf("system[0] = %#v, want text block", systemBlocks[0])
	}
	systemText, ok := firstSystemBlock["text"].(string)
	if !ok {
		t.Fatalf("system[0].text = %#v, want string", firstSystemBlock["text"])
	}
	if !strings.Contains(systemText, "The current operating system is "+runtime.GOOS) {
		t.Fatalf("system prompt missing runtime OS %q: %q", runtime.GOOS, systemText)
	}
	wantShellTool := "bash"
	if runtime.GOOS == "windows" {
		wantShellTool = "powershell"
	}
	if !strings.Contains(systemText, `Use the "`+wantShellTool+`" tool to execute shell commands`) {
		t.Fatalf("system prompt missing shell tool guidance %q: %q", wantShellTool, systemText)
	}
	if !strings.Contains(systemText, "When presenting output for the OmniCode conversation UI, follow a modern Go TUI-friendly presentation style") {
		t.Fatalf("system prompt missing Go TUI output guidance: %q", systemText)
	}
	if !strings.Contains(systemText, "Prefer Markdown-first output with structured headings") {
		t.Fatalf("system prompt missing markdown-first guidance: %q", systemText)
	}
	if !strings.Contains(systemText, "use clean panel/card-style sections where helpful") {
		t.Fatalf("system prompt missing panel/card guidance: %q", systemText)
	}
	if !strings.Contains(systemText, "present progress as streaming event blocks") {
		t.Fatalf("system prompt missing streaming event guidance: %q", systemText)
	}
	if !strings.Contains(systemText, "format it as a compact, readable markdown table whenever practical") {
		t.Fatalf("system prompt missing markdown table guidance: %q", systemText)
	}
	if !strings.Contains(systemText, "Bubble Tea for the TUI framework, Lip Gloss for styling/layout, Bubbles for components, Glamour for Markdown rendering, and charmbracelet/x/ansi for ANSI helpers") {
		t.Fatalf("system prompt missing Charm stack guidance: %q", systemText)
	}
	if !strings.Contains(systemText, "avoid raw ANSI spaghetti") {
		t.Fatalf("system prompt missing copy/paste friendly output guidance: %q", systemText)
	}
}

type stubAgentClient struct {
	postFn func(path string, body any) ([]byte, error)
}

func (s *stubAgentClient) Post(path string, body any) ([]byte, error) {
	return s.postFn(path, body)
}

func (s *stubAgentClient) PostStream(path string, body any) (*http.Response, error) {
	return nil, fmt.Errorf("not implemented")
}

func stringPtr(value string) *string {
	return &value
}

func TestBuildSystemPromptIncludesGoTUIOutputGuidance(t *testing.T) {
	systemText := buildSystemPrompt("windows")

	checks := map[string]string{
		"runtime OS":          "The current operating system is windows",
		"PowerShell guidance": `Use the "powershell" tool to execute shell commands`,
		"OS-first routing":    "Before invoking any shell-related tool, first confirm the host OS",
		"PowerShell first":    "on `windows`, the PowerShell tool is the FIRST CHOICE",
		"bash first":          "on `macos`/`darwin` or `linux`, the bash/shell tool is the FIRST CHOICE",
		"no-OS-guess":         "Never guess the OS or fall back to the wrong shell tool",
		"Go TUI style":        "When presenting output for the OmniCode conversation UI, follow a modern Go TUI-friendly presentation style",
		"Markdown-first":      "Prefer Markdown-first output with structured headings",
		"panel/card sections": "use clean panel/card-style sections where helpful",
		"streaming events":    "present progress as streaming event blocks",
		"markdown tables":     "format it as a compact, readable markdown table whenever practical",
		"Charm stack":         "Bubble Tea for the TUI framework, Lip Gloss for styling/layout, Bubbles for components, Glamour for Markdown rendering, and charmbracelet/x/ansi for ANSI helpers",
		"copy/paste friendly": "avoid raw ANSI spaghetti",
		"reactive workflows":  "reactive state, streaming views, Markdown rendering, viewport abstractions, and async tool-event blocks",
	}
	for name, want := range checks {
		if !strings.Contains(systemText, want) {
			t.Fatalf("system prompt missing %s guidance %q: %q", name, want, systemText)
		}
	}
}
func TestSelectDispatchAlwaysUsesMessagesProxy(t *testing.T) {
	var capturedPath string
	client := &stubAgentClient{
		postFn: func(path string, body any) ([]byte, error) {
			capturedPath = path
			return []byte(`{"id":"msg_1","type":"message","role":"assistant","model":"claude-opus-4-5","content":[{"type":"text","text":"ok"}],"stop_reason":"end_turn","usage":{"input_tokens":3,"output_tokens":1}}`), nil
		},
	}
	dispatch := selectDispatch(client, "claude-opus-4-5", "google-adk", "responses")
	_, err := dispatch(context.Background(), testMessagesRequest("", testUserMessage("hi")))
	if err != nil {
		t.Fatalf("dispatch error: %v", err)
	}
	if capturedPath != "/v1/messages" {
		t.Fatalf("expected /v1/messages, got %q", capturedPath)
	}
}

func TestStreamEmitsTurnProgress(t *testing.T) {
	ag := NewAgent(tools.NewRegistry(), NewBufferMemory(8), 3, func(ctx context.Context, req *MessagesRequest) (<-chan *MessagesResponse, error) {
		ch := make(chan *MessagesResponse, 1)
		ch <- &MessagesResponse{
			ID:         "msg-progress",
			Model:      req.Model,
			Content:    []ContentBlock{TextBlock("ok")},
			StopReason: StopReasonEndTurn,
		}
		close(ch)
		return ch, nil
	})

	events, err := ag.Stream(context.Background(), "sess-progress", "hello")
	if err != nil {
		t.Fatalf("Stream returned error: %v", err)
	}

	var progress []Event
	for event := range events {
		if event.Type == EventTurnProgress {
			progress = append(progress, event)
		}
	}

	if len(progress) != 1 {
		t.Fatalf("progress events len = %d, want 1 (%#v)", len(progress), progress)
	}
	if progress[0].Turn != 1 || progress[0].MaxTurns != 3 {
		t.Fatalf("progress event = turn %d max %d, want 1/3", progress[0].Turn, progress[0].MaxTurns)
	}
}

func newTestAgent() *Agent {
	registry := tools.NewRegistry()
	registry.Register(tools.Bash())
	memory := NewBufferMemory(8)
	return NewAgent(registry, memory, 10, nil)
}

func TestRunTurnAppendsDailyLogEntries(t *testing.T) {
	dir := t.TempDir()
	t.Setenv(workspaceDirEnv, dir)

	client := &stubAgentClient{
		postFn: func(_ string, _ any) ([]byte, error) {
			return []byte(`{"id":"msg_test","type":"message","role":"assistant","model":"deepseek-v4-flash","content":[{"type":"text","text":"ok"}],"stop_reason":"end_turn","usage":{"input_tokens":5,"output_tokens":1}}`), nil
		},
	}

	_, err := RunTurn(context.Background(), client, "sess-log", "deepseek-v4-flash", "google-adk", DefaultAPIShape, "List files", nil, nil, nil, 10)
	if err != nil {
		t.Fatalf("RunTurn returned error: %v", err)
	}

	logPath := filepath.Join(dir, memoryLogDir, time.Now().Format("2006-01-02")+".md")
	data, readErr := os.ReadFile(logPath)
	if readErr != nil {
		t.Fatalf("reading daily log: %v", readErr)
	}
	content := string(data)
	if !strings.Contains(content, "[sess-log] run_start") {
		t.Fatalf("daily log missing run_start entry: %q", content)
	}
	if !strings.Contains(content, "[sess-log] run_done") {
		t.Fatalf("daily log missing run_done entry: %q", content)
	}
}

func TestRunTurnAppendsDailyLogErrorEntry(t *testing.T) {
	dir := t.TempDir()
	t.Setenv(workspaceDirEnv, dir)

	client := &stubAgentClient{
		postFn: func(_ string, _ any) ([]byte, error) {
			return nil, fmt.Errorf("upstream unavailable")
		},
	}

	_, err := RunTurn(context.Background(), client, "sess-log-err", "deepseek-v4-flash", "google-adk", DefaultAPIShape, "List files", nil, nil, nil, 10)
	if err == nil {
		t.Fatal("expected RunTurn error")
	}

	logPath := filepath.Join(dir, memoryLogDir, time.Now().Format("2006-01-02")+".md")
	data, readErr := os.ReadFile(logPath)
	if readErr != nil {
		t.Fatalf("reading daily log: %v", readErr)
	}
	content := string(data)
	if !strings.Contains(content, "[sess-log-err] run_start") {
		t.Fatalf("daily log missing run_start entry: %q", content)
	}
	if !strings.Contains(content, "[sess-log-err] run_error") {
		t.Fatalf("daily log missing run_error entry: %q", content)
	}
}
