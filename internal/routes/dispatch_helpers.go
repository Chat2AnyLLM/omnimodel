package routes

import "omnillm/internal/providerdispatch"

func toDispatchAttempts(attempts []resolvedModelAttempt) []providerdispatch.Attempt {
	out := make([]providerdispatch.Attempt, 0, len(attempts))
	for _, attempt := range attempts {
		out = append(out, providerdispatch.Attempt{
			RequestedModel:  attempt.RequestedModel,
			NormalizedModel: attempt.NormalizedModel,
			ProviderID:      attempt.ProviderID,
		})
	}
	return out
}
