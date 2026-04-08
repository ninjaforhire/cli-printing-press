---
date: 2026-04-06
topic: mega-mcp-generic-proxy
---

# Mega MCP: Generic HTTP Proxy for the Printing Press Library

## Problem Frame

Every printed CLI ships an MCP binary, but using them requires installing each one individually — `go install` of a long GitHub path, Go toolchain required, one `claude mcp add` per API. With 6 CLIs today and a growing catalog, this friction prevents adoption. Agents have no way to discover available APIs without the user manually configuring each MCP connection.

Step 1 (PR #145) built the data layer: MCP metadata in manifests and registry, per-endpoint auth awareness, self-describing `about` tools. This plan builds the access layer: a single MCP server binary that gives agents access to every API in the library through one install.

**The key insight:** the per-API MCP servers' core handler (`makeAPIHandler`) is a generic HTTP client — method, path template, positional params, query params. The mega MCP replicates this pattern at runtime using pre-computed tool manifests from the publish pipeline, rather than requiring pre-generated code or local binaries. The mega MCP is a **discovery and demo layer** — it covers the 95% CRUD case. Users who want the full experience (sync, search, SQL, adaptive rate limiting, cookie auth) install the per-API CLI/MCP individually.

## Requirements

**Distribution & Installation**

- R1. Single binary (`printing-press-mcp`) installable via `go install` and Homebrew. No local printing press library required. No Go toolchain required at runtime for Homebrew users.
- R2. One-command setup for Claude Code (`claude mcp add printing-press -- printing-press-mcp`) and Claude Desktop (single JSON config entry).
- R3. Marketplace-ready: the mega MCP includes `smithery.yaml` and documentation sufficient for listing on Smithery and the Claude marketplace as a single entry covering all library APIs. Exact marketplace requirements TBD in planning.

**Discovery & Activation**

- R4. At startup, fetch `registry.json` from the public library repo (`mvanhorn/printing-press-library`) on GitHub to discover available APIs and their metadata.
- R5. For each API, fetch and cache the **tools manifest** (`tools-manifest.json`) from the API's path in the public library repo. The manifest is generated at publish time and contains tool names, descriptions, parameter schemas (with location: path/query/body), auth config, base URL, and required headers. Cache at `~/.cache/printing-press-mcp/` with a 24h TTL. Verify cached manifests against the registry's `spec_checksum` field on each startup — re-fetch on mismatch. Graceful degradation: use cache if fetch fails; on first run with no cache and no network, start with zero APIs and surface the error through `library_info`.
- R6. Expose a `library_info` meta-tool listing all available APIs with: name, description, tool count, auth type, auth status (configured or not), novel features, and whether a richer per-API MCP exists locally. When a per-API MCP is available, include the install command. Does NOT expose specific env var names in the response — agents use `library_info` for discovery, and the `setup_guide` meta-tool for auth configuration details.
- R7. Expose a `setup_guide` meta-tool that returns auth configuration instructions for a specific API: required env vars, key URL, and example `claude mcp add --env` commands. Separated from `library_info` so credential-related metadata is behind a deliberate introspection step, not broadcast in the catalog listing.
- R8. Expose an `activate_api` meta-tool that dynamically registers all tools for a specific API by slug. By default, only meta-tools are registered at startup. The agent calls `library_info` → discovers APIs → calls `activate_api("espn")` → ESPN tools become available. This keeps the default tool count at ~4 (meta-tools only) and scales to any library size. Uses the MCP `tools/list_changed` notification to inform the client. A `deactivate_api` meta-tool unregisters tools for an API.
- R9. Expose a `search_tools` meta-tool that searches across all APIs' tool names and descriptions by keyword, returning matching tools with their API slug — without requiring activation first. Lets the agent find the right API for a task before activating it.

**Tool Registration & Routing**

- R10. Register MCP tools from the tools manifest (not by parsing OpenAPI specs at runtime). Tool names follow the existing `{snake_resource}_{snake_endpoint}` convention including sub-resource endpoints, prefixed with `{normalized_slug}__` (double underscore separator, slug hyphens → underscores). Registration-time collision detection: reject duplicate prefixed names with a warning.
- R11. When a tool is called, make the HTTP request directly to the API using a generic HTTP client. The handler classifies each parameter by location (from the manifest): **path** params for URL template substitution, **query** params appended to the URL, **body** params serialized as the JSON request body. Path and query params are excluded from the body. Attach RequiredHeaders (API version headers like `cal-api-version`, `Stripe-Version`) and per-endpoint HeaderOverrides from the manifest.
- R12. Startup latency: the mega MCP initializes meta-tools and loads manifests in under 5 seconds with cached data (parallel loading). First-run startup (no cache) targets <30 seconds.

**Authentication**

- R13. Auth handled via env vars configured through `claude mcp add --env` or Claude Desktop config. The mega MCP reads each API's auth config from the tools manifest — auth type, header name, format string (e.g., `Bearer {TOKEN}`), header-vs-query placement (`In` field), and env var names. At runtime: read the env var value, apply the format string substitution, set the result on the correct header or query param. Validate format strings against a safe pattern set (header/query substitution only, never path segment) before accepting them from the manifest.
- R14. Supported auth types: `api_key` (header or query), `bearer_token` (Authorization header), `oauth2` (same treatment as `bearer_token` — env var holds the token; interactive OAuth flows are out of scope), `none`. For `cookie`/`composed` auth types, only endpoints marked `NoAuth` in the manifest are registered (public endpoints work, auth-required endpoints are excluded with a description noting the limitation).
- R15. Credential isolation: env vars for one API are not used in requests to another API. Each API's HTTP client reads only its own `AuthEnvVars`. Startup-time collision check: if two APIs declare the same env var name, warn the user and require the API filter to separate them.
- R16. Auth-aware error handling: 401/403 responses return a generic error ("authentication not configured for this API — call `setup_guide` for instructions"). 429 responses surface the error body. Credentials never appear in error output or logs.

**Integrity & Security**

- R17. Spec integrity: verify fetched tools manifests against the registry's `spec_checksum` before accepting them. Reject manifests that fail verification and fall back to cache. Require all fetch URLs to use HTTPS.
- R18. Server URL validation: before registering tools from a manifest, validate that the API's base URL is HTTPS, not a private IP range (RFC 1918, loopback, link-local, cloud metadata), and resolves to a public address. Reject APIs with invalid base URLs with a warning.
- R19. Sanitize all manifest-derived text (tool descriptions, parameter descriptions) before registering as MCP tool metadata — strip control characters and length-limit to prevent prompt injection via malicious manifest content.

**Scaling & Filtering**

- R20. API filter: `PRINTING_PRESS_APIS=espn,dub` env var limits which APIs are available for activation. Default is all APIs in the registry.
- R21. The activation model (R8) keeps the default tool count at ~4-5 meta-tools regardless of library size. Tool count only grows when the agent explicitly activates APIs. This scales to any number of APIs without degrading agent tool selection.

## Success Criteria

- A user installs `printing-press-mcp` and runs `claude mcp add printing-press -- printing-press-mcp` with no other setup. Agent calls `library_info` and sees all available APIs.
- Agent calls `activate_api("espn")` → ESPN tools appear → agent calls `espn__scores_get` (no auth) and receives real ESPN data.
- Agent calls `search_tools("pizza")` → finds `pagliacci_pizza__stores_list` → calls `activate_api("pagliacci-pizza")` → calls the tool and gets store data.
- Agent calls `setup_guide("dub")` → gets auth instructions → user configures `DUB_TOKEN` → agent calls `dub__links_list` and receives real Dub data.
- Auth-required Pagliacci endpoints are not registered. Tool descriptions note "install pagliacci-pizza-pp-mcp for full access including ordering."
- The mega MCP works identically in Claude Code and Claude Desktop.
- Newly published APIs appear on restart without rebuilding the binary (consequence of R4's startup-time fetch).

## Scope Boundaries

- **In scope:** Generic HTTP proxy for all APIs in the public library registry. Marketplace metadata (`smithery.yaml`) for the mega MCP itself. Activation-based tool registration for agent-controlled scaling.
- **Out of scope:** Subprocess/hybrid mode (spawning per-API MCP binaries for richer features). This is a follow-up — the generic proxy is the v1.
- **Out of scope:** Per-API MCP binary distribution (goreleaser per API). The mega MCP IS the distribution strategy.
- **Out of scope:** Directory restructure of `~/printing-press/library/`. The mega MCP reads from GitHub, not local library.
- **Out of scope:** Novel features (sync, search, SQL), adaptive rate limiting, response caching — these require the per-API MCP binary. The mega MCP is a discovery/demo layer; tool descriptions surface when a richer per-API MCP exists with install instructions.
- **Out of scope:** Hot-reloading. A restart picks up registry changes.
- **Out of scope:** Interactive OAuth2 flows, cookie/composed auth beyond public endpoints.
- **Known limitation:** Cookie/composed auth APIs (Pagliacci) only expose public endpoints (~17% of tools). Tool descriptions note this and include upgrade instructions.

## Key Decisions

- **Tools manifest over runtime spec parsing:** The publish pipeline generates a `tools-manifest.json` per API containing pre-computed tool schemas (names, descriptions, parameters with location classification, auth config, base URL, required headers). The mega MCP reads these lightweight JSON files instead of fetching and parsing raw OpenAPI/internal/GraphQL specs at runtime. This eliminates: runtime kin-openapi dependency, spec-format concerns (OpenAPI vs internal YAML vs GraphQL), parser global mutable state, auth format re-derivation, and startup performance issues. The manifest is format-agnostic — it's always the output of whichever parser was used at generation time.
- **Activation model over all-at-once registration:** Only meta-tools (`library_info`, `setup_guide`, `activate_api`, `deactivate_api`, `search_tools`, `about`) are registered at startup. API tools are registered on demand via `activate_api`. This keeps the default tool count at ~6 regardless of library size, preventing agent tool selection degradation. Uses MCP's `tools/list_changed` notification to inform clients.
- **Discovery/demo layer with upgrade path:** The mega MCP is explicitly positioned as a discovery and demo layer — not the full per-API MCP experience. It covers CRUD operations via generic HTTP proxy. Tool descriptions surface when a richer per-API MCP exists locally (with install commands). `library_info` shows upgrade availability. This is honest framing that drives adoption of the full printing press stack.
- **Generic HTTP proxy over subprocess architecture:** The original plan (Step 2) used per-API MCP subprocesses. This required a local library with pre-built binaries, Go at runtime, and complex subprocess management. The generic proxy eliminates all of that — one binary, no local library, no runtime Go dependency for binary-distribution users.
- **Fetch from GitHub, our copies:** Tool manifests and registry live in the public library repo under our control. Spec URLs point to our copies (not third-party vendor URLs), ensuring stability and known formats. Integrity verified via checksums.
- **Cookie/composed auth: public endpoints only:** Rather than excluding cookie-auth APIs entirely, register their public endpoints. Pagliacci's store finder and menu browser work without auth. Tool descriptions note the limitation and include the per-API MCP install command for full access.
- **Mega MCP as the marketplace listing:** One Smithery/Claude marketplace entry for the aggregator, not one per API. Individual per-API `smithery.yaml` files still exist for standalone discovery, but the mega MCP is the primary install path.
- **Hybrid mode deferred:** The subprocess-based hybrid mode (detect local per-API binary → route through it for richer features) is explicitly deferred. v1 is pure generic proxy. The tool handler interface is designed so hybrid can slot in later: each API's handler is a function that takes a tool call and returns a result — swapping the generic HTTP handler for a subprocess handler is a per-API config change, not an architectural change.

## Prerequisites (must ship before or alongside the mega MCP)

- **Tools manifest generation in the publish pipeline:** The publish pipeline must generate `tools-manifest.json` per API at publish time, containing: tool names, descriptions, parameter schemas with location (path/query/body), auth config (type, header, format, in, env vars), base URL, required headers, and per-endpoint header overrides. This is a machine change to the printing press generator/publish flow.
- **Registry schema extension:** Add `spec_checksum`, `spec_format`, and `manifest_url` (pointing to `tools-manifest.json` in the repo) to `registry.json` entries. Update the publish skill to populate these from the CLI manifest.
- **Re-publish existing CLIs:** The 6 existing CLIs need to be re-published with tools manifests. This can be done as part of the prerequisite work.

## Dependencies / Assumptions

- The public library repo (`mvanhorn/printing-press-library`) has a `registry.json` with MCP metadata (added in Step 1, PR #145). The registry will be extended with `manifest_url` and `spec_checksum` per the prerequisites above.
- The tools manifest format is generated by the printing press publish pipeline and is format-agnostic (works for OpenAPI, internal YAML, GraphQL, and sniffed specs).
- Tool manifests live in the public library repo at the path indicated by each registry entry's `path` field: `{path}/tools-manifest.json`.
- The `mcp-go` library (`github.com/mark3labs/mcp-go`) must be added as a direct dependency in `go.mod`. It supports `tools/list_changed` notifications for dynamic tool registration.
- MCP marketplace listing (Smithery) accepts a `smithery.yaml` at the repo root level, not just per-directory.
- The deployment model is single-user local: the MCP server runs as a subprocess of Claude Code/Desktop, which is the only process that can reach its stdio. Multi-user or network-exposed deployments are out of scope.

## Outstanding Questions

### Deferred to Planning

- [Affects R3][Needs research] What are the exact requirements for Smithery marketplace listing? Does it need a Docker image, or can it reference a binary?
- [Affects R10][Technical] The `__` separator could theoretically collide with tool names containing double underscores. Validate against current APIs during implementation and confirm registration-time collision detection catches any issues.
- [Affects R20][Technical] Should the API filter support category-based filtering (e.g., `PRINTING_PRESS_CATEGORIES=sports,developer-tools`) in addition to slug-based?
- [Affects R8][Technical] Does `mcp-go` support the `tools/list_changed` notification? If not, the activation model may require the client to re-fetch tools manually (e.g., agent instructs user to "refresh tools" after activation).

## Next Steps

-> `ce:plan` for structured implementation planning. The old subprocess plan (`docs/plans/2026-04-06-002-feat-mega-mcp-aggregate-server-plan.md`) is superseded by this requirements doc. It remains as reference material for the deferred hybrid mode.
