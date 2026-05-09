package copilot

import (
	"net/http"
	"net/http/httptest"
	"testing"
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
