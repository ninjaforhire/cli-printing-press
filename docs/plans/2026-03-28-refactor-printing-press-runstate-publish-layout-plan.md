---
title: "refactor: Move printing-press mutable state into .runstate and publish outputs separately"
type: refactor
status: proposed
date: 2026-03-28
origin: direct user request and design decisions from 2026-03-28 architecture discussion
---

# refactor: Move printing-press mutable state into .runstate and publish outputs separately

## Overview

Rework printing-press storage so active runs no longer depend on globally shared mutable directories. The new model separates:

- **mutable run state** in `~/printing-press/.runstate/`
- **published CLIs** in `~/printing-press/library/`
- **archived manuscripts** in `~/printing-press/manuscripts/`

The goal is to keep the repo location irrelevant, preserve worktree isolation, and stop resume/discovery logic from treating published artifacts as live working state.

## Problem Frame

The press currently mixes two concerns:

1. **Live pipeline state** that must be isolated per checkout and per run
2. **Published output** that should be globally discoverable and deduped

Flattening everything into global `library/` and `manuscripts/` fixes repo pollution but not correctness. Resume, score, proof, and discovery flows can still read or overwrite artifacts created by another worktree for the same API. Checkout-scoped top-level roots avoid that, but they conflate "where an active run writes" with "where the finished artifact lives."

The cleaner architecture is:

```text
~/printing-press/
  .runstate/
    <scope>/
      current/
        <api>.json
      runs/
        <run-id>/
          state.json
          spec.json
          working/
            <api>-pp-cli/
          research/
          proofs/
          pipeline/
          manifest.json
  library/
    <api>-pp-cli[-N]/
  manuscripts/
    <api>/
      <run-id>/
        research/
        proofs/
        pipeline/
        manifest.json
```

This makes `.runstate` the source of truth for active work. `library/` and `manuscripts/` become publish/archive destinations.

## Requirements Trace

- R1. The press must work from any clone path, subdirectory, or git worktree.
- R2. Mutable pipeline state must be isolated per checkout scope and per run.
- R3. Published CLIs must live in a single global `~/printing-press/library/` namespace with deduped directory claims.
- R4. Research, proofs, and pipeline records for a finished run must be publishable to a global manuscript archive without becoming the live resume source.
- R5. Resume, score, and skill discovery must resolve the current run from `.runstate`, not by scanning global `library/` or `manuscripts/`.
- R6. Every run must have durable metadata linking scope, run ID, git root, API name, spec source, working dir, and published outputs.
- R7. Existing users must retain compatibility with older repo-local and workspace-scoped state layouts during the migration window.
- R8. Skill docs, README, and onboarding docs must document the new contract clearly.
- R9. Contract tests must cover the path/naming/discovery rules so future docs or code changes drift less easily.

## Scope Boundaries

- No changes to scorecard scoring dimensions or verify semantics beyond path resolution.
- No change to the generated CLI naming contract: the canonical product name remains `<api>-pp-cli`.
- No migration of existing generated CLIs into new library paths; old outputs only need read compatibility.
- No cloud sync or multi-machine state sharing.
- No new interactive UX for run selection beyond the current patterns already used by the skills.

## Context & Research

### Relevant Code and Patterns

- `internal/pipeline/paths.go` currently centralizes press-home, scope, library, and manuscript path derivation.
- `internal/pipeline/state.go` currently stores pipeline state in `manuscripts/<api>/pipeline/` with legacy fallback to `docs/plans/<api>-pipeline/state.json`.
- `internal/pipeline/pipeline.go` owns default output selection, output-dir claiming, and pipeline initialization.
- `internal/pipeline/fullrun.go` and `internal/pipeline/planner.go` orchestrate run lifecycle and seed/planning files.
- `internal/cli/root.go` exposes the `print` flow and emits JSON path metadata used by skills.
- `internal/cli/vision.go` and `internal/cli/emboss.go` are the main artifact-producing helpers outside generation.
- `internal/pipeline/contracts_test.go` already acts as the main cross-layer contract test surface for naming and path rules.
- `skills/printing-press/SKILL.md`, `skills/printing-press-score/SKILL.md`, and `skills/printing-press-catalog/SKILL.md` compute path roots and are path-contract consumers, not sources of truth.

### Institutional Learnings

- `docs/solutions/best-practices/checkout-scoped-printing-press-output-layout-2026-03-28.md` captures the main failure modes to preserve:
  - hardcoded repo paths break immediately
  - global mutable output roots cross-contaminate worktrees
  - command-directory discovery must not assume the outer claimed dir name is canonical
- The recent path migration confirmed that **shared mutable state** is the real problem, not just directory-creation races.
- Contract tests are worthwhile here because the bugs are cross-layer and mostly invisible to ordinary `go test ./...` coverage.

### External Research Decision

Skipped. This is an internal storage-architecture refactor with strong local patterns and no dependency on external APIs or framework-specific behavior.

## Key Technical Decisions

### 1. Separate mutable state from published artifacts

`.runstate` is the only live source of truth for:

- active pipeline state
- current-run pointers
- in-progress research/proofs/pipeline artifacts
- working CLI trees owned by an active press run

`library/` and `manuscripts/` are publish/archive locations. Active resume and discovery logic must not depend on them by default.

### 2. Keep checkout scoping, but only inside `.runstate`

Checkout scope still matters because active runs are tied to a specific git checkout. The scope derivation pattern remains:

- derive `REPO_ROOT` from `git rev-parse --show-toplevel`
- sanitize the repo basename
- suffix with a short hash of the full repo root

That scope moves under `.runstate/<scope>/...` instead of being the top-level output layout.

### 3. Give every managed run a stable run ID and one owner

Each managed `print` run gets a generated `run_id`. That `run_id` is used for:

- `.runstate/<scope>/runs/<run-id>/...`
- `manuscripts/<api>/<run-id>/...`
- manifests and publish metadata

The implementation should use a human-sortable UTC timestamp plus a short random suffix rather than a bare UUID so manuscript directories remain inspectable.

Ownership is explicit:

- `pipeline.Init(...)` is responsible for allocating `run_id`, creating the runstate directories, writing the current pointer, and recording the working CLI path in state.
- The skill-driven phase workflow reads and writes through that state for all subsequent phase work.
- `MakeBestCLI(...)` must adopt the same run model rather than inventing a parallel layout. In autonomous mode it should either call `Init(...)` directly or a shared helper used by `Init(...)`, then write research, generation output, proofs, and publish results into that same run root.

There should be one run model, not one for `print` and another for `MakeBestCLI`.

### 4. Track “current run” per API, not one global current pointer

Use:

```text
.runstate/<scope>/current/<api>.json
```

This avoids ambiguity when the same checkout is actively generating multiple APIs. The current pointer should contain at least:

- `api_name`
- `run_id`
- `scope`
- `git_root`
- `working_dir`
- `state_path`
- `updated_at`

### 5. Treat pipeline-generated CLI trees as working directories until publish

The full press pipeline should generate into:

```text
.runstate/<scope>/runs/<run-id>/working/<api>-pp-cli/
```

Only the publish step should claim a global `library/<api>-pp-cli[-N]/` directory and copy/promote the finished CLI there.

This keeps iterative emboss/review/fix loops from mutating the globally published artifact in place.

### 6. Archive manuscripts by API and run ID after publish

When a run reaches ship or an explicit publish boundary, copy the durable artifacts needed for inspection into:

```text
manuscripts/<api>/<run-id>/
```

At minimum this archive should contain:

- `research/`
- `proofs/`
- `pipeline/`
- `manifest.json`

The archive is intended for inspection, historical comparison, and later export. It is not the primary resume source, and it is not updated continuously during active execution.

### 7. Preserve direct one-shot CLI generation behavior

`printing-press generate` is a direct artifact-producing command, not a multi-phase managed run. Its default output should remain publish-oriented:

- default path: `~/printing-press/library/<api>-pp-cli[-N]/`
- explicit `--output` still wins

The `.runstate` layout is primarily for managed `print` runs and the supporting skills that need safe mutable state.

### 7.5. Emboss must treat published CLIs as input, not mutable state

`emboss` currently assumes it can write baseline files into the target CLI directory. Under the new contract, published `library/` artifacts should not be the writable working area.

The rule is:

- If `emboss --dir` points at a `.runstate/.../working/...` directory, emboss operates in place on that active run.
- If `emboss --dir` points at a published `library/...` directory, emboss starts a new managed run in `.runstate/<scope>/runs/<run-id>/`, copies the published CLI into that run's `working/` directory, writes baseline/proof artifacts there, and publishes a new claimed CLI on success.
- `emboss` must never mutate a previously published library directory in place.

This keeps published outputs immutable while still supporting iterative second-pass improvement workflows.

### 8. Publish a manifest everywhere state crosses boundaries

Both `.runstate/.../manifest.json` and archived `manuscripts/.../manifest.json` should record:

- `api_name`
- `run_id`
- `scope`
- `git_root`
- `git_commit` if available
- `spec_path`
- `spec_url`
- `working_dir`
- `published_cli_dir`
- `archived_manuscript_dir`
- timestamps

This gives future tooling a stable contract for lookup, auditing, and possible cleanup.

### 9. Compatibility should be read-first, not migration-first

On initial rollout:

- read from new `.runstate` first
- fall back to workspace-scoped manuscript state if present
- finally fall back to legacy repo `docs/plans/*-pipeline/state.json`

Do not attempt an eager filesystem migration of existing outputs. Instead, lazily re-save into the new layout when an old run is resumed.

## Open Questions

### Resolved During Planning

- **Do we still need checkout scope?** Yes, but only for active mutable state under `.runstate`.
- **Can global `library/` stay shared?** Yes. Claimed-dir deduping is enough there because published CLIs are immutable outputs.
- **Should global `manuscripts/` be shared?** Yes, if manuscripts are archived by run ID and active resume never treats them as the current writable state.

### Deferred to Implementation

- Exact manifest schema versioning strategy
- Whether future CLI commands should expose `--run-id` for manual resume/debug flows

## Implementation Units

- [ ] **Unit 1: Introduce runstate path helpers and publish/archive path helpers**

  **Goal:** Centralize the new storage contract in one place so code and docs have a canonical source of truth.

  **Requirements:** R1, R2, R3, R4, R6

  **Dependencies:** None

  **Files:**
  - Modify: `internal/pipeline/paths.go`
  - Modify: `internal/pipeline/paths_test.go`
  - Modify: `internal/pipeline/contracts_test.go`

  **Approach:**
  - Keep `PressHome()` and scope derivation, but add helpers for:
    - `RunstateRoot()`
    - `ScopedRunstateRoot()`
    - `CurrentRunDir()`
    - `CurrentRunPointerPath(apiName string)`
    - `RunRoot(runID string)`
    - `WorkingCLIDir(apiName, runID string)`
    - `RunResearchDir(apiName, runID string)` or `RunResearchDir(runID string)`
    - `RunProofsDir(...)`
    - `RunPipelineDir(...)`
    - `PublishedLibraryRoot()`
    - `PublishedManuscriptsRoot()`
    - `ArchivedManuscriptDir(apiName, runID string)`
  - Keep naming logic out of path helpers; continue to use `internal/naming/` for `<api>-pp-cli`.
  - Update contract tests to assert the new canonical layout strings in docs and skills.

  **Patterns to follow:**
  - Existing `PressHome()` and scope sanitization in `internal/pipeline/paths.go`
  - Existing cross-layer contract approach in `internal/pipeline/contracts_test.go`

  **Test scenarios:**
  - Press home override via env var still works
  - Scope sanitization remains stable
  - New runstate paths are deterministic for a fixed scope/run ID
  - Docs contract strings point to `.runstate`, `library/`, and `manuscripts/<api>/<run-id>/`

  **Verification:**
  - `go test ./internal/pipeline -run 'Test.*Path'`

- [ ] **Unit 2: Move pipeline state and current-run discovery into `.runstate`**

  **Goal:** Make `.runstate` the live source of truth for init, resume, and current-run discovery.

  **Requirements:** R2, R5, R6, R7

  **Dependencies:** Unit 1

  **Files:**
  - Modify: `internal/pipeline/state.go`
  - Modify: `internal/pipeline/state_test.go`
  - Modify: `internal/pipeline/pipeline.go`
  - Modify: `internal/pipeline/planner.go`
  - Modify: `internal/pipeline/fullrun.go`

  **Approach:**
  - Extend `PipelineState` with `RunID`, `Scope`, and runstate-specific directories.
  - Save state in `.runstate/<scope>/runs/<run-id>/state.json`.
  - Write/update `.runstate/<scope>/current/<api>.json` on init and after phase transitions.
  - Update `Init`, `LoadState`, and `StateExists` so they resolve through `.runstate` first.
  - Preserve lazy fallback loading from older workspace-scoped and repo-local state files.
  - Ensure plan seed files live under the runstate pipeline dir for the active run.
  - Make the active working CLI path a first-class field in state so phase execution and autonomous full-run code use the same location contract.

  **Patterns to follow:**
  - Existing lazy migration pattern in `LoadState`
  - Existing stable phase filename logic in `PlanFilename`

  **Test scenarios:**
  - New run creates runstate state and current-pointer files
  - Resume prefers `.runstate` over legacy locations
  - Legacy state can still be resumed and re-saved into the new layout
  - Two different scopes can hold current runs for the same API without collision

  **Verification:**
  - `go test ./internal/pipeline -run 'Test(State|Init|Resume)'`

- [ ] **Unit 3: Publish finished CLIs and manuscripts from runstate**

  **Goal:** Keep active runs isolated while still producing globally discoverable outputs.

  **Requirements:** R3, R4, R6, R7

  **Dependencies:** Unit 2

  **Files:**
  - Modify: `internal/pipeline/fullrun.go`
  - Modify: `internal/pipeline/pipeline.go`
  - Modify: `internal/cli/root.go`
  - Test: `internal/pipeline/fullrun_test.go`
  - Test: `internal/cli/claim_integration_test.go`

  **Approach:**
  - For managed `print` runs, generate into `.runstate/.../working/<api>-pp-cli/`.
  - Update `MakeBestCLI(...)` to initialize or accept a managed run context and use that run root for research, generation, dogfood, verification, and scorecard artifacts.
  - Keep `ClaimOutputDir()` for publish-time claiming into global `library/`.
  - Add a publish step that copies the finished working tree into `library/<api>-pp-cli[-N]/`.
  - Archive research/proofs/pipeline artifacts into `manuscripts/<api>/<run-id>/` only at publish time, not continuously during every phase.
  - Write manifests after publish so downstream tooling can correlate working, published, and archived paths.
  - Keep `generate` command defaulting directly to global `library/` to avoid changing the one-shot CLI workflow.

  **Patterns to follow:**
  - Existing deduping behavior in `ClaimOutputDir`
  - Existing JSON output from `print` in `internal/cli/root.go`

  **Test scenarios:**
  - Managed pipeline run writes working files to `.runstate`
  - Publish copies the CLI to deduped global library dir
  - Re-running the same API publishes to `...-2` without mutating the first published CLI
  - Manuscript archive includes the expected subdirectories and manifest

  **Verification:**
  - `go test ./internal/pipeline ./internal/cli -run 'Test.*(Publish|Claim|FullRun)'`

- [ ] **Unit 4: Update helper commands and skills to resolve active runs from `.runstate`**

  **Goal:** Ensure research, scoring, embossing, and skill-driven flows stop inferring “current” from published outputs.

  **Requirements:** R1, R4, R5, R8

  **Dependencies:** Unit 2

  **Files:**
  - Modify: `internal/cli/vision.go`
  - Modify: `internal/cli/emboss.go`
  - Modify: `skills/printing-press/SKILL.md`
  - Modify: `skills/printing-press-score/SKILL.md`
  - Modify: `skills/printing-press-catalog/SKILL.md`
  - Modify: `README.md`
  - Modify: `ONBOARDING.md`

  **Approach:**
  - Update skill setup blocks so they compute `REPO_ROOT`, derive scope, and resolve `.runstate/<scope>` as the active home.
  - Change score/resume/current-run instructions to inspect `.runstate/<scope>/current/` first.
  - Keep helper commands path-driven, but define default resolution explicitly:
    - `vision --api <name>` writes into the current runstate research dir for that API when a current pointer exists; otherwise it should fail fast with a message to run `print` first or pass `--output`.
    - `emboss --dir <path>` accepts explicit dirs, but a published library dir must be copied into a new runstate working dir before emboss writes any baseline or proof artifacts.
  - Document the distinction between:
    - active runstate
    - published library
    - archived manuscripts
  - Keep direct path-based commands working when users pass `--dir` or `--output`.

  **Patterns to follow:**
  - Existing contract markers in skill setup blocks
  - Existing doc contract tests in `internal/pipeline/contracts_test.go`

  **Test scenarios:**
  - Skills mention `.runstate/<scope>` for active work
  - README explains where active vs published artifacts live
  - No docs instruct users to resume by scanning global manuscripts as the first step

  **Verification:**
  - `go test ./internal/pipeline -run 'Test.*Contract'`

- [ ] **Unit 5: Harden compatibility and end-to-end contract coverage**

  **Goal:** Make future regressions harder by testing the storage contract across code, docs, and claimed output dirs.

  **Requirements:** R7, R8, R9

  **Dependencies:** Units 1-4

  **Files:**
  - Modify: `internal/pipeline/contracts_test.go`
  - Modify: `internal/pipeline/runtime_test.go`
  - Modify: `internal/pipeline/state_test.go`
  - Modify: `internal/cli/emboss_test.go`
  - Modify: `internal/cli/vision_test.go`

  **Approach:**
  - Extend contract tests to assert:
    - current-run lookup is `.runstate`-first
    - docs describe global `library/` plus archived `manuscripts/`
    - managed runs generate into runstate working dirs
  - Keep the existing claimed-dir test for `...-pp-cli-2`
  - Add migration tests that load legacy workspace-scoped and repo-local state files into the new runstate layout

  **Patterns to follow:**
  - Existing contract test style: assert a few stable strings and behaviors, not full markdown snapshots

  **Test scenarios:**
  - Generate same API twice, publish twice, and verify both outputs remain valid
  - Legacy state resume path still works
  - Current-run resolution never selects a published CLI from another scope as the active run

  **Verification:**
  - `go test ./internal/pipeline ./internal/cli`
  - `go test ./...`

## System-Wide Impact

- **Interaction graph:** `print` becomes explicitly run-oriented. It owns a mutable runstate area and publishes outward on success. `generate` remains a direct artifact generator into the global library.
- **Error propagation:** Publish/copy failures need explicit handling because working output may exist even if archive or library publish fails. The plan should preserve runstate for recovery rather than deleting it on partial publish failure.
- **State lifecycle risks:** The biggest risk is confusing active and published paths during migration. That is why `.runstate` must be the only default resume/discovery source.
- **Operational behavior:** Users gain a clearer mental model:
  - active work lives in `.runstate`
  - finished CLIs live in `library`
  - archived research/proofs live in `manuscripts`

## Risks & Dependencies

| Risk | Mitigation |
|------|------------|
| Helper commands or skills still scan global manuscripts for “current” | Add contract tests around current-run lookup and skill setup blocks |
| Publish step copies a partially broken working tree into library | Publish only after existing verify/ship gates succeed; keep manifest so failures are inspectable |
| Legacy state becomes unreadable after the refactor | Keep read fallback to workspace-scoped and repo-local layouts; test migration paths explicitly |
| Users assume `library/` is the active working tree | Document the difference clearly in README, onboarding, and skills |
| Multiple current runs for the same API in one scope overwrite pointers | Define current pointers as “latest active run for this API” and keep run history under `runs/` |

## Sources & References

- Existing path contract and failure analysis: `docs/solutions/best-practices/checkout-scoped-printing-press-output-layout-2026-03-28.md`
- Prior output-dir planning context: `docs/plans/2026-03-27-020-feat-cli-output-to-library-plan.md`
- Relevant code:
  - `internal/pipeline/paths.go`
  - `internal/pipeline/state.go`
  - `internal/pipeline/pipeline.go`
  - `internal/pipeline/fullrun.go`
  - `internal/cli/root.go`
  - `internal/cli/vision.go`
  - `internal/cli/emboss.go`
  - `internal/pipeline/contracts_test.go`
