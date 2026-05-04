package omnicode

// Runtime will own the dedicated coding-agent managers as OmniCode grows.
// For now it exists as the anchor package for the separate omnicode binary.
type Runtime struct{}

func NewRuntime() *Runtime {
	return &Runtime{}
}
