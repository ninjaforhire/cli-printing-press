# Tools Polish — MCP Tool Quality Playbook

**Your goal:** ensure every MCP tool exposed by this CLI carries the metadata an agent needs to use it correctly. Tool descriptions and classifications are how agents discover and decide whether to call a tool — thin descriptions and missing annotations directly degrade agent UX.

This playbook has two passes. Both run on every CLI; do not skip either.

## Pass 1: Address mechanical findings

Run the audit:

```bash
printing-press tools-audit <cli-dir> --json
```

The audit emits findings of three kinds. Address each:

### `empty-short`

A `cobra.Command{}` with no `Short:` field. The runtime walker falls back to `"Run \`<cmd>\` through the companion CLI binary."` — a meaningless description for agents.

**Fix:** write a verb-led, action-specific Short. Read the command's `RunE` body and `Flags()` block to ground the description in actual behavior. See **Description criteria** below.

### `thin-short`

Short text under 30 characters AND fewer than 4 words. Examples from real CLIs: `"Advertiser Search"`, `"Search Ads"`, `"Subreddit Posts"`, `"Product Reviews"`.

**This is a suspect, not a verdict.** Some short Shorts are accurate (`"Print version"`, `"Check CLI health"`). Decide per finding:

- If the Short is a fragment that doesn't tell an agent what action runs, what parameters it takes, or what comes back → rewrite per **Description criteria**.
- If the Short is brief but precise → leave it; document the decision implicitly by skipping the rewrite.

### `missing-read-only`

Command name matches a read-shaped pattern (`list`, `get`, `search`, `show`, `view`, `find`, `describe`, `context`, `sql`, `stats`, `trending`, `trust`, `health`, `stale`, `orphans`, `reconcile`, `doctor`, `version`, `analytics`) but no `mcp:read-only` annotation, AND no `pp:endpoint` (so the runtime walker registers it as a shell-out tool).

**Fix:** add `Annotations: map[string]string{"mcp:read-only": "true"},` to the command literal. The walker then emits `readOnlyHint: true` so MCP hosts skip the per-call permission prompt.

**Don't blindly accept.** If the command name matches the heuristic but the body actually mutates state (writes to the local store, opens a browser, sends a notification, modifies config), do NOT add the annotation. The heuristic is a starting point; the body is the truth.

## Pass 2: Evaluate every command (audit-independent)

Run a judgment pass over every user-facing command in `internal/cli/`, including ones the audit didn't flag. Mechanical detection misses two real classes:

1. **Read-only commands whose names don't match the heuristic.** Platform-named shortcuts (`tiktok`, `instagram`, `google`), one-word reads (`search`, `analytics`, `export`), commands renamed away from their `list-*`/`get-*` form (`facebook_list-post-3` → `post-transcript`). All of these are real reads that need `mcp:read-only` but won't trip the audit.
2. **Descriptions that pass the length check but are still poor for agents.** A 50-character Short that says `"Manage your saved cookbook (list, search, tag, match)"` is category-shaped, not action-shaped — fine for human help text, weak for an agent deciding whether to call it.

For each command:

- Read its `RunE` body. Does it call an HTTP method (`client.Get`, `resolveRead`, `c.Post`, …)? Does it write to the local store, the filesystem, or config?
  - **HTTP GET only, no local writes** → mark `mcp:read-only` (even if the audit didn't flag).
  - **Any local write or external mutation** → do NOT mark; the default destructive hint is correct.
  - **Side effect** (browser open, notification, audio) → do NOT mark; agents should be prompted.
- Read its `Short:` and `Long:` against the criteria below. If thin or category-shaped, rewrite.

## Description criteria

Agent-grade Shorts share four properties:

1. **Verb-led.** Open with the action: `"Search ..."`, `"Fetch ..."`, `"List ..."`, `"Estimate ..."`, `"Save ..."`. Not `"Cookbook commands"` or `"Manage ..."`.
2. **Action-specific, not category-shaped.** `"Search the LinkedIn Ad Library by company, keyword, country, and date range"` — not `"Search Ads"`. The reader should know the *one specific operation* this command performs.
3. **Parameter-aware.** Name the meaningful filters/options inline so the agent knows what to pass: `"... by max cook time, recency, and dietary tags"`, not just `"Pick dinner"`.
4. **Scope-explicit.** When the command operates on a subset, say so: `"Search recipe titles across curated sites without fetching the full recipes (returns metadata only)"` — tells the agent the cost/output trade-off vs. a richer search.

### Anti-patterns to remove

- **Dev-state leakage.** `"(wip)"`, `"(planned)"`, `"(ranking integration wip)"`, `"(coming soon)"` — useless to agents, leaks dev backlog. Strip during polish.
- **Self-referential Long.** `Long: "Shortcut for 'feed get-on-this-day'. Events on this day"` — repeats the Short and tells the reader nothing new. Rewrite Long to add genuine context (parameters, output shape, edge cases) or delete it.
- **Bare verb fragments.** `"Show cooking history"`, `"List saved recipes"`, `"Print version"` — fine for humans browsing `--help`, weak for agents deciding which of many similar tools to invoke. Add the qualifier (`"... most recent first, with rating and notes"`, `"... optionally filtered by tag, site, or author"`).
- **Empty Long with non-empty Short.** Long should add genuine context or be omitted. A Long that just restates Short adds noise to the MCP tool catalog.

## Worked examples

Pre/post pairs from real polish passes. The "before" lines are what we found in the wild; the "after" lines were the rewrites that landed.

### recipe-goat (from PR #154 polish commit)

```
Before: "Lightweight cross-site recipe search (metadata only, no fetch)"
After:  "Search recipe titles across curated sites without fetching the full recipes (returns metadata only)"
```

```
Before: "Show cooking history"
After:  "List past cooking sessions with ratings and notes, most recent first"
```

```
Before: "View and override site trust scores (ranking integration wip)"
After:  "View and override per-site trust scores used by the cross-site ranker"
```

```
Before: "Persist a site trust override (ranking integration wip)"
After:  "Save a local per-site trust adjustment that the ranker applies on top of the built-in scores"
```

### scrape-creators (from PR #113 polish commit)

```
Before: "Advertiser Search"
After:  "Search the Google Ads Transparency Center for advertisers by name or domain"
```

```
Before: "Search Ads"
After:  "Search the LinkedIn Ad Library by company, keyword, country, and date range"
```

```
Before: "Subreddit Posts"
After:  "Fetch posts from a subreddit, with sort (hot/new/top) and timeframe filters"
```

```
Before: "Product Reviews"
After:  "Fetch product reviews from a TikTok Shop product page (by URL or product ID)"
```

## Per-finding response templates

When the audit returns a finding, follow this minimum response.

### Empty Short

```go
// Add the Short field; choose verb-led action description.
Short:       "<verb> <object> <key qualifier>",
Annotations: map[string]string{"mcp:read-only": "true"}, // only if read
```

### Thin Short

```go
// Replace the existing Short with a parameter-aware rewrite.
// Read RunE body and Flags() block to ground the description in real behavior.
Short: "<verb> <specific scope> by <main filter>, <secondary filter>",
```

### Missing read-only

```go
// Add or extend the Annotations map. Preserve any existing keys.
Annotations: map[string]string{"mcp:read-only": "true"},
```

The audit already exempts commands carrying `pp:endpoint` (those get typed-tool registration with method-derived classification), so this finding never fires on endpoint mirrors.

## Ledger and resumability

`tools-audit` writes `<cli-dir>/.printing-press-tools-polish.json` after every run. It contains the timestamp, cli-dir, and one entry per finding. The ledger serves three purposes:

1. **Delta computation.** On a second run within 24 hours, the audit prints `since last run: N resolved, M new` so you can see your progress without re-counting. Stale ledgers (>24h) are deleted automatically.
2. **Resumability.** If your context window flushes mid-polish, re-run `tools-audit <cli-dir>`. Findings you've fixed have disappeared from the new scan; findings you accepted are still recorded with status. You pick up where you left off.
3. **Audit trail of accept decisions.** When you decide a `thin-short` is fine as-is, you mark the entry `accepted` and write a one-sentence rationale. The next run filters it out of the pending table.

### Marking a finding accepted

When the table shows a `thin-short` whose Short is brief but precise (`"Print version"`, `"Check CLI health"`, `"Show authentication status"`), edit the ledger entry directly:

```json
{
  "kind": "thin-short",
  "command": "version",
  "file": "version.go",
  "line": 12,
  "evidence": "Print version",
  "status": "accepted",
  "note": "Standard 'version' command; agents understand this without elaboration"
}
```

After marking, re-run `tools-audit <cli-dir>`. The pending table will exclude this finding. The accepted count in the header (`(1 accepted)`) confirms the decision persisted.

**Don't accept `empty-short` or `missing-read-only` findings.** They have no judgment call to make: empty Shorts always need text, and a read command always wants the annotation. Acceptance is reserved for `thin-short`.

### Don't manually mark findings as fixed

If you fixed a finding via code change, do nothing in the ledger. The next audit re-scans the source; finding gone from the AST → finding gone from the table → delta line shows `1 resolved`. Never set `status: "fixed"` by hand — the tool detects this automatically and your manual marking will read as "still flagged but accepted-without-rationale."

## Verification checklist

After applying fixes, before declaring the polish complete:

- [ ] `go build ./...` clean (annotations don't break compilation)
- [ ] `printing-press tools-audit <cli-dir>` shows zero pending findings — every finding is either fixed (auto-removed) or explicitly accepted with a `note`
- [ ] `printing-press dogfood --dir <cli-dir>` reports `MCP Surface: PASS`
- [ ] If commands were renamed or had their annotations restructured, smoke-test the binary by inspecting `--help` output for the affected commands

The ledger file persists until it ages out (24h). Once the polish PR merges and the CLI is rebuilt, the file is no longer load-bearing — the next `tools-audit` run can start fresh.

If you're polishing a CLI inside a clone of the public library repo (not the internal `~/printing-press/library/`), add `.printing-press-tools-polish.json` to that repo's root `.gitignore` before committing — the ledger is local working state, not part of the published CLI.
