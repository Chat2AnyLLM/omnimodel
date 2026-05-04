package chat

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	toolspkg "omnillm/internal/tools"

	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
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
				"agent_backend": "google-adk",
				"messages":      []any{},
			})
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	cmd := newTestCommandContext()
	client := &testClient{baseURL: server.URL, http: server.Client()}
	session := &SessionState{ID: "session-1", Mode: "agent", AgentBackend: "agent-sdk-go"}

	result, err := handleSlashCommand(cmd, client, session, "/agent")
	if err != nil {
		t.Fatalf("handleSlashCommand returned error: %v", err)
	}
	if !result.handled || result.agentBackend != "google-adk" {
		t.Fatalf("unexpected result: %+v", result)
	}
	text := cmd.out.String()
	if !strings.Contains(text, "Current agent backend: google-adk") {
		t.Fatalf("unexpected output: %s", text)
	}
	if !strings.Contains(text, "Supported backends: agent-sdk-go, google-adk") {
		t.Fatalf("missing supported backends in output: %s", text)
	}
}

func TestHandleSlashCommandAgentSwitch(t *testing.T) {
	var updatedBackend string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodPut && r.URL.Path == "/api/admin/chat/sessions/session-1":
			var body map[string]string
			if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
				t.Fatalf("decode request: %v", err)
			}
			updatedBackend = body["agent_backend"]
			_ = json.NewEncoder(w).Encode(map[string]any{"success": true})
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	cmd := newTestCommandContext()
	client := &testClient{baseURL: server.URL, http: server.Client()}

	result, err := handleSlashCommand(cmd, client, &SessionState{ID: "session-1", Mode: "agent"}, "/agent google-adk")
	if err != nil {
		t.Fatalf("handleSlashCommand returned error: %v", err)
	}
	if !result.handled || result.agentBackend != "google-adk" {
		t.Fatalf("unexpected result: %+v", result)
	}
	if updatedBackend != "google-adk" {
		t.Fatalf("updated backend = %q, want google-adk", updatedBackend)
	}
	text := cmd.out.String()
	if !strings.Contains(text, "Switched agent backend to google-adk") {
		t.Fatalf("unexpected output: %s", text)
	}
	if !strings.Contains(text, "Supported backends: agent-sdk-go, google-adk") {
		t.Fatalf("missing supported backends in output: %s", text)
	}
}

func TestHandleSlashCommandAgentInvalid(t *testing.T) {
	cmd := newTestCommandContext()
	_, err := handleSlashCommand(cmd, nil, &SessionState{ID: "session-1", Mode: "agent"}, "/agent nope")
	if err == nil || !strings.Contains(err.Error(), "supported backends: agent-sdk-go, google-adk") {
		t.Fatalf("expected supported backend error, got %v", err)
	}
}

func TestSupportedAgentBackendsText(t *testing.T) {
	if got := supportedAgentBackendsText(); got != "agent-sdk-go, google-adk" {
		t.Fatalf("supportedAgentBackendsText() = %q", got)
	}
	if !isSupportedAgentBackend("agent-sdk-go") {
		t.Fatal("expected agent-sdk-go to be supported")
	}
	if !isSupportedAgentBackend("google-adk") {
		t.Fatal("expected google-adk to be supported")
	}
	if isSupportedAgentBackend("nope") {
		t.Fatal("did not expect nope to be supported")
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

func TestHandleSlashCommandModelSwitch(t *testing.T) {
	var updatedModel string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodPut && r.URL.Path == "/api/admin/chat/sessions/session-1":
			var body map[string]string
			if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
				t.Fatalf("decode request: %v", err)
			}
			updatedModel = body["model_id"]
			_ = json.NewEncoder(w).Encode(map[string]any{"success": true})
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	cmd := newTestCommandContext()
	client := &testClient{baseURL: server.URL, http: server.Client()}

	result, err := handleSlashCommand(cmd, client, &SessionState{ID: "session-1", Mode: "chat"}, "/model qwen3")
	if err != nil {
		t.Fatalf("handleSlashCommand returned error: %v", err)
	}
	if !result.handled || result.model != "qwen3" {
		t.Fatalf("unexpected result: %+v", result)
	}
	if updatedModel != "qwen3" {
		t.Fatalf("updated model = %q, want qwen3", updatedModel)
	}
	if !strings.Contains(cmd.out.String(), "Switched model to qwen3") {
		t.Fatalf("unexpected output: %s", cmd.out.String())
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
				"agent_backend": "agent-sdk-go",
				"messages":      []map[string]any{{"role": "user", "content": "show me disk usage"}},
			})
		case r.Method == http.MethodPost && r.URL.Path == "/v1/chat/completions":
			var body map[string]any
			if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
				t.Fatalf("decode request: %v", err)
			}
			postBodies = append(postBodies, body)
			if len(postBodies) == 1 {
				_ = json.NewEncoder(w).Encode(map[string]any{
					"id":    "resp-1",
					"model": "gpt-5.4",
					"choices": []map[string]any{{
						"index": 0,
						"message": map[string]any{
							"role": "assistant",
							"tool_calls": []map[string]any{{
								"id":   "call-1",
								"type": "function",
								"function": map[string]any{
									"name":      "bash",
									"arguments": `{"command":"Write-Output disk-ok"}`,
								},
							}},
						},
						"finish_reason": "tool_calls",
					}},
				})
				return
			}
			_ = json.NewEncoder(w).Encode(map[string]any{
				"id":    "resp-2",
				"model": "gpt-5.4",
				"choices": []map[string]any{{
					"index": 0,
					"message": map[string]any{
						"role":    "assistant",
						"content": "disk-ok",
					},
					"finish_reason": "stop",
				}},
			})
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	client := &testClient{baseURL: server.URL, http: server.Client()}
	checkerCalls := 0
	content, err := RunAgentTurnWithChecker(context.Background(), client, "session-1", "gpt-5.4", "agent-sdk-go", "show me disk usage", func(ctx context.Context, req toolspkg.PermissionRequest) (bool, error) {
		checkerCalls++
		if req.ToolName != "bash" {
			t.Fatalf("tool name = %q", req.ToolName)
		}
		return true, nil
	})
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
				"agent_backend": "agent-sdk-go",
				"messages":      []map[string]any{{"role": "user", "content": "show me disk usage"}},
			})
		case r.Method == http.MethodPost && r.URL.Path == "/v1/chat/completions":
			var body map[string]any
			if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
				t.Fatalf("decode request: %v", err)
			}
			postBodies = append(postBodies, body)
			if len(postBodies) == 1 {
				_ = json.NewEncoder(w).Encode(map[string]any{
					"id":    "resp-1",
					"model": "gpt-5.4",
					"choices": []map[string]any{{
						"index": 0,
						"message": map[string]any{
							"role": "assistant",
							"tool_calls": []map[string]any{{
								"id":   "call-1",
								"type": "function",
								"function": map[string]any{
									"name":      "bash",
									"arguments": `{"command":"Write-Output disk-ok"}`,
								},
							}},
						},
						"finish_reason": "tool_calls",
					}},
				})
				return
			}
			_ = json.NewEncoder(w).Encode(map[string]any{
				"id":    "resp-2",
				"model": "gpt-5.4",
				"choices": []map[string]any{{
					"index": 0,
					"message": map[string]any{
						"role":    "assistant",
						"content": "disk-ok",
					},
					"finish_reason": "stop",
				}},
			})
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	cmd := newTestCommandContext()
	cmd.in.WriteString("y\n")
	client := &testClient{baseURL: server.URL, http: server.Client()}

	content, err := RunAgentTurn(client, "session-1", "gpt-5.4", "agent-sdk-go", "show me disk usage", cmd)
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

func TestTUIHandlesPermissionRequestInTranscript(t *testing.T) {
	m := newChatTUIModel(nil, "session-1", "gpt-5.4", "agent", "agent-sdk-go", nil)
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
