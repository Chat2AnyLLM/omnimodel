package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
)

// allSkills is the authoritative list of available skill names and their
// human-readable descriptions. Keeping this in one place makes it easy to
// update and ensures load_skill can enumerate valid values for the model.
var allSkills = map[string]string{
	SkillFilesystem:   "Destructive filesystem ops: multiedit, apply_patch, notebook_edit",
	SkillWeb:          "Web access: web_fetch, web_search, codesearch",
	SkillTask:         "Background tasks: task_create/get/list/output/stop/update",
	SkillPlan:         "Plan mode: enter_plan_mode, exit_plan_mode",
	SkillWorktree:     "Git worktrees: enter_worktree, exit_worktree",
	SkillScheduler:    "Cron scheduling: schedule_cron",
	SkillAgent:        "Multi-agent: send_message, agent, batch",
	SkillUtilityExtra: "Extra utilities: calculator, sleep, lsp, tool_search, config",
}

type loadSkillTool struct{}

// LoadSkill returns the load_skill tool which activates a named skill group,
// making its tools available for the remainder of the session.
func LoadSkill() Tool { return &loadSkillTool{} }

func (t *loadSkillTool) Name() string { return "load_skill" }

func (t *loadSkillTool) Description() string {
	skills := make([]string, 0, len(allSkills))
	for k := range allSkills {
		skills = append(skills, k)
	}
	sort.Strings(skills)
	return fmt.Sprintf(
		"Activate a skill to unlock additional tools for this session. "+
			"Call this before using any skill-specific tool. "+
			"Available skills: %s.",
		strings.Join(skills, ", "),
	)
}

func (t *loadSkillTool) InputSchema() map[string]any {
	skillNames := make([]string, 0, len(allSkills))
	for k := range allSkills {
		skillNames = append(skillNames, k)
	}
	sort.Strings(skillNames)

	enumValues := make([]any, len(skillNames))
	for i, n := range skillNames {
		enumValues[i] = n
	}

	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"skill": map[string]any{
				"type":        "string",
				"description": "The skill to activate.",
				"enum":        enumValues,
			},
		},
		"required": []string{"skill"},
	}
}

func (t *loadSkillTool) Execute(_ context.Context, call Context, input json.RawMessage) Result {
	var p struct {
		Skill string `json:"skill"`
	}
	if err := json.Unmarshal(input, &p); err != nil {
		return Result{Output: "error: " + err.Error(), IsError: true}
	}
	p.Skill = strings.TrimSpace(p.Skill)
	if p.Skill == "" {
		return Result{Output: "error: skill name is required", IsError: true}
	}

	desc, known := allSkills[p.Skill]
	if !known {
		valid := make([]string, 0, len(allSkills))
		for k := range allSkills {
			valid = append(valid, k)
		}
		sort.Strings(valid)
		return Result{
			Output:  fmt.Sprintf("unknown skill %q; valid skills: %s", p.Skill, strings.Join(valid, ", ")),
			IsError: true,
		}
	}

	// The Registry is accessible via the Context's session-scoped registry pointer.
	// We activate through the registry embedded in the call context.
	if call.Registry == nil {
		return Result{Output: "error: registry not available in tool context", IsError: true}
	}
	call.Registry.ActivateSkill(p.Skill)

	return Result{
		Output: fmt.Sprintf("skill %q activated: %s", p.Skill, desc),
	}
}
