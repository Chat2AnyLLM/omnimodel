package chat

import (
	"bytes"
	"strings"
	"testing"
)

func TestSlashCommandsCatalogShape(t *testing.T) {
	cmds := slashCommands()
	if len(cmds) == 0 {
		t.Fatalf("slashCommands() returned empty catalog")
	}

	seen := map[string]bool{}
	for _, c := range cmds {
		if c.Name == "" || c.Name[0] != '/' {
			t.Errorf("invalid command name %q", c.Name)
		}
		if c.Summary == "" {
			t.Errorf("command %q has empty summary", c.Name)
		}
		if seen[c.Name] {
			t.Errorf("duplicate command name %q", c.Name)
		}
		seen[c.Name] = true
		for _, a := range c.Aliases {
			if seen[a] {
				t.Errorf("duplicate alias %q on %q", a, c.Name)
			}
			seen[a] = true
		}
	}

	for _, must := range []string{"/help", "/new", "/sessions", "/session", "/mode", "/apishape", "/permissions", "/model", "/agent", "/max-turns", "/models", "/specify.init", "/speckit.specify", "/speckit.status", "/opsx:init", "/opsx:propose", "/clear", "/quit"} {
		if !seen[must] {
			t.Errorf("catalog missing required command %q", must)
		}
	}

	for _, mustNot := range []string{"/spec", "/spec mode spec-kit", "/spec mode openspec", "/spec mode off", "/spec init", "/spec status", "/spec help"} {
		if seen[mustNot] {
			t.Errorf("catalog should NOT contain retired command %q", mustNot)
		}
	}
}

func TestFuzzySlashFilter(t *testing.T) {
	all := slashCommands()

	cases := []struct {
		name    string
		filter  string
		want    []string
		notWant []string
	}{
		{name: "empty returns all", filter: "", want: []string{"/help"}},
		{name: "leading slash only returns all", filter: "/", want: []string{"/help"}},
		{name: "prefix match", filter: "/mo", want: []string{"/mode", "/model", "/models"}},
		{name: "no leading slash still matches", filter: "mo", want: []string{"/mode", "/model", "/models"}},
		{name: "question mark alias", filter: "?", want: []string{"/help"}},
		{name: "specify init prefix match", filter: "/specify", want: []string{"/specify.init"}},
		{name: "speckit status fuzzy match", filter: "speckit.stat", want: []string{"/speckit.status"}},
		{name: "opsx init prefix match", filter: "opsx:init", want: []string{"/opsx:init"}},
		{name: "no match", filter: "/zzzz", notWant: []string{"/help"}},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := fuzzySlashFilter(all, tc.filter)
			if tc.filter == "" || tc.filter == "/" {
				if len(got) != len(all) {
					t.Fatalf("filter %q: want all %d, got %d", tc.filter, len(all), len(got))
				}
				return
			}
			if len(tc.want) > 0 && len(got) < len(tc.want) {
				t.Fatalf("filter %q: want at least %v, got %v", tc.filter, tc.want, slashNames(got))
			}
			for i, name := range tc.want {
				if got[i].Name != name {
					t.Errorf("filter %q: position %d want %q got %q (full=%v)", tc.filter, i, name, got[i].Name, slashNames(got))
				}
			}
			for _, nw := range tc.notWant {
				for _, g := range got {
					if g.Name == nw {
						t.Errorf("filter %q: did not want %q", tc.filter, nw)
					}
				}
			}
		})
	}
}

func slashNames(cs []slashCommand) []string {
	out := make([]string, len(cs))
	for i, c := range cs {
		out[i] = c.Name
	}
	return out
}

func TestRenderSlashHelp(t *testing.T) {
	out := renderSlashHelp(slashCommands())
	for _, want := range []string{"/help", "/models", "/specify.init", "/speckit.status", "/opsx:init", "/quit", "show available commands"} {
		if !strings.Contains(out, want) {
			t.Errorf("renderSlashHelp output missing %q\n---\n%s", want, out)
		}
	}
	for _, mustNot := range []string{"/spec mode spec-kit", "/spec mode openspec"} {
		if strings.Contains(out, mustNot) {
			t.Errorf("renderSlashHelp output should NOT contain retired %q\n---\n%s", mustNot, out)
		}
	}
	if !strings.HasPrefix(strings.TrimSpace(out), "**Commands:**") {
		t.Errorf("renderSlashHelp output should start with **Commands:** header; got:\n%s", out)
	}
}

func TestHandleDirectSpecCommandSpecifyInitRequiresTitle(t *testing.T) {
	var buf bytes.Buffer
	handled, err := handleDirectSpecCommand(&buf, []string{"/specify.init"})
	if !handled {
		t.Fatalf("expected /specify.init to be handled directly")
	}
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(buf.String(), "Usage: /specify.init <title>") {
		t.Fatalf("expected usage hint, got %q", buf.String())
	}
}

func TestHandleDirectSpecCommandOpsxInitRequiresName(t *testing.T) {
	var buf bytes.Buffer
	handled, err := handleDirectSpecCommand(&buf, []string{"/opsx:init"})
	if !handled || err != nil {
		t.Fatalf("expected /opsx:init handled without error, got handled=%v err=%v", handled, err)
	}
	if !strings.Contains(buf.String(), "Usage: /opsx:init <change-name>") {
		t.Fatalf("expected usage hint, got %q", buf.String())
	}
}

func TestHandleDirectSpecCommandSpeckitStatusMissingDir(t *testing.T) {
	var buf bytes.Buffer
	handled, err := handleDirectSpecCommand(&buf, []string{"/speckit.status", "definitely-not-a-real-dir-xyz"})
	if !handled || err != nil {
		t.Fatalf("expected /speckit.status handled without error, got handled=%v err=%v", handled, err)
	}
	out := buf.String()
	if !strings.Contains(out, "No specs directory") {
		t.Fatalf("expected missing-dir hint, got %q", out)
	}
}

func TestHandleDirectSpecCommandUnknownPassesThrough(t *testing.T) {
	var buf bytes.Buffer
	handled, _ := handleDirectSpecCommand(&buf, []string{"/speckit.specify", "stuff"})
	if handled {
		t.Fatalf("/speckit.specify should fall through to agent routing, not be handled directly")
	}
}

