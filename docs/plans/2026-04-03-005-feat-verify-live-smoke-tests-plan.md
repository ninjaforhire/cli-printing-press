---
title: "feat: Add live smoke tests to verify pipeline"
type: feat
status: active
date: 2026-04-03
---

# Add Live Smoke Tests to Verify Pipeline

## Overview

Add a `--smoke` flag to the `verify` command that runs a small set of live read-only API calls to catch request-shape bugs (wrong param names, broken auth headers, incorrect paths) before the CLI ships. Today, verify tests `--help`, `--dry-run`, and mock execution — none of which send a real HTTP request. Bugs like sending `{"q": "steam"}` instead of `{"queryText": "steam"}` pass all existing checks and only surface when the user manually runs the command.

## Problem Frame

The verify pipeline has two modes: mock (default) and live (`--api-key`). Mock mode starts an `httptest.Server` that returns synthetic JSON — it validates CLI plumbing (flags parsed, output formatted) but cannot detect wrong API paths, wrong parameter names, or broken auth headers. Live mode (`--api-key`) runs ALL commands against the real API, which is heavy, requires auth for every API, and conflates "does the request shape work?" with "does every command work end-to-end?"

The gap is a lightweight middle ground: send 2-4 real HTTP requests to verify the request shape is correct, without running the full command suite against the live API. This catches the class of bugs where the CLI compiles, help works, dry-run shows the right shape, but the actual API rejects the request.

## Requirements Trace

- R1. `verify --smoke` runs 2-4 live read-only API requests against representative commands
- R2. Smoke tests only run read-only operations (GET, search). No mutations.
- R3. Smoke tests check for HTTP 2xx/3xx success — any 4xx/5xx fails the smoke test with the response body in the report
- R4. Smoke test results appear in the `VerifyReport` alongside existing help/dry-run/exec results
- R5. `--smoke` requires either `--api-key` or a reachable unauthenticated API — error clearly if neither
- R6. Existing `--api-key` full-live mode is unchanged. `--smoke` is additive.
- R7. The printing-press skill's shipcheck phase can pass `--smoke` when an API key is available

## Scope Boundaries

- **Not changing mock mode.** Default `verify` (no flags) continues to use the mock server.
- **Not replacing `--api-key` mode.** Full live verification continues to work as before.
- **Not testing write commands.** Smoke tests are read-only.
- **Not adding retry logic to smoke tests.** Single attempt per command. Transient failures are acceptable — smoke tests catch shape bugs, not availability issues.
- **Not auto-discovering which commands to smoke test.** The selection is deterministic based on command classification.

## Context & Research

### Relevant Code and Patterns

- `internal/pipeline/runtime.go` — `RunVerify()`, `VerifyConfig`, `VerifyReport`, `CommandResult`, command discovery, classification, mock server setup
- `internal/pipeline/runtime.go:473` — command classification: hardcoded data-layer (sync, search), local (doctor, auth), read (tail), spec-based method inference
- `internal/pipeline/runtime.go:441` — synthetic argument values for positional args (maps placeholder names to test values)
- `internal/cli/verify.go` — CLI flag definitions: `--dir`, `--spec`, `--api-key`, `--env-var`, `--threshold`, `--fix`, `--json`, `--cleanup`
- `internal/pipeline/fixloop.go` — fix loop pattern (iterative test → fix → re-test)
- `internal/pipeline/workflow_verify.go` — workflow verification with `mode: live` and 3-retry logic for transient errors

### Institutional Learnings

- **Postman Explore search bug** (this session): `search "steam"` sent `{"q": "steam"}` instead of `{"queryText": "steam"}` because the profiler didn't recognize `queryText`. Passed all verify checks. Would have been caught by a single live search request.
- **Verify gap for local-data commands** (Postman Explore Run 2 retro): Commands needing local SQLite DB can't be tested during verify without a prior sync. Smoke tests should skip data-layer commands that require local state.

## Key Technical Decisions

- **`--smoke` is a separate flag, not a mode of `--api-key`**: `--api-key` runs ALL commands live (existing behavior). `--smoke` runs only 2-4 representative commands. They can be combined (`--smoke --api-key KEY`) or `--smoke` can work without a key for unauthenticated APIs.

- **Command selection is deterministic**: Pick one command from each category that exists: (1) the first `read` GET command, (2) the `search` command if it exists, (3) the `sync` command if it exists (with `--resources <first-resource>` to limit scope). No randomness — reproducible across runs.

- **Smoke results are a new field on `VerifyReport`**: Add `SmokeResults []SmokeResult` to the report. The existing `Verdict` computation considers smoke failures as warnings (not hard failures) so they don't block shipping CLIs that work in mock mode but have a flaky API.

- **Smoke tests use the same environment setup as live mode**: Auth env vars, base URL override, timeout. The infrastructure already exists in `RunVerify()`.

## Open Questions

### Resolved During Planning

- **Should smoke failures affect the verdict?** Yes, but as warnings only. A smoke failure means the request shape may be wrong, but could also be a transient API issue. The verdict should note "smoke: WARN" but not downgrade a PASS to FAIL solely on smoke results.

- **How does `--smoke` interact with `--fix`?** Smoke tests run after the fix loop, not during it. The fix loop repairs plumbing issues (dry-run, help). Smoke tests validate request shape, which the fix loop can't repair automatically.

### Deferred to Implementation

- **Exact timeout for smoke requests.** The existing live mode uses 15 seconds. Smoke tests may want a shorter timeout (5-10 seconds) since they're meant to be quick. Determine during implementation.

## Implementation Units

- [ ] **Unit 1: Add `--smoke` flag and `SmokeResult` type**

**Goal:** Add the `--smoke` CLI flag and the data types for smoke test results.

**Requirements:** R1, R4, R6

**Dependencies:** None

**Files:**
- Modify: `internal/cli/verify.go`
- Modify: `internal/pipeline/runtime.go`
- Test: `internal/pipeline/runtime_test.go`

**Approach:**
- Add `Smoke bool` field to `VerifyConfig`
- Add `--smoke` flag to the verify CLI command
- Add `SmokeResult` struct: `Command string`, `Path string`, `Method string`, `StatusCode int`, `Success bool`, `Error string`, `ResponseSnippet string`
- Add `SmokeResults []SmokeResult` and `SmokePass bool` fields to `VerifyReport`
- Pass `Smoke` through to `RunVerify()`

**Patterns to follow:**
- Existing `APIKey` field on `VerifyConfig` and `--api-key` flag registration in `internal/cli/verify.go`
- Existing `CommandResult` struct pattern for per-command results

**Test scenarios:**
- Happy path: `VerifyConfig{Smoke: true}` is accepted, `SmokeResults` field is present on report
- Happy path: `VerifyConfig{Smoke: false}` (default) produces report with nil/empty `SmokeResults`
- Edge case: `--smoke` without `--api-key` on an API that requires auth → clear error message

**Verification:**
- `printing-press verify --help` shows `--smoke` flag
- Generated report JSON includes `smoke_results` field when `--smoke` is set

---

- [ ] **Unit 2: Implement smoke test execution**

**Goal:** Run 2-4 live read-only requests against representative commands and record results.

**Requirements:** R1, R2, R3, R5

**Dependencies:** Unit 1

**Files:**
- Modify: `internal/pipeline/runtime.go`
- Test: `internal/pipeline/runtime_test.go`

**Approach:**
- Add `runSmokeTests()` function called from `RunVerify()` when `cfg.Smoke` is true
- Command selection: from the discovered commands, pick deterministically:
  1. First command classified as `read` with no required positional args (simplest GET)
  2. `search` command if it exists (the exact bug class this feature catches)
  3. `sync` command if it exists (with `--resources <first-resource> --full` to limit scope, 10-second timeout)
- For each selected command, run the CLI binary with real API credentials (same env setup as live mode) and capture the exit code plus stderr/stdout
- Parse the HTTP status from the output — if the command exits 0, it's a pass. If it exits non-zero, capture the error message (which includes the HTTP status from `classifyAPIError`)
- For search: run with a generic query like `"test"` — the goal is to verify the request shape, not find results
- Skip smoke tests entirely if no API key is provided AND the API requires auth (determined from spec or VerifyConfig)

**Patterns to follow:**
- Existing `runCommandTests()` function for CLI invocation with environment and timeout
- Existing `runDataPipelineTest()` for sync invocation pattern
- Existing synthetic argument values map for generating test inputs

**Test scenarios:**
- Happy path: API reachable, all smoke commands return 2xx → `SmokePass: true`, each `SmokeResult.Success: true`
- Happy path: unauthenticated API, no `--api-key` needed → smoke tests run against live API without auth
- Error path: search command returns 400 (wrong param name) → `SmokeResult{Success: false, StatusCode: 400, Error: "..."}` with response body snippet
- Error path: API unreachable (DNS/timeout) → `SmokeResult{Success: false, Error: "connection refused"}` — not confused with a request-shape bug
- Edge case: CLI has no `search` command → only GET and sync are tested (or just GET if no sync)
- Edge case: CLI has no `read` commands at all → smoke returns empty results with a note, not an error

**Verification:**
- Run `verify --smoke --api-key <key>` on a generated CLI with a working API → smoke results show in report
- Run `verify --smoke` on a CLI for an unauthenticated API → smoke tests run without a key

---

- [ ] **Unit 3: Integrate smoke results into verdict and reporting**

**Goal:** Surface smoke results in the human-readable and JSON reports, and factor them into the verdict as warnings.

**Requirements:** R3, R4

**Dependencies:** Unit 1, Unit 2

**Files:**
- Modify: `internal/pipeline/runtime.go` (verdict computation)
- Modify: `internal/cli/verify.go` (report rendering)
- Test: `internal/pipeline/runtime_test.go`

**Approach:**
- In the verdict computation section of `RunVerify()`, if smoke tests ran and any failed, append "smoke failures" to the report but do NOT downgrade PASS to FAIL. If the mock/live verdict is PASS but smoke failed, set verdict to "PASS (smoke: WARN)".
- In the human-readable report output, add a "Smoke Tests" section after the per-command table showing each smoke result with command name, HTTP status, and pass/fail
- In the JSON report, include `smoke_results` array and `smoke_pass` boolean

**Patterns to follow:**
- Existing verdict derivation logic in `RunVerify()` (PassRate, DataPipeline, Critical thresholds)
- Existing human report rendering in `internal/cli/verify.go` (tabular command results)

**Test scenarios:**
- Happy path: all smoke tests pass → verdict unchanged, smoke section shows all green
- Happy path: smoke tests fail but mock tests pass → verdict "PASS (smoke: WARN)", report shows which smoke commands failed with HTTP status
- Edge case: `--smoke` not set → no smoke section in report, verdict unaffected
- Happy path: JSON output includes `smoke_results` array with `command`, `status_code`, `success`, `error` fields

**Verification:**
- Human report shows smoke test table when `--smoke` is used
- JSON report includes `smoke_results` and `smoke_pass`
- Verdict computation correctly warns on smoke failure without blocking

---

- [ ] **Unit 4: Wire `--smoke` into printing-press skill shipcheck**

**Goal:** The printing-press skill passes `--smoke` to verify during shipcheck when an API key is available.

**Requirements:** R7

**Dependencies:** Unit 3

**Files:**
- Modify: `skills/printing-press/SKILL.md` (Phase 4 shipcheck verify invocation)

**Approach:**
- In the Phase 4 shipcheck section, update the `printing-press verify` invocation to include `--smoke` when an API key is available (same condition that currently gates Phase 5 live smoke testing)
- The skill already tracks whether an API key was provided — use that flag
- Update the shipcheck interpretation to note smoke warnings in the shipcheck report

**Test expectation:** none — skill file edit, no behavioral code change. Verified by running a full printing-press session with an API key and confirming the shipcheck report includes smoke results.

## System-Wide Impact

- **Interaction graph:** `--smoke` integrates into the existing `RunVerify()` flow. It runs after command discovery and classification (which it depends on) and before verdict computation (which it feeds into). The fix loop is unaffected — smoke runs after fixes, not during.
- **Error propagation:** Smoke failures are warnings, not errors. They appear in the report but don't block the verdict. This is intentional — transient API issues shouldn't block shipping.
- **Unchanged invariants:** Mock mode, `--api-key` full-live mode, `--fix` loop, workflow-verify, dogfood, and scorecard are all unchanged. `--smoke` is purely additive.

## Risks & Dependencies

| Risk | Mitigation |
|------|------------|
| Smoke tests flaky due to API rate limits | Single attempt, no retry. Smoke failures are warnings, not blockers. The report includes the HTTP status so the user can distinguish rate limits (429) from shape bugs (400). |
| Smoke tests slow down verify | At most 3 commands with 10-15 second timeouts = 45 seconds worst case. Acceptable for an opt-in flag. |
| Search smoke test returns 0 results (not a bug) | The test checks for HTTP 2xx, not result count. Zero results with 200 is a pass — the request shape was correct. |

## Sources & References

- Related code: `internal/pipeline/runtime.go` (RunVerify, VerifyConfig, VerifyReport, command discovery)
- Related code: `internal/cli/verify.go` (CLI flag definitions, report rendering)
- Related code: `internal/pipeline/workflow_verify.go` (live mode retry pattern — reference but not used for smoke)
- Retro finding: Postman Explore `queryText` bug — the exact bug class this feature prevents
