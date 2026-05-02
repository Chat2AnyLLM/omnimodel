package routes

import (
	"strings"

	"omnillm/internal/database"
	"omnillm/internal/lib/modelrouting"
	"omnillm/internal/lib/virtualmodelrouting"

	"github.com/rs/zerolog/log"
)

type resolvedModelAttempt struct {
	RequestedModel  string
	NormalizedModel string
	ProviderID      string // non-empty when resolved from a virtual model upstream with a specific provider
}

func resolveRequestedModel(requestID, requestedModel string) (string, string) {
	attempts := resolveRequestedModels(requestID, requestedModel)
	if len(attempts) == 0 {
		normalizedModel := modelrouting.NormalizeModelName(requestedModel)
		return requestedModel, normalizedModel
	}
	return attempts[0].RequestedModel, attempts[0].NormalizedModel
}

func resolveRequestedModels(requestID, requestedModel string) []resolvedModelAttempt {
	// Strip optional "<instanceID>/<modelID>" prefix before any further resolution.
	// When a prefix is present the request is pinned to that specific provider.
	prefixProviderID, bareModel := modelrouting.ParseProviderPrefix(requestedModel)
	if prefixProviderID != "" {
		// Resolve the prefix: first try it as a literal instance ID, then fall
		// back to matching against provider subtitles (the user-visible short
		// label shown in the UI, e.g. "alipay01").
		resolvedInstanceID := resolveProviderPrefix(prefixProviderID)
		log.Debug().
			Str("request_id", requestID).
			Str("provider_prefix", prefixProviderID).
			Str("resolved_instance_id", resolvedInstanceID).
			Str("model", bareModel).
			Msg("Provider prefix detected in model name")
		normalizedModel := modelrouting.NormalizeModelName(bareModel)
		return []resolvedModelAttempt{{
			RequestedModel:  bareModel,
			NormalizedModel: normalizedModel,
			ProviderID:      resolvedInstanceID,
		}}
	}

	normalizedModel := modelrouting.NormalizeModelName(requestedModel)

	vmodelStore := database.NewVirtualModelStore()
	vm, err := vmodelStore.Get(requestedModel)
	if err != nil {
		log.Warn().Err(err).Str("request_id", requestID).Str("model", requestedModel).Str("normalized_model", normalizedModel).Msg("Failed to load virtual model")
		return []resolvedModelAttempt{{RequestedModel: requestedModel, NormalizedModel: normalizedModel}}
	}
	if vm == nil && normalizedModel != requestedModel {
		vm, err = vmodelStore.Get(normalizedModel)
		if err != nil {
			log.Warn().Err(err).Str("request_id", requestID).Str("model", requestedModel).Str("normalized_model", normalizedModel).Msg("Failed to load normalized virtual model")
			return []resolvedModelAttempt{{RequestedModel: requestedModel, NormalizedModel: normalizedModel}}
		}
	}
	if vm == nil || !vm.Enabled {
		return []resolvedModelAttempt{{RequestedModel: requestedModel, NormalizedModel: normalizedModel}}
	}

	upstreamStore := database.NewVirtualModelUpstreamStore()
	upstreams, err := upstreamStore.GetForVModel(vm.VirtualModelID)
	if err != nil {
		log.Warn().Err(err).Str("request_id", requestID).Str("virtual_model", vm.VirtualModelID).Msg("Failed to load virtual model upstreams")
		return []resolvedModelAttempt{{RequestedModel: requestedModel, NormalizedModel: normalizedModel}}
	}

	ordered := virtualmodelrouting.OrderUpstreams(upstreams, vm.LbStrategy, vm.VirtualModelID)
	if len(ordered) == 0 {
		log.Warn().Str("request_id", requestID).Str("virtual_model", vm.VirtualModelID).Msg("Virtual model has no routable upstream")
		return []resolvedModelAttempt{{RequestedModel: requestedModel, NormalizedModel: normalizedModel}}
	}

	attempts := make([]resolvedModelAttempt, 0, len(ordered))
	for _, upstream := range ordered {
		upstreamPrefix, bareUpstreamModel := modelrouting.ParseProviderPrefix(upstream.ModelID)
		executionModel := upstream.ModelID
		providerID := upstream.ProviderID
		if upstreamPrefix != "" {
			executionModel = bareUpstreamModel
			if providerID == "" {
				providerID = resolveProviderPrefix(upstreamPrefix)
			}
		}

		log.Debug().
			Str("request_id", requestID).
			Str("virtual_model", vm.VirtualModelID).
			Str("upstream", upstream.ModelID).
			Str("execution_model", executionModel).
			Str("provider", providerID).
			Str("strategy", string(vm.LbStrategy)).
			Msg("Virtual model routing candidate")
		attempts = append(attempts, resolvedModelAttempt{
			RequestedModel:  executionModel,
			NormalizedModel: modelrouting.NormalizeModelName(executionModel),
			ProviderID:      providerID,
		})
	}

	return attempts
}

// resolveProviderPrefix maps a user-supplied prefix to a registry instance ID.
//
// The lookup order is:
//  1. Exact match against a registered instance ID (e.g. "alibaba-2").
//  2. Case-insensitive match against the provider's subtitle — the short label
//     users set in the UI (e.g. "alipay01").
//
// If neither matches, the original prefix is returned unchanged so the caller
// can produce a meaningful "provider not found" error downstream.
func resolveProviderPrefix(prefix string) string {
	instanceStore := database.NewProviderInstanceStore()
	instances, err := instanceStore.GetAll()
	if err != nil {
		return prefix
	}

	lowerPrefix := strings.ToLower(prefix)

	var subtitleMatch string
	for _, inst := range instances {
		// 1. Exact instance ID match — highest priority.
		if inst.InstanceID == prefix {
			return prefix
		}
		// 2. Case-insensitive subtitle match — collect first hit.
		if subtitleMatch == "" && strings.ToLower(inst.Subtitle) == lowerPrefix {
			subtitleMatch = inst.InstanceID
		}
	}

	if subtitleMatch != "" {
		return subtitleMatch
	}
	return prefix
}
