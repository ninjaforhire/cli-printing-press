---
title: "feat: Add parameter discovery to crowd-sniff"
type: feat
status: completed
date: 2026-03-30
origin: docs/brainstorms/2026-03-29-crowd-sniff-requirements.md
deepened: 2026-03-30
---

# feat: Add parameter discovery to crowd-sniff

## Overview

Extend crowd-sniff's npm source to extract query parameters from SDK source code alongside endpoints. Currently crowd-sniff discovers method + path correctly but emits `params: []` for every endpoint. The parameter data is present in the code crowd-sniff already downloads and greps — it just isn't extracted.

## Problem Frame

The Steam API crowd-sniff found 23 correct endpoints but zero parameters. All 24 params had to be added manually during the generate phase. The params were visible in three places the npm source already reads: function signatures (`getOwnedGames(steamid, opts = {})`), request builder calls (`this.get('/path', { steamid, include_appinfo: includeAppInfo })`), and README documentation. Eliminating this manual enrichment step makes crowd-sniff a complete discovery source rather than a skeleton that requires human intervention. (see origin: `docs/brainstorms/2026-03-29-crowd-sniff-requirements.md`, gap analysis: `~/printing-press/manuscripts/postman-explore/20260330-105847/proofs/2026-03-30-crowd-sniff-param-discovery-gap.md`)

## Requirements Trace

- P1. Extract query parameter names from object literals passed as the second argument to HTTP method calls (`.get(url, { key1, key2: val })`)
- P2. Infer parameter types using heuristic rules: `.join(',')` → string, numeric defaults → integer, `true`/`false` → boolean, name matches whole-word `id` → string (use `\b` word boundary to avoid matching `userId`, `guild_id`), name is `count`/`limit`/`offset`/`page`/`maxlength` → integer, default → string
- P3. Infer required vs optional from function signatures: positional args without defaults → required, destructured args with defaults → optional. Default to `required: false` when the function signature cannot be correlated
- P4. Handle multi-line object literals (params object spanning 2-10 lines with one key per line)
- P5. Carry extracted params through aggregation (merge across sources) and into `spec.Endpoint.Params`
- P6. Preserve all existing endpoint discovery behavior — param extraction is additive only. `GrepEndpoints` returns unchanged `DiscoveredEndpoint` (Params field is nil). `EnrichWithParams` is an optional enrichment step called after `GrepEndpoints`
- P7. GitHub source is out of scope for param extraction (text match fragments are too short to contain full function bodies). npm source only

## Scope Boundaries

- **npm source only** — GitHub code search returns text fragments, not full function bodies. Param extraction from GitHub is a future enhancement
- **No AST parsing** — lightweight brace-matching scanner, not a JS/TS parser
- **No README/JSDoc parsing** — v2 enhancement. Requires multi-line section extraction beyond what heuristic scanning handles well
- **No body field extraction** — POST/PUT body structure is harder to infer from SDK code (often just `data` or `body`). Future enhancement
- **No parameter description generation** — params get `Name`, `Type`, `Required`, `Default` only. Descriptions are empty

## Context & Research

### Relevant Code and Patterns

| Purpose | File |
|---------|------|
| Types to extend (add Params) | `internal/crowdsniff/types.go` — `DiscoveredEndpoint`, `AggregatedEndpoint` |
| Extraction target (multi-line scanner) | `internal/crowdsniff/patterns.go` — `GrepEndpoints()`, `extractMethodCallEndpoints()` |
| Aggregation to extend (merge params) | `internal/crowdsniff/aggregate.go` — `Aggregate()`, accumulator struct |
| Spec builder to wire up | `internal/crowdsniff/specgen.go` — `BuildSpec()` |
| Reference implementation for `[]spec.Param` | `internal/websniff/specgen.go:182` — `inferURLParams()` |
| Target spec type | `internal/spec/spec.go:62` — `Param` struct with Name, Type, Required, Default, etc. |
| Test patterns | `internal/crowdsniff/patterns_test.go` — inline SDK content strings, `t.Parallel()`, `testify/assert` |
| Test patterns | `internal/crowdsniff/aggregate_test.go` — table-driven, subtest groups |

### Institutional Learnings

- **Cartesian product risk** (from `docs/solutions/best-practices/multi-source-api-discovery-design-2026-03-30.md`): `extractMethodCallEndpoints` previously had a bug where every method was paired with every path on the same line. The fix used `FindAllStringSubmatchIndex` for positional correlation. The same cross-product risk exists when extracting params from a line with multiple function calls — use positional matching.
- **Word-boundary regex** (from `docs/solutions/logic-errors/scorecard-accuracy-broadened-pattern-matching-2026-03-27.md`): `strings.Contains` produces false positives on substrings. When extracting param names like `id` that are substrings of `userId`, `guild_id`, etc., use `\b` word boundaries.
- **Scope broadening breaks downstream** (same source): When broadening from single-line to multi-line extraction, audit all downstream consumers of the extracted data.

## Key Technical Decisions

- **Brace-matching scanner over multi-line regex**: Go's `regexp` does not support lookahead/lookbehind. A lightweight scanner that finds `{` after the URL match and counts brace depth until balanced `}` is more reliable than attempting multi-line regex on concatenated content. ~50 lines of Go, handles nested objects and trailing commas naturally.

- **Content-level scanning (not line-by-line) for params**: The existing `GrepEndpoints` line loop stays for endpoint discovery (works well). A new `EnrichWithParams` function operates on full file content to handle multi-line object literals. It is called after `GrepEndpoints` and enriches the discovered endpoints with params by matching on method+path.

- **Separate `DiscoveredParam` type over reusing `spec.Param`**: A lightweight `DiscoveredParam{Name, Type, Required, Default}` in types.go keeps the discovery layer decoupled from the spec layer. `specgen.go` maps `DiscoveredParam` → `spec.Param`. This avoids importing `spec` into the core discovery types.

- **Union merge for params in aggregation**: When the same endpoint is discovered by multiple sources, take the union of param names. For conflicts on the same param name, prefer metadata from the higher-tier source (official-sdk > community-sdk > code-search). This mirrors how `source_tier` is already resolved.

- **Default `required: false`**: When the function signature cannot be correlated with the HTTP call (common), default to `required: false`. A false-required param breaks CLI usage; a false-optional param is merely inconvenient. Safer default.

- **Enrichment pass architecture**: `GrepEndpoints` returns endpoints as before. A new `EnrichWithParams(content string, endpoints []DiscoveredEndpoint) []DiscoveredEndpoint` function does a second pass over the same content, matching each discovered endpoint's path to HTTP calls, then extracting the params object. This avoids modifying the existing extraction functions and their carefully-tested positional logic. Note: `EnrichWithParams` operates on full content (not line-by-line) — a deliberate architectural asymmetry with `GrepEndpoints` that should be documented in the function's doc comment.

- **EnrichWithParams must match raw URL patterns, not cleaned paths**: `GrepEndpoints` transforms paths during extraction (e.g., `${userId}` → `{id}` via `cleanPath`). The enrichment pass must search content for the raw URL string as it appears in source code, not the cleaned `DiscoveredEndpoint.Path`. Use the same `httpMethodCall` + `urlPathLiteral` regex patterns independently to locate HTTP calls, then correlate by position — do not substring-match `endpoint.Path` against content.

- **String-typed defaults, no type coercion**: `DiscoveredParam.Default` is `string` while `spec.Param.Default` is `any`. Numeric defaults like `10` are stored as `"10"` and passed through as strings. Do not over-engineer type coercion — the data is heuristic. This means YAML output will emit `default: "10"` not `default: 10`, which is acceptable for crowd-sniffed params.

- **Per-param tier tracking in aggregation**: The accumulator's `bestTier` is per-endpoint, but param merge needs per-param tier resolution. Chosen approach: params inherit tier from their parent endpoint's `SourceTier`. The accumulator stores `params map[string]paramEntry` where `paramEntry` is `struct{ param DiscoveredParam; tier string }`. When a param name conflicts, compare `tierRank(incoming.tier)` vs `tierRank(stored.tier)` — higher tier wins. This keeps `DiscoveredParam` itself lightweight (no tier field) while giving the accumulator the data it needs for tier-aware merge.

## Open Questions

### Resolved During Planning

- **Where to extract params — same pass or second pass?** Second pass. The existing line-by-line extraction in `GrepEndpoints` is well-tested and uses positional index matching. Adding multi-line brace scanning into the same loop would mix two scanning strategies. A separate enrichment pass is cleaner.
- **Should we extract from GitHub source too?** No. GitHub code search returns text match fragments (~300 chars), not full function bodies. Params require seeing both the function signature and the HTTP call, which are typically 3-10 lines apart. npm tarballs have full source files.
- **Normalize param names across sources?** No normalization needed for v1. Param names from SDK code are already the canonical API param names (e.g., `steamid`, `include_appinfo`). Path params need normalization because different syntaxes represent the same concept; query params don't have this problem.

### Deferred to Implementation

- Exact regex patterns for object literal key extraction — prototype against Steam, Notion, Discord SDK source
- Whether function-signature correlation works reliably enough to set `required: true` — if too noisy in practice, fall back to all-optional
- Whether the brace scanner should track string literal state (skip `{`/`}` inside quotes) — adds ~10 lines but prevents misparse on params like `{ query: '{complex}' }`. Prototype against real SDKs to gauge frequency
- Param name normalization across sources is deferred. If two sources use different names for the same parameter (e.g., `steamid` vs `steam_id`), both will appear in the union-merged param list. Acceptable for v1 but may produce duplicate params in edge cases

## High-Level Technical Design

> *This illustrates the intended approach and is directional guidance for review, not implementation specification. The implementing agent should treat it as context, not code to reproduce.*

```
SDK source content (from npm tarball)
        |
        v
  GrepEndpoints()          ← existing, unchanged
  returns []DiscoveredEndpoint (method+path only)
        |
        v
  EnrichWithParams()       ← NEW
  For each endpoint found:
    1. Find the HTTP call in content: .get("/matching/path"
    2. Scan forward from the path for , { (params object start)
    3. Brace-match to find the closing }
    4. Extract keys from the captured block
    5. Look backward for enclosing function definition
    6. Correlate function args with param keys for required/optional
    7. Apply type heuristics to values
  returns []DiscoveredEndpoint (now with Params populated)
        |
        v
  Aggregate()              ← extended with param merging
  Union-merge params for same method+path
        |
        v
  BuildSpec()              ← extended to map DiscoveredParam → spec.Param
        |
        v
  spec.Endpoint.Params populated
```

**Brace-matching scanner pseudocode:**

```
given: content string, position after the URL path match
1. scan forward for comma followed by { (skipping whitespace/newlines)
2. if not found within 500 chars, no params → return nil
3. set depth = 1, start collecting
4. for each char after {:
     if { → depth++
     if } → depth--; if depth == 0 → stop
5. the captured block between { and } contains param entries
6. split on commas (respecting nested braces)
7. for each entry, extract the key (before : or the bare identifier for shorthand)
```

## Implementation Units

- [x] **Unit 1: Add DiscoveredParam type and Params field to discovery types**

  **Goal:** Extend the type hierarchy so params can flow from extraction through aggregation to spec generation.

  **Requirements:** P5

  **Dependencies:** None

  **Files:**
  - Modify: `internal/crowdsniff/types.go`

  **Approach:**
  - Add `DiscoveredParam` struct with `Name string`, `Type string`, `Required bool`, `Default string` (all-string default to keep it simple)
  - Add `Params []DiscoveredParam` field to `DiscoveredEndpoint`
  - Add `Params []DiscoveredParam` field to `AggregatedEndpoint`
  - No test file for this unit — pure type definitions. Tests come with the functions that use them.

  **Patterns to follow:**
  - `DiscoveredEndpoint` and `AggregatedEndpoint` struct style in `types.go`

  **Test expectation: none** — struct definitions with no behavior. Exercised extensively by Unit 2 (EnrichWithParams), Unit 3 (Aggregate), and Unit 4 (BuildSpec), which instantiate and assert on these structs.

  **Verification:**
  - `go build ./internal/crowdsniff/...` succeeds
  - Existing tests still pass (`go test ./internal/crowdsniff/...`)

---

- [x] **Unit 2: Multi-line parameter extraction scanner**

  **Goal:** Implement `EnrichWithParams` that does a second pass over SDK source content, finding the params object literal after each HTTP method call and extracting param names, types, and required/optional status.

  **Requirements:** P1, P2, P3, P4, P6

  **Dependencies:** Unit 1

  **Files:**
  - Create: `internal/crowdsniff/params.go`
  - Test: `internal/crowdsniff/params_test.go`

  **Approach:**
  - `EnrichWithParams(content string, endpoints []DiscoveredEndpoint) []DiscoveredEndpoint` — the main entry point
  - Independently scan content for all HTTP method calls using the same `httpMethodCall` + `urlPathLiteral` regex patterns. For each raw match, apply `cleanPath` to the extracted raw URL, then match the resulting `(method, cleanedPath)` pair against the `DiscoveredEndpoint` list by equality. This avoids substring-matching `endpoint.Path` against content (see Key Technical Decisions on raw-vs-cleaned matching). For matching endpoints, extract params from that call position.
  - From the HTTP call position, scan forward for the params object:
    - Skip whitespace/newlines after the path string's closing quote
    - Look for `, {` or `,\n{` (comma then opening brace)
    - If found, run the brace-matching scanner to capture the full object literal
    - If not found within a reasonable distance (~200 chars), assume no params
  - Extract keys from the captured object block:
    - Shorthand properties: bare identifier on its own (e.g., `steamid` without `:`)
    - Key-value pairs: `key: value` or `key: value.join(',')` — extract key
    - Ignore nested objects (depth > 1)
    - Handle trailing commas
  - Type inference heuristics (from the gap artifact's table):
    - Value contains `.join(',')` → `"string"`
    - Value is `true` or `false` → `"boolean"`
    - Value is a numeric literal → `"integer"`
    - Key name matches `count`, `limit`, `offset`, `page`, `maxlength` → `"integer"`
    - Default → `"string"`
  - Function signature correlation (best-effort):
    - From the HTTP call position, scan backward (max 1000 chars) for the nearest function definition pattern: `function\s+\w+\s*\(`, `async\s+function\s+\w+\s*\(`, `\w+\s*\([^)]*\)\s*\{` (class method shorthand), or `\w+\s*=\s*(?:async\s+)?(?:function)?\s*\(` (arrow/assigned). Do NOT use bare `\w+\s*\(` — it matches ordinary function calls like `console.log(`, `JSON.stringify(` and will almost always false-positive
    - Extract the parameter list
    - Positional args without `=` → required
    - Destructured args `{ key = default }` → optional
    - If no function definition is found within 1000 chars, or correlation fails, default all to `required: false`
  - **Critical: use positional matching** — if content has multiple HTTP calls, each endpoint must only extract params from its own call, not cross-product with others

  **Patterns to follow:**
  - Inline SDK content strings in tests (see `patterns_test.go`)
  - `t.Parallel()` on every test and subtest
  - `testify/assert` for assertions
  - Include negative fixtures: lines that should NOT produce params (comments, string literals in non-SDK code)

  **Test scenarios:**
  - Happy path: Single-line params `this.get('/path', { key1: val1, key2: val2 })` → extracts key1 and key2
  - Happy path: Multi-line params spanning 4 lines → extracts all keys
  - Happy path: Shorthand property `{ steamid }` → extracts `steamid`
  - Happy path: Key with `.join(',')` value → type is `"string"`
  - Happy path: Key with boolean value `true`/`false` → type is `"boolean"`
  - Happy path: Key with numeric default → type is `"integer"`
  - Happy path: Key name `count` or `limit` → type is `"integer"`
  - Happy path: Function signature `fn(steamid)` with matching param → `required: true`
  - Happy path: Destructured `fn(id, { opt1 = true } = {})` → `id` required, `opt1` optional
  - Edge case: No params object after URL → returns endpoint with nil params (not empty slice — consistent with GrepEndpoints zero-value)
  - Edge case: Nested object in params `{ filter: { type: "active" } }` → extracts `filter` only (depth 1), ignores nested keys
  - Edge case: Trailing comma after last param → handles correctly
  - Edge case: Content with multiple HTTP calls → each endpoint gets only its own params, no cross-product
  - Edge case: Multi-line call with whitespace/newlines between URL and params `this.get('/path',\n    { key: val })` → still extracts params correctly
  - Error path: Malformed object literal (unbalanced braces) → returns endpoint without params, does not panic
  - Integration: `GrepEndpoints` then `EnrichWithParams` on same content → endpoints have both path and params populated
  - Negative: Existing `GrepEndpoints` behavior unchanged — calling `GrepEndpoints` alone still returns endpoints with nil params

  **Verification:**
  - `go test ./internal/crowdsniff/...` passes
  - Tests cover the Steam SDK patterns from the gap artifact (`.get('/path', { steamid: steamids.join(',') })` and `getOwnedGames(steamid, { includeAppInfo = true } = {})`)

---

- [x] **Unit 3: Param merging in aggregation**

  **Goal:** Extend `Aggregate()` to merge params across sources using union-merge, preferring metadata from higher-tier sources.

  **Requirements:** P5

  **Dependencies:** Units 1, 2

  **Files:**
  - Modify: `internal/crowdsniff/aggregate.go`
  - Modify: `internal/crowdsniff/aggregate_test.go`

  **Approach:**
  - Extend the `accumulator` struct to hold `params map[string]paramEntry` where `paramEntry` is `struct{ param DiscoveredParam; tier string }` — keyed by param name, tier tracks the source that contributed each param
  - During aggregation, for each endpoint's params:
    - If param name not yet seen → add it with the endpoint's `SourceTier`
    - If param name already exists → compare `tierRank(incoming)` vs `tierRank(stored)`: higher tier wins. If same tier, keep the one with more fields populated (non-empty type, has default, etc.)
  - Convert the accumulated params map to a sorted `[]DiscoveredParam` on the `AggregatedEndpoint` (sort by name for deterministic output — this is a core correctness requirement for reproducible YAML, not just an edge case)
  - Per-file dedup in `GrepEndpoints` uses `{method, path, sourceName}` as key — params ride along on the winning entry. If two files in the same SDK define the same endpoint with different param sets, only the first-walked file's params are kept. This is an accepted minor data-quality trade-off
  - `deduplicateEndpoints` key is unchanged — params do not affect dedup

  **Patterns to follow:**
  - Existing `Aggregate()` accumulator pattern (bestTier, sources map)
  - Table-driven tests in `aggregate_test.go`

  **Test scenarios:**
  - Happy path: Two sources discover same endpoint, source A has `{steamid: string}`, source B has `{steamid: string, count: integer}` → aggregated has both params (union)
  - Happy path: Same param from official-sdk (type: string) and code-search (type: integer) → keeps official-sdk's version
  - Happy path: One source has params, other has none → params from the source that has them are preserved
  - Happy path: Params are sorted alphabetically by name in output (deterministic YAML)
  - Edge case: Same param name from same-tier sources with different types → first-seen wins (deterministic)
  - Edge case: Endpoints with no params from any source → AggregatedEndpoint.Params is nil
  - Negative: Existing aggregate tests still pass unchanged (endpoints without params work exactly as before)

  **Verification:**
  - `go test ./internal/crowdsniff/...` passes
  - Existing `TestAggregate` subtests pass without modification

---

- [x] **Unit 4: Wire params through spec generation and npm source**

  **Goal:** Map `DiscoveredParam` → `spec.Param` in `BuildSpec()` and call `EnrichWithParams` in the npm source after `GrepEndpoints`.

  **Requirements:** P5, P7

  **Dependencies:** Units 1, 2, 3

  **Files:**
  - Modify: `internal/crowdsniff/specgen.go`
  - Modify: `internal/crowdsniff/specgen_test.go`
  - Modify: `internal/crowdsniff/npm.go`
  - Modify: `internal/crowdsniff/npm_test.go`
  - Modify: `internal/cli/crowd_sniff.go` (add `param_count` to JSON output)
  - Modify: `internal/cli/crowd_sniff_test.go` (add param flow integration test)

  **Approach:**
  - **specgen.go**: In `BuildSpec()`, after constructing `spec.Endpoint`, map each `AggregatedEndpoint.Params` entry to a `spec.Param{Name, Type, Required, Default}`. Set `Positional: false` (these are query params, not path params). Set `Description: ""` (per scope boundary). Note: `DiscoveredParam.Default` is `string`, `spec.Param.Default` is `any` — assign the string directly, no type coercion.
  - **npm.go**: In `processPackageTarball`, inside the `filepath.Walk` callback, after `GrepEndpoints(string(content), pkgName, tier)`, call `EnrichWithParams(string(content), endpoints)` while `content` is still in scope. This enriches per-file, before appending to `allEndpoints`; cross-source dedup happens later in `Aggregate()`.
  - **crowd_sniff.go**: Add `param_count` to the `--json` output struct. Compute by summing `len(endpoint.Params)` across all spec endpoints. Low-cost additive change that gives downstream tooling (retro skill, provenance) visibility into param discovery quality.
  - **crowd_sniff_test.go**: Add one test case with a mock source returning endpoints with params, assert the written spec YAML contains param entries. The existing mock tests pass without modification but don't verify param flow through the CLI layer.
  - **No changes to github.go** — per P7, GitHub source is out of scope for param extraction.

  **Patterns to follow:**
  - `websniff/specgen.go:inferURLParams()` for how `spec.Param` is constructed
  - Existing `BuildSpec` field mapping pattern (Method, Path, Description, Meta)

  **Test scenarios:**
  - Happy path: `BuildSpec` with aggregated endpoints that have params → `spec.Endpoint.Params` populated with correct Name, Type, Required values
  - Happy path: Param type mapping: DiscoveredParam{Type: "integer"} → spec.Param{Type: "integer"}
  - Happy path: npm source returns endpoints with params populated (end-to-end with mock tarball containing SDK code with params)
  - Edge case: AggregatedEndpoint with nil Params → spec.Endpoint.Params is nil (not empty slice)
  - Edge case: Mix of endpoints with and without params in same spec → endpoints without params serialize as `params: []` (existing behavior — `spec.Endpoint.Params` YAML tag lacks `omitempty`)
  - Happy path: `--json` output includes `param_count` field with correct total
  - Happy path: CLI integration — mock source with params → written spec YAML contains `params:` entries
  - Negative: GitHub source endpoints still have no params (verify github_test.go still passes)
  - Negative: Existing specgen tests pass without modification
  - Integration: Full pipeline — mock npm tarball with Steam-like SDK code → crowd-sniff produces spec with both endpoints AND params populated

  **Verification:**
  - `go test ./internal/crowdsniff/...` passes
  - `go test ./...` passes (no regressions anywhere)
  - `go build -o ./printing-press ./cmd/printing-press` succeeds

## System-Wide Impact

- **Spec output changes**: Generated crowd-sniff spec YAML files will now include `params:` sections on endpoints where params were discovered. This is purely additive — existing consumers (generate, mergeSpecs, templates) already handle `Params` from other spec sources (OpenAPI parser, websniff).
- **No generator template changes**: Templates already iterate `endpoint.Params` when present. crowd-sniff endpoints that previously had empty params now have populated params — the templates handle this naturally.
- **Minor CLI output addition**: `--json` output gains a `param_count` field (additive, non-breaking). Terminal summary unchanged. Params are visible in the output spec YAML.
- **Unchanged invariants**: `GrepEndpoints` return contract is unchanged (params field is nil when `EnrichWithParams` is not called). Existing endpoint dedup logic is unaffected. GitHub source is unmodified. All existing tests pass.

## Risks & Dependencies

| Risk | Mitigation |
|------|------------|
| Brace-matching scanner breaks on edge cases (template literals with `${}`, regex literals with `{n}`) | Cap scan distance at 500 chars from URL match. SDK HTTP calls typically have params within 200 chars. Test with diverse real-world SDK samples. |
| Raw-vs-cleaned path mismatch — endpoint.Path contains `{id}` but source code contains `${userId}` | EnrichWithParams must independently scan content with raw regex patterns, not substring-match the cleaned endpoint.Path. See Key Technical Decisions. Most likely implementation bug if missed. |
| Cross-product bug — params from one HTTP call attributed to another endpoint | Use positional matching (match endpoint's specific path in content to locate the right call). Include multi-call test fixtures. Known bug pattern in this codebase (cartesian product learnings doc). |
| Function signature correlation is unreliable for complex code | Default to `required: false` when correlation fails. This is the safe default — false-optional is inconvenient, false-required breaks the CLI. |
| Type inference heuristics are wrong for some APIs | Heuristics are conservative (default to `"string"`). Wrong type on a query param degrades UX slightly but doesn't break functionality. |
| Multi-line scanning introduces quadratic behavior on large files | Bound the scan: max 500 chars forward from URL match, max 1000 chars backward for function signature. SDK source files are typically 5-50KB. |

## Sources & References

- **Origin document:** [docs/brainstorms/2026-03-29-crowd-sniff-requirements.md](docs/brainstorms/2026-03-29-crowd-sniff-requirements.md)
- **Gap analysis:** `~/printing-press/manuscripts/postman-explore/20260330-105847/proofs/2026-03-30-crowd-sniff-param-discovery-gap.md`
- **Original crowd-sniff plan:** [docs/plans/2026-03-29-003-feat-crowd-sniff-plan.md](docs/plans/2026-03-29-003-feat-crowd-sniff-plan.md)
- **Cartesian product learning:** `docs/solutions/best-practices/multi-source-api-discovery-design-2026-03-30.md` (Pattern 6)
- **Word-boundary learning:** `docs/solutions/logic-errors/scorecard-accuracy-broadened-pattern-matching-2026-03-27.md`
- Related code: `internal/crowdsniff/`, `internal/websniff/specgen.go:inferURLParams()`, `internal/spec/spec.go:Param`
