package providerdispatch

import (
	"fmt"
	"omnillm/internal/cif"
	"omnillm/internal/lib/modelrouting"
)

type Attempt struct {
	RequestedModel  string
	NormalizedModel string
	ProviderID      string
}

type ResolveFunc func(requestedModel, normalizedModel, providerID string, cache *modelrouting.ModelCache) (*modelrouting.ResolvedModelRoute, error)

type PreparedCandidate struct {
	Candidate  *Candidate
	ProviderID string
}

func (e *Executor) PrepareCandidates(attempt Attempt, request *cif.CanonicalRequest, cache *modelrouting.ModelCache, resolve ResolveFunc) ([]PreparedCandidate, error) {
	if request == nil {
		return nil, fmt.Errorf("request is nil")
	}
	if resolve == nil {
		return nil, fmt.Errorf("resolve function is nil")
	}

	modelRoute, err := resolve(attempt.RequestedModel, attempt.NormalizedModel, attempt.ProviderID, cache)
	if err != nil {
		return nil, err
	}
	if len(modelRoute.CandidateProviders) == 0 {
		return nil, nil
	}

	attemptRequest := *request
	attemptRequest.Model = attempt.RequestedModel
	if attempt.ProviderID == "" && attempt.NormalizedModel != attemptRequest.Model {
		attemptRequest.Model = attempt.NormalizedModel
	}

	prepared := make([]PreparedCandidate, 0, len(modelRoute.CandidateProviders))
	for _, provider := range modelRoute.CandidateProviders {
		candidate, err := e.BuildCandidate(provider, &attemptRequest)
		if err != nil {
			continue
		}
		prepared = append(prepared, PreparedCandidate{
			Candidate:  candidate,
			ProviderID: provider.GetInstanceID(),
		})
	}

	return prepared, nil
}
