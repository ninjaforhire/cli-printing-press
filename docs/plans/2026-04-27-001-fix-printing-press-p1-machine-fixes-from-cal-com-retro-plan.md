---
title: "fix: Printing Press P1 machine fixes from Cal.com retro #334"
type: fix
status: active
date: 2026-04-27
origin: https://github.com/mvanhorn/cli-printing-press/issues/334
deepened: 2026-04-27
---

# fix: Printing Press P1 machine fixes from Cal.com retro #334

## Overview

Cal.com Run 3 surfaced 12 findings; ten landed in the Do bucket. WU-10 was already fixed by PR #332 ahead of this plan. This plan covers the remaining five P1 work units (WU-1 through WU-5).

This plan was rewritten after document review surfaced three factual errors in the first draft (WU-2 misdiagnosed the root cause, WU-4 named a template that doesn't have the relevant handler, WU-1 picked a design that can't actually look up templated paths at runtime). The diagnostic work is recorded in **Key Technical Decisions** so the implementer doesn't have to re-derive it.

The five units touch separate subsystems and can ship in any order, but three of them (U1, U4, U5) modify generator templates that the still-in-flight allrecipes-retro plan (`docs/plans/2026-04-26-002-feat-printing-press-p1-machine-fixes-plan.md`) is also touching — the dependency table calls out the conflict surface so reviewers can sequence merges.

## Problem Frame

The Cal.com regenerate against the live API hit five distinct machine defects that required hand-patching before the printed CLI worked:

1. `bookings get --data-source live` returned `{"bookings":[],"totalCount":0}` despite 5 real bookings on the account. The `cal-api-version: 2024-08-13` header never landed on the wire because the store-backed read path (`{{- if .HasStore}}`) calls `resolveRead(c, flags, ..., path, params)` which has no `headers` parameter, while only the non-store branch wires up `c.GetWithHeaders(path, params, headerOverrides)`. The template builds the `headerOverrides` map correctly per-endpoint at codegen time; only the store-side caller drops it.
2. `printing-press publish validate` failed the transcendence check ("no novel features recorded") on every CLI through publish even when `research.json` had 12 verified `novel_features_built`. Diagnosed during plan review: `writeCLIManifestForPublish` calls `LoadResearch(state.PipelineDir())` which reads `<RunRoot>/pipeline/research.json` — but the printing-press skill writes research.json to `<RunRoot>/research.json` directly (the `pipeline/` subdirectory is reserved for `printing-press print`'s phase artifacts, not the skill flow). The file-not-found is silent. Compounding this, the carry-forward block at `publish.go:208-219` preserves MCP and Auth fields from the existing manifest but does NOT preserve `existing.NovelFeatures`, so even when generate populated novel_features at codegen time, publish strips them.
3. Five proof files contained real attendee names, emails, and the account username. Library is public — would have shipped real PII for the second time (PR #52 likely had the same issue and went unnoticed). Existing scrub catches API key VALUES via "Exact-value scan" plus inline prose "Workspace & organization PII redaction" guidance, but the prose path requires a human to follow it during run-time and has now demonstrably failed twice.
4. `auth set-token <token>` silently had no effect when `config.toml` already had `auth_header` populated (the common regen scenario). API calls returned 401 because `Config.AuthHeader()` reads `auth_header` first. Diagnosed during plan review: the set-token handler lives ONLY in `auth_simple.go.tmpl` (`auth.go.tmpl` is the OAuth template — no set-token there).
5. `sync --full` ingested only 5 of 11 Cal.com resources. Errors are silent (sync exits 0 with summary errors). Failed resources have row count 0 in the local store. Cal.com uses 4 distinct envelope shapes; the runtime extractor in the generated CLI knows only `data.<resource>: [...]`. The OpenAPI spec already contains response-schema info that could drive per-resource extraction, but the parser doesn't surface it onto `profiler.SyncableResource` and the sync template doesn't read it.

Each failure mode is silent enough that several survived prior cal-com runs (including PR #52). The retro elevates them to P1 because they affect generation reliability across the full catalog, not Cal.com alone.

## Requirements Trace

- R1. Per-endpoint required headers detected by the OpenAPI parser must be sent on every request, including store-backed reads. Verified end-to-end against Cal.com bookings (covers U1, fixes regression of #135).
- R2. `lock promote` must produce a `.printing-press.json` with `novel_features` populated whenever `research.json` has `novel_features_built` populated, regardless of where research.json lives within the run directory (covers U2).
- R3. The publish skill must scrub PII captured during dogfood — emails, bearer-token tails, capitalized name patterns — before uploading to a public repository. The scrub must default to safe behavior (warn) over information-loss behavior (auto-redact) for ambiguous patterns (covers U3).
- R4. `auth set-token <token>` must change the active credential. Subsequent API calls must use the new token, regardless of whether `config.toml` had a legacy `auth_header` field. The fix must not regress golden harness fixtures (covers U4).
- R5. `sync --full` must ingest all envelope shapes the spec declares (`data:[]`, `data:<object>`, `data:<key>:[...]`, nested grouped shapes), not only `data.<resource>: [...]`. Falls back to current heuristic when the spec is genuinely ambiguous, with a warning (covers U5).

**Origin:** [Issue #334](https://github.com/mvanhorn/cli-printing-press/issues/334) and [retro document](https://files.catbox.moe/itg03h.md). Local copy at `/tmp/printing-press/retro/20260427-005908-cal-com-retro.md`.

## Scope Boundaries

- This plan does NOT address WU-6 (operationId-derived path components), WU-7 (body-field --json flags), WU-8 (publish package run_id selection), or WU-9 (legacy NOT NULL migration). Those are P2 items in the retro and warrant separate plans.
- This plan does NOT touch WU-10 (auth_protocol scorer) — already fixed by PR #332.
- This plan does NOT redesign the storage or sync architecture. Envelope handling adds spec-driven extraction without changing how rows are persisted or how the resources table is keyed.
- This plan does NOT introduce a new MCP server, new CLI subcommand, or change the publish PR template beyond the PII scrub.
- This plan does NOT add generic "silent-failure detection" to dogfood (the meta-fix that would catch the next WU-1-shaped bug — see Deferred to Follow-Up Work). Surfacing this honestly because the retro called it out as the higher-leverage move.

### Deferred to Follow-Up Work

- **Dogfood silent-success detection** (e.g., flag HTTP 200 + empty list when the dogfood matrix or research.json declares the account has N>0 of the resource). Would have caught U1 and U5 BEFORE Cal.com Run 3 generated this retro. Separate plan, separate PR.
- **Manifest-merge helper that walks struct tags** to carry non-empty fields forward by default in `writeCLIManifestForPublish`. Eliminates the class of bug where adding a new manifest field requires manual sync between two write sites. The two-write-site architecture is the deeper root cause of the U2 class — fixing it generally is out of scope here; U2 patches the specific symptom.

## Context & Research

### Relevant Code and Patterns

- `internal/generator/templates/command_endpoint.go.tmpl` lines 148-204: builds `headerOverrides := map[string]string{ ... }` correctly per-endpoint at codegen time, then forks on `{{- if .HasStore}}`. The non-store branch correctly passes `headerOverrides` to `c.GetWithHeaders`. The store branch calls `resolveRead(c, flags, ..., path, params)` and drops the map. U1 fix lives here and in `data_source.go.tmpl`.
- `internal/generator/templates/data_source.go.tmpl` line 88: `resolveRead(c *client.Client, flags *rootFlags, resourceType string, isList bool, path string, params map[string]string) (json.RawMessage, DataProvenance, error)`. Same gap on `resolvePaginatedRead`. The signature change is the U1 mechanism.
- `internal/generator/templates/client.go.tmpl` lines 247-457: `c.Get` → `c.GetWithHeaders(path, params, nil)` → `do(method, path, params, body, headerOverrides)`. The merge-into-request loop already handles caller-supplied headers correctly (lines 453-459). Once `resolveRead` accepts and forwards a headers parameter, the existing template branches on lines 199-201 stay byte-identical.
- `internal/openapi/parser.go` lines 482-656: `detectRequiredHeaders` returns `(globalRequired []spec.RequiredHeader, perEndpointHeaders map[string]map[string]string)`. `applyHeaderOverrides` (line 620) distributes per-endpoint values onto each `Endpoint.HeaderOverrides`. The per-endpoint override is ALREADY available at codegen time as a literal map in the template — no new parser data is needed for U1.
- `internal/pipeline/publish.go` lines 195-275 (`writeCLIManifestForPublish`): the carry-forward block (lines 207-219) preserves MCP and Auth fields but NOT NovelFeatures. The LoadResearch call (lines 265-274) tries `<RunRoot>/pipeline/research.json` via `state.PipelineDir()`. U2 fix lives here.
- `internal/pipeline/research.go` line 259: `func LoadResearch(pipelineDir string) (*ResearchResult, error)` reads `<dir>/research.json`. Caller passes the wrong directory; U2 fixes the resolution.
- `internal/pipeline/paths.go` line 92: `RunPipelineDir(runID)` returns `<RunRoot>/pipeline/`. The skill writes research.json to `<RunRoot>/research.json` (the run root itself, not the pipeline subdir). Two parallel conventions; U2 must reconcile.
- `internal/pipeline/climanifest.go` line 47-65: `CLIManifest.NovelFeatures []NovelFeatureManifest` field already exists with `omitempty`. Marshal logic is fine; only the population path is missing for the publish/promote case.
- `internal/generator/templates/auth_simple.go.tmpl` lines 31, 57, 69-90: `newAuthSetTokenCmd` lives here. `auth.go.tmpl` is OAuth-only — no set-token. U4 must target `auth_simple.go.tmpl`.
- `internal/generator/templates/config.go.tmpl` lines 91-128 + 183-190: `Config.AuthHeader()` reads `AuthHeaderVal` first; `SaveTokens(...)` does not touch `AuthHeaderVal`. U4 needs to set `cfg.AuthHeaderVal = ""` before the SaveTokens call (or extend SaveTokens with a clear-legacy flag).
- `skills/printing-press/references/secret-protection.md`: actual file structure is "Exact-value scan before archiving" / "Strip auth from HAR captures before archiving" / "API key handling during the run" / "Workspace & organization PII redaction" / "Session state cleanup". There are NO "Layer 1/Layer 2" headings — adding "Layer 3" would orphan the labels. U3 must restructure with new section headings or add a properly-named sibling.
- `skills/printing-press-publish/SKILL.md` Step 6 is "Package", and the secret-handling content is in an unnumbered section near line 784 ("Secret & PII Protection"). U3's invocation site lives in that section, not Step 6.
- `internal/profiler/profiler.go` lines 58-61: `SyncableResource struct { Name string; Path string }`. No envelope-shape field. U5 needs to add one and populate from parser output.
- `internal/spec/spec.go`: `Endpoint.Response` exists (`*ResponseDef`) but the sync template never reads it (`grep` returns 0 hits). U5 must extend BOTH the data-flow (parser → spec.APISpec → profiler) AND the sync template's consumer code.
- `internal/generator/templates/sync.go.tmpl` line 446 (`extractPageItems`) and line 352 (`db.UpsertBatch(resource, items)`): the runtime envelope walker. Heuristic falls back to "exactly one array key" when `data.<resource>` doesn't match. U5 emits per-resource extractor code that calls `UpsertBatch` directly with shape-specific extraction.
- `internal/generator/templates/graphql_sync.go.tmpl` is the GraphQL sync template (separate from `sync.go.tmpl`). GraphQL specs use `{data: {<query-name>: [...]}}` shape — a fifth pattern. U5 either includes GraphQL or explicitly defers it.

### Institutional Learnings

- **Issue #135 closed prematurely** with the parser fix landed but the template consumer half-wired. WU-1 surfacing again as the same regression confirms the anti-pattern: "we fixed the data structure but not the consumer." Mitigation in this plan: each WU includes an explicit verification step that exercises the consumer end-to-end, not just the structural change.
- **PR #335 (an hour ago)** modified `internal/generator/templates/doctor.go.tmpl` to make doctor probe through `flags.newClient()` and `c.Get/c.GetWithHeaders` — meaning doctor reachability now flows through `do()` and inherits U1's per-endpoint header injection. Confirm during U1 implementation that doctor's root-path probe (`c.Get("/", nil)`) correctly receives no per-path headers (its target is the root, not a versioned endpoint) — this is the right behavior; the API-version header should only fire for versioned paths.
- **Silent failures slip past dogfood** (cal-com `bookings get` returned HTTP 200 with empty list). Four of the five bugs in this plan share this shape: structural OK, payload empty/wrong. Without dogfood improvements (deferred above), each future occurrence will still ship undetected.
- **PII scrub already failed twice publicly** despite inline guidance in `secret-protection.md`. The "warn-and-rubber-stamp" failure mode after false-positive fatigue is the dominant risk — U3's design must make the safe choice the default and the unsafe choice the explicit override, not the other way around.

### External References

None required. The work is internal templates, parser, pipeline, and skill changes — no new dependencies, no protocol research.

## Key Technical Decisions

- **U1 design — option (a) thread headers through `resolveRead`, NOT option (b) global path-prefix map in `client.do()`.** The first draft of this plan recommended (b) on the grounds that it covers more call sites. Document review surfaced that (b) cannot work as designed: the parser keys per-endpoint headers by template path (`/v2/bookings/{uid}/cancel`), but at runtime the client sees substituted paths (`/v2/bookings/UID-123/cancel`). Longest-prefix matching against a literal `{uid}` placeholder will not match the substituted path. Option (a) sidesteps the entire path-matching problem because `command_endpoint.go.tmpl` already builds `headerOverrides` at codegen time per endpoint as a baked-in literal map — there is nothing to look up at runtime; the headers are already in scope at the call site. The template change is three lines (signature + caller); `resolveRead`/`resolvePaginatedRead`/`paginatedGet` get a `headers map[string]string` parameter that they forward to `c.GetWithHeaders`. Novel-feature commands using `c.Get` directly are not auto-injected — they're hand-written code where the author controls the call site, and explicit `c.GetWithHeaders` is the right pattern there.
- **U2 fix — both the carry-forward block AND LoadResearch path resolution.** Defense in depth. The LoadResearch path mismatch (`<RunRoot>/pipeline/research.json` vs `<RunRoot>/research.json`) is the proximate cause; fix it by trying the run-root path first and falling back to the pipeline-dir path (or vice versa — order doesn't matter as long as both are checked). Independently, add `m.NovelFeatures = existing.NovelFeatures` to the carry-forward block so the publish-time rewrite never silently strips a populated field. Either fix alone closes the visible bug; both together close the class of bug where research.json convention drifts in the future.
- **U3 inversion of auto-redact policy — warn for everything by default; auto-redact ONLY when matching vendor-prefix anchors.** The first draft auto-redacted emails (permissive pattern, irreversible mutation) and warned on names (constrained pattern, easy to rubber-stamp). Document review (adversarial F5) called this cost-asymmetry inverted. New policy: emails get a warn-with-suggested-redaction (user one-key-confirms in interactive mode); names get the same warn flow with allowlist suppression; only patterns with a vendor-specific prefix anchor (`Bearer cal_live_*`, `Bearer sk_live_*`, `Bearer ghp_*`, `xoxp-*`, etc.) get auto-redacted because the false-positive rate on those is near-zero. The interactive prompt gates on "any non-anchored finding" — so a user with no PII hits sees no prompt, a user with mixed findings sees one prompt with all of them.
- **U3 allowlist — derived from spec content, not hand-curated.** Document review (security F1) called the hand-curated allowlist structurally incomplete. Instead: at scrub time, build the allowlist from the spec's operation summaries, tag names, parameter descriptions, and the printed CLI's command names (`<cli> --help` walked recursively). A capitalized phrase that appears in any of those is suppressed. This catches "Event Types", "Booking Links", "Webhook Triggers" automatically without manual maintenance, and grows naturally as new APIs ship.
- **U3 file scope — content-sniff non-binary text files, not extension-list.** Document review (security F2) noted `.yaml`, HAR variants, and future formats would slip through an extension-list approach. Use `file --mime-type` (Unix `file` command) or a small Go helper to detect text/* files in the staged dir; sweep all of them.
- **U4 target template — `auth_simple.go.tmpl` exclusively.** `auth.go.tmpl` (OAuth) has no set-token handler. The auth-template-selection logic in `internal/generator/generator.go` chooses among three templates based on auth type; only `auth_simple.go.tmpl` is in scope here. If a future OAuth template grows a set-token handler it can apply the same fix.
- **U4 deprecation log — silent clear OR TTY-gated.** The first draft proposed a stderr log line including a masked token tail (`****<tail>`). Document review (security F3) noted the log line could leak tail bytes through proof captures (scripted dogfood often tees stderr). Two safe options: (1) silently clear `auth_header` with no log line — the user can run `doctor` to see the new auth source; (2) emit the log line only when `os.Stderr` is a TTY (`golang.org/x/term.IsTerminal(int(os.Stderr.Fd()))`). Picked option (1) because it's the simpler safe default and the user can always run doctor for visibility. If implementation reveals the silent clear is too surprising, switch to (2).
- **U5 GraphQL coverage — included via `graphql_sync.go.tmpl` modification.** Document review (adversarial F4) noted GraphQL has its own sync template that the first draft missed. Including it here keeps the fix coherent (one PR, one merge) rather than splitting envelope-shape work across two plans. The GraphQL shape is `{data: {<query-name>: [...]}}` which fits the vocabulary as `wrapped_at_data:<query-name>` — same shape primitive, different key. Validation includes regenerating a GraphQL fixture (Linear or similar) alongside Cal.com.
- **U5 shape vocabulary — `array_at_data`, `single_at_data`, `wrapped_at_data:<key>`, `nested:<group>,<list>`, `unknown`.** Five shapes. `wrapped_at_data:<key>` accepts arbitrary keys (covers Cal.com bookings, GraphQL queries, Stripe-with-pagination using `data.<key>` patterns). `nested:<group>,<list>` for two-level group shapes (Cal.com event-types). `unknown` falls back to current heuristic. Vocabulary correctness is validated against four specs during implementation: Cal.com (REST, all four shapes), Stripe (REST with pagination metadata), Linear (GraphQL), and a synthetic fixture. If validation surfaces a shape outside the vocabulary, the implementation can extend the enum — the fallback to `unknown` ensures existing CLIs never regress.
- **No new flags or CLI surface.** All five units are internal mechanism fixes; the printed CLI's user-facing API is unchanged. Bounds regression risk.
- **Tests live next to code under test.** Each unit's test file is in the same package as the change, following existing repo convention.

## Open Questions

### Resolved During Planning

- *Should U1 use option (a) or (b)?* — Option (a). See Key Technical Decisions. Document review changed this answer from the first draft.
- *What is the actual root cause of U2's visible bug?* — The LoadResearch path mismatch (skill writes to `<RunRoot>/research.json`, code reads `<RunRoot>/pipeline/research.json`) compounded with the carry-forward block dropping `existing.NovelFeatures`. Both must be fixed.
- *Where does U4's set-token handler live?* — Only `auth_simple.go.tmpl`. `auth.go.tmpl` is OAuth-only.
- *Does the existing `secret-protection.md` have a layered scrub model?* — No. U3 either restructures with explicit headings or adds a sibling section to "Workspace & organization PII redaction" without claiming a layer-numbering it doesn't have.
- *Does WU-3's auto-redact email policy have an irreversibility risk?* — Yes. Inverted to warn-by-default; only vendor-prefix anchors auto-redact.
- *Does U5 need to cover GraphQL?* — Yes, via `graphql_sync.go.tmpl`. Same vocabulary, parallel template change.

### Deferred to Implementation

- *U1: should the `headers` parameter be `nil`-default for callers that don't have headers?* — Probably yes (matches the existing `c.GetWithHeaders(path, params, nil)` pattern in the non-store branch). Confirm during implementation.
- *U2: which lookup order — RunRoot first then pipeline/, or pipeline/ first then RunRoot?* — Probably RunRoot first (skill convention is the dominant case). Confirm by checking how many existing plan-driven CLIs in the catalog rely on the pipeline/ convention.
- *U3: exact regex for "vendor-prefix anchored" auto-redact tier* — Start with: `Bearer (sk_live_|sk_test_|cal_live_|cal_test_|ghp_|gho_|xoxp-|xoxb-)[A-Za-z0-9_\-]{8,}`. Tune during implementation by sweeping the catalog for vendor patterns.
- *U3: how to invoke `file --mime-type` portably (macOS `file` vs GNU `file`)?* — Probably a small Go helper that reads the first 512 bytes and checks for binary content (`bytes.IndexByte(data, 0) != -1`). Decide during implementation.
- *U5: how granular should the response-shape representation be on `SyncableResource`?* — A struct with shape enum + optional path components. Exact field set confirmed when reviewing the four validation specs during implementation.
- *U5: what does the parser do when a response schema is genuinely undocumented (a `application/json` response with no `schema`)?* — Records `unknown`; sync template falls through to current heuristic. Same as today's behavior for that subset.

---

## Implementation Units

- U1. **Add `headers` parameter to `resolveRead`/`resolvePaginatedRead`/`paginatedGet`; thread per-endpoint `headerOverrides` through the store-backed read path**

**Goal:** Restore per-endpoint header injection on store-backed reads. APIs like Cal.com, Stripe, GitHub send the correct API-version header on every request without hand-patching.

**Requirements:** R1

**Dependencies:** None.

**Files:**
- Modify: `internal/generator/templates/data_source.go.tmpl` — add `headers map[string]string` parameter to `resolveRead` and `resolvePaginatedRead`; forward to `c.GetWithHeaders`/`paginatedGet`.
- Modify: `internal/generator/templates/sync.go.tmpl` — `paginatedGet` helper signature gets `headers` parameter (wire it from callers; end users use `c.GetWithHeaders`).
- Modify: `internal/generator/templates/command_endpoint.go.tmpl` lines 196-201 — store-backed branches pass `headerOverrides` (which is already declared above on line 149 when `.Endpoint.HeaderOverrides` is truthy) or `nil` when there are no overrides.
- Test: `internal/generator/generator_test.go` — golden test asserting that an endpoint with per-endpoint `cal-api-version` headers generates a CLI whose store-backed handler passes the headers map all the way through to `c.GetWithHeaders`.

**Approach:**
- The `headerOverrides` literal map is already in scope at the template emit site for endpoints with per-endpoint headers. The template currently only uses it on the non-store branch. The store branch ignores the variable, leaving it unused (Go compile error if `.Endpoint.HeaderOverrides` is set but the branch ignores the var — verify the template guards correctly).
- After the change, the store branch becomes:
  - With overrides: `data, prov, err := resolveRead(c, flags, "<resource>", isList, path, params, headerOverrides)`
  - Without overrides: `data, prov, err := resolveRead(c, flags, "<resource>", isList, path, params, nil)`
- `resolveRead` forwards: in the `case "live":` branch, change `c.Get(path, params)` to `c.GetWithHeaders(path, params, headers)`. In the `default: // "auto"` branch, same swap. The local cache fallback path (`resolveLocal`) doesn't make HTTP requests so it doesn't need headers.
- `paginatedGet` similarly forwards. The cursor-loop iterations all use the same headers (the per-endpoint version doesn't change between pages).
- Doctor's root-path reachability probe (`c.Get("/", nil)` per PR #335) doesn't go through `resolveRead`; it stays as-is. The probe SHOULD NOT receive a per-endpoint version header (the root path isn't a versioned endpoint).
- Novel-feature commands using `c.Get` directly are unchanged — hand-written code uses `c.GetWithHeaders` explicitly when needed.

**Patterns to follow:**
- Existing `headerOverrides` declaration block in `command_endpoint.go.tmpl` lines 148-153.
- Existing `c.GetWithHeaders(path, params, headerOverrides)` call in the non-store branch (line 197).

**Test scenarios:**
- Happy path: An OpenAPI endpoint with `parameters: [{name: "Stripe-Version", in: header, required: true, schema: {default: "2024-01-01"}}]` and a syncable resource generates a CLI whose store-backed `<resource> get` call places `Stripe-Version: 2024-01-01` on the wire. Verify via a test fake intercepting `do()`.
- Happy path: A non-store-backed endpoint with the same per-endpoint header continues to work (existing path unchanged).
- Edge case: An endpoint with NO per-endpoint headers and a syncable resource generates `resolveRead(..., nil)` — confirm via golden output that the `nil` is emitted rather than an empty map literal (cosmetic but matters for diff churn).
- Edge case: An endpoint with per-endpoint headers AND pagination — `resolvePaginatedRead` and the underlying `paginatedGet` BOTH forward the headers; verify on a paginated cal-com endpoint (e.g., bookings get with `take`+pagination).
- Integration: Regenerate cal-com fixture WITHOUT the hand-written `internal/client/calcom_versions.go` workaround. Confirm `bookings get --dry-run --json` shows `cal-api-version: 2024-08-13` in the request preview AND `bookings get --data-source live` returns real bookings against the live API.
- Negative: An API with no per-endpoint headers (e.g., a no-version REST API) regenerates byte-identical to current output (golden harness).
- Doctor probe: `cal-com-pp-cli doctor` continues to send the existing `RequiredHeaders` (global) but does NOT send `cal-api-version` on the root-path probe, because the root path isn't in `Endpoint.HeaderOverrides`.

**Verification:**
- `go test ./internal/generator/...` and `./internal/openapi/...` pass.
- `scripts/golden.sh verify` passes (no fixture changes for non-versioned APIs).
- Cal.com regenerate (separate validation) confirms `bookings get` returns real bookings without the workaround.

---

- U2. **Promote populates `novel_features` reliably: fix LoadResearch path resolution AND preserve `NovelFeatures` in the carry-forward block**

**Goal:** `lock promote` writes `.printing-press.json` with `novel_features` whenever `research.json` has `novel_features_built` populated, regardless of whether research.json lives in `<RunRoot>/` (skill convention) or `<RunRoot>/pipeline/` (printing-press print convention).

**Requirements:** R2

**Dependencies:** None.

**Files:**
- Modify: `internal/pipeline/publish.go` — in the carry-forward block (lines 207-219), add `m.NovelFeatures = existing.NovelFeatures`. In the LoadResearch block (lines 265-274), try `state.PipelineDir()` first, fall back to `state.RunRoot()` (or vice versa — see Open Questions).
- Modify: `internal/pipeline/state.go` — add a `RunRoot()` helper if it doesn't already exist (returns the parent of `PipelineDir()`).
- Test: `internal/pipeline/publish_test.go` — happy paths for both research.json locations, plus the carry-forward-only path.

**Approach:**
- Carry-forward block change: one line. After the existing `m.AuthEnvVars = existing.AuthEnvVars`, add `m.NovelFeatures = existing.NovelFeatures`. This handles the case where generate populated novel_features (via `WriteManifestForGenerate`'s `p.NovelFeatures` path) and publish should preserve them.
- LoadResearch resolution: refactor to a small helper `loadResearchForState(state *PipelineState) (*ResearchResult, error)` that tries `state.RunRoot()/research.json` first (skill-flow convention), then `state.PipelineDir()/research.json` (print-flow convention). If both fail, return the second error (more useful diagnostic for print-mode users).
- Even with the carry-forward fix, the LoadResearch fix is necessary because plan-driven CLIs that DIDN'T go through `WriteManifestForGenerate` won't have novel_features in `existing.NovelFeatures` at all — they need the research.json read to work.
- When `loadResearchForState` succeeds and `NovelFeaturesBuilt` is non-empty, OVERRIDE `m.NovelFeatures` with the loaded values (research.json is the source of truth post-dogfood). When it fails or `NovelFeaturesBuilt` is nil/empty, KEEP the carry-forward value.

**Patterns to follow:**
- Existing carry-forward block (lines 207-219) for MCPBinary/MCPToolCount — same pattern: read existing data, mutate manifest.
- `internal/pipeline/dogfood.go` `checkNovelFeatures` for the research.json read pattern.

**Test scenarios:**
- Happy path A (skill flow): research.json at `<RunRoot>/research.json` with 12 entries in `novel_features_built`. Promote. Manifest has 12 novel_features.
- Happy path B (print flow): research.json at `<RunRoot>/pipeline/research.json`. Promote. Manifest has 12 novel_features.
- Happy path C (carry-forward): research.json missing entirely. The `existing.NovelFeatures` (from WriteManifestForGenerate's earlier write) has 12 entries. Promote. Manifest preserves the 12.
- Edge case: research.json present but `novel_features_built` empty array. Carry-forward has values. Promote keeps the carry-forward values (don't overwrite with empty research data).
- Edge case: research.json present AND existing manifest both have entries but different. Research wins (post-dogfood is authoritative).
- Edge case: research.json malformed. Function logs warning, falls through to carry-forward. No panic.
- Integration: After promote, run `publish validate`. Transcendence check returns PASS for happy paths A/B/C; FAIL with the existing "no novel features recorded" message when ALL sources are empty (true no-transcendence case).

**Verification:**
- `go test ./internal/pipeline/...` passes including new tests.
- A regenerate + promote of cal-com (manual smoke) writes a manifest with 12 novel_features visible; `publish validate --dir ~/printing-press/library/cal-com` returns `transcendence: PASS` without manual editing.

---

- U3. **PII scrubber added to publish flow: warn-by-default with vendor-anchored auto-redact, spec-derived allowlist, content-sniffed file scope**

**Goal:** Block PII captured during dogfood from leaking into the public library repo. Scrub fails safe (warn over auto-redact) so a regression in the regex set or allowlist cannot corrupt manuscripts irreversibly.

**Requirements:** R3

**Dependencies:** None.

**Files:**
- Modify: `skills/printing-press/references/secret-protection.md` — add a new section "PII pattern scanning" as a sibling to the existing "Workspace & organization PII redaction" section (no Layer-numbering rename, since the file doesn't currently use that scheme). Document the regex patterns, the warn-vs-auto-redact policy split, the spec-derived allowlist mechanism, and the content-sniff file scope.
- Modify: `skills/printing-press-publish/SKILL.md` — in the unnumbered "Secret & PII Protection" section near line 784, add an invocation of the new PII scan after the existing scrub steps.
- Test: this is a skill-prose change; testability is via worked examples documented inline in `secret-protection.md`. Document positive and negative examples for each regex pattern.

**Approach:**
- Two-tier scrub:
  - **Auto-redact tier** (low false-positive, high precision): vendor-prefix-anchored bearer tokens. Patterns: `Bearer (sk_live_|sk_test_|cal_live_|cal_test_|ghp_|gho_|xoxp-|xoxb-|pat-|github_pat_)[A-Za-z0-9_\-]{8,}`. Replace with `Bearer <REDACTED:vendor-token>`. Single Python regex sub call, no user prompt — these patterns have near-zero false-positive rate.
  - **Warn tier** (any false-positive risk): generic emails, capitalized name patterns, generic bearer tails (`Bearer [A-Za-z0-9._\-+/=]{20,}` not matching the vendor-anchored set). Each finding reported with file + line + matched text. User decides per finding via the platform's blocking question tool. Recovery is built-in: original staged copy is preserved at `/tmp/<staging>.pre-pii-scrub/` so the user can re-stage if they want a different scrub policy.
- Allowlist (suppresses warn-tier findings): build at scrub time from:
  - The spec's operation summaries, descriptions, tag names, parameter descriptions
  - The printed CLI's command tree (`<cli> --help` walked recursively, capitalized two-word phrases extracted)
  - A small static list of universal terms ("Open Source", "Pull Request", "Bearer Token", "API Key", "Access Token")
  - Match a finding against the allowlist by exact string AND by case-insensitive contains; if either hits, suppress.
- File scope: walk the staged dir, for each file detect text vs binary by reading the first 512 bytes and checking for null bytes (`bytes.IndexByte(data[:n], 0) != -1` → binary, skip). Sweep all text files regardless of extension.
- Sweep ordering: existing exact-value scan → existing HAR strip → new PII scan (auto-redact tier first, then warn tier). Warn tier runs LAST so the user sees only the surviving findings after auto-redaction.

**Patterns to follow:**
- Existing exact-value scan in `secret-protection.md` lines 18-47 — same structure: bash heredoc + Python regex sub.
- Existing "Workspace & organization PII redaction" section as a sibling reference for prose voice.
- Document review's adversarial F5 cost-asymmetry insight informs the "warn-by-default for everything except vendor-anchored" policy.

**Test scenarios:**
- Happy path (auto-redact tier): Stage a CLI with `Bearer sk_live_abc123def456ghi789` in `proofs/foo.md`. Run publish. Pattern auto-redacts to `Bearer <REDACTED:vendor-token>`. No user prompt for this finding.
- Happy path (warn tier): Stage a CLI with `henryopenclaw@gmail.com` in a proof file. Scrub flags it via warn tier with file + line. User picks "redact" → replaces with `<REDACTED:email>`. User picks "keep" → original preserved with audit-trail note.
- Allowlist: Stage a CLI whose spec has "Event Types" in tag names. Scrub finds "Event Types" in a manifest description, matches against derived allowlist, suppresses the warning.
- Allowlist: Stage a CLI with "Open Source" in README. Universal-list match suppresses.
- Edge case: A proof with both `Bearer sk_live_abc...` AND `Henry Claw` AND `henryopenclaw@gmail.com`. Vendor token auto-redacted silently. Email and name reported via warn tier — one prompt with both findings, user decides each.
- Edge case: Scrub finds nothing (clean staged dir). No prompt, no warning, publish proceeds.
- Edge case: A `.yaml` file in `discovery/` contains an email. Content-sniff detects text, sweep includes it. Email flagged.
- Recovery: User runs publish, sees warn-tier findings, picks "abort to review". Original staged copy is at `/tmp/<staging>.pre-pii-scrub/`. User inspects, modifies scrub policy if needed, re-runs publish.
- Negative: After publish completes with all findings addressed, the staged dir contains `<REDACTED:...>` substitutions but no live PII or live secrets.

**Verification:**
- The scrub logic in `secret-protection.md` is testable via the documented worked examples — a sample proof file before and after scrubbing.
- `skills/printing-press-publish/SKILL.md` references the new section in the right sequence.
- Re-publish cal-com (manual smoke) — vendor tokens auto-redacted in proofs, emails/names warn-flagged, no surprise leaks.

---

- U4. **Fix `auth set-token` to clear legacy `auth_header` and write to `access_token` (target `auth_simple.go.tmpl` only; silent clear)**

**Goal:** `auth set-token <token>` actually changes the active credential. Subsequent API calls use the new token regardless of whether `config.toml` had a legacy `auth_header` field.

**Requirements:** R4

**Dependencies:** None.

**Files:**
- Modify: `internal/generator/templates/auth_simple.go.tmpl` lines 69-90 (`newAuthSetTokenCmd` handler) — set `cfg.AuthHeaderVal = ""` before calling `cfg.SaveTokens(...)`. Silent clear (no stderr log).
- Modify: `internal/generator/templates/config.go.tmpl` — no behavior change required. The existing `Config.AuthHeader()` resolver order is correct once `AuthHeaderVal` is cleared. Confirm by inspection.
- Test: `internal/generator/generator_test.go` — golden test asserting the generated `auth.go`'s set-token handler clears `AuthHeaderVal` before save. Plus a golden-fixture verification pass to confirm the change doesn't break existing fixtures.

**Approach:**
- In the `newAuthSetTokenCmd` handler in `auth_simple.go.tmpl`, modify the existing call sequence:
  ```
  cfg.AuthHeaderVal = ""  // clear legacy auth_header so AuthHeader() falls through to access_token
  cfg.SaveTokens("", "", args[0], "", cfg.TokenExpiry)
  ```
- No deprecation log line (silent clear). The user can run `doctor` to see the new auth source. Document review (security F3) noted that any masked-token log line risks leaking tail bytes through scripted dogfood that captures stderr; silent clear is the safest default.
- `Config.AuthHeader()` resolver order stays unchanged: `if AuthHeaderVal != "" return AuthHeaderVal; else return "Bearer " + AccessToken`. Once AuthHeaderVal is cleared, the new token wins via the fallback path.
- Auth template selection in `internal/generator/generator.go` (`renderAuthFiles`) chooses `auth_simple.go.tmpl` for the default auth case; `auth.go.tmpl` is OAuth-only (no set-token) and `auth_browser.go.tmpl` covers cookie/composed/persisted-query (no set-token in scope). Only `auth_simple.go.tmpl` needs modification.

**Patterns to follow:**
- Existing `cfg.SaveTokens(...)` call in `auth_simple.go.tmpl`.
- `Config.AuthHeader()` resolver order in `config.go.tmpl`.

**Test scenarios:**
- Happy path: `config.toml` has `auth_header = 'Bearer OLD'` and `access_token = ''`. Run `set-token NEW`. Assert post-call config: `auth_header = ''`, `access_token = 'NEW'`. Subsequent `Config.AuthHeader()` returns `Bearer NEW`. No stderr output (silent clear).
- Happy path: `config.toml` has only `access_token = 'OLD'` (clean state). Run `set-token NEW`. Assert: `access_token = 'NEW'`, `auth_header` remains empty. Same code path, no special handling.
- Edge case: `config.toml` has both `auth_header = 'Bearer OLD1'` AND `access_token = 'OLD2'`. Run `set-token NEW`. Assert: `auth_header = ''`, `access_token = 'NEW'`.
- Edge case: User had set `auth_header` manually with a custom prefix like `'Token foo'`. After `set-token`, their custom prefix is cleared. Documented as expected — set-token is the canonical write path. User can re-set `auth_header` manually if they need a non-Bearer prefix.
- Golden-fixture verification: Run `scripts/golden.sh verify`. If the fixture's expected `auth.go` golden differs (because the new template emits an additional line), update the golden in the same commit and verify the diff is exactly the `cfg.AuthHeaderVal = ""` line — no other unexpected changes.
- Integration: After the generator change, regenerate cal-com fixture, run `cal-com-pp-cli auth set-token NEW; doctor`. Doctor reports auth from `access_token`. API call uses `Bearer NEW`.
- Negative: An empty `set-token ""` returns the existing usage error and does NOT clear `auth_header`. Existing arg validation handles this; verify it still does.

**Verification:**
- `go test ./internal/generator/...` passes including new test.
- `scripts/golden.sh verify` passes (with the documented fixture update for the new clear-line if needed).
- Manual smoke: regenerate cal-com, run set-token + doctor + a real API call. Confirm the new token is in the Authorization header.

---

- U5. **Sync emits per-resource envelope extractors driven by spec response shape (REST + GraphQL)**

**Goal:** `sync --full` ingests responses with `data:[]`, `data:<single-object>`, `data:<key>:[...]`, and nested grouped shapes correctly across both REST and GraphQL CLIs, not only `data.<resource>: [...]`. Failed extractions surface as actionable errors, not silent zero-row stores.

**Requirements:** R5

**Dependencies:** None.

**Files:**
- Modify: `internal/openapi/parser.go` — record per-resource response envelope shape during parsing. Extract from the response schema's `application/json` content.
- Modify: `internal/graphql/parser.go` — record envelope shape for GraphQL responses (always `wrapped_at_data:<query-name>` for list queries).
- Modify: `internal/spec/spec.go` — add a `ResponseEnvelope` field to the parsed resource representation (or extend `Endpoint` if the field belongs there).
- Modify: `internal/profiler/profiler.go` — `SyncableResource` struct gets an `Envelope ResponseEnvelope` field; populated from the parser output.
- Modify: `internal/generator/templates/sync.go.tmpl` — emit per-resource extractor logic driven by the recorded shape. Fall back to current heuristic (`extractPageItems`) when shape is `unknown`.
- Modify: `internal/generator/templates/graphql_sync.go.tmpl` — same per-resource extractor pattern for GraphQL.
- Test: `internal/openapi/parser_test.go` and `internal/graphql/parser_test.go` — assert recorded shapes for fixture specs covering all five shape variants.
- Test: `internal/generator/generator_test.go` — golden test that the sync templates emit the right extractor for each shape variant.

**Approach:**
- Define the shape vocabulary as a small Go type:
  ```
  type ResponseEnvelope struct {
      Shape ShapeKind  // array_at_data, single_at_data, wrapped_at_data, nested, unknown
      Keys  []string   // for wrapped_at_data: [single key]; for nested: [group, list]
  }
  ```
- During OpenAPI parsing, for each list endpoint that's a sync candidate, walk the response schema:
  - `data` is `type: array` → `ResponseEnvelope{Shape: ArrayAtData}`
  - `data` is `type: object` with no obvious list field → `ResponseEnvelope{Shape: SingleAtData}`
  - `data` is `type: object` with one `type: array` property `<key>` → `ResponseEnvelope{Shape: WrappedAtData, Keys: ["<key>"]}`
  - `data` is `type: object` containing a `type: array` of objects each with one `type: array` field → `ResponseEnvelope{Shape: Nested, Keys: ["<group>", "<list>"]}`
  - Otherwise → `ResponseEnvelope{Shape: Unknown}`
- During GraphQL parsing, for each query selected as syncable, record `ResponseEnvelope{Shape: WrappedAtData, Keys: ["<query-name>"]}`.
- The profiler propagates the envelope from each parsed `Endpoint.Response` onto the corresponding `SyncableResource`.
- The sync templates branch on `.Envelope.Shape` to emit the right extractor:
  - `ArrayAtData`: `var items []json.RawMessage; json.Unmarshal(env.Data, &items)`
  - `SingleAtData`: `var item json.RawMessage; json.Unmarshal(env.Data, &item); items := []json.RawMessage{item}`
  - `WrappedAtData`: `var w struct{ Items []json.RawMessage `json:"<key>"` }; json.Unmarshal(env.Data, &w); items := w.Items`
  - `Nested`: walk `[<group>][i].<list>` and concatenate
  - `Unknown`: emit the existing `extractPageItems` heuristic call (zero behavior change for current CLIs)
- All five shape extractors fall through to `db.UpsertBatch(resource, items)` — only the upstream extraction differs.
- Input validation on `Keys`: enforce that path components are valid JSON keys (no `.`, no `[`, no shell metacharacters) before baking them into emitted Go code. Document review (security top-3 #3) flagged that adversarial spec keys could otherwise compromise the emitted extractor.

**Patterns to follow:**
- The current heuristic walker (`extractPageItems` in `internal/generator/templates/sync.go.tmpl:446`) — its envelope-walking logic becomes the `Unknown` shape's fallback path.
- `client.go.tmpl`'s template directives that consume parser output (`range .RequiredHeaders`) — same pattern: parser produces structured data, template walks it.
- Existing GraphQL handling in `graphql_sync.go.tmpl` for the parallel template change.

**Test scenarios:**
- Happy path A (REST `wrapped_at_data:<resource>`): Fixture spec declares `data: {bookings: [...]}` for `/bookings`. Parser records `WrappedAtData, Keys=["bookings"]`. Generated sync extracts via that key. Run sync against a stub returning `{"data": {"bookings": [{"id":1},{"id":2}]}}`. Assert 2 rows.
- Happy path B (REST `single_at_data`): Fixture spec declares `data: <object>` for `/me`. Parser records `SingleAtData`. Generated sync extracts the single object as a one-item slice. Assert 1 row.
- Happy path C (REST `array_at_data`): Fixture spec declares `data: [...]` for `/teams`. Parser records `ArrayAtData`. Generated sync extracts the array directly. Assert N rows.
- Happy path D (REST `nested`): Fixture spec declares the nested `eventTypeGroups[].eventTypes` shape for `/event-types`. Parser records `Nested, Keys=["eventTypeGroups", "eventTypes"]`. Generated sync walks groups, concatenates items. Assert sum.
- Happy path E (GraphQL): GraphQL fixture with `query Bookings { bookings { id title } }`. Parser records `WrappedAtData, Keys=["bookings"]`. Generated GraphQL sync extracts via `data.bookings`. Assert N rows.
- Edge case (unknown): Spec is genuinely ambiguous (response schema absent or doesn't match any pattern). Parser records `Unknown`. Generator emits `extractPageItems` fallback. Existing CLIs regenerate byte-identically.
- Edge case (validation): A spec with a malicious key like `.;rm -rf` or `__proto__` — parser rejects (returns `Unknown` plus a warning), template emits the safe fallback. No injection into emitted Go code.
- Integration: Regenerate cal-com fixture. Run `sync --full`. Assert all 11 resources sync without "missing id for X" errors. Assert sync_summary's `errored` count is 0 for resources whose envelope shape is now correctly handled.
- Integration GraphQL: Regenerate Linear (or another GraphQL fixture). Run sync. Assert resources ingest correctly.
- Negative: An API with only `data.<resource>: [...]` shape (current default) regenerates byte-identical to current output (golden harness).

**Verification:**
- `go test ./internal/openapi/...`, `./internal/graphql/...`, `./internal/profiler/...`, `./internal/generator/...` all pass.
- `scripts/golden.sh verify` passes for non-Cal.com APIs (no fixture changes for currently-working shapes).
- Cal.com regenerate (separate validation) syncs all 11 resources without errors.
- Linear/GraphQL regenerate (separate validation) syncs without regression.

## System-Wide Impact

- **Interaction graph:** U1 changes `resolveRead`/`resolvePaginatedRead`/`paginatedGet` signatures — every store-backed handler across every printed CLI is regenerated. Test coverage must verify both store-backed and direct-API paths still work for unaffected APIs (golden harness handles non-Cal.com cases; Cal.com regen verifies the affected case). U5 changes the sync templates — every store-backed CLI regenerates; `Unknown` shape preserves current behavior so no regression.
- **Error propagation:** U2 must not panic when `research.json` is missing or malformed. The path-resolution fix tries both locations; on miss, the carry-forward path takes over silently. U4 keeps existing input validation (empty token rejected) intact. U3's auto-redact runs without prompting; warn-tier findings present a single batched prompt — connection failure or interrupt aborts the publish, leaves the staged dir for review.
- **State lifecycle risks:** U2 mutates the manifest after carry-forward; if `novel_features_built` is malformed (entries missing required fields), skip the bad entries rather than aborting. U4 clears `auth_header` — users with intentional custom prefixes will lose that override (acceptable per Key Technical Decisions). U3 preserves a pre-scrub copy at `/tmp/` for recovery; if the user aborts mid-prompt, that copy lets them re-stage.
- **API surface parity:** U1's headers parameter applies to all HTTP verbs in the store-backed branch (`resolveRead` is GET-only by design; mutations always go through `c.GetWithHeaders`/`c.PostWithHeaders` directly with their per-call headers). Doctor's root-path probe correctly receives no per-endpoint headers because the root path isn't in `Endpoint.HeaderOverrides`.
- **Integration coverage:** Per-unit golden tests cover the structural changes. End-to-end regenerate-and-test of cal-com (manually before merge) proves the integrated fixes work against the live API.
- **Unchanged invariants:** The `CLIManifest` struct shape, the `Config.AuthHeader()` resolver order, the `do()` dispatch pattern (auth → required headers → header overrides), the existing `secret-protection.md` "Workspace & organization PII redaction" prose section, and the existing `extractPageItems` heuristic all stay in place. New behavior is additive in every unit. The `c.Get` and `c.GetWithHeaders` public surfaces are unchanged.
- **Concurrent-PR risk surface:** This plan modifies `client.go.tmpl`/`data_source.go.tmpl`/`command_endpoint.go.tmpl`/`sync.go.tmpl`/`auth_simple.go.tmpl`/`graphql_sync.go.tmpl`/`publish.go`/`secret-protection.md`/`publish/SKILL.md`. The still-in-flight allrecipes-retro plan (`docs/plans/2026-04-26-002-feat-printing-press-p1-machine-fixes-plan.md`) modifies `doctor.go.tmpl`/`html_extract.go.tmpl`/`helpers.go.tmpl`/`scorecard.go`. No file overlap — but both plans touch the same generator surface. Coordinate merge order to avoid unnecessary regen churn.

## Risks & Dependencies

| Risk | Mitigation |
|------|------------|
| U1's `headers` parameter cascade could miss a less-common code path (e.g., a future cache-aware helper) and silently drop headers for that path. | Make `headers` non-optional in the function signatures (caller must pass `nil` or a map), so the Go compiler flags any new caller that forgot the parameter. |
| U2's carry-forward path could over-populate manifests for plan-driven CLIs where research.json was intentionally absent. | The carry-forward path only fires when `existing.NovelFeatures` is non-empty; the LoadResearch path only fires when `NovelFeaturesBuilt` is non-empty. Plan-driven CLIs without either will keep `m.NovelFeatures` empty (current behavior preserved). |
| U3's spec-derived allowlist could include sensitive terms (e.g., a spec description mentions a real customer name). | Allowlist is INPUTS to suppression, not outputs to publication. A spec that names "Acme Corp" causes "Acme Corp" findings to be suppressed in proofs — but the spec itself is a separate scrub target. The exact-value scan on the spec is unchanged; spec-derived allowlist only relaxes warn-tier suppression, not auto-redact. |
| U3's content-sniff could miss text files with a leading null byte (e.g., UTF-16 BOM that starts with `\x00`). | Detect BOM-prefixed UTF encodings as text. Document the limitation in `secret-protection.md` and recommend manual review for unusual encodings. |
| U4's silent clear could surprise users who rely on the deprecation log line for visibility. | Document in the publish-time release notes; users can check `cfg.AuthHeaderVal` via `doctor --json` to see the current state. |
| U5's per-resource extraction could miss edge cases the current heuristic catches. | Unknown shapes fall through to the heuristic. The change is strictly additive: known shapes get a precise extractor; unknown shapes preserve current behavior. |
| U5's input validation on Keys could be incomplete and let injection through. | Use a strict allowlist regex on Keys (`^[a-zA-Z_][a-zA-Z0-9_]*$`); reject anything else with a parser warning and `Unknown` shape fallback. |
| Regenerating the entire fixture catalog after this PR could surface unexpected churn. | Each unit has a golden-harness verification step; the consolidated `scripts/golden.sh verify` should pass before merge. Document any intentional fixture diffs in the PR. |
| Concurrent merges with the allrecipes-retro plan (#333/#335) could create rebase pain even though no files overlap. | Coordinate merge order; whichever lands second runs the golden harness against the merged state before final approval. |

## Documentation / Operational Notes

- Update CHANGELOG (or release notes) with a `fix(cli):` entry per unit. Patch-level version bump (no breaking changes to printed CLI surface).
- U3's PII-scrub section in `secret-protection.md` should be cross-linked from `skills/printing-press-publish/SKILL.md` so users see it during publish.
- After merge, regenerate cal-com end-to-end (without the workaround `internal/client/calcom_versions.go`) to validate the integrated fixes. Document the regen in a follow-up note if anything else surfaces.
- File a follow-up issue for the deferred meta-fixes (silent-failure detection in dogfood; generic manifest-merge helper). Link from this plan's PR description.

## Sources & References

- **Origin issue:** [#334](https://github.com/mvanhorn/cli-printing-press/issues/334)
- **Origin retro doc:** [https://files.catbox.moe/itg03h.md](https://files.catbox.moe/itg03h.md), local copy at `/tmp/printing-press/retro/20260427-005908-cal-com-retro.md`
- **Related closed issue:** [#135 — retro(cli): Cal.com Run 2](https://github.com/mvanhorn/cli-printing-press/issues/135) (per-endpoint headers; this plan addresses the regression)
- **Related landed PR:** [#332 — fix(cli): score auth prefixes from config](https://github.com/mvanhorn/cli-printing-press/pull/332) (resolves WU-10 ahead of this plan)
- **Related landed PR:** [#335 — fix(cli): printing-press P1 machine fixes (issue #333)](https://github.com/mvanhorn/cli-printing-press/pull/335) (allrecipes retro fixes; touches doctor.go.tmpl and others, no file overlap with this plan)
- **Concurrent plan:** `docs/plans/2026-04-26-002-feat-printing-press-p1-machine-fixes-plan.md` (allrecipes retro, also modifying generator templates)
- **Related code:** `internal/generator/templates/data_source.go.tmpl`, `internal/generator/templates/command_endpoint.go.tmpl`, `internal/generator/templates/client.go.tmpl`, `internal/generator/templates/sync.go.tmpl`, `internal/generator/templates/graphql_sync.go.tmpl`, `internal/generator/templates/auth_simple.go.tmpl`, `internal/generator/templates/config.go.tmpl`, `internal/openapi/parser.go`, `internal/graphql/parser.go`, `internal/spec/spec.go`, `internal/profiler/profiler.go`, `internal/pipeline/publish.go`, `internal/pipeline/research.go`, `internal/pipeline/state.go`, `internal/pipeline/climanifest.go`, `internal/pipeline/lock.go`, `skills/printing-press/references/secret-protection.md`, `skills/printing-press-publish/SKILL.md`
