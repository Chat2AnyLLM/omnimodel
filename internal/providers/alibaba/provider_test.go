package alibaba

import (
	"omnillm/internal/database"
	"testing"
	"os"
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
