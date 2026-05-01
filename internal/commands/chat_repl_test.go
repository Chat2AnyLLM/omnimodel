package commands

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/spf13/cobra"
)

func TestHandleChatSlashCommandHelp(t *testing.T) {
	cmd := &cobra.Command{}
	var out bytes.Buffer
	cmd.SetOut(&out)

	result, err := handleChatSlashCommand(cmd, &Client{}, &chatSessionState{ID: "session-1"}, "/help")
	if err != nil {
		t.Fatalf("handleChatSlashCommand returned error: %v", err)
	}
	if !result.handled || result.exit {
		t.Fatalf("unexpected result: %+v", result)
	}
	if !strings.Contains(out.String(), "/models <filter>") {
		t.Fatalf("help output missing filter guidance:\n%s", out.String())
	}
}

func TestHandleChatSlashCommandUnknown(t *testing.T) {
	cmd := &cobra.Command{}
	_, err := handleChatSlashCommand(cmd, &Client{}, &chatSessionState{ID: "session-1"}, "/wat")
	if err == nil || !strings.Contains(err.Error(), "unknown command") {
		t.Fatalf("expected unknown command error, got %v", err)
	}
}

func TestListChatModelsSorted(t *testing.T) {
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

	client := &Client{BaseURL: server.URL, http: server.Client()}
	models, err := listChatModels(client)
	if err != nil {
		t.Fatalf("listChatModels returned error: %v", err)
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

func TestFilterChatModels(t *testing.T) {
	models := []chatModelInfo{
		{ID: "gpt-4", Owner: "provider-a", Name: "GPT 4"},
		{ID: "qwen3", Owner: "provider-b", Name: "Qwen 3"},
		{ID: "claude-sonnet", Owner: "provider-c", Name: "Claude Sonnet"},
	}

	filtered := filterChatModels(models, "qwe")
	if len(filtered) != 1 || filtered[0].ID != "qwen3" {
		t.Fatalf("unexpected filtered models: %+v", filtered)
	}
}

func TestHandleChatSlashCommandModelSwitch(t *testing.T) {
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

	cmd := &cobra.Command{}
	var out bytes.Buffer
	cmd.SetOut(&out)
	client := &Client{BaseURL: server.URL, http: server.Client()}

	result, err := handleChatSlashCommand(cmd, client, &chatSessionState{ID: "session-1"}, "/model qwen3")
	if err != nil {
		t.Fatalf("handleChatSlashCommand returned error: %v", err)
	}
	if !result.handled || result.model != "qwen3" {
		t.Fatalf("unexpected result: %+v", result)
	}
	if updatedModel != "qwen3" {
		t.Fatalf("updated model = %q, want qwen3", updatedModel)
	}
	if !strings.Contains(out.String(), "Switched model to qwen3") {
		t.Fatalf("unexpected output: %s", out.String())
	}
}

func TestHandleChatSlashCommandModelsFilterOutput(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/admin/status":
			_ = json.NewEncoder(w).Encode(map[string]any{
				"activeProviders": []map[string]any{
					{"id": "provider-a", "name": "Provider A"},
					{"id": "provider-b", "name": "Provider B"},
				},
			})
		case "/api/admin/providers/provider-a/models":
			_ = json.NewEncoder(w).Encode(map[string]any{
				"models": []map[string]any{{"id": "gpt-4", "name": "GPT 4", "enabled": true}},
			})
		case "/api/admin/providers/provider-b/models":
			_ = json.NewEncoder(w).Encode(map[string]any{
				"models": []map[string]any{{"id": "qwen3", "name": "Qwen 3", "enabled": true}},
			})
		case "/models":
			_ = json.NewEncoder(w).Encode(map[string]any{"data": []map[string]any{}})
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	cmd := &cobra.Command{}
	var out bytes.Buffer
	cmd.SetOut(&out)
	client := &Client{BaseURL: server.URL, http: server.Client()}
	session := &chatSessionState{ID: "session-1", IsTTY: false}

	result, err := handleChatSlashCommand(cmd, client, session, "/models qwe")
	if err != nil {
		t.Fatalf("handleChatSlashCommand returned error: %v", err)
	}
	if !result.handled {
		t.Fatalf("expected handled result, got %+v", result)
	}
	text := out.String()
	if !strings.Contains(text, "provider-b/qwen3") || strings.Contains(text, "provider-a/gpt-4") {
		t.Fatalf("unexpected filtered output:\n%s", text)
	}
}

func TestHandleChatSlashCommandModelsPicker(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/api/admin/status":
			_ = json.NewEncoder(w).Encode(map[string]any{
				"activeProviders": []map[string]any{
					{"id": "provider-a", "name": "Provider A"},
					{"id": "provider-b", "name": "Provider B"},
				},
			})
		case r.Method == http.MethodGet && r.URL.Path == "/api/admin/providers/provider-a/models":
			_ = json.NewEncoder(w).Encode(map[string]any{
				"models": []map[string]any{{"id": "gpt-4", "name": "GPT 4", "enabled": true}},
			})
		case r.Method == http.MethodGet && r.URL.Path == "/api/admin/providers/provider-b/models":
			_ = json.NewEncoder(w).Encode(map[string]any{
				"models": []map[string]any{{"id": "qwen3", "name": "Qwen 3", "enabled": true}},
			})
		case r.Method == http.MethodGet && r.URL.Path == "/models":
			_ = json.NewEncoder(w).Encode(map[string]any{"data": []map[string]any{}})
		case r.Method == http.MethodPut && r.URL.Path == "/api/admin/chat/sessions/session-1":
			_ = json.NewEncoder(w).Encode(map[string]any{"success": true})
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	cmd := &cobra.Command{}
	var out bytes.Buffer
	cmd.SetOut(&out)
	client := &Client{BaseURL: server.URL, http: server.Client()}
	session := &chatSessionState{
		ID:    "session-1",
		IsTTY: true,
		Picker: func(prompt string, models []chatModelInfo) (string, error) {
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

	result, err := handleChatSlashCommand(cmd, client, session, "/models")
	if err != nil {
		t.Fatalf("handleChatSlashCommand returned error: %v", err)
	}
	if result.model != "provider-b/qwen3" {
		t.Fatalf("expected selected model provider-b/qwen3, got %+v", result)
	}
	if !strings.Contains(out.String(), "Switched model to provider-b/qwen3") {
		t.Fatalf("unexpected output: %s", out.String())
	}
}

func TestFormatChatPrompt(t *testing.T) {
	if got := FormatChatPrompt("You", false); got != "You> " {
		t.Fatalf("unexpected non-tty prompt: %q", got)
	}
	if got := FormatChatHeader("Assistant", false); got != "Assistant>" {
		t.Fatalf("unexpected non-tty header: %q", got)
	}
	if got := FormatChatPrompt("You", true); !strings.Contains(got, "You>") {
		t.Fatalf("unexpected tty prompt: %q", got)
	}
	if got := FormatChatHeader("Assistant", true); !strings.Contains(got, "Assistant>") {
		t.Fatalf("unexpected tty header: %q", got)
	}
}

func TestPromptModelPickerUsesSelectedModel(t *testing.T) {
	picker := func(prompt string, models []chatModelInfo) (string, error) {
		if prompt != "Select a model" {
			t.Fatalf("unexpected prompt: %s", prompt)
		}
		return models[1].ID, nil
	}

	selected, err := picker("Select a model", []chatModelInfo{{ID: "gpt-4"}, {ID: "qwen3"}})
	if err != nil {
		t.Fatalf("picker returned error: %v", err)
	}
	if selected != "qwen3" {
		t.Fatalf("selected = %q, want qwen3", selected)
	}
}

func TestFilterChatModelsNoMatch(t *testing.T) {
	models := []chatModelInfo{{ID: "gpt-4", Owner: "provider-a", Name: "GPT 4"}}
	filtered := filterChatModels(models, "nomatch")
	if len(filtered) != 0 {
		t.Fatalf("expected no models, got %+v", filtered)
	}
}

func TestPrintChatModelsFilteredTable(t *testing.T) {
	models := []chatModelInfo{{ID: "qwen3", Owner: "provider-b", Name: "Qwen 3"}}
	filtered := filterChatModels(models, "qwe")
	if len(filtered) != 1 {
		t.Fatalf("expected one model, got %+v", filtered)
	}
	if filtered[0].ID != "qwen3" {
		t.Fatalf("unexpected model: %+v", filtered)
	}
	_ = fmt.Sprintf("%v", filtered)
}
