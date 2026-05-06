package alibaba

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"omnillm/internal/database"
	"omnillm/internal/providers/types"
	"strings"
	"testing"
)

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

func TestFetchModelsFromAPI(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/models" {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"data":[{"id":"qwen3-max"},{"id":"qwen-realtime-v1"},{"id":"unknown-model-xyz"}]}`))
	}))
	defer srv.Close()

	resp, err := FetchModelsFromAPI("alibaba-1", "test-token", srv.URL+"/v1", nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
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

func modelIDs(models []types.Model) []string {
	ids := make([]string, len(models))
	for i, m := range models {
		ids[i] = fmt.Sprintf("%s(%s)", m.ID, m.Name)
	}
	return ids
}
