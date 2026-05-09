package copilot

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"omnillm/internal/cif"
)

func TestGetModels_PopulatesShapeCache(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/models" {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"object":"list","data":[
						{"id":"gpt-5.4","name":"GPT-5.4","capabilities":{},"supported_endpoints":["/responses","ws:/responses"]},
						{"id":"claude-opus-4.7","name":"Claude Opus 4.7","capabilities":{},"supported_endpoints":["/v1/messages","/chat/completions"]},
						{"id":"gpt-4o","name":"GPT-4o","capabilities":{},"supported_endpoints":["/chat/completions"]}
					]}`))
	}))
	defer server.Close()

	provider := NewGitHubCopilotProvider("test", "")
	provider.baseURL = server.URL
	provider.token = "test-token"

	_, err := provider.GetModels()
	if err != nil {
		t.Fatalf("GetModels returned error: %v", err)
	}

	if provider.shapeCache == nil {
		t.Fatal("expected shapeCache to be populated after GetModels")
	}
	if got := provider.shapeCache["gpt-5.4"]; got != shapeResponses {
		t.Errorf("gpt-5.4: expected shapeResponses, got %q", got)
	}
	if got := provider.shapeCache["claude-opus-4.7"]; got != shapeChat {
		t.Errorf("claude-opus-4.7: expected shapeChat, got %q", got)
	}
	if got := provider.shapeCache["gpt-4o"]; got != shapeChat {
		t.Errorf("gpt-4o: expected shapeChat, got %q", got)
	}
}

func TestGetModels_ShapeCacheRemainsNilOnServerError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte(`{"error":"internal server error"}`))
	}))
	defer server.Close()

	provider := NewGitHubCopilotProvider("test", "")
	provider.baseURL = server.URL
	provider.token = "test-token"

	_, _ = provider.GetModels() // error is expected; ignore it

	if provider.shapeCache != nil {
		t.Errorf("expected shapeCache to remain nil on server error, got %v", provider.shapeCache)
	}
}

func TestSelectShape_CacheHit(t *testing.T) {
	provider := NewGitHubCopilotProvider("test", "")
	provider.shapeCache = modelShapeCache{
		"gpt-5.4":         shapeResponses,
		"claude-opus-4.7": shapeChat,
	}
	adapter := provider.GetAdapter().(*CopilotAdapter)

	if got := adapter.selectShape("gpt-5.4", nil); got != shapeResponses {
		t.Errorf("expected shapeResponses for gpt-5.4, got %q", got)
	}
	if got := adapter.selectShape("claude-opus-4.7", nil); got != shapeChat {
		t.Errorf("expected shapeChat for claude-opus-4.7, got %q", got)
	}
}

func TestSelectShape_ForceChatCompletionsOverridesCache(t *testing.T) {
	provider := NewGitHubCopilotProvider("test", "")
	provider.shapeCache = modelShapeCache{"gpt-5.4": shapeResponses}
	adapter := provider.GetAdapter().(*CopilotAdapter)

	force := true
	req := &cif.CanonicalRequest{Extensions: &cif.Extensions{ForceChatCompletions: &force}}
	if got := adapter.selectShape("gpt-5.4", req); got != shapeChat {
		t.Errorf("expected shapeChat when ForceChatCompletions=true, got %q", got)
	}
}

func TestSelectShape_CacheMissFallsBackToHeuristic(t *testing.T) {
	provider := NewGitHubCopilotProvider("test", "")
	// shapeCache is nil — simulates pre-GetModels state
	adapter := provider.GetAdapter().(*CopilotAdapter)

	if got := adapter.selectShape("gpt-5.5", nil); got != shapeResponses {
		t.Errorf("expected shapeResponses for gpt-5.5 heuristic fallback, got %q", got)
	}
	if got := adapter.selectShape("claude-opus-4.7", nil); got != shapeChat {
		t.Errorf("expected shapeChat for claude fallback, got %q", got)
	}
}

func TestSelectShape_GPT5MiniDoesNotUseResponses(t *testing.T) {
	provider := NewGitHubCopilotProvider("test", "")
	adapter := provider.GetAdapter().(*CopilotAdapter)

	if got := adapter.selectShape("gpt-5-mini", nil); got != shapeChat {
		t.Errorf("expected shapeChat for gpt-5-mini, got %q", got)
	}
}

func TestCopilotAdapter_ShapeCacheDrivesRouting(t *testing.T) {
	// Verify that a model explicitly listed as responses-only in the cache
	// routes to /responses, and a model listed as chat routes to /chat/completions,
	// regardless of the model name's GPT-5 heuristic.

	cases := []struct {
		name         string
		model        string
		shape        copilotAPIShape
		expectedPath string
		serverResp   string
	}{
		{
			name:         "cache says responses",
			model:        "some-future-model",
			shape:        shapeResponses,
			expectedPath: "/responses",
			serverResp:   `{"id":"resp_cache","model":"some-future-model","status":"completed","output":[{"type":"message","id":"m1","role":"assistant","content":[{"type":"output_text","text":"ok"}]}],"usage":{"input_tokens":1,"output_tokens":1}}`,
		},
		{
			name:         "cache says chat",
			model:        "gpt-5.5", // would normally route to /responses by heuristic
			shape:        shapeChat,
			expectedPath: "/chat/completions",
			serverResp: func() string {
				b, _ := json.Marshal(map[string]interface{}{
					"id":    "chatcmpl_cache",
					"model": "gpt-5.5",
					"choices": []map[string]interface{}{{
						"index":         0,
						"message":       map[string]interface{}{"role": "assistant", "content": "ok"},
						"finish_reason": "stop",
					}},
				})
				return string(b)
			}(),
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var capturedPath string
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				capturedPath = r.URL.Path
				w.Header().Set("Content-Type", "application/json")
				_, _ = w.Write([]byte(tc.serverResp))
			}))
			defer server.Close()

			provider := NewGitHubCopilotProvider("test", "")
			provider.baseURL = server.URL
			provider.token = "test-token"
			provider.shapeCache = modelShapeCache{tc.model: tc.shape}
			adapter := provider.GetAdapter().(*CopilotAdapter)

			_, err := adapter.Execute(context.Background(), &cif.CanonicalRequest{
				Model: tc.model,
				Messages: []cif.CIFMessage{
					cif.CIFUserMessage{Role: "user", Content: []cif.CIFContentPart{cif.CIFTextPart{Type: "text", Text: "ping"}}},
				},
			})
			if err != nil {
				t.Fatalf("Execute returned error: %v", err)
			}
			if capturedPath != tc.expectedPath {
				t.Errorf("expected path %q, got %q", tc.expectedPath, capturedPath)
			}
		})
	}
}
