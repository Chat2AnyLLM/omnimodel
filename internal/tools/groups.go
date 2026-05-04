package tools

// RegisterCoreTools adds the full OmniCode tool set into the provided manager
// with initial metadata. This mirrors the complete tool surface of claude-code3,
// opencode, and pi-mono.
func RegisterCoreTools(m *Manager) {
	// ── Shell ──────────────────────────────────────────────────────────────────
	m.Register(Bash(), Metadata{Category: CategoryShell, ReadOnly: false})
	m.Register(PowerShell(), Metadata{Category: CategoryShell, ReadOnly: false})

	// ── Filesystem ────────────────────────────────────────────────────────────
	m.Register(Read(), Metadata{Category: CategoryFilesystem, ReadOnly: true})
	m.Register(Write(), Metadata{Category: CategoryFilesystem, ReadOnly: false})
	m.Register(Edit(), Metadata{Category: CategoryFilesystem, ReadOnly: false})
	m.Register(MultiEdit(), Metadata{Category: CategoryFilesystem, ReadOnly: false})
	m.Register(ApplyPatch(), Metadata{Category: CategoryFilesystem, ReadOnly: false})
	m.Register(Glob(), Metadata{Category: CategoryFilesystem, ReadOnly: true})
	m.Register(Grep(), Metadata{Category: CategoryFilesystem, ReadOnly: true})
	m.Register(LS(), Metadata{Category: CategoryFilesystem, ReadOnly: true})
	m.Register(NotebookEdit(), Metadata{Category: CategoryNotebook, ReadOnly: false})

	// ── Web ───────────────────────────────────────────────────────────────────
	m.Register(WebFetch(), Metadata{Category: CategoryWeb, ReadOnly: true})
	m.Register(WebSearch(), Metadata{Category: CategoryWeb, ReadOnly: true})
	m.Register(CodeSearch(), Metadata{Category: CategoryWeb, ReadOnly: true})

	// ── Utility ───────────────────────────────────────────────────────────────
	m.Register(CurrentTime(), Metadata{Category: CategoryUtility, ReadOnly: true})
	m.Register(Calculator(), Metadata{Category: CategoryUtility, ReadOnly: true})
	m.Register(Sleep(), Metadata{Category: CategoryUtility, ReadOnly: false})
	m.Register(AskUser(), Metadata{Category: CategoryUtility, ReadOnly: false})
	m.Register(LSP(), Metadata{Category: CategoryUtility, ReadOnly: true})
	m.Register(ToolSearch(), Metadata{Category: CategoryUtility, ReadOnly: true})
	m.Register(Config(), Metadata{Category: CategoryUtility, ReadOnly: false})

	// ── Task / Todo ───────────────────────────────────────────────────────────
	m.Register(TodoWrite(), Metadata{Category: CategoryTask, ReadOnly: false})
	m.Register(TaskCreate(), Metadata{Category: CategoryTask, ReadOnly: false, SupportsBackground: true})
	m.Register(TaskGet(), Metadata{Category: CategoryTask, ReadOnly: true})
	m.Register(TaskList(), Metadata{Category: CategoryTask, ReadOnly: true})
	m.Register(TaskOutput(), Metadata{Category: CategoryTask, ReadOnly: true})
	m.Register(TaskStop(), Metadata{Category: CategoryTask, ReadOnly: false})
	m.Register(TaskUpdate(), Metadata{Category: CategoryTask, ReadOnly: false})

	// ── Planning ──────────────────────────────────────────────────────────────
	m.Register(EnterPlanMode(), Metadata{Category: CategoryPlan, ReadOnly: false})
	m.Register(ExitPlanMode(), Metadata{Category: CategoryPlan, ReadOnly: false})

	// ── Worktree ──────────────────────────────────────────────────────────────
	m.Register(EnterWorktree(), Metadata{Category: CategoryWorktree, ReadOnly: false})
	m.Register(ExitWorktree(), Metadata{Category: CategoryWorktree, ReadOnly: false})

	// ── Scheduler ─────────────────────────────────────────────────────────────
	m.Register(ScheduleCron(), Metadata{Category: CategoryScheduler, ReadOnly: false})

	// ── Multi-agent / Messaging ───────────────────────────────────────────────
	m.Register(SendMessage(), Metadata{Category: CategoryUtility, ReadOnly: false})
	m.Register(AgentTool(), Metadata{Category: CategoryUtility, ReadOnly: false})
	m.Register(Batch(), Metadata{Category: CategoryUtility, ReadOnly: false})
}

// InitRegistryStores creates fresh session-scoped stores and attaches them to
// the registry so all tool executions share the same state.
func InitRegistryStores(r *Registry) {
	r.TodoStore = NewTodoStore()
	r.TaskStore = NewTaskStore()
	r.PlanState = NewPlanState()
	r.WorktreeState = NewWorktreeState()
	r.ConfigStore = NewConfigStore()
}
