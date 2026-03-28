# CLI Printing Press - Development Conventions

## Build & Test

```bash
go build -o ./printing-press ./cmd/printing-press
go test ./...
```

**IMPORTANT: Always use relative paths for build output.** Never build to `/tmp` or any shared absolute path. Multiple worktrees run concurrently and will stomp on each other. Use `./printing-press` exactly as shown above.

## Project Structure

- `cmd/printing-press/` - CLI entry point
- `internal/spec/` - Internal YAML spec parser
- `internal/openapi/` - OpenAPI 3.0+ parser
- `internal/generator/` - Template engine + quality gates
- `internal/catalog/` - Catalog schema validator
- `catalog/` - API catalog entries (YAML)
- `skills/` - Claude Code skill definitions
- `testdata/` - Test fixtures (internal + OpenAPI specs)

## Commit Style

Conventional commits: `feat(scope):`, `fix(scope):`, `docs:`, `refactor:`

## Adding Catalog Entries

Catalog entries in `catalog/` must pass `internal/catalog` validation:
- Required fields: name, display_name, description, category, spec_url, spec_format, tier
- spec_url must use HTTPS
- category must be: auth, payments, email, developer-tools, project-management, communication, crm, example
- tier must be: official or community

## Testing

**When you change code, check for a `_test.go` file in the same package.** If one exists, read it — your change likely requires a test update. If tests fail after your change, investigate whether it's a bug in your code or a stale test — don't just delete.

Add tests for new non-trivial logic. Match the package's existing style (typically table-driven with `testify/assert`). Skip tests for CLI glue, trivial wrappers, and code only meaningfully tested via integration (`FULL_RUN=1`).

Run `go test ./...` before considering your work done.

## Quality Gates

Generated CLIs must pass 7 gates: go mod tidy, go vet, go build, binary build, --help, version, doctor.

## `~/printing-press/` Layout

Generated artifacts live under the user's home directory, not in this repo.

- `library/<cli-name>/` — Published CLIs (e.g., `notion-pp-cli`). Directory name matches the derived CLI name from `naming.CLI()`.
- `manuscripts/<api-slug>/` — Archived research and verification proofs, keyed by API slug (e.g., `notion`), not CLI name. One API can have multiple runs.
- `.runstate/<scope>/` — Mutable per-workspace state (current run, sync cursors). Scoped by repo basename + hash.

The API slug is derived by the generator from the spec title (`cleanSpecName`), not manually chosen. The CLI name is `<api-slug>-pp-cli`. Never hardcode an API slug when the generator can derive it — names with periods (cal.com, dub.co) normalize differently than you'd guess.

The `-pp-` infix exists to avoid colliding with official CLIs. `notion-pp-cli` can coexist with whatever `notion-cli` Notion ships themselves.
