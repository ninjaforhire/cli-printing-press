---
date: 2026-03-29
topic: publish-skill
---

# Publish Skill: Ship CLIs to the Library Repo

## Problem Frame

Users generate CLIs with the printing press and they end up in `~/printing-press/library/`. But there's no streamlined way to contribute a finished CLI to the shared printing-press-library repo (github.com/mvanhorn/printing-press-library). Today this would require manually cloning the repo, copying files, organizing them, and creating a PR. The publish skill makes this a single command: `/printing-press publish notion-pp-cli`.

## User Flow

```
/printing-press publish notion-pp-cli
         │
         ▼
┌─ 1. RESOLVE CLI NAME ─────────────────────────────────┐
│  CLI binary: printing-press library list --json        │
│                                                        │
│  "notion-pp-cli" ──exact match──▶ found                │
│  "notion" ──suffix match──▶ notion-pp-cli              │
│  "cal" ──glob match──▶ multiple? ──▶ [ASK USER]       │
│  no match ──▶ show all CLIs ──▶ [ASK USER]            │
└────────────────────────────────────────────────────────┘
         │
         ▼
┌─ 2. DETERMINE CATEGORY ───────────────────────────────┐
│  Read .printing-press.json                             │
│                                                        │
│  has category? ──▶ "Publish as productivity?" ─[ASK]─▶│
│  has catalog_entry? ──▶ look up ──▶ confirm ──[ASK]──▶│
│  neither? ──▶ show 14 categories ────────────[ASK]──▶ │
└────────────────────────────────────────────────────────┘
         │
         ▼
┌─ 3. VALIDATE ─────────────────────────────────────────┐
│  CLI binary: printing-press publish validate --json    │
│                                                        │
│  ✓/✗ .printing-press.json    ✓/✗ go mod tidy          │
│  ✓/✗ go vet                  ✓/✗ go build             │
│  ✓/✗ --help responds         ✓/✗ --version responds   │
│  ✓/✗ manuscripts found                                │
│                                                        │
│  any ✗? ──▶ report errors, STOP                       │
└────────────────────────────────────────────────────────┘
         │
         ▼
┌─ 4. PACKAGE ──────────────────────────────────────────┐
│  CLI binary: printing-press publish package            │
│    --dir ~/printing-press/library/notion-pp-cli        │
│    --category productivity                             │
│    --target /tmp/staging                               │
│                                                        │
│  Creates:  library/productivity/notion-pp-cli/          │
│              cmd/ internal/ go.mod ...                  │
│              .manuscripts/                             │
│                research/ proofs/                        │
└────────────────────────────────────────────────────────┘
         │
         ▼
┌─ 5. GIT + PR (skill handles directly) ────────────────┐
│  Ensure managed clone at ~/.publish-repo/              │
│  git fetch + checkout main + pull                      │
│  git checkout -b feat/notion-pp-cli                    │
│  Copy staged package into clone                        │
│  Update registry.json                                  │
│  git add + commit: "feat(notion): add notion-pp-cli"   │
│  git push + gh pr create                               │
│                                                        │
│  ✓ PR opened: github.com/.../pull/42                   │
└────────────────────────────────────────────────────────┘
```

**User decisions:** Steps 1-2 may require input (name disambiguation, category confirmation). Steps 3-5 are automatic. Happy path = 1 decision (category confirm).

## Architecture

```
LOCAL MACHINE                                    GITHUB
─────────────────────────────────────────────    ──────────────────────

~/printing-press/
├── library/                                     mvanhorn/
│   └── notion-pp-cli/  ◄── source of truth      printing-press-library
│       ├── cmd/                                  ├── library/
│       ├── internal/                             │   ├── productivity/
│       ├── .printing-press.json                  │   │   └── notion-pp-cli/
│       └── ...                                   │   │       ├── cmd/
│                                                 │   │       ├── .manuscripts/
├── manuscripts/                                  │   │       └── ...
│   └── notion/                                   │   └── developer-tools/
│       └── 20260328-132022/  ◄── provenance      │       └── github-pp-cli/
│           ├── research/                         ├── registry.json
│           └── proofs/                           └── README.md
│           └── proofs/
│
├── .publish-repo/  ◄── managed clone (skill)
│   └── (clone of printing-press-library)
│
└── .publish-config.json  ◄── cached state
    { access: "push|fork",
      protocol: "ssh|https",
      repo_url: "..." }


COMPONENT RESPONSIBILITIES
──────────────────────────

┌──────────────────────┐    ┌──────────────────────┐    ┌─────────────┐
│    CLI BINARY (Go)   │    │   SKILL (LLM/bash)   │    │  GITHUB     │
│                      │    │                      │    │             │
│ • library list       │───▶│ • name resolution UX │    │             │
│ • publish validate   │───▶│ • category assignment │    │             │
│ • publish package    │───▶│ • validation display  │    │             │
│                      │    │ • git clone/branch    │───▶│ • fork      │
│ Structured JSON out  │    │ • git commit/push     │───▶│ • PR create │
│ Zero LLM tokens      │    │ • registry.json edit  │    │ • PR merge  │
│ Reuses Go types      │    │ • error guidance      │    │             │
└──────────────────────┘    └──────────────────────┘    └─────────────┘
     deterministic               interactive               remote
```

## Requirements

**Category Taxonomy**

- R1. The printing-press-library repo organizes CLIs into category folders using a 14-category single-level taxonomy: `developer-tools`, `monitoring`, `cloud`, `project-management`, `productivity`, `social-and-messaging`, `sales-and-crm`, `marketing`, `payments`, `auth`, `commerce`, `ai`, `devices`, `other`
- R2. The `other` category acts as a catch-all. When enough CLIs accumulate around a theme in `other`, that signals a new category should be split out
- R3. The catalog's `validCategories` in `internal/catalog/catalog.go` must be updated to match this taxonomy. Four current categories are dropped and require mandatory reassignment: `email` -> `marketing`, `crm` -> `sales-and-crm`, `communication` -> `social-and-messaging`. The `example` category is removed from the public taxonomy; `petstore.yaml` (test fixture) retains `example` as an internal-only category not used in the library repo. Additional reassignments: DigitalOcean `developer-tools` -> `cloud`, Sentry `developer-tools` -> `monitoring`. All 17 catalog YAML files must be validated against the new taxonomy before merging
- R4. [Cross-cutting: requires changes to the printing-press skill and CLI binary] The `.printing-press.json` CLI manifest gains a `category` field (string, matching one of the 14 slugs). This is populated during the printing process using the catalog entry's category when available, or determined by the research phase for non-catalog APIs. The publish skill reads this field (R20) but does not write it
- R4a. [Cross-cutting: requires changes to the printing-press skill and CLI binary] The `.printing-press.json` CLI manifest gains a `description` field (string, one-liner). This description is generated after shipcheck/emboss completes — when the CLI is finalized and the actual command surface is known. The manifest stores it as the canonical one-liner. The CLI README and PR description both consume it. The publish skill reads this field but does not write it
- R4b. [Cross-cutting: requires changes to the printing-press skill] Each generated CLI includes a `README.md` in its root directory. The README is generated after shipcheck/emboss — not during the research phase — because the CLI's actual commands, features, and product thesis may diverge from the initial brief during build and verification. The publish skill assumes the README already exists when the user runs `/printing-press publish`. The README follows this structure:
  1. **Title + one-liner** from the manifest `description`
  2. **API links** — links to the API provider's homepage, developer docs, and public spec (if available)
  3. **Why this exists** — 2-3 sentences. The product thesis tested against the actual built CLI
  4. **Install** — Homebrew, `go install`, binary download. Copy-paste commands only
  5. **Quick start** — Numbered steps: install → `doctor` → first real command
  6. **Commands** — Grouped by resource, one-liner per command. Generated from the actual `--help` output
  7. **Agent & automation** — Factual description of the CLI's agent-optimized properties: non-interactive (never prompts), `--json` to stdout, `--select` for field filtering, `--dry-run` for safe preview, typed exit codes. Not promotional — states how the CLI behaves in pipelines and agent contexts
  8. **Exit codes** — Table mapping codes to meanings
  9. **Configuration** — Config file path, environment variables
  10. **Troubleshooting** — Exit code → cause → fix, domain-specific
  11. **Attribution** — "Built with [CLI Printing Press](https://github.com/mvanhorn/cli-printing-press)." Single line, end of file. Users can edit or remove this

**Library Repo Structure**

- R5. The printing-press-library repo nests all CLIs under a `library/` root folder: `library/<category>/<cli-name>/`. This keeps the repo root clean for README, registry.json, contributing docs, and future tooling
- R6. Each CLI folder (`library/<category>/<cli-name>/`) contains the full CLI source code plus a `.manuscripts/` directory (dot-prefixed) containing the research and proof artifacts from the printing run
- R7. The repo root contains a `registry.json` — a machine-readable index of all CLIs with their name, category, api_name, and description
- R8. The repo's `.gitignore` is the single source of truth for excluding build artifacts (compiled binaries, .DS_Store, vendor/, etc.) from commits

**CLI/Skill Responsibility Split**

- R9. The CLI binary owns all deterministic, local operations: validation checks (`publish validate`), package assembly (`publish package`), and library listing (`library list`). These commands accept `--json` for structured output and cost zero LLM tokens
- R10. The skill owns all interactive and networked operations: name resolution UX, category assignment, git plumbing (clone, branch, commit, push), PR creation via `gh`, registry.json updates, and error guidance
- R11. CLI validation (`printing-press publish validate --dir <path> --json`) runs all checks from R22 and returns structured JSON with pass/fail per check and error details. The skill interprets the results and presents them to the user
- R12. CLI package assembly (`printing-press publish package --dir <path> --category <cat> --target <staging-dir> --json`) copies the CLI source and manuscripts into a staging directory matching the library repo structure (`library/<category>/<cli-name>/`). It re-validates before packaging and returns JSON describing what was assembled

**Managed Clone**

- R13a. The publish skill manages its own clone of the printing-press-library repo at `~/printing-press/.publish-repo/`. Users never need to manually clone, pull, or interact with this repo
- R13b. On first publish, the skill detects push access via `gh api` and either clones directly (push access) or forks first (no push access) using `gh repo fork`. SSH vs HTTPS is auto-detected based on the user's git configuration
- R13c. On subsequent publishes, the skill freshens the clone (`git fetch`, checkout main, pull) before creating a new branch
- R13d. The target repo URL is configured in one place so it can be changed in the future without user-facing impact
- R13e. Access level (push vs fork), git protocol (SSH vs HTTPS), and clone path are cached in `~/printing-press/.publish-config.json` so the skill does not re-probe on every publish

**Name Resolution**

- R14. The skill accepts a CLI name argument: `/printing-press publish <name>`. The skill uses `printing-press library list --json` to get the available CLIs, then applies resolution logic
- R15. Exact match: look for `<name>` in the library list. If found, use it
- R16. Suffix match: if no exact match, try `<name>-pp-cli` (e.g., `notion` -> `notion-pp-cli`). If found, use it
- R17. Glob match: if no suffix match, search for entries containing `<name>` as a substring. If multiple matches, present via AskUserQuestion for user selection. Show at most 5 matches sorted by modification time (most recent first). This resolution order (exact -> suffix -> glob) matches the score skill for consistent behavior across skills
- R18. No match: if nothing matches, list available CLIs and ask the user to pick or re-enter
- R19. No argument: if invoked as `/printing-press publish` with no name, list all CLIs in the library sorted by modification time and let the user pick

**Category Assignment**

- R20. If the CLI's `.printing-press.json` has a `category` field, use it as the default. Present to the user for confirmation with the option to change
- R21. If no `category` in manifest but `catalog_entry` is present, look up the category from the embedded catalog. Present for confirmation
- R22. If neither source provides a category, present the full category list and ask the user to choose

**Publish Validation**

- R23. The CLI binary's `publish validate` command checks: `.printing-press.json` exists with required fields, `go mod tidy` reports no changes needed, `go vet ./...` passes, `go build ./...` succeeds, the built binary responds to `--help` and `--version`, and manuscripts exist. Returns structured JSON with pass/fail per check
- R24. If validation fails, the skill reports what's wrong (from the JSON output) and stops. Do not create a partial PR

**Package Assembly**

- R25. The CLI binary's `publish package` command resolves manuscripts from `$PRESS_MANUSCRIPTS/<api-name>/`, selecting the most recent run by lexicographic sort on the run-id directory name (run-ids are timestamp-prefixed, e.g., `20260328-132022`, so lexicographic order equals chronological order). If no manuscripts exist, it warns but proceeds
- R26. The package command copies the CLI source and manuscripts into a staging directory structured as `library/<category>/<cli-name>/` with manuscripts in `.manuscripts/`, ready to drop into the library repo

**PR Creation**

- R27. The skill creates a branch named `feat/<cli-name>` in the managed clone. If a local or remote branch with that name already exists (stale from a previous attempt), detect it and ask the user whether to overwrite or create a timestamped variant (e.g., `feat/<cli-name>-20260329`)
- R28. The skill commits with conventional format: `feat(<api-name>): add <cli-name>`
- R29. The skill pushes and creates a PR via `gh pr create` with a best-in-class structured description that enables any reviewer (human or agent) to understand the contribution without prior context. The PR body includes: the one-liner description from the manifest, the API name and service it connects to, the category, the full `--help` output of the built CLI binary in a bash code block (giving reviewers an instant view of the command surface), a link to the CLI's README within the PR branch, links to the `.manuscripts/` folders (research brief, absorb manifest, shipcheck results) within the PR branch, the validation check results, and the printing-press version used to generate it. If any manifest fields are missing (e.g., no `description`, no `spec_url`), the PR description flags these gaps explicitly rather than silently omitting them
- R30. If a PR already exists for this CLI name (e.g., from a previous publish attempt), warn the user and ask whether to update the existing branch or create a new one

**Error Handling**

- R31. If `gh` CLI is not authenticated, detect this early and tell the user to run `gh auth login`
- R32. If the printing-press-library repo is unreachable, report the error clearly
- R33. If there are uncommitted changes in the managed clone from a previous interrupted publish, detect and offer to reset or continue

**Registry Update**

- R34. During each publish, the skill updates `registry.json` at the repo root by adding or updating the entry for the published CLI. The entry is derived from the CLI's `.printing-press.json` manifest and the chosen category

## Success Criteria

- A user can go from `/printing-press publish notion-pp-cli` to an open PR in under 2 minutes with no manual git operations
- Non-catalog CLIs get published just as easily as catalog CLIs (category is asked, not blocked)
- The publish flow is recoverable — interrupted runs don't leave the managed clone in a broken state
- The printing-press-library repo is browsable by category, and `registry.json` is machine-readable for tooling

## Scope Boundaries

- This skill does NOT run `printing-press verify` or `printing-press scorecard` as part of validation. It checks build + manifest + manuscripts only. Full quality gates are the printing process's responsibility
- This skill does NOT modify the user's local library (`~/printing-press/library/`). It only reads from it
- This skill does NOT handle updating an already-published CLI (re-publish / version bump). That's a future capability
- The category taxonomy update to `internal/catalog/catalog.go` is in scope. Reassigning existing catalog entries to new categories is in scope
- Updating the printing-press-library README to reflect the new structure is in scope
- The `registry.json` schema and initial generation are in scope, but tooling that consumes it is not
- CLI README generation (R4b) is a cross-cutting change to the printing-press skill and generator, not part of the publish skill itself. The publish skill assumes a README already exists. If it doesn't, the PR description notes its absence
- **Trust verification is NOT in scope for the publish skill.** The publish skill ensures provenance data (manuscripts, manifest) is always included so trust can be verified, but the actual verification happens in the printing-press-library repo's CI and review process. See "Future: Library Trust Verification" below

## Key Decisions

- **Single-level taxonomy**: Two-level nesting (category/subcategory) was considered but rejected. Adds classification complexity without enough benefit at current scale. Can revisit when the library has 50+ CLIs
- **`other` catch-all**: Preferred over forcing every API into an imperfect category. Signals when new categories are needed organically
- **Managed clone at `~/printing-press/.publish-repo/`**: Users never manually interact with the library repo. The skill handles all git plumbing. This follows the existing pattern where `~/printing-press/` is managed space
- **CLI binary for validation + packaging, skill for interaction + git**: Deterministic local operations (validation checks, file assembly, library listing) run in the CLI binary with `--json` output — zero tokens, structured errors. Interactive and networked operations (name resolution UX, category assignment, git/GitHub plumbing, PR creation) stay in the skill. This split keeps the skill thin and token-efficient while ensuring validation is repeatable and fast
- **Category in manifest**: Adding `category` to `.printing-press.json` means the CLI folder is self-describing even outside the library repo. The printing process determines category via catalog lookup or research-phase classification
- **Replaced `communication` with `social-and-messaging`**: Broadens the scope to cover messaging infrastructure (Twilio, Front), social platforms (Twitter, Reddit), and media/streaming (Spotify, YouTube) under one umbrella. This is a semantic expansion, not just a rename — Front and Twilio are communication tools, while Spotify and YouTube are media platforms, but all share the "connect people to content or each other" pattern
- **`.manuscripts/` dot-prefixed**: Keeps provenance data present but unobtrusive in the CLI folder listing
- **Auto-classification for non-catalog APIs**: The research phase (Phase 1) auto-classifies the API against the taxonomy and writes the category to the manifest. It always picks a best-fit category rather than leaving it empty. The user can override at publish time (R20)
- **Description and README generated after shipcheck/emboss**: Both artifacts are produced when the CLI is finalized, not during the research phase. The research brief contains a thesis, but the actual CLI may diverge significantly during build and verification — commands renamed, features added/dropped, transcendence features built. The description and README reflect the real CLI, not the plan
- **One-liner description as canonical source**: The manifest stores the one-liner. The README uses it as the opening line. The PR description pulls it from the manifest. This avoids having multiple independent sources for "what does this CLI do"
- **CLI README is a generation-time artifact**: The README exists before publish and is part of the generated CLI. The publish skill consumes it (for PR description context and linking) but doesn't create it. This separation means READMEs can improve through emboss passes before publishing

## Dependencies / Assumptions

- `gh` CLI is installed and authenticated (the skill checks this early)
- User has a GitHub account (needed for fork/PR workflow)
- The printing process already ran and the CLI is in `~/printing-press/library/`
- Manuscripts exist in `~/printing-press/manuscripts/` from the printing run
- The printing-press-library repo is cloned locally at `~/Code/printing-press-library` (github.com/mvanhorn/printing-press-library). README, CONTRIBUTING.md, .gitignore, and initial repo scaffolding (category folders, registry.json) are updated there as part of this work

## Future: Library Trust Verification

The printing-press-library endorses CLIs — anything merged carries an implicit stamp of quality and safety. The publish skill ensures provenance (manuscripts + manifest) ships with every submission, but verifying trust is the library repo's responsibility. CLIs are explicitly allowed to be human-modified after printing (emboss passes, manual improvements), so an immutable-artifact approach won't work.

**Recommended approach: CI-based verification on every PR to printing-press-library.**

Three layers, in priority order:

1. **Regeneration diff** — Re-run `printing-press generate` from the spec in `.manuscripts/` and produce a focused diff against the submitted code. Human modifications are expected and allowed, but the diff makes them explicit. Reviewers (human or automated) only need to scrutinize the delta, not the entire CLI. Unexplained changes to network-facing code get flagged.

2. **Network audit** — Statically scan all Go source for outbound HTTP calls (`http.Get`, `http.Post`, `net/http.NewRequest`, etc.) and compare every target URL/host against the spec's `servers` URLs. Any call to a host not in the spec is a hard flag. This catches the exact malicious-exfiltration scenario regardless of whether code was modified.

3. **Dependency audit** — Scan `go.mod` for unexpected dependencies not present in the press-generated baseline. New dependencies in a modified CLI aren't automatically bad, but they warrant review.

This is a separate initiative owned by the printing-press-library repo. The publish skill's contribution is ensuring `.manuscripts/` and `.printing-press.json` are always present (R6, R25) so these checks have the provenance data they need.

## Outstanding Questions

### Resolve Before Planning

(None — all blocking questions resolved)

### Deferred to Planning

- [Affects R7][Technical] What fields should `registry.json` contain? Likely: cli_name, api_name, category, description, printing_press_version, published_date. Needs schema design
- [Affects R13b][Needs research] Does `gh repo fork --clone` handle the case where the user already has a fork gracefully? Need to test the exact `gh` behavior
- [Affects R13d][Technical] Where should the target repo URL be configured? Options: skill frontmatter, a press config file at `~/printing-press/.publish-config.json`, or hardcoded with a flag override
- [Affects R26][Technical] Should manuscripts be copied flat (all runs merged) or preserve the run-ID directory structure inside `.manuscripts/`?
- [Affects R9-R12][Technical] Exact CLI subcommand naming and flag design for `publish validate`, `publish package`, and `library list`. Needs to align with existing CLI command structure
- [Affects R10][Technical] If the skill's inline git/GitHub bash (managed clone, fork detection, branch management, PR creation) grows beyond ~30 lines or accumulates complex error handling, consider extracting it into shell scripts under `skills/printing-press-publish/scripts/`. Go CLI is still preferred for validation and packaging (reuses Go types), but scripts are a good middle ground for orchestration logic that is too complex for inline skill bash but doesn't need Go

## Next Steps

-> `/ce:plan` for structured implementation planning
