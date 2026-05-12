# AGENTS.md

## OmniCode Agent Output Guidelines

When displaying content to the OmniCode conversation UI, follow a modern Go TUI-friendly presentation style inspired by Bubble Tea + Lip Gloss + Glamour patterns:

- **Markdown-first:** Prefer structured Markdown with headings, bullets, fenced code blocks, and concise sections.
- **Panel/card style for workflows:** When showing tasks, tool output, status, or summaries, use clean section blocks or box-style panels where helpful.
- **Streaming/event-block mindset:** Present multi-step work as incremental status events, e.g. `⠋ Running tests...` then `✓ Running tests...`.
- **Tables for dense operational data:** Use compact Markdown tables only when they improve readability, such as task lists, test results, metrics, schedules, or comparisons.
- **Split-pane mental model:** Organize complex responses into clear areas such as `Current Task`, `Tool Output`, `Result`, `Next Steps`, and `Notes`.
- **Keep output copy/paste friendly:** Avoid raw ANSI spaghetti, pixel-perfect coordinate layouts, or overly decorative formatting.
- **Prefer concise, composable sections:** Use short headings and reusable blocks rather than large unstructured paragraphs.

For Go TUI implementation guidance, prefer this stack unless there is a specific reason not to:

| Purpose | Library |
| --- | --- |
| TUI framework | Bubble Tea |
| Styling/layout | Lip Gloss |
| Components | Bubbles |
| Markdown rendering | Glamour |
| ANSI helpers | charmbracelet/x/ansi |

Design OmniCode agent workflows around reactive state, streaming views, Markdown rendering, viewport abstractions, and async tool-event blocks.

## Build, Lint, and Test Commands

- **Build:**  
  `bun run build` (uses tsup)
- **Dev:**  
  `bun run dev`
- **Lint:**  
  `bun run lint` (uses @echristian/eslint-config)
- **Lint & Fix staged files:**  
  `bunx lint-staged`
- **Test all:**  
   `bun test`
- **Test single file:**  
   `bun test tests/claude-request.test.ts`
- **Start (prod):**  
  `bun run start`

## Code Style Guidelines

- **Imports:**  
  Use ESNext syntax. Prefer absolute imports via `~/*` for `src/*` (see `tsconfig.json`).
- **Formatting:**  
  Follows Prettier (with `prettier-plugin-packagejson`). Run `bun run lint` to auto-fix.
- **Types:**  
  Strict TypeScript (`strict: true`). Avoid `any`; use explicit types and interfaces.
- **Naming:**  
  Use `camelCase` for variables/functions, `PascalCase` for types/classes.
- **Error Handling:**  
  Use explicit error classes (see `src/lib/error.ts`). Avoid silent failures.
- **Unused:**  
  Unused imports/variables are errors (`noUnusedLocals`, `noUnusedParameters`).
- **Switches:**  
  No fallthrough in switch statements.
- **Modules:**  
  Use ESNext modules, no CommonJS.
- **Testing:**  
   Use Bun's built-in test runner. Place tests in `tests/`, name as `*.test.ts`.
- **Linting:**  
  Uses `@echristian/eslint-config` (see npm for details). Includes stylistic, unused imports, regex, and package.json rules.
- **Paths:**  
  Use path aliases (`~/*`) for imports from `src/`.

## Critical Change Documentation

- **Every critical change** (breaking API changes, major refactors, critical bug fixes, architecture shifts, provider protocol changes, etc.) must be documented in `docs/` as a markdown file for record.
- **File naming:** Use descriptive names, e.g. `docs/qwen3.6-plus-claude-code-critical-changes.md`, `docs/CIF_MIGRATION.md`.
- **Content convention:** Include context, what changed (before/after), why it's critical, affected files, and commit range. See existing docs for examples.
- **This rule is mandatory:** Do not skip documentation for critical changes, even if the change seems obvious.

---

This file is tailored for agentic coding agents. For more details, see the configs in `eslint.config.js` and `tsconfig.json`. No Cursor or Copilot rules detected.
