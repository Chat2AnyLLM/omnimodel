package tools

import "omnillm/internal/specdriven"

// RegisterCoreTools adds the full OmniCode tool set into the provided manager
// with initial metadata. This mirrors the complete tool surface of claude-code3,
// opencode, and pi-mono.
func RegisterCoreTools(m *Manager) {
	// 鈹€鈹€ Shell 鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€
	m.Register(Bash(), Metadata{Category: CategoryShell, ReadOnly: false})
	m.Register(PowerShell(), Metadata{Category: CategoryShell, ReadOnly: false})

	// 鈹€鈹€ Filesystem 鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€
	m.Register(Read(), Metadata{Category: CategoryFilesystem, ReadOnly: true})
	m.Register(Write(), Metadata{Category: CategoryFilesystem, ReadOnly: false})
	m.Register(Edit(), Metadata{Category: CategoryFilesystem, ReadOnly: false})
	m.Register(MultiEdit(), Metadata{Category: CategoryFilesystem, ReadOnly: false})
	m.Register(ApplyPatch(), Metadata{Category: CategoryFilesystem, ReadOnly: false})
	m.Register(Glob(), Metadata{Category: CategoryFilesystem, ReadOnly: true})
	m.Register(Grep(), Metadata{Category: CategoryFilesystem, ReadOnly: true})
	m.Register(LS(), Metadata{Category: CategoryFilesystem, ReadOnly: true})
	m.Register(NotebookEdit(), Metadata{Category: CategoryNotebook, ReadOnly: false})

	// 鈹€鈹€ Web 鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€
	m.Register(WebFetch(), Metadata{Category: CategoryWeb, ReadOnly: true})
	m.Register(WebSearch(), Metadata{Category: CategoryWeb, ReadOnly: true})
	m.Register(CodeSearch(), Metadata{Category: CategoryWeb, ReadOnly: true})

	// 鈹€鈹€ Utility 鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€
	m.Register(CurrentTime(), Metadata{Category: CategoryUtility, ReadOnly: true})
	m.Register(Calculator(), Metadata{Category: CategoryUtility, ReadOnly: true})
	m.Register(Sleep(), Metadata{Category: CategoryUtility, ReadOnly: false})
	m.Register(AskUser(), Metadata{Category: CategoryUtility, ReadOnly: false})
	m.Register(LSP(), Metadata{Category: CategoryUtility, ReadOnly: true})
	m.Register(ToolSearch(), Metadata{Category: CategoryUtility, ReadOnly: true})
	m.Register(Config(), Metadata{Category: CategoryUtility, ReadOnly: false})

	// 鈹€鈹€ Task / Todo 鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€
	m.Register(TodoWrite(), Metadata{Category: CategoryTask, ReadOnly: false})
	m.Register(TaskCreate(), Metadata{Category: CategoryTask, ReadOnly: false, SupportsBackground: true})
	m.Register(TaskGet(), Metadata{Category: CategoryTask, ReadOnly: true})
	m.Register(TaskList(), Metadata{Category: CategoryTask, ReadOnly: true})
	m.Register(TaskOutput(), Metadata{Category: CategoryTask, ReadOnly: true})
	m.Register(TaskStop(), Metadata{Category: CategoryTask, ReadOnly: false})
	m.Register(TaskUpdate(), Metadata{Category: CategoryTask, ReadOnly: false})

	// 鈹€鈹€ Planning 鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€
	m.Register(EnterPlanMode(), Metadata{Category: CategoryPlan, ReadOnly: false})
	m.Register(ExitPlanMode(), Metadata{Category: CategoryPlan, ReadOnly: false})

	// 鈹€鈹€ Worktree 鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€
	m.Register(EnterWorktree(), Metadata{Category: CategoryWorktree, ReadOnly: false})
	m.Register(ExitWorktree(), Metadata{Category: CategoryWorktree, ReadOnly: false})

	// 鈹€鈹€ Scheduler 鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€
	m.Register(ScheduleCron(), Metadata{Category: CategoryScheduler, ReadOnly: false})
	m.Register(ScheduleHeartbeat(), Metadata{Category: CategoryScheduler, ReadOnly: false})
	m.Register(TriggerEvent(), Metadata{Category: CategoryScheduler, ReadOnly: false})

	// 鈹€鈹€ Multi-agent / Messaging 鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€
	m.Register(SendMessage(), Metadata{Category: CategoryUtility, ReadOnly: false})
	m.Register(AgentTool(), Metadata{Category: CategoryUtility, ReadOnly: false})
	m.Register(Batch(), Metadata{Category: CategoryUtility, ReadOnly: false})
	m.Register(OrchestrateAgents(), Metadata{Category: CategoryUtility, ReadOnly: false})

	// 鈹€鈹€ Skill loader 鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€
	m.Register(LoadSkill(), Metadata{Category: CategoryUtility, ReadOnly: false})

	// 鈹€鈹€ Spec-driven 鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€
	// Legacy spec_* agent tools (spec_init, spec_write, spec_read, spec_plan,
	// spec_tasks, spec_status) have been removed. Use the speckit_* and
	// openspec_* tools below instead.
	m.Register(SpecKitConstitution(), Metadata{Category: CategorySpec, ReadOnly: false})
	m.Register(SpecKitSpecify(), Metadata{Category: CategorySpec, ReadOnly: false})
	m.Register(SpecKitClarify(), Metadata{Category: CategorySpec, ReadOnly: false})
	m.Register(SpecKitPlan(), Metadata{Category: CategorySpec, ReadOnly: false})
	m.Register(SpecKitTasks(), Metadata{Category: CategorySpec, ReadOnly: false})
	m.Register(SpecKitAnalyze(), Metadata{Category: CategorySpec, ReadOnly: false})
	m.Register(SpecKitImplement(), Metadata{Category: CategorySpec, ReadOnly: true})
	m.Register(SpecKitChecklist(), Metadata{Category: CategorySpec, ReadOnly: false})
	m.Register(SpecKitLifecycleStatus(), Metadata{Category: CategorySpec, ReadOnly: true})
	m.Register(SpecKitComplete(), Metadata{Category: CategorySpec, ReadOnly: false})
	m.Register(SpecKitArchive(), Metadata{Category: CategorySpec, ReadOnly: false})
	m.Register(OpenSpecPropose(), Metadata{Category: CategorySpec, ReadOnly: false})
	m.Register(OpenSpecExplore(), Metadata{Category: CategorySpec, ReadOnly: false})
	m.Register(OpenSpecNew(), Metadata{Category: CategorySpec, ReadOnly: false})
	m.Register(OpenSpecContinue(), Metadata{Category: CategorySpec, ReadOnly: false})
	m.Register(OpenSpecFF(), Metadata{Category: CategorySpec, ReadOnly: false})
	m.Register(OpenSpecApply(), Metadata{Category: CategorySpec, ReadOnly: true})
	m.Register(OpenSpecVerify(), Metadata{Category: CategorySpec, ReadOnly: false})
	m.Register(OpenSpecSync(), Metadata{Category: CategorySpec, ReadOnly: false})
	m.Register(OpenSpecArchive(), Metadata{Category: CategorySpec, ReadOnly: false})
	m.Register(OpenSpecBulkArchive(), Metadata{Category: CategorySpec, ReadOnly: false})
	m.Register(OpenSpecOnboard(), Metadata{Category: CategorySpec, ReadOnly: false})
	m.Register(OpenSpecLegacyProposal(), Metadata{Category: CategorySpec, ReadOnly: false})
	m.Register(OpenSpecLegacyApply(), Metadata{Category: CategorySpec, ReadOnly: true})
	m.Register(OpenSpecLegacyArchive(), Metadata{Category: CategorySpec, ReadOnly: false})
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

// SkillConsoleOutputFormatter enables presentation-focused console rendering.
const SkillConsoleOutputFormatter = "console_output_formatter"

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
	for _, name := range []string{
		"speckit_constitution", "speckit_specify", "speckit_clarify", "speckit_plan", "speckit_tasks", "speckit_analyze", "speckit_implement", "speckit_checklist", "speckit_lifecycle_status", "speckit_complete", "speckit_archive",
		"openspec_propose", "openspec_explore", "openspec_new", "openspec_continue", "openspec_ff", "openspec_apply", "openspec_verify", "openspec_sync", "openspec_archive", "openspec_bulk_archive", "openspec_onboard",
		"openspec_legacy_proposal", "openspec_legacy_apply", "openspec_legacy_archive",
	} {
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
