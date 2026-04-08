---
title: "feat: Mega MCP — Library-Wide Aggregate MCP Server with Catalog Discovery and Tool Passthrough"
type: feat
status: active
date: 2026-04-06
deepened: 2026-04-06
---

# feat: Mega MCP — Library-Wide Aggregate MCP Server with Catalog Discovery and Tool Passthrough

## Overview

**Step 2 of 2** toward a Printing Press Mega MCP. Step 1 (PR #145) built the data layer: per-endpoint auth awareness, MCP metadata in manifests, self-describing `about` tools, minority-side auth annotations, smithery.yaml, and registry.json MCP fields.

This plan builds the access layer: a single MCP server binary that aggregates every API in the local library into one installable MCP connection. An agent adds one MCP server and gets access to every printed CLI's tools — Dub links, ESPN scores, Steam achievements, Pagliacci menus — with catalog discovery, auth awareness, and passthrough execution to the per-API MCP binaries.

The plan has two phases:
1. **Directory restructure** — library directories keyed by API slug (`dub/`) instead of CLI name (`dub-pp-cli/`). Prerequisite for clean mega MCP discovery and an overdue alignment with manuscripts (already slug-keyed).
2. **Mega MCP server** — new binary that scans the library, discovers tools from per-API MCP servers, and routes tool calls through.

## Problem Frame

Every printed CLI already ships an MCP binary — but using them requires adding each one individually to Claude Code or Claude Desktop. With 6 CLIs today and a growing catalog, this is friction that compounds. Agents have no way to discover what APIs are available without the user manually configuring each MCP connection.

The data layer from Step 1 makes each MCP server self-describing. This plan makes them collectively accessible: one `claude mcp add` gives agents access to every API in the library.

A prerequisite directory restructure is included because the current library layout (`dub-pp-cli/`) names directories after the CLI binary, which misrepresents the contents (each directory contains both CLI and MCP binaries) and creates awkward mapping for the mega MCP's catalog-style discovery. Manuscripts are already keyed by API slug; the library should match.

## Requirements Trace

- R1. Library directories keyed by API slug, not CLI name
- R2. Backward-compatible discovery: accept both old (`dub-pp-cli/`) and new (`dub/`) directory layouts during transition
- R3. Mega MCP binary discovers all MCP-ready CLIs in `~/printing-press/library/` via manifest scanning
- R4. Mega MCP exposes a `library_info` tool listing all available APIs, tool counts, auth status, and readiness
- R5. Mega MCP registers all per-API tools with namespaced names and routes calls to per-API MCP subprocesses
- R6. Auth-aware: surfaces missing credentials and forwards per-API env vars to subprocesses with credential isolation (each subprocess receives only its own API's env vars)
- R7. Extensible: picks up newly published CLIs on restart without rebuilding the mega MCP binary
- R8. Installable via `claude mcp add` with a single command
- R9. Publish pipeline and skills updated for slug-keyed library paths

## Scope Boundaries

- **In scope:** Local library aggregation only. The mega MCP reads `~/printing-press/library/`, not the public library repo.
- **In scope:** Startup-time probing of per-API MCP binaries via `tools/list` to discover full tool schemas. This avoids a new generation artifact and uses the MCP protocol as designed.
- **In scope:** `mcp-go` as a direct dependency in the printing-press `go.mod` — the mega MCP is a first-class MCP server.
- **Out of scope:** Public library repo directory migration — that's a separate PR in `mvanhorn/printing-press-library` after the machine changes land.
- **Out of scope:** MCP marketplace submission for the mega MCP itself. We build the binary; marketplace listing is a separate effort.
- **Out of scope:** Hot-reloading of the library while the mega MCP is running. A restart picks up changes. Live watch can be added later.
- **Out of scope:** Cross-API workflows or tool composition. The mega MCP is a multiplexer, not an orchestrator.
- **Out of scope:** Changing binary names (`dub-pp-cli`, `dub-pp-mcp`) — these stay as-is. Only the directory key changes.
- **Known limitation:** CLIs without MCP metadata in their manifest (e.g., generated via `--docs` before Step 1) will not appear in the mega MCP. They need to be regenerated or manually have MCP metadata added.

## Context & Research

### Relevant Code and Patterns

- **Library scanner:** `scanLibrary()` at `internal/cli/library.go:86-144` already iterates `~/printing-press/library/`, reads `.printing-press.json` manifests, and returns structured entries. Inclusion criteria: `naming.IsCLIDirName(dirName) || entry.APIName != ""`. The `APIName != ""` branch means manifest-bearing directories are accepted regardless of naming — this is the migration bridge.
- **Path construction:** `DefaultOutputDir(apiName)` at `internal/pipeline/pipeline.go:16` returns `filepath.Join(PublishedLibraryRoot(), naming.CLI(apiName))`. This is the single point that determines library directory names.
- **Naming functions:** `internal/naming/naming.go` has `CLI()`, `MCP()`, `TrimCLISuffix()`, `IsCLIDirName()`. The restructure needs a counterpart for slug-keyed dirs.
- **CLIManifest MCP fields:** `MCPBinary`, `MCPToolCount`, `MCPPublicToolCount`, `MCPReady`, `AuthType`, `AuthEnvVars`, `NovelFeatures` — all added in Step 1. The mega MCP reads these directly.
- **Per-API MCP server template:** `main_mcp.go.tmpl` creates an `mcp-go` server via `server.NewMCPServer()` + `server.ServeStdio()`. Tool names follow `{resource}_{endpoint}` pattern (e.g., `links_list`, `links_create`). The `about` tool returns self-describing metadata.
- **MCP Go SDK (`mark3labs/mcp-go`):** Used in generated templates for per-API MCP servers. Currently NOT a direct dependency of the printing-press binary. Supports `server.ServeStdio()` for server-side. Client-side capabilities (for probing sub-MCPs) need to be verified during implementation.
- **Publish pipeline:** `PublishWorkingCLI()` at `internal/pipeline/publish.go:83` copies working dir to library and writes manifest. Uses `DefaultOutputDir()` for the target path.
- **`ClaimOutputDir()`:** Atomic directory claiming with `-2`, `-3` suffixes for reruns. Works on any base path — slug-keyed dirs will work without changes to this function.
- **`RenameCLI()`** at `internal/pipeline/renamecli.go` already handles MCP directory renaming alongside CLI renaming. Needs update for slug-keyed outer directories.
- **Goreleaser config:** `internal/generator/templates/goreleaser.yaml.tmpl` builds both CLI and MCP binaries. The printing-press binary's own `.goreleaser.yaml` would need a new entry for the mega MCP binary.
- **Test patterns:** `PRINTING_PRESS_HOME` env var override for isolation. Table-driven tests with `testify/assert`. `writeTestManifest` helper in pipeline tests.

### Institutional Learnings

- **Layout contract** (`docs/solutions/best-practices/checkout-scoped-printing-press-output-layout`): Published CLIs live at `~/printing-press/library/<dir>/`. The manifest is the source of truth for identity, not the directory name. Rerun suffixes (`-pp-cli-2`) are valid — the actual command entrypoint lives inside `cmd/<api>-pp-cli/`.
- **Filepath traversal protection** (`docs/solutions/security-issues/filepath-join-traversal-with-user-input`): Any MCP tool name or argument that feeds into `filepath.Join` is an attack surface. Belt-and-suspenders: reject `..`, `/`, `\` in input AND verify resolved path is under library root with `strings.HasPrefix(absResult, absRoot + string(filepath.Separator))`.
- **Validation must not mutate source directory** (`docs/solutions/best-practices/validation-must-not-mutate-source-directory`): The mega MCP must treat `~/printing-press/library/` as read-only. If it needs to build MCP binaries, use temp directories.
- **Independent source discovery** (`docs/solutions/best-practices/multi-source-api-discovery-design`): Use `errgroup.Group` (not `WithContext`) when probing CLIs. A broken CLI binary should degrade gracefully (log warning, exclude), not crash the server.

## Key Technical Decisions

- **Separate binary, not a subcommand:** The mega MCP lives at `cmd/printing-press-mcp/main.go` as its own binary. MCP servers are long-running processes invoked by `claude mcp add`; they should not depend on invoking the full printing-press generator binary. The printing-press binary is for code generation; the mega MCP binary is for runtime tool serving. Goreleaser builds both.

- **API-slug directory key with backward compat:** `DefaultOutputDir` changes from `naming.CLI(apiName)` to just `apiName`. The library scanner accepts both layouts — manifest presence (`APIName != ""`) is the inclusion criterion, not directory name suffix. This means existing `dub-pp-cli/` directories continue to work alongside new `dub/` directories. A `library migrate` command is deferred to a follow-up since it's cosmetic — backward-compat discovery makes it unnecessary for functionality.

- **Double-underscore tool namespacing (`dub__links_list`):** Per-API tool names are prefixed with `{normalized-slug}__` (double underscore). Single underscore is already used within tool names (`links_list`), so double underscore is the unambiguous separator. The mega MCP strips the prefix to route to the correct subprocess. Examples: `dub__links_list`, `espn__get_scores`, `steam_web__get_player_summaries`. **Slug-to-prefix normalization:** API slugs are kebab-case (`steam-web`), but MCP tool names cannot contain hyphens. Slugs are normalized to snake_case for the prefix: hyphens → underscores, consecutive underscores collapsed to single (`steam--web` → `steam_web`, not `steam__web`). The normalizer rejects any slug that produces a prefix containing `__` after conversion (would be ambiguous with the separator). In practice this cannot happen with `cleanSpecName()`'s output, but the guard prevents future regressions.

- **Persistent per-API MCP subprocesses:** The mega MCP starts each per-API MCP binary as a long-running subprocess at startup, rather than spawning ephemeral processes per tool call. Rationale: (1) amortizes Go binary startup time, (2) the per-API MCP server may hold state (config, connection pools), (3) the MCP protocol is designed for persistent connections. The subprocess manager restarts crashed processes on the next tool call.

- **Startup-time tool discovery via `tools/list`:** At startup, the mega MCP sends a `tools/list` JSON-RPC request to each running sub-MCP to discover the full tool schema (names, descriptions, parameters). This avoids a new generation artifact and uses the MCP protocol as designed. Discovery results are cached in memory. A `library_info` meta-tool surfaces the catalog for agents.

- **Require pre-built MCP binaries (no build-on-demand):** The mega MCP expects each per-API MCP binary to be pre-built and present in the library directory. It looks for the binary at `cmd/{mcp-binary-name}/{mcp-binary-name}` (the standard `go build` output location) or the Makefile's output path. If the binary doesn't exist, the API is excluded with a clear error message telling the user to run `cd ~/printing-press/library/{slug} && go build -o ./cmd/{name}-pp-mcp/{name}-pp-mcp ./cmd/{name}-pp-mcp`. This avoids the complexity of a build cache, eliminates the runtime Go toolchain requirement for users who receive pre-built binaries, and keeps the mega MCP's startup fast. A follow-up can add build-on-demand for convenience.

- **Credential isolation via env var subtraction:** Each subprocess receives the full parent environment **minus other APIs' `AuthEnvVars`**. For example, the Dub subprocess inherits `DUB_TOKEN` (its own) but NOT `STEAM_WEB_API_KEY` (Steam's). This is implemented by starting from `os.Environ()`, collecting all `AuthEnvVars` from ALL discovered APIs, then removing everything except the current API's vars from that set. System vars (`PATH`, `HOME`, `TMPDIR`, proxy vars like `HTTPS_PROXY`/`HTTP_PROXY`/`NO_PROXY`, TLS vars like `SSL_CERT_FILE`/`SSL_CERT_DIR`, locale vars like `LANG`) are preserved — stripping these would silently break HTTP requests in corporate/non-default environments (Go's `exec.Cmd.Env`, when non-nil, replaces the entire environment). This prevents credential leakage across APIs while preserving all networking and system functionality.

- **`mcp-go` as direct dependency:** Added to the printing-press `go.mod`. The mega MCP uses both server-side (for its own MCP interface) and client-side (for probing sub-MCPs) capabilities. Verify `mcp-go` client support (`client.NewStdioMCPClient` or equivalent) at the start of Unit 5. If `mcp-go` lacks client-side support, implement a JSON-RPC client that handles the MCP initialization handshake (`initialize` → response → `initialized` notification) before sending `tools/list` or `tools/call`. Budget ~200-300 lines for a proper client with handshake, request-response correlation, and timeout handling — not the ~100 lines originally estimated.

- **Printing-press MCP binary naming:** The mega MCP binary is called `printing-press-mcp` (no API-slug prefix, no `-pp-` infix). It represents the printing press itself, not a single API. This distinguishes it from per-API MCP servers (`dub-pp-mcp`, `espn-pp-mcp`).

## Open Questions

### Resolved During Planning

- **Should the directory restructure be in this plan or separate?** → In this plan as Phase 1. The restructure is a prerequisite that makes the mega MCP's discovery cleaner, and the user presented both together.
- **Separate binary or subcommand?** → Separate binary (`printing-press-mcp`). Clean separation between generation-time and runtime concerns. MCP servers are long-running; coupling with the generator binary adds unnecessary weight.
- **How to namespace tool names?** → Double-underscore prefix: `{api-slug}__{tool-name}`. Unambiguous since single underscore is already used within tool names. Easy to split for routing.
- **Ephemeral or persistent subprocesses?** → Persistent. Amortizes startup, supports stateful per-API servers, matches MCP protocol's session model.
- **Where does tool schema come from?** → Probed from running sub-MCPs via `tools/list` at startup. No new generation artifact needed.
- **How does auth work?** → Environment inheritance. User configures env vars in `claude mcp add --env`. Mega MCP passes them through to subprocesses.

### Deferred to Implementation

- **`mcp-go` client-side capabilities** — verify whether `mark3labs/mcp-go` supports MCP client operations (`initialize`, `tools/list`, `tools/call` over stdio). If not, implement a JSON-RPC client with MCP handshake support (~200-300 lines). Check at the start of Unit 5 since it affects subprocess manager design.
- **Tool name collision across APIs** — the prefix + `__` + original name scheme could theoretically produce collisions. Unit 5 validates uniqueness at registration time. Verify with the current 6 CLIs during implementation.
- **MCP proxy alternatives** — the MCP specification supports native proxy/aggregation patterns, and tools like `mcp-proxy` exist. The custom approach is chosen for: control over tool naming, `library_info` integration, auth-aware credential isolation, and library-specific discovery. If implementation complexity exceeds expectations, evaluate off-the-shelf alternatives before building more than Unit 4.

### Resolved During Document Review

- **Subprocess restart strategy** — resolved: restart once on crash (not 3 retries). Log crash events visibly. Don't retry the call that caused the crash. Exclude on failed restart.
- **RenameCLI adaptation for slug-keyed directories** — resolved: option (a). Modify `validateRenameInputs()` to accept names that pass either `IsCLIDirName()` or `IsLibraryDir()`.
- **Serialized vs multiplexed subprocess communication** — resolved: multiplexed using JSON-RPC `id` correlation. Head-of-line blocking from serialized model would degrade UX, and multiplexing is ~50 lines beyond what serialized needs.
- **Credential isolation** — resolved: subtraction model. Each subprocess gets full parent env minus other APIs' AuthEnvVars. Preserves networking/TLS/proxy vars. Safer than allowlist approach which would silently break HTTP in corporate environments.

## High-Level Technical Design

> *This illustrates the intended approach and is directional guidance for review, not implementation specification. The implementing agent should treat it as context, not code to reproduce.*

```
                              ┌─────────────────────────┐
                              │     Agent (Claude)       │
                              └──────────┬──────────────┘
                                         │ MCP (stdio)
                              ┌──────────▼──────────────┐
                              │   printing-press-mcp     │
                              │   (Mega MCP Server)      │
                              │                          │
                              │  ┌───────────────────┐   │
                              │  │   Tool Router      │   │
                              │  │   dub__*  → dub    │   │
                              │  │   espn__* → espn   │   │
                              │  │   steam_web__* → … │   │
                              │  └───────┬───────────┘   │
                              │          │               │
                              │  ┌───────▼───────────┐   │
                              │  │ Subprocess Manager │   │
                              │  │ (per-API MCP bins) │   │
                              │  └───────┬───────────┘   │
                              └──────────┼──────────────┘
                       ┌─────────────────┼──────────────────┐
                       │                 │                   │
              ┌────────▼──────┐  ┌───────▼───────┐  ┌───────▼───────┐
              │  dub-pp-mcp   │  │ espn-pp-mcp   │  │steam-web-pp-  │
              │  (subprocess) │  │ (subprocess)   │  │mcp (subproc)  │
              └───────────────┘  └───────────────┘  └───────────────┘
```

**Startup flow:**
1. Scan `~/printing-press/library/` for directories with `.printing-press.json`
2. Filter to `MCPReady != ""` entries with pre-built MCP binaries
3. For each: start MCP binary as subprocess, perform MCP handshake (`initialize` → response → `initialized`)
4. Send `tools/list` to each initialized subprocess, collect tool schemas
5. Register all tools in the mega MCP with `{normalized-slug}__` prefix
6. Register `library_info` and `about` meta-tools
7. Begin serving on stdio

**Tool call flow:**
1. Agent calls `dub__links_list` with arguments
2. Mega MCP extracts prefix `dub`, tool name `links_list`
3. Routes to the `dub` subprocess: sends `tools/call` with original tool name and arguments
4. Returns the subprocess response to the agent

## Implementation Units

### Phase 1: Directory Restructure

- [ ] **Unit 1: Change library directory key from CLI name to API slug**

  **Goal:** New CLIs land in `~/printing-press/library/{api-slug}/` instead of `~/printing-press/library/{api-slug}-pp-cli/`. Existing directories with either naming continue to be discovered.

  **Requirements:** R1, R2

  **Dependencies:** None

  **Files:**
  - Modify: `internal/pipeline/pipeline.go` (`DefaultOutputDir`)
  - Modify: `internal/cli/root.go` (lines 325-338: the `generate --spec` path renames output dir to `naming.CLI(apiSpec.Name)` — this would undo the slug-keyed change. Update `derivedDir` to use API slug instead of `naming.CLI()`)
  - Modify: `internal/cli/library.go` (`scanLibrary` inclusion criteria)
  - Modify: `internal/cli/emboss.go` (lines 196-211: library CLI resolution tries exact match then `naming.CLI(target)` — add `naming.TrimCLISuffix(target)` fallback for slug-keyed lookup)
  - Modify: `internal/naming/naming.go` (add `IsLibraryDir()` that accepts slug-keyed dirs)
  - Test: `internal/pipeline/pipeline_test.go`
  - Test: `internal/cli/library_test.go`
  - Test: `internal/naming/naming_test.go`

  **Not modified:** `internal/pipeline/paths.go` (`WorkingCLIDir`) — this constructs ephemeral working directories during generation runs, not published library paths. The working dir naming (`naming.CLI()`) is unchanged. Also not modified: `internal/pipeline/dogfood.go` and `internal/pipeline/runtime.go` — their `IsCLIDirName` calls scan `cmd/` subdirectories within CLIs (binary names), not library directories.

  **Approach:**
  `DefaultOutputDir` changes from `filepath.Join(PublishedLibraryRoot(), naming.CLI(apiName))` to `filepath.Join(PublishedLibraryRoot(), apiName)`. `scanLibrary` already has the `entry.APIName != ""` branch that accepts manifest-bearing directories regardless of naming — this becomes the primary inclusion path. `IsCLIDirName` check stays as a fallback for legacy directories without manifests (unlikely but safe). Add `IsLibraryDir(name)` to naming package that returns true for valid API slugs (no path separators, no `..`, non-empty).

  **This unit does NOT rename existing directories.** Old `dub-pp-cli/` directories continue to be discovered via backward-compat logic. Only newly published CLIs get slug-keyed directories. A `library migrate` command is deferred to a follow-up since backward-compat discovery means migration is cosmetic, not functional.

  **Critical: `generate --spec` rename logic.** At `internal/cli/root.go:325-338`, the `--spec` code path explicitly renames the output directory to `naming.CLI(apiSpec.Name)` after generation. This would undo the `DefaultOutputDir` change for every `--spec` run without explicit `--output`. Update `derivedDir` to use the API slug directly instead of `naming.CLI()`. The `--docs` code path at lines 148-200 does NOT have this rename logic and works correctly as-is.

  **Emboss library resolution.** At `internal/cli/emboss.go:196-211`, the emboss command resolves library CLIs by trying the exact target name, then `naming.CLI(target)`. After restructuring, `emboss dub-pp-cli` would try `naming.CLI("dub-pp-cli")` = `"dub-pp-cli-pp-cli"` (double-suffixed). Add a fallback: try `naming.TrimCLISuffix(target)` to resolve the slug.

  **Patterns to follow:**
  - `naming.IsCLIDirName()` pattern for the new `IsLibraryDir()`
  - `scanLibrary()` manifest-first discovery pattern
  - `PRINTING_PRESS_HOME` env var override in tests

  **Test scenarios:**
  - Happy path: `DefaultOutputDir("dub")` returns `~/printing-press/library/dub` (not `dub-pp-cli`)
  - Happy path: `scanLibrary` finds a CLI at `library/dub/` with a valid manifest
  - Happy path: `scanLibrary` finds a CLI at `library/dub-pp-cli/` with a valid manifest (backward compat)
  - Happy path: `IsLibraryDir("dub")` returns true, `IsLibraryDir("dub-pp-cli")` returns true
  - Edge case: `scanLibrary` ignores directories without manifests AND without CLI suffix (e.g., `library/.DS_Store/`)
  - Edge case: `IsLibraryDir("")` returns false, `IsLibraryDir("../etc")` returns false
  - Edge case: Rerun suffix directories (`library/dub-2/`) are discovered when they have manifests
  - Edge case: `ClaimOutputDir` works correctly with slug-keyed base path (append `-2`, `-3`)

  **Verification:** `go test ./internal/pipeline/... ./internal/cli/... ./internal/naming/...` passes. New CLIs published during a fullrun land in slug-keyed directories.

- [ ] **Unit 2: Update publish pipeline for slug-keyed paths**

  **Goal:** The publish pipeline writes to slug-keyed directories and updates module paths, branch names, and collision detection accordingly.

  **Requirements:** R9

  **Dependencies:** Unit 1

  **Files:**
  - Modify: `internal/pipeline/publish.go` (if any path construction uses `naming.CLI` for library dir)
  - Modify: `internal/pipeline/renamecli.go` (directory rename logic — now renames API slug as directory, binary names as content)
  - Modify: `internal/cli/publish.go` (the `publish package` command constructs `outCLIDir` using `cliName` — must use API slug from `manifest.APIName` instead; also `stashExistingCLI` search, `RewriteModulePath`, and module path construction)

  **Not modified in this plan:** `skills/printing-press-publish/SKILL.md` — the skill's branch naming (`feat/dub-pp-cli` → `feat/dub`) and registry entry paths reference the public library repo, which is out of scope. Updating the skill before the public repo accepts slug-keyed directories would cause the skill to produce broken PRs. The skill update belongs in the public library migration PR.

  **Not modified in this plan:** `internal/generator/templates/readme.md.tmpl` — the README template's `go install` path points at the public library repo, which is out of scope for this plan. Changing the template now would generate install paths referencing a directory layout (`library/<cat>/dub/...`) that doesn't exist in the public repo until the separate migration PR lands. The template change belongs in that PR.
  - Test: `internal/pipeline/renamecli_test.go`

  **Approach:**
  `PublishWorkingCLI` already delegates to `DefaultOutputDir` (updated in Unit 1), so the local publish path changes automatically. `RenameCLI` needs two changes: (1) **Validation:** Modify `validateRenameInputs()` at `renamecli.go:162-165` to accept names that pass either `IsCLIDirName()` or `IsLibraryDir()` (from Unit 1). Currently it strictly requires `-pp-cli`/`-cli` suffix and would reject slug-keyed names like `dub`. (2) **Inner logic:** `RenameCLI`'s inner logic derives `cmd/` subdirectory names from `naming.TrimCLISuffix(oldCLIName)`. When the outer directory is slug-keyed (e.g., `dub`), the function must still receive the CLI name (e.g., `dub-pp-cli`) as the `oldCLIName` parameter so inner `cmd/` directory resolution works correctly. The slug is the directory key; the CLI name is the rename parameter.

  **`internal/cli/publish.go` changes:** The `publish package` command at line ~290 constructs `outCLIDir = filepath.Join(dest, "library", category, cliName)` — this must change to use the API slug (from the manifest's `APIName` field) instead of `cliName`. The `stashExistingCLI` function searches by CLI name across categories — update to search by API slug. The `RewriteModulePath` call uses the directory path which now contains the slug. The `modulePath` flag's default also needs updating.

  The publish skill's collision detection and branch naming are deferred to the public library migration PR (see "Not modified" note above).

  **Patterns to follow:**
  - Existing `RenameCLI` test patterns in `renamecli_test.go`
  - README template auth-conditional blocks

  **Test scenarios:**
  - Happy path: `RenameCLI` renames a slug-keyed directory (e.g., `dub/` to `newname/`) and updates CLI/MCP binary names in content
  - Happy path: `RenameCLI` with slug-keyed directory correctly renames inner `cmd/` subdirectories when passed the CLI name as the rename parameter
  - Edge case: `RenameCLI` correctly handles the case where the old directory is CLI-name-keyed (backward compat during transition)

  **Verification:** `go test ./internal/pipeline/...` passes.

### Phase 2: Mega MCP Server

- [ ] **Unit 3: Add `mcp-go` dependency and create `internal/megamcp/` package with discovery**

  **Goal:** The mega MCP can scan the local library and build an in-memory catalog of all MCP-ready APIs with their metadata.

  **Requirements:** R3, R7

  **Dependencies:** Units 1-2 (slug-keyed paths make discovery cleaner, but the scanner works with both layouts)

  **Files:**
  - Modify: `go.mod` (add `github.com/mark3labs/mcp-go`)
  - Create: `internal/megamcp/discovery.go`
  - Create: `internal/megamcp/discovery_test.go`

  **Approach:**
  Add `mcp-go` as a direct dependency. Create the `internal/megamcp/` package with a `DiscoverAPIs(libraryRoot string) ([]APIEntry, error)` function. `APIEntry` contains:
  - `Slug string` — API slug derived from the manifest's `APIName` field (e.g., `"dub"`)
  - `Dir string` — absolute path to the library directory
  - `Manifest pipeline.CLIManifest` — the full parsed manifest
  - `MCPBinaryPath string` — absolute path to the pre-built MCP binary (resolved in Unit 5)
  - `NormalizedPrefix string` — slug normalized to snake_case for tool namespacing (e.g., `"steam_web"`)

  The `Manifest.MCPBinary` field (e.g., `"dub-pp-mcp"`) is set by `naming.MCP(apiName)` during generation. It matches the `cmd/{name}-pp-mcp/` source directory name exactly. This is guaranteed by the generator — both `cmd/` directory creation and manifest population use `naming.MCP()`.

  Discovery iterates `libraryRoot`, reads `.printing-press.json` from each directory (reusing `pipeline.CLIManifest` struct), and filters to entries where `MCPReady` is non-empty and `MCPBinary` is non-empty. Uses `errgroup.Group` (not `WithContext`) so a broken manifest doesn't block discovery of other CLIs. Applies path traversal validation from the institutional learning: reject directory names containing `..`, `/`, or `\`.

  `NormalizedPrefix` is computed from the slug: hyphens → underscores, consecutive underscores collapsed, validated to not contain `__` (which would collide with the tool name separator).

  **Binary resolution (merged from former Unit 4):** After manifest parsing, `ResolveMCPBinary(entry *APIEntry) error` searches for the pre-built MCP binary within the library directory. Search order: (1) `{dir}/cmd/{mcp-binary}/{mcp-binary}` (standard `go build` output), (2) `{dir}/{mcp-binary}` (Makefile output to project root). The `mcp-binary` name comes from `entry.Manifest.MCPBinary` (e.g., `dub-pp-mcp`), which is set by `naming.MCP(apiName)` during generation (Step 1, PR #145). This field is guaranteed to match the `cmd/{name}-pp-mcp/` directory name.

  `FilterReady(entries []APIEntry) (ready []APIEntry, warnings []string)` returns only entries with resolvable binaries. For each entry without a binary, it returns a warning message telling the user exactly how to build it: `cd ~/printing-press/library/{slug} && go build -o ./cmd/{name}-pp-mcp/{name}-pp-mcp ./cmd/{name}-pp-mcp`. This is deliberately minimal — no build cache, no `go build` invocation, no Go toolchain dependency.

  **Patterns to follow:**
  - `scanLibrary()` in `internal/cli/library.go` — directory iteration and manifest parsing pattern
  - `errgroup.Group` without context per multi-source discovery learning
  - `os.Stat` for binary existence checking
  - `PRINTING_PRESS_HOME` env var in tests

  **Test scenarios:**
  - Happy path: Library with 3 CLIs (full, partial, cli-only readiness) → returns 2 entries (full + partial), excludes cli-only
  - Happy path: CLI in slug-keyed directory (`library/dub/`) discovered correctly
  - Happy path: CLI in CLI-name-keyed directory (`library/dub-pp-cli/`) discovered correctly
  - Happy path: API slug `steam-web` → NormalizedPrefix `steam_web`
  - Happy path: Binary exists at `cmd/{name}-pp-mcp/{name}-pp-mcp` → resolved
  - Happy path: Binary exists at project root `{name}-pp-mcp` → resolved
  - Edge case: Empty library → returns empty slice, no error
  - Edge case: Library with a directory missing `.printing-press.json` → skipped with no error
  - Edge case: Library with a corrupt manifest JSON → skipped with warning, other CLIs still discovered
  - Edge case: Directory name contains `..` → rejected (traversal protection)
  - Edge case: Binary missing → excluded with actionable warning message
  - Edge case: Binary exists but is not executable → excluded with warning

  **Verification:** `go test ./internal/megamcp/...` passes. `go mod tidy` succeeds.

- [ ] **Unit 4: Subprocess manager**

  **Goal:** The mega MCP can start, monitor, and communicate with per-API MCP subprocesses over stdio.

  **Requirements:** R5, R6

  **Dependencies:** Unit 3

  **Files:**
  - Create: `internal/megamcp/subprocess.go`
  - Create: `internal/megamcp/subprocess_test.go`

  **Approach:**
  `SubprocessManager` manages the lifecycle of per-API MCP subprocesses. Each subprocess is a `*exec.Cmd` running the pre-built MCP binary with stdin/stdout piped for JSON-RPC communication.

  **First task in this unit:** Verify `mcp-go` client support. Check whether `mark3labs/mcp-go` provides `client.NewStdioMCPClient` or equivalent. If it does, use it — it will handle the MCP initialization handshake, request-response correlation, and JSON-RPC framing. If not, implement a lightweight MCP client (~200-300 lines) that handles: (a) the three-message initialization handshake (`initialize` request → server response → `initialized` notification), (b) JSON-RPC 2.0 request/response correlation via `id` field, (c) timeout per request.

  `Start(entry APIEntry) (*ManagedProcess, error)` starts the MCP binary with a credential-isolated environment (full parent env minus other APIs' `AuthEnvVars` per R6), then performs the MCP initialization handshake. The subprocess is not considered ready until the handshake completes successfully. `Stop(slug string)` sends SIGTERM and waits. `StopAll()` stops all subprocesses (called at mega MCP shutdown).

  `SendRequest(slug string, method string, params json.RawMessage) (json.RawMessage, error)` sends a JSON-RPC request to a subprocess and reads the response. Used for both `tools/list` (at startup) and `tools/call` (at runtime). Includes a timeout to prevent hanging on unresponsive subprocesses.

  **Concurrency model:** Per-subprocess communication is **multiplexed** using JSON-RPC `id` correlation. The `SubprocessManager` maintains a `map[int]chan json.RawMessage` of pending requests and a goroutine per subprocess reading stdout responses and dispatching them to the correct channel by `id`. This allows concurrent tool calls to the same API (~50 lines beyond what a serialized approach would need). Multiplexing is chosen over serialization because: (1) the mega MCP aggregates N APIs behind one connection — head-of-line blocking from a slow upstream API would stall all subsequent calls to that API, (2) the JSON-RPC id correlation is already needed for the MCP handshake, and (3) retrofitting multiplexing later requires rewriting the core I/O path.

  **Stdout reader dispatch logic:** The reader goroutine must handle three message types: (1) JSON-RPC responses with `id` field → dispatch to matching pending request channel, (2) JSON-RPC notifications without `id` field (MCP servers may send progress, logging, resource update notifications at any time) → discard or route to a notification handler, (3) non-JSON lines (Go panics, diagnostic output) → skip and log. Restart is protected by a per-subprocess mutex so concurrent crash detection results in exactly one restart attempt.

  If a subprocess has died when a tool call arrives, the manager attempts one restart (including re-running the MCP handshake). If the restart fails, the subprocess is excluded until the next mega MCP restart. Crash events are logged visibly to stderr with the subprocess name and timestamp. The manager does NOT silently retry the tool call that caused the crash — it returns an error to the agent. The next distinct tool call triggers the restart attempt.

  **Subprocess stderr:** Explicitly set `cmd.Stderr` to a per-subprocess log buffer or file — do NOT inherit the mega MCP's stderr (which would leak per-API diagnostic messages, potentially including credential-shaped strings, into the mega MCP's output stream). The stdout reader must handle non-JSON lines gracefully (skip and log) since Go panics and diagnostic output may appear on stdout despite best efforts.

  **Patterns to follow:**
  - `exec.Command` with `StdinPipe()` and `StdoutPipe()` for bidirectional communication
  - JSON-RPC 2.0 message format matching `mcp-go`'s wire protocol — verify framing (newline-delimited JSON vs LSP-style Content-Length headers) by reading `mcp-go`'s `ServeStdio` source at the start of this unit
  - MCP specification initialization handshake: `initialize` (client→server), response (server→client), `initialized` notification (client→server)

  **Test scenarios:**
  - Happy path: Start a subprocess, MCP handshake completes, send `tools/list`, receive tool list response
  - Happy path: Send `tools/call` with arguments, receive tool result
  - Happy path: Subprocess receives its own AuthEnvVars but NOT other APIs' AuthEnvVars (DUB_TOKEN present for Dub, STEAM_WEB_API_KEY absent; system/networking vars like PATH, HTTPS_PROXY preserved)
  - Happy path: Concurrent tool calls to same API are multiplexed (both in-flight simultaneously, responses matched by id)
  - Edge case: MCP handshake fails (subprocess returns error) → subprocess excluded with warning
  - Edge case: Subprocess crashes → crash logged to stderr, error returned to agent, next tool call triggers one restart attempt
  - Edge case: Subprocess unresponsive → request times out with error
  - Edge case: Stop all subprocesses → all processes terminated, no zombies
  - Edge case: Subprocess fails to restart → excluded from routing with warning
  - Integration: Full round-trip with a real (simple) MCP binary — verify initialization handshake + tool call correctness

  **Verification:** `go test ./internal/megamcp/...` passes. No zombie processes after test cleanup.

- [ ] **Unit 5: Tool aggregation and routing**

  **Goal:** The mega MCP discovers tools from all sub-MCPs, registers them with namespaced names, and routes incoming tool calls to the correct subprocess.

  **Requirements:** R5

  **Dependencies:** Unit 4

  **Files:**
  - Create: `internal/megamcp/router.go`
  - Create: `internal/megamcp/router_test.go`

  **Approach:**
  `ToolRouter` is initialized at startup. For each running subprocess, it sends `tools/list` and collects the tool schemas. Each tool is re-registered in the mega MCP with the name `{normalized-prefix}__{original-name}` (double underscore separator). The router maintains a mapping from prefixed tool name → (normalized prefix, original tool name).

  When a tool call arrives:
  1. Parse the prefix: split on `__` to extract the normalized prefix and original tool name
  2. Validate the prefix exists in the router
  3. Forward the call to the correct subprocess via `SendRequest("tools/call", ...)`
  4. Return the subprocess response to the agent

  **Tool name collision detection:** At registration time, validate that no prefixed tool name is ambiguous. The `__` separator could theoretically collide if an original tool name starts with a known prefix + `_` (e.g., API slug `user` + tool `accounts_list` → `user__accounts_list`, but tool `user_accounts_list` from a different API with slug `user` would produce the same prefixed name). The router validates at registration time that all prefixed names are unique and logs a warning for any collision. If a tool name itself contains `__`, the router splits on the first occurrence only.

  **Tool count awareness:** With 6 CLIs and ~270 tools, MCP clients may experience degraded tool selection. The plan ships with all tools registered at startup. If tool count becomes a UX problem at scale, a follow-up can implement lazy tool registration (register only meta-tools initially, dynamically register per-API tools on agent intent via a `use_api` meta-tool). This is documented as a known tradeoff.

  Traversal protection: the normalized prefix extracted from the tool name is validated against the known set of prefixes — it never flows into path construction.

  **Patterns to follow:**
  - `mcp-go` tool registration via `server.AddTool()`
  - Existing `makeAPIHandler` pattern in `mcp_tools.go.tmpl` for handler function signatures

  **Test scenarios:**
  - Happy path: Tools from 3 APIs registered with correct prefixes → `dub__links_list`, `espn__get_scores`, etc.
  - Happy path: Tool call `dub__links_list` routed to dub subprocess with original name `links_list`
  - Happy path: Tool call arguments forwarded correctly to subprocess
  - Edge case: Tool call with unknown prefix → returns MCP error "unknown API: xyz"
  - Edge case: Tool call with unknown tool name for a known API → forwards to subprocess (let the sub-MCP handle the error)
  - Edge case: Tool name containing `__` within the original name → split on first occurrence only
  - Edge case: Subprocess died → router returns error to agent, next distinct call triggers restart attempt

  **Verification:** `go test ./internal/megamcp/...` passes. End-to-end tool call routing works with mock subprocess.

- [ ] **Unit 6: `library_info` and `about` meta-tools**

  **Goal:** The mega MCP exposes discovery tools that let agents learn what APIs are available, their capabilities, and auth requirements.

  **Requirements:** R4, R6

  **Dependencies:** Unit 5

  **Files:**
  - Create: `internal/megamcp/metatools.go`
  - Create: `internal/megamcp/metatools_test.go`

  **Approach:**
  Two meta-tools registered directly in the mega MCP (not routed to subprocesses):

  **`library_info`**: Returns a JSON object listing all discovered APIs:
  ```
  {
    "apis": [
      {
        "slug": "dub",
        "name": "Dub",
        "description": "Short link management",
        "tool_count": 53,
        "public_tool_count": 0,
        "auth_type": "api_key",
        "auth_env_vars": ["DUB_TOKEN"],
        "mcp_ready": "full",
        "auth_configured": true,
        "novel_features": [...]
      },
      ...
    ],
    "total_tools": 270,
    "total_apis": 6
  }
  ```
  The `auth_configured` field checks `os.Getenv()` for each API's env vars — returning a boolean "ready" status per API without naming which specific env vars are or aren't set. This prevents `library_info` from serving as a credential reconnaissance tool if a prompt injection reaches the agent. The `auth_env_vars` array lists what's needed (from the manifest); `auth_configured` says whether it's satisfied.

  **`about`**: Returns metadata about the mega MCP itself — version, library root, total API count, total tool count. Matches the pattern of per-API `about` tools from Step 1.

  **Patterns to follow:**
  - Per-API `about` tool pattern from `mcp_tools.go.tmpl`
  - `CLIManifest` field access patterns

  **Test scenarios:**
  - Happy path: `library_info` returns all discovered APIs with correct metadata
  - Happy path: `library_info` shows `auth_configured: true` when env var is set
  - Happy path: `library_info` shows `auth_configured: false` when env var is missing
  - Happy path: `about` returns mega MCP version and summary stats
  - Edge case: No APIs discovered → `library_info` returns empty `apis` array with `total_tools: 0`

  **Verification:** `go test ./internal/megamcp/...` passes. Meta-tool responses are valid JSON.

- [ ] **Unit 7: Entry point, distribution, and setup**

  **Goal:** The mega MCP is buildable, distributable, and installable with a single `claude mcp add` command.

  **Requirements:** R8

  **Dependencies:** Unit 6

  **Files:**
  - Create: `cmd/printing-press-mcp/main.go`
  - Modify: `internal/version/version.go` (reuse for mega MCP version reporting)

  **Approach:**
  `main.go` creates an `mcp-go` server, runs the discovery + subprocess startup pipeline, registers the aggregated tools and meta-tools, and serves on stdio via `server.ServeStdio()`. Graceful shutdown on SIGINT/SIGTERM stops all subprocesses.

  **Initial distribution: `go install` only.** Do NOT add a goreleaser entry yet. The mega MCP requires per-API MCP binaries to be pre-built in the library, which only works when the user has Go installed (the `go install` path). Adding goreleaser would distribute a standalone binary to users who may not have Go, resulting in a mega MCP that silently excludes all APIs. Goreleaser distribution is a follow-up that depends on pre-building MCP binaries during the publish pipeline.

  Install and usage:
  ```
  # Install
  go install github.com/mvanhorn/cli-printing-press/cmd/printing-press-mcp@latest

  # Add to Claude Code
  claude mcp add printing-press -- printing-press-mcp

  # Add to Claude Desktop (config.json)
  { "mcpServers": { "printing-press": { "command": "printing-press-mcp" } } }
  ```

  The mega MCP reuses `internal/version.Version` for its version string so it stays in sync with the printing-press binary across releases.

  **Patterns to follow:**
  - `cmd/printing-press/main.go` entry point pattern
  - `main_mcp.go.tmpl` for `mcp-go` server setup

  **Test scenarios:**
  - Happy path: `go build ./cmd/printing-press-mcp` succeeds
  - Happy path: `printing-press-mcp --help` or version flag works (if supported by mcp-go server)
  - Integration: Start mega MCP with a test library containing 2 CLIs → `tools/list` returns all tools prefixed, `library_info` returns catalog

  **Verification:** `go build -o ./printing-press-mcp ./cmd/printing-press-mcp` succeeds. Binary starts and serves MCP on stdio.

## System-Wide Impact

- **Interaction graph:** The mega MCP introduces a new process tree: `printing-press-mcp` → N per-API MCP subprocesses. Claude Desktop/Code communicates with the mega MCP via stdio; the mega MCP communicates with sub-MCPs via stdio. The per-API MCP servers make HTTP requests to their respective APIs (unchanged).
- **Error propagation:** Subprocess errors (crashed process, build failure, timeout) are caught at the mega MCP layer and returned as MCP tool errors. Individual API failures do not crash the mega MCP or affect other APIs.
- **State lifecycle risks:** The subprocess manager holds process handles that must be cleaned up on shutdown. If the mega MCP crashes without cleanup, orphan processes may linger. Graceful shutdown handlers and process group management mitigate this.
- **API surface parity:** The mega MCP's tool list should exactly match the union of all per-API MCP tool lists (plus meta-tools). Any discrepancy means the discovery or registration logic has a bug.
- **Unchanged invariants:** Per-API MCP servers (`dub-pp-mcp`, `espn-pp-mcp`) continue to work independently. The mega MCP is additive — it does not modify or replace per-API MCP functionality. Users who prefer individual MCP connections can still use them.

## Risks & Dependencies

| Risk | Mitigation |
|------|------------|
| `mcp-go` lacks client-side support for communicating with sub-MCPs | Implement a JSON-RPC client with MCP handshake support (~200-300 lines). Verify `mcp-go` client support at start of Unit 6 before committing to custom implementation. |
| Pre-built MCP binaries not present in library | Clear error messages with exact build commands. Initially scoped to `go install` users (who have Go). Follow-up: pre-build during publish pipeline to support binary-only distribution. |
| Subprocess startup time for large libraries | With 6 CLIs, startup should be <10s. If the library grows to 50+, consider lazy subprocess startup (start on first tool call, not at boot). |
| Orphan processes on ungraceful shutdown | Primary mitigation: sub-MCPs detect parent death via stdin EOF (mcp-go's `ServeStdio` exits when stdin closes, which happens when the parent dies). Secondary: SIGINT/SIGTERM handler calls `StopAll()`. Known limitation: SIGKILL leaves no cleanup window — document that orphan processes are possible after hard kills. macOS has no `PR_SET_PDEATHSIG`; stdin EOF is the cross-platform mechanism. |
| Breaking `go install` for existing users after directory restructure | Old `go install .../dub-pp-cli/cmd/dub-pp-cli@latest` paths stop working. The migration PR in the public library repo must document this. Binary names are unchanged — only the directory component of the install path changes. |
| Directory restructure conflicts with in-flight publish PRs | Coordinate: ensure no publish PRs are open when the public library migration PR lands. The 6 existing CLIs are stable per the user's input. |

## Phased Delivery

### Phase 1: Directory Restructure (Units 1-2)
Can be shipped as its own PR. Unblocks Phase 2 but also stands alone as a correctness improvement (manuscripts already slug-keyed; library should match).

### Phase 2: Mega MCP Server (Units 3-7)
Depends on Phase 1 for clean discovery, but the mega MCP scanner works against either layout via backward-compat logic. Can be shipped as one PR or split into two (discovery/subprocess in one, routing/meta-tools/entry-point in another).

**Prerequisite for testing:** Existing library CLIs were generated before Step 1 (PR #145) and lack MCP metadata. At least 2-3 CLIs must be regenerated (or embossed) with the MCP readiness layer before the mega MCP can be validated against real data. Integration tests in Unit 7 should use both synthetic test fixtures and at least one real regenerated CLI.

### Follow-up (not in this plan)
- `library migrate` command — rename existing `dub-pp-cli/` directories to `dub/` (cosmetic since backward-compat discovery works without it)
- Public library repo migration (rename directories, update registry.json paths, update go.mod module paths)
- Goreleaser distribution of `printing-press-mcp` (depends on pre-built MCP binaries)
- Build MCP binaries at publish time (avoid runtime Go toolchain requirement, enable goreleaser distribution)
- Build-on-demand cache for MCP binaries (convenience for users who modify library source)
- MCP binary integrity verification — record SHA-256 hash in manifest at publish time, verify before execution at startup
- Lazy tool registration — register only meta-tools at startup, dynamically register per-API tools on agent intent (addresses tool count scaling at 50+ APIs)
- Lazy subprocess startup for large libraries (start on first tool call, idle timeout to reclaim memory)
- Live library watch (detect newly published CLIs without restart)
- Marketplace listing for the mega MCP

## Sources & References

- Related PR: mvanhorn/cli-printing-press#145 (Step 1: MCP readiness layer)
- Related plan: `docs/plans/2026-04-05-001-feat-mcp-readiness-layer-plan.md`
- Institutional learnings:
  - `docs/solutions/best-practices/checkout-scoped-printing-press-output-layout` (layout contract)
  - `docs/solutions/security-issues/filepath-join-traversal-with-user-input` (traversal protection)
  - `docs/solutions/best-practices/validation-must-not-mutate-source-directory` (immutable source)
  - `docs/solutions/best-practices/multi-source-api-discovery-design` (errgroup pattern)
