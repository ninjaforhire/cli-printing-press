# CLI Printing Press

Just making a CLI is not hard. Making a CLI that understands the power user is extremely hard.

```bash
/printing-press Discord
/printing-press Stripe
/printing-press Linear
```

One command. 13 phases. ~1 hour. Produces a Go CLI + MCP server + 8 analysis documents. REST or GraphQL. Matches every competitor feature, then adds the data layer they don't have.

### Get it

```bash
/install-skill https://github.com/mvanhorn/cli-printing-press
```

## Every Endpoint. Every Insight. One Command.

Discord's API has 300+ endpoints. Most generators stop there - wrap every endpoint, ship it, done. But [discrawl](https://github.com/steipete/discrawl) - Peter Steinberger's Discord tool - ignores most of them. It ships 11 commands: `sync`, `search`, `sql`, `tail`, `mentions`, `members`. **583 stars.**

Why does the 11-command tool win? Because Steinberger saw something Discord's own API designers didn't: **conversations are institutional knowledge.** Every message thread is a document that should be archived, indexed, and searched locally. Those 11 commands embody that insight. The 300 endpoint wrappers don't.

Until now, you had to choose: breadth (wrap every endpoint) or depth (understand the user). The printing press eliminates that choice. It generates the full API surface AND matches every feature the top competitor has AND adds the discrawl-style intelligence layer AND an MCP server. One spec in. Everything out.

## The Non-Obvious Insight

Every API has a secret identity. The data it exposes is useful for something its creators never designed for. The printing press finds that secret and builds a CLI around it.

The **Non-Obvious Insight (NOI)** is a one-sentence reframe:

```
"[API] isn't just [obvious thing]. It's [non-obvious thing].
 Every [data point] is a signal about [hidden truth]."
```

| API | What they think it is | What it actually is |
|-----|----------------------|---------------------|
| Discord | A chat app | A **searchable knowledge base**. Every message thread is institutional memory. |
| Linear | An issue tracker | A **team behavior observatory**. Every state change is a signal about how your team actually works vs. how they think they work. |
| Stripe | A payment processor | A **business health monitor**. Every failed charge and churn event is a signal about product-market fit. |
| GitHub | A code host | An **engineering culture fingerprint**. Every review turnaround and merge pattern is a signal about how your team ships. |
| Notion | A doc editor | A **knowledge decay detector**. Every stale page and orphaned database is a signal about what your team has forgotten. |
| Slack | Messaging | An **organizational nervous system**. Every response time and channel silence is a signal about team health. |

The NOI is the creative DNA of every CLI the press generates. Phase 0 cannot complete without one. If the LLM can't write an NOI, the research wasn't deep enough.

The printing press automates what Steinberger does intuitively: look at an API, see what power users actually do with it, and build the commands that matter - then also wrap every endpoint for completeness.

## How I Knew This Was Real

I was deciding which Google Workspace CLI to use. Peter Steinberger's [gogcli](https://github.com/steipete/gogcli) (6.5K+ stars, Go) or Google's official [Workspace CLI](https://github.com/googleworkspace/cli) (10K+ stars in a week, Rust, dynamically generated from Google's Discovery Service).

I ran [/last30days](https://github.com/mvanhorn/last30days-skill) - my recency research skill that searches Reddit, X, YouTube, and the web for what people actually say about tools. It searched 34 X posts (1,437 likes), 5 YouTube videos (57K views), and 10 web sources.

The verdict surprised me: **use gogcli**. The newer, official tool with 10x the API coverage lost to the older third-party one. As [@7dyhn4542y put it on X](https://x.com): "my preference is 100% gogcli since I have my agent working a lot with Google Docs and sheets, and gogcli just makes him able to do what he needs to do."

Google's CLI wraps every endpoint but doesn't understand the user. Steinberger's CLI understands what people actually do with Gmail, Calendar, and Sheets - and builds human-friendly commands around those workflows. Setup is `brew install gogcli` vs. a multi-step Google Cloud Console OAuth dance.

That's the NOI again. Breadth doesn't beat depth. Understanding the user beats understanding the API. And /last30days saw it in the community data before I could see it myself.

## The Creativity Ladder

Most API CLIs stop at Rung 1. The printing press climbs to Rung 5.

| Rung | What It Is | Auto-Generated? | Example |
|------|-----------|-----------------|---------|
| 1 | API wrapper commands | Yes (from spec) | `issue create --title "..."` |
| 2 | Output formatting | Yes (always) | `--json`, `--select`, `--csv`, `--dry-run` |
| 3 | Local persistence | Yes (conditional) | `sync`, `search`, `export`, `tail` |
| 4 | **Domain analytics** | **Yes (from archetype)** | `stale --days 30`, `orphans`, `load` |
| 5 | **Behavioral insights** | **Yes (from archetype)** | `health` (composite score), `similar` (duplicate detection) |

Rung 3 is table stakes. Rung 4 is where discrawl lives. Rung 5 is where nobody else is yet.

The press generates the API wrapper in Phase 2 (Rung 1-2). Then it generates the discrawl-style data layer and workflow commands in Phase 4 (Rung 3-5) from domain archetype templates. Both in one run.

## Why Not Just CLIs - CLIs + MCP

The NOI is the creative intelligence. The printing press generates **both interfaces** from one spec:

- **`<api>-pp-cli`** - cobra CLI for humans + shell agents (Claude Code, Codex, Gemini CLI)
- **`<api>-pp-mcp`** - MCP server for Claude Desktop, Cursor, Windsurf, Cline

Same `internal/client`, same `internal/store`, same auth. Two binaries, zero code duplication.

**CLIs win for agents:** 100x fewer tokens than MCP tool definitions. LLMs were trained on shell interactions. Exit code 0 = done. `--json | jq` is a first-class composition pattern.

**MCP wins for IDE integration:** Claude Desktop and Cursor discover tools automatically via MCP. No shell needed. The MCP server exposes the same operations as the CLI - including the data layer (sync, search, sql).

```
One spec  -->  printing-press generate  -->  <api>-pp-cli (cobra)  +  <api>-pp-mcp (MCP server)
                                              |                         |
                                              same internal/client, internal/store
```

Every API that gets a CLI+MCP becomes instantly accessible to every AI coding tool. The printing press is the factory.

## Domain Archetypes

The profiler classifies every API into a domain archetype and auto-generates the right workflow + insight commands:

| Archetype | Detected By | Auto-Generated Commands |
|-----------|------------|------------------------|
| **Project Management** | issue/task/ticket resources, assignee fields, priority levels | `stale`, `orphans`, `load`, `health`, `similar` |
| **Communication** | message/channel/thread resources, threading fields | `channel-health`, `message-stats`, `health`, `similar` |
| **Payments** | charge/payment/invoice resources, amount/currency fields | `reconcile`, `revenue`, `health`, `similar` |
| **Infrastructure** | server/deploy/instance resources | `health`, `similar` |
| **Content** | document/page/block resources | `health`, `similar` |

The archetype is detected automatically from the spec. The entity mapper figures out which resource is the "primary entity" (issues for PM, messages for comms, charges for payments) and wires the templates accordingly.

## How It Works

12 phases. Each writes a plan document. The artifacts are the product.

```
Phase 0     Visionary Research        (3-5 min)    NOI + domain identity + usage patterns
Phase 0.1   API Key Prompt            (optional)   Offer live testing at end
Phase 0.5   Power User Workflows      (2-3 min)    Compound commands power users want
Phase 0.6   Feature Parity Audit      (5 min)      [NEW] Catalog competitor features, classify table stakes
Phase 0.7   Prediction Engine         (15-25 min)  SQLite schema + FTS5 + sync strategy
Phase 0.8   Product Thesis            (2 min)      Name (<api>-pp-cli), positioning, anti-scope with cost analysis
Phase 1     Deep Research             (5-8 min)    Competitors, strategic justification
Phase 2     Generate                  (1-2 min)    Go CLI + MCP server from spec + name/path/version validation
Phase 3     Non-Obvious Insight Review(5-8 min)    Two-tier scoring + competitor feature matrix
Phase 4     GOAT Build                (20-30 min)  7 priorities: data layer, table stakes, workflows, names, tests, distribution
Phase 4.7   Proof of Behavior         (30 sec)     Verify data actually flows (no hallucinations)
Phase 4.9   Agent Readiness           (auto)       CLI agent readiness reviewer loop (max 2 passes)
Phase 5     Ship Readiness Assessment (2-3 min)    Three-benchmark gate: architecture + quality + features
Phase 5.5   Live API Testing          (optional)   Read-only tests + data pipeline smoke test
Phase 5.7   Ship Loop                 (auto)       Fix issues and re-score until PASS
Phase 5.9   Offer Emboss              (prompt)     [NEW] Opt-in second pass to improve further
```

### Codex Mode (opt-in)

```bash
/printing-press Discord codex    # Offload code generation + fix patches to Codex CLI (~60% Opus token savings)
/printing-press Discord          # Standard Opus mode (default)
```

When you add `codex`, Phase 4's code generation tasks and later fix-application passes are delegated to Codex CLI. Claude stays the brain (research, planning, scoring, review). Codex does the hands (writing Go code from scoped prompts). Same quality, 60% fewer Opus tokens.

## What Gets Generated

**Agent-first flags** (every command): `--json`, `--select`, `--dry-run`, `--stdin`, `--csv`, `--compact`, `--quiet`, `--yes`, `--no-input`, `--no-cache`, `--no-color`. Auto-JSON when piped (no `--json` needed). Typed exit codes (`0`=success, `2`=usage, `3`=not found, `4`=auth, `5`=API, `7`=rate limited).

**Actionable errors**: errors include the specific flag/arg that's wrong, the correct usage pattern, and the command path. Agents self-correct in one retry.

**Bounded output**: list commands show "Showing N results. To narrow: add --limit, --json --select, or filter flags." Token-conscious `--compact` mode returns only high-gravity fields (id, name, status, timestamps) - 60-80% fewer tokens.

**Table stakes features** (from Phase 0.6): every feature the top competitor has, classified and built before novel features. If schpet/linear-cli has `start` (git branch from issue), you get it. If 4ier/notion-cli has human-friendly filters, you get it. Anti-gaming rules prevent scorecard optimization over real features.

**Data layer** (high-gravity entities): domain-specific SQLite tables with proper columns (not JSON blobs), FTS5 full-text search, incremental sync with cursor tracking, `sql` command for raw queries, domain-specific `UpsertX()` and `SearchX()` methods.

**Workflow commands** (from archetype): `stale`, `orphans`, `load`, `channel-health`, `reconcile`, etc.

**Insight commands** (Rung 5): `health` (composite score), `similar` (duplicate detection), `trends`, `bottleneck`, `forecast`, `patterns`.

**Command name normalization**: generated names like `retrieve-a` become `get`, `post` becomes `create`, `patch` becomes `update`. Clean names, not operationId garbage.

**Tests**: minimum 1 test file per package (store, cli). Table-driven tests for data layer queries and workflow commands. No more shipping with 0 test files.

**Distribution scaffold**: `.goreleaser.yaml`, Homebrew formula, GitHub Actions CI. A CLI that can only be `go install`'d is not a real CLI.

**REST + GraphQL**: OpenAPI specs generate full CLIs. GraphQL SDL files are parsed with Relay pagination detection and produce the same domain-specific output.

**MCP server** (auto-generated): Every CLI gets a companion `cmd/api-mcp/main.go` that exposes the same operations as MCP tools. Same client, same store, same auth. Works with `claude mcp add ./bin/api-mcp`.

**Sync performance** (discrawl-inspired): Cursor-based pagination, batch SQLite transactions, tuned pragmas (`synchronous=NORMAL`, `mmap_size=256MB`), `--since` incremental sync, `--concurrency` parallel workers, progress reporting to stderr.

## Quality Scoring (v2 - Three Benchmarks)

Three benchmarks, not one. All must pass:

1. **Architecture** (discrawl benchmark): Does it have a real data layer - domain-specific SQLite, FTS5, incremental sync, workflow commands?
2. **Quality** (gogcli benchmark): Does the code have proper output modes, typed errors, agent-native flags, doctor, README with cookbook?
3. **Features** (competitor benchmark): Would a user of the top competitor switch to this CLI?

Architecture without features is a toy. Features without architecture is a thin wrapper. Quality without either is polished nothing.

Inspired by Peter Steinberger's [gogcli](https://github.com/steipete/gogcli). Two tiers, 100 points max, weighted 50/50. Grade A = 85+.

**Tier 1: Infrastructure** (50 points) - does the skeleton have the right patterns?

| Dimension | What It Checks |
|-----------|---------------|
| Output Modes | --json, --csv, --select, --quiet, --compact, auto-JSON when piped |
| Auth | OAuth flow, format-aware headers (Bot/Bearer/Basic from spec) |
| Error Handling | Typed exits, retry with backoff, actionable error messages |
| Agent-Native | --json, --select, --dry-run, --stdin, --no-input, --compact, --yes |
| + 5 more | Terminal UX, README, Doctor, Local Cache, Breadth |

**Tier 2: Domain Correctness** (50 points) - does the code actually work?

| Dimension | What It Checks |
|-----------|---------------|
| Path Validity | Generated paths exist in the OpenAPI spec |
| Auth Protocol | Auth format matches spec's securitySchemes |
| Data Pipeline | Sync calls domain-specific UpsertX(), not generic Upsert() |
| Sync Correctness | Real resources, nested paths, pagination, incremental cursors |
| Type Fidelity | String IDs (not int), required params marked, quality descriptions |
| Dead Code | No unwired flags, no uncalled functions, no ghost tables |

**Why two tiers?** The original scorecard tested syntax (does this string exist in the file?) not semantics (does this code actually work?). Generated CLIs scored Grade A and failed on the first real API call. The v2 scorecard catches that.

```bash
# Run the honest scorecard
printing-press scorecard --dir ./discord-cli --spec /tmp/discord-spec.json

# Run the mechanical dogfood validator
printing-press dogfood --dir ./discord-cli --spec /tmp/discord-spec.json
```

## Quick Start

### Install the Skill (Claude Code)

```bash
/install-skill https://github.com/mvanhorn/cli-printing-press
```

### Install the Compound Engineering Plugin

The printing press leverages agents from the [Compound Engineering plugin](https://github.com/EveryInc/compound-engineering-plugin) to improve generated CLI quality and agent readiness. Install it before running the press:

```bash
/plugin marketplace add EveryInc/compound-engineering-plugin
/plugin install compound-engineering
```

After installing both the skill and plugin, reload:

```bash
/reload-plugins
```

### Build the Binary

Build the binary (needed for scorecard, verify, and dogfood commands):

```bash
cd ~/cli-printing-press
go build -o ./printing-press ./cmd/printing-press
```

### Run It

```bash
/printing-press Discord                  # Full Opus run - CLI + MCP server
/printing-press Stripe codex             # Codex mode - 60% fewer Opus tokens
/printing-press --spec ./openapi.yaml    # From local spec file
```

Each run produces two binaries (`<api>-pp-cli` + `<api>-pp-mcp`), 8 analysis documents, and a Quality Score.

## Verification Tools

Four layers of mechanical validation - no vibes, no self-assessment.

```bash
# Quality Scorecard: two-tier scoring (infrastructure + domain correctness)
printing-press scorecard --dir ./my-cli --spec ./openapi.json

# Dogfood: catches dead flags, dead functions, auth mismatches, invalid paths
printing-press dogfood --dir ./my-cli --spec ./openapi.json
```

### Proof of Behavior (Phase 4.7)

The v1 scorecard checked string presence ("does sync.go exist?"). The Proof of Behavior checks data flow ("does sync.go actually call UpsertMessage on a table that search.go queries?").

Four behavioral proofs:
- **Path Proof**: Every URL in generated commands exists in the OpenAPI spec
- **Flag Proof**: Every registered flag is referenced in at least one command
- **Pipeline Proof**: Every SQLite table has a WRITE path (sync) and READ path (search/query)
- **Auth Proof**: Auth header format matches the spec's securitySchemes

If any proof fails, auto-remediation removes dead code and re-verifies. Hallucinated paths and auth mismatches are hard FAIL gates.

### Live API Testing (Phase 5.5)

When you provide an API key at the start, Phase 5.5 runs read-only tests against the real API:

```
LIVE API TEST RESULTS
=====================
Auth:     PASS (200 OK on doctor)
List:     3/3 passed (users, channels, guilds)
Get:      1/1 passed (user abc123)
Sync:     PASS (5 pages synced, 12 blocks)
Search:   PASS (3 results for "a")

Verdict:  PASS - CLI works against real API
```

Safety: GET only, --limit 1, 10s timeout, stops on 401. Never creates, posts, or deletes anything.

### Ship Loop (Phase 5.7)

"Is this shippable?" triggers a fix cycle: identify top 3 issues, fix them, re-score. Max 3 iterations. No more dead-end assessments.

## What's New in v2 (2026-03-27)

Synthesized from post-mortems on Notion and Linear runs. 14 changes to the skill.

**The problem:** v1 did excellent competitive research, then ignored it to chase scorecard numbers. Every CLI came out as a discrawl clone with cute names nobody asked for.

**The fix:** Three root problems addressed:

| Problem | v1 | v2 |
|---------|----|----|
| Scorecard-driven development | Priority 2: "raise the scorecard number" | Priority 4 with anti-gaming rules. Table stakes are Priority 1. |
| Same architecture for every API | discrawl clone: SQLite + 8 insight commands regardless of domain | Phase 0.6 Feature Parity Audit: build what competitors have FIRST |
| Names nobody asked for | "noto" (Notion), "lz" (Linear) | `<api>-pp-cli` by default. Discoverable, branded, no confusion. |

**New phases:** 0.6 (Feature Parity Audit), 5.9 (Offer Emboss)

**New Phase 4 priorities:** P0 Data Layer, P1 Table Stakes (NEW), P2 Workflows, P3 Command Name Normalization (NEW), P4 Scorecard with anti-gaming, P5 Tests (NEW), P6 Distribution (NEW), P7 Polish

**New validations:** Module path (2.0b), API version header (2.7), data pipeline smoke test (5.5g), 8 new anti-shortcut rules

**Emboss:** Now opt-in only. Offered at end of run, never triggered automatically.

## Credits

- **Peter Steinberger** ([@steipete](https://github.com/steipete)) - [discrawl](https://github.com/steipete/discrawl) and [gogcli](https://github.com/steipete/gogcli) set the bar. The quality scoring system is inspired by his work. discrawl v0.2.0's sync architecture directly influenced the printing press templates.
- **Trevin Chow** ([@trevin](https://x.com/trevin)) - [7 Principles for Agent-Friendly CLIs](https://x.com/trevin) shaped the agent-first template design.
- **Ramp** ([@tryramp](https://github.com/ramp-public/ramp-cli)) - Their agent-first CLI inspired auto-JSON piping, --no-input, and --compact output.
## License

MIT
