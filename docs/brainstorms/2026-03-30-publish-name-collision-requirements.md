---
date: 2026-03-30
topic: publish-name-collision
---

# Publish Name Collision Handling

## Problem Frame

When a user runs `printing-press-publish`, there is no check for whether a CLI with the same name already exists in the library repo — either merged into main or in another user's open PR. This can result in accidental overwrites or duplicate submissions that create noise for reviewers.

Users need to know about collisions before they submit, and they need clear paths forward: intentionally replace an existing CLI, publish alongside it under a different name, or bail out.

## Requirements

**Detection**

- R1. Before creating a branch or PR, check the managed clone (`~/.printing-press/.publish-repo`) for an existing directory matching the CLI name in `library/`
- R2. Before creating a branch or PR, check `gh pr list` on the library repo (without `--author @me` filter) for open PRs whose head branch matches `feat/<cli-name>`
- R3. If a collision is detected, show the user what exists: CLI name, source (merged vs open PR), author if available, and PR link if applicable

**Collision Resolution Paths**

- R4. When a collision is detected, offer three paths:
  - **Replace** — intentionally overwrite the existing CLI (opens a PR that replaces it)
  - **Alongside** — rename yours with a qualifier and publish next to the existing one
  - **Bail** — cancel the publish and optionally view the existing CLI or PR
- R5. The replace path must produce a PR description that clearly states the intent to replace an existing CLI, distinguishing it from an accidental collision

**Rename (Alongside) Flow**

- R6. The renamed CLI must follow the format `<api-slug>-<qualifier>-pp-cli` — the user picks only the qualifier portion; prefix and suffix are locked
- R7. Present the user with suggestions: 1 numeric fallback (e.g., `<api-slug>-2-pp-cli`) and 1 non-numeric fallback (e.g., `<api-slug>-alt-pp-cli`), plus a custom qualifier option (same format lock applies — user enters only the qualifier word)
- R8. All suggestions must be verified non-colliding against both merged CLIs and open PRs before being presented
- R9. A rename must propagate to all places the CLI name appears: directory name, Go module path, `.printing-press.json` manifest (`cli_name`), binary name, `cmd/<cli-name>/` directory, `.goreleaser.yaml` build targets, branch name, and PR metadata. (This list is known to be non-exhaustive — planning must trace the full set.)
- R10. A renamed CLI's `.printing-press.json` manifest must preserve the **original** API slug in the `api_name` field (e.g., `notion`, not `notion-alt`). The `cli_name` field updates to the new name; `api_name` stays canonical.

**Ownership-Aware Resolution**

- R11. Collision resolution must distinguish between the current user's own PRs and other users' PRs. Replacing your own previous PR is routine (update flow). Replacing another user's PR requires explicit stronger confirmation acknowledging the other author.

## Success Criteria

- A user who publishes a CLI that already exists is never surprised — they always see the collision and make an explicit choice
- The replace path produces PRs that reviewers can clearly identify as intentional replacements
- The alongside path produces a validly-named, non-colliding CLI that passes all quality gates under its new name

## Scope Boundaries

- Per-CLI versioning is out of scope — deferred to a future brainstorm
- Collision detection is scoped to the publish flow only; the generate command does not check the library repo
- Auto-suggesting semantically meaningful qualifiers (e.g., based on spec content analysis) is out of scope — generic fallbacks + custom input only

## Key Decisions

- **Three-path resolution over block-or-rename:** Users should be able to intentionally replace an existing CLI, not just rename. The PR review process protects against bad replacements.
- **Locked name format for renames:** Users pick only the qualifier, not the full name. This prevents inconsistent naming conventions and keeps the `<api-slug>-...-pp-cli` pattern intact.
- **Generic fallback suggestions:** Auto-suggesting semantically meaningful qualifiers (from spec content) is impractical and low-value. One numeric + one non-numeric fallback covers the common cases.
- **Same rules for merged and open-PR collisions:** Both trigger the same detection and resolution flow, but with ownership-aware confirmation for other users' PRs.
- **Manifest preserves original API slug:** Renamed CLIs keep the canonical `api_name` in the manifest. Code that needs the true API slug reads the manifest rather than reverse-engineering it from the CLI name.

## Dependencies / Assumptions

- The managed clone at `~/.printing-press/.publish-repo` must be freshened (git fetch/reset) **before** collision detection runs — collision check must happen after the existing Step 6 clone management, not before
- `gh` CLI is authenticated and can list PRs on the library repo
- The rename propagation touches the same files that the normal publish flow already writes — no net-new file types

## Outstanding Questions

### Deferred to Planning

- [Affects R9][Technical] What is the full list of files/paths that need updating during rename propagation? Need to trace through the publish skill's packaging step.
- [Affects R1][Technical] Should collision detection live in Go code (a new subcommand or flag) or purely in the skill instructions? Go code would be more reliable; skill instructions would be faster to iterate.
- [Affects R7][Needs research] Should numeric fallback start at 2 (like `postman-explore-2-pp-cli`) and scan upward, or use a fixed suggestion? Need to check what `ClaimOutputDir` already does for local collisions.
- [Affects R6, R10][Technical] `TrimCLISuffix` returns `notion-alt` for `notion-alt-pp-cli` instead of `notion`. Since the manifest carries the canonical `api_name`, code paths that need the true API slug should prefer the manifest. Planning should audit all `TrimCLISuffix` call sites and determine if any need updating for renamed CLI support.

## Next Steps

→ `/ce:plan` for structured implementation planning
