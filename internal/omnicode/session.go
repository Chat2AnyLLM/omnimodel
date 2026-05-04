package omnicode

// SessionState is reserved for OmniCode-owned session/runtime state that may
// eventually diverge from the proxy-backed chat session representation.
type SessionState struct {
	ID string
}
