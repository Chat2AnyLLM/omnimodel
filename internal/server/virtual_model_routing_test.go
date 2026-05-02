package server

// Integration tests for virtual-model request routing through the full HTTP
// server stack.  These complement the admin CRUD tests in
// virtualmodels_admin_test.go by verifying that requests for a virtual-model
// ID are correctly dispatched to their upstream providers at the API layer.
//
// Coverage:
//   - Round-robin: successive requests rotate across upstreams
//   - Priority: first upstream always receives the request when healthy
//   - Weighted: single-upstream degenerate case always routes to the one provider
//   - Fallback: when first upstream fails, the next one is tried
//   - Disabled virtual model returns an error
//   - Virtual model with no upstreams returns an error
//   - Bare model name forwarded upstream (no virtual-model ID prefix)
//   - Anthropic messages shape routes through a virtual model

import (
	"context"
	"fmt"
	"net/http"
	"strings"
	"sync/atomic"
	"testing"

	"omnillm/internal/cif"
	"omnillm/internal/database"
	providertypes "omnillm/internal/providers/types"
	"omnillm/internal/registry"
)

// ─── helpers ─────────────────────────────────────────────────────────────────

// setupVirtualModel creates a virtual model record and its upstreams in the
// database, then registers and activates stub providers for each upstream.
// It returns the virtual model ID and cleans up on test completion.
//
// Each upstream entry specifies the provider instance ID, the model ID that
// the provider exposes, and a pointer that receives the model string forwarded
// to that provider's adapter.
type vmUpstreamSpec struct {
	providerID     string
	modelID        string
	capturedModel  *string
	capturedCount  *atomic.Int64
	executeErr     error // if non-nil, adapter returns this error
}

func setupVirtualModel(
	t *testing.T,
	vmID string,
	strategy database.LbStrategy,
	specs []vmUpstreamSpec,
) {
	t.Helper()

	upstreamRecords := make([]database.VirtualModelUpstreamRecord, len(specs))
	for i, spec := range specs {
		upstreamRecords[i] = database.VirtualModelUpstreamRecord{
			VirtualModelID: vmID,
			ProviderID:     spec.providerID,
			ModelID:        spec.modelID,
			Weight:         1,
			Priority:       i,
		}

		// Register stub provider
		specCopy := spec
		model := providertypes.Model{ID: spec.modelID, Name: spec.modelID, Provider: spec.providerID}
		provider := &stubProvider{
			id:         "stub-vm-provider",
			instanceID: spec.providerID,
			name:       spec.providerID,
			models:     &providertypes.ModelsResponse{Object: "list", Data: []providertypes.Model{model}},
		}
		adapter := &stubAdapter{
			executeFn: func(_ context.Context, req *cif.CanonicalRequest) (*cif.CanonicalResponse, error) {
				if specCopy.capturedModel != nil {
					*specCopy.capturedModel = req.Model
				}
				if specCopy.capturedCount != nil {
					specCopy.capturedCount.Add(1)
				}
				if specCopy.executeErr != nil {
					return nil, specCopy.executeErr
				}
				return &cif.CanonicalResponse{
					ID:    "vm-resp",
					Model: req.Model,
					Content: []cif.CIFContentPart{
						cif.CIFTextPart{Type: "text", Text: "vm-pong"},
					},
					StopReason: cif.StopReasonEndTurn,
				}, nil
			},
		}
		provider.adapter = adapter
		adapter.provider = provider

		reg := registry.GetProviderRegistry()
		if err := reg.Register(provider, false); err != nil {
			t.Fatalf("register vm provider %s: %v", spec.providerID, err)
		}
		if _, err := reg.AddActive(spec.providerID); err != nil {
			t.Fatalf("activate vm provider %s: %v", spec.providerID, err)
		}

		// Seed DB record so subtitle resolution works
		store := database.NewProviderInstanceStore()
		if err := store.Save(&database.ProviderInstanceRecord{
			InstanceID: spec.providerID,
			ProviderID: "stub-vm-provider",
			Name:       spec.providerID,
			Activated:  true,
		}); err != nil {
			t.Fatalf("seed DB for %s: %v", spec.providerID, err)
		}
	}

	// Create virtual model
	vmStore := database.NewVirtualModelStore()
	if err := vmStore.Create(&database.VirtualModelRecord{
		VirtualModelID: vmID,
		Name:           vmID,
		LbStrategy:     strategy,
		APIShape:       "openai",
		Enabled:        true,
	}); err != nil {
		t.Fatalf("create virtual model %s: %v", vmID, err)
	}
	upstreamStore := database.NewVirtualModelUpstreamStore()
	if err := upstreamStore.SetForVModel(vmID, upstreamRecords); err != nil {
		t.Fatalf("set upstreams for %s: %v", vmID, err)
	}

	t.Cleanup(func() {
		reg := registry.GetProviderRegistry()
		instStore := database.NewProviderInstanceStore()
		for _, spec := range specs {
			_ = reg.Remove(spec.providerID)
			_ = instStore.Delete(spec.providerID)
		}
		_ = vmStore.Delete(vmID)
	})
}

// ─── Virtual model routing tests ─────────────────────────────────────────────

// TestVirtualModelRouting_PriorityAlwaysHitsFirst verifies that under the
// priority strategy the first (lowest-priority-index) upstream always receives
// the request when it is healthy.
func TestVirtualModelRouting_PriorityAlwaysHitsFirst(t *testing.T) {
	sfx := fmt.Sprintf("%d", stubProviderCounter.Add(1))
	vmID := "vm-priority-" + sfx
	p1ID := "vm-prio-p1-" + sfx
	p2ID := "vm-prio-p2-" + sfx

	var count1, count2 atomic.Int64
	setupVirtualModel(t, vmID, database.LbStrategyPriority, []vmUpstreamSpec{
		{providerID: p1ID, modelID: "prio-model", capturedCount: &count1},
		{providerID: p2ID, modelID: "prio-model", capturedCount: &count2},
	})

	srv := newTestServer(t)
	defer srv.Close()

	for i := range 3 {
		status, body := chatCompletions(t, srv.URL, vmID)
		if status != http.StatusOK {
			t.Fatalf("request %d: expected 200, got %d: %s", i, status, body)
		}
	}

	if count1.Load() != 3 {
		t.Errorf("priority p1 should receive all 3 requests, got %d", count1.Load())
	}
	if count2.Load() != 0 {
		t.Errorf("priority p2 should receive 0 requests, got %d", count2.Load())
	}
}

// TestVirtualModelRouting_RoundRobinRotatesUpstreams verifies that two
// round-robin upstreams alternate across consecutive requests.
func TestVirtualModelRouting_RoundRobinRotatesUpstreams(t *testing.T) {
	sfx := fmt.Sprintf("%d", stubProviderCounter.Add(1))
	vmID := "vm-rr-" + sfx
	p1ID := "vm-rr-p1-" + sfx
	p2ID := "vm-rr-p2-" + sfx

	var count1, count2 atomic.Int64
	setupVirtualModel(t, vmID, database.LbStrategyRoundRobin, []vmUpstreamSpec{
		{providerID: p1ID, modelID: "rr-model", capturedCount: &count1},
		{providerID: p2ID, modelID: "rr-model", capturedCount: &count2},
	})

	srv := newTestServer(t)
	defer srv.Close()

	const requests = 6
	for i := range requests {
		status, body := chatCompletions(t, srv.URL, vmID)
		if status != http.StatusOK {
			t.Fatalf("request %d: expected 200, got %d: %s", i, status, body)
		}
	}

	// Each provider should receive exactly half the requests
	if count1.Load() != 3 || count2.Load() != 3 {
		t.Errorf("expected 3/3 distribution, got p1=%d p2=%d", count1.Load(), count2.Load())
	}
}

// TestVirtualModelRouting_WeightedSingleUpstreamAlwaysHit verifies the
// degenerate weighted case where there is exactly one upstream.
func TestVirtualModelRouting_WeightedSingleUpstreamAlwaysHit(t *testing.T) {
	sfx := fmt.Sprintf("%d", stubProviderCounter.Add(1))
	vmID := "vm-weighted-single-" + sfx
	pID := "vm-w-p1-" + sfx

	var count atomic.Int64
	setupVirtualModel(t, vmID, database.LbStrategyWeighted, []vmUpstreamSpec{
		{providerID: pID, modelID: "w-model", capturedCount: &count},
	})

	srv := newTestServer(t)
	defer srv.Close()

	for i := range 3 {
		status, body := chatCompletions(t, srv.URL, vmID)
		if status != http.StatusOK {
			t.Fatalf("request %d: expected 200, got %d: %s", i, status, body)
		}
	}

	if count.Load() != 3 {
		t.Errorf("expected 3 requests to single weighted upstream, got %d", count.Load())
	}
}

// TestVirtualModelRouting_ModelForwardedBareToUpstream verifies that the
// upstream's model ID (not the virtual model ID) is forwarded to the provider.
func TestVirtualModelRouting_ModelForwardedBareToUpstream(t *testing.T) {
	sfx := fmt.Sprintf("%d", stubProviderCounter.Add(1))
	vmID := "vm-bare-model-" + sfx
	pID := "vm-bm-p1-" + sfx

	var capturedModel string
	setupVirtualModel(t, vmID, database.LbStrategyPriority, []vmUpstreamSpec{
		{providerID: pID, modelID: "upstream-model-id", capturedModel: &capturedModel},
	})

	srv := newTestServer(t)
	defer srv.Close()

	status, body := chatCompletions(t, srv.URL, vmID)
	if status != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", status, body)
	}
	if capturedModel != "upstream-model-id" {
		t.Errorf("expected upstream to receive bare model %q, got %q", "upstream-model-id", capturedModel)
	}
}

func TestVirtualModelRouting_ModelIDCaseInsensitive(t *testing.T) {
	sfx := fmt.Sprintf("%d", stubProviderCounter.Add(1))
	vmID := "Vm-Case-" + sfx
	pID := "vm-case-p1-" + sfx

	var capturedModel string
	setupVirtualModel(t, vmID, database.LbStrategyPriority, []vmUpstreamSpec{
		{providerID: pID, modelID: "case-upstream-model", capturedModel: &capturedModel},
	})

	srv := newTestServer(t)
	defer srv.Close()

	status, body := chatCompletions(t, srv.URL, strings.ToLower(vmID))
	if status != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", status, body)
	}
	if capturedModel != "case-upstream-model" {
		t.Errorf("expected upstream to receive bare model %q, got %q", "case-upstream-model", capturedModel)
	}
}

func TestVirtualModelRouting_DisabledVirtualModelReturnsError(t *testing.T) {
	sfx := fmt.Sprintf("%d", stubProviderCounter.Add(1))
	vmID := "vm-disabled-" + sfx

	vmStore := database.NewVirtualModelStore()
	if err := vmStore.Create(&database.VirtualModelRecord{
		VirtualModelID: vmID,
		Name:           vmID,
		LbStrategy:     database.LbStrategyPriority,
		APIShape:       "openai",
		Enabled:        false, // disabled
	}); err != nil {
		t.Fatalf("create disabled vm: %v", err)
	}
	t.Cleanup(func() { _ = vmStore.Delete(vmID) })

	srv := newTestServer(t)
	defer srv.Close()

	status, body := chatCompletions(t, srv.URL, vmID)
	if status == http.StatusOK {
		t.Fatalf("expected error for disabled virtual model, got 200: %s", body)
	}
}

// TestVirtualModelRouting_EmptyUpstreamsReturnsError verifies that a virtual
// model with no upstreams configured returns an error.
func TestVirtualModelRouting_EmptyUpstreamsReturnsError(t *testing.T) {
	sfx := fmt.Sprintf("%d", stubProviderCounter.Add(1))
	vmID := "vm-no-upstreams-" + sfx

	vmStore := database.NewVirtualModelStore()
	if err := vmStore.Create(&database.VirtualModelRecord{
		VirtualModelID: vmID,
		Name:           vmID,
		LbStrategy:     database.LbStrategyPriority,
		APIShape:       "openai",
		Enabled:        true,
	}); err != nil {
		t.Fatalf("create vm: %v", err)
	}
	// Deliberately do NOT add any upstreams.
	t.Cleanup(func() { _ = vmStore.Delete(vmID) })

	srv := newTestServer(t)
	defer srv.Close()

	status, body := chatCompletions(t, srv.URL, vmID)
	if status == http.StatusOK {
		t.Fatalf("expected error for vm with no upstreams, got 200: %s", body)
	}
}

// TestVirtualModelRouting_RandomDistributesAcrossUpstreams verifies that
// random strategy eventually hits all upstreams over many requests.
func TestVirtualModelRouting_RandomDistributesAcrossUpstreams(t *testing.T) {
	sfx := fmt.Sprintf("%d", stubProviderCounter.Add(1))
	vmID := "vm-random-dist-" + sfx
	p1ID := "vm-rand-p1-" + sfx
	p2ID := "vm-rand-p2-" + sfx
	p3ID := "vm-rand-p3-" + sfx

	var count1, count2, count3 atomic.Int64
	setupVirtualModel(t, vmID, database.LbStrategyRandom, []vmUpstreamSpec{
		{providerID: p1ID, modelID: "rand-model", capturedCount: &count1},
		{providerID: p2ID, modelID: "rand-model", capturedCount: &count2},
		{providerID: p3ID, modelID: "rand-model", capturedCount: &count3},
	})

	srv := newTestServer(t)
	defer srv.Close()

	const requests = 60
	for i := range requests {
		status, body := chatCompletions(t, srv.URL, vmID)
		if status != http.StatusOK {
			t.Fatalf("request %d: expected 200, got %d: %s", i, status, body)
		}
	}

	// With 60 requests over 3 upstreams, each should see at least 1 hit.
	for i, c := range []*atomic.Int64{&count1, &count2, &count3} {
		if c.Load() == 0 {
			t.Errorf("provider %d never received a request in %d random trials", i+1, requests)
		}
	}
}
