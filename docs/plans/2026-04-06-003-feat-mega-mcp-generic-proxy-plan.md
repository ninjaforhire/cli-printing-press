---
title: "feat: Mega MCP — Generic HTTP Proxy with Activation Model"
type: feat
status: active
date: 2026-04-06
origin: docs/brainstorms/2026-04-06-mega-mcp-generic-proxy-requirements.md
---

# feat: Mega MCP — Generic HTTP Proxy with Activation Model

## Overview

A single MCP server binary (`printing-press-mcp`) that gives agents access to every API in the printing press library through one install. The mega MCP fetches pre-computed tool manifests from the public library repo on GitHub, registers tools on demand via an activation model, and proxies HTTP requests directly to APIs — no local library, no subprocess management, no Go toolchain at runtime.

This plan has two phases:
1. **Publish pipeline prerequisites** — generate `tools-manifest.json` per API at publish time, extend the registry schema
2. **Mega MCP binary** — new binary that fetches manifests, registers meta-tools, proxies HTTP calls

## Problem Frame

Every printed CLI ships an MCP binary, but each requires individual installation via `go install` of a long GitHub path. With 6 CLIs and a growing catalog, this friction prevents adoption. The mega MCP reduces setup to one `claude mcp add` command for all APIs. (see origin: `docs/brainstorms/2026-04-06-mega-mcp-generic-proxy-requirements.md`)

## Requirements Trace

**Distribution:** R1 (single binary), R2 (one-command setup), R3 (marketplace-ready)
**Discovery:** R4 (registry fetch), R5 (manifest fetch + cache), R6 (library_info), R7 (setup_guide), R8 (activate_api), R9 (search_tools)
**Routing:** R10 (tool registration from manifest), R11 (generic HTTP handler with param classification), R12 (startup latency)
**Auth:** R13 (format string expansion), R14 (auth type support), R15 (credential isolation), R16 (error handling)
**Security:** R17 (integrity verification), R18 (URL validation), R19 (text sanitization)
**Scaling:** R20 (API filter), R21 (activation model scaling)

## Scope Boundaries

- **In scope:** Generic HTTP proxy, activation model, manifest generation, registry extension, marketplace metadata
- **Out of scope:** Subprocess/hybrid mode, novel features (sync/search/SQL), directory restructure, hot-reloading, interactive OAuth, adaptive rate limiting
- **Known limitation:** Cookie/composed auth APIs expose only public endpoints. Tool descriptions include per-API MCP install instructions for full access.

## Context & Research

### Relevant Code and Patterns

- **Publish pipeline:** `internal/pipeline/publish.go` — `writeCLIManifestForPublish()` has access to the parsed spec and writes `.printing-press.json`. The new `writeToolsManifest()` goes alongside it at the same call sites: `PublishWorkingCLI()` and `PromoteWorkingCLI()`.
- **CLIManifest:** `internal/pipeline/climanifest.go` — already has `SpecChecksum`, `SpecFormat`, `MCPReady`, `AuthType`, `AuthEnvVars`. `populateMCPMetadata()` at line 104 shows how to compute MCP fields from a parsed spec.
- **MCP tool registration:** `internal/generator/templates/mcp_tools.go.tmpl` — `RegisterTools()` iterates `Resources/SubResources/Endpoints`, registers each via `s.AddTool()`. Tool names: `{{snake $name}}_{{snake $eName}}`. `makeAPIHandler()` at line 148 is the generic HTTP handler pattern.
- **Auth config:** `internal/spec/spec.go` `AuthConfig` struct — `Type`, `Header`, `Format`, `In`, `EnvVars`, `KeyURL`. Generated `config.go.tmpl` expands format strings via `applyAuthFormat()`.
- **RequiredHeaders:** `internal/spec/spec.go` `RequiredHeader` struct — `Name`, `Value`. Applied at `client.go.tmpl:368-370`. Per-endpoint `HeaderOverrides` at line 372.
- **HTTP client:** `internal/generator/templates/client.go.tmpl` — `do()` method handles auth injection, required headers, retries, error classification (401/403/429/5xx).
- **Parameter classification:** Implicit in templates — `Positional == true` means path param, others are query (GET) or body (POST/PUT/PATCH). `Endpoint.Params` vs `Endpoint.Body` in `spec.go:66-78`.
- **Naming:** `internal/naming/naming.go` — `CLI()`, `MCP()`. `toSnakeCase()` at `internal/openapi/schema_builder.go:362` handles hyphens (needed for slug normalization).
- **Registry schema:** `skills/printing-press-publish/SKILL.md` lines 566-587 — current schema has `name`, `category`, `api`, `description`, `path`, `mcp` block.
- **Goreleaser:** `.goreleaser.yaml` — single binary build. Needs second entry for `printing-press-mcp`.
- **Catalog embed:** `catalog/catalog.go` — `//go:embed *.yaml` pattern. Not needed for mega MCP (fetches at runtime) but informs the caching pattern.
- **Test patterns:** `setPressTestEnv(t)` for isolation, `t.TempDir()` for filesystem tests, table-driven with `testify/assert`.

### Institutional Learnings

- **Filepath traversal:** Belt-and-suspenders — reject `..`/`/`/`\` in inputs AND verify resolved path is under expected root. Applies to cache path construction from API slugs. (`docs/solutions/security-issues/filepath-join-traversal-with-user-input`)
- **Multi-source discovery:** Use `errgroup.Group` (not `WithContext`) for parallel manifest fetches. Injectable base URLs for testable HTTP clients. HTTPS-only enforcement. (`docs/solutions/best-practices/multi-source-api-discovery-design`)
- **Immutable source:** Cache writes via temp-then-rename to prevent partial reads on restart. (`docs/solutions/best-practices/validation-must-not-mutate-source-directory`)

## Key Technical Decisions

- **Tools manifest generated at publish time, not runtime spec parsing:** The publish pipeline writes `tools-manifest.json` alongside `.printing-press.json`. Contains tool names, descriptions, parameter schemas with location (path/query/body), auth config, base URL, required headers. Format-agnostic — works for OpenAPI, internal YAML, GraphQL, and sniffed specs. Eliminates runtime kin-openapi dependency and parser global state concerns.

- **Activation model with `tools/list_changed`:** Only ~6 meta-tools registered at startup. Agent calls `activate_api("espn")` → tools registered → `tools/list_changed` notification sent to client. If `mcp-go` v0.26+ doesn't support `tools/list_changed`, fall back to re-registering tools and documenting that the client may need to re-fetch. Verify at the start of Unit 5 implementation.

- **Auth format string expansion at runtime:** The manifest contains `auth.format` (e.g., `Bearer {DUB_TOKEN}`), `auth.header` (e.g., `Authorization`), `auth.in` (header/query). At runtime: read env var value, substitute into format string, set on the correct header or query param. Validate format strings at manifest load time — reject patterns that would place credentials in URL path segments.

- **Parameter classification from manifest, not guessed at runtime:** Each tool parameter in the manifest has an explicit `location` field: `path`, `query`, or `body`. The generic HTTP handler uses this directly — no heuristics needed. Path params substituted into URL template, query params appended to URL, body params serialized as JSON. This fixes the existing template bug where POST/PUT/PATCH sends path params in the body.

- **Two different snake functions for two different jobs:** Tool name segments use `toSnake()` from `generator.go:794` (CamelCase → snake_case only, does NOT convert hyphens). API slug prefixes use `toSnakeCase()` from `schema_builder.go:362` (also converts hyphens → underscores). `WriteToolsManifest` must use `toSnake` for resource/endpoint names (matching the MCP template's `{{snake}}` function) and `toSnakeCase` behavior for the slug prefix. The slug normalizer lives in `internal/megamcp/` as a private function (no second consumer justifies exporting to the naming package yet). Collapse consecutive underscores, reject if result contains `__`.

- **Credential isolation via separate HTTP clients:** Each API gets its own `http.Client` (or at minimum its own auth configuration). The generic handler reads only the env vars declared in that API's manifest `auth.env_vars`. Startup-time collision check warns if two APIs share an env var name.

## Open Questions

### Resolved During Planning

- **Where do tools manifests live?** → In the public library repo at `{path}/tools-manifest.json`, fetched via GitHub raw content URL. Our copies, not third-party URLs.
- **Which `toSnake` function for slug normalization?** → `toSnakeCase()` from `schema_builder.go:362` (handles hyphens). Export as `naming.SlugToToolPrefix()`.
- **Where does `writeToolsManifest()` go?** → In `internal/pipeline/publish.go`, called from the same sites as `writeCLIManifestForPublish()`: `PublishWorkingCLI()` and `PromoteWorkingCLI()`.
- **How does the mega MCP know param location (path/query/body)?** → Explicit `location` field in the tools manifest. Derived from `Param.Positional` (true = path) and `Endpoint.Body` (body params) at manifest generation time.

### Deferred to Implementation

- **`mcp-go` `tools/list_changed` support** — verify at start of Unit 5. If unsupported, the agent can instruct the user to refresh, or the mega MCP can return the tool list in the `activate_api` response text as a workaround.
- **Smithery marketplace listing requirements** — needs research. Does it need Docker, or can it reference a Go binary?
- **Category-based filtering** — whether `PRINTING_PRESS_CATEGORIES` is needed in addition to `PRINTING_PRESS_APIS`. Defer to user feedback.

## High-Level Technical Design

> *This illustrates the intended approach and is directional guidance for review, not implementation specification.*

```
User: claude mcp add printing-press -- printing-press-mcp
                          │
                          ▼
              ┌──────────────────────┐
              │  printing-press-mcp   │
              │  (Mega MCP Server)    │
              │                       │
              │  Startup:             │
              │  1. Fetch registry    │
              │  2. Fetch manifests   │◄── GitHub raw content
              │  3. Cache locally     │    (our copies)
              │  4. Register 6 meta-  │
              │     tools only        │
              │                       │
              │  Runtime:             │
              │  library_info → catalog│
              │  activate_api(espn) → │
              │    register tools +   │
              │    tools/list_changed  │
              │  espn__scores_get →   │
              │    generic HTTP GET   │──► https://site.api.espn.com/...
              │    to API             │◄── JSON response
              │    return to agent    │
              └──────────────────────┘
```

**Activation flow:**
1. Agent calls `library_info` → sees 6 APIs with descriptions and auth status
2. Agent calls `activate_api("espn")` → loads ESPN manifest, registers ~3 tools with `espn__` prefix
3. Agent calls `espn__scores_get` → mega MCP substitutes path params, makes GET request, returns response
4. Agent calls `setup_guide("dub")` → gets "Set DUB_TOKEN via `claude mcp add --env DUB_TOKEN=xxx`"

## Implementation Units

### Phase 1: Publish Pipeline Prerequisites

- [ ] **Unit 1: Generate tools-manifest.json at publish time**

  **Goal:** The publish pipeline writes a `tools-manifest.json` per API containing everything the mega MCP needs to register and execute tools — without runtime spec parsing.

  **Requirements:** R5, R10, R11, R13

  **Dependencies:** None

  **Files:**
  - Create: `internal/pipeline/toolsmanifest.go`
  - Modify: `internal/pipeline/publish.go` (call `WriteToolsManifest` from inside `writeCLIManifestForPublish` after spec parsing)
  - Test: `internal/pipeline/toolsmanifest_test.go`

  **Approach:**
  `WriteToolsManifest(dir string, parsed *spec.APISpec) error` iterates the same `Resources/SubResources/Endpoints` hierarchy the MCP template uses, with map keys sorted alphabetically for deterministic JSON output (critical for stable `manifest_checksum` values across re-publishes). For each endpoint, emits a tool entry with: name (same `snake(resource)_snake(endpoint)` convention), description (with `mcpDescription` minority-side annotations), method, path template, `no_auth` flag, and parameters with explicit `location` field.

  Parameter location is derived from existing spec data: `Param.Positional == true` → `"path"`, params from `Endpoint.Body` → `"body"`, all others → `"query"`. This makes explicit what the template currently leaves implicit.

  API-level metadata includes: `api_name`, `base_url` (from `spec.BaseURL`), full `auth` config (type, header, format, in, env_vars, key_url), `required_headers`, and `mcp_ready`.

  The function is called from a single site: inside `writeCLIManifestForPublish()` after the spec parsing block (after `publish.go:248`), where the `parsed` variable is available. Both `PublishWorkingCLI()` and `PromoteWorkingCLI()` flow through `writeCLIManifestForPublish`, so both paths get the tools manifest automatically. If `parsed` is nil (e.g., `--docs` mode, no spec.json), `WriteToolsManifest` is skipped — no manifest generated, and the API is invisible to the mega MCP.

  `SlugToToolPrefix` in the naming package converts API slugs to tool name prefixes: hyphens → underscores, consecutive underscores collapsed, rejects if result contains `__`.

  **Patterns to follow:**
  - `writeCLIManifestForPublish()` at `publish.go:176` — manifest writing pattern
  - `mcp_tools.go.tmpl` `RegisterTools()` — resource/endpoint iteration and tool naming
  - `mcpDescription()` template function in `generator.go` — minority-side auth annotation logic
  - `populateMCPMetadata()` at `climanifest.go:104` — spec-to-manifest data flow

  **Test scenarios:**
  - Happy path: OpenAPI spec with 3 resources, 10 endpoints → manifest has 10 tool entries with correct names, methods, paths
  - Happy path: Sub-resource endpoints included (`guild_members_list`)
  - Happy path: Each param has explicit `location` field — positional params marked `"path"`, body params marked `"body"`, others `"query"`
  - Happy path: Auth config serialized completely (type, header, format, in, env_vars, key_url)
  - Happy path: RequiredHeaders and per-endpoint HeaderOverrides included
  - Happy path: NoAuth endpoints flagged correctly
  - Happy path: `SlugToToolPrefix("steam-web")` → `"steam_web"`, `SlugToToolPrefix("dub")` → `"dub"`
  - Edge case: API with no auth (type "none") → auth block present but minimal
  - Edge case: Cookie/composed auth → only NoAuth endpoints in manifest tools list
  - Edge case: `SlugToToolPrefix("steam--web")` → `"steam_web"` (consecutive underscores collapsed)
  - Edge case: API with empty description → manifest still valid
  - Edge case: Round-trip: write manifest, read back, all fields preserved

  **Verification:** `go test ./internal/pipeline/... ./internal/naming/...` passes. A generated `tools-manifest.json` for the petstore test fixture contains all expected tools with correct parameter locations.

- [ ] **Unit 2: Extend registry schema and publish skill**

  **Goal:** Registry entries include `manifest_checksum`, `spec_format`, and `manifest_url` so the mega MCP can fetch and verify tools manifests.

  **Requirements:** R4, R5, R17

  **Dependencies:** Unit 1

  **Files:**
  - Modify: `skills/printing-press-publish/SKILL.md` (add fields to registry entry schema in Step 8)
  - Modify: `internal/pipeline/climanifest.go` (ensure `SpecChecksum` and `SpecFormat` are always populated when a spec is available)

  **Approach:**
  The registry entry's `mcp` block gains three fields:
  - `manifest_checksum`: SHA-256 hash of the `tools-manifest.json` file, computed at publish time. Distinct from `CLIManifest.SpecChecksum` which hashes the raw spec file — different artifacts, different checksums
  - `spec_format`: from `CLIManifest.SpecFormat` (openapi3, graphql, internal)
  - `manifest_url`: derived from the entry's `path` field: `{path}/tools-manifest.json`

  The publish skill's Step 8 (registry entry construction) is updated to include these fields. The skill reads them from the CLI manifest which is already written by this point in the pipeline.

  `manifest_url` is a relative path within the library repo, not a full URL. The mega MCP constructs the full GitHub raw content URL at runtime: `https://raw.githubusercontent.com/mvanhorn/printing-press-library/main/{manifest_url}`.

  **Patterns to follow:**
  - Existing registry schema extension pattern in `SKILL.md` Step 8
  - `specChecksum()` at `publish.go` for checksum computation

  **Test scenarios:**
  - Happy path: Publish skill Step 8 includes `manifest_checksum`, `spec_format`, `manifest_url` in the registry entry
  - Happy path: `manifest_url` is `library/<category>/<cli-name>/tools-manifest.json`
  - Edge case: CLI without a spec (--docs mode) → `manifest_checksum` and `manifest_url` empty, mega MCP skips this API

  **Verification:** A simulated publish produces a registry entry with all three new fields populated.

### Phase 2: Mega MCP Binary

- [ ] **Unit 3: Skeleton, registry fetching, and manifest caching**

  **Goal:** The mega MCP binary starts, fetches the registry from GitHub, fetches and caches tools manifests, and verifies integrity.

  **Requirements:** R1, R4, R5, R12, R17, R18, R19, R20

  **Dependencies:** Units 1-2

  **Files:**
  - Modify: `go.mod` (add `github.com/mark3labs/mcp-go`)
  - Create: `cmd/printing-press-mcp/main.go`
  - Create: `internal/megamcp/registry.go` (registry fetcher)
  - Create: `internal/megamcp/manifest.go` (manifest fetcher + cache)
  - Create: `internal/megamcp/types.go` (ToolsManifest, ToolEntry, ParamEntry structs)
  - Create: `internal/megamcp/security.go` (URL validation, text sanitization, checksum verification)
  - Test: `internal/megamcp/registry_test.go`
  - Test: `internal/megamcp/manifest_test.go`
  - Test: `internal/megamcp/security_test.go`

  **Approach:**
  `FetchRegistry(baseURL string) ([]RegistryEntry, error)` fetches `registry.json` from GitHub raw content. `baseURL` is injectable for testing (per multi-source discovery learning). Filters to entries with non-empty `manifest_url` and `mcp_ready != "cli-only"`. Applies `PRINTING_PRESS_APIS` env var filter (R20).

  `FetchManifest(entry RegistryEntry, cacheDir string) (*ToolsManifest, error)` fetches `tools-manifest.json` from the manifest_url. Cache logic: check cache → if cached, verify checksum against registry's `manifest_checksum` → if match, use cache → if mismatch or missing, fetch from GitHub → verify checksum → write to cache via temp-then-rename. Cache dir: `~/.cache/printing-press-mcp/manifests/{slug}/tools-manifest.json` with `0700` dir permissions, `0600` file permissions.

  Security: `ValidateBaseURL(url string) error` rejects non-HTTPS, private IP ranges (10.x, 172.16-31.x, 192.168.x), loopback (127.x, ::1), link-local (169.254.x, fe80::), and cloud metadata (169.254.169.254). `SanitizeText(s string, maxLen int) string` strips control characters and length-limits manifest-derived descriptions. `VerifyChecksum(data []byte, expected string) error` compares SHA-256.

  Path traversal protection on cache paths: reject API slugs containing `..`, `/`, `\` AND verify resolved cache path is under cache root.

  Parallel manifest loading via `errgroup.Group` (not `WithContext`). Failed fetches log warnings and exclude the API — other APIs continue.

  First run with no cache and no network: start with zero APIs, surface error through `library_info`.

  **Patterns to follow:**
  - `setPressTestEnv(t)` pattern for test isolation
  - `httptest.NewServer` for mock GitHub responses
  - `errgroup.Group` per multi-source discovery learning
  - Temp-then-rename for cache writes per immutable source learning
  - Belt-and-suspenders path traversal per filepath traversal learning

  **Test scenarios:**
  - Happy path: Fetch registry with 3 entries → filter to 2 (one is cli-only) → fetch 2 manifests → both cached
  - Happy path: Cached manifest with matching checksum → no re-fetch
  - Happy path: Cached manifest with mismatched checksum → re-fetched and updated
  - Happy path: `PRINTING_PRESS_APIS=espn,dub` → only 2 APIs loaded
  - Edge case: GitHub returns 404 for registry → start with zero APIs, library_info shows error
  - Edge case: One manifest fetch fails, others succeed → failed API excluded with warning, others available
  - Edge case: First run, no cache, no network → zero APIs, clear error in library_info
  - Edge case: API slug with traversal characters (`../etc`) → rejected before path construction
  - Edge case: Manifest base_url is private IP (10.0.0.1) → API rejected with warning
  - Edge case: Manifest base_url is HTTP (not HTTPS) → API rejected with warning
  - Edge case: Manifest description contains control characters → sanitized before registration
  - Edge case: Cache directory doesn't exist → created with 0700 permissions

  **Verification:** `go test ./internal/megamcp/...` passes. `go build ./cmd/printing-press-mcp` succeeds. Binary starts and loads manifests from a mock server.

- [ ] **Unit 4: Generic HTTP handler with auth and param routing**

  **Goal:** The mega MCP can make HTTP requests to any API using the tools manifest data — correct auth headers, proper parameter classification, required headers.

  **Requirements:** R11, R13, R14, R15, R16

  **Dependencies:** Unit 3

  **Files:**
  - Create: `internal/megamcp/handler.go`
  - Create: `internal/megamcp/auth.go` (format string expansion, auth header construction)
  - Test: `internal/megamcp/handler_test.go`
  - Test: `internal/megamcp/auth_test.go`

  **Approach:**
  `MakeToolHandler(manifest *ToolsManifest, tool ToolEntry) server.ToolHandlerFunc` returns a handler that:
  1. Builds the URL: substitute `location: "path"` params into the path template, append `location: "query"` params as query string
  2. Builds the body: serialize `location: "body"` params as JSON (POST/PUT/PATCH only). Path and query params are excluded from the body.
  3. Constructs auth: read env var from `manifest.Auth.EnvVars`, apply `manifest.Auth.Format` string substitution (e.g., `Bearer {DUB_TOKEN}` → `Bearer abc123`), set on `manifest.Auth.Header` or query param based on `manifest.Auth.In`
  4. Attaches `RequiredHeaders` from manifest to every request
  5. Attaches per-tool `HeaderOverrides` if present
  6. **Fail-closed auth check:** Before making the request, if `auth.type` is not `"none"` and the required env var is absent or empty AND the tool's `no_auth` flag is false, return an MCP error immediately ("Authentication not configured — call `setup_guide` for instructions"). Only proceed without auth when `no_auth: true`.
  7. **Post-assembly URL validation:** After substituting path params and appending query params, verify the assembled URL's host still matches the validated base_url host. Reject if the host changed (prevents path-param-based SSRF). URL-encode path param values before substitution. Use `url.Values` for query param construction (prevents `&`/`#` injection).
  8. **Missing path param check:** Before making the request, verify all `{placeholder}` tokens in the path template were substituted. If any remain, return an MCP error listing the missing params.
  9. Makes the HTTP request with a per-API `http.Client` (30s timeout, response body capped at 10MB via `io.LimitReader`)
  10. Handles errors: 401/403 → generic "auth not configured" message (no env var names in error). 429 → surface response body. 4xx → return API error (with credential value redaction pass — replace known auth values with `[REDACTED]` before returning). 5xx/network → MCP error

  `ApplyAuthFormat(format string, envVars []string) (string, error)` expands `{PLACEHOLDER}` in the format string with env var values. Validates that the format string only contains recognized placeholders and doesn't place credentials in URL paths.

  Credential isolation: each API's handler reads only its own `Auth.EnvVars`. A per-API HTTP client is constructed at activation time (not shared across APIs).

  **Patterns to follow:**
  - `makeAPIHandler` in `mcp_tools.go.tmpl` — path substitution, query params, body construction, error handling
  - `AuthHeader()` in `config.go.tmpl` — format string expansion
  - `do()` in `client.go.tmpl` — required headers, error classification

  **Test scenarios:**
  - Happy path: GET request with path param → param substituted in URL, not in query or body
  - Happy path: GET request with query params → appended to URL
  - Happy path: POST request with body params → serialized as JSON body. Path params excluded from body
  - Happy path: API key auth with `In: "header"` → auth header set correctly
  - Happy path: API key auth with `In: "query"` → auth appended as query param
  - Happy path: Bearer token auth → `Authorization: Bearer {value}` header set
  - Happy path: RequiredHeaders attached to every request
  - Happy path: Per-tool HeaderOverrides applied
  - Happy path: No auth (type "none") → no auth header set
  - Edge case: Missing env var for auth on non-NoAuth endpoint → MCP error returned before request ("call setup_guide")
  - Edge case: Missing env var for auth on NoAuth endpoint → request proceeds without auth
  - Edge case: 401 response → generic error message, no env var names, credential values redacted from response body
  - Edge case: 429 response → error includes response body
  - Edge case: Network timeout → MCP error
  - Edge case: Response body exceeds 10MB → truncated with error
  - Edge case: Format string with unrecognized placeholder → rejected at load time
  - Edge case: Path param value containing `../` → URL-encoded before substitution, assembled URL host matches base_url
  - Edge case: Required path param missing from arguments → MCP error listing missing params
  - Edge case: Two APIs share an env var name → startup warning
  - Integration: Round-trip against `httptest.NewServer` — GET with params, POST with body, auth header verified, assembled URL host matches

  **Verification:** `go test ./internal/megamcp/...` passes. A test hitting a mock API server receives correctly formed requests with proper auth and parameters.

- [ ] **Unit 5: Meta-tools and activation model**

  **Goal:** The mega MCP exposes discovery and activation tools that let agents find APIs, get setup instructions, and dynamically register API tools.

  **Requirements:** R6, R7, R8, R9, R21

  **Dependencies:** Units 3-4

  **Files:**
  - Create: `internal/megamcp/metatools.go`
  - Create: `internal/megamcp/activation.go` (tool registration/deregistration state)
  - Test: `internal/megamcp/metatools_test.go`
  - Test: `internal/megamcp/activation_test.go`

  **Approach:**
  Six meta-tools registered at startup:

  **`library_info`**: Returns JSON listing all loaded APIs: name, description, tool count, auth type, auth status (checks `os.Getenv` for each API's env vars — returns boolean only, NOT env var names), `mcp_ready` level, novel features, and whether a local per-API MCP binary exists (checks `~/printing-press/library/{slug}/cmd/{mcp-binary}/` path). If local binary exists, includes install hint.

  **`setup_guide(api_slug)`**: Returns auth configuration instructions for a specific API: required env vars, key URL, example `claude mcp add --env` command. This is the deliberate introspection step for credential metadata — separated from `library_info` to avoid broadcasting env var names in catalog listings.

  **`activate_api(api_slug)`**: Loads the API's tools manifest (already cached from startup), registers all tools with `{normalized_slug}__` prefix via `s.AddTool()`. Sends `tools/list_changed` notification if `mcp-go` supports it. Returns confirmation with tool count.

  **`deactivate_api(api_slug)`**: Removes all tools for an API. Sends `tools/list_changed`.

  **`search_tools(query)`**: Searches across ALL loaded manifests' tool names and descriptions by keyword (case-insensitive substring match). Returns matching tools with API slug, tool name, and description — without requiring activation. Lets the agent find the right API before activating it.

  **`about`**: Returns mega MCP version (from `internal/version.Version`), total API count, total tool count across all manifests, and library root path.

  **First task in this unit:** Verify `mcp-go` support for `tools/list_changed` notification and dynamic tool add/remove. If `mcp-go` doesn't support removing tools after registration, the `deactivate_api` implementation may need to re-create the MCP server with the reduced tool set (heavier but functional).

  **Patterns to follow:**
  - Per-API `about` tool in `mcp_tools.go.tmpl` — handler pattern
  - `mcp-go` `server.AddTool()` API for dynamic registration
  - `version.Version` for version reporting

  **Test scenarios:**
  - Happy path: `library_info` returns all APIs with correct metadata, auth_configured reflects env var presence
  - Happy path: `library_info` shows local upgrade available when per-API MCP binary exists
  - Happy path: `setup_guide("dub")` returns env var names, key URL, example command
  - Happy path: `activate_api("espn")` → 3 ESPN tools registered with `espn__` prefix
  - Happy path: After activation, `espn__scores_get` is callable
  - Happy path: `deactivate_api("espn")` → ESPN tools removed
  - Happy path: `search_tools("scores")` → returns `espn__scores_get` match with description
  - Happy path: `search_tools("pizza")` → returns Pagliacci tools even before activation
  - Happy path: `about` returns version and counts
  - Edge case: `activate_api` with unknown slug → error "API not found"
  - Edge case: `activate_api` called twice for same API → idempotent (no duplicate tools)
  - Edge case: `search_tools` with no matches → empty results array
  - Edge case: `library_info` with no APIs loaded (network failure) → empty list with error message
  - Edge case: `setup_guide` for no-auth API → "No authentication required"

  **Verification:** `go test ./internal/megamcp/...` passes. Meta-tools return valid JSON responses.

- [ ] **Unit 6: Entry point, goreleaser, and marketplace metadata**

  **Goal:** The mega MCP binary is buildable, distributable via `go install` and goreleaser, and includes marketplace metadata.

  **Requirements:** R1, R2, R3

  **Dependencies:** Unit 5

  **Files:**
  - Modify: `cmd/printing-press-mcp/main.go` (wire everything together)
  - Modify: `.goreleaser.yaml` (add second binary build)
  - Create: `smithery.yaml` (repo-root level, for the mega MCP)
  - Modify: `internal/version/version.go` (reuse for mega MCP)

  **Approach:**
  `main.go` creates an `mcp-go` server, runs the registry fetch → manifest load → cache pipeline, registers the 6 meta-tools, and serves on stdio. Graceful shutdown on SIGINT/SIGTERM.

  Add `printing-press-mcp` as a second build in `.goreleaser.yaml` alongside the existing `printing-press` binary. Both distributed in the same release. The mega MCP binary reuses `internal/version.Version` for its version string.

  `smithery.yaml` at repo root describes the mega MCP for Smithery marketplace listing. No per-API env vars are required (they're all optional). Description highlights the catalog: "270+ tools across 6 APIs — sports scores, link management, pizza ordering, and more."

  Install and usage:
  ```
  go install github.com/mvanhorn/cli-printing-press/cmd/printing-press-mcp@latest
  claude mcp add printing-press -- printing-press-mcp
  ```

  **Patterns to follow:**
  - `cmd/printing-press/main.go` entry point pattern
  - `.goreleaser.yaml` existing build configuration
  - `main_mcp.go.tmpl` for `mcp-go` server setup (`server.NewMCPServer`, `server.ServeStdio`)
  - `smithery.yaml` generation pattern from `writeSmitheryYAML` in `publish.go`

  **Test scenarios:**
  - Happy path: `go build ./cmd/printing-press-mcp` succeeds
  - Integration: Start mega MCP with mock GitHub server → `library_info` returns API catalog → `activate_api` registers tools → tool call returns mock API response

  **Verification:** Binary builds and starts. End-to-end flow works with mock server: library_info → activate → tool call → response.

## System-Wide Impact

- **Interaction graph:** The mega MCP binary is a new artifact in this repo. It imports `internal/megamcp/` (new), `internal/naming/` (modified), `internal/pipeline/` (types only), and `internal/version/`. The publish pipeline gains `writeToolsManifest()`. The publish skill gains registry fields. No existing behavior changes — all additions.
- **Error propagation:** HTTP errors from APIs are caught in the handler and returned as MCP tool errors. Registry/manifest fetch failures are caught at startup and surfaced via `library_info`. Individual API failures do not affect other APIs.
- **State lifecycle risks:** The manifest cache at `~/.cache/printing-press-mcp/` persists across restarts. Stale cache is mitigated by checksum verification against the registry. Cache corruption is mitigated by temp-then-rename writes.
- **API surface parity:** Tool names match (both use `toSnake` on `spec.APISpec` keys). Tool parameter schemas intentionally diverge for POST/PUT/PATCH: the mega MCP registers body params separately (with `location: "body"`) and excludes path params from the request body. The per-API MCP template has a known bug where it sends path params in the body. This is an improvement, not a parity violation — a follow-up should fix the template to match. Agents switching from mega MCP to per-API MCP may notice different parameter lists for mutation endpoints.
- **Unchanged invariants:** Per-API MCP servers continue to work independently. The mega MCP is additive. The publish pipeline still generates all existing artifacts (`.printing-press.json`, CLI binary, MCP binary). `tools-manifest.json` is a new addition.

## Risks & Dependencies

| Risk | Mitigation |
|------|------------|
| `mcp-go` doesn't support `tools/list_changed` or dynamic tool removal | Verify at start of Unit 5. Fallback: return tool list in activation response text; re-create server for deactivation. |
| GitHub rate limits on raw content fetches | Cache aggressively (24h TTL + checksum-based invalidation). First-run fetches ~7 files (1 registry + 6 manifests). Well within anonymous rate limits. |
| Manifest format drift between publish pipeline and mega MCP | Both are in the same repo. `ToolsManifest` struct is shared. Test fixtures verify round-trip compatibility. |
| Tool naming mismatch between mega MCP and per-API MCP servers | Both derive tool names from `spec.APISpec` using the same naming convention. Unit 1 tests verify parity with template output. |
| Existing CLIs need re-publishing with tools manifests | Documented as a prerequisite. Can be batched as a single publish run after Units 1-2 land. |

## Phased Delivery

### Phase 1: Publish Pipeline Prerequisites (Units 1-2)
Ship as one PR. Additive only — doesn't change existing publish behavior, just generates an additional artifact. Can be validated by running a test publish and inspecting the `tools-manifest.json`.

### Phase 2: Mega MCP Binary (Units 3-6)
Ship as one or two PRs. Can be split: Units 3-4 (fetching + handler) in one PR, Units 5-6 (meta-tools + entry point) in another. The binary is usable only after all 4 units land.

### Post-ship
- Re-publish existing 6 CLIs with tools manifests (batch operation)
- List mega MCP on Smithery marketplace
- Announce via README update in the public library repo

## Sources & References

- **Origin document:** `docs/brainstorms/2026-04-06-mega-mcp-generic-proxy-requirements.md`
- **Superseded plan:** `docs/plans/2026-04-06-002-feat-mega-mcp-aggregate-server-plan.md` (subprocess architecture — reference for deferred hybrid mode)
- **Step 1 PR:** mvanhorn/cli-printing-press#145 (MCP readiness layer)
- **Step 1 plan:** `docs/plans/2026-04-05-001-feat-mcp-readiness-layer-plan.md`
- Institutional learnings:
  - `docs/solutions/security-issues/filepath-join-traversal-with-user-input` (traversal protection)
  - `docs/solutions/best-practices/multi-source-api-discovery-design` (errgroup, injectable URLs)
  - `docs/solutions/best-practices/validation-must-not-mutate-source-directory` (temp-then-rename)
