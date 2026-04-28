---
title: "feat(cli): Apply Cloudflare Wrangler CLI learnings to the printing press"
type: feat
status: active
date: 2026-04-13
---

# feat(cli): Apply Cloudflare Wrangler CLI learnings to the printing press

## Overview

Cloudflare's "Building a CLI for all of Cloudflare" (2026-04-13) describes their rebuild of Wrangler into a single CLI for 3000+ API operations. Four of their design choices map to the printing press. After checking current `main` against the article:

| Cloudflare learning | PP status |
|---|---|
| Agent Skills auto-generated alongside CLI | ALREADY DONE (#186 ships SKILL.md; #194/#212 validate it; #212 adds agentic SKILL reviewer) |
| Schema-layer naming consistency guardrails (`get` not `info`, `--json` not `--format`, `--force`) | NOT YET - gap this plan closes |
| Token-cost awareness for generated MCP surface | NOT YET - gap this plan closes |
| Self-describing agent-context endpoint for runtime introspection | NOT YET - gap this plan closes |
| TypeScript-as-schema | NOT APPLICABLE - PP's YAML already extends OpenAPI; replacing the schema layer is a separate strategic decision, not a 2026-04 tactical change |
| Local/remote mirror pattern (Local Explorer) | NOT APPLICABLE - most APIs PP wraps have no local equivalent |

The three gaps are additive and independent. Each can ship as its own PR.

## Problem Frame

Printed CLIs are one SKILL.md away from being genuinely agent-ready, but three issues keep them from compounding:

1. Without enforced verb and flag naming, agents guess and miss. If one printed CLI uses `info` and another uses `get`, an agent trained on one hallucinates commands on the other.
2. MCP tool surfaces are generated without any visibility into their token weight. Cloudflare's Code Mode MCP serves 3000 operations in <1000 tokens. A printed CLI's MCP could be 10x that and nobody would know.
3. SKILL.md is static at install time. A running CLI cannot introspect itself, so agents fall back to parsing `--help` or reading source.

## Requirements Trace

- R1. Every printed CLI uses a single, enforced vocabulary for command verbs and standard flags
- R2. Scorecard reports the token weight of the generated MCP tool surface, flagging bloated outputs
- R3. Every printed CLI exposes its own commands/flags/auth as structured JSON via a built-in subcommand for runtime agent discovery
- R4. None of the above break existing commands, existing gates, or already-published printed CLIs

## Scope Boundaries

- Does NOT rewrite PP's YAML/OpenAPI schema into a TypeScript schema
- Does NOT add local/remote mirroring (Cloudflare's Local Explorer)
- Does NOT modify SKILL.md generation - that work is already shipped and has its own validator
- Does NOT touch `verify` runtime testing, `emboss`, `publish`, or `research`
- Does NOT modify already-published CLIs in the public library repo - new generations get the new behavior; old CLIs pick it up on next regeneration

## Context & Research

### Relevant Code and Patterns

- `internal/pipeline/dogfood.go` - check functions (`checkPaths`, `checkDeadFlags`, `checkExamples`, `checkWiring`, `checkDeadFunctions`) are the pattern for a new `checkNamingConsistency` check. Note: `checkDeadFunctions` gained transitive reachability on 2026-04-12 (PR #183), confirming the regex+fixed-point pattern is the accepted style for structural checks
- `internal/pipeline/scorecard.go` - `Scorecard` struct with dimension scoring; pattern for a new `mcp_token_efficiency` dimension. Existing `UnscoredDimensions` mechanic handles "not applicable" cases cleanly
- `internal/generator/templates/command_endpoint.go.tmpl`, `command_promoted.go.tmpl` - command generators where naming consistency must originate
- `internal/generator/templates/mcp_tools.go.tmpl`, `main_mcp.go.tmpl` - the MCP surface whose size gets measured
- `internal/generator/templates/root.go.tmpl` - where the new `agent-context` subcommand registers
- `internal/generator/templates/skill.md.tmpl` - existing SKILL.md template. Agent-context subcommand should expose a superset of the same data structure so the two stay in sync
- `internal/generator/templates/doctor.go.tmpl` - shape for a purpose-built utility subcommand; agent-context should mirror this

### Institutional Learnings

- `docs/solutions/best-practices/steinberger-scorecard-scoring-architecture-2026-03-27.md` - scorecard dimension design; token-efficiency dimension should follow the same banding pattern
- `docs/solutions/logic-errors/scorecard-accuracy-broadened-pattern-matching-2026-03-27.md` - regex word-boundary lesson applies directly to Unit 1's naming detection
- AGENTS.md glossary - canonical names for `scorecard`, `dogfood`, `shipcheck` must be preserved
- AGENTS.md Machine vs Printed CLI rule - all three changes are machine-layer; every future CLI benefits

### External References

- Cloudflare blog, "Building a CLI for all of Cloudflare" (2026-04-13) - origin of these learnings
- Existing SKILL.md format on main (post-#186) - canonical agent-facing documentation shape for printed CLIs

## Key Technical Decisions

- **Naming consistency enforced in two places**: templates emit canonical names (source of truth), dogfood catches drift. Matches how PP handles auth and paths today.
- **Token measurement uses a lightweight approximation, not a real tokenizer**: `len(payload)/4` is good enough for relative comparison across CLIs and detecting regression. Swap for a real tokenizer only if a retro finds it misleading.
- **Agent-context subcommand emits JSON, not OpenAPI**: printed CLIs are not REST APIs - describing them as OpenAPI forces a lossy translation. Purpose-built JSON shape is clearer, smaller, and versioned via `schema_version` so future shape changes are signaled.
- **Agent-context data shape is a superset of SKILL.md's data**: both draw from the same generator-time inputs. SKILL.md is narrative markdown for agents at install time; agent-context is structured JSON at runtime. No shared runtime code needed - the generator emits each from the same template data.
- **All three changes are additive**: no existing behavior changes, no existing tests break.

## Open Questions

### Resolved During Planning

- **Is SKILL.md generation already done?** Yes - #186 shipped the template, #194 added a static validator, #212 added an agentic reviewer. Unit 2 from the first draft of this plan is removed.
- **Should naming consistency be a dogfood check or a scorecard dimension?** Dogfood. Wrong verb is a structural bug, not a quality gradient.
- **Does MCP token cost go in scorecard or as a separate report?** Scorecard dimension, using the existing `UnscoredDimensions` mechanic for CLIs that opt out of MCP generation.
- **Should agent-context replicate SKILL.md?** No - they serve different consumers (install-time markdown vs runtime JSON). Agent-context is additive.

### Deferred to Implementation

- **Exact list of banned/canonical verb pairs**: Start with the Cloudflare-cited set and let retros grow it. The rule catalog lives in one file so additions are trivial.
- **Token count algorithm choice**: `len(payload)/4` is the starting heuristic. Swap for `tiktoken-go` if accuracy matters.
- **Token scoring bands**: Exact thresholds are tuned from measuring the current catalog CLIs, not invented in the plan.

## Implementation Units

- [ ] **Unit 1: Naming consistency dogfood check**

**Goal:** Add a `checkNamingConsistency` dogfood check that catches non-canonical command verbs and flag names in the generated CLI. Every printed CLI fails dogfood if it uses `info` instead of `get`, `--format` instead of `--json`, or other banned variants. Fix the generator templates at the same time so new CLIs pass by default.

**Requirements:** R1, R4

**Dependencies:** None

**Files:**
- Create: `internal/pipeline/naming_rules.go` - canonical banned/preferred pairs with category (verb, flag) so rules live in one place and grow over time
- Modify: `internal/pipeline/dogfood.go` - add `checkNamingConsistency`, a new result type (similar in shape to `DeadCodeResult`), and wire it into `DogfoodReport`, `RunDogfood`, `deriveDogfoodVerdict`, and `collectDogfoodIssues`
- Modify: `internal/pipeline/dogfood_test.go` - tests for the new check
- Modify: `internal/generator/templates/command_endpoint.go.tmpl`, `command_promoted.go.tmpl` - audit and normalize any non-canonical names so generated output passes the new check
- Modify: `internal/generator/generator_test.go` - if the template audit finds any violations, add a test asserting the generated output is clean against the naming rules

**Approach:**
- Rules table entries: `{Banned: "info", Preferred: "get", Category: "verb"}`, `{Banned: "--format", Preferred: "--json", Category: "flag"}`, `{Banned: "--skip-confirmations", Preferred: "--force", Category: "flag"}`, plus whatever the template audit surfaces
- For verb checks, extract cobra `Use:` values from generated files and parse the first token
- For flag checks, parse cobra flag registrations (`Flags().Bool`, `Flags().String`, etc.) and check flag names
- Use word boundaries in regexes so `getInfoCached` does not trigger `info`
- Result includes the offending file, the banned name, and the canonical suggestion so the failure message is actionable

**Patterns to follow:**
- `checkDeadFlags` for file walking, flag extraction, and result construction
- `checkDeadFunctions` (post-#183 transitive version) for regex-with-word-boundaries discipline
- `deriveDogfoodVerdict` for how a new check folds into the overall verdict

**Test scenarios:**
- Happy path: generated fixture with `Use: "get"` and `--json` only - check reports 0 violations, verdict PASS
- Error path: fixture with a command declaring `Use: "info"` - violation reported with suggestion `get`
- Error path: fixture with a flag registration `--format` - violation reported with suggestion `--json`
- Edge case: identifier `getInfoCached` appears in body - no false positive (word boundary works)
- Edge case: a banned name appears inside a string literal comment - no false positive (parse context matters)
- Integration: full `RunDogfood` on a template-generated CLI - verdict is PASS after the template audit

**Verification:**
- `go test ./internal/pipeline/... -run TestCheckNamingConsistency` passes
- Regenerating a representative catalog CLI and running `printing-press dogfood` on it shows `NamingCheck: PASS`
- A deliberately-broken fixture produces FAIL with specific suggestions in the output

- [ ] **Unit 2: MCP token-cost scorecard dimension**

**Goal:** Measure and report the token weight of the generated MCP tool surface so bloated outputs are visible before publish. Cloudflare's Code Mode MCP serves 3000 operations in <1000 tokens; printed CLIs should have a visible target to compare against.

**Requirements:** R2, R4

**Dependencies:** None

**Files:**
- Create: `internal/pipeline/mcp_size.go` - helper to read the generated MCP tool surface and estimate token count, plus the scoring function for the new dimension
- Create: `internal/pipeline/mcp_size_test.go` - tests for size calculation and scoring bands
- Modify: `internal/pipeline/scorecard.go` - register `mcp_token_efficiency` as a new dimension, include in total, and support `UnscoredDimensions` for CLIs without MCP

**Approach:**
- After MCP templates render, scan `internal/mcp/` (or wherever the generated MCP tool list lives) and compute total char count of the serialized tool descriptions and parameter schemas
- Token estimate: `chars / 4` - simple heuristic good enough for relative comparison and regression detection
- Scoring bands (exact thresholds tuned post-measurement of catalog CLIs): small footprint = full marks, medium = partial, oversized = 0
- Scorecard report surfaces total tokens, tokens per tool, and the top-3 heaviest tools so authors know where to trim
- CLIs without MCP generation add `mcp_token_efficiency` to `UnscoredDimensions` so their total score is not penalized

**Patterns to follow:**
- Existing scorecard dimension scorers in `internal/pipeline/scorecard.go`
- `docs/solutions/best-practices/steinberger-scorecard-scoring-architecture-2026-03-27.md` for banding discipline
- Existing `UnscoredDimensions` mechanic for the opt-out case

**Test scenarios:**
- Happy path: small MCP surface (few tools, short descriptions) scores full marks
- Happy path: scorecard report includes total token count and per-tool breakdown
- Edge case: CLI with no MCP surface at all - dimension appears in `UnscoredDimensions`, not scored as zero
- Boundary: MCP size at exactly a scoring band threshold - verify inclusive/exclusive behavior is documented and tested
- Integration: real catalog CLI scorecard output shows the new dimension with a plausible token count

**Verification:**
- `go test ./internal/pipeline/... -run TestMCPTokenEfficiency` passes
- Running `printing-press scorecard` on a known catalog CLI shows the new dimension with a reasonable count
- A deliberately bloated MCP (inflated descriptions) drops the score and lists the bloated tools in the report

- [ ] **Unit 3: Agent-context JSON subcommand**

**Goal:** Every printed CLI exposes `<cli> agent-context` that emits a structured JSON description of its commands, flags, auth, and doctor capabilities. Agents can introspect a running CLI without parsing `--help` or reading source. Mirrors Cloudflare's `/cdn-cgi/explorer/api` pattern.

**Requirements:** R3, R4

**Dependencies:** None (the SKILL.md generation path supplies a reference for what data to expose, but no code dependency)

**Files:**
- Create: `internal/generator/templates/agent_context.go.tmpl` - the subcommand template
- Modify: `internal/generator/templates/root.go.tmpl` - register the new subcommand
- Modify: `internal/generator/generator.go` - wire the new template into generation
- Modify: `internal/generator/generator_test.go` - assert the file is generated and the subcommand is wired; update expected file counts if relevant
- Modify: `internal/pipeline/dogfood.go` (`checkWiring` or sibling) - assert `agent-context` is registered

**Approach:**
- Generated subcommand outputs JSON by default; `--pretty` for human reading
- JSON shape (version-gated): `{ schema_version: "1", cli: {name, version, description}, auth: {mode, env_vars}, commands: [{use, short, flags: [{name, type, description, required}], examples: []}], doctor: {checks: []} }`
- Data source: generator has all of this at build time; emit as an embedded constant inside the generated file (no runtime spec parsing, no file IO)
- Stable ordering: sort commands and flags alphabetically so output diffs cleanly across regenerations
- `schema_version` exists from day one so future shape changes are signaled

**Patterns to follow:**
- `doctor.go.tmpl` for purpose-built utility subcommand shape
- `command_endpoint.go.tmpl` for cobra command registration idioms
- Existing SKILL.md generation for how the same generator data feeds multiple outputs

**Test scenarios:**
- Happy path: generated CLI runs `<cli> agent-context` and emits valid JSON that parses
- Happy path: JSON includes every registered top-level command
- Happy path: `--pretty` produces indented output; default is compact
- Edge case: CLI with no auth (rare) produces `auth: {mode: "none"}`, not an omitted field
- Edge case: CLI with zero custom commands (only scaffolding) produces `commands: []`, not a missing field
- Integration: generating two different catalog CLIs produces structurally identical JSON shapes with API-specific contents

**Verification:**
- `go test ./internal/generator/...` passes
- Running a generated CLI's `agent-context` produces valid JSON matching the documented shape
- Running the same against two catalog CLIs shows structural parity with different payloads
- `dogfood` wiring check confirms `agent-context` is registered

## System-Wide Impact

- **Interaction graph:** Only pipeline (dogfood, scorecard) and generator (templates, pipeline) are touched. No changes to `verify`, `emboss`, `publish`, `research`, `crowdsniff`, or SKILL.md generation.
- **Error propagation:** New dogfood check surfaces through existing `DogfoodReport` and `deriveDogfoodVerdict`. New scorecard dimension uses the existing `UnscoredDimensions` pattern for opt-outs.
- **State lifecycle risks:** None - all changes are generation-time and static-analysis-time.
- **API surface parity:** Agent-context subcommand is additive. SKILL.md (already generated) remains unchanged. Existing commands are untouched.
- **Unchanged invariants:** Quality gates (7 static checks) unchanged. Existing scorecard dimensions and weights unchanged. Existing dogfood checks unchanged except for additive wiring. SKILL.md generation, validator, and agentic reviewer (#186/#194/#212) unchanged. Published CLIs in the public library repo are not auto-modified.

## Risks & Dependencies

| Risk | Mitigation |
|------|------------|
| Unit 1 naming rules cause mass failures on existing catalog CLIs when regenerated | Audit and fix templates in the same PR. Run Unit 1 locally against all catalog CLIs before merging; if any fail, the fix goes into the same PR as the check |
| Token heuristic (`chars/4`) diverges from real tokenizer on non-English or code-heavy content | Accepted - goal is relative comparison and regression detection. Swap for `tiktoken-go` if a retro finds it misleading |
| Agent-context JSON shape becomes a de-facto public contract that constrains future changes | `schema_version: "1"` from day one; breaking changes increment the version and are signaled |
| Scoring band thresholds for MCP token efficiency are invented without data | Measure current catalog CLIs first; set bands based on observed distribution, not guess |
| Three units landing as separate PRs could each trigger a minor version bump | Acceptable - each is an additive feature and deserves its own changelog entry. If batching is preferred, land them as one PR with three commits |

## Documentation / Operational Notes

- Update `AGENTS.md` glossary to define `agent-context` subcommand and the `schema_version` convention
- Update `AGENTS.md` Commit Style section only if a new scope is needed (likely not - `cli` covers all three)
- Consider a short section in the retro template or `/printing-press-retro` skill that calls out the new checks so retros can identify which CLIs would regress under them
- Release plan: each unit can ship as `feat(cli)` - release-please accumulates and the next release PR picks them up

## Sources & References

- Cloudflare blog, "Building a CLI for all of Cloudflare" (2026-04-13)
- PR #186 - SKILL.md generation (reason Unit 2 from first draft was dropped)
- PR #194 - static SKILL.md validator
- PR #212 - agentic SKILL reviewer
- PR #183 - transitive dogfood dead function scanner (recent example of additive dogfood check)
- `docs/solutions/best-practices/steinberger-scorecard-scoring-architecture-2026-03-27.md`
- `AGENTS.md` for naming conventions, versioning rules, and Machine vs Printed CLI discipline
