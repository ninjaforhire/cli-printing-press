---
title: "feat: Add publish skill to ship CLIs to printing-press-library"
type: feat
status: active
date: 2026-03-29
origin: docs/brainstorms/2026-03-29-publish-skill-requirements.md
---

# feat: Add publish skill to ship CLIs to printing-press-library

## Overview

Add a `/printing-press publish` skill that packages a generated CLI from the local library, validates it, and opens a PR against the printing-press-library repo — all from a single command. This also requires expanding the catalog taxonomy to 14 categories, adding a `category` field to the CLI manifest, adding new CLI subcommands (`library list`, `publish validate`, `publish package`), and scaffolding the printing-press-library repo.

## Problem Frame

Users generate CLIs with the printing press and they land in `~/printing-press/library/`. There's no streamlined way to contribute a finished CLI to the shared printing-press-library repo. Today this would require manually cloning the repo, copying files, organizing them, and creating a PR. The publish skill makes this a single command. (see origin: docs/brainstorms/2026-03-29-publish-skill-requirements.md)

## Requirements Trace

- R1-R2. 14-category single-level taxonomy with `other` catch-all
- R3. Catalog `validCategories` updated; all 17 YAML files reassigned
- R4. `category` field added to `.printing-press.json` manifest
- R4a. `description` field added to `.printing-press.json` manifest (one-liner, canonical source)
- R4b. [Dependency, not in this plan's scope] CLI README generation at print time (11-section structure: title + one-liner, API links, why-it-exists, install, quick start, commands, agent & automation, exit codes, config, troubleshooting, attribution)
- R5-R8. Library repo structure: `library/<category>/<cli-name>/` with `.manuscripts/`, `registry.json`, `.gitignore`
- R9-R12. CLI binary owns validation, packaging, library listing; skill owns interaction + git
- R13a-R13e. Managed clone at `~/printing-press/.publish-repo/` with cached config
- R14-R19. Name resolution: exact → suffix → glob → ask user
- R20-R22. Category assignment: manifest → catalog lookup → ask user
- R23-R24. Publish validation via CLI binary with structured JSON
- R25-R26. Package assembly via CLI binary with manuscript resolution
- R27-R30. PR creation: branch naming, conventional commits, `gh pr create`
- R31-R33. Error handling: `gh` auth check, repo reachability, interrupted state
- R34. Registry.json updated during each publish

## Scope Boundaries

- Does NOT run `printing-press verify` or `scorecard` — checks build + manifest + manuscripts only
- Does NOT modify the user's local library — read-only
- Does NOT handle re-publish / version bump — future capability
- CLI README generation (R4b) is out of scope — it's a cross-cutting change to the printing-press skill and generator. The publish skill assumes a README already exists and flags its absence in the PR description if missing
- Trust verification is NOT in scope — future CI work in the library repo (see origin: Future: Library Trust Verification)

## Context & Research

### Relevant Code and Patterns

- **Cobra command pattern:** Each command is a `newXxxCmd()` function in its own file under `internal/cli/`. Parent commands with subcommands follow `internal/cli/catalog.go` pattern (`newCatalogCmd()` adds `list/show/search` children)
- **Flag conventions:** `cmd.Flags().StringVar()` (not `cmd.Flags().String()`), `--json` for machine output, `--dir` for directory input. Human text to stderr, JSON to stdout
- **Exit codes:** `ExitError` with typed codes from `internal/cli/exitcodes.go`
- **Manifest:** `CLIManifest` in `internal/pipeline/climanifest.go`. Written by `WriteCLIManifest()` with `json.MarshalIndent`. Optional fields use `omitempty`
- **Catalog validation:** `validCategories` map in `internal/catalog/catalog.go`. Error message on line 138 hardcodes the list. `ParseEntry` validates on read — categories and YAML files must change atomically
- **Path helpers:** `internal/pipeline/paths.go` — `PressHome()`, `PublishedLibraryRoot()`, `PublishedManuscriptsRoot()`, `ArchivedManuscriptDir()`. Reuse these, don't re-derive
- **Naming:** `internal/naming/naming.go` — `CLI()`, `TrimCLISuffix()`, `IsCLIDirName()`
- **File copying:** `CopyDir()` in `internal/pipeline/publish.go` — recursive with symlink support
- **Skill structure:** YAML frontmatter (`name`, `description`, `version`, `min-binary-version`, `allowed-tools`) + markdown body. Setup contract between `PRESS_SETUP_CONTRACT_START/END` markers, validated by `contracts_test.go`
- **Test pattern:** Table-driven with `testify/assert`. Use `setPressTestEnv(t)` for isolation. CLI commands tested via `newXxxCmd()` + `cmd.SetArgs()`

### Institutional Learnings

- Catalog categories and YAML entries must be updated atomically — `ParseEntry` validates on read (docs/solutions/best-practices/checkout-scoped-printing-press-output-layout)
- `example` category stays in `validCategories` for petstore.yaml test fixture but isn't part of the public 14-category taxonomy
- Skills invoke `printing-press` on PATH, never `./printing-press`. New skill must follow this and pass `contracts_test.go`
- Adding `category` to CLIManifest with `omitempty` doesn't require a schema version bump — it's additive
- Library listing should handle both `-pp-cli` and `-pp-cli-N` suffixed directories (claimed reruns)
- Decoupling plan explicitly deferred the publish skill as "a separate future effort" — this plan implements it

## Key Technical Decisions

- **`example` retained in `validCategories`:** Petstore test fixture needs it. The publish skill and library repo don't offer it as a choice. Adding it to the map alongside the 14 public categories is simpler than special-case handling. (see origin: R3 discussion of `example`)
- **`publish` as parent command with `validate`/`package` children:** Follows the `catalog` parent pattern. Keeps `printing-press publish validate --dir ... --json` and `printing-press publish package --dir ... --json` as distinct subcommands
- **`library` as parent command with `list` child:** Leaves room for future `library show`, `library search`, etc. Follows `catalog` convention
- **registry.json schema:** `{ "schema_version": 1, "entries": [{ "cli_name", "api_name", "category", "description", "printing_press_version", "published_date" }] }`. Description sourced from catalog entry (if `catalog_entry` present) or first line of README. The skill writes this, not the CLI binary (see origin: R34, R10)
- **Manuscripts preserve run-ID structure:** `.manuscripts/<run-id>/research/`, `.manuscripts/<run-id>/proofs/`. Preserves provenance trail and avoids ambiguity across runs. Most recent run selected by lexicographic sort on directory name (see origin: R25)
- **Target repo URL in `.publish-config.json`:** Alongside cached access level and protocol. Single file for all publish state at `~/printing-press/.publish-config.json` (see origin: R13d, R13e)
- **New exit code for publish validation failures:** Add `ExitPublishError = 5` to `exitcodes.go` for publish-specific failures distinct from spec/generation errors
- **`description` as canonical one-liner in manifest:** Generated after shipcheck/emboss when the CLI is finalized — not during research, because the CLI's actual features may diverge from the brief. The manifest stores it, the README uses it, the PR description pulls from it. Single source of truth for "what does this CLI do." (see origin: R4a)
- **Best-in-class PR description:** The PR body is the first impression for any reviewer. It includes the one-liner, API details, README excerpt, manuscript links (within the PR branch), validation results table, and an explicit Gaps section for missing manifest fields. Designed so a reviewer with zero prior context can evaluate the contribution (see origin: R29)
- **CLI README is a dependency, not in scope:** The publish skill assumes a README exists at `<cli-dir>/README.md`. If missing, the PR description notes the gap. Generating READMEs at print time is a cross-cutting change to the printing-press skill tracked separately. The README has an 11-section structure defined in the origin doc (R4b) including API links, agent/automation properties, exit codes, and a one-line attribution to the printing press (see origin: R4b)

## Open Questions

### Resolved During Planning

- **registry.json schema:** Fields are `cli_name`, `api_name`, `category`, `description`, `printing_press_version`, `published_date`. Description from catalog entry or README first paragraph
- **Manuscripts structure:** Preserve run-ID directory structure inside `.manuscripts/`
- **Target repo URL config:** In `~/printing-press/.publish-config.json` alongside other publish state
- **`example` category:** Keep in `validCategories` for test fixture, exclude from public taxonomy

### Deferred to Implementation

- **`gh repo fork` behavior with existing fork:** Need to test `gh repo fork --clone` when fork already exists. If it errors, the skill should detect the existing fork and clone it instead
- **Scripts escape hatch:** If the skill's git/GitHub inline bash grows beyond ~30 lines, consider extracting to `skills/printing-press-publish/scripts/`. Assess during implementation
- **library list output format:** Exact JSON field names and whether to include manifest parse errors per-entry or filter silently. Decide during implementation based on what the skill needs

## High-Level Technical Design

> *This illustrates the intended approach and is directional guidance for review, not implementation specification. The implementing agent should treat it as context, not code to reproduce.*

```
PUBLISH FLOW — CLI BINARY + SKILL COOPERATION

Skill invocation:
  /printing-press publish [name]

Step 1 (Skill): Name Resolution
  printing-press library list --json
  → JSON array of { cli_name, dir, api_name, category, modified }
  → Skill applies: exact → suffix(-pp-cli) → glob(*name*, max 5 by recency) → ask user

Step 2 (Skill): Category Assignment
  Read .printing-press.json from resolved CLI dir
  → has category? → confirm with user
  → has catalog_entry? → printing-press catalog show <entry> --json → get category → confirm
  → neither? → present 14 categories → ask user

Step 3 (CLI): Validate
  printing-press publish validate --dir <cli-dir> --json
  → { "passed": bool, "checks": [{ "name": "manifest", "passed": bool, "error": "..." }, ...] }
  → Skill shows results, stops if failed

Step 4 (CLI): Package
  printing-press publish package --dir <cli-dir> --category <cat> --target <staging> --json
  → Copies CLI + .manuscripts/<run-id>/ to staging/library/<cat>/<cli-name>/
  → { "staged_dir": "...", "cli_name": "...", "manuscripts_included": bool, "run_id": "..." }

Step 5 (Skill): Git + PR
  Managed clone at ~/printing-press/.publish-repo/
  → git checkout -b feat/<cli-name>
  → cp -r <staging>/* .
  → update registry.json
  → git add + commit + push
  → gh pr create
```

## Implementation Units

- [ ] **Unit 1: Expand catalog taxonomy to 14 categories**

**Goal:** Update `validCategories` to the new 14-category taxonomy and reassign all catalog YAML files.

**Requirements:** R1, R2, R3

**Dependencies:** None

**Files:**
- Modify: `internal/catalog/catalog.go`
- Modify: `internal/catalog/catalog_test.go`
- Modify: `catalog/asana.yaml`, `catalog/digitalocean.yaml`, `catalog/discord.yaml`, `catalog/front.yaml`, `catalog/github.yaml`, `catalog/hubspot.yaml`, `catalog/launchdarkly.yaml`, `catalog/pipedrive.yaml`, `catalog/plaid.yaml`, `catalog/sendgrid.yaml`, `catalog/sentry.yaml`, `catalog/square.yaml`, `catalog/stripe.yaml`, `catalog/stytch.yaml`, `catalog/telegram.yaml`, `catalog/twilio.yaml`
- Modify: `catalog/petstore.yaml` (stays `example`)
- Modify: `internal/cli/catalog_test.go` (if category strings appear in test expectations)
- Test: `internal/catalog/catalog_test.go`

**Approach:**
- Replace `validCategories` map with the 14 public categories plus `example` (15 total in the map)
- Update the error message string in `Validate()` to list the 14 public categories (exclude `example` from the user-facing message since it's internal-only)
- Reassign YAML files: `sendgrid`: `email`→`marketing`, `hubspot`/`pipedrive`: `crm`→`sales-and-crm`, `discord`/`front`/`telegram`/`twilio`: `communication`→`social-and-messaging`, `digitalocean`: `developer-tools`→`cloud`, `sentry`: `developer-tools`→`monitoring`
- Remaining entries keep their current categories: `asana`→`project-management`, `github`/`launchdarkly`→`developer-tools`, `plaid`/`square`/`stripe`→`payments`, `stytch`→`auth`, `petstore`→`example`

**Patterns to follow:**
- `internal/catalog/catalog.go` line 17-26 for the map structure
- `internal/catalog/catalog_test.go` for table-driven validation tests

**Test scenarios:**
- Happy path: each of the 14 public categories passes validation
- Happy path: `example` still passes validation (petstore backward compat)
- Error path: old categories (`email`, `crm`, `communication`) are rejected
- Happy path: `go test ./...` passes with all 17 YAML files after reassignment
- Edge case: error message lists the 14 public categories, not `example`

**Verification:**
- `go test ./internal/catalog/...` passes
- `go test ./internal/cli/...` passes (catalog command tests)
- All 17 catalog YAML files load without validation errors

---

- [ ] **Unit 2: Add `category` and `description` fields to CLIManifest**

**Goal:** Add `Category` and `Description` to the `CLIManifest` struct and populate them during CLI publishing.

**Requirements:** R4, R4a

**Dependencies:** Unit 1 (valid categories must exist for validation context)

**Files:**
- Modify: `internal/pipeline/climanifest.go`
- Modify: `internal/pipeline/climanifest_test.go`
- Modify: `internal/pipeline/publish.go` (in `writeCLIManifestForPublish`)
- Test: `internal/pipeline/climanifest_test.go`

**Approach:**
- Add `Category string \`json:"category,omitempty"\`` to `CLIManifest` after `CatalogEntry`
- Add `Description string \`json:"description,omitempty"\`` to `CLIManifest` after `Category`
- In `writeCLIManifestForPublish`, after the catalog lookup (line 206), set `m.Category = entry.Category` and `m.Description = entry.Description` when the catalog entry is found
- No schema version bump — additive fields with `omitempty`
- The `description` field is the canonical one-liner for the CLI. It is generated after shipcheck/emboss when the CLI is finalized — not during research, because the CLI's actual commands and features may diverge from the initial brief. At generation time, the catalog entry description serves as a placeholder when available. The full description is a cross-cutting change to the printing-press skill tracked separately. At publish time, the skill reads it from the manifest for the PR description

**Patterns to follow:**
- Existing `CatalogEntry` field for placement and `omitempty` convention
- `writeCLIManifestForPublish` in `internal/pipeline/publish.go` for where to populate

**Test scenarios:**
- Happy path: manifest written with `category` and `description` when catalog entry exists — verify JSON contains both fields
- Happy path: `category` and `description` omitted from JSON when catalog entry not found (non-catalog API)
- Happy path: round-trip read/write preserves both fields
- Edge case: existing manifests without `category` or `description` fields parse without error

**Verification:**
- `go test ./internal/pipeline/...` passes
- Test confirms category and description appear in JSON output when catalog entry has them

---

- [ ] **Unit 3: Add `library list` command**

**Goal:** Add a `printing-press library list` command that lists all CLIs in the local library with manifest metadata.

**Requirements:** R9, R14

**Dependencies:** Unit 2 (reads `category` from manifest)

**Files:**
- Create: `internal/cli/library.go`
- Create: `internal/cli/library_test.go`
- Modify: `internal/cli/root.go` (register `newLibraryCmd()`)
- Test: `internal/cli/library_test.go`

**Approach:**
- Follow `catalog.go` parent command pattern: `newLibraryCmd()` → `newLibraryListCmd()`
- Scan `PublishedLibraryRoot()` for directories matching `IsCLIDirName()` or containing `.printing-press.json`
- For each directory, attempt to read `.printing-press.json`. If readable, extract `api_name`, `cli_name`, `category`, `catalog_entry`. If not readable, still include with empty manifest fields
- Sort by directory modification time (most recent first)
- `--json` outputs array of `{ "cli_name", "dir", "api_name", "category", "catalog_entry", "modified" }`
- Human output: table format similar to `catalog list`

**Patterns to follow:**
- `internal/cli/catalog.go` for parent/child command structure
- `json.NewEncoder(os.Stdout)` for `--json` output
- `ExitError` with `ExitInputError` for filesystem errors

**Test scenarios:**
- Happy path: library with 2 CLIs, both with manifests — JSON output contains both entries with correct fields
- Happy path: human output shows formatted table
- Edge case: library directory doesn't exist — returns empty list, no error
- Edge case: CLI directory exists but `.printing-press.json` is missing — entry included with empty manifest fields
- Edge case: CLI directory with malformed `.printing-press.json` — entry included with empty manifest fields, no crash
- Happy path: handles `-pp-cli-2` suffixed directories (claimed reruns)

**Verification:**
- `go test ./internal/cli/...` passes
- `printing-press library list --json` produces valid JSON in a test environment

---

- [ ] **Unit 4: Add `publish validate` and `publish package` commands**

**Goal:** Add `printing-press publish validate` and `printing-press publish package` commands for pre-publish validation and package assembly.

**Requirements:** R9, R11, R12, R23, R24, R25, R26

**Dependencies:** Unit 2 (reads `category` from manifest), Unit 3 (shared library infrastructure)

**Files:**
- Create: `internal/cli/publish.go`
- Create: `internal/cli/publish_test.go`
- Modify: `internal/cli/root.go` (register `newPublishCmd()`)
- Modify: `internal/cli/exitcodes.go` (add `ExitPublishError`)
- Test: `internal/cli/publish_test.go`

**Approach:**

`publish validate --dir <path> --json`:
- Required flag: `--dir`
- Read `.printing-press.json` from `--dir`, unmarshal into `CLIManifest`
- Run check sequence: manifest exists + required fields, `go mod tidy` (check for diff), `go vet ./...`, `go build ./...`, built binary `--help` and `--version`, manuscripts exist in `PublishedManuscriptsRoot()/<api_name>/`
- **Manuscripts check is warn-only:** Unlike other checks, missing manuscripts sets `"passed": true` with a `"warning"` field rather than failing. This matches R25 ("warns but proceeds"). The overall `"passed"` result is unaffected by manuscript warnings. The package command's re-validation (below) uses this same semantic
- JSON output: `{ "passed": bool, "cli_name": "...", "api_name": "...", "help_output": "...", "checks": [{ "name": "manifest", "passed": bool, "error": "..." }, ...] }`
- The `help_output` field captures the full `--help` output of the built binary. The skill uses this in the PR description to show the CLI's command surface without needing to re-run the binary
- Each check runs independently (don't short-circuit) so the user sees all failures at once
- Use `ExitPublishError` exit code on validation failure

`publish package --dir <path> --category <cat> --target <staging-dir> --json`:
- Required flags: `--dir`, `--category`, `--target`
- Re-validate (call same validation logic, fail early if invalid)
- Resolve manuscripts: scan `PublishedManuscriptsRoot()/<api_name>/` for run directories, lexicographic sort, use most recent
- Create staging structure: `<target>/library/<category>/<cli_name>/`
- `CopyDir` CLI source into staging
- `CopyDir` manuscripts into `<target>/library/<category>/<cli_name>/.manuscripts/<run_id>/`
- JSON output: `{ "staged_dir": "...", "cli_name": "...", "api_name": "...", "category": "...", "manuscripts_included": bool, "run_id": "..." }`

**Patterns to follow:**
- `internal/cli/catalog.go` for parent/child structure
- `internal/pipeline/publish.go` `CopyDir()` for file copying
- `internal/pipeline/paths.go` for `PublishedManuscriptsRoot()`
- `exec.Command("go", "vet", "./...")` for running go tools (see `contracts_test.go` `runGoContractCommand`)

**Test scenarios:**
- Happy path: valid CLI directory passes all checks — JSON shows all checks passed
- Error path: missing `.printing-press.json` — manifest check fails, others may still run
- Error path: `go vet` fails — that check fails, build check may still run
- Error path: manuscripts directory doesn't exist — manuscripts check warns but doesn't fail (per R25)
- Happy path: package creates correct directory structure with CLI source and manuscripts
- Happy path: package resolves most recent manuscript run by lexicographic sort
- Edge case: multiple manuscript runs — uses the most recent
- Edge case: no manuscript runs — packages without `.manuscripts/`, warns in output
- Happy path: `--json` output contains all expected fields
- Error path: `--target` directory already exists — fails with clear error

**Verification:**
- `go test ./internal/cli/...` passes
- Validate command exits with `ExitPublishError` on failure, `ExitSuccess` on pass
- Package command creates the correct directory tree in a temp staging dir

---

- [ ] **Unit 5: Scaffold printing-press-library repo**

**Goal:** Set up the printing-press-library repo at `~/Code/printing-press-library` with the correct directory structure, README, CONTRIBUTING.md, .gitignore, and empty registry.json.

**Requirements:** R5, R6, R7, R8

**Dependencies:** None (can run in parallel with Units 1-4)

**Files (in ~/Code/printing-press-library/):**
- Modify: `README.md`
- Modify: `CONTRIBUTING.md`
- Modify: `.gitignore`
- Create: `registry.json`
- Create: `library/.gitkeep` (anchor for the library root)

**Approach:**
- Rewrite README.md to reflect the new structure: `library/<category>/<cli-name>/`, the 14-category taxonomy, how the publish skill works, what "endorsed" means
- Rewrite CONTRIBUTING.md to describe the publish-skill-driven workflow instead of manual PR instructions. Include: how to install printing-press, how to run `/printing-press publish`, what validation checks run, what the PR will contain
- Update .gitignore to exclude: compiled Go binaries (`*-pp-cli` pattern without extension), `.DS_Store`, `vendor/`, `*.exe`, `*.test`, `*.out`, `__debug_bin*`
- Create `registry.json` with schema: `{ "schema_version": 1, "entries": [] }`
- Create `library/.gitkeep` so the directory exists in git. Category subdirectories are created by the publish skill on first use — no need to pre-create all 14

**Patterns to follow:**
- Current README.md structure for tone and content organization
- Standard Go .gitignore patterns

**Test scenarios:**
- Happy path: `registry.json` is valid JSON with `schema_version: 1` and empty `entries` array
- Happy path: `.gitignore` excludes compiled binaries and common artifacts
- Happy path: README documents the `library/<category>/<cli-name>/` structure and the 14 categories

**Verification:**
- `cat registry.json | jq .` succeeds (valid JSON)
- README accurately describes the publish workflow and category taxonomy
- .gitignore covers the expected artifact patterns

---

- [ ] **Unit 6: Create printing-press-publish skill**

**Goal:** Create the `/printing-press publish` skill that orchestrates the full publish flow: name resolution, category assignment, validation, packaging, managed clone, and PR creation.

**Requirements:** R10, R13a-R13e, R14-R22, R27-R34

**Dependencies:** Units 3, 4 (CLI commands must exist), Unit 5 (library repo must be scaffolded)

**Files:**
- Create: `skills/printing-press-publish/SKILL.md`
- Modify: `internal/pipeline/contracts_test.go` (add new skill to setup contract test table)
- Test: `internal/pipeline/contracts_test.go`

**Approach:**

SKILL.md frontmatter:
- `name: printing-press-publish`
- `description: Publish a generated CLI to the printing-press-library repo`
- `version: 0.1.0`
- `min-binary-version:` set to whatever version includes the new `library list` and `publish` commands
- `allowed-tools: Bash, Read, Write, Edit, Glob, Grep, AskUserQuestion`

Setup contract: Copy the existing setup contract block from the main skill. It must pass `contracts_test.go` assertions (binary on PATH, `PRESS_HOME`, `PRESS_SCOPE`, `PRESS_RUNSTATE`, `PRESS_LIBRARY`, `PRESS_MANUSCRIPTS`, no `./printing-press`, no `go build`)

Skill flow (markdown instructions in SKILL.md):

**Step 1: Prerequisites check**
- Verify `gh auth status` succeeds, else tell user to run `gh auth login`
- Run setup contract

**Step 2: Name resolution**
- Run `printing-press library list --json`
- Apply resolution: exact → suffix (`-pp-cli`) → glob (substring, capped at 5 most-recent matches per R17) → ask user
- If no argument, show all CLIs sorted by modification time, ask user to pick

**Step 3: Read manifest + determine category**
- Read `.printing-press.json` from resolved CLI directory
- If `category` present → confirm with user ("Publish as <category>?")
- Else if `catalog_entry` present → `printing-press catalog show <entry> --json` → extract category → confirm
- Else → present the 14 categories via AskUserQuestion

**Step 4: Validate**
- Run `printing-press publish validate --dir <path> --json`
- Parse JSON result, display check results
- If any check failed → report and stop

**Step 5: Package**
- Create temp staging dir
- Run `printing-press publish package --dir <path> --category <cat> --target <staging> --json`
- Parse JSON result

**Step 6: Managed clone**
- Check if `~/printing-press/.publish-repo/` exists
- If not: detect push access via `gh api repos/mvanhorn/printing-press-library --jq '.permissions.push'`. Fork or clone based on result. Detect SSH vs HTTPS from git config. Cache in `~/printing-press/.publish-config.json`
- If exists: read config, `git fetch origin`, `git checkout main`, `git pull`

**Step 7: Branch + commit + PR**
- Check for existing branch `feat/<cli-name>` (local and remote). If exists, ask user
- `git checkout -b feat/<cli-name>`
- Copy staged package into the managed clone
- Update `registry.json` (read, add/update entry, write back)
- `git add library/ registry.json` (targeted, not `git add .`)
- `git commit -m "feat(<api-name>): add <cli-name>"`
- `git push`
- `gh pr create` with best-in-class PR description (see PR Description Format below)
- Display the PR URL

**PR Description Format:**
The PR body is built from the manifest, README, and manuscripts. It should enable any reviewer (human or agent with no prior context) to understand the contribution:

```
## <cli-name>

<one-liner description from manifest, or "No description available" if missing>

**API:** <api_name> | **Category:** <category> | **Press version:** <printing_press_version>
**Spec:** <spec_url or spec_path from manifest, or "Not specified">

### CLI Shape

\`\`\`bash
$ <cli-name> --help
<full --help output captured during validation>
\`\`\`

### What This CLI Does

<First 2-3 paragraphs from the CLI's README, or note that README is missing>

### Manuscripts

- [Research Brief](link to .manuscripts/<run-id>/research/ in the PR branch)
- [Shipcheck Results](link to .manuscripts/<run-id>/proofs/ in the PR branch)

### Validation Results

| Check | Result |
|-------|--------|
| Manifest | pass/fail |
| go mod tidy | pass/fail |
| go vet | pass/fail |
| go build | pass/fail |
| --help | pass/fail |
| --version | pass/fail |
| Manuscripts | present/missing |

### Gaps

<List any missing manifest fields: no description, no spec_url, no category, etc. Omit section if no gaps>
```

If any manifest fields are empty (description, spec_url, category), the Gaps section explicitly flags them rather than silently omitting. This gives reviewers a clear checklist of what's missing

**Error recovery (Step 6/7):**
- If managed clone has uncommitted changes, detect via `git status --porcelain` and ask user whether to reset or continue
- If push fails, report the error clearly

Update `contracts_test.go`: Add the new skill to the `TestSkillSetupBlocksMatchWorkspaceContract` test table with `expectsManuscripts: true`

**Patterns to follow:**
- `skills/printing-press/SKILL.md` for setup contract and structure
- `skills/printing-press-score/SKILL.md` for name resolution pattern (exact → suffix → glob)

**Test scenarios:**
- Happy path: `contracts_test.go` validates the setup contract block contains all required variables and markers
- Happy path: skill SKILL.md contains `PRESS_SETUP_CONTRACT_START` and `PRESS_SETUP_CONTRACT_END` markers
- Happy path: skill SKILL.md does not reference `./printing-press` or `go build`
- Happy path: skill SKILL.md references `PRESS_MANUSCRIPTS`

**Verification:**
- `go test ./internal/pipeline/...` passes (contracts test)
- SKILL.md is valid YAML frontmatter + markdown
- The skill flow covers all steps from the user flow diagram in the origin document

## System-Wide Impact

- **Interaction graph:** The publish skill invokes three new CLI commands (`library list`, `publish validate`, `publish package`) and two existing ones (`catalog show`, `version`). It also uses `gh` CLI for GitHub operations. The new CLI commands read from `~/printing-press/library/` and `~/printing-press/manuscripts/` (read-only)
- **Error propagation:** CLI commands return structured JSON with per-check results. The skill interprets these and provides user-facing guidance. `ExitPublishError` code distinguishes publish failures from other error types
- **State lifecycle risks:** The managed clone at `~/printing-press/.publish-repo/` can accumulate stale state from interrupted publishes. The skill detects uncommitted changes and offers reset. The `.publish-config.json` cache can become stale if the user changes GitHub access — cache entries should be re-probed on auth errors
- **API surface parity:** The `library list` command introduces a new public CLI surface. It should follow the same `--json` conventions as `catalog list`
- **Unchanged invariants:** The printing process itself is not modified. Existing CLIs without a `category` field continue to work — the field is `omitempty`. The `catalog` commands continue to work with the expanded category set. `petstore.yaml` continues to use `example` category

## Risks & Dependencies

| Risk | Mitigation |
|------|------------|
| `gh repo fork` may behave unexpectedly when fork already exists | Test during implementation; fall back to detecting existing fork and cloning it |
| Catalog category changes break existing tests | Atomic update in Unit 1 — categories and YAML files change together, all tests run before merge |
| Version consistency test fails when version files are out of sync | Don't manually edit versions; let release-please handle it (see AGENTS.md versioning) |
| Skill becomes token-heavy due to inline git bash | Monitor complexity during implementation; extract to scripts/ if git logic exceeds ~30 lines |
| printing-press-library repo URL changes in the future | URL stored in `.publish-config.json`, single point of change (R13d) |

## Documentation / Operational Notes

- The printing-press-library README and CONTRIBUTING.md are rewritten as part of Unit 5
- AGENTS.md in this repo may need a note about the new `library` and `publish` commands once they ship
- The new skill needs a `min-binary-version` in frontmatter that gates on the version containing the new commands

## Sources & References

- **Origin document:** [docs/brainstorms/2026-03-29-publish-skill-requirements.md](docs/brainstorms/2026-03-29-publish-skill-requirements.md)
- Catalog validation: `internal/catalog/catalog.go`
- CLI manifest: `internal/pipeline/climanifest.go`
- Path helpers: `internal/pipeline/paths.go`
- Cobra command pattern: `internal/cli/catalog.go`
- Setup contract tests: `internal/pipeline/contracts_test.go`
- Naming conventions: `internal/naming/naming.go`
- File copying: `internal/pipeline/publish.go` (`CopyDir`)
- Score skill name resolution: `skills/printing-press-score/SKILL.md`
- Decoupling plan: `docs/plans/2026-03-28-001-refactor-repo-decoupling-plan.md`
- Library repo: `~/Code/printing-press-library` (github.com/mvanhorn/printing-press-library)
