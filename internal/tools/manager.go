package tools

// Manager owns a registry plus grouping metadata so OmniCode can expose
// different tool sets without hardcoding everything into session_runner.
type Manager struct {
	registry  *Registry
	metadata  map[string]Metadata
}

func NewManager() *Manager {
	return &Manager{
		registry: NewRegistry(),
		metadata: make(map[string]Metadata),
	}
}

func (m *Manager) Registry() *Registry {
	return m.registry
}

func (m *Manager) Register(tool Tool, meta Metadata) {
	m.registry.Register(tool)
	m.metadata[tool.Name()] = meta
}

func (m *Manager) Metadata(name string) (Metadata, bool) {
	meta, ok := m.metadata[name]
	return meta, ok
}

func (m *Manager) ToolNamesByCategory(category Category) []string {
	var out []string
	for name, meta := range m.metadata {
		if meta.Category == category {
			out = append(out, name)
		}
	}
	return out
}
