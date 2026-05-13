package chat

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"

	toolspkg "omnillm/internal/tools"

	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	xansi "github.com/charmbracelet/x/ansi"
)

type testCommandContext struct {
	in  *bytes.Buffer
	out *bytes.Buffer
	err *bytes.Buffer
}

func newTestCommandContext() *testCommandContext {
	return &testCommandContext{
		in:  &bytes.Buffer{},
		out: &bytes.Buffer{},
		err: &bytes.Buffer{},
	}
}

func (c *testCommandContext) InOrStdin() io.Reader   { return c.in }
func (c *testCommandContext) OutOrStdout() io.Writer { return c.out }
func (c *testCommandContext) ErrOrStderr() io.Writer { return c.err }

func TestHandleSlashCommandHelp(t *testing.T) {
	cmd := newTestCommandContext()

	result, err := handleSlashCommand(cmd, nil, &SessionState{ID: "session-1", Mode: "chat"}, "/help")
	if err != nil {
		t.Fatalf("handleSlashCommand returned error: %v", err)
	}
	if !result.handled || result.exit {
		t.Fatalf("unexpected result: %+v", result)
	}
	if !strings.Contains(cmd.out.String(), "/agent <backend>") {
		t.Fatalf("help output missing agent backend guidance:\n%s", cmd.out.String())
	}
	if !strings.Contains(cmd.out.String(), "/models <filter>") {
		t.Fatalf("help output missing filter guidance:\n%s", cmd.out.String())
	}
	if !strings.Contains(cmd.out.String(), "/mode <chat|agent>") {
		t.Fatalf("help output missing mode guidance:\n%s", cmd.out.String())
	}
}

func TestHandleSlashCommandUnknown(t *testing.T) {
	cmd := newTestCommandContext()
	_, err := handleSlashCommand(cmd, nil, &SessionState{ID: "session-1", Mode: "chat"}, "/wat")
	if err == nil || !strings.Contains(err.Error(), "unknown command") {
		t.Fatalf("expected unknown command error, got %v", err)
	}
}

func TestHandleSlashCommandOpenSpecAliasRoutesToAgent(t *testing.T) {
	cmd := newTestCommandContext()
	session := &SessionState{ID: "session-1", Mode: "chat"}
	result, err := handleSlashCommand(cmd, nil, session, "/openspec:explore auth ideas")
	if err != nil {
		t.Fatalf("handleSlashCommand returned error: %v", err)
	}
	if !result.handled {
		t.Fatalf("expected handled result, got %+v", result)
	}
	if result.agentPrompt == "" {
		t.Fatalf("expected agent prompt, got empty result: %+v", result)
	}
	if session.SpecMode != "openspec" || session.Mode != "agent" {
		t.Fatalf("expected openspec agent mode, got mode=%q specMode=%q", session.Mode, session.SpecMode)
	}
	out := cmd.out.String()
	for _, want := range []string{"/openspec:explore", "openspec_explore", "Running agent workflow"} {
		if !strings.Contains(out, want) {
			t.Fatalf("output missing %q:\n%s", want, out)
		}
	}
	for _, want := range []string{"load the spec skill", "openspec_explore", "auth ideas"} {
		if !strings.Contains(result.agentPrompt, want) {
			t.Fatalf("agent prompt missing %q:\n%s", want, result.agentPrompt)
		}
	}
}

func TestHandleSlashCommandSpecKitAliasRoutesToAgent(t *testing.T) {
	cmd := newTestCommandContext()
	session := &SessionState{ID: "session-1", Mode: "chat"}
	result, err := handleSlashCommand(cmd, nil, session, "/speckit.lifecycle specs/001-demo")
	if err != nil {
		t.Fatalf("handleSlashCommand returned error: %v", err)
	}
	if !result.handled {
		t.Fatalf("expected handled result, got %+v", result)
	}
	if result.agentPrompt == "" {
		t.Fatalf("expected agent prompt, got empty result: %+v", result)
	}
	if session.SpecMode != "spec-kit" || session.Mode != "agent" {
		t.Fatalf("expected spec-kit agent mode, got mode=%q specMode=%q", session.Mode, session.SpecMode)
	}
	out := cmd.out.String()
	for _, want := range []string{"/speckit.lifecycle", "speckit_lifecycle_status", "Running agent workflow"} {
		if !strings.Contains(out, want) {
			t.Fatalf("output missing %q:\n%s", want, out)
		}
	}
	for _, want := range []string{"load the spec skill", "speckit_lifecycle_status", "specs/001-demo"} {
		if !strings.Contains(result.agentPrompt, want) {
			t.Fatalf("agent prompt missing %q:\n%s", want, result.agentPrompt)
		}
	}
}

func TestRenderMarkdownTable(t *testing.T) {
	t.Parallel()

	model := newChatTUIModel(nil, "session-1", "model", "chat", "anthropic", "", nil, nil)
	model.mainWidth = 100

	tests := []struct {
		name  string
		input string
		want  []string
	}{
		{
			name: "standalone table",
			input: strings.Join([]string{
				"| Name | Status |",
				"| --- | --- |",
				"| printer-1 | normal |",
			}, "\n"),
			want: []string{"Name", "printer-1", "Status"},
		},
		{
			name: "prose surrounding table",
			input: strings.Join([]string{
				"Here are all the printers available on your system:",
				"",
				"| Name | Status |",
				"| --- | --- |",
				"| printer-1 | normal |",
				"",
				"Summary: 1 printer found.",
			}, "\n"),
			want: []string{"Here are all the printers available on your system:", "printer-1", "Summary: 1 printer found."},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got := model.renderMD(tt.input)
			for _, want := range tt.want {
				if !strings.Contains(got, want) {
					t.Fatalf("renderMD() missing %q in output:\n%s", want, got)
				}
			}
		})
	}
}

func TestRenderAssistantBodyMatchesInputWidth(t *testing.T) {
	m := newChatTUIModel(nil, "session-1", "claude-haiku-4.5", "chat", DefaultAPIShape, "", nil, nil)
	m.mainWidth = 80

	body := m.renderAssistantBody("short", false)
	lines := strings.Split(body, "\n")
	if len(lines) != 1 {
		t.Fatalf("line count = %d, want 1", len(lines))
	}
	if got := xansi.StringWidth(lines[0]); got != m.transcriptBlockMaxWidth() {
		t.Fatalf("assistant body width = %d, want %d", got, m.transcriptBlockMaxWidth())
	}
	input := lipgloss.Width(m.renderTextarea())
	if got := lipgloss.Width(lines[0]); got != input {
		t.Fatalf("assistant body width = %d, want input width %d", got, input)
	}
}

func TestRenderTextareaDoesNotShowTopStatusChips(t *testing.T) {
	m := newChatTUIModel(nil, "session-1", "claude-haiku-4.5", "agent", DefaultAPIShape, "", nil, nil)
	m.mainWidth = 80
	m.textarea.SetValue("hello")

	rendered := m.renderTextarea()
	if strings.Contains(rendered, "AGENT") {
		t.Fatalf("renderTextarea() duplicated mode chip: %q", rendered)
	}
	if strings.Contains(rendered, "MANUAL APPROVAL") {
		t.Fatalf("renderTextarea() still shows manual chip: %q", rendered)
	}
	if strings.Contains(rendered, "AUTOPILOT") {
		t.Fatalf("renderTextarea() still shows autopilot chip: %q", rendered)
	}
}

func TestRenderFooterStatusPrefersContextualState(t *testing.T) {
	m := newChatTUIModel(nil, "session-1", "claude-haiku-4.5", "chat", DefaultAPIShape, "", nil, nil)
	m.mainWidth = 80

	if got := m.renderFooterStatus(); !strings.Contains(got, "Ctrl+O toggle all blocks") {
		t.Fatalf("default footer = %q", got)
	}

	m.streamActive = true
	if got := m.renderFooterStatus(); !strings.Contains(got, "Streaming response") {
		t.Fatalf("stream footer = %q", got)
	}
}

func TestRenderSidebarPrioritizesActionsOverMessageCount(t *testing.T) {
	m := newChatTUIModel(nil, "session-1", "claude-haiku-4.5", "agent", DefaultAPIShape, "omnicode", nil, nil)
	m.sidebarWidth = 30
	m.height = 20
	m.autopilot = true
	m.appendEntry(transcriptAssistant, "hello")

	rendered := m.renderSidebar()
	if !strings.Contains(rendered, "Context") {
		t.Fatalf("renderSidebar() missing Context section: %q", rendered)
	}
	if !strings.Contains(rendered, "Working dir") {
		t.Fatalf("renderSidebar() missing working directory: %q", rendered)
	}
	if !strings.Contains(rendered, "Actions") {
		t.Fatalf("renderSidebar() missing Actions section: %q", rendered)
	}
	if strings.Contains(rendered, "total") {
		t.Fatalf("renderSidebar() still shows message count: %q", rendered)
	}
}

func TestRenderSidebarShowsAgentTurnUsage(t *testing.T) {
	m := newChatTUIModel(nil, "session-1", "claude-haiku-4.5", "agent", DefaultAPIShape, "omnicode", nil, nil)
	m.sidebarWidth = 30
	m.height = 20
	m.maxTurns = 8
	m.streamActive = true
	m.currentAgentTurn = 3
	m.currentAgentMaxTurns = 8

	rendered := m.renderSidebar()
	if !strings.Contains(rendered, "Turns") {
		t.Fatalf("renderSidebar() missing Turns section: %q", rendered)
	}
	if !strings.Contains(rendered, "3 / 8") {
		t.Fatalf("renderSidebar() missing turn usage: %q", rendered)
	}
	if !strings.Contains(rendered, "Turn 3/8") {
		t.Fatalf("renderSidebar() missing active turn status: %q", rendered)
	}
}

func TestHandleSlashCommandModeShow(t *testing.T) {
	cmd := newTestCommandContext()
	session := &SessionState{ID: "session-1", Mode: "chat"}

	result, err := handleSlashCommand(cmd, nil, session, "/mode")
	if err != nil {
		t.Fatalf("handleSlashCommand returned error: %v", err)
	}
	if !result.handled {
		t.Fatalf("expected handled result, got %+v", result)
	}
	if session.Mode != "chat" {
		t.Fatalf("mode = %q, want chat", session.Mode)
	}
	if !strings.Contains(cmd.out.String(), "Current mode: chat") {
		t.Fatalf("unexpected output: %s", cmd.out.String())
	}
}

func TestHandleSlashCommandModeSwitch(t *testing.T) {
	cmd := newTestCommandContext()
	session := &SessionState{ID: "session-1", Mode: "chat"}

	result, err := handleSlashCommand(cmd, nil, session, "/mode agent")
	if err != nil {
		t.Fatalf("handleSlashCommand returned error: %v", err)
	}
	if !result.handled {
		t.Fatalf("expected handled result, got %+v", result)
	}
	if session.Mode != "agent" {
		t.Fatalf("mode = %q, want agent", session.Mode)
	}
	if !strings.Contains(cmd.out.String(), "Agent mode enabled") {
		t.Fatalf("unexpected output: %s", cmd.out.String())
	}
}

func TestHandleSlashCommandAgentShow(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/api/admin/chat/sessions/session-1":
			_ = json.NewEncoder(w).Encode(map[string]any{
				"id":            "session-1",
				"model_id":      "gpt-4",
				"agent_backend": "omnicode",
				"messages":      []any{},
			})
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	cmd := newTestCommandContext()
	client := &testClient{baseURL: server.URL, http: server.Client()}
	session := &SessionState{ID: "session-1", Mode: "agent", AgentBackend: "omnicode"}

	result, err := handleSlashCommand(cmd, client, session, "/agent")
	if err != nil {
		t.Fatalf("handleSlashCommand returned error: %v", err)
	}
	if !result.handled || result.agentBackend != "omnicode" {
		t.Fatalf("unexpected result: %+v", result)
	}
	text := cmd.out.String()
	if !strings.Contains(text, "Agent backend: omnicode") {
		t.Fatalf("unexpected output: %s", text)
	}
}

func TestHandleSlashCommandAPIShapeSwitchOpenAI(t *testing.T) {
	var updatedShape string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodPut && r.URL.Path == "/api/admin/chat/sessions/session-1":
			var body map[string]string
			if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
				t.Fatalf("decode request: %v", err)
			}
			updatedShape = body["api_shape"]
			_ = json.NewEncoder(w).Encode(map[string]any{"success": true})
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	cmd := newTestCommandContext()
	client := &testClient{baseURL: server.URL, http: server.Client()}

	result, err := handleSlashCommand(cmd, client, &SessionState{ID: "session-1", Mode: "agent"}, "/apishape openai")
	if err != nil {
		t.Fatalf("handleSlashCommand returned error: %v", err)
	}
	if !result.handled || result.apiShape != "openai" {
		t.Fatalf("unexpected result: %+v", result)
	}
	if updatedShape != "openai" {
		t.Fatalf("updated shape = %q, want openai", updatedShape)
	}
	if !strings.Contains(cmd.out.String(), "Switched API shape to openai (/v1/chat/completions)") {
		t.Fatalf("unexpected output: %s", cmd.out.String())
	}
}

func TestListModelsSorted(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/admin/status":
			_ = json.NewEncoder(w).Encode(map[string]any{
				"activeProviders": []map[string]any{
					{"id": "provider-b", "name": "Provider B"},
					{"id": "provider-a", "name": "Provider A"},
				},
			})
		case "/api/admin/providers/provider-a/models":
			_ = json.NewEncoder(w).Encode(map[string]any{
				"models": []map[string]any{{"id": "a-model", "name": "Alpha", "enabled": true}},
			})
		case "/api/admin/providers/provider-b/models":
			_ = json.NewEncoder(w).Encode(map[string]any{
				"models": []map[string]any{{"id": "z-model", "name": "Zed", "enabled": true}},
			})
		case "/models":
			_ = json.NewEncoder(w).Encode(map[string]any{"data": []map[string]any{}})
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	client := &testClient{baseURL: server.URL, http: server.Client()}
	models, err := ListModels(client)
	if err != nil {
		t.Fatalf("ListModels returned error: %v", err)
	}
	if len(models) != 2 {
		t.Fatalf("expected 2 models, got %d", len(models))
	}
	if models[0].ID != "a-model" || models[1].ID != "z-model" {
		t.Fatalf("models not sorted by provider/name: %+v", models)
	}
	if models[0].ProviderName != "Provider A" || models[1].ProviderName != "Provider B" {
		t.Fatalf("unexpected provider names: %+v", models)
	}
}

func TestFilterModels(t *testing.T) {
	models := []ModelInfo{
		{ID: "gpt-4", Owner: "provider-a", Name: "GPT 4"},
		{ID: "qwen3", Owner: "provider-b", Name: "Qwen 3"},
		{ID: "claude-sonnet", Owner: "provider-c", Name: "Claude Sonnet"},
	}

	filtered := FilterModels(models, "qwe")
	if len(filtered) != 1 || filtered[0].ID != "qwen3" {
		t.Fatalf("unexpected filtered models: %+v", filtered)
	}
}

func TestHandleSlashCommandModelsFilterOutput(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/admin/status":
			_ = json.NewEncoder(w).Encode(map[string]any{
				"activeProviders": []map[string]any{{"id": "provider-a", "name": "Provider A"}, {"id": "provider-b", "name": "Provider B"}},
			})
		case "/api/admin/providers/provider-a/models":
			_ = json.NewEncoder(w).Encode(map[string]any{"models": []map[string]any{{"id": "gpt-4", "name": "GPT 4", "enabled": true}}})
		case "/api/admin/providers/provider-b/models":
			_ = json.NewEncoder(w).Encode(map[string]any{"models": []map[string]any{{"id": "qwen3", "name": "Qwen 3", "enabled": true}}})
		case "/models":
			_ = json.NewEncoder(w).Encode(map[string]any{"data": []map[string]any{}})
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	cmd := newTestCommandContext()
	client := &testClient{baseURL: server.URL, http: server.Client()}
	session := &SessionState{ID: "session-1", IsTTY: false, Mode: "chat"}

	result, err := handleSlashCommand(cmd, client, session, "/models qwe")
	if err != nil {
		t.Fatalf("handleSlashCommand returned error: %v", err)
	}
	if !result.handled {
		t.Fatalf("expected handled result, got %+v", result)
	}
	text := cmd.out.String()
	if !strings.Contains(text, "provider-b/qwen3") || strings.Contains(text, "provider-a/gpt-4") {
		t.Fatalf("unexpected filtered output:\n%s", text)
	}
}

func TestRunAgentTurnWithCheckerExecutesPermissionedCommand(t *testing.T) {
	var postBodies []map[string]any
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodPost && r.URL.Path == "/api/admin/chat/sessions/session-1/messages":
			_ = json.NewEncoder(w).Encode(map[string]any{"success": true})
		case r.Method == http.MethodGet && r.URL.Path == "/api/admin/chat/sessions/session-1":
			_ = json.NewEncoder(w).Encode(map[string]any{
				"id":            "session-1",
				"model_id":      "gpt-5.4",
				"agent_backend": "omnicode",
				"messages":      []map[string]any{{"role": "user", "content": "show me disk usage"}},
			})
		case r.Method == http.MethodPost && r.URL.Path == "/v1/messages":
			var body map[string]any
			if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
				t.Fatalf("decode request: %v", err)
			}
			postBodies = append(postBodies, body)
			if len(postBodies) == 1 {
				// First turn: respond with a tool_use block
				_ = json.NewEncoder(w).Encode(map[string]any{
					"id":          "msg-1",
					"type":        "message",
					"role":        "assistant",
					"model":       "gpt-5.4",
					"stop_reason": "tool_use",
					"usage":       map[string]any{"input_tokens": 10, "output_tokens": 5},
					"content": []map[string]any{{
						"type":  "tool_use",
						"id":    "call-1",
						"name":  "bash",
						"input": map[string]any{"command": "Write-Output disk-ok"},
					}},
				})
				return
			}
			// Second turn: respond with final text
			_ = json.NewEncoder(w).Encode(map[string]any{
				"id":          "msg-2",
				"type":        "message",
				"role":        "assistant",
				"model":       "gpt-5.4",
				"stop_reason": "end_turn",
				"usage":       map[string]any{"input_tokens": 20, "output_tokens": 3},
				"content":     []map[string]any{{"type": "text", "text": "disk-ok"}},
			})
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	client := &testClient{baseURL: server.URL, http: server.Client()}
	checkerCalls := 0
	content, err := RunAgentTurnWithChecker(context.Background(), client, "session-1", "gpt-5.4", "omnicode", DefaultAPIShape, "show me disk usage", func(ctx context.Context, req toolspkg.PermissionRequest) (bool, error) {
		checkerCalls++
		if req.ToolName != "bash" {
			t.Fatalf("tool name = %q", req.ToolName)
		}
		return true, nil
	}, 10)
	if err != nil {
		t.Fatalf("RunAgentTurnWithChecker returned error: %v", err)
	}
	if content != "disk-ok" {
		t.Fatalf("content = %q", content)
	}
	if checkerCalls != 1 {
		t.Fatalf("checker calls = %d, want 1", checkerCalls)
	}
	if len(postBodies) != 2 {
		t.Fatalf("expected 2 completion calls, got %d", len(postBodies))
	}
}

func TestRunAgentTurnExecutesPermissionedCommand(t *testing.T) {
	var postBodies []map[string]any
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodPost && r.URL.Path == "/api/admin/chat/sessions/session-1/messages":
			_ = json.NewEncoder(w).Encode(map[string]any{"success": true})
		case r.Method == http.MethodGet && r.URL.Path == "/api/admin/chat/sessions/session-1":
			_ = json.NewEncoder(w).Encode(map[string]any{
				"id":            "session-1",
				"model_id":      "gpt-5.4",
				"agent_backend": "omnicode",
				"messages":      []map[string]any{{"role": "user", "content": "show me disk usage"}},
			})
		case r.Method == http.MethodPost && r.URL.Path == "/v1/messages":
			var body map[string]any
			if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
				t.Fatalf("decode request: %v", err)
			}
			postBodies = append(postBodies, body)
			if len(postBodies) == 1 {
				_ = json.NewEncoder(w).Encode(map[string]any{
					"id":          "msg-1",
					"type":        "message",
					"role":        "assistant",
					"model":       "gpt-5.4",
					"stop_reason": "tool_use",
					"usage":       map[string]any{"input_tokens": 10, "output_tokens": 5},
					"content": []map[string]any{{
						"type":  "tool_use",
						"id":    "call-1",
						"name":  "bash",
						"input": map[string]any{"command": "Write-Output disk-ok"},
					}},
				})
				return
			}
			_ = json.NewEncoder(w).Encode(map[string]any{
				"id":          "msg-2",
				"type":        "message",
				"role":        "assistant",
				"model":       "gpt-5.4",
				"stop_reason": "end_turn",
				"usage":       map[string]any{"input_tokens": 20, "output_tokens": 3},
				"content":     []map[string]any{{"type": "text", "text": "disk-ok"}},
			})
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	cmd := newTestCommandContext()
	cmd.in.WriteString("y\n")
	client := &testClient{baseURL: server.URL, http: server.Client()}

	content, err := RunAgentTurn(client, "session-1", "gpt-5.4", "omnicode", DefaultAPIShape, "show me disk usage", cmd)
	if err != nil {
		t.Fatalf("RunAgentTurn returned error: %v", err)
	}
	if content != "disk-ok" {
		t.Fatalf("content = %q", content)
	}
	if len(postBodies) != 2 {
		t.Fatalf("expected 2 completion calls, got %d", len(postBodies))
	}
	if !strings.Contains(cmd.err.String(), "Allow tool execution?") {
		t.Fatalf("expected permission prompt, got %q", cmd.err.String())
	}
}

func TestTUIAgentTurnProgressMsgUpdatesCounter(t *testing.T) {
	m := newChatTUIModel(nil, "session-1", "claude-haiku-4.5", "agent", DefaultAPIShape, "omnicode", nil, nil)

	model, _ := m.Update(agentTurnProgressMsg{turn: 2, maxTurns: 5})
	updated := model.(chatTUIModel)
	if updated.currentAgentTurn != 2 || updated.currentAgentMaxTurns != 5 {
		t.Fatalf("agent turn state = %d/%d, want 2/5", updated.currentAgentTurn, updated.currentAgentMaxTurns)
	}
}

func TestTUIAgentDoneMsgShowsVisibleCompletionWhenContentEmpty(t *testing.T) {
	m := newChatTUIModel(nil, "session-1", "claude-haiku-4.5", "agent", DefaultAPIShape, "omnicode", nil, nil)
	m.ready = true
	m.mainWidth = 80
	m.viewport = viewport.New(80, 20)
	m.textarea.Focus()
	m.streamActive = true
	m.spinning = true
	m.agentTurnCancel = func() {}

	toModel := func(model tea.Model) *chatTUIModel {
		switch v := model.(type) {
		case chatTUIModel:
			copy := v
			return &copy
		case *chatTUIModel:
			return v
		default:
			t.Fatalf("unexpected model type %T", model)
			return nil
		}
	}

	model, _ := m.Update(agentDoneMsg{})
	updated := toModel(model)
	if updated.streamActive {
		t.Fatal("expected streamActive to be false")
	}
	if updated.spinning {
		t.Fatal("expected spinning to be false")
	}
	if len(updated.entries) == 0 {
		t.Fatal("expected a visible transcript entry")
	}
	last := updated.entries[len(updated.entries)-1]
	if last.kind != transcriptInfo {
		t.Fatalf("last entry kind = %v, want %v", last.kind, transcriptInfo)
	}
	if last.content != "(agent completed with no text response)" {
		t.Fatalf("last entry content = %q", last.content)
	}
}

type stubClipboard struct {
	text     string
	readText string
	err      error
}

func (s *stubClipboard) ReadAll() (string, error) {
	return s.readText, s.err
}

func (s *stubClipboard) WriteAll(text string) error {
	s.text = text
	return s.err
}

func TestTUIMiddleMouseDragScrollsViewport(t *testing.T) {
	m := newChatTUIModel(nil, "session-1", "claude-haiku-4.5", "chat", DefaultAPIShape, "", nil, nil)
	m.ready = true
	m.mainWidth = 80
	m.autoFollow = true
	m.viewport = viewport.New(80, 4)
	m.viewport.SetContent(strings.Join([]string{"0", "1", "2", "3", "4", "5", "6", "7", "8", "9"}, "\n"))
	m.viewport.SetYOffset(4)

	model, _ := m.Update(tea.MouseMsg{X: 3, Y: 3, Action: tea.MouseActionPress, Button: tea.MouseButtonMiddle})
	updated := model.(chatTUIModel)
	if !updated.middleDragging {
		t.Fatal("expected middle dragging to start")
	}
	if updated.autoFollow {
		t.Fatal("expected autoFollow to be disabled on middle drag start")
	}

	model, _ = updated.Update(tea.MouseMsg{X: 3, Y: 1, Action: tea.MouseActionMotion, Button: tea.MouseButtonMiddle})
	updated = model.(chatTUIModel)
	if updated.viewport.YOffset >= 4 {
		t.Fatalf("expected viewport to scroll up, offset=%d", updated.viewport.YOffset)
	}

	model, _ = updated.Update(tea.MouseMsg{X: 3, Y: 1, Action: tea.MouseActionRelease, Button: tea.MouseButtonNone})
	updated = model.(chatTUIModel)
	if updated.middleDragging {
		t.Fatal("expected middle dragging to stop on release")
	}
}

func TestTUILeftMouseCopiesVisibleTranscriptTextOnRelease(t *testing.T) {
	m := newChatTUIModel(nil, "session-1", "claude-haiku-4.5", "chat", DefaultAPIShape, "", nil, nil)
	m.ready = true
	m.mainWidth = 80
	m.viewport = viewport.New(80, 6)
	m.appendEntry(transcriptAssistant, "Hello world")
	m.syncViewport()
	clip := &stubClipboard{}
	m.clipboard = clip
	entryCount := len(m.entries)

	model, _ := m.Update(tea.MouseMsg{X: 0, Y: 2, Action: tea.MouseActionPress, Button: tea.MouseButtonLeft})
	updated := model.(chatTUIModel)
	model, _ = updated.Update(tea.MouseMsg{X: 4, Y: 2, Action: tea.MouseActionMotion, Button: tea.MouseButtonLeft})
	updated = model.(chatTUIModel)
	model, _ = updated.Update(tea.MouseMsg{X: 4, Y: 2, Action: tea.MouseActionRelease, Button: tea.MouseButtonNone})
	updated = model.(chatTUIModel)

	if clip.text != "Assis" {
		t.Fatalf("clipboard text = %q, want %q", clip.text, "Assis")
	}
	if !updated.selection.active {
		t.Fatal("expected selection to remain active after auto-copy")
	}
	if got := updated.selectedTranscriptText(); got != "Assis" {
		t.Fatalf("selectedTranscriptText() = %q, want %q", got, "Assis")
	}
	if len(updated.entries) != entryCount {
		t.Fatalf("entry count = %d, want %d", len(updated.entries), entryCount)
	}
}

func TestTUILeftMouseClickToolResultHintExpands(t *testing.T) {
	m := newChatTUIModel(nil, "session-1", "claude-haiku-4.5", "chat", DefaultAPIShape, "", nil, nil)
	m.ready = true
	m.mainWidth = 80
	m.viewport = viewport.New(80, 10)
	m.entries = append(m.entries, transcriptEntry{
		id:       1,
		kind:     transcriptToolResult,
		toolName: "Read",
		content:  strings.Join([]string{"0", "1", "2", "3", "4", "5", "6", "7", "8", "9", "10", "11", "12"}, "\n"),
	})
	m.syncViewport()
	if len(m.transcriptLayout) != 1 {
		t.Fatalf("transcript layout entries = %d, want 1", len(m.transcriptLayout))
	}
	_, viewportTop, _, _ := m.viewportBounds()
	hintY := viewportTop + m.transcriptLayout[0].endLine - m.viewport.YOffset
	hintY -= tuiToolResultHitRowOffset

	model, _ := m.Update(tea.MouseMsg{X: 3, Y: hintY, Action: tea.MouseActionPress, Button: tea.MouseButtonLeft})
	updated := model.(chatTUIModel)
	model, _ = updated.Update(tea.MouseMsg{X: 3, Y: hintY, Action: tea.MouseActionRelease, Button: tea.MouseButtonNone})
	updated = model.(chatTUIModel)

	if !updated.expandedEntries[1] {
		t.Fatal("clicking tool result hint should expand it")
	}
}

func TestTUILeftMouseClickExpandsToolResultRegardlessOfHover(t *testing.T) {
	m := newChatTUIModel(nil, "session-1", "claude-haiku-4.5", "chat", DefaultAPIShape, "", nil, nil)
	m.ready = true
	m.mainWidth = 80
	m.viewport = viewport.New(80, 20)
	m.entries = append(m.entries,
		transcriptEntry{
			id:      1,
			kind:    transcriptAssistant,
			content: strings.Join([]string{"assistant", "0", "1", "2", "3", "4", "5", "6", "7", "8", "9", "10", "11", "12", "13", "14", "15", "16", "17", "18", "19", "20", "21", "22", "23", "24", "25", "26", "27", "28", "29", "30"}, "\n"),
		},
		transcriptEntry{
			id:       2,
			kind:     transcriptToolResult,
			toolName: "Read",
			content:  strings.Join([]string{"0", "1", "2", "3", "4", "5", "6", "7", "8", "9", "10", "11", "12"}, "\n"),
		},
	)
	m.syncViewport()
	if len(m.transcriptLayout) != 2 {
		t.Fatalf("transcript layout entries = %d, want 2", len(m.transcriptLayout))
	}
	m.hoveredEntry = 0
	_, viewportTop, _, _ := m.viewportBounds()
	toolY := viewportTop + m.transcriptLayout[1].clickableStartLine - m.viewport.YOffset

	model, _ := m.Update(tea.MouseMsg{X: 3, Y: toolY, Action: tea.MouseActionPress, Button: tea.MouseButtonLeft})
	updated := model.(chatTUIModel)
	model, _ = updated.Update(tea.MouseMsg{X: 3, Y: toolY, Action: tea.MouseActionRelease, Button: tea.MouseButtonNone})
	updated = model.(chatTUIModel)

	if updated.expandedEntries[1] {
		t.Fatal("did not expect assistant entry to expand")
	}
	if !updated.expandedEntries[2] {
		t.Fatal("clicking tool result should expand it (entry under cursor wins, not stale hover)")
	}
}

func TestTUILeftMouseClickExpandsToolResultBody(t *testing.T) {
	m := newChatTUIModel(nil, "session-1", "claude-haiku-4.5", "chat", DefaultAPIShape, "", nil, nil)
	m.ready = true
	m.mainWidth = 80
	m.viewport = viewport.New(80, 20)
	m.entries = append(m.entries, transcriptEntry{
		id:       1,
		kind:     transcriptToolResult,
		toolName: "Read",
		content:  strings.Join([]string{"0", "1", "2", "3", "4", "5", "6", "7", "8", "9", "10", "11", "12"}, "\n"),
	})
	m.syncViewport()
	if len(m.transcriptLayout) != 1 {
		t.Fatalf("transcript layout entries = %d, want 1", len(m.transcriptLayout))
	}
	_, viewportTop, _, _ := m.viewportBounds()
	bodyY := viewportTop + m.transcriptLayout[0].startLine + 1 - m.viewport.YOffset

	model, _ := m.Update(tea.MouseMsg{X: 3, Y: bodyY, Action: tea.MouseActionPress, Button: tea.MouseButtonLeft})
	updated := model.(chatTUIModel)
	model, _ = updated.Update(tea.MouseMsg{X: 3, Y: bodyY, Action: tea.MouseActionRelease, Button: tea.MouseButtonNone})
	updated = model.(chatTUIModel)

	if !updated.expandedEntries[1] {
		t.Fatal("clicking tool result body should expand it")
	}
}

func TestTUILeftMouseClickToolResultLabelExpands(t *testing.T) {
	m := newChatTUIModel(nil, "session-1", "claude-haiku-4.5", "chat", DefaultAPIShape, "", nil, nil)
	m.ready = true
	m.mainWidth = 80
	m.viewport = viewport.New(80, 20)
	m.entries = append(m.entries, transcriptEntry{
		id:       1,
		kind:     transcriptToolResult,
		toolName: "Read",
		content:  strings.Join([]string{"0", "1", "2", "3", "4", "5", "6", "7", "8", "9", "10", "11", "12"}, "\n"),
	})
	m.syncViewport()
	_, viewportTop, _, _ := m.viewportBounds()
	labelY := viewportTop + m.transcriptLayout[0].startLine - m.viewport.YOffset

	model, _ := m.Update(tea.MouseMsg{X: 3, Y: labelY, Action: tea.MouseActionPress, Button: tea.MouseButtonLeft})
	updated := model.(chatTUIModel)
	model, _ = updated.Update(tea.MouseMsg{X: 3, Y: labelY, Action: tea.MouseActionRelease, Button: tea.MouseButtonNone})
	updated = model.(chatTUIModel)

	if !updated.expandedEntries[1] {
		t.Fatal("tool result label click should expand the result")
	}
}

func TestTUILeftMouseClickBetweenToolResultsDoesNotExpand(t *testing.T) {
	m := newChatTUIModel(nil, "session-1", "claude-haiku-4.5", "chat", DefaultAPIShape, "", nil, nil)
	m.ready = true
	m.mainWidth = 80
	m.viewport = viewport.New(80, 30)
	content := strings.Join([]string{"0", "1", "2", "3", "4", "5", "6", "7", "8", "9", "10", "11", "12"}, "\n")
	m.entries = append(m.entries,
		transcriptEntry{id: 1, kind: transcriptToolResult, toolName: "first", content: content},
		transcriptEntry{id: 2, kind: transcriptToolResult, toolName: "second", content: content},
	)
	m.syncViewport()
	_, viewportTop, _, _ := m.viewportBounds()
	betweenY := viewportTop + m.transcriptLayout[0].endLine + 1 - m.viewport.YOffset

	model, _ := m.Update(tea.MouseMsg{X: 3, Y: betweenY, Action: tea.MouseActionPress, Button: tea.MouseButtonLeft})
	updated := model.(chatTUIModel)
	model, _ = updated.Update(tea.MouseMsg{X: 3, Y: betweenY, Action: tea.MouseActionRelease, Button: tea.MouseButtonNone})
	updated = model.(chatTUIModel)

	if updated.expandedEntries[1] || updated.expandedEntries[2] {
		t.Fatal("clicking between tool results should not expand either result")
	}
}

func TestTUIAssistantResponseDoesNotCollapseLongMessages(t *testing.T) {
	m := newChatTUIModel(nil, "session-1", "claude-haiku-4.5", "chat", DefaultAPIShape, "", nil, nil)
	m.ready = true
	m.mainWidth = 80
	m.viewport = viewport.New(80, 50)
	lines := make([]string, 35)
	for i := range lines {
		lines[i] = "assistant line"
	}
	m.entries = append(m.entries, transcriptEntry{
		id:      1,
		kind:    transcriptAssistant,
		content: strings.Join(lines, "\n"),
	})
	m.syncViewport()

	rendered := m.renderTranscript()
	if strings.Contains(rendered, "lines hidden") {
		t.Fatalf("assistant response should not render an expansion hint:\n%s", rendered)
	}
	if strings.Count(xansi.Strip(rendered), "assistant line") != 35 {
		t.Fatalf("assistant response should render all lines, got:\n%s", rendered)
	}
}

func TestTUIAssistantResponseClickDoesNotToggleExpansion(t *testing.T) {
	m := newChatTUIModel(nil, "session-1", "claude-haiku-4.5", "chat", DefaultAPIShape, "", nil, nil)
	m.ready = true
	m.mainWidth = 80
	m.viewport = viewport.New(80, 50)
	m.entries = append(m.entries, transcriptEntry{
		id:      1,
		kind:    transcriptAssistant,
		content: strings.Repeat("assistant response\n", 35),
	})
	m.syncViewport()
	_, viewportTop, _, _ := m.viewportBounds()
	bodyY := viewportTop + m.transcriptLayout[0].startLine + 1 - m.viewport.YOffset

	model, _ := m.Update(tea.MouseMsg{X: 3, Y: bodyY, Action: tea.MouseActionPress, Button: tea.MouseButtonLeft})
	updated := model.(chatTUIModel)
	model, _ = updated.Update(tea.MouseMsg{X: 3, Y: bodyY, Action: tea.MouseActionRelease, Button: tea.MouseButtonNone})
	updated = model.(chatTUIModel)

	if updated.expandedEntries[1] {
		t.Fatal("assistant response click should not toggle expansion")
	}
}

func TestTUICtrlODoesNotToggleAssistantResponse(t *testing.T) {
	// Ctrl+O should only toggle tool results, not assistant responses
	m := newChatTUIModel(nil, "session-1", "claude-haiku-4.5", "chat", DefaultAPIShape, "", nil, nil)
	m.ready = true
	m.mainWidth = 80
	m.viewport = viewport.New(80, 50)
	m.entries = append(m.entries, transcriptEntry{
		id:      1,
		kind:    transcriptAssistant,
		content: strings.Repeat("assistant response\n", 35),
	})
	m.syncViewport()
	m.textarea.Reset()

	model, _ := m.Update(tea.KeyMsg{Type: tea.KeyCtrlO})
	updated := model.(chatTUIModel)
	if updated.expandedEntries[1] {
		t.Fatal("Ctrl+O should not toggle assistant response expansion")
	}
}

func TestTUILeftMouseClickDoesNotToggleToolResultAfterEntryIndexShift(t *testing.T) {
	m := newChatTUIModel(nil, "session-1", "claude-haiku-4.5", "chat", DefaultAPIShape, "", nil, nil)
	m.ready = true
	m.mainWidth = 80
	m.viewport = viewport.New(80, 30)
	m.entries = append(m.entries,
		transcriptEntry{
			id:       1,
			kind:     transcriptToolResult,
			toolName: "first",
			content:  strings.Join([]string{"first", "0", "1", "2", "3", "4", "5", "6", "7", "8", "9", "10", "11", "12"}, "\n"),
		},
		transcriptEntry{
			id:       2,
			kind:     transcriptToolResult,
			toolName: "second",
			content:  strings.Join([]string{"second", "0", "1", "2", "3", "4", "5", "6", "7", "8", "9", "10", "11", "12"}, "\n"),
		},
	)
	m.syncViewport()
	_, viewportTop, _, _ := m.viewportBounds()
	secondY := viewportTop + m.transcriptLayout[1].clickableStartLine - m.viewport.YOffset
	secondY -= tuiToolResultHitRowOffset

	model, _ := m.Update(tea.MouseMsg{X: 3, Y: secondY, Action: tea.MouseActionPress, Button: tea.MouseButtonLeft})
	updated := model.(chatTUIModel)
	updated.entries = append([]transcriptEntry{{id: 3, kind: transcriptInfo, content: "inserted"}}, updated.entries...)
	updated.syncViewport()
	model, _ = updated.Update(tea.MouseMsg{X: 3, Y: secondY, Action: tea.MouseActionRelease, Button: tea.MouseButtonNone})
	updated = model.(chatTUIModel)

	if updated.expandedEntries[1] {
		t.Fatal("first tool result expanded after clicking second tool result")
	}
	if updated.expandedEntries[2] {
		t.Fatal("clicking second tool result should not expand after entry index shifted")
	}
}

func TestTUICtrlOTogglesAllToolResults(t *testing.T) {
	m := newChatTUIModel(nil, "session-1", "claude-haiku-4.5", "chat", DefaultAPIShape, "", nil, nil)
	m.ready = true
	m.mainWidth = 80
	m.viewport = viewport.New(80, 50)
	m.entries = append(m.entries,
		transcriptEntry{
			id:       1,
			kind:     transcriptToolResult,
			toolName: "Read",
			content:  strings.Join([]string{"0", "1", "2", "3", "4", "5", "6", "7", "8", "9", "10", "11", "12"}, "\n"),
		},
		transcriptEntry{
			id:       2,
			kind:     transcriptToolResult,
			toolName: "Write",
			content:  strings.Join([]string{"0", "1", "2", "3", "4", "5", "6", "7", "8", "9", "10", "11", "12"}, "\n"),
		},
		transcriptEntry{
			id:       3,
			kind:     transcriptToolResult,
			toolName: "Short",
			content:  "short output",
		},
	)
	m.syncViewport()
	m.textarea.Reset()

	// Ctrl+O expands all tool results when any are collapsed
	model, _ := m.Update(tea.KeyMsg{Type: tea.KeyCtrlO})
	updated := model.(chatTUIModel)
	if !updated.expandedEntries[1] || !updated.expandedEntries[2] || !updated.expandedEntries[3] {
		t.Fatal("Ctrl+O should expand all tool results when any are collapsed")
	}

	// Ctrl+O collapses all tool results when all are expanded
	model, _ = updated.Update(tea.KeyMsg{Type: tea.KeyCtrlO})
	updated = model.(chatTUIModel)
	if updated.expandedEntries[1] || updated.expandedEntries[2] || updated.expandedEntries[3] {
		t.Fatal("Ctrl+O should collapse all tool results when all are expanded")
	}

	// Ctrl+O does not work when textarea has input
	updated.textarea.InsertString("text")
	model, _ = updated.Update(tea.KeyMsg{Type: tea.KeyCtrlO})
	updated = model.(chatTUIModel)
	if updated.expandedEntries[1] || updated.expandedEntries[2] || updated.expandedEntries[3] {
		t.Fatal("Ctrl+O should not toggle when textarea has input")
	}
}

func TestFormatToolCallProgressShowsPayloadInline(t *testing.T) {
	got := formatToolCallProgress("powershell", "Get-NetIPAddress -AddressFamily IPv4")
	want := "妫ｅ啯鏆?Calling tool `powershell`: Get-NetIPAddress -AddressFamily IPv4"
	if got != want {
		t.Fatalf("formatToolCallProgress() = %q, want %q", got, want)
	}
}

func TestTUIAgentToolCallProgressShowsPayloadInline(t *testing.T) {
	m := newChatTUIModel(nil, "session-1", "claude-haiku-4.5", "chat", DefaultAPIShape, "", nil, nil)
	m.ready = true
	m.mainWidth = 120
	m.viewport = viewport.New(120, 10)

	model, _ := m.Update(agentProgressMsg{text: "妫ｅ啯鏆?Calling tool `powershell`: Get-NetIPAddress -AddressFamily IPv4"})
	updated := model.(chatTUIModel)
	rendered := updated.renderTranscript()

	if !strings.Contains(rendered, "Calling tool `powershell`: Get-NetIPAddress -AddressFamily IPv4") {
		t.Fatalf("rendered transcript missing inline payload:\n%s", rendered)
	}
}

func TestTUISelectionHighlightStripsNestedANSICodes(t *testing.T) {
	line := "prefix \x1b[31mred\x1b[0m suffix"
	highlighted := highlightVisibleRange(line, 7, 10)

	if strings.Contains(highlighted, "\x1b[31mred\x1b[0m") {
		t.Fatalf("highlighted selection retained nested ANSI style: %q", highlighted)
	}
	if !strings.Contains(highlighted, "red") {
		t.Fatalf("highlighted selection lost text: %q", highlighted)
	}
}

func TestTUINormalEnterSubmitsAfterDeferredTick(t *testing.T) {
	m := newChatTUIModel(nil, "session-1", "claude-haiku-4.5", "chat", DefaultAPIShape, "", nil, nil)
	m.ready = true
	m.mainWidth = 80
	m.viewport = viewport.New(80, 6)
	m.textarea.Focus()
	m.textarea.SetValue("hello")

	model, _ := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	updated := model.(chatTUIModel)

	if len(updated.entries) != 0 {
		t.Fatalf("entry count before deferred submit = %d, want 0", len(updated.entries))
	}
	if !updated.pendingSubmitNewline {
		t.Fatal("expected Enter to start deferred submit")
	}

	model, _ = updated.Update(submitInputMsg(updated.submitSeq))
	updated = model.(chatTUIModel)
	if len(updated.entries) != 1 {
		t.Fatalf("entry count after deferred submit = %d, want 1", len(updated.entries))
	}
	if got := updated.entries[0].content; got != "hello" {
		t.Fatalf("submitted content = %q, want hello", got)
	}
	if !updated.streamActive {
		t.Fatal("expected deferred submit to start streaming")
	}
}

func TestTUICtrlJAddsNewlineWithoutSubmitting(t *testing.T) {
	m := newChatTUIModel(nil, "session-1", "claude-haiku-4.5", "chat", DefaultAPIShape, "", nil, nil)
	m.ready = true
	m.mainWidth = 80
	m.viewport = viewport.New(80, 6)
	m.textarea.Focus()
	m.textarea.SetValue("first line")

	model, _ := m.Update(tea.KeyMsg{Type: tea.KeyCtrlJ})
	updated := model.(chatTUIModel)
	updated.textarea.SetValue(updated.textarea.Value() + "second line")

	if len(updated.entries) != 0 {
		t.Fatalf("entry count before submit = %d, want 0", len(updated.entries))
	}
	if got := updated.textarea.Value(); got != "first line\nsecond line" {
		t.Fatalf("textarea after newline = %q, want %q", got, "first line\nsecond line")
	}

	model, _ = updated.Update(tea.KeyMsg{Type: tea.KeyEnter})
	updated = model.(chatTUIModel)

	if len(updated.entries) != 0 {
		t.Fatalf("entry count before deferred submit = %d, want 0", len(updated.entries))
	}
	if !updated.pendingSubmitNewline {
		t.Fatal("expected Enter to start deferred submit")
	}

	model, _ = updated.Update(submitInputMsg(updated.submitSeq))
	updated = model.(chatTUIModel)
	if len(updated.entries) != 1 {
		t.Fatalf("entry count after deferred submit = %d, want 1", len(updated.entries))
	}
	if got := updated.entries[0].content; got != "first line\nsecond line" {
		t.Fatalf("submitted content = %q, want %q", got, "first line\nsecond line")
	}
	if got := updated.textarea.Value(); got != "" {
		t.Fatalf("textarea after submit = %q, want empty", got)
	}
	if !updated.streamActive {
		t.Fatal("expected deferred submit to start streaming")
	}
}

func TestTUIPastedEnterDoesNotSubmit(t *testing.T) {
	m := newChatTUIModel(nil, "session-1", "claude-haiku-4.5", "chat", DefaultAPIShape, "", nil, nil)
	m.ready = true
	m.mainWidth = 80
	m.viewport = viewport.New(80, 6)
	m.textarea.Focus()
	m.textarea.SetValue("first line")

	model, _ := m.Update(tea.KeyMsg{Type: tea.KeyEnter, Paste: true})
	updated := model.(chatTUIModel)

	if len(updated.entries) != 0 {
		t.Fatalf("entry count after pasted enter = %d, want 0", len(updated.entries))
	}
	if got := updated.textarea.Value(); got != "first line\n" {
		t.Fatalf("textarea after pasted enter = %q, want %q", got, "first line\n")
	}
	if updated.streamActive {
		t.Fatal("expected pasted enter not to submit")
	}
}

func TestTUIRepeatedEnterOnlyUsesLatestDeferredSubmit(t *testing.T) {
	m := newChatTUIModel(nil, "session-1", "claude-haiku-4.5", "chat", DefaultAPIShape, "", nil, nil)
	m.ready = true
	m.mainWidth = 80
	m.viewport = viewport.New(80, 6)
	m.textarea.Focus()
	m.textarea.SetValue("first line")

	model, _ := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	updated := model.(chatTUIModel)
	seq1 := updated.submitSeq
	if len(updated.entries) != 0 {
		t.Fatalf("entry count after first enter = %d, want 0", len(updated.entries))
	}
	if !updated.pendingSubmitNewline {
		t.Fatal("expected first Enter to start deferred submit")
	}

	model, _ = updated.Update(tea.KeyMsg{Type: tea.KeyEnter})
	updated = model.(chatTUIModel)
	seq2 := updated.submitSeq
	if seq2 == seq1 {
		t.Fatal("expected second Enter to advance submit sequence")
	}

	model, _ = updated.Update(submitInputMsg(seq1))
	updated = model.(chatTUIModel)
	if len(updated.entries) != 0 {
		t.Fatalf("stale deferred submit fired: entry count = %d, want 0", len(updated.entries))
	}

	model, _ = updated.Update(submitInputMsg(seq2))
	updated = model.(chatTUIModel)
	if len(updated.entries) != 1 {
		t.Fatalf("entry count after latest deferred submit = %d, want 1", len(updated.entries))
	}
}

func TestTUICtrlVPastesMultilineClipboardWithoutSubmitting(t *testing.T) {
	m := newChatTUIModel(nil, "session-1", "claude-haiku-4.5", "chat", DefaultAPIShape, "", nil, nil)
	m.ready = true
	m.mainWidth = 80
	m.viewport = viewport.New(80, 6)
	m.textarea.Focus()
	m.textarea.SetValue("prefix: ")
	m.clipboard = &stubClipboard{readText: "IPv4 Address. . . . . . . . . . . : 10.131.185.246\nSubnet Mask . . . . . . . . . . . : 255.255.255.0\nDefault Gateway . . . . . . . . . : 10.131.185.1"}
	entryCount := len(m.entries)

	model, _ := m.Update(tea.KeyMsg{Type: tea.KeyCtrlV})
	updated := model.(chatTUIModel)

	want := "prefix: IPv4 Address. . . . . . . . . . . : 10.131.185.246\nSubnet Mask . . . . . . . . . . . : 255.255.255.0\nDefault Gateway . . . . . . . . . : 10.131.185.1"
	if got := updated.textarea.Value(); got != want {
		t.Fatalf("textarea after Ctrl+V = %q, want %q", got, want)
	}
	if !updated.textareaExpanded {
		t.Fatal("expected Ctrl+V multiline paste to expand textarea")
	}
	if updated.textarea.Height() < 3 {
		t.Fatalf("textarea height = %d, want at least 3", updated.textarea.Height())
	}
	if len(updated.entries) != entryCount {
		t.Fatalf("entry count = %d, want %d", len(updated.entries), entryCount)
	}
	if updated.streamActive {
		t.Fatal("expected Ctrl+V paste not to start sending")
	}
}

func TestTUIBracketedPasteMultilineDoesNotSubmit(t *testing.T) {
	m := newChatTUIModel(nil, "session-1", "claude-haiku-4.5", "chat", DefaultAPIShape, "", nil, nil)
	m.ready = true
	m.mainWidth = 80
	m.viewport = viewport.New(80, 6)
	m.textarea.Focus()
	m.textarea.SetValue("prefix: ")
	entryCount := len(m.entries)

	model, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("first line\nsecond line\nthird line"), Paste: true})
	updated := model.(chatTUIModel)

	want := "prefix: first line\nsecond line\nthird line"
	if got := updated.textarea.Value(); got != want {
		t.Fatalf("textarea after bracketed paste = %q, want %q", got, want)
	}
	if !updated.textareaExpanded {
		t.Fatal("expected bracketed multiline paste to expand textarea")
	}
	if len(updated.entries) != entryCount {
		t.Fatalf("entry count = %d, want %d", len(updated.entries), entryCount)
	}
	if updated.streamActive {
		t.Fatal("expected bracketed paste not to start sending")
	}

	updated.textarea.SetValue(updated.textarea.Value() + " edited")
	model, _ = updated.Update(tea.KeyMsg{Type: tea.KeyEnter})
	updated = model.(chatTUIModel)
	if !updated.pendingSubmitNewline {
		t.Fatal("expected explicit Enter to start deferred submit")
	}
	if len(updated.entries) != 0 {
		t.Fatalf("entry count before deferred submit = %d, want 0", len(updated.entries))
	}

	model, _ = updated.Update(submitInputMsg(updated.submitSeq))
	updated = model.(chatTUIModel)
	if len(updated.entries) != 1 {
		t.Fatalf("entry count after deferred submit = %d, want 1", len(updated.entries))
	}
	if got := updated.entries[0].content; got != want+" edited" {
		t.Fatalf("submitted content = %q, want %q", got, want+" edited")
	}
}

func TestTUIMultilineRapidEnterIsTreatedAsPasteContinuation(t *testing.T) {
	m := newChatTUIModel(nil, "session-1", "claude-haiku-4.5", "chat", DefaultAPIShape, "", nil, nil)
	m.ready = true
	m.mainWidth = 80
	m.viewport = viewport.New(80, 6)
	m.textarea.Focus()

	model, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("first line\nsecond line"), Paste: true})
	updated := model.(chatTUIModel)
	if got := updated.textarea.Value(); got != "first line\nsecond line" {
		t.Fatalf("textarea after bracketed paste = %q", got)
	}

	model, _ = updated.Update(tea.KeyMsg{Type: tea.KeyEnter})
	updated = model.(chatTUIModel)
	if len(updated.entries) != 0 {
		t.Fatalf("entry count after explicit enter = %d, want 0", len(updated.entries))
	}
	if updated.streamActive {
		t.Fatal("expected explicit enter not to submit before deferred timer")
	}
	if !updated.pendingSubmitNewline {
		t.Fatal("expected explicit Enter to start deferred submit")
	}
	if got := updated.textarea.Value(); got != "first line\nsecond line" {
		t.Fatalf("textarea after rapid enter = %q", got)
	}

	model, _ = updated.Update(submitInputMsg(updated.submitSeq))
	updated = model.(chatTUIModel)
	if len(updated.entries) != 1 {
		t.Fatalf("entry count after deferred submit = %d, want 1", len(updated.entries))
	}
	if got := updated.entries[0].content; got != "first line\nsecond line" {
		t.Fatalf("submitted content = %q", got)
	}
}

func TestTUIUnmarkedMulticharacterPasteDoesNotSubmit(t *testing.T) {
	m := newChatTUIModel(nil, "session-1", "claude-haiku-4.5", "chat", DefaultAPIShape, "", nil, nil)
	m.ready = true
	m.mainWidth = 80
	m.viewport = viewport.New(80, 6)
	m.textarea.Focus()

	model, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("Check its version or help info?")})
	updated := model.(chatTUIModel)
	model, _ = updated.Update(tea.KeyMsg{Type: tea.KeyEnter})
	updated = model.(chatTUIModel)
	model, _ = updated.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("Configure it for a proxy setup?")})
	updated = model.(chatTUIModel)

	if len(updated.entries) != 0 {
		t.Fatalf("entry count after unmarked multiline paste = %d, want 0", len(updated.entries))
	}
	if updated.streamActive {
		t.Fatal("expected unmarked multiline paste not to start sending")
	}
	want := "Check its version or help info?\nConfigure it for a proxy setup?"
	if got := updated.textarea.Value(); got != want {
		t.Fatalf("textarea after unmarked multiline paste = %q, want %q", got, want)
	}
}

func TestTUIEscapeCancelsStreamAndClearsInput(t *testing.T) {
	m := newChatTUIModel(nil, "session-1", "claude-haiku-4.5", "chat", DefaultAPIShape, "", nil, nil)
	m.ready = true
	m.mainWidth = 80
	m.viewport = viewport.New(80, 6)
	m.textarea.Focus()
	m.textarea.SetValue("draft message")
	m.streamActive = true
	m.spinning = true
	m.streamBuf = "partial"
	cancelled := false
	m.agentTurnCancel = func() { cancelled = true }

	model, _ := m.Update(tea.KeyMsg{Type: tea.KeyEscape})
	updated := model.(chatTUIModel)

	if updated.streamActive {
		t.Fatal("expected streamActive to be false")
	}
	if updated.spinning {
		t.Fatal("expected spinning to be false")
	}
	if got := updated.textarea.Value(); got != "" {
		t.Fatalf("textarea after Esc = %q, want empty", got)
	}
	if !cancelled {
		t.Fatal("expected Esc to cancel ongoing task")
	}
	if len(updated.entries) == 0 || updated.entries[len(updated.entries)-1].content != "(cancelled)" {
		t.Fatalf("expected cancellation entry, got %+v", updated.entries)
	}
}

func TestTUICtrlCRequiresSecondPressBeforeQuitWhenInputStartsEmpty(t *testing.T) {
	m := newChatTUIModel(nil, "session-1", "claude-haiku-4.5", "chat", DefaultAPIShape, "", nil, nil)
	m.ready = true
	m.mainWidth = 80
	m.viewport = viewport.New(80, 6)
	m.textarea.Focus()

	model, cmd := m.Update(tea.KeyMsg{Type: tea.KeyCtrlC})
	updated := model.(chatTUIModel)
	if cmd != nil {
		t.Fatal("expected first Ctrl+C with empty input not to quit")
	}
	if !updated.ctrlCPrimed {
		t.Fatal("expected first Ctrl+C to prime quit")
	}

	_, cmd = updated.Update(tea.KeyMsg{Type: tea.KeyCtrlC})
	if cmd == nil {
		t.Fatal("expected second Ctrl+C with empty input to quit")
	}
}

func TestTUICtrlCClearsInputBeforeQuitting(t *testing.T) {
	m := newChatTUIModel(nil, "session-1", "claude-haiku-4.5", "chat", DefaultAPIShape, "", nil, nil)
	m.ready = true
	m.mainWidth = 80
	m.viewport = viewport.New(80, 6)
	m.textarea.Focus()
	m.textarea.SetValue("draft message")
	m.promptHistory = []string{"older prompt"}
	m.historyIndex = 0
	m.historyDraft = "saved draft"
	m.historySearchMode = true
	m.historySearchQuery = "draft"

	model, cmd := m.Update(tea.KeyMsg{Type: tea.KeyCtrlC})
	updated := model.(chatTUIModel)

	if got := updated.textarea.Value(); got != "" {
		t.Fatalf("textarea after first Ctrl+C = %q", got)
	}
	if updated.historyIndex != -1 {
		t.Fatalf("historyIndex = %d, want -1", updated.historyIndex)
	}
	if updated.historyDraft != "" {
		t.Fatalf("historyDraft = %q, want empty", updated.historyDraft)
	}
	if updated.historySearchMode {
		t.Fatal("expected history search to exit after clearing input")
	}
	if !updated.ctrlCPrimed {
		t.Fatal("expected first Ctrl+C to prime quit after clearing input")
	}
	if cmd != nil {
		t.Fatal("expected no quit command when first Ctrl+C clears input")
	}

	_, cmd = updated.Update(tea.KeyMsg{Type: tea.KeyCtrlC})
	if cmd == nil {
		t.Fatal("expected second Ctrl+C to quit after input has been cleared")
	}
}

func TestTUIUpDownStillNavigatePromptHistory(t *testing.T) {
	m := newChatTUIModel(nil, "session-1", "claude-haiku-4.5", "chat", DefaultAPIShape, "", nil, nil)
	m.ready = true
	m.mainWidth = 80
	m.viewport = viewport.New(80, 6)
	m.promptHistory = []string{"first prompt", "second prompt"}
	m.textarea.Focus()
	m.textarea.SetValue("draft")
	m.autoFollow = true

	model, _ := m.Update(tea.KeyMsg{Type: tea.KeyUp})
	updated := model.(chatTUIModel)
	if got := updated.textarea.Value(); got != "second prompt" {
		t.Fatalf("textarea after KeyUp = %q", got)
	}
	if updated.historyDraft != "draft" {
		t.Fatalf("historyDraft = %q", updated.historyDraft)
	}
	if updated.autoFollow {
		t.Fatal("expected autoFollow to be disabled after manual upward navigation")
	}

	model, _ = updated.Update(tea.KeyMsg{Type: tea.KeyDown})
	updated = model.(chatTUIModel)
	if got := updated.textarea.Value(); got != "draft" {
		t.Fatalf("textarea after KeyDown = %q", got)
	}
}

func TestTUISyncViewportFollowsLatestContent(t *testing.T) {
	m := newChatTUIModel(nil, "session-1", "claude-haiku-4.5", "chat", DefaultAPIShape, "", nil, nil)
	m.ready = true
	m.mainWidth = 80
	m.viewport = viewport.New(80, 4)
	m.appendEntry(transcriptAssistant, strings.Join([]string{"0", "1", "2", "3", "4", "5", "6", "7", "8", "9", "10", "11", "12", "13", "14", "15", "16", "17", "18", "19"}, "\n"))
	m.syncViewport()
	m.setAutoFollow(false)
	m.viewport.SetYOffset(1)

	m.appendEntry(transcriptAssistant, "next")
	m.syncViewport()

	if !m.viewport.AtBottom() {
		t.Fatal("expected viewport to follow the newest content")
	}
}

func TestTUIEndKeyResumesAutoFollow(t *testing.T) {
	m := newChatTUIModel(nil, "session-1", "claude-haiku-4.5", "chat", DefaultAPIShape, "", nil, nil)
	m.ready = true
	m.mainWidth = 80
	m.viewport = viewport.New(80, 4)
	m.appendEntry(transcriptAssistant, strings.Join([]string{"0", "1", "2", "3", "4", "5", "6", "7", "8", "9", "10", "11", "12", "13", "14", "15", "16", "17", "18", "19"}, "\n"))
	m.syncViewport()
	m.viewport.SetYOffset(2)
	m.autoFollow = false

	model, _ := m.Update(tea.KeyMsg{Type: tea.KeyEnd})
	updated := model.(chatTUIModel)
	if !updated.autoFollow {
		t.Fatal("expected autoFollow to be enabled by End key")
	}
	if !updated.viewport.AtBottom() {
		t.Fatal("expected viewport to jump to bottom on End key")
	}
}

func TestStreamAgentTurnWithCheckerAllowsEmptyFinalTurnAndDropsPreToolNarration(t *testing.T) {
	var completionCalls int
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/api/admin/chat/sessions/session-1":
			_ = json.NewEncoder(w).Encode(map[string]any{
				"id":            "session-1",
				"model_id":      "claude-haiku-4.5",
				"mode":          "agent",
				"agent_backend": "omnicode",
				"messages":      []map[string]any{{"role": "user", "content": "show disk info"}},
			})
		case r.Method == http.MethodPost && r.URL.Path == "/api/admin/chat/sessions/session-1/messages":
			_ = json.NewEncoder(w).Encode(map[string]any{"success": true})
		case r.Method == http.MethodPost && r.URL.Path == "/v1/messages":
			completionCalls++
			if completionCalls == 1 {
				_ = json.NewEncoder(w).Encode(map[string]any{
					"id":          "msg-1",
					"type":        "message",
					"role":        "assistant",
					"model":       "claude-haiku-4.5",
					"stop_reason": "tool_use",
					"usage":       map[string]any{"input_tokens": 10, "output_tokens": 5},
					"content": []map[string]any{
						{"type": "text", "text": "I'll get that for you."},
						{"type": "tool_use", "id": "call-1", "name": "bash", "input": map[string]any{"command": "Write-Output disk-ok"}},
					},
				})
				return
			}
			_ = json.NewEncoder(w).Encode(map[string]any{
				"id":          "msg-2",
				"type":        "message",
				"role":        "assistant",
				"model":       "claude-haiku-4.5",
				"stop_reason": "end_turn",
				"usage":       map[string]any{"input_tokens": 20, "output_tokens": 1},
				"content":     []map[string]any{},
			})
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	client := &testClient{baseURL: server.URL, http: server.Client()}
	eventCh, err := StreamAgentTurnWithChecker(context.Background(), client, "session-1", "claude-haiku-4.5", "omnicode", DefaultAPIShape, "show disk info", func(context.Context, toolspkg.PermissionRequest) (bool, error) {
		return true, nil
	}, 10)
	if err != nil {
		t.Fatalf("StreamAgentTurnWithChecker returned error: %v", err)
	}

	var finalContent string
	var toolCalls, toolResults, doneCount int
	var toolCallContent string
	for event := range eventCh {
		switch event.Type {
		case "token":
			finalContent += event.Content
		case "tool_call":
			toolCalls++
			toolCallContent = event.Content
			finalContent = ""
		case "tool_result":
			toolResults++
		case "done":
			doneCount++
		case "error":
			t.Fatalf("unexpected error event: %s", event.Content)
		}
	}

	if completionCalls != 2 {
		t.Fatalf("completion calls = %d, want 2", completionCalls)
	}
	if toolCalls != 1 {
		t.Fatalf("toolCalls = %d, want 1", toolCalls)
	}
	if toolCallContent != "Write-Output disk-ok" {
		t.Fatalf("tool call content = %q, want command payload", toolCallContent)
	}
	if toolResults != 1 {
		t.Fatalf("toolResults = %d, want 1", toolResults)
	}
	if doneCount != 1 {
		t.Fatalf("doneCount = %d, want 1", doneCount)
	}
	if finalContent != "" {
		t.Fatalf("finalContent = %q, want empty final content", finalContent)
	}
}

func TestRunAgentTurnWithCheckerReturnsEmptyForToolOnlyCompletion(t *testing.T) {
	var completionCalls int
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/api/admin/chat/sessions/session-1":
			_ = json.NewEncoder(w).Encode(map[string]any{
				"id":            "session-1",
				"model_id":      "claude-sonnet-4.5",
				"mode":          "agent",
				"agent_backend": "omnicode",
				"messages":      []map[string]any{{"role": "user", "content": "show disk info"}},
			})
		case r.Method == http.MethodPost && r.URL.Path == "/api/admin/chat/sessions/session-1/messages":
			_ = json.NewEncoder(w).Encode(map[string]any{"success": true})
		case r.Method == http.MethodPost && r.URL.Path == "/v1/messages":
			completionCalls++
			if completionCalls == 1 {
				_ = json.NewEncoder(w).Encode(map[string]any{
					"id":          "msg-1",
					"type":        "message",
					"role":        "assistant",
					"model":       "claude-sonnet-4.5",
					"stop_reason": "tool_use",
					"usage":       map[string]any{"input_tokens": 10, "output_tokens": 5},
					"content": []map[string]any{{
						"type":  "tool_use",
						"id":    "call-1",
						"name":  "bash",
						"input": map[string]any{"command": "Write-Output disk-ok"},
					}},
				})
				return
			}
			_ = json.NewEncoder(w).Encode(map[string]any{
				"id":          "msg-2",
				"type":        "message",
				"role":        "assistant",
				"model":       "claude-sonnet-4.5",
				"stop_reason": "end_turn",
				"usage":       map[string]any{"input_tokens": 20, "output_tokens": 1},
				"content":     []map[string]any{},
			})
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	client := &testClient{baseURL: server.URL, http: server.Client()}
	content, err := RunAgentTurnWithChecker(context.Background(), client, "session-1", "claude-sonnet-4.5", "omnicode", DefaultAPIShape, "show disk info", func(context.Context, toolspkg.PermissionRequest) (bool, error) {
		return true, nil
	}, 10)
	if err != nil {
		t.Fatalf("RunAgentTurnWithChecker returned error: %v", err)
	}
	if completionCalls != 2 {
		t.Fatalf("completion calls = %d, want 2", completionCalls)
	}
	if content != "" {
		t.Fatalf("content = %q, want empty string", content)
	}
}

func TestTUIHandlesPermissionRequestInTranscript(t *testing.T) {
	m := newChatTUIModel(nil, "session-1", "gpt-5.4", "agent", DefaultAPIShape, "omnicode", nil, nil)
	m.ready = true
	m.mainWidth = 80
	m.viewport = viewport.New(80, 20)
	m.textarea.Focus()

	toModel := func(model tea.Model) *chatTUIModel {
		switch v := model.(type) {
		case chatTUIModel:
			copy := v
			return &copy
		case *chatTUIModel:
			return v
		default:
			t.Fatalf("unexpected model type %T", model)
			return nil
		}
	}

	respCh := make(chan bool, 1)
	model, _ := m.Update(permissionRequestMsg{
		req: toolspkg.PermissionRequest{
			SessionID: "session-1",
			ToolName:  "run_command",
			Arguments: map[string]any{"command": "Get-PSDrive"},
		},
		respCh: respCh,
	})
	updated := toModel(model)
	if updated.pendingPermission == nil {
		t.Fatal("expected pending permission to be set")
	}
	if len(updated.entries) == 0 || updated.entries[len(updated.entries)-1].kind != transcriptPermission {
		t.Fatalf("expected permission transcript entry, got %+v", updated.entries)
	}
	if updated.textarea.Placeholder != "y/n (approve tool execution)" {
		t.Fatalf("placeholder = %q", updated.textarea.Placeholder)
	}

	updated.textarea.SetValue("y")
	model, _ = updated.Update(tea.KeyMsg{Type: tea.KeyEnter})
	approved := toModel(model)
	if approved.pendingPermission != nil {
		t.Fatal("expected pending permission to clear")
	}
	select {
	case decision := <-respCh:
		if !decision {
			t.Fatal("expected approval decision true")
		}
	default:
		t.Fatal("expected approval decision to be sent")
	}
	if approved.textarea.Placeholder != approved.normalPlaceholder {
		t.Fatalf("placeholder = %q, want %q", approved.textarea.Placeholder, approved.normalPlaceholder)
	}
}

func TestHandleSlashCommandModelsPicker(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/api/admin/status":
			_ = json.NewEncoder(w).Encode(map[string]any{"activeProviders": []map[string]any{{"id": "provider-a", "name": "Provider A"}, {"id": "provider-b", "name": "Provider B"}}})
		case r.Method == http.MethodGet && r.URL.Path == "/api/admin/providers/provider-a/models":
			_ = json.NewEncoder(w).Encode(map[string]any{"models": []map[string]any{{"id": "gpt-4", "name": "GPT 4", "enabled": true}}})
		case r.Method == http.MethodGet && r.URL.Path == "/api/admin/providers/provider-b/models":
			_ = json.NewEncoder(w).Encode(map[string]any{"models": []map[string]any{{"id": "qwen3", "name": "Qwen 3", "enabled": true}}})
		case r.Method == http.MethodGet && r.URL.Path == "/models":
			_ = json.NewEncoder(w).Encode(map[string]any{"data": []map[string]any{}})
		case r.Method == http.MethodPut && r.URL.Path == "/api/admin/chat/sessions/session-1":
			_ = json.NewEncoder(w).Encode(map[string]any{"success": true})
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	cmd := newTestCommandContext()
	client := &testClient{baseURL: server.URL, http: server.Client()}
	session := &SessionState{
		ID:    "session-1",
		IsTTY: true,
		Mode:  "chat",
		Picker: func(prompt string, models []ModelInfo) (string, error) {
			if prompt != "Select a model" {
				t.Fatalf("unexpected prompt: %s", prompt)
			}
			if len(models) != 2 {
				t.Fatalf("expected 2 models, got %d", len(models))
			}
			if models[0].ProviderName == "" || models[1].ProviderName == "" {
				t.Fatalf("expected provider names to be populated: %+v", models)
			}
			return "provider-b/qwen3", nil
		},
	}

	result, err := handleSlashCommand(cmd, client, session, "/models")
	if err != nil {
		t.Fatalf("handleSlashCommand returned error: %v", err)
	}
	if result.model != "provider-b/qwen3" {
		t.Fatalf("expected selected model provider-b/qwen3, got %+v", result)
	}
	if !strings.Contains(cmd.out.String(), "Switched model to provider-b/qwen3") {
		t.Fatalf("unexpected output: %s", cmd.out.String())
	}
}

func TestHandleSlashCommandModelPickerSelectsProviderQualifiedModel(t *testing.T) {
	putCount := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/api/admin/status":
			_ = json.NewEncoder(w).Encode(map[string]any{"activeProviders": []map[string]any{{"id": "provider-a", "name": "Provider A"}, {"id": "provider-b", "name": "Provider B"}}})
		case r.Method == http.MethodGet && r.URL.Path == "/api/admin/providers/provider-a/models":
			_ = json.NewEncoder(w).Encode(map[string]any{"models": []map[string]any{{"id": "gpt-4", "name": "GPT 4", "enabled": true}}})
		case r.Method == http.MethodGet && r.URL.Path == "/api/admin/providers/provider-b/models":
			_ = json.NewEncoder(w).Encode(map[string]any{"models": []map[string]any{{"id": "qwen3", "name": "Qwen 3", "enabled": true}}})
		case r.Method == http.MethodGet && r.URL.Path == "/models":
			_ = json.NewEncoder(w).Encode(map[string]any{"data": []map[string]any{}})
		case r.Method == http.MethodPut && r.URL.Path == "/api/admin/chat/sessions/session-1":
			putCount++
			var body map[string]string
			if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
				t.Fatalf("decode update request: %v", err)
			}
			if body["model_id"] != "provider-b/qwen3" {
				t.Fatalf("model_id = %q, want provider-b/qwen3", body["model_id"])
			}
			_ = json.NewEncoder(w).Encode(map[string]any{"success": true})
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	cmd := newTestCommandContext()
	client := &testClient{baseURL: server.URL, http: server.Client()}
	session := &SessionState{
		ID:    "session-1",
		Model: "provider-a/gpt-4",
		IsTTY: true,
		Mode:  "chat",
		Picker: func(prompt string, models []ModelInfo) (string, error) {
			if prompt != "Select a model" {
				t.Fatalf("unexpected prompt: %s", prompt)
			}
			if len(models) != 2 {
				t.Fatalf("expected 2 models, got %d", len(models))
			}
			found := false
			for _, model := range models {
				if model.Selector == "provider-b/qwen3" && model.ProviderName == "Provider B" {
					found = true
				}
			}
			if !found {
				t.Fatalf("picker models missing provider-qualified provider-b/qwen3: %+v", models)
			}
			return "provider-b/qwen3", nil
		},
	}

	result, err := handleSlashCommand(cmd, client, session, "/model")
	if err != nil {
		t.Fatalf("handleSlashCommand returned error: %v", err)
	}
	if !result.handled || result.model != "provider-b/qwen3" {
		t.Fatalf("unexpected result: %+v", result)
	}
	if putCount != 1 {
		t.Fatalf("session update count = %d, want 1", putCount)
	}
	if !strings.Contains(cmd.out.String(), "Switched model to provider-b/qwen3") {
		t.Fatalf("unexpected output: %s", cmd.out.String())
	}
}

func TestHandleSlashCommandModelNonTTYShowsCurrentModel(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/api/admin/chat/sessions/session-1":
			_ = json.NewEncoder(w).Encode(map[string]any{"model_id": "provider-a/gpt-4", "messages": []map[string]any{}})
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	pickerCalled := false
	cmd := newTestCommandContext()
	client := &testClient{baseURL: server.URL, http: server.Client()}
	session := &SessionState{
		ID:    "session-1",
		Model: "fallback-model",
		IsTTY: false,
		Mode:  "chat",
		Picker: func(prompt string, models []ModelInfo) (string, error) {
			pickerCalled = true
			return "provider-b/qwen3", nil
		},
	}

	result, err := handleSlashCommand(cmd, client, session, "/model")
	if err != nil {
		t.Fatalf("handleSlashCommand returned error: %v", err)
	}
	if pickerCalled {
		t.Fatal("picker should not be invoked for non-TTY /model")
	}
	if !result.handled || result.model != "provider-a/gpt-4" {
		t.Fatalf("unexpected result: %+v", result)
	}
	if !strings.Contains(cmd.out.String(), "Current model: provider-a/gpt-4") {
		t.Fatalf("unexpected output: %s", cmd.out.String())
	}
}

func TestHandleSlashCommandModelDirectSwitchDoesNotOpenPicker(t *testing.T) {
	putCount := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodPut && r.URL.Path == "/api/admin/chat/sessions/session-1":
			putCount++
			var body map[string]string
			if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
				t.Fatalf("decode update request: %v", err)
			}
			if body["model_id"] != "provider-a/gpt-4" {
				t.Fatalf("model_id = %q, want provider-a/gpt-4", body["model_id"])
			}
			_ = json.NewEncoder(w).Encode(map[string]any{"success": true})
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	pickerCalled := false
	cmd := newTestCommandContext()
	client := &testClient{baseURL: server.URL, http: server.Client()}
	session := &SessionState{
		ID:    "session-1",
		IsTTY: true,
		Mode:  "chat",
		Picker: func(prompt string, models []ModelInfo) (string, error) {
			pickerCalled = true
			return "provider-b/qwen3", nil
		},
	}

	result, err := handleSlashCommand(cmd, client, session, "/model provider-a/gpt-4")
	if err != nil {
		t.Fatalf("handleSlashCommand returned error: %v", err)
	}
	if pickerCalled {
		t.Fatal("picker should not be invoked for direct /model switch")
	}
	if putCount != 1 {
		t.Fatalf("session update count = %d, want 1", putCount)
	}
	if !result.handled || result.model != "provider-a/gpt-4" {
		t.Fatalf("unexpected result: %+v", result)
	}
	if !strings.Contains(cmd.out.String(), "Switched model to provider-a/gpt-4") {
		t.Fatalf("unexpected output: %s", cmd.out.String())
	}
}

func TestHandleSlashCommandModelPickerCancellationLeavesModelUnchanged(t *testing.T) {
	putCount := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/api/admin/status":
			_ = json.NewEncoder(w).Encode(map[string]any{"activeProviders": []map[string]any{{"id": "provider-a", "name": "Provider A"}}})
		case r.Method == http.MethodGet && r.URL.Path == "/api/admin/providers/provider-a/models":
			_ = json.NewEncoder(w).Encode(map[string]any{"models": []map[string]any{{"id": "gpt-4", "name": "GPT 4", "enabled": true}}})
		case r.Method == http.MethodGet && r.URL.Path == "/models":
			_ = json.NewEncoder(w).Encode(map[string]any{"data": []map[string]any{}})
		case r.Method == http.MethodPut && r.URL.Path == "/api/admin/chat/sessions/session-1":
			putCount++
			_ = json.NewEncoder(w).Encode(map[string]any{"success": true})
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	cmd := newTestCommandContext()
	client := &testClient{baseURL: server.URL, http: server.Client()}
	session := &SessionState{
		ID:    "session-1",
		Model: "provider-a/gpt-4",
		IsTTY: true,
		Mode:  "chat",
		Picker: func(prompt string, models []ModelInfo) (string, error) {
			return "", nil
		},
	}

	result, err := handleSlashCommand(cmd, client, session, "/model")
	if err != nil {
		t.Fatalf("handleSlashCommand returned error: %v", err)
	}
	if !result.handled || result.model != "" {
		t.Fatalf("unexpected result: %+v", result)
	}
	if putCount != 0 {
		t.Fatalf("session update count = %d, want 0", putCount)
	}
	if session.Model != "provider-a/gpt-4" {
		t.Fatalf("session model = %q, want provider-a/gpt-4", session.Model)
	}
}

func TestEnsureSessionDefaultsModeToChat(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodPost && r.URL.Path == "/api/admin/chat/sessions":
			_ = json.NewEncoder(w).Encode(map[string]any{"session_id": "session-1"})
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	cmd := newTestCommandContext()
	client := &testClient{baseURL: server.URL, http: server.Client()}

	session, err := EnsureSession(cmd, client, "", "")
	if err != nil {
		t.Fatalf("EnsureSession returned error: %v", err)
	}
	if session.Mode != "chat" {
		t.Fatalf("mode = %q, want chat", session.Mode)
	}
}

func TestChatPrompt(t *testing.T) {
	if got := ChatPrompt("You", false); got != "You> " {
		t.Fatalf("unexpected non-tty prompt: %q", got)
	}
	if got := ChatHeader("Assistant", false); got != "Assistant>" {
		t.Fatalf("unexpected non-tty header: %q", got)
	}
	if got := ChatPrompt("You", true); !strings.Contains(got, "You>") {
		t.Fatalf("unexpected tty prompt: %q", got)
	}
	if got := ChatHeader("Assistant", true); !strings.Contains(got, "Assistant>") {
		t.Fatalf("unexpected tty header: %q", got)
	}
}

func TestPromptModelPickerUsesSelectedModel(t *testing.T) {
	selected, err := PromptModelPicker("Select a model", []ModelInfo{{ID: "gpt-4", Selector: "gpt-4"}, {ID: "qwen3", Selector: "qwen3"}}, func(prompt string, options []string) (string, error) {
		if prompt != "Select a model" {
			t.Fatalf("unexpected prompt: %s", prompt)
		}
		return options[1], nil
	})
	if err != nil {
		t.Fatalf("picker returned error: %v", err)
	}
	if selected != "qwen3" {
		t.Fatalf("selected = %q, want qwen3", selected)
	}
}

func TestFilterModelsNoMatch(t *testing.T) {
	models := []ModelInfo{{ID: "gpt-4", Owner: "provider-a", Name: "GPT 4"}}
	filtered := FilterModels(models, "nomatch")
	if len(filtered) != 0 {
		t.Fatalf("expected no models, got %+v", filtered)
	}
}

type testClient struct {
	baseURL string
	http    *http.Client
}

func (c *testClient) request(method, path string, body string) ([]byte, error) {
	var reqBody *strings.Reader
	if body == "" {
		reqBody = strings.NewReader("")
	} else {
		reqBody = strings.NewReader(body)
	}
	req, err := http.NewRequest(method, c.baseURL+path, reqBody)
	if err != nil {
		return nil, err
	}
	if body != "" {
		req.Header.Set("Content-Type", "application/json")
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	return io.ReadAll(resp.Body)
}

func (c *testClient) Get(path string) ([]byte, error) { return c.request(http.MethodGet, path, "") }
func (c *testClient) Post(path string, body any) ([]byte, error) {
	b, _ := json.Marshal(body)
	return c.request(http.MethodPost, path, string(b))
}
func (c *testClient) Put(path string, body any) ([]byte, error) {
	b, _ := json.Marshal(body)
	return c.request(http.MethodPut, path, string(b))
}
func (c *testClient) Delete(path string) ([]byte, error) {
	return c.request(http.MethodDelete, path, "")
}
func (c *testClient) PostStream(path string, body any) (*http.Response, error) {
	b, _ := json.Marshal(body)
	req, err := http.NewRequest(http.MethodPost, c.baseURL+path, strings.NewReader(string(b)))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	return c.http.Do(req)
}

// 闁冲厜鍋撻柍鍏夊亾闁冲厜鍋?Paste simulation tests 闁冲厜鍋撻柍鍏夊亾闁冲厜鍋撻柍鍏夊亾闁冲厜鍋撻柍鍏夊亾闁冲厜鍋撻柍鍏夊亾闁冲厜鍋撻柍鍏夊亾闁冲厜鍋撻柍鍏夊亾闁冲厜鍋撻柍鍏夊亾闁冲厜鍋撻柍鍏夊亾闁冲厜鍋撻柍鍏夊亾闁冲厜鍋撻柍鍏夊亾闁冲厜鍋撻柍鍏夊亾闁冲厜鍋撻柍鍏夊亾闁冲厜鍋撻柍鍏夊亾闁冲厜鍋撻柍鍏夊亾闁冲厜鍋撻柍鍏夊亾闁冲厜鍋撻柍鍏夊亾闁冲厜鍋撻柍鍏夊亾闁冲厜鍋撻柍鍏夊亾闁冲厜鍋撻柍鍏夊亾闁冲厜鍋撻柍鍏夊亾闁冲厜鍋撻柍鍏夊亾闁冲厜鍋撻柍鍏夊亾闁冲厜鍋撻柍鍏夊亾闁冲厜鍋撻柍鍏夊亾闁冲厜鍋撻柍鍏夊亾闁冲厜鍋?

func readyChatTUIModelForInput() chatTUIModel {
	m := newChatTUIModel(nil, "session-1", "model", "chat", "openai", "", nil, nil)
	raw, _ := m.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	return raw.(chatTUIModel)
}

func TestTUITerminalOptions(t *testing.T) {
	t.Parallel()

	programIntField := func(p *tea.Program, name string) int64 {
		return reflect.ValueOf(p).Elem().FieldByName(name).Int()
	}
	actual := tea.NewProgram(nil, tuiTerminalOptions()...)
	expected := tea.NewProgram(
		nil,
		tea.WithAltScreen(),
		tea.WithMouseCellMotion(),
		tea.WithInputTTY(),
	)
	for _, name := range []string{"inputType", "startupOptions"} {
		if got, want := programIntField(actual, name), programIntField(expected, name); got != want {
			t.Fatalf("%s = %d, want %d", name, got, want)
		}
	}
}

// sendLineByLinePaste simulates a terminal that pastes each line as a rune
// block separated by regular Enter key events. This is the regression path:
// each pasted newline can look like a real submit unless the TUI defers it.
func sendLineByLinePaste(m chatTUIModel, text string) chatTUIModel {
	lines := strings.Split(text, "\n")
	for i, line := range lines {
		if line != "" {
			raw, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(line)})
			m = raw.(chatTUIModel)
		}
		if i < len(lines)-1 {
			raw, _ := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
			m = raw.(chatTUIModel)
		}
	}
	return m
}

// sendBracketedPaste simulates terminals that mark pasted runes and newline
// events with Paste=true.
func sendBracketedPaste(m chatTUIModel, text string) chatTUIModel {
	lines := strings.Split(text, "\n")
	for i, line := range lines {
		if line != "" {
			raw, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(line), Paste: true})
			m = raw.(chatTUIModel)
		}
		if i < len(lines)-1 {
			raw, _ := m.Update(tea.KeyMsg{Type: tea.KeyEnter, Paste: true})
			m = raw.(chatTUIModel)
		}
	}
	return m
}

func TestPasteMultiLineLineByLineDoesNotAutoSend(t *testing.T) {
	t.Parallel()

	m := readyChatTUIModelForInput()
	m = sendLineByLinePaste(m, "line one\nline two\nline three")

	if m.streamActive {
		t.Fatal("line-by-line paste triggered an automatic send")
	}
	if m.pendingSubmitNewline {
		t.Fatal("line-by-line paste left submit pending instead of absorbing newline")
	}
	if got := m.textarea.Value(); got != "line one\nline two\nline three" {
		t.Fatalf("textarea = %q, want full multiline paste", got)
	}
	if got := len(m.entries); got != 0 {
		t.Fatalf("entries = %d, want 0 before explicit user send", got)
	}
}

func TestPasteMultiLineBracketedDoesNotAutoSend(t *testing.T) {
	t.Parallel()

	m := readyChatTUIModelForInput()
	m = sendBracketedPaste(m, "alpha\nbeta\ngamma")

	if m.streamActive {
		t.Fatal("bracketed paste triggered an automatic send")
	}
	if got := m.textarea.Value(); got != "alpha\nbeta\ngamma" {
		t.Fatalf("textarea = %q, want full bracketed paste", got)
	}
	if got := len(m.entries); got != 0 {
		t.Fatalf("entries = %d, want 0 before explicit user send", got)
	}
}

func TestPasteMultiLineSendsAllTogetherAfterExplicitEnter(t *testing.T) {
	t.Parallel()

	m := readyChatTUIModelForInput()
	m = sendLineByLinePaste(m, "hello world\nsecond line")

	raw, _ := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = raw.(chatTUIModel)
	if !m.pendingSubmitNewline {
		t.Fatal("explicit Enter did not start deferred submit")
	}
	if got := m.textarea.Value(); got != "hello world\nsecond line" {
		t.Fatalf("textarea changed before deferred submit fired: %q", got)
	}

	raw, _ = m.Update(submitInputMsg(m.submitSeq))
	m = raw.(chatTUIModel)
	if got := m.textarea.Value(); got != "" {
		t.Fatalf("textarea = %q, want cleared after submit", got)
	}
	if !m.streamActive {
		t.Fatal("submit did not transition into streaming state")
	}
	if got := len(m.entries); got != 1 {
		t.Fatalf("entries = %d, want 1 submitted user entry", got)
	}
	if got := m.entries[0].content; got != "hello world\nsecond line" {
		t.Fatalf("submitted content = %q, want full multiline text", got)
	}
}

func TestPendingSubmitIsAbsorbedByIncomingPasteText(t *testing.T) {
	t.Parallel()

	m := readyChatTUIModelForInput()
	m.applyTextareaValue("line one")

	raw, _ := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = raw.(chatTUIModel)
	if !m.pendingSubmitNewline {
		t.Fatal("Enter did not mark submit as pending")
	}

	raw, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("line two")})
	m = raw.(chatTUIModel)
	if m.pendingSubmitNewline {
		t.Fatal("incoming paste text did not absorb pending submit newline")
	}
	if got := m.textarea.Value(); got != "line one\nline two" {
		t.Fatalf("textarea = %q, want absorbed newline before pasted text", got)
	}
}

func TestChineseRunesArePreservedInTextareaInput(t *testing.T) {
	t.Parallel()

	m := readyChatTUIModelForInput()
	input := "你好，世界"
	raw, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(input)})
	m = raw.(chatTUIModel)

	if got, want := m.textarea.Value(), input; got != want {
		t.Fatalf("textarea value = %q, want %q", got, want)
	}
}

func TestRenderSingleLineTextareaCJKFitsInWidth(t *testing.T) {
	t.Parallel()

	m := readyChatTUIModelForInput()
	m.applyTextareaValue("你好世界你好世界")
	rendered := m.renderSingleLineTextarea(40)

	displayWidth := xansi.StringWidth(rendered)
	if displayWidth > 40 {
		t.Fatalf("rendered display width = %d, exceeds available width 40", displayWidth)
	}
	if !strings.Contains(rendered, "你好") {
		t.Fatal("rendered output should contain Chinese characters")
	}
}

func TestRenderSingleLineTextareaCJKScrolling(t *testing.T) {
	t.Parallel()

	m := readyChatTUIModelForInput()
	long := strings.Repeat("界", 50)
	m.applyTextareaValue(long)
	rendered := m.renderSingleLineTextarea(30)

	displayWidth := xansi.StringWidth(rendered)
	if displayWidth > 30 {
		t.Fatalf("rendered display width = %d, exceeds available width 30; text: %q", displayWidth, rendered)
	}
}

func TestRenderSingleLineTextareaMixedASCIIAndCJK(t *testing.T) {
	t.Parallel()

	m := readyChatTUIModelForInput()
	m.applyTextareaValue("hello你好world世界")
	rendered := m.renderSingleLineTextarea(40)

	displayWidth := xansi.StringWidth(rendered)
	if displayWidth > 40 {
		t.Fatalf("rendered display width = %d, exceeds available width 40", displayWidth)
	}
}
func TestConfigureUTF8ConsoleIsSafeToCall(t *testing.T) {
	if err := configureUTF8Console(); err != nil {
		t.Fatalf("configureUTF8Console() returned error: %v", err)
	}
}
