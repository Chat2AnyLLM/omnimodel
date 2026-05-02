package server

import (
	"context"
	"encoding/json"
	"net/http"
	"omnillm/internal/cif"
	"omnillm/internal/database"
	"testing"

	providertypes "omnillm/internal/providers/types"
)

func TestModelsEndpointIncludesEnabledVirtualModels(t *testing.T) {
	registerStubModelsProvider(t, []providertypes.Model{
		{
			ID:        "provider-model",
			Name:      "Provider Model",
			Provider:  "stub-provider",
			MaxTokens: 8192,
		},
	}, true)

	store := database.NewVirtualModelStore()
	if err := store.Create(&database.VirtualModelRecord{
		VirtualModelID: "virtualmodel-1",
		Name:           "Virtual Model",
		Description:    "Aggregated route",
		APIShape:       "responses",
		LbStrategy:     database.LbStrategyPriority,
		Enabled:        true,
	}); err != nil {
		t.Fatalf("create virtual model: %v", err)
	}

	srv := newTestServer(t)
	defer srv.Close()

	resp := getWithAuth(t, srv.URL+"/v1/models")
	body := readBody(t, resp)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", resp.StatusCode, body)
	}

	var payload struct {
		Data []map[string]interface{} `json:"data"`
	}
	if err := json.Unmarshal([]byte(body), &payload); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}

	foundProvider := false
	foundVirtual := false
	for _, model := range payload.Data {
		if model["id"] == "provider-model" {
			foundProvider = true
		}
		if model["id"] == "virtualmodel-1" {
			foundVirtual = true
			if model["owned_by"] != "virtual" {
				t.Fatalf("unexpected virtual model owner: %#v", model)
			}
			if model["api_shape"] != "responses" {
				t.Fatalf("unexpected virtual model api shape: %#v", model)
			}
		}
	}

	if !foundProvider {
		t.Fatal("expected provider model in response")
	}
	if !foundVirtual {
		t.Fatal("expected virtual model in response")
	}
}

func TestVirtualModelRoutingFallbackToSecondaryUpstream(t *testing.T) {
	// Register two upstream providers with different models
	p1 := registerStubProvider(
		t,
		"model-p1",
		func(_ context.Context, req *cif.CanonicalRequest) (*cif.CanonicalResponse, error) {
			return &cif.CanonicalResponse{
				ID:    "resp_p1",
				Model: req.Model,
				Content: []cif.CIFContentPart{
					cif.CIFTextPart{Type: "text", Text: "from p1"},
				},
				StopReason: cif.StopReasonEndTurn,
			}, nil
		},
		nil,
	)

	p2 := registerStubProvider(
		t,
		"model-p2",
		func(_ context.Context, req *cif.CanonicalRequest) (*cif.CanonicalResponse, error) {
			return &cif.CanonicalResponse{
				ID:    "resp_p2",
				Model: req.Model,
				Content: []cif.CIFContentPart{
					cif.CIFTextPart{Type: "text", Text: "from p2"},
				},
				StopReason: cif.StopReasonEndTurn,
			}, nil
		},
		nil,
	)

	// Create a virtual model with two upstreams in priority order: p1 (model-p1), p2 (model-p2)
	vmStore := database.NewVirtualModelStore()
	if err := vmStore.Create(&database.VirtualModelRecord{
		VirtualModelID: "priority-test",
		Name:           "Priority Test",
		APIShape:       "responses",
		LbStrategy:     database.LbStrategyPriority,
		Enabled:        true,
	}); err != nil {
		t.Fatalf("failed to create virtual model: %v", err)
	}

	upstreamStore := database.NewVirtualModelUpstreamStore()
	if err := upstreamStore.SetForVModel("priority-test", []database.VirtualModelUpstreamRecord{
		{VirtualModelID: "priority-test", ProviderID: p1, ModelID: "model-p1", Priority: 0},
		{VirtualModelID: "priority-test", ProviderID: p2, ModelID: "model-p2", Priority: 1},
	}); err != nil {
		t.Fatalf("failed to set virtual model upstreams: %v", err)
	}

	srv := newTestServer(t)
	defer srv.Close()

	// Route to the virtual model — should prefer p1
	resp := postJSON(
		t,
		srv.URL+"/v1/messages",
		`{"model":"priority-test","max_tokens":20,"messages":[{"role":"user","content":"test"}]}`,
		map[string]string{"anthropic-version": "2023-06-01"},
	)
	body := readBody(t, resp)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", resp.StatusCode, body)
	}

	var result struct {
		Content []struct {
			Type string `json:"type"`
			Text string `json:"text"`
		} `json:"content"`
	}
	if err := json.Unmarshal([]byte(body), &result); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}

	if len(result.Content) < 1 || result.Content[0].Text != "from p1" {
		t.Fatalf("expected response from p1 (primary), got: %#v", result.Content)
	}
}

func TestVirtualModelRoutingRoundRobinLoadBalancing(t *testing.T) {
	// Register two providers for round-robin testing
	p1 := registerStubProvider(
		t,
		"rr-model-1",
		func(_ context.Context, req *cif.CanonicalRequest) (*cif.CanonicalResponse, error) {
			return &cif.CanonicalResponse{
				ID:    "resp_rr_1",
				Model: req.Model,
				Content: []cif.CIFContentPart{
					cif.CIFTextPart{Type: "text", Text: "provider-1"},
				},
				StopReason: cif.StopReasonEndTurn,
			}, nil
		},
		nil,
	)

	p2 := registerStubProvider(
		t,
		"rr-model-2",
		func(_ context.Context, req *cif.CanonicalRequest) (*cif.CanonicalResponse, error) {
			return &cif.CanonicalResponse{
				ID:    "resp_rr_2",
				Model: req.Model,
				Content: []cif.CIFContentPart{
					cif.CIFTextPart{Type: "text", Text: "provider-2"},
				},
				StopReason: cif.StopReasonEndTurn,
			}, nil
		},
		nil,
	)

	// Create round-robin virtual model
	vmStore := database.NewVirtualModelStore()
	if err := vmStore.Create(&database.VirtualModelRecord{
		VirtualModelID: "round-robin-test",
		Name:           "Round Robin Test",
		APIShape:       "responses",
		LbStrategy:     database.LbStrategyRoundRobin,
		Enabled:        true,
	}); err != nil {
		t.Fatalf("failed to create virtual model: %v", err)
	}

	upstreamStore := database.NewVirtualModelUpstreamStore()
	if err := upstreamStore.SetForVModel("round-robin-test", []database.VirtualModelUpstreamRecord{
		{VirtualModelID: "round-robin-test", ProviderID: p1, ModelID: "rr-model-1", Priority: 0},
		{VirtualModelID: "round-robin-test", ProviderID: p2, ModelID: "rr-model-2", Priority: 0},
	}); err != nil {
		t.Fatalf("failed to set virtual model upstreams: %v", err)
	}

	srv := newTestServer(t)
	defer srv.Close()

	// Make three requests to the virtual model — should alternate between p1 and p2
	var providers []string
	for i := 0; i < 3; i++ {
		resp := postJSON(
			t,
			srv.URL+"/v1/messages",
			`{"model":"round-robin-test","max_tokens":20,"messages":[{"role":"user","content":"test"}]}`,
			map[string]string{"anthropic-version": "2023-06-01"},
		)
		body := readBody(t, resp)
		if resp.StatusCode != http.StatusOK {
			t.Fatalf("request %d: expected 200, got %d: %s", i, resp.StatusCode, body)
		}

		var result struct {
			Content []struct {
				Text string `json:"text"`
			} `json:"content"`
		}
		if err := json.Unmarshal([]byte(body), &result); err != nil {
			t.Fatalf("request %d: invalid JSON: %v", i, err)
		}
		if len(result.Content) < 1 {
			t.Fatalf("request %d: expected content, got none", i)
		}
		providers = append(providers, result.Content[0].Text)
	}

	// Verify round-robin pattern: should alternate between provider-1 and provider-2
	if len(providers) != 3 || providers[0] != "provider-1" || providers[1] != "provider-2" || providers[2] != "provider-1" {
		t.Fatalf("expected round-robin pattern [provider-1, provider-2, provider-1], got %v", providers)
	}
}
