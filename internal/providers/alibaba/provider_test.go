package alibaba

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"omnillm/internal/cif"
	"omnillm/internal/database"
	"omnillm/internal/providers/openaicompat"
	"omnillm/internal/providers/types"
	"os"
	"strings"
	"testing"
)

func TestMain(m *testing.M) {
	dir, err := os.MkdirTemp("", "alibaba-provider-test-*")
	if err != nil {
		panic(err)
	}
	defer os.RemoveAll(dir)
	if err := database.InitializeDatabase(dir); err != nil {
		panic(err)
	}
	os.Exit(m.Run())
}

// ─── NormalizeAPIPlan ────────────────────────────────────────────────────────

func TestNormalizeAPIPlan(t *testing.T) {
	cases := []struct {
		input string
		want  string
	}{
		{"", "standard"},
		{"standard", "standard"},
		{"STANDARD", "standard"},
		{"coding", "coding-plan"},
		{"coding-plan", "coding-plan"},
		{"coding_plan", "coding-plan"},
		{"CODING", "coding-plan"},
		{"other", "standard"},
	}
	for _, tc := range cases {
		got := NormalizeAPIPlan(tc.input)
		if got != tc.want {
			t.Errorf("NormalizeAPIPlan(%q) = %q, want %q", tc.input, got, tc.want)
		}
	}
}

// ─── DefaultAPIBaseURL ───────────────────────────────────────────────────────

func TestDefaultAPIBaseURL(t *testing.T) {
	cases := []struct {
		plan   string
		region string
		want   string
	}{
		{"standard", "global", BaseURLGlobal},
		{"standard", "china", BaseURLChina},
		{"standard", "", BaseURLGlobal},
		{"coding-plan", "global", CodingPlanBaseURLGlobal},
		{"coding-plan", "china", CodingPlanBaseURLChina},
		{"coding", "china", CodingPlanBaseURLChina},
	}
	for _, tc := range cases {
		got := DefaultAPIBaseURL(tc.plan, tc.region)
		if got != tc.want {
			t.Errorf("DefaultAPIBaseURL(%q, %q) = %q, want %q", tc.plan, tc.region, got, tc.want)
		}
	}
}

// ─── EnsureBaseURL ───────────────────────────────────────────────────────────

func TestEnsureBaseURL(t *testing.T) {
	cases := []struct {
		raw  string
		want string
	}{
		{"", BaseURLGlobal},
		{"dashscope-intl.aliyuncs.com/compatible-mode", "https://dashscope-intl.aliyuncs.com/compatible-mode/v1"},
		{"https://dashscope-intl.aliyuncs.com/compatible-mode/v1", "https://dashscope-intl.aliyuncs.com/compatible-mode/v1"},
		{"https://dashscope-intl.aliyuncs.com/compatible-mode/v1/", "https://dashscope-intl.aliyuncs.com/compatible-mode/v1"},
		{"  https://example.com/v1  ", "https://example.com/v1"},
	}
	for _, tc := range cases {
		got := EnsureBaseURL(tc.raw)
		if got != tc.want {
			t.Errorf("EnsureBaseURL(%q) = %q, want %q", tc.raw, got, tc.want)
		}
	}
}

// ─── IsChatCompletionsModel ──────────────────────────────────────────────────

func TestIsChatCompletionsModel(t *testing.T) {
	cases := []struct {
		modelID string
		want    bool
	}{
		{"qwen3-max", true},
		{"qwen3-coder-plus", true},
		{"qwen-turbo", true},
		{"qwen-realtime-v1", false},
		{"REALTIME-model", false},
	}
	for _, tc := range cases {
		got := IsChatCompletionsModel(tc.modelID)
		if got != tc.want {
			t.Errorf("IsChatCompletionsModel(%q) = %v, want %v", tc.modelID, got, tc.want)
		}
	}
}

// ─── NormalizeBaseURL ────────────────────────────────────────────────────────

func TestNormalizeBaseURL(t *testing.T) {
	t.Run("standard global", func(t *testing.T) {
		cfg := map[string]interface{}{"auth_type": "api-key", "plan": "standard", "region": "global"}
		got := NormalizeBaseURL(cfg)
		if got != BaseURLGlobal {
			t.Errorf("got %q, want %q", got, BaseURLGlobal)
		}
	})
	t.Run("coding-plan china", func(t *testing.T) {
		cfg := map[string]interface{}{"auth_type": "api-key", "plan": "coding-plan", "region": "china"}
		got := NormalizeBaseURL(cfg)
		if got != CodingPlanBaseURLChina {
			t.Errorf("got %q, want %q", got, CodingPlanBaseURLChina)
		}
	})
	t.Run("explicit base_url wins", func(t *testing.T) {
		cfg := map[string]interface{}{"base_url": "https://custom.example.com/v1"}
		got := NormalizeBaseURL(cfg)
		if got != "https://custom.example.com/v1" {
			t.Errorf("got %q", got)
		}
	})
}

// ─── APIKeyProviderName ──────────────────────────────────────────────────────

func TestAPIKeyProviderName(t *testing.T) {
	t.Run("standard plan", func(t *testing.T) {
		cfg := map[string]interface{}{"plan": "standard", "region": "global"}
		got := APIKeyProviderName(cfg)
		want := "Alibaba DashScope Standard (global)"
		if got != want {
			t.Errorf("got %q, want %q", got, want)
		}
	})
	t.Run("coding plan", func(t *testing.T) {
		cfg := map[string]interface{}{"plan": "coding-plan", "region": "china"}
		got := APIKeyProviderName(cfg)
		want := "Alibaba Coding Plan (china)"
		if got != want {
			t.Errorf("got %q, want %q", got, want)
		}
	})
	t.Run("empty region defaults to global", func(t *testing.T) {
		cfg := map[string]interface{}{"plan": "standard"}
		got := APIKeyProviderName(cfg)
		want := "Alibaba DashScope Standard (global)"
		if got != want {
			t.Errorf("got %q, want %q", got, want)
		}
	})
}

// ─── Headers ─────────────────────────────────────────────────────────────────

func TestHeaders(t *testing.T) {
	t.Run("api-key headers", func(t *testing.T) {
		h := Headers("my-token", false, nil)
		if h["Authorization"] != "Bearer my-token" {
			t.Errorf("Authorization = %q", h["Authorization"])
		}
		if h["Content-Type"] != "application/json" {
			t.Errorf("Content-Type = %q", h["Content-Type"])
		}
	})
	t.Run("stream sets text/event-stream accept", func(t *testing.T) {
		h := Headers("tok", true, nil)
		if h["Accept"] != "text/event-stream" {
			t.Errorf("Accept = %q, want text/event-stream", h["Accept"])
		}
	})
}

// ─── GetModelsHardcoded ──────────────────────────────────────────────────────

func TestGetModelsHardcoded(t *testing.T) {
	resp := GetModelsHardcoded("alibaba-1")
	for _, m := range resp.Data {
		if m.Provider != "alibaba-1" {
			t.Errorf("model %q has provider %q, want alibaba-1", m.ID, m.Provider)
		}
		if strings.Contains(strings.ToLower(m.ID), "deepseek") {
			t.Errorf("hardcoded fallback should not include DeepSeek model %q — it must come from live API", m.ID)
		}
	}
	// Sanity: all Qwen models from the Models slice must be present in the fallback
	for _, meta := range Models {
		if strings.Contains(strings.ToLower(meta.ID), "deepseek") {
			continue
		}
		found := false
		for _, m := range resp.Data {
			if m.ID == meta.ID {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("expected Qwen model %q in hardcoded fallback but it was missing", meta.ID)
		}
	}
}

// ─── FetchModelsFromAPI ──────────────────────────────────────────────────────

func TestFetchModelsFromAPI(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/models" {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{
			"data": []map[string]interface{}{
				{"id": "qwen3-max"},
				{"id": "qwen-realtime-v1"}, // should be filtered out
				{"id": "unknown-model-xyz"},
			},
		})
	}))
	defer srv.Close()

	resp, err := FetchModelsFromAPI("alibaba-1", "test-token", srv.URL+"/v1", nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// qwen3-max kept, qwen-realtime-v1 filtered, unknown-model-xyz kept
	if len(resp.Data) != 2 {
		t.Errorf("got %d models, want 2; models: %v", len(resp.Data), modelIDs(resp.Data))
	}
}

func TestFetchModelsFromAPIError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
	}))
	defer srv.Close()

	_, err := FetchModelsFromAPI("alibaba-1", "bad-token", srv.URL+"/v1", nil)
	if err == nil {
		t.Error("expected error, got nil")
	}
}

// ─── SetupAPIKeyAuth ─────────────────────────────────────────────────────────

func TestSetupAPIKeyAuth(t *testing.T) {
	t.Run("requires API key", func(t *testing.T) {
		_, _, _, _, err := SetupAPIKeyAuth("alibaba-1", &types.AuthOptions{})
		if err == nil {
			t.Error("expected error for missing API key")
		}
	})
	t.Run("saves token and returns correct values", func(t *testing.T) {
		token, baseURL, name, cfg, err := SetupAPIKeyAuth("alibaba-test-1", &types.AuthOptions{
			APIKey: "sk-test-key",
			Region: "global",
			Plan:   "standard",
		})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if token != "sk-test-key" {
			t.Errorf("token = %q, want sk-test-key", token)
		}
		if baseURL != BaseURLGlobal {
			t.Errorf("baseURL = %q, want %q", baseURL, BaseURLGlobal)
		}
		if name != "Alibaba DashScope Standard (global)" {
			t.Errorf("name = %q", name)
		}
		if cfg["auth_type"] != "api-key" {
			t.Errorf("config auth_type = %v", cfg["auth_type"])
		}

		store := database.NewTokenStore()
		rec, err := store.Get("alibaba-test-1")
		if err != nil || rec == nil {
			t.Fatalf("token not persisted: err=%v", err)
		}
	})
}

// ─── IsReasoningModel ────────────────────────────────────────────────────────

func TestIsReasoningModel(t *testing.T) {
	cases := []struct {
		modelID string
		want    bool
	}{
		{"qwen3-max", true},
		{"qwen3-coder-plus", true},
		{"qwen3-235b-a22b-instruct", true},
		{"qwen-plus", true},
		{"qwen3.6-plus", true},
		{"deepseek-r1", true},
		{"deepseek-r1-0528", true},
		{"deepseek-v4-flash", true},
		{"QWEN3-MAX", true},
		{"glm-5.1", false},
		{"qwen2-5-72b-instruct", false},
		{"qwen-turbo", false},
		{"gpt-4o", false},
		{"", false},
	}
	for _, tc := range cases {
		got := IsReasoningModel(tc.modelID)
		if got != tc.want {
			t.Errorf("IsReasoningModel(%q) = %v, want %v", tc.modelID, got, tc.want)
		}
	}
}

func TestAlibabaBuildRequestDeepSeekV4ToolsDisablesThinkingAndOmitsToolChoice(t *testing.T) {
	p := NewProvider("alibaba-test-deepseek", "Alibaba Test")
	adapter := &Adapter{provider: p}

	for _, model := range []string{"deepseek-v4-flash", "alibaba-sk-ab2c5/deepseek-v4-flash"} {
		t.Run(model, func(t *testing.T) {
			req := &cif.CanonicalRequest{
				Model: model,
				Messages: []cif.CIFMessage{
					cif.CIFUserMessage{
						Role: "user",
						Content: []cif.CIFContentPart{
							cif.CIFTextPart{Type: "text", Text: "List files"},
						},
					},
				},
				Tools: []cif.CIFTool{{
					Name:             "ls",
					ParametersSchema: map[string]interface{}{"type": "object", "properties": map[string]interface{}{}},
				}},
				ToolChoice: "auto",
			}

			chatReq, err := adapter.buildRequest(req, false)
			if err != nil {
				t.Fatalf("buildRequest returned error: %v", err)
			}
			if chatReq.Model != "deepseek-v4-flash" {
				t.Fatalf("model = %q, want deepseek-v4-flash", chatReq.Model)
			}
			if chatReq.ToolChoice != nil {
				t.Fatalf("expected DeepSeek V4 upstream tool_choice to be omitted, got %#v", chatReq.ToolChoice)
			}
			if _, exists := chatReq.Extras["enable_thinking"]; exists {
				t.Fatalf("enable_thinking must not be sent for DeepSeek V4 tools: %#v", chatReq.Extras)
			}
			thinking, ok := chatReq.Extras["thinking"].(map[string]interface{})
			if !ok {
				t.Fatalf("expected thinking extra, got %#v", chatReq.Extras)
			}
			if thinking["type"] != "disabled" {
				t.Fatalf("expected thinking.type=disabled, got %#v", thinking)
			}
		})
	}
}

func TestAlibabaBuildRequestGLMToolsOmitsToolChoiceAndSetsEmptyContent(t *testing.T) {
	p := NewProvider("alibaba-test-glm", "Alibaba Test")
	adapter := &Adapter{provider: p}

	req := &cif.CanonicalRequest{
		Model: "alibaba-sk-ab2c5/glm-5.1",
		Messages: []cif.CIFMessage{
			cif.CIFAssistantMessage{
				Role: "assistant",
				Content: []cif.CIFContentPart{
					cif.CIFThinkingPart{Type: "thinking", Thinking: "hidden reasoning"},
					cif.CIFToolCallPart{
						Type:          "tool_call",
						ToolCallID:    "call_ls",
						ToolName:      "ls",
						ToolArguments: map[string]interface{}{"path": "."},
					},
				},
			},
			cif.CIFUserMessage{
				Role: "user",
				Content: []cif.CIFContentPart{
					cif.CIFTextPart{Type: "text", Text: "List files"},
				},
			},
		},
		Tools: []cif.CIFTool{{
			Name:             "ls",
			ParametersSchema: map[string]interface{}{"type": "object", "properties": map[string]interface{}{}},
		}},
		ToolChoice: "auto",
	}

	chatReq, err := adapter.buildRequest(req, false)
	if err != nil {
		t.Fatalf("buildRequest returned error: %v", err)
	}
	if chatReq.Model != "glm-5.1" {
		t.Fatalf("model = %q, want glm-5.1", chatReq.Model)
	}
	if chatReq.ToolChoice != nil {
		t.Fatalf("expected GLM upstream tool_choice to be omitted, got %#v", chatReq.ToolChoice)
	}
	if _, exists := chatReq.Extras["enable_thinking"]; exists {
		t.Fatalf("enable_thinking must not be sent for GLM tools, got %#v", chatReq.Extras)
	}
	if len(chatReq.Messages) == 0 {
		t.Fatalf("expected messages to be present")
	}
	if chatReq.Messages[0].ReasoningContent != "" {
		t.Fatalf("expected reasoning_content to be stripped for GLM, got %q", chatReq.Messages[0].ReasoningContent)
	}
	content, ok := chatReq.Messages[0].Content.(string)
	if !ok {
		t.Fatalf("expected assistant tool-only content to be empty string, got %#v (type %T)", chatReq.Messages[0].Content, chatReq.Messages[0].Content)
	}
	if content != "" {
		t.Fatalf("expected assistant tool-only content to be empty string, got %q", content)
	}
}

// ─── Qwen3.5-Plus tools ─────────────────────────────────────────────────────

func TestAlibabaBuildRequestQwen35PlusToolsHandling(t *testing.T) {
	p := NewProvider("alibaba-test-qwen35", "Alibaba Test")
	adapter := &Adapter{provider: p}

	for _, model := range []string{"qwen3.5-plus", "alibaba-sk-ab2c5/qwen3.5-plus"} {
		t.Run(model, func(t *testing.T) {
			req := &cif.CanonicalRequest{
				Model: model,
				Messages: []cif.CIFMessage{
					cif.CIFAssistantMessage{
						Role: "assistant",
						Content: []cif.CIFContentPart{
							cif.CIFThinkingPart{Type: "thinking", Thinking: "hidden reasoning"},
							cif.CIFToolCallPart{
								Type:          "tool_call",
								ToolCallID:    "call_ls",
								ToolName:      "ls",
								ToolArguments: map[string]interface{}{"path": "."},
							},
						},
					},
					cif.CIFUserMessage{
						Role: "user",
						Content: []cif.CIFContentPart{
							cif.CIFTextPart{Type: "text", Text: "List files"},
						},
					},
				},
				Tools: []cif.CIFTool{{
					Name:             "ls",
					ParametersSchema: map[string]interface{}{"type": "object", "properties": map[string]interface{}{}},
				}},
				ToolChoice: "auto",
			}

			chatReq, err := adapter.buildRequest(req, false)
			if err != nil {
				t.Fatalf("buildRequest returned error: %v", err)
			}
			if chatReq.Model != "qwen3.5-plus" {
				t.Fatalf("model = %q, want qwen3.5-plus", chatReq.Model)
			}
			if chatReq.ToolChoice != nil {
				t.Fatalf("expected qwen3.5-plus upstream tool_choice to be omitted, got %#v", chatReq.ToolChoice)
			}
			if _, exists := chatReq.Extras["enable_thinking"]; exists {
				t.Fatalf("enable_thinking must not be sent for non-reasoning tool models, got %#v", chatReq.Extras)
			}
			if chatReq.Messages[0].ReasoningContent != "" {
				t.Fatalf("expected reasoning_content to be stripped for qwen3.5-plus, got %q", chatReq.Messages[0].ReasoningContent)
			}
			content, ok := chatReq.Messages[0].Content.(string)
			if !ok {
				t.Fatalf("expected tool-only assistant content to be empty string, got %#v", chatReq.Messages[0].Content)
			}
			if content != "" {
				t.Fatalf("expected tool-only assistant content to be empty string, got %q", content)
			}
		})
	}
}

// ─── Non-reasoning model without tools ───────────────────────────────────────

func TestAlibabaBuildRequestNonReasoningModelWithoutToolsStillWorks(t *testing.T) {
	p := NewProvider("alibaba-test-glm", "Alibaba Test")
	adapter := &Adapter{provider: p}

	req := &cif.CanonicalRequest{
		Model: "glm-5.1",
		Messages: []cif.CIFMessage{
			cif.CIFUserMessage{
				Role: "user",
				Content: []cif.CIFContentPart{
					cif.CIFTextPart{Type: "text", Text: "Hello"},
				},
			},
		},
	}

	chatReq, err := adapter.buildRequest(req, false)
	if err != nil {
		t.Fatalf("buildRequest returned error: %v", err)
	}
	if _, exists := chatReq.Extras["enable_thinking"]; exists {
		t.Fatalf("enable_thinking must not be set when no tools, got %#v", chatReq.Extras)
	}
	if chatReq.ToolChoice != nil {
		t.Fatalf("tool_choice must be nil when not provided, got %#v", chatReq.ToolChoice)
	}
	if chatReq.Messages[0].Content != "Hello" {
		t.Fatalf("content = %#v, want Hello", chatReq.Messages[0].Content)
	}
}

// ─── isNonReasoningToolModel ─────────────────────────────────────────────────

func TestIsNonReasoningToolModel(t *testing.T) {
	cases := []struct {
		modelID string
		want    bool
	}{
		{"qwen3.5-plus", true},
		{"glm-5.1", true},
		{"QWEN3.5-PLUS", true},
		{"alibaba-sk-ab2c5/qwen3.5-plus", true},
		{"qwen3-max", false},
		{"qwen-turbo", false},
		{"deepseek-v4-flash", false},
		{"gpt-4o", false},
		{"", false},
	}
	for _, tc := range cases {
		got := isNonReasoningToolModel(tc.modelID)
		if got != tc.want {
			t.Errorf("isNonReasoningToolModel(%q) = %v, want %v", tc.modelID, got, tc.want)
		}
	}
}

// ─── ensureToolAssistantContent ──────────────────────────────────────────────

func TestEnsureToolAssistantContent(t *testing.T) {
	messages := []openaicompat.Message{
		{Role: "assistant", ToolCalls: []openaicompat.ToolCall{{ID: "call_1"}}},
		{Role: "assistant", Content: "Hello"},
		{Role: "user", Content: "Hi"},
	}
	ensureToolAssistantContent(messages)
	if messages[0].Content != "" {
		t.Fatalf("expected empty string content for tool-only assistant, got %#v", messages[0].Content)
	}
	if messages[1].Content != "Hello" {
		t.Fatalf("expected 'Hello' unchanged, got %#v", messages[1].Content)
	}
	if messages[2].Content != "Hi" {
		t.Fatalf("expected 'Hi' unchanged, got %#v", messages[2].Content)
	}
}

// ─── RemapModel ──────────────────────────────────────────────────────────────

func TestRemapModel(t *testing.T) {
	cases := []struct{ in, want string }{
		{"qwen3-max", "qwen3-max"},
		{"  qwen3-coder-plus  ", "qwen3-coder-plus"},
		{"alibaba-sk-ab2c5/deepseek-v4-flash", "deepseek-v4-flash"},
		{"  alibaba-sk-ab2c5/deepseek-v4-flash  ", "deepseek-v4-flash"},
	}
	for _, tc := range cases {
		if got := RemapModel(tc.in); got != tc.want {
			t.Errorf("RemapModel(%q) = %q, want %q", tc.in, got, tc.want)
		}
	}
}

func modelIDs(models []types.Model) []string {
	ids := make([]string, len(models))
	for i, m := range models {
		ids[i] = fmt.Sprintf("%s(%s)", m.ID, m.Name)
	}
	return ids
}
