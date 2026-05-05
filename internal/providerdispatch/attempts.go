package providerdispatch

import (
	"fmt"
	"omnillm/internal/cif"
	"omnillm/internal/lib/modelrouting"
)

type CandidateHandler func(candidate *Candidate, providerID string) error

type AttemptErrorHandler func(attempt Attempt, err error)

type AttemptEmptyHandler func(attempt Attempt)

func (e *Executor) TryAttempts(attempts []Attempt, request *cif.CanonicalRequest, cache *modelrouting.ModelCache, resolve ResolveFunc, onEmpty AttemptEmptyHandler, onError AttemptErrorHandler, handle CandidateHandler) error {
	var lastErr error

	for _, attempt := range attempts {
		prepared, err := e.PrepareCandidates(attempt, request, cache, resolve)
		if err != nil {
			if onError != nil {
				onError(attempt, err)
			}
			return err
		}

		if len(prepared) == 0 {
			lastErr = fmt.Errorf("model '%s' not found or no providers available", attempt.RequestedModel)
			if onEmpty != nil {
				onEmpty(attempt)
			}
			continue
		}

		for _, preparedCandidate := range prepared {
			candidate := preparedCandidate.Candidate
			providerID := preparedCandidate.ProviderID
			lastErr = handle(candidate, providerID)
			if lastErr == nil {
				return nil
			}
		}
	}

	return lastErr
}
