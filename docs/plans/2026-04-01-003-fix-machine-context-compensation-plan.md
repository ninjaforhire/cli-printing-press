---
title: "fix: Machine context compensation — use research intelligence beyond the spec"
type: fix
status: active
date: 2026-04-01
origin: docs/retros/2026-04-01-steam-run4-retro.md
---

# Fix: Machine Context Compensation

## Overview

The machine leaves 9 recoverable scorecard points on the table because it only uses the spec for decisions that research context could inform better. When the spec says `steamid` is an integer with description "access key", the machine trusts that — but the research brief already says "API key required (free at steamcommunity.com/dev/apikey)" and competing tools all use `STEAM_API_KEY`. This plan makes the machine use its full context — research brief, ecosystem scan, absorb manifest — to compensate for spec gaps.

## Problem Frame

The scorecard measures CLI quality, not spec compliance. A CLI generated from an incomplete spec should still be good — the machine has research intelligence to fill the gaps. Currently, the generator uses only the spec. The skill (Claude's orchestration) has the research context but doesn't systematically use it to fix generator output. This plan bridges that gap.

(see origin: docs/retros/2026-04-01-steam-run4-retro.md — Key Insight section)

## Requirements Trace

- R1. `usageErr` only emitted when commands actually call it (generator template gate)
- R2. README always includes 5 scored sections; skill prevents Claude from dropping them; extra sections allowed
- R3. Profiler identifies searchable fields from response schemas, not just request body params
- R4. Skill instructs Claude to wire auth from research context when spec detection fails
- R5. Skill instructs Claude to enrich terse flag descriptions from research brief
- R6. Terminal UX scorer detects uninformative boilerplate instead of counting words
- R7. Sync correctness scorer adapts bonus when API has no parameterized list endpoints

## Scope Boundaries

- Not changing the auth inference 30% threshold (it's correct for spec-based detection)
- Not changing the scorecard's overall scoring architecture — only specific heuristics in terminal_ux and sync_correctness
- Not building domain-specific Search methods in the store template (that's a separate generator change — the profiler fix here enables it downstream)

## Key Technical Decisions

- **Skill instructions over generator changes for research-context items (#4, #5):** Auth wiring and description enrichment depend on research context that only exists during the skill-orchestrated build phase. The generator doesn't have access to the research brief. The right layer is the skill instruction, not the generator template.

- **README: mandatory sections + flexible extras:** The template emits all 5 scored sections. The skill tells Claude to preserve them but allows adding API-specific sections. The scorer checks for presence of required sections and doesn't penalize extras.

- **Terminal UX: boilerplate detection over word counting:** Replace the 5-word-average threshold with pattern detection for uninformative descriptions. "Check CLI health" (3 words, informative) should pass. "GetPlayerSummaries operation of ISteamUser" (6 words, boilerplate) should fail.

- **Sync correctness: conditional bonus:** The +3 path-params bonus should only apply when the spec has parameterized list endpoints. For flat APIs like Steam, the bonus is N/A and the dimension max becomes 7/7 (rescaled to 10).

## Implementation Units

- [ ] **Unit 1: Gate usageErr emission in helpers template**

**Goal:** `usageErr` only emitted when the spec has endpoints that use it (gated on `HasPositionalArgs`)

**Requirements:** R1

**Dependencies:** None

**Files:**
- Modify: `internal/generator/templates/helpers.go.tmpl`
- Modify: `internal/generator/generator.go` (add `HasUsageErr` to `HelperFlags` if needed — or reuse existing `HasPathParams`)

**Approach:**
- `usageErr` was previously called by commands with `Args: cobra.ExactArgs(N)`. PR #102 replaced those with `cmd.Help()` help-guards. Now `usageErr` is dead in every generated CLI. Either: (a) gate behind a flag that's never true (effectively removing it), or (b) just remove it from the template since no generated command calls it anymore.
- Simplest: remove `usageErr` from the template entirely. If a future template change reintroduces calls to it, the build will fail and we'll add it back then.

**Test scenarios:**
- Happy path: Generate from any spec → helpers.go does NOT contain `usageErr`
- Negative: Build still succeeds (no missing reference)

**Verification:**
- Dogfood reports 0 dead functions for `usageErr`

---

- [ ] **Unit 2: Harden README template with required sections**

**Goal:** README template always emits 5 scored sections; skill instruction preserves them

**Requirements:** R2

**Dependencies:** None

**Files:**
- Modify: `internal/generator/templates/readme.md.tmpl`
- Modify: `skills/printing-press/SKILL.md` (Phase 3 instruction)

**Approach:**
- Verify the README template already has all 5 sections: Quick Start, Agent Usage, Doctor/Health Check, Troubleshooting, Cookbook. Based on PR #102, it should — but confirm each is present with real content.
- Add a skill instruction in Phase 3 (after generation, during build): "The generated README contains 5 standard sections (Quick Start, Agent Usage, Health Check, Troubleshooting, Cookbook). When rewriting the README for this API, preserve all 5 sections. You may add additional sections that help users of this specific API, but never remove the standard ones."
- The scorer's README check should: (a) require the 5 sections, (b) not penalize extra sections. Check if the current scorer already works this way.

**Test scenarios:**
- Happy path: Generate from any spec → README has all 5 scored sections
- Happy path: Claude rewrites README during build → all 5 sections preserved
- Edge case: Claude adds "Rate Limits" section → no penalty from scorer

**Verification:**
- Scorecard README score ≥9/10 consistently across runs

---

- [ ] **Unit 3: Profiler analyzes response schemas for searchable fields**

**Goal:** Profiler identifies searchable string fields from GET response schemas, not just POST request bodies

**Requirements:** R3

**Dependencies:** None

**Files:**
- Modify: `internal/profiler/profiler.go` (`collectStringFields` or the searchable field logic)
- Test: `internal/profiler/profiler_test.go`

**Approach:**
- Currently `collectStringFields` only examines `endpoint.Body` params (request body). GET endpoints don't have bodies — their entities are in response schemas.
- The spec's `Endpoint.Response` field (a `ResponseDef`) may contain schema information. Check what `ResponseDef` provides.
- If response schemas have field names, add those to `SearchableFields` alongside body fields.
- If `ResponseDef` doesn't carry field info, this may need the OpenAPI parser to extract response field names during parsing. Defer that complexity to implementation.

**Test scenarios:**
- Happy path: Discord spec with messages resource (GET returns `content` field) → `SearchableFields["messages"]` includes "content"
- Happy path: Steam spec → profiler identifies string fields from response shapes
- Edge case: Endpoint with no response schema → no crash, no searchable fields added

**Verification:**
- Profile Steam spec → `SearchableFields` is non-empty
- Generated store has FTS5-enabled Search methods

---

- [ ] **Unit 4: Skill instruction — wire auth from research context**

**Goal:** When spec-based auth detection fails, Claude wires auth from the research brief during Phase 3

**Requirements:** R4

**Dependencies:** None

**Files:**
- Modify: `skills/printing-press/SKILL.md` (Phase 3 — after generation, during build)

**Approach:**
- Add instruction after the "REQUIRED: Rewrite the CLI description" block in Phase 2 (post-generation):
  - "Check if the generated config.go has auth env var support. If not, check the research brief for auth requirements. If the brief identifies an API key, token, or auth method, add the appropriate env var support to config.go (e.g., `STEAM_API_KEY` for Steam, `DISCORD_TOKEN` for Discord). Use the pattern from existing generated CLIs."
- This makes Claude responsible for compensating when the spec-based parser misses auth. Claude has the research brief in context and knows the auth pattern from the absorb manifest.

**Test scenarios:**
- Integration: Generate from Steam spec (no securitySchemes, auth inference misses by 0.3%) → Claude adds STEAM_API_KEY to config.go from research brief
- Negative: Generate from Stripe spec (auth correctly detected by parser) → Claude doesn't duplicate auth setup

**Test expectation: Skill instruction change — tested via full generation run, not unit test.**

**Verification:**
- Run `/printing-press steamapi` → config.go has STEAM_API_KEY without manual intervention

---

- [ ] **Unit 5: Skill instruction — enrich terse flag descriptions**

**Goal:** Claude enriches unhelpful flag descriptions from research context during Phase 3

**Requirements:** R5

**Dependencies:** None

**Files:**
- Modify: `skills/printing-press/SKILL.md` (Phase 3 — during build)

**Approach:**
- Add instruction in Phase 3 (Priority 3 — polish): "Review generated command flag descriptions. If any are under 5 words or are generic spec-derived text (e.g., 'access key', 'The player'), improve them using the research brief. For example, change 'access key' to 'Steam API key (get one at steamcommunity.com/dev/apikey)'. Focus on the flags users interact with most: auth keys, IDs, and filter parameters."
- This is a lightweight polish step, not a full rewrite. Claude should touch only terse/unhelpful descriptions.

**Test expectation: Skill instruction change — tested via full generation run.**

**Verification:**
- Generated commands have flag descriptions >5 words for key params

---

- [ ] **Unit 6: Scorer — terminal UX boilerplate detection**

**Goal:** Terminal UX scorer detects uninformative boilerplate instead of penalizing short-but-clear descriptions

**Requirements:** R6

**Dependencies:** None

**Files:**
- Modify: `internal/pipeline/scorecard.go` (`scoreTerminalUX`, specifically the description quality check)
- Test: `internal/pipeline/scorecard_test.go` or `scorecard_artifacts_test.go`

**Approach:**
- Replace the 5-word-average threshold with a boilerplate detection check. A description fails if it matches uninformative patterns:
  - Contains "operation of" (e.g., "GetPlayerSummaries operation of ISteamUser")
  - Starts with "Manage " followed by a raw interface/resource name with no verb describing what it manages
  - Is a raw camelCase operationId without humanization
- A description passes if it describes what the command DOES, regardless of length. "Check CLI health" (3 words) passes. "List friends" (2 words) passes.
- Keep the >10 chars + contains-a-space minimum as a baseline sanity check.

**Test scenarios:**
- Happy path: "Check CLI health" → passes (short but informative)
- Happy path: "List games owned by a Steam player, sorted by playtime" → passes
- Fail: "GetPlayerSummaries operation of ISteamUser" → fails (boilerplate)
- Fail: "Manage isteam cdn" → fails (raw interface name, no useful info)
- Edge case: "" (empty) → fails

**Verification:**
- Scorecard terminal_ux = 10/10 for CLIs with clear descriptions regardless of length

---

- [ ] **Unit 7: Scorer — sync correctness conditional path-params bonus**

**Goal:** Sync correctness +3 path-params bonus only applies when the API has parameterized list endpoints

**Requirements:** R7

**Dependencies:** None

**Files:**
- Modify: `internal/pipeline/scorecard.go` (`scoreSyncCorrectness`)

**Approach:**
- Before awarding the +3 bonus for `/{` patterns, check whether the spec has any parameterized list endpoints (paths containing `{` that also have pagination). If the spec has no such endpoints, the bonus is N/A — don't penalize.
- When the bonus doesn't apply, rescale the dimension: max becomes 7 instead of 10, and the final score maps 7/7 → 10/10.
- This ensures flat APIs like Steam aren't penalized for not having hierarchical resources.

**Test scenarios:**
- Happy path: Steam spec (no parameterized list endpoints) → sync_correctness bonus is N/A, score rescales to 10/10 from 7/7
- Happy path: Discord spec (has `/guilds/{guild_id}/channels`) → bonus applies normally
- Edge case: Spec with 0 syncable resources → sync_correctness is 0 (no bonus either way)

**Verification:**
- Scorecard sync_correctness = 10/10 for Steam CLI

## System-Wide Impact

- **Generator template (Unit 1):** Removes one function from generated CLIs. No runtime impact.
- **Skill instructions (Units 2, 4, 5):** Change Claude's behavior during generation. No binary or template changes.
- **Profiler (Unit 3):** Changes what fields the profiler identifies as searchable. Affects downstream store generation. No breaking changes — additive only.
- **Scorecard (Units 6, 7):** Changes how terminal_ux and sync_correctness are scored. Existing CLIs may score differently — likely higher since the changes remove false penalties.

## Risks & Dependencies

| Risk | Mitigation |
|------|------------|
| Boilerplate detection false positives (Unit 6) | Test with both informative and uninformative descriptions across multiple specs |
| Sync rescaling changes scores for existing CLIs | Only affects CLIs without parameterized list endpoints — same as current false penalty but in the correct direction |
| Skill instruction compliance varies by run | README section preservation is verifiable post-generation; auth wiring is checked by scorecard |
| Profiler response schema analysis adds complexity | Defer to implementation if ResponseDef doesn't carry field names |

## Sources & References

- **Origin:** [docs/retros/2026-04-01-steam-run4-retro.md](docs/retros/2026-04-01-steam-run4-retro.md)
- Prior PRs: #100, #101, #102, #103
- Scorecard: `internal/pipeline/scorecard.go`
- Generator templates: `internal/generator/templates/`
- Profiler: `internal/profiler/profiler.go`
- Skill: `skills/printing-press/SKILL.md`
