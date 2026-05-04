package tools

// Metadata describes non-execution properties of a tool for higher-level
// managers and future enablement/permission logic.
type Metadata struct {
	Category            Category
	ReadOnly            bool
	SupportsBackground  bool
	RequiresManager     string
}
