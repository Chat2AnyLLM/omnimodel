# README Enhancement

## Context

The project README.md was updated to improve its visual design, structure, and overall appeal to make the project more discoverable and professional on GitHub.

## What Changed

### Before

- Plain `<p align="center">` badges with `style=flat-square`
- Simple bullet-point Features list
- Screenshots buried inside a collapsed `<details>` block with generic label "More screenshots"
- No "Why OmniLLM?" value proposition section
- No Chinese documentation link in the nav
- No Contributing section
- Plain title without emoji or tagline
- No "PRs Welcome" badge

### After

- **Hero section** (`<div align="center">`) with:
  - Emoji logo (`🌐`) in the title
  - Two-line tagline: bold subtitle + italicized hook *"One gateway. Every model. Zero rewrites."*
  - Two-row badge layout: large `style=for-the-badge` tech stack badges (Go, Bun, TypeScript) + smaller `flat-square` metadata badges (npm, Docker, License, Stars, Last Commit, PRs Welcome)
  - Centered navigation links including 🇨🇳 Chinese docs link
- **"What is OmniLLM?" section** with:
  - Improved intro paragraph
  - "Why OmniLLM?" problem/solution comparison table
- **Screenshots section** promoted to a top-level named section (`## 📸 Screenshots`) with the admin console screenshot visible by default and other screenshots behind a descriptively-labeled `<details>` summary
- **Features section** reformatted as a two-column HTML table with emoji icons, making it easier to scan
- **Supported Providers** table enhanced with provider-specific emojis and a tip linking to `ADDING_A_PROVIDER.md`
- **Quick Start** section improved with a "one-line install" callout at the top (`bunx omnillm@latest start`)
- **Contributing section** added with step-by-step instructions and a link to the provider guide
- **Footer** added with centered "Made with ❤️" attribution and star CTA

## Why It's Critical

The README is the first thing potential users and contributors see. A more professional, visually organized README directly improves:
- Project discoverability on GitHub search
- First-impression quality for evaluators comparing LLM gateway solutions
- Contributor onboarding via the new Contributing section

## Affected Files

- `README.md`

## Commit Range

See the PR for the full diff.
