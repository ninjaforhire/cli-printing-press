# Printing Press Retro: Steam Web API (Run 4)

## Session Stats
- API: Steam Web API
- Spec source: Zuplo/Steam-OpenAPI (158 operations, OpenAPI 3.0)
- Scorecard: **84/100 Grade A** (stable from Run 3's 85)
- Verify pass rate: 77% (62/81, 0 critical)
- Machine PRs applied: #100, #101, #102, #103
- Journey: 68 → 70 → 84 → 85 → 84

## Core Question: Scorer accuracy vs API limitations

For each remaining deduction, the question is: **does the scoring reflect something the CLI could realistically do better given Steam's API, or is it penalizing the CLI for Steam's limitations?**

## Findings

### 1. Auth 8/10 — inference threshold off by 1 operation (Scorer design edge case)

- **Scorer correct?** The scoring itself is correct — the CLI genuinely doesn't have auth wired in the generated config. But the auth inference (PR #103) didn't fire because Steam has `key` on 47/158 operations = 29.7%, just under the 30% threshold. One more operation would trigger it.
- **Is this a real CLI gap?** No — the CLI works fine with `STEAM_API_KEY` env var (we added it manually). The generated config just doesn't auto-detect it.
- **Is the threshold right?** 30% is reasonable for avoiding false positives. Steam being at 29.7% is bad luck, not a design flaw. Lowering to 25% would catch Steam but might false-positive on APIs where only a few optional endpoints accept keys.
- **Recommendation:** Don't change the 30% threshold — it's correct for spec-based detection. But the machine has a second path: the research brief explicitly says "API key required (free at steamcommunity.com/dev/apikey)" and the absorb manifest shows every competing tool uses `STEAM_API_KEY`. The skill should tell Claude: "If research identified auth requirements that the spec didn't declare, wire them into config.go during Phase 3." This uses context the machine already has.
- **Verdict: Fixable via skill instruction.** The spec-based inference is borderline, but the research-based context is unambiguous. +2 points recoverable.

### 2. Data pipeline 7/10 — no domain-specific Search methods (Generator gap — scorer is correct)

- **Scorer correct?** Yes. The store has generic `Search()` but no `SearchPlayers()`, `SearchGames()`, etc. The scorer gives +3 for domain-specific Search.
- **Why isn't the generator emitting them?** The store template gates Search on `{{if .FTS5}}` — but the Steam spec's tables aren't getting the FTS5 flag from the profiler. The profiler's `collectStringFields()` looks for string-type body params, but Steam's entities (games, players) are returned in GET responses, not POST bodies. The profiler doesn't analyze response schemas for searchable fields.
- **Is this a real CLI gap?** Yes — domain-specific Search would make the CLI genuinely better. The Steam CLI already has a generic `search` command, but `SearchPlayers("gabe")` would be more useful.
- **Recommendation: Fix the profiler.** `collectStringFields` should also analyze response schemas (not just request body params) to find searchable fields. This is a real generator improvement.
- **Verdict: Real gap, fixable. +3 points recoverable.**

### 3. Sync correctness 7/10 — no path params in list endpoints (API limitation — scorer should adapt)

- **Scorer correct?** Partially. The +3 bonus for `/{` path params rewards APIs like Discord (`/guilds/{guild_id}/channels`) where sync needs to iterate parent resources. Steam's list endpoints (`/ISteamApps/GetAppList/v2/`) genuinely don't have path params — there's no parent-child hierarchy to traverse.
- **Is this a real CLI gap?** No. Steam's sync doesn't NEED path params because the list endpoints return all data without parameterization. The CLI isn't worse for lacking something the API doesn't require.
- **Should the scorer penalize this?** This is a scorer design question. The +3 bonus assumes path-parameterized sync is universally better. For Steam, it's irrelevant. The scorer should either: (a) not award the bonus when the API has no parameterized list endpoints (don't penalize what doesn't apply), or (b) keep the bonus as-is and accept that non-hierarchical APIs score lower on this dimension.
- **Recommendation: This is a scorer design issue worth discussing.** The scorer rewards a capability that not every API needs. It's not "wrong" — it's a design choice about what "complete sync" means. For now, accept the 3 points.
- **Verdict: API limitation, not CLI gap. Scorer could adapt but it's a design tradeoff, not a bug.**

### 4. README 7/10 — inconsistent section generation (Skill instruction gap)

- **Scorer correct?** Yes — the README is missing content. Run 3 got 9/10 with a better README. The difference is Claude's README generation varies between runs.
- **Should the machine harden this?** Yes, but carefully. The README template already has Cookbook, Agent Usage, Troubleshooting, Health Check sections. The issue is Claude's agent sometimes rewrites the README and drops sections, or the template's output doesn't fully satisfy the scorer's quality checks.
- **Is mandating sections too inflexible?** No — the scorer checks for specific sections (Quick Start, Agent Usage, Doctor, Troubleshooting, Cookbook). These are genuinely useful for every CLI. The machine should ensure they're always present. But the CONTENT should vary by API — mandating sections is fine, mandating content is not.
- **Recommendation:** Two improvements: (1) The README template should emit all 5 scored sections with real content. (2) The skill instruction for Phase 3 should say "preserve all README sections from the template — do not delete Agent Usage, Troubleshooting, or Cookbook when rewriting." This prevents Claude from accidentally dropping sections.
- **Verdict: Skill instruction gap, fixable. +2 points recoverable by hardening the README template.**

### 5. Terminal UX 9/10 — short descriptions on generated commands (Both — partially scorer, partially real)

- **Scorer correct?** Partially. The scorer checks that sampled command descriptions are >10 chars AND contain a space (multi-word). Descriptions like "Manage isteam cdn" (17 chars) pass the length check. The issue is the scorer's quality heuristic: average word count >5 across sampled commands.
- **Are short descriptions actually bad?** Not always. "Check CLI health" (16 chars, 3 words) is perfectly clear. "Manage isteam cdn" is fine for a generated subcommand. The scorer's 5-word average threshold penalizes APIs with many simple subcommands — which is what happens when the spec has 50+ resources with terse names.
- **Should the scorer penalize this?** The 5-word average threshold is too blunt. A description should be penalized for being UNINFORMATIVE ("GetPlayerSummaries operation of ISteamUser") not for being SHORT ("Check VAC and game bans"). The quality signal should be "does the description tell the user what the command does?" not "is it long enough?"
- **Recommendation:** This is a scorer design issue. The word-count heuristic is a proxy for quality but penalizes naturally concise descriptions. Better: detect boilerplate patterns ("operation of", "Manage <interface>") rather than counting words. For now, accept the 1 point — the descriptions are adequate.
- **Verdict: Scorer design issue — word-count proxy penalizes concise commands. Not a CLI gap.**

### 6. Dead code 4/5 — usageErr still emitted (Generator bug — scorer is correct)

- **Scorer correct?** Yes. `usageErr` is defined but never called.
- **Why is it still there?** PR #100's retro identified this and PR #102 made `replacePathParam` conditional. But `usageErr` was supposed to be gated on positional args too — PR #102's help-guard pattern replaced `usageErr` calls with `cmd.Help()`. The function is now dead because no command calls it, but the template still emits it unconditionally.
- **Why can't the machine do this automatically?** It should. The generator template emits `usageErr` unconditionally — it should be gated behind a `HasPositionalArgs` flag (same pattern as `replacePathParam`). This is the same fix approach as PR #102's conditional emission, just for one more function.
- **Why does polish need to catch it?** It shouldn't. This is a generator bug — the function should never be emitted if no command uses it. The dogfood tool catches it, but the generator should have prevented it.
- **Recommendation: Fix the generator template.** Gate `usageErr` emission behind the same `HasPathParams` or `HasPositionalArgs` flag that gates `replacePathParam`. This is a 1-line template fix.
- **Verdict: Generator bug, trivially fixable. +1 point recoverable.**

### 7. Vision 9/10 and Insight 9/10 — within noise (Minor)

- Both are 1 point from 10/10. The specific missing element varies by run — sometimes FTS5 detection, sometimes a wiring issue. Not worth investigating further for Steam. Would need to verify on a different API to know if it's systematic.
- **Verdict: Accept. 2 points within run-to-run variation.**

### 8. Type fidelity 3/5 — flag descriptions and required count (Spec-dependent)

- **Scorer correct?** Yes. Short flag descriptions come from the spec's parameter descriptions. "access key" (2 words) is what the Zuplo spec provides.
- **Is this a real CLI gap?** Yes — "access key" is unhelpful to a user. The research brief says "API key required (free at steamcommunity.com/dev/apikey)" — that's what the description SHOULD say. The machine has the context to write better descriptions; it's just not using it.
- **Can the machine compensate?** Yes. During Phase 3, when Claude builds wrapper commands, it already writes rich descriptions from research context ("Check VAC and game bans for a Steam player"). The generated raw commands could be enriched the same way — either by Claude during build, or by a post-generation enrichment pass that maps terse spec descriptions to research-informed alternatives.
- **Verdict: Fixable via skill instruction.** Tell Claude to enrich terse flag descriptions (under 5 words) from the research brief during Phase 3. +1-2 points recoverable.

## Prioritized Improvements

### Fix the Scorer
| # | Scorer | Issue | Recommendation |
|---|--------|-------|----------------|
| 3 | Sync correctness | +3 bonus for path params penalizes APIs that don't need them | **Discuss design:** should the bonus scale down or not apply when spec has 0 parameterized list endpoints? Not clearly wrong — it's a design tradeoff. |
| 5 | Terminal UX | 5-word avg description threshold penalizes concise commands | **Fix:** detect uninformative boilerplate ("operation of", "Manage <interface>") instead of counting words |

### Do Now
| # | Fix | Component | Impact | Complexity |
|---|-----|-----------|--------|------------|
| 6 | Gate `usageErr` emission behind HasPositionalArgs | `helpers.go.tmpl` | +1 point, every API | Trivial |
| 4 | Harden README template to always emit scored sections | `readme.md.tmpl` + skill instruction | +2 points | Small |

### Do Next
| # | Fix | Component | Impact | Complexity |
|---|-----|-----------|--------|------------|
| 2 | Profiler: analyze response schemas for searchable fields | `profiler.go` | +3 points | Medium |

### Do Next (cont.)
| # | Fix | Component | Impact | Complexity |
|---|-----|-----------|--------|------------|
| 1 | Auth: wire from research context when spec detection fails | Skill instruction + Phase 3 | +2 points | Small |
| 8 | Enrich terse flag descriptions from research brief | Skill instruction + Phase 3 | +1-2 points | Small |
| 7 | Investigate vision/insight consistency across runs | Scorecard + generator | +1-2 points | Small |

### Scorer Design Questions (not bugs — tradeoffs to discuss)
| # | Dimension | Question |
|---|-----------|----------|
| 3 | Sync correctness | Should +3 path-params bonus apply when the API has no parameterized list endpoints? Currently penalizes flat APIs for not having hierarchical resources. |
| 5 | Terminal UX | Should description quality check detect uninformative boilerplate instead of counting words? "Check CLI health" (3 words) is clear; "GetPlayerSummaries operation of ISteamUser" (5 words) is useless. |

### Accept (truly API-limited)
| # | Gap | Why accept |
|---|-----|-----------|
| 3 | Sync path params 3pts | Steam genuinely has no hierarchical list endpoints. The CLI can't add path params the API doesn't have. This is the one truly API-limited gap. |

## Key Insight: The Scorecard Measures CLI Quality, Not Spec Compliance

An earlier version of this retro framed 4 points as "spec quality limitations" and set a ceiling of ~90. **That framing was wrong.** The scorecard measures "how good is this CLI as a tool," not "how well does the CLI implement the spec." A CLI generated from a bad spec that perfectly implements that bad spec is still a bad CLI.

The corrected breakdown:

**Of the 16 lost points:**
- **3 points truly API-limited** (sync path params — Steam doesn't have hierarchical resources, nothing to fix)
- **9 points fixable by the machine** — the machine has context beyond the spec (research brief, ecosystem scan, absorb manifest) that it's not using:
  - Auth +2: Research identified Steam needs API keys. The spec didn't declare it, but Claude knows. The skill should wire auth from research when spec detection fails.
  - Data pipeline +3: Profiler should analyze response schemas, not just request params.
  - README +2: Template should always emit scored sections; skill should prevent Claude from dropping them.
  - Dead code +1: Generator should gate `usageErr` behind `HasPositionalArgs`.
  - Type fidelity +1: Claude could enrich terse flag descriptions from the research brief during Phase 3.
- **2 points from scorer design tradeoffs** (terminal UX word-count heuristic, sync bonus design)
- **2 points from run-to-run variation** (vision, insight consistency)

**The theoretical ceiling is ~97.** Only the 3 sync points are truly unrecoverable for this API. Everything else is the machine not using context it already has.

The previous "ceiling of 90" was giving the machine too much credit and the spec too much blame. A bad spec is not an excuse — it's a signal that the machine needs to compensate using its other intelligence sources.

## README Design Decision

The README template should:
1. **Always include** the 5 scored sections: Quick Start, Agent Usage, Doctor, Troubleshooting, Cookbook
2. **Allow additional sections** when they're useful for the specific API (e.g., "Rate Limits" for APIs with documented limits, "Pagination" for APIs with complex paging)
3. The skill should tell Claude: "You may add sections that help users of this specific API, but never remove the 5 standard sections."
4. The scorer should: (a) verify required sections are present, (b) not penalize extra sections, (c) penalize filler/irrelevant sections if they add noise

## Anti-patterns to Avoid

- **Blaming the spec for machine failures.** When the spec is incomplete, the machine has research context, ecosystem data, and Claude's knowledge to compensate. "The spec didn't declare auth" is not an excuse when the research brief says "API key required (free at steamcommunity.com/dev/apikey)."
- **Setting artificial ceilings.** A ceiling implies "we can't do better." But 9 of the 16 lost points ARE fixable. Calling them "spec limitations" creates a false sense of completion.
- **Optimizing the auth threshold for one borderline API.** 30% is still the right threshold for spec-based detection. The fix is a different path — use research context as a fallback, not weaken the spec-based heuristic.

## What the Machine Got Right

**Stable Grade A across 2 runs (84-85).** The retro→plan→implement loop works. But the machine is leaving 9 recoverable points on the table by not using context it already has (research brief, ecosystem scan). The next improvement round should focus on making the machine use its full context — not just the spec — to compensate for spec gaps.
