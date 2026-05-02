// Package virtualmodelrouting implements load-balancing strategies for virtual models.
package virtualmodelrouting

import (
	// math/rand/v2 (Go 1.22+) uses a lock-free per-goroutine PCG source,
	// eliminating the global mutex contention of math/rand under concurrent load.
	"math/rand/v2"
	"omnillm/internal/database"
	"sync"
)

// roundRobinState holds the per-virtual-model cursor for round-robin selection.
var (
	rrMu    sync.Mutex
	rrState = make(map[string]int)
)

// SelectUpstream picks one upstream from the list according to the given
// load-balancing strategy. Returns nil when the upstream list is empty.
func SelectUpstream(
	upstreams []database.VirtualModelUpstreamRecord,
	strategy database.LbStrategy,
	virtualModelID string,
) *database.VirtualModelUpstreamRecord {
	ordered := OrderUpstreams(upstreams, strategy, virtualModelID)
	if len(ordered) == 0 {
		return nil
	}
	return &ordered[0]
}

// OrderUpstreams returns all upstreams in the order they should be attempted.
func OrderUpstreams(
	upstreams []database.VirtualModelUpstreamRecord,
	strategy database.LbStrategy,
	virtualModelID string,
) []database.VirtualModelUpstreamRecord {
	if len(upstreams) == 0 {
		return nil
	}

	ordered := append([]database.VirtualModelUpstreamRecord(nil), upstreams...)

	switch strategy {
	case database.LbStrategyRoundRobin:
		rrMu.Lock()
		idx := rrState[virtualModelID] % len(ordered)
		rrState[virtualModelID] = idx + 1
		rrMu.Unlock()
		return append(ordered[idx:], ordered[:idx]...)

	case database.LbStrategyRandom:
		idx := rand.IntN(len(ordered))
		return moveToFront(ordered, idx)

	case database.LbStrategyPriority:
		return ordered

	case database.LbStrategyWeighted:
		totalWeight := 0
		for _, u := range ordered {
			w := u.Weight
			if w < 1 {
				w = 1
			}
			totalWeight += w
		}
		roll := rand.IntN(totalWeight)
		for i, u := range ordered {
			w := u.Weight
			if w < 1 {
				w = 1
			}
			roll -= w
			if roll < 0 {
				return moveToFront(ordered, i)
			}
		}
		return moveToFront(ordered, len(ordered)-1)

	default:
		return ordered
	}
}

func moveToFront(upstreams []database.VirtualModelUpstreamRecord, idx int) []database.VirtualModelUpstreamRecord {
	if idx <= 0 || idx >= len(upstreams) {
		return upstreams
	}

	ordered := make([]database.VirtualModelUpstreamRecord, 0, len(upstreams))
	ordered = append(ordered, upstreams[idx])
	ordered = append(ordered, upstreams[:idx]...)
	ordered = append(ordered, upstreams[idx+1:]...)
	return ordered
}

func MigrateRoundRobinCursor(oldID, newID string) {
	rrMu.Lock()
	defer rrMu.Unlock()

	if cursor, exists := rrState[oldID]; exists {
		rrState[newID] = cursor
		delete(rrState, oldID)
	}
}
