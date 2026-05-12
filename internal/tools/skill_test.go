package tools

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
)

func TestDefinitionsFiltersByActiveSkill(t *testing.T) {
	r := newFullRegistry(t)

	// Before activating any skill, skill tools should be absent.
	defs := r.Definitions()
	defNames := defNameSet(defs)

	// Core tool always present.
	if _, ok := defNames["bash"]; !ok {
		t.Error("expected 'bash' (core) in definitions")
	}
	// Skill tool absent until skill is activated.
	if _, ok := defNames["web_fetch"]; ok {
		t.Error("expected 'web_fetch' (web skill) absent before activation")
	}

	// Activate the web skill.
	r.ActivateSkill("web")
	defs = r.Definitions()
	defNames = defNameSet(defs)

	if _, ok := defNames["web_fetch"]; !ok {
		t.Error("expected 'web_fetch' present after activating 'web' skill")
	}
	// Other skill tools still absent.
	if _, ok := defNames["task_create"]; ok {
		t.Error("expected 'task_create' (task skill) still absent")
	}
}

func TestSpecKitLifecycleToolsGatedBySpecSkill(t *testing.T) {
	r := newFullRegistry(t)

	gated := []string{"speckit_lifecycle_status", "speckit_complete", "speckit_archive"}

	before := defNameSet(r.Definitions())
	for _, name := range gated {
		if before[name] {
			t.Errorf("%s should NOT be visible before load_skill(%q)", name, SkillSpec)
		}
	}

	r.ActivateSkill(SkillSpec)
	after := defNameSet(r.Definitions())
	for _, name := range gated {
		if !after[name] {
			t.Errorf("%s should be visible after load_skill(%q)", name, SkillSpec)
		}
	}
}

func TestIsSkillActiveAndActiveSkillNames(t *testing.T) {
	r := newFullRegistry(t)

	if r.IsSkillActive("web") {
		t.Error("skill 'web' should not be active initially")
	}

	r.ActivateSkill("web")
	r.ActivateSkill("task")

	if !r.IsSkillActive("web") {
		t.Error("skill 'web' should be active")
	}
	if !r.IsSkillActive("task") {
		t.Error("skill 'task' should be active")
	}

	names := r.ActiveSkillNames()
	wantNames := map[string]bool{"web": true, "task": true}
	for _, n := range names {
		if !wantNames[n] {
			t.Errorf("unexpected active skill: %q", n)
		}
	}
	if len(names) != 2 {
		t.Errorf("expected 2 active skills, got %d: %v", len(names), names)
	}
}

func TestToolSkillMembership(t *testing.T) {
	r := newFullRegistry(t)

	// Core tools have no skill.
	if s := r.ToolSkill("bash"); s != "" {
		t.Errorf("bash should be a core tool (no skill), got %q", s)
	}
	// Known skill tools (using actual registered names).
	cases := map[string]string{
		"web_fetch":                SkillWeb,
		"task_create":              SkillTask,
		"notebook_edit":            SkillFilesystem, // notebook_edit is in the filesystem skill group
		"multiedit":                SkillFilesystem,
		"speckit_lifecycle_status": SkillSpec,
		"speckit_complete":         SkillSpec,
		"speckit_archive":          SkillSpec,
	}
	for tool, wantSkill := range cases {
		got := r.ToolSkill(tool)
		if got != wantSkill {
			t.Errorf("ToolSkill(%q): want %q, got %q", tool, wantSkill, got)
		}
	}
}

func TestLoadSkillToolActivatesSkill(t *testing.T) {
	r := newFullRegistry(t)

	tool := r.Get("load_skill")
	if tool == nil {
		t.Fatal("load_skill tool not registered")
	}

	input, _ := json.Marshal(map[string]any{"skill": "web"})
	callCtx := Context{
		SessionID: "test",
		Registry:  r,
	}
	result := tool.Execute(context.Background(), callCtx, input)
	if result.IsError {
		t.Fatalf("load_skill returned error: %s", result.Output)
	}
	if !strings.Contains(result.Output, "web") {
		t.Errorf("unexpected output: %q", result.Output)
	}

	if !r.IsSkillActive("web") {
		t.Error("skill 'web' should be active after load_skill call")
	}

	// web_fetch should now appear in definitions.
	defs := r.Definitions()
	if _, ok := defNameSet(defs)["web_fetch"]; !ok {
		t.Error("web_fetch should be in definitions after load_skill('web')")
	}
}

func TestLoadSkillToolRejectsUnknownSkill(t *testing.T) {
	r := newFullRegistry(t)

	tool := r.Get("load_skill")
	input, _ := json.Marshal(map[string]any{"skill": "nonexistent"})
	callCtx := Context{SessionID: "test", Registry: r}
	result := tool.Execute(context.Background(), callCtx, input)
	if !result.IsError {
		t.Error("expected error for unknown skill")
	}
}

// ── helpers ────────────────────────────────────────────────────────────────

// newFullRegistry registers all core tools and initialises skill membership.
func newFullRegistry(t *testing.T) *Registry {
	t.Helper()
	m := NewManager()
	RegisterCoreTools(m)
	r := NewRegistry()
	for _, tool := range m.Registry().List() {
		r.Register(tool)
	}
	InitSkillMembership(r)
	return r
}

func defNameSet(defs []ToolDefinition) map[string]bool {
	out := make(map[string]bool, len(defs))
	for _, d := range defs {
		out[d.Name] = true
	}
	return out
}
