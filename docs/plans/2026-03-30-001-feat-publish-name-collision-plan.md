---
title: "feat: Add name collision detection and resolution to publish workflow"
type: feat
status: active
date: 2026-03-30
origin: docs/brainstorms/2026-03-30-publish-name-collision-requirements.md
---

# feat: Add name collision detection and resolution to publish workflow

## Overview

Add collision detection to the publish skill so users are never surprised when a CLI name already exists in the library repo. When a collision is detected, offer three resolution paths: intentionally replace, publish alongside with a different name, or bail out.

## Problem Frame

When a user runs `printing-press-publish`, there is no check for whether a CLI name already exists — merged into main or in another user's open PR. This can result in accidental overwrites or duplicate submissions. Users need to see collisions before they submit and make an explicit choice about how to proceed. (see origin: docs/brainstorms/2026-03-30-publish-name-collision-requirements.md)

## Requirements Trace

- R1. Check managed clone `library/` for existing CLI directory before PR creation
- R2. Check `gh pr list` on library repo (no `--author @me` filter) for open PRs with matching branch
- R3. Show collision info: CLI name, source (merged/open PR), author, PR link
- R4. Offer three paths: Replace, Alongside (rename), Bail
- R5. Replace path PR description clearly states intent to replace
- R6. Rename format locked to `<api-slug>-<qualifier>-pp-cli`
- R7. Suggest 1 numeric + 1 non-numeric fallback + custom qualifier option
- R8. Verify all suggestions are non-colliding
- R9. Rename propagates to all name-bearing locations
- R10. Manifest preserves original `api_name` for renamed CLIs
- R11. Ownership-aware resolution — stronger confirmation for replacing other users' PRs

## Scope Boundaries

- Per-CLI versioning is out of scope (see origin)
- Collision detection scoped to publish flow only; `generate` does not check the library repo
- No semantically meaningful qualifier suggestions — generic fallbacks + custom only
- `TrimCLISuffix` compatibility for renamed CLIs is out of scope for this plan — noted as technical debt (affects `emboss.go:236`, `runtime.go:97`, and `runtime.go:198` in `findCLICommandDir`; note that `runtime.go:198` is likely safe because it reconstructs the CLI name via `naming.CLI(apiName)` which roundtrips correctly). Additionally, `publish.go:230` and `publish.go:379` use `TrimCLISuffix` as a fallback when the manifest lacks `api_name` — these ARE in the publish path but are guarded by the manifest check, so they are safe as long as the manifest is present

## Context & Research

### Relevant Code and Patterns

- **Publish skill:** `skills/printing-press-publish/SKILL.md` — 8-step workflow; collision detection inserts between Steps 6 and 7
- **Naming module:** `internal/naming/naming.go` — `CLI()`, `TrimCLISuffix()`, `IsCLIDirName()`, `trimNumericRunSuffix()`
- **Publish commands:** `internal/cli/publish.go` — `publish validate` and `publish package` subcommands
- **Module path rewriting:** `internal/pipeline/modulepath.go` — `RewriteModulePath()` handles go.mod, import paths, install paths, GitHub URLs. Does NOT handle: Use strings, version template, User-Agent, goreleaser project/binary/brew names, Makefile, README title
- **CLI manifest:** `internal/pipeline/climanifest.go` — `CLIManifest` struct with `APIName` and `CLIName` fields
- **Local collision handling:** `internal/pipeline/pipeline.go` — `ClaimOutputDir()` uses `-2` through `-99` suffixes
- **Module path tests:** `internal/pipeline/modulepath_test.go` — table-driven with explicit "does not corrupt non-import strings" assertions

### Institutional Learnings

- **Path traversal protection:** User-supplied qualifiers in `filepath.Join` need belt-and-suspenders validation: reject traversal chars AND verify resolved path is contained within expected root (from `docs/solutions/security-issues/filepath-join-traversal-with-user-input-2026-03-29.md`)
- **Validation immutability:** Validation must not mutate the source directory; use snapshot-compare-restore pattern (from `docs/solutions/best-practices/validation-must-not-mutate-source-directory-2026-03-29.md`)
- **Test collision-suffixed dirs:** Tests should cover both canonical (`notion-pp-cli`) and collision-suffixed (`notion-pp-cli-2`) directories (from `docs/solutions/best-practices/checkout-scoped-printing-press-output-layout-2026-03-28.md`)

## Key Technical Decisions

- **Hybrid Go + skill approach:** Collision detection UX lives in the skill (already orchestrates `gh` commands and user interaction). Rename propagation lives in Go code (a new `RenameCLI` function + `publish rename` subcommand) for reliability across 18+ file locations. Rationale: the skill can't reliably do targeted string replacements in Go source files, but it's great at presenting choices and calling CLI commands.

- **Rename operates on the staging copy:** The publish flow packages to a staging directory first (`publish package`), then copies to the managed clone. The rename step operates on the staging copy between packaging and clone-copy, avoiding mutation of the user's library directory. Rationale: follows the immutability principle from institutional learnings.

- **`RenameCLI` is separate from `RewriteModulePath`:** Module path rewriting handles import paths and Go module declarations. CLI name renaming handles user-visible strings (Use, version, User-Agent, goreleaser, Makefile, README). These are distinct operations with different replacement rules. Rationale: `RewriteModulePath` deliberately avoids touching bare CLI name references — the rename function handles exactly what `RewriteModulePath` intentionally skips.

- **Collision detection merges with Step 7:** Instead of a separate step, collision detection replaces the existing Step 7 ("Check for Existing PR"). The current `--author @me` check becomes one branch of the broader collision check. Rationale: avoids duplicate `gh pr list` calls and keeps the decision tree unified.

- **Numeric fallback starts at 2:** Matches `ClaimOutputDir` convention. The existing `trimNumericRunSuffix` already handles this pattern.

## Open Questions

### Resolved During Planning

- **Full rename propagation list:** 18 locations identified via research — 7 handled by `RewriteModulePath`, 7 need `RenameCLI`, 4 are metadata/workflow handled by the skill. See Unit 1 approach for details.
- **Go code vs skill for collision detection:** Hybrid — detection UX in skill, rename propagation in Go. The Go binary provides `publish rename` subcommand; the skill orchestrates the flow.
- **Numeric fallback convention:** Start at 2, matching `ClaimOutputDir`. `trimNumericRunSuffix` already strips these.
- **TrimCLISuffix impact:** 2 unsafe call sites (`emboss.go:236`, `runtime.go:97`) are outside the publish flow. Noted as tech debt, not blocking.

### Deferred to Implementation

- **Exact replacement regex for CLI name in goreleaser:** The goreleaser template embeds the CLI name in ~8 positions. Implementation should verify the string replacement doesn't create false positives. The `modulepath_test.go` "does not corrupt" pattern should be followed.
- **Registry.json schema for renamed CLIs:** The registry entry needs the new `cli_name` but should the `api_name` field also be present? Implementation should check the current schema.

## High-Level Technical Design

> *This illustrates the intended approach and is directional guidance for review, not implementation specification. The implementing agent should treat it as context, not code to reproduce.*

```
User runs printing-press-publish
  │
  ├─ Steps 1-6: resolve name, validate, package (runs RewriteModulePath), freshen clone (unchanged)
  │   Note: RewriteModulePath normalizes import paths and module declarations during packaging.
  │   RenameCLI operates on a pre-rewritten staging copy and handles only user-visible CLI name references.
  │
  ▼
Step 7 (NEW): Collision Detection & Resolution
  │
  ├─ Check managed clone: ls library/*/<cli-name>
  │   └─ Found? → merged_collision = true, record category path
  │
  ├─ Check gh pr list (no --author @me): feat/<cli-name>
  │   └─ Found? → pr_collision = true, record author + URL
  │
  ├─ Check gh pr list (--author @me): feat/<cli-name>
  │   └─ Found? → own_pr = true, record PR number
  │
  ├─ No collision? → proceed to Step 8 normally
  │
  └─ Collision detected → present info (R3) → offer paths (R4):
      │
      ├─ Replace:
      │   ├─ Own PR → update existing (current behavior)
      │   ├─ Other user's PR → strong confirmation (R11)
      │   └─ Merged only → standard confirmation
      │   → PR description states "Replaces existing <cli-name>" (R5)
      │   → proceed to Step 8
      │
      ├─ Alongside:
      │   ├─ Present qualifier options (R7)
      │   ├─ Verify non-colliding (R8)
      │   ├─ Call: printing-press publish rename --dir <staging> \
      │   │       --old-name <old> --new-name <new> --json
      │   ├─ Use new name for branch, PR, registry
      │   └─ proceed to Step 8 with new name
      │
      └─ Bail: exit with link to existing CLI/PR
```

## Implementation Units

- [ ] **Unit 1: Add `RenameCLI` function**

**Goal:** Create a function that renames all non-module-path CLI name references in a staged CLI directory, plus renames the filesystem directories.

**Requirements:** R6, R9, R10

**Dependencies:** None

**Files:**
- Create: `internal/pipeline/renamecli.go`
- Test: `internal/pipeline/renamecli_test.go`

**Approach:**
- `RenameCLI(dir, oldCLIName, newCLIName, originalAPIName string) error`
- **Filesystem renames:**
  - Rename outer directory from `oldCLIName` to `newCLIName`
  - Rename `cmd/oldCLIName/` to `cmd/newCLIName/`
- **File content replacements:** Walk `.go`, `.yaml`, `.yml`, `.md` files, plus files named `Makefile` (no extension — walk filter must check `filepath.Base(path) == "Makefile"` in addition to extension matching; do not reuse `hasRewriteExtension` from modulepath.go without this check). Replace occurrences of `oldCLIName` with `newCLIName`. This covers: `Use:` string, version template, version printf, User-Agent, goreleaser project_name/binary/brew/install, Makefile targets, README title/usage. **Skip the `.manuscripts/` subtree** — these are archival provenance records that should preserve original names
- **Manifest update:** Read `.printing-press.json`, set `CLIName` to `newCLIName`, preserve `APIName` as `originalAPIName`, write back
- **Input validation:** Both names must match `naming.IsCLIDirName()`. Apply path traversal protection per institutional learning (reject `/`, `\`, `..` in qualifier; verify resolved path within expected root)
- **Does NOT call `RewriteModulePath`** — that's already done during packaging. This function handles exactly what `RewriteModulePath` intentionally skips

**Patterns to follow:**
- `RewriteModulePath` in `internal/pipeline/modulepath.go` — same walk/replace pattern but different target strings
- `modulepath_test.go` — table-driven with explicit "does not corrupt" assertions
- Path traversal protection from `internal/cli/publish.go` category validation

**Test scenarios:**
- Happy path: Rename `notion-pp-cli` to `notion-alt-pp-cli` — verify all 7 non-module name references updated, both directories renamed, manifest has new cli_name + original api_name
- Happy path: Rename `notion-pp-cli` to `notion-2-pp-cli` (numeric qualifier) — same verifications
- Edge case: CLI name appears as substring in unrelated strings (e.g., comment "see notion-pp-cli docs") — should still be replaced (it's a CLI name reference)
- Edge case: API name without suffix (e.g., `notion` in a comment) should NOT be replaced — only full CLI name matches
- Edge case: `cmd/` directory doesn't exist (some CLIs may have different structures) — graceful handling
- Error path: Path traversal in new name (`../evil-pp-cli`) — rejected with error
- Error path: Invalid CLI name format (missing `-pp-cli` suffix) — rejected with error
- Error path: Old name not found in directory — clear error message
- Integration: After rename, verify `go build ./cmd/<new-name>` would find the right entrypoint (directory exists with correct name)

**Verification:**
- All tests pass
- `go vet ./internal/pipeline/...` clean

---

- [ ] **Unit 2: Add `publish rename` subcommand**

**Goal:** Expose `RenameCLI` as a CLI subcommand the skill can call.

**Requirements:** R9

**Dependencies:** Unit 1

**Files:**
- Modify: `internal/cli/publish.go`
- Test: `internal/cli/publish_test.go`

**Approach:**
- New subcommand: `printing-press publish rename --dir <path> --old-name <old> --new-name <new> [--api-name <original>] --json`
- `--api-name` defaults to `naming.TrimCLISuffix(oldName)` if not provided — ensures manifest gets the right canonical API name even without explicit flag
- JSON output: `{ "success": true, "old_name": "...", "new_name": "...", "new_dir": "<path>", "files_modified": N }` on success, or `{ "success": false, "error": "..." }` on failure. The `new_dir` field returns the renamed directory path so the skill doesn't need to reconstruct it (matches the `staged_dir` pattern from `PackageResult`)
- Follow existing subcommand patterns: `newPublishValidateCmd()`, `newPublishPackageCmd()` style

**Patterns to follow:**
- `newPublishValidateCmd()` and `newPublishPackageCmd()` in `internal/cli/publish.go` for command structure and JSON output pattern
- Existing flag patterns: `--dir`, `--json`

**Test scenarios:**
- Happy path: Call `publish rename` with valid args — verify JSON output reports success and correct names
- Error path: Missing required flags — clear error message
- Error path: Invalid CLI name in `--new-name` — error before any filesystem changes
- Edge case: `--api-name` flag omitted — falls back to `TrimCLISuffix(oldName)` correctly

**Verification:**
- `printing-press publish rename --help` shows the expected flags
- JSON output is parseable by the skill

---

- [ ] **Unit 3: Add collision detection to publish skill**

**Goal:** Detect name collisions after the managed clone is freshened (between current Steps 6 and 7) and display collision information to the user.

**Requirements:** R1, R2, R3, R11

**Dependencies:** None (skill-only change, independent of Go code)

**Files:**
- Modify: `skills/printing-press-publish/SKILL.md`

**Approach:**
- Replace current Step 7 ("Check for Existing PR") with a merged collision detection + resolution step
- **Detection sequence:**
  1. Check managed clone: `ls "$PUBLISH_REPO_DIR/library"/*/"<cli-name>" 2>/dev/null` — if found, record as merged collision
  2. Check all open PRs: `gh pr list --repo <lib-repo> --head "feat/<cli-name>" --state open --json number,title,url,author` — if found, record as PR collision
  3. Check own PRs: filter the above result by `--author @me` to distinguish ownership
- **Display (R3):** Show CLI name, collision source (merged/open PR/both), author name for PR collisions, and PR URL
- **Ownership distinction (R11):** Tag each collision as "yours" or "other user's" based on the `--author @me` filter result

**Patterns to follow:**
- Current Step 7 in SKILL.md for `gh pr list` command structure
- Existing skill error handling patterns (check exit codes, fallback behavior)

**Test scenarios:**
- Not applicable (skill markdown, not executable code). Verification is via manual testing during a publish run.

**Verification:**
- The skill instructions are clear enough that a Claude Code agent can execute them correctly
- The collision info display covers all combinations: merged only, PR only (own), PR only (other user), merged + PR

---

- [ ] **Unit 4: Add resolution paths to publish skill**

**Goal:** When a collision is detected, present three resolution paths and execute the chosen path.

**Requirements:** R4, R5, R6, R7, R8, R10, R11

**Dependencies:** Units 1-2 (for Alongside path), Unit 3 (collision detection must exist)

**Files:**
- Modify: `skills/printing-press-publish/SKILL.md`

**Approach:**

**Three-path choice (R4):**
- If user's own PR exists: default to "Update your existing PR" (preserves current behavior), also offer Alongside and Bail
- If other user's PR only: offer Replace (with strong confirmation), Alongside, Bail
- If merged only: offer Replace, Alongside, Bail

**Replace path (R5, R11):**
- PR description template includes: "⚠️ Replaces existing `<cli-name>` — [reason: newer spec / improved coverage / etc.]"
- For other user's CLIs: require explicit confirmation naming the other author ("This will replace <author>'s `<cli-name>`. Are you sure?")
- Proceed to Step 8 with normal flow (the existing `rm -rf "$PUBLISH_REPO_DIR/library"/*/"<cli-name>"` already handles the overwrite)

**Alongside path (R6, R7, R8, R10):**
- Extract `<api-slug>` from manifest `api_name` field
- Generate suggestions:
  - Numeric: `<api-slug>-2-pp-cli` (scan upward if 2 collides)
  - Non-numeric: `<api-slug>-alt-pp-cli`
  - Custom: prompt for qualifier word
- Verify each suggestion against merged CLIs (`ls library/*/`) and open PRs (`gh pr list --head "feat/<suggestion>"`)
- Call: `printing-press publish rename --dir <staging-dir>/<category>/<old-cli-name> --old-name <old> --new-name <new> --api-name <api-slug> --json`
- Update all downstream references in the skill: branch name → `feat/<new-name>`, PR title, registry.json entry, copy commands

**Bail path:**
- Show link to existing PR if applicable
- Show path to existing CLI in managed clone if merged
- Exit publish flow

**Patterns to follow:**
- Current Step 7/8 flow for PR creation commands
- Existing skill choice patterns (numbered options presented to user)

**Test scenarios:**
- Not applicable (skill markdown). Verification via manual testing.

**Verification:**
- The skill instructions cover all collision scenarios: merged only, own PR, other user's PR, merged + PR
- The Alongside flow correctly calls `publish rename` with the right flags
- The Replace flow produces a PR description that reviewers can identify as intentional
- All downstream references (branch, PR title, registry) use the new name after Alongside

## System-Wide Impact

- **Interaction graph:** The publish skill calls `publish rename` as a new binary invocation. No callbacks, middleware, or observers affected. The existing `publish validate` and `publish package` commands are unchanged.
- **Error propagation:** If `publish rename` fails, the skill should show the error and offer to retry with a different qualifier or bail. The staging directory may be in a partial state — the skill should re-run `publish package` to get a fresh staging copy.
- **State lifecycle risks:** Rename operates on the staging copy, not the user's library. If the publish fails after rename, the staging copy is disposable. No persistent state is corrupted.
- **API surface parity:** The `publish rename` subcommand is a new CLI surface. It follows existing `publish validate`/`publish package` conventions.
- **Unchanged invariants:** `printing-press generate`, `printing-press library`, `printing-press emboss` are all unaffected. The `RewriteModulePath` function is not modified. The `naming` module functions are not modified.

## Risks & Dependencies

| Risk | Mitigation |
|------|------------|
| String replacement in `RenameCLI` could false-positive on substrings | The old CLI name is highly specific (e.g., `notion-pp-cli`) — false positives on arbitrary text are unlikely. Test with explicit "does not corrupt" assertions following `modulepath_test.go` pattern. |
| `gh pr list` without `--author @me` may hit rate limits with many PRs | The library repo is small-to-medium. One additional API call per publish is negligible. |
| User-supplied qualifier could be empty or invalid | Go code validates: must be non-empty, kebab-case, no path traversal chars. Skill also validates before calling. |
| Managed clone could be stale if `git fetch` fails | Existing skill behavior: if clone management fails, the skill falls back gracefully. Collision detection should follow the same pattern — if the clone isn't fresh, warn but don't block. |

## Documentation / Operational Notes

- The `publish rename` subcommand will appear in `printing-press publish --help`
- No AGENTS.md changes needed — the skill is the user-facing interface
- TrimCLISuffix technical debt (`emboss.go:236`, `runtime.go:97`) should be tracked for a future cleanup when renamed CLIs start appearing in local libraries

## Sources & References

- **Origin document:** [docs/brainstorms/2026-03-30-publish-name-collision-requirements.md](docs/brainstorms/2026-03-30-publish-name-collision-requirements.md)
- Related code: `internal/pipeline/modulepath.go`, `internal/naming/naming.go`, `internal/cli/publish.go`
- Related learnings: `docs/solutions/security-issues/filepath-join-traversal-with-user-input-2026-03-29.md`, `docs/solutions/best-practices/validation-must-not-mutate-source-directory-2026-03-29.md`
