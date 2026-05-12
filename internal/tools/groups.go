package tools

import "omnillm/internal/specdriven"

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
	m.Register(ScheduleHeartbeat(), Metadata{Category: CategoryScheduler, ReadOnly: false})
	m.Register(TriggerEvent(), Metadata{Category: CategoryScheduler, ReadOnly: false})

	// ── Multi-agent / Messaging ───────────────────────────────────────────────
	m.Register(SendMessage(), Metadata{Category: CategoryUtility, ReadOnly: false})
	m.Register(AgentTool(), Metadata{Category: CategoryUtility, ReadOnly: false})
	m.Register(Batch(), Metadata{Category: CategoryUtility, ReadOnly: false})
	m.Register(OrchestrateAgents(), Metadata{Category: CategoryUtility, ReadOnly: false})

	// ── Skill loader ──────────────────────────────────────────────────────────
	m.Register(LoadSkill(), Metadata{Category: CategoryUtility, ReadOnly: false})

	// ── Spec-driven ───────────────────────────────────────────────────────────
	m.Register(SpecInit(), Metadata{Category: CategorySpec, ReadOnly: false})
	m.Register(SpecWrite(), Metadata{Category: CategorySpec, ReadOnly: false})
	m.Register(SpecRead(), Metadata{Category: CategorySpec, ReadOnly: true})
	m.Register(SpecPlan(), Metadata{Category: CategorySpec, ReadOnly: false})
	m.Register(SpecTasks(), Metadata{Category: CategorySpec, ReadOnly: false})
	m.Register(SpecStatus(), Metadata{Category: CategorySpec, ReadOnly: true})
}

// SkillFilesystem groups write-capable filesystem tools.
const SkillFilesystem = "filesystem"

// SkillWeb groups web-access tools.
const SkillWeb = "web"

// SkillTask groups background task management tools.
const SkillTask = "task"

// SkillNotebook groups Jupyter notebook tools.
const SkillNotebook = "notebook"

// SkillPlan groups plan-mode tools.
const SkillPlan = "plan"

// SkillWorktree groups git worktree tools.
const SkillWorktree = "worktree"

// SkillScheduler groups scheduler/cron tools.
const SkillScheduler = "scheduler"

// SkillAgent groups multi-agent / sub-agent tools.
const SkillAgent = "agent"

// SkillUtilityExtra groups less-common utility tools (calculator, sleep, lsp, etc).
const SkillUtilityExtra = "utility_extra"

// SkillSpec groups spec-driven development tools.
const SkillSpec = "spec"

// InitSkillMembership assigns non-core tools in r to their skill group.
// Must be called after RegisterCoreTools has populated the registry.
// Core tools (always visible): bash, powershell, read, write, edit, glob,
// grep, ls, ask_user_question, get_current_time, todo_write, load_skill.
// Everything else requires the user/model to call load_skill first.
func InitSkillMembership(r *Registry) {
	// filesystem skill: destructive / bulk write operations
	for _, name := range []string{"multiedit", "apply_patch", "notebook_edit"} {
		if t := r.Get(name); t != nil {
			r.RegisterSkillTool(t, SkillFilesystem)
		}
	}
	// web skill
	for _, name := range []string{"web_fetch", "web_search", "codesearch"} {
		if t := r.Get(name); t != nil {
			r.RegisterSkillTool(t, SkillWeb)
		}
	}
	// task skill
	for _, name := range []string{
		"task_create", "task_get", "task_list", "task_output", "task_stop", "task_update",
	} {
		if t := r.Get(name); t != nil {
			r.RegisterSkillTool(t, SkillTask)
		}
	}
	// plan skill
	for _, name := range []string{"enter_plan_mode", "exit_plan_mode"} {
		if t := r.Get(name); t != nil {
			r.RegisterSkillTool(t, SkillPlan)
		}
	}
	// worktree skill
	for _, name := range []string{"enter_worktree", "exit_worktree"} {
		if t := r.Get(name); t != nil {
			r.RegisterSkillTool(t, SkillWorktree)
		}
	}
	// scheduler skill
	for _, name := range []string{"schedule_cron", "schedule_heartbeat", "trigger_event"} {
		if t := r.Get(name); t != nil {
			r.RegisterSkillTool(t, SkillScheduler)
		}
	}
	// agent skill
	for _, name := range []string{"send_message", "agent", "batch", "orchestrate_agents"} {
		if t := r.Get(name); t != nil {
			r.RegisterSkillTool(t, SkillAgent)
		}
	}
	// utility_extra skill
	for _, name := range []string{"calculator", "sleep", "lsp", "tool_search", "config"} {
		if t := r.Get(name); t != nil {
			r.RegisterSkillTool(t, SkillUtilityExtra)
		}
	}
	// spec skill
	for _, name := range []string{"spec_init", "spec_write", "spec_read", "spec_plan", "spec_tasks", "spec_status"} {
		if t := r.Get(name); t != nil {
			r.RegisterSkillTool(t, SkillSpec)
		}
	}
}

// InitRegistryStores creates fresh session-scoped stores and attaches them to
// the registry so all tool executions share the same state.
func InitRegistryStores(r *Registry) {
	r.TodoStore = NewTodoStore()
	r.TaskStore = NewTaskStore()
	r.PlanState = NewPlanState()
	r.WorktreeState = NewWorktreeState()
	r.ConfigStore = NewConfigStore()
	r.SpecState = specdriven.NewSpecStore()
}
