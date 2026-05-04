package tools

// RegisterCoreTools adds the currently implemented OmniLLM-native core tool set
// into the provided manager with initial metadata. This is the foundation that
// future OmniCode tool families will build on.
func RegisterCoreTools(m *Manager) {
	m.Register(Bash(), Metadata{Category: CategoryShell, ReadOnly: false})
	m.Register(Read(), Metadata{Category: CategoryFilesystem, ReadOnly: true})
	m.Register(Write(), Metadata{Category: CategoryFilesystem, ReadOnly: false})
	m.Register(Edit(), Metadata{Category: CategoryFilesystem, ReadOnly: false})
	m.Register(Glob(), Metadata{Category: CategoryFilesystem, ReadOnly: true})
	m.Register(Grep(), Metadata{Category: CategoryFilesystem, ReadOnly: true})
	m.Register(LS(), Metadata{Category: CategoryFilesystem, ReadOnly: true})
	m.Register(CurrentTime(), Metadata{Category: CategoryUtility, ReadOnly: true})
	m.Register(WebFetch(), Metadata{Category: CategoryWeb, ReadOnly: true})
	m.Register(Calculator(), Metadata{Category: CategoryUtility, ReadOnly: true})
}
