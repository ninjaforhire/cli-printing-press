---
title: "fix: Use runstate for active builds, add heartbeat lock for parallel safety"
type: fix
status: completed
date: 2026-03-31
deepened: 2026-03-31
---

# fix: Use runstate for active builds, add heartbeat lock for parallel safety

## Overview

The main printing-press skill writes generated CLIs directly to `$PRESS_LIBRARY/<api>-pp-cli` during Phases 2-5, contradicting its own stated architecture ("active mutable work lives under `$PRESS_RUNSTATE/`"). This breaks parallel build safety because `$PRESS_LIBRARY` is global while `$PRESS_RUNSTATE` is scoped per workspace. Interrupted sessions also leave partial CLIs in the library that look complete to subsequent runs.

The fix moves active builds to the run-scoped working directory (`$API_RUN_DIR/working/<api>-pp-cli`), adds a heartbeat lock mechanism via `printing-press lock` CLI commands, and copies to library only after shipcheck passes.

## Problem Frame

Two bugs discovered:

1. **Architectural contradiction**: `PRESS_CURRENT` is defined and `mkdir -p`'d in the setup contract but never referenced in Phases 2-5. All generation, build, and verification commands write directly to the global `$PRESS_LIBRARY`.

2. **No interrupted-build detection**: If a session dies mid-build, the next run's Phase 0 Library Check sees a partial CLI in `$PRESS_LIBRARY` and treats it as a complete, published CLI.

The Go binary's automated `print` pipeline already does this correctly — `WorkingCLIDir(apiName, runID)` writes to `.runstate/<scope>/runs/<run-id>/working/<api>-pp-cli`, and `PublishWorkingCLI()` copies to library as a separate step. The interactive skill flow just never adopted the same pattern.

## Requirements Trace

- R1. Active CLI builds must live under `$PRESS_RUNSTATE`, not `$PRESS_LIBRARY`
- R2. Two parallel agents building the same API from different worktrees must not collide during build
- R3. A heartbeat lock in a global locks directory must signal build-in-progress to other sessions
- R4. Interrupted builds must be detectable (stale heartbeat) and reclaimable
- R5. CLI code moves to `$PRESS_LIBRARY` only after shipcheck passes
- R6. The lock mechanism must be a deterministic CLI command, not model-improvised bash
- R7. Existing consuming skills (polish, score, publish) must continue to work
- R8. The setup contract validation tests must pass after changes

## Scope Boundaries

- The `DefaultOutputDir()` Go function is NOT changed — it's only used when `--output` isn't specified (direct CLI usage), which is a separate concern
- The `printing-press-catalog` skill is deprecated and gets minimal changes (no lock integration)
- The polish skill continues to operate on library copies (published CLIs), not working copies
- No distributed locking or file locking (`flock`) — the heartbeat/staleness approach is sufficient for the use case

## Context & Research

### Relevant Code and Patterns

- `internal/pipeline/paths.go`: `WorkingCLIDir(apiName, runID)` — the correct runstate-scoped path the skill should use
- `internal/pipeline/publish.go`: `PublishWorkingCLI()` — copies from working dir to library via `CopyDir()` + `ClaimOutputDir()`
- `internal/pipeline/state.go`: `CurrentRunPointer` struct with `UpdatedAt` — existing timestamp pattern
- `internal/pipeline/pipeline.go`: `ClaimOutputDir()` — atomic `os.Mkdir` for concurrent directory claiming
- `internal/pipeline/contracts_test.go`: Setup contract validation tests that must pass
- Setup contract delimiters: `<!-- PRESS_SETUP_CONTRACT_START -->` / `<!-- PRESS_SETUP_CONTRACT_END -->`

### Key Insight: Go Binary Already Has the Right Pattern

`pipeline.Init()` in `pipeline.go:63` uses `WorkingCLIDir(apiName, runID)` as the default output dir. `PublishWorkingCLI()` in `publish.go:78` copies from working to library. The skill just needs to follow this same two-phase pattern.

## Key Technical Decisions

- **Lock file location: `$PRESS_HOME/.locks/<api>-pp-cli.lock`**: The lock lives in a dedicated global locks directory, separate from both library and runstate. This avoids creating anything in the library during build (the original bug) while keeping lock checks simple — one file path, no multi-scope scanning. Phase 0 checks the library for existing CLIs and the locks directory for active builds as two separate concerns.

- **Lock atomicity via `O_CREATE|O_EXCL` on the lock file**: Atomicity is achieved by `os.OpenFile("<api>-pp-cli.lock", O_CREATE|O_EXCL, 0644)` — first writer wins. The `.locks/` directory is created with `os.MkdirAll` (idempotent). For stale lock reclaim, read-then-replace is acceptable given the heartbeat-based design.

- **`CLI_WORK_DIR` variable set after `<api>` is known, not in setup contract**: The setup contract doesn't know `<api>` yet. The new variable is set in the "After you know `<api>`" section alongside `RUN_ID`, `API_RUN_DIR`, etc. This avoids changing the shared setup contract and its validation tests.

- **Staleness threshold: 30 minutes**: Phase 3 (Build) can involve long stretches within a single priority level — a complex P0 implementation with Codex delegation can run 20-30 minutes without a phase-boundary update. 30 minutes avoids false-positive staleness while still detecting genuinely dead sessions. Heartbeat updates happen at phase transitions AND after each priority level in Phase 3, giving 7-10 updates per typical run.

- **Promotion to library via `printing-press lock promote` CLI command, not raw `cp -r`**: The copy-to-library step must be a deterministic CLI command (R6), handle clean replacement of existing library contents (not additive merge), write the CLI manifest, update the `CurrentRunPointer` to reflect the library path, and release the lock — all in one step. Raw `cp -r` would violate R6, leave orphaned files from previous builds, and not update state. The Go binary already has `PublishWorkingCLI()` and `CopyDir()` — the `promote` subcommand wraps this existing logic.

- **Promotion happens immediately after shipcheck, before archiving**: The CLI being in library is the primary deliverable. Archiving manuscripts is supplementary. Promoting first minimizes the window of "verified but not in library" state if the session dies between shipcheck and archive.

- **`lock acquire` auto-reclaims stale locks; `--force` needed only for fresh locks held by a different scope**: This is the common case — a stale lock means the previous session died. Auto-reclaim simplifies the skill's Phase 0 flow (just call acquire, check the result). `--force` is a safety valve for the rare case where a user wants to override a fresh lock they know is theirs from a different worktree.

- **Explicit lock release on all failure/abort paths**: The skill must call `printing-press lock release` whenever a build fails and the user chooses not to retry (generation failure, shipcheck hold, user cancels). This prevents the 30-minute staleness window from blocking other agents after a known failure.

## Open Questions

### Resolved During Planning

- **Should the setup contract change?** No. `CLI_WORK_DIR` is set after `<api>` is known, outside the contract block. The contract test validates the contract block contents, so avoiding changes there means no test breakage from contract changes alone.

- **Should `DefaultOutputDir()` change?** No. It's used by the `generate` subcommand's default `--output` for direct CLI usage. The skill always passes `--output` explicitly.

- **What if two agents build different APIs from the same worktree?** Safe — each gets a unique `RUN_ID` and thus a unique `API_RUN_DIR/working/<api>-pp-cli` path. The locks in `.locks/` are per-CLI-name, so different APIs don't collide.

- **How does `lock acquire` handle rebuild case (library dir exists, no lock, user wants rebuild)?** Acquire writes the lock file to `.locks/<api>-pp-cli.lock` with `O_CREATE|O_EXCL`. The existing library dir is untouched until `lock promote` replaces its contents.

- **How does Phase 0 distinguish debris directories from completed CLIs?** Check for `go.mod` or `.printing-press.json` presence alongside directory existence. A library directory with no `go.mod` and no active lock is debris from a previous promote that was interrupted — offer cleanup rather than treating as complete CLI.

- **Should `printing-press library list` filter out incomplete directories?** Yes. Library directories that have no `go.mod` or `.printing-press.json` should be excluded from library list results to avoid confusing the publish skill.

### Deferred to Implementation

- **Exact JSON schema for lock file (`.locks/<cli-name>.lock`)**: The lock struct fields and JSON format will be finalized during implementation. Conceptual shape: `{scope, phase, pid, acquired_at, updated_at}`.

## High-Level Technical Design

> *This illustrates the intended approach and is directional guidance for review, not implementation specification. The implementing agent should treat it as context, not code to reproduce.*

```
Build lifecycle (before fix):

  Phase 2 ──write──> $PRESS_LIBRARY/<api>-pp-cli
  Phase 3 ──modify─> $PRESS_LIBRARY/<api>-pp-cli
  Phase 4 ──verify─> $PRESS_LIBRARY/<api>-pp-cli
  Phase 6 ──publish from─> $PRESS_LIBRARY/<api>-pp-cli

Build lifecycle (after fix):

  Directory layout:
    ~/printing-press/.locks/<api>-pp-cli.lock       (global lock file)
    ~/printing-press/.runstate/<scope>/runs/<id>/working/<api>-pp-cli  (build here)
    ~/printing-press/library/<api>-pp-cli            (only after promote)

  Phase 2 ──lock acquire──> .locks/<api>-pp-cli.lock             (claim name)
           ──write────────> $API_RUN_DIR/working/<api>-pp-cli    (build here)
  Phase 3 ──lock update───> (heartbeat in .locks/)
           ──modify───────> $API_RUN_DIR/working/<api>-pp-cli
  Phase 4 ──lock update───> (heartbeat in .locks/)
           ──verify───────> $API_RUN_DIR/working/<api>-pp-cli
  Phase 5.5 ──promote─────> $PRESS_LIBRARY/<api>-pp-cli          (copy + release lock)
  Phase 6 ──publish from──> $PRESS_LIBRARY/<api>-pp-cli
```

Phase 0 checks two things independently:
1. Does `$PRESS_LIBRARY/<api>-pp-cli` exist? (completed CLI)
2. Does `printing-press lock status --cli <api>-pp-cli` report an active lock? (build in progress)

```
Library dir? | Lock? | Stale? | Has go.mod? | Action
-------------|-------|--------|-------------|-------
No           | No    | N/A    | N/A         | Proceed normally, acquire lock in Phase 2
No           | Yes   | No     | N/A         | "Actively being built (phase X, Ys ago). Wait, use a different name, or pick different API."
No           | Yes   | Yes    | N/A         | "Interrupted build (stale since X). Reclaim and start fresh?"
Yes          | No    | N/A    | Yes         | Completed CLI — existing "Found existing" flow
Yes          | No    | N/A    | No          | Debris — offer cleanup
Yes          | Yes   | No     | Any         | "Actively being rebuilt. Wait, use a different name, or pick different API."
Yes          | Yes   | Yes    | Any         | "Interrupted rebuild (stale since X). Reclaim?"
```

Rebuild case: When user chooses "Generate a fresh CLI" from the existing
"Found existing" flow, Phase 2 acquires the lock (writes to .locks/),
builds in runstate, and the promote step replaces library contents cleanly.

## Implementation Units

- [ ] **Unit 1: Go — Lock state management**

  **Goal:** Add lock file operations (acquire, update, status, release) to the pipeline package.

  **Requirements:** R3, R4, R6

  **Dependencies:** None

  **Files:**
  - Create: `internal/pipeline/lock.go`
  - Test: `internal/pipeline/lock_test.go`

  **Approach:**
  - Define `LockState` struct (scope, phase, PID, acquired_at, updated_at)
  - Locks directory: `PressPressHome()/.locks/` (global, not scoped — visible to all agents)
  - Lock file path: `PressPressHome()/.locks/<cli-name>.lock`
  - `AcquireLock(cliName, scope string)` — creates `.locks/` with `os.MkdirAll` (idempotent), then writes lock file with `os.OpenFile("<cli-name>.lock", O_CREATE|O_EXCL, 0644)` for atomicity. Auto-reclaims stale locks. Returns error if fresh lock held by different scope. Returns success if no lock, stale lock, or same-scope lock. With `force=true`, overrides even fresh locks from other scopes.
  - `UpdateLock(cliName, phase string)` — refreshes `updated_at` and `phase`
  - `LockStatus(cliName string)` — returns current lock state including staleness, and separately checks the library dir for a completed CLI (`go.mod` or `.printing-press.json` present → `has_cli=true`)
  - `ReleaseLock(cliName string)` — removes lock file (idempotent)
  - `PromoteWorkingCLI(cliName, workingDir string, state *PipelineState)` — the promotion sequence: if library dir exists, clear its contents; copy working dir contents to library via `CopyDir`; write CLI manifest; update `CurrentRunPointer` so `working_dir` reflects the library path; then release the lock. This wraps the existing `PublishWorkingCLI` + `CopyDir` pattern.
  - `IsStale(lock LockState)` — checks `updated_at` against 30-minute threshold
  - Stale lock reclaim: read the existing lock, verify staleness, delete it, then create new lock with `O_CREATE|O_EXCL`. There is a TOCTOU window between delete and create, but this is acceptable — two agents reclaiming the same stale lock is an extremely unlikely race, and the loser gets a clear "lock held" error.

  **Patterns to follow:**
  - `internal/pipeline/state.go` — JSON struct marshaling pattern (`CurrentRunPointer`)
  - `internal/pipeline/publish.go` — `PublishWorkingCLI()`, `CopyDir()`, `writeCLIManifestForPublish()` for the promotion logic
  - `internal/pipeline/pipeline.go` — `ClaimOutputDir()` for directory operations

  **Test scenarios:**
  - Happy path: Acquire lock when no lock exists — .locks/ dir created, lock file written with correct fields
  - Happy path: Acquire lock when library dir exists but no lock (rebuild) — lock file created in .locks/
  - Happy path: Update lock phase, verify updated_at changes and phase changes
  - Happy path: Release lock, verify lock file removed from .locks/
  - Happy path: Status on active lock returns held=true, stale=false
  - Happy path: Promote copies working dir to library, writes manifest, updates run pointer, removes lock
  - Edge case: Acquire when stale lock exists — auto-reclaims, acquires successfully
  - Edge case: Acquire when fresh lock exists from different scope — returns error/blocked status
  - Edge case: Acquire when fresh lock exists from same scope — succeeds (re-entrant for retries)
  - Edge case: Acquire with force=true when fresh lock exists from different scope — succeeds
  - Edge case: Status on non-existent lock — returns held=false
  - Edge case: Status when no lock but library dir has go.mod — returns held=false, has_cli=true
  - Edge case: Status when no lock and library dir has no go.mod — returns held=false, has_cli=false
  - Edge case: Status when no lock and no library dir — returns held=false, has_cli=false
  - Edge case: Promote when library dir has files from previous build — old files replaced, not merged
  - Error path: Release on non-existent lock file — no error (idempotent)
  - Error path: Promote when working dir is empty — returns error
  - Integration: Concurrent acquire from two goroutines — exactly one succeeds via O_CREATE|O_EXCL

  **Verification:**
  - All tests pass
  - Lock file is valid JSON readable by `LockStatus`
  - Promote produces a library dir identical to what `PublishWorkingCLI` would produce

- [ ] **Unit 2: Go — Lock CLI subcommands**

  **Goal:** Expose lock operations as `printing-press lock {acquire,update,status,release}` subcommands.

  **Requirements:** R6

  **Dependencies:** Unit 1

  **Files:**
  - Create: `internal/cli/lock.go`
  - Modify: `internal/cli/root.go` (add `rootCmd.AddCommand(newLockCmd())`)
  - Test: `internal/cli/lock_test.go`

  **Approach:**
  - Parent command `lock` with subcommands: `acquire`, `update`, `status`, `release`, `promote`
  - Common flags: `--cli <name>` (required for all)
  - `acquire` flags: `--scope <scope>` (required), `--force` (override fresh locks from other scopes)
  - `update` flags: `--phase <phase>` (required)
  - `status` flags: `--json` (structured output, includes `held`, `stale`, `phase`, `has_cli`, `scope`, `age_seconds`)
  - `release` flags: none beyond `--cli`
  - `promote` flags: `--dir <working-dir>` (required — path to the working CLI to promote)
  - All subcommands output JSON to stdout for deterministic parsing by the skill
  - Non-zero exit code on blocked acquire (fresh lock held by another scope)
  - `promote` handles the full sequence: clear old library dir contents (if exists), copy working dir to library, write CLI manifest, update CurrentRunPointer, release lock from `.locks/`

  **Patterns to follow:**
  - `internal/cli/library.go` — subcommand registration pattern
  - `internal/cli/publish.go` — nested subcommand pattern (`publish validate`, `publish package`)

  **Test scenarios:**
  - Happy path: `lock acquire --cli test-pp-cli --scope scope-1` writes lock file, exits 0
  - Happy path: `lock status --cli test-pp-cli --json` returns JSON with held/stale/phase/has_cli fields
  - Happy path: `lock update --cli test-pp-cli --phase build` refreshes heartbeat
  - Happy path: `lock release --cli test-pp-cli` removes lock, exits 0
  - Happy path: `lock promote --cli test-pp-cli --dir /path/to/working` copies, writes manifest, exits 0
  - Error path: `lock acquire` without `--cli` flag — exits non-zero with usage
  - Error path: `lock acquire --cli x --scope s` when fresh lock held by different scope — exits non-zero, JSON indicates blocked
  - Error path: `lock promote --cli test-pp-cli --dir /nonexistent` — exits non-zero

  **Verification:**
  - `go build ./...` and `go vet ./...` pass
  - Subcommands appear in `printing-press lock --help`

- [ ] **Unit 3: Skill — Add `CLI_WORK_DIR` and lock lifecycle to main skill**

  **Goal:** Replace all Phase 2-5 references to `$PRESS_LIBRARY/<api>-pp-cli` with run-scoped `$CLI_WORK_DIR`, and add lock commands at phase boundaries.

  **Requirements:** R1, R2, R3, R5

  **Dependencies:** Unit 2

  **Files:**
  - Modify: `skills/printing-press/SKILL.md`

  **Approach:**

  This is the largest unit. Changes span multiple sections of the 1662-line skill file. All changes are to SKILL.md content (markdown + bash blocks), not Go code.

  **A. Add `CLI_WORK_DIR` to "After you know `<api>`" setup block (around line 168-179):**
  - Add `CLI_WORK_DIR="$API_RUN_DIR/working/<api>-pp-cli"` after the existing variable definitions
  - Add `mkdir -p "$CLI_WORK_DIR"` to the existing `mkdir -p` call

  **B. Update state file schema documentation (around line 183-189):**
  - `working_dir` should point to `$CLI_WORK_DIR`
  - `output_dir` should point to `$CLI_WORK_DIR` during build

  **C. Update Phase 0 Library Check (around line 254-286):**
  - Two independent checks: (1) does `$PRESS_LIBRARY/<api>-pp-cli` exist with `go.mod`? (2) is there an active lock?
    ```bash
    printing-press lock status --cli <api>-pp-cli --json
    ```
  - Route based on combined result per the decision matrix: library + no lock → existing "Found existing" flow; no library + active lock → warn user; stale lock → offer reclaim; neither → proceed

  **D. Replace `$PRESS_LIBRARY/<api>-pp-cli` in Phase 2 (around lines 1078-1168):**
  - All 7 `--output "$PRESS_LIBRARY/<api>-pp-cli"` variants → `--output "$CLI_WORK_DIR"`
  - The description rewrite path → `$CLI_WORK_DIR/internal/cli/root.go`
  - Add `printing-press lock acquire --cli <api>-pp-cli --scope "$PRESS_SCOPE"` before generation
  - Add `printing-press lock update --cli <api>-pp-cli --phase generate` after generation

  **E. Replace `$PRESS_LIBRARY/<api>-pp-cli` in Phase 3 (around lines 1175-1400):**
  - All `cd "$PRESS_LIBRARY/<api>-pp-cli"` → `cd "$CLI_WORK_DIR"`
  - All codex delegation references to the library path → `$CLI_WORK_DIR`
  - Add `printing-press lock update --cli <api>-pp-cli --phase build-p0` after Priority 0
  - Add `printing-press lock update --cli <api>-pp-cli --phase build-p1` after Priority 1
  - Add `printing-press lock update --cli <api>-pp-cli --phase build-p2` after Priority 2

  **F. Replace `$PRESS_LIBRARY/<api>-pp-cli` in Phase 4 (around lines 1406-1510):**
  - All `--dir "$PRESS_LIBRARY/<api>-pp-cli"` → `--dir "$CLI_WORK_DIR"`
  - All codex fix delegation references → `$CLI_WORK_DIR`
  - Add `printing-press lock update --cli <api>-pp-cli --phase shipcheck` before shipcheck

  **G. Replace `$PRESS_LIBRARY/<api>-pp-cli` in Phase 5 (around lines 1512-1527):**
  - Smoke test references → `$CLI_WORK_DIR`

  **H. Add promotion to library in Phase 5.5 — BEFORE archiving manuscripts (around line 1529-1557):**
  - Reorder Phase 5.5 so promotion happens first, then archiving:
    ```bash
    # Promote verified CLI to library (before archiving — CLI is the primary deliverable)
    printing-press lock promote --cli <api>-pp-cli --dir "$CLI_WORK_DIR"
    ```
  - Then archive manuscripts as before (using `$CLI_WORK_DIR` references for source paths)
  - The `promote` command handles: clearing old library files, copying working dir, writing CLI manifest, updating CurrentRunPointer, and releasing the lock — all in one deterministic step

  **I. Add lock release on all failure/abort paths:**
  - Whenever the skill's flow terminates early (generation fails, shipcheck fails with "hold" verdict, user cancels, API reachability gate fails after lock acquire), add:
    ```bash
    printing-press lock release --cli <api>-pp-cli
    ```
  - This prevents the 30-minute staleness window from unnecessarily blocking other agents after a known failure
  - The skill already has well-defined failure points — add release at each one

  **J. Bump `min-binary-version` in the main skill's setup contract:**
  - Update the `# min-binary-version:` comment to the version that ships the `lock` subcommands
  - This must also be bumped in all 4 skills' setup contracts for consistency

  **K. Update Phase 6 references (around lines 1559-1620):**
  - Phase 6 reads from `$PRESS_LIBRARY/<api>-pp-cli` — this is CORRECT after the promote step
  - No changes needed for Phase 6 publish flow itself

  **L. Handle shipcheck "hold" verdict:**
  - When shipcheck verdict is "hold" and the user chooses not to retry: release the lock, do NOT promote to library. The working copy remains in runstate for potential future retry. Archive manuscripts as normal.

  **Critical: Variable reference contexts.** The skill uses `$PRESS_LIBRARY/<api>-pp-cli` in three contexts:
  1. Inside ` ```bash ``` ` blocks — these are executed by the model as shell commands. Replace with `$CLI_WORK_DIR`.
  2. Inside inline backtick references like `` `$PRESS_LIBRARY/<api>-pp-cli/internal/cli/root.go` `` — these guide the model to file paths. Replace with `$CLI_WORK_DIR/internal/cli/root.go`.
  3. Inside prose descriptions like "the CLI in $PRESS_LIBRARY/<api>-pp-cli" — replace with appropriate new path reference.

  Do NOT replace `$PRESS_LIBRARY` references in Phase 0's Library Check (it correctly checks the library) or Phase 6 (it correctly reads from the library after promotion).

  **Test scenarios:**
  - Test expectation: none — this is a skill markdown file, not code. Verification is behavioral during skill execution.

  **Verification:**
  - All `$PRESS_LIBRARY/<api>-pp-cli` references in Phases 2-5 are replaced with `$CLI_WORK_DIR`
  - Lock acquire appears before Phase 2 generation
  - Lock update appears at each phase boundary and priority level
  - `printing-press lock promote` appears in Phase 5.5 (before manuscript archiving)
  - Lock release appears at every failure/abort path
  - Phase 0 Library Check uses `lock status --json` to check for active builds separately from library dir existence
  - Phase 0 handles all decision matrix cases: library+lock, library+no lock, no library+lock, debris
  - Shipcheck "hold" verdict releases lock without promoting
  - Phase 6 still references `$PRESS_LIBRARY` (correct — reads from promoted location)
  - `min-binary-version` bumped in setup contract
  - No broken backtick/quote pairing in the edited markdown

- [ ] **Unit 4: Skill — Update consuming skills for new flow**

  **Goal:** Ensure polish, score, and publish skills work with the new build/library separation.

  **Requirements:** R7

  **Dependencies:** Unit 3

  **Files:**
  - Modify: `skills/printing-press-publish/SKILL.md`
  - Modify: `skills/printing-press-score/SKILL.md`
  - Modify: `skills/printing-press-polish/SKILL.md`

  **Approach:**

  **Publish skill:** Minimal changes. It uses `printing-press library list --json` (which scans `$PRESS_LIBRARY`) and `printing-press publish validate --dir <cli-dir>`. After the fix, CLIs in library are always promoted (shipcheck-passed), so publish reads complete CLIs. No path changes needed. The only change is to bump `min-binary-version` to match the version that includes `lock` subcommands (so the setup contract stays compatible).

  **Score skill:** Already uses `$PRESS_CURRENT/*.json` correctly to find current-run pointers. The `working_dir` in those pointers will initially point to `$API_RUN_DIR/working/<api>-pp-cli`. After promotion, `lock promote` updates the pointer so `working_dir` reflects the library path. If a user runs `/printing-press-score` mid-build (before promotion), it reads from runstate — this is correct. If they run it after promotion, it reads from library — also correct. Verify that the score skill's path resolution does not assume the working dir is under the current session's runstate, since runstate directories persist across sessions.

  **Polish skill:** Operates on CLIs in `$PRESS_LIBRARY` for "second-pass improvements to an existing CLI." After the fix, library only contains promoted (verified) CLIs, which is exactly what polish expects. The only consideration: if a user runs `/printing-press-polish` during an active build (before promotion), the CLI won't be in library yet. Add a note to the polish skill that if a CLI is not found in library, check if there's an active build in progress (`printing-press lock status --cli <name> --json`) and advise the user to wait or run polish after the build completes.

  **Patterns to follow:**
  - Each skill's existing path resolution logic

  **Test scenarios:**
  - Test expectation: none — skill markdown files. Verification is behavioral.

  **Verification:**
  - Publish skill's `min-binary-version` bumped
  - Score skill path resolution still works (uses pointer's `working_dir`, not hardcoded library path)
  - Polish skill has fallback guidance when CLI not yet in library

- [ ] **Unit 5: Go — Update contract tests**

  **Goal:** Ensure contract tests pass with any setup contract changes, and add contract coverage for lock behavior.

  **Requirements:** R8

  **Dependencies:** Units 1-4

  **Files:**
  - Modify: `internal/pipeline/contracts_test.go`

  **Approach:**
  - The setup contract block itself is unlikely to change (CLI_WORK_DIR is set outside the contract). But the contract tests also validate:
    - `TestGenerateHelpMentionsPublishedLibraryDefault` — may need to acknowledge that the skill now uses `$CLI_WORK_DIR` for `--output`
    - `TestREADMEOutputContract` — may need updating if README references change
    - `TestPrintingPressSkillExamplesUseCurrentCLINaming` — may need updating if naming patterns change
  - Run existing tests after Units 1-4 to see what breaks, then fix specifically

  **Patterns to follow:**
  - Existing test patterns in `contracts_test.go`

  **Test scenarios:**
  - Happy path: All existing contract tests pass after changes
  - Happy path: New test validates that SKILL.md Phase 2-5 no longer reference `$PRESS_LIBRARY/<api>-pp-cli` as `--output` target
  - Happy path: New test validates that SKILL.md Phase 2 includes `printing-press lock acquire` before generation

  **Verification:**
  - `go test ./internal/pipeline/...` passes
  - No regressions in contract validation

## System-Wide Impact

- **Interaction graph:** The main skill's Phase 2-5 output path changes from library to runstate. The publish skill, polish skill, and score skill all consume library paths — publish and polish are unaffected (they read promoted CLIs). Score reads from current-run pointers — these point to runstate during build and are updated to library after promotion via `lock promote`.
- **Error propagation:** If lock acquire fails (another session holds it), the skill should present the user with the lock status and let them decide (wait, use a different CLI name, force-reclaim, or pick a different API). Not a silent failure. Lock release is called on all known failure paths so other agents aren't blocked.
- **State lifecycle risks:** Interrupted sessions leave a stale lock in `.locks/` and a partial build in runstate. Library is untouched. Next run detects stale lock via `lock status` and offers reclaim. After successful promote, the lock is removed as part of the promote sequence.
- **API surface parity:** The `printing-press lock` subcommands are new public CLI surface. They should follow the existing JSON output convention used by other subcommands. `printing-press library list` should skip directories without `go.mod` or manifest.
- **CurrentRunPointer lifecycle:** During build, `working_dir` points to runstate. After `lock promote`, it's updated to point to library. This means the score skill gets the right path regardless of when it's invoked. If runstate is later cleaned up, the pointer still works because it reflects the library location.
- **Unchanged invariants:** `printing-press generate --output` still accepts any path. `DefaultOutputDir()` still returns the library path for direct CLI usage. The `print` pipeline's working-dir pattern is unchanged.

## Risks & Dependencies

| Risk | Mitigation |
|------|------------|
| Skill changes span many sections of a 1662-line file — easy to miss a reference | Grep for all `PRESS_LIBRARY.*-pp-cli` in the skill and verify each is addressed. Contract test validates no stale references. |
| Lock file left behind after session killed during promote | The `promote` command handles copy-to-library + release-lock as one sequence. If killed mid-copy, the library dir may have partial new files. The lock in `.locks/` goes stale and next run detects it. |
| Lock file left behind after deliberate build failure/abort | Skill calls `lock release` on all failure paths. If the model skips this instruction, staleness fallback still works. |
| The `min-binary-version` bump means older binaries won't have `lock` subcommands | The skill already handles version mismatches with a warning. Lock commands will fail with "unknown command" on old binaries — the skill should catch this and fall back to no-lock behavior. |
| Promote overwrites library dir that publish skill is currently reading | Narrow window. The publish skill is interactive and human-driven. Concurrent promote + publish for the same CLI is a user error. |
| TOCTOU window on stale lock reclaim (two agents both detect stale, both try to reclaim) | Acceptable. The `O_CREATE|O_EXCL` on the new lock file means one wins and one gets a clear error. Second agent retries and sees a fresh lock. |
| 30-minute staleness threshold may be too long for fast-failing builds | The explicit lock-release-on-failure mitigates this. The 30-minute threshold is only the fallback for truly killed sessions. |

## Sources & References

- Go binary path functions: `internal/pipeline/paths.go`
- Existing publish-to-library pattern: `internal/pipeline/publish.go:PublishWorkingCLI()`
- Existing atomic claiming: `internal/pipeline/pipeline.go:ClaimOutputDir()`
- Current run pointer: `internal/pipeline/state.go:CurrentRunPointer`
- Setup contract test: `internal/pipeline/contracts_test.go`
- Main skill: `skills/printing-press/SKILL.md`
- Publish skill: `skills/printing-press-publish/SKILL.md`
- Polish skill: `skills/printing-press-polish/SKILL.md`
- Score skill: `skills/printing-press-score/SKILL.md`
