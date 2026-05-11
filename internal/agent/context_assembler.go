package agent

// ContextAssembler assembles a message slice into a token-budget-respecting
// context for each model dispatch.
//
// Priority rules (lowest priority is dropped first when over budget):
//
//	Priority 0 — system messages           (never dropped)
//	Priority 1 — most recent N turns       (always kept if budget allows)
//	Priority 2 — older conversation turns  (trimmed first)
//
// The assembler keeps system messages whole, then fills the remaining budget
// with conversation messages from newest to oldest.
type ContextAssembler struct {
	// TokenBudget is the maximum estimated tokens for the assembled slice.
	// Estimation uses word-count × 1.3 (same heuristic as estimateMessageTokens).
	TokenBudget int
	// MinRecentTurns is the minimum number of conversation messages always kept
	// regardless of budget (guards against over-aggressive trimming).
	MinRecentTurns int
}

// NewContextAssembler returns a ContextAssembler with sensible defaults.
func NewContextAssembler(tokenBudget int) *ContextAssembler {
	if tokenBudget <= 0 {
		tokenBudget = contextTokenBudget
	}
	return &ContextAssembler{
		TokenBudget:    tokenBudget,
		MinRecentTurns: 6,
	}
}

// Assemble returns a budget-respecting copy of messages.
// System messages are always included; conversation messages are trimmed from
// the oldest end first when the total estimate exceeds TokenBudget.
func (ca *ContextAssembler) Assemble(messages []Message) []Message {
	if len(messages) == 0 {
		return messages
	}

	// Separate system messages (priority 0) from conversation messages.
	var systemMsgs []Message
	var convMsgs []Message
	for _, m := range messages {
		if m.Role == "system" {
			systemMsgs = append(systemMsgs, m)
		} else {
			convMsgs = append(convMsgs, m)
		}
	}

	// Fast path: everything fits.
	total := 0
	for _, m := range messages {
		total += estimateMessageTokens(m)
	}
	if total <= ca.TokenBudget {
		return messages
	}

	// Compute budget remaining for conversation messages after system messages.
	sysTokens := 0
	for _, m := range systemMsgs {
		sysTokens += estimateMessageTokens(m)
	}
	remaining := ca.TokenBudget - sysTokens
	if remaining <= 0 {
		// System messages alone exceed budget — return system-only (cannot trim further).
		return systemMsgs
	}

	// Always include at least MinRecentTurns conversation messages.
	minKeep := ca.MinRecentTurns
	if minKeep > len(convMsgs) {
		minKeep = len(convMsgs)
	}

	// Greedily include conversation messages from newest to oldest until budget
	// is exhausted (but always keep at least minKeep).
	included := make([]Message, 0, len(convMsgs))
	for i := len(convMsgs) - 1; i >= 0; i-- {
		cost := estimateMessageTokens(convMsgs[i])
		mandatory := (len(convMsgs) - 1 - i) < minKeep
		if !mandatory && remaining-cost < 0 {
			break
		}
		remaining -= cost
		included = append(included, convMsgs[i])
	}

	// Reverse to restore chronological order.
	for i, j := 0, len(included)-1; i < j; i, j = i+1, j-1 {
		included[i], included[j] = included[j], included[i]
	}

	result := make([]Message, 0, len(systemMsgs)+len(included))
	result = append(result, systemMsgs...)
	result = append(result, included...)
	return result
}
