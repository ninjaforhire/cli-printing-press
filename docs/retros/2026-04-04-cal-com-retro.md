# Printing Press Retro: Cal.com

## Session Stats
- API: Cal.com (scheduling / calendar management)
- Spec source: OpenAPI 3.0.0 from GitHub (calcom/cal.com/docs/api-reference/v2/openapi.json)
- Scorecard: 92/100 (Grade A)
- Verify pass rate: 89% (25/28 after polish)
- Fix loops: 1 shipcheck + 1 polish
- Manual code edits: 7 (description rewrite, CAL_COM_API_KEY env var, cal-api-version header, duplicate auth removal, promoted bookings subcommand wiring, dead code removal, verify failure fixes)
- Features built from scratch: 8 (sync, health, stale, no-show, today, busy, sql, enhanced bookings store)

## Findings

### 1. SQL reserved words in generated table names (Bug)
- **What happened:** Generator created tables named `default`, `check`, `references` with column names `from` and `to`. All are SQL reserved words. SQLite migration failed with "syntax error" on first `store.Open()`.
- **Root cause:** `schema_builder.go:315` `toSnakeCase()` converts resource names to table names but never checks against SQL reserved words. No quoting is applied.
- **Cross-API check:** Any API with resources named default, check, references, order, group, select, key, from, to, etc. will hit this. Cal.com has 5 reserved words in one spec. Stripe has "refund" (safe) but could have "default" for default payment methods. GitHub has "check" (check runs/suites).
- **Frequency:** most APIs — common resource names collide with SQL keywords
- **Fallback if machine doesn't fix it:** Claude must manually find and fix every reserved word in every generated store migration. This is error-prone (took 4 fix cycles in this session because reserved words surfaced one at a time). Claude sometimes catches it, sometimes doesn't — each migration error surfaces one keyword, requiring iterative fixes.
- **Worth a machine fix?** Yes. This broke the CLI at runtime and took multiple fix cycles.
- **Inherent or fixable:** Fixable. Add a reserved word check to `toSnakeCase()` or a post-processing step.
- **Durable fix:** In `schema_builder.go`, add a `sqlSafeIdentifier()` function that wraps table/column names in double-quotes if they match a known set of SQL reserved words. Apply it in `buildResourceTable()` and `buildSubResourceTable()` for both table names and column names. SQLite accepts double-quoted identifiers universally, so quoting all names is also safe (simpler but slightly less readable SQL).
- **Test:** Generate a CLI from a spec with resources named "default", "check", "references", "order". Verify `store.Open()` succeeds and all tables are queryable. Negative test: a CLI for Stripe (no reserved words) still generates unquoted tables.
- **Evidence:** 4 sequential migration failures during this session, each revealing a different reserved word.

### 2. FTS triggers reference non-existent table columns (Bug)
- **What happened:** Generator emitted FTS5 triggers like `INSERT INTO event_types_fts(...) VALUES (new.description, new.title)` but the `event_types` table only has `id, data, synced_at`. `description` and `title` are inside the JSON `data` column, not extracted as real columns. Trigger creation fails.
- **Root cause:** `store.go.tmpl:90-111` generates FTS triggers using `FTS5Fields` from the schema builder. The schema builder picks searchable fields from the spec's response schema. But the entity mapper only extracts a subset of fields as real columns — the FTS fields may not be among them. The trigger template assumes all FTS fields are real columns.
- **Cross-API check:** Any API where the high-searchable fields aren't among the fields extracted as real table columns. This is common — the entity mapper extracts IDs, foreign keys, and temporal fields, but FTS typically needs title/name/description which may not be extracted.
- **Frequency:** most APIs — FTS fields and extracted columns are chosen by different criteria
- **Fallback if machine doesn't fix it:** Claude must remove or rewrite broken FTS triggers. Medium reliability — Claude catches it when migration fails, but may not proactively check.
- **Worth a machine fix?** Yes. Broken migrations prevent the CLI from working.
- **Inherent or fixable:** Fixable. Either extract FTS fields as real columns, or generate standalone FTS tables (not content-sync) that are populated during sync.
- **Durable fix:** In `schema_builder.go`, when setting `FTS5Fields`, check each field against the table's actual columns. If a field isn't a real column, either: (a) add it as an extracted column, or (b) switch to a standalone FTS table (without `content=` and triggers) and populate it during upsert. Option (b) is more robust because it doesn't force column extraction just for search.
- **Test:** Generate a CLI where event_types has FTS on `title` and `description` but neither is an extracted column. Verify migration succeeds and search returns results after sync.
- **Evidence:** `CREATE TRIGGER IF NOT EXISTS event_types_ai` failed because `new.description` doesn't exist.

### 3. Promoted commands don't include mutation subcommands (Template gap)
- **What happened:** The promoted `bookings` command is a list-only shortcut. `bookings cancel`, `bookings confirm`, etc. are not accessible — they exist only in the hidden raw command group. Users must know the raw operationId-derived name to access mutations.
- **Root cause:** `command_promoted.go.tmpl` generates a single RunE function for the default list operation. It doesn't `AddCommand()` the sibling mutation commands from the raw command group. The raw command is marked `Hidden: true` because the promoted version replaces it.
- **Cross-API check:** Every API with promoted commands and mutation subcommands. Stripe, GitHub, Linear, Notion — all have list as the default but cancel/update/delete as subcommands.
- **Frequency:** every API
- **Fallback if machine doesn't fix it:** Claude must manually add `cmd.AddCommand(...)` for every mutation subcommand. Medium reliability — Claude catches it when the user asks, but won't proactively wire subcommands.
- **Worth a machine fix?** Yes. This affects UX for every generated CLI.
- **Inherent or fixable:** Fixable. The template can iterate over sibling endpoints in the same resource and add them as subcommands.
- **Durable fix:** In `command_promoted.go.tmpl`, after defining the promoted command and before `return cmd`, add a loop that registers subcommands from the raw command group. The generator already knows which endpoints belong to each resource — emit `cmd.AddCommand(new{{camel .SubcommandName}}Cmd(flags))` for each non-default endpoint.
- **Test:** Generate a CLI for an API with bookings (list, create, cancel, update). Verify `bookings cancel --help` works.
- **Evidence:** `bookings cancel --help` showed the list help, not the cancel subcommand.

### 4. Verify misclassifies data-layer commands as "read" (Scorer bug)
- **What happened:** `stale`, `tail`, and `api` failed verify because they're classified as "read" but don't behave like read commands. `stale` reads from SQLite (data-layer). `tail` is a long-running poller. `api` is a local discovery command.
- **Scorer correct?** No. The scorer's `classifyCommandKind()` in `internal/pipeline/runtime.go:472-496` has a hardcoded list of data-layer command names (`sync, search, sql, health, trends, patterns, analytics, export, import`). New transcendence commands like `stale`, `no-show`, `today`, `busy` aren't in the list. `api` and `tail` are also misclassified.
- **Root cause:** `runtime.go:477` uses a static `switch` statement. Any new command name not in the list defaults to "read" via the fallthrough at line 494.
- **Cross-API check:** Every CLI that adds transcendence commands beyond the hardcoded list.
- **Frequency:** every API — the transcendence command set grows with each generation
- **Durable fix:** Two approaches:
  1. Add `stale`, `no-show`, `today`, `busy`, `diff` to the data-layer list and `api` to the local list. Quick but fragile — breaks again when new commands are added.
  2. Have the generator emit a `command_kinds.json` manifest listing each command and its kind. The verify tool reads this instead of guessing. More durable — new commands self-classify.
- **Test:** Generate cal-com-pp-cli, run verify. `stale`, `no-show`, `today`, `busy` should be data-layer. `api` should be local. `tail` should be local or data-layer.
- **Evidence:** Verify output showed `stale: read PASS FAIL FAIL 1/3` despite `stale --dry-run` returning exit 0 locally.

### 5. Missing API-specific required headers (Template gap)
- **What happened:** Cal.com API v2 requires `cal-api-version: 2024-08-13` on every request. Without it, endpoints return 404. The generator didn't emit this header.
- **Root cause:** `client.go.tmpl:343-346` only sets `Authorization` and `User-Agent` headers. There's no mechanism to detect or emit API-specific required headers from the spec's parameter definitions or extensions.
- **Cross-API check:** Some APIs require version headers (cal-api-version, X-API-Version, Stripe-Version). Anthropic requires `anthropic-version`. This is a recognizable pattern.
- **Frequency:** API subclass — APIs with explicit version headers. ~10-20% of APIs.
- **Fallback if machine doesn't fix it:** Claude must read the spec or docs, notice the required header, and manually add it to client.go. Medium reliability — easy to miss.
- **Worth a machine fix?** Yes. Missing a required header breaks all API calls silently (404s).
- **Inherent or fixable:** Fixable. The spec declares required parameters with `in: header`. The generator can detect these and emit them.
- **Durable fix:** In the OpenAPI parser, detect parameters with `in: header` and `required: true` at the path or operation level. If a required header appears on >80% of operations (like a version header), emit it in the client template as a default header. Store it in the generator's `APIConfig` struct so the client template can iterate: `{{range .RequiredHeaders}}req.Header.Set("{{.Name}}", "{{.Default}}"){{end}}`.
  - **Condition:** Spec has required header parameters on most operations
  - **Guard:** Skip headers that are per-operation (like Content-Type)
  - **Frequency estimate:** ~10-20% of APIs use version headers
- **Test:** Generate from Cal.com spec. Verify client.go sets `cal-api-version`. Negative: Generate from Stripe spec (no required version header) — client.go should not have extra headers.
- **Evidence:** All Cal.com API calls returned 404 until `cal-api-version: 2024-08-13` was manually added.

### 6. Duplicate auth command registration (Bug)
- **What happened:** root.go registered `newAuthCmd(&flags)` twice — once from the resource loop (line 102 in template) and once from the hardcoded auth registration (line 106).
- **Root cause:** `root.go.tmpl:106` always registers `newAuthCmd`. But if the API spec has a resource named "auth" (Cal.com has `/v2/auth/*` endpoints), the resource loop at line 100-102 also registers it. The template doesn't check for overlap.
- **Cross-API check:** Any API with auth-related endpoints in the spec (/auth, /oauth, /token). Cal.com, Discord, any OAuth provider.
- **Frequency:** API subclass — APIs with auth endpoints in the spec. ~30-40%.
- **Fallback if machine doesn't fix it:** Claude notices the duplicate in `--help` output and removes one. High reliability, but annoying.
- **Worth a machine fix?** Yes. Simple guard.
- **Inherent or fixable:** Fixable. One-line conditional.
- **Durable fix:** In `root.go.tmpl`, wrap line 106 in `{{if not (index .Resources "auth")}}...{{end}}`. If auth is already a resource, it's registered by the loop and doesn't need a second registration.
- **Test:** Generate from Cal.com spec (has auth resource). Verify `--help` shows one `auth` entry. Negative: Generate from a spec without auth resource — auth still registered.
- **Evidence:** `--help` showed `auth` listed twice.

### 7. Config template doesn't emit env var auth when spec lacks security section (Assumption mismatch)
- **What happened:** Config.go had no `CAL_COM_API_KEY` env var support. The spec's security section was empty, so the generator didn't know about Bearer auth. But Cal.com documents Bearer auth in prose and all SDKs/MCPs use it.
- **Root cause:** `config.go.tmpl:62-67` iterates over `.Auth.EnvVars` which comes from the spec's `securityDefinitions`/`security`. Cal.com's OpenAPI spec has no `security` section — auth is described only in the `info.description` text and in parameter descriptions.
- **Cross-API check:** APIs with incomplete or missing security definitions in their spec. Common for community-maintained specs, reverse-engineered specs, and quick-and-dirty OpenAPI files.
- **Frequency:** API subclass — specs without formal security sections. ~20-30%.
- **Fallback if machine doesn't fix it:** Claude must manually add env var support. The skill already instructs this ("Compensate for missing auth" in Phase 2), so Claude catches it. Medium-high reliability but adds ~5 min of manual work.
- **Worth a machine fix?** Yes, but needs careful design. The generator shouldn't invent auth patterns — but it could have a fallback: if no security section is found, check the spec's description and parameter descriptions for "Bearer", "API key", "Authorization" mentions, and infer a default env var.
- **Inherent or fixable:** Partially fixable. A heuristic that detects auth mentions in description text would catch ~80% of cases. The remaining 20% need manual intervention.
- **Durable fix:** In the OpenAPI parser, after parsing `security`/`securityDefinitions`, if no auth is found, scan `info.description` for patterns like "Bearer", "API key", "Authorization header", "cal_live_", "sk_live_". If found, set a default `Auth.Type = "api_key"` with env var derived from the API name (`<API_NAME>_API_KEY`). Mark it as `inferred` so the config template can add a comment.
  - **Condition:** No security section in spec AND description mentions auth keywords
  - **Guard:** Skip if security section exists (explicit > inferred)
- **Test:** Generate from Cal.com spec (no security section, description mentions Bearer). Verify config.go has `CAL_COM_API_KEY` env var. Negative: Generate from Stripe spec (has security section) — env var comes from spec, not inference.
- **Evidence:** Manual addition of `CAL_COM_API_KEY` in config.go.

## Prioritized Improvements

### Fix the Scorer
| # | Scorer | Bug | Impact | Fix target |
|---|--------|-----|--------|------------|
| 4 | verify `classifyCommandKind()` | Hardcoded data-layer list misses new transcendence commands | 3 false failures (stale, tail, api) per CLI = -9% verify rate | `internal/pipeline/runtime.go:472-496` |

### Do Now
| # | Fix | Component | Frequency | Fallback Reliability | Complexity | Guards |
|---|-----|-----------|-----------|---------------------|------------|--------|
| 1 | Quote SQL reserved words in table/column names | `internal/generator/schema_builder.go` | most APIs | Low (iterative fix cycles) | small | None needed — quoting is always safe |
| 6 | Deduplicate auth command registration | `internal/generator/templates/root.go.tmpl` | 30-40% of APIs | High but annoying | small | Check if "auth" is in Resources |
| 3 | Wire mutation subcommands into promoted commands | `internal/generator/templates/command_promoted.go.tmpl` | every API | Medium | medium | Only add subcommands from same resource |

### Do Next (needs design)
| # | Fix | Component | Frequency | Fallback Reliability | Complexity | Guards |
|---|-----|-----------|-----------|---------------------|------------|--------|
| 2 | Fix FTS triggers for non-extracted columns | `schema_builder.go` + `store.go.tmpl` | most APIs | Medium | medium | Only when FTS fields aren't extracted columns |
| 5 | Detect and emit required API headers | OpenAPI parser + `client.go.tmpl` | 10-20% | Medium | medium | Only for headers on >80% of operations |
| 7 | Infer auth from spec description when security section missing | OpenAPI parser + `config.go.tmpl` | 20-30% | Medium-high | medium | Only when no security section exists |

### Skip
| # | Fix | Why unlikely to recur |
|---|-----|----------------------|
| (none) | | All findings apply across multiple APIs |

## Work Units

### WU-1: SQL identifier safety (finding #1)
- **Goal:** Generated table and column names never collide with SQL reserved words
- **Target files:**
  - `internal/generator/schema_builder.go` — add `sqlSafeIdentifier()`, apply in `buildResourceTable()` and `buildSubResourceTable()`
  - `internal/generator/templates/store.go.tmpl` — use safe identifiers in CREATE TABLE, INSERT, and SELECT templates
- **Acceptance criteria:**
  - Generate from Cal.com spec (has `default`, `check`, `references`, `from`, `to`) → migration succeeds, all tables queryable
  - Generate from Stripe spec (no reserved words) → tables still unquoted (no unnecessary quoting)
  - Generate from a synthetic spec with resource named `order` → table is `"order"` or `order_items`
- **Scope boundary:** Does NOT rename existing tables in published CLIs
- **Complexity:** small

### WU-2: Promoted command subcommand wiring + auth dedup (findings #3, #6)
- **Goal:** Promoted commands expose mutation subcommands; auth is never registered twice
- **Target files:**
  - `internal/generator/templates/command_promoted.go.tmpl` — add `AddCommand()` for sibling endpoints
  - `internal/generator/templates/root.go.tmpl` — guard auth registration
  - `internal/generator/promoted.go` (or wherever promotion logic lives) — pass sibling info to template
- **Acceptance criteria:**
  - Generate from Cal.com spec → `bookings cancel --help` works
  - Generate from Cal.com spec → `--help` shows one `auth` entry
  - Generate from Stripe spec → promoted `charges` includes `refund` subcommand
- **Scope boundary:** Does NOT rename operationId-derived subcommand names (that's a separate cosmetic fix)
- **Complexity:** medium

### WU-3: FTS trigger safety (finding #2)
- **Goal:** FTS triggers only reference columns that exist on the table
- **Target files:**
  - `internal/generator/schema_builder.go` — validate FTS fields against extracted columns
  - `internal/generator/templates/store.go.tmpl` — support standalone FTS (without content-sync triggers) as fallback
- **Acceptance criteria:**
  - Generate from Cal.com spec → event_types FTS works (either with extracted columns or standalone table)
  - FTS search returns results after sync
  - Negative: Generate from spec where FTS fields ARE extracted columns → content-sync triggers still used (more efficient)
- **Scope boundary:** Does NOT change the FTS field selection heuristic — only the trigger generation
- **Complexity:** medium

### WU-4: Verify command classification (finding #4)
- **Goal:** Verify correctly classifies transcendence commands as data-layer and discovery commands as local
- **Target files:**
  - `internal/pipeline/runtime.go:472-496` — expand classification lists or implement manifest-based classification
- **Acceptance criteria:**
  - Run verify on cal-com-pp-cli → stale, no-show, today, busy classified as data-layer, api as local
  - Run verify on a CLI without transcendence → no regression in existing classification
- **Scope boundary:** Quick fix (expand lists) now; manifest approach can be a follow-up
- **Complexity:** small

## Anti-patterns

- **Iterative reserved-word discovery**: Each migration failure surfaced one reserved word at a time. Fix all known reserved words in one pass — don't fix-build-fail-fix.
- **Testing sync without cleaning the DB**: Leftover DB from a prior run can mask or cause migration errors. Always clean before testing migration changes.

## What the Machine Got Right

- **Quality gates**: All 7 gates passed on first generation — no build or compilation failures from the generator itself.
- **API coverage breadth**: 181 paths → ~250 command files with correct routing. The OpenAPI parser handled this large spec well.
- **Data layer scaffolding**: The store template generated a working SQLite schema with FTS, sync_state, and per-resource tables. The foundation was solid — transcendence commands built on top of it.
- **Promoted command pattern**: The shortcut-for-list pattern works well for UX. The gap is subcommand wiring, not the pattern itself.
- **Dogfood accuracy**: Dead flags, dead functions, path validity, and example coverage were all accurately measured. Dogfood is trustworthy.
- **Score composition**: 92/100 on first shipcheck is excellent for a fresh generation. The machine produces Grade A CLIs from well-structured specs.
