---
title: "fix: Generator machinery — dry-run compatibility, import guards, type fidelity"
type: fix
status: active
date: 2026-04-04
---

# fix: Generator machinery — dry-run compatibility, import guards, type fidelity

## Overview

Three systemic generator bugs cause every printed CLI to lose scorecard points and fail verify dry-run checks. These were discovered during the cal.com CLI polish and confirmed against the Steam Run 2 retro findings. All three are machine-level fixes in the Go binary templates.

## Problem Frame

When polishing the cal.com CLI (94/100 Grade A), three categories of issues required manual fixes to 20+ files. Every future printed CLI will hit the same problems because the generator templates produce the problematic patterns.

1. **`MarkFlagRequired` blocks dry-run** — Cobra enforces required flags before RunE executes, so `--dry-run` never reaches the handler. Verify fails for every command with required params.
2. **Import guards penalize type fidelity** — `var _ = strings.ReplaceAll` and friends in promoted command templates cause the scorer to deduct 1 point from type fidelity.
3. **`goType()` maps object/array to string** — The type template calls `goType()` which has no case for `object` or `array`, so nested API response fields become `string` instead of `json.RawMessage`.

## Requirements Trace

- R1. Commands with required flags must still be runnable with `--dry-run`
- R2. Generated promoted commands must not contain `var _` import guard lines
- R3. Generated type structs must use `json.RawMessage` for object/array fields, not `string`
- R4. Existing tests must continue to pass; new tests must cover each fix

## Scope Boundaries

- OAuth2 browser flow generation is out of scope (tracked separately)
- Dogfood false-positive detection for nested sub-resources is out of scope (separate machine issue)
- No changes to the scorecard — it already correctly rewards the right patterns

## Context & Research

### Relevant Code and Patterns

- `internal/generator/templates/command_endpoint.go.tmpl` lines 281-290 — emits `MarkFlagRequired`
- `internal/generator/templates/command_promoted.go.tmpl` lines 16-20 — emits `var _` guards
- `internal/generator/templates/command_promoted.go.tmpl` lines 143-145 — emits `MarkFlagRequired` for promoted commands
- `internal/generator/generator.go` `goType()` lines 775-788 — type mapping with no object/array case
- `internal/generator/templates/types.go.tmpl` line 8 — calls `goType(.Type)` for every struct field
- `internal/spec/spec.go` `Param.Required` field — drives the `MarkFlagRequired` decision
- `internal/generator/generator_test.go` — 30+ existing tests for generated output

### Institutional Learnings

- Steam Run 2 retro (`docs/retros/2026-03-31-steam-run2-retro.md`) flagged the import guard issue as priority #5 with "Fix the generator" recommendation
- Same retro flagged ID params as StringVar vs IntVar — adjacent but separate issue

## Key Technical Decisions

- **Move validation to RunE instead of removing required enforcement entirely**: Required params should still error when `--dry-run` is not set. The fix moves the check to RunE where `flags.dryRun` is accessible, not removes it.
- **Remove import guards rather than making imports conditional**: Promoted commands only generate GET endpoints. The `io`, `os`, and `strings` imports are provably unused. Remove both the guards and the unused imports from the promoted template.
- **Use `json.RawMessage` for non-scalar types, not typed structs**: Generating full Go structs for every nested object requires deep schema traversal. `json.RawMessage` preserves JSON fidelity without that complexity and is the standard Go pattern for "pass-through JSON."

## Open Questions

### Resolved During Planning

- **Should dry-run provide default values for required params?** No — the template fix should only skip the validation error. The client's existing dry-run handler already prints the request without sending it. If a required param is empty, it shows as empty in the dry-run output, which is correct.
- **Do endpoint commands (not just promoted) also have import guards?** No — `command_endpoint.go.tmpl` does not emit `var _` guards. Only `command_promoted.go.tmpl` does.

### Deferred to Implementation

- Whether `types.go.tmpl` needs an `import "encoding/json"` conditional or can always import it — depends on whether any spec produces zero object/array fields (unlikely but check during implementation)

## Implementation Units

- [ ] **Unit 1: Replace MarkFlagRequired with RunE validation in endpoint template**

**Goal:** Commands with required flags accept `--dry-run` without error

**Requirements:** R1

**Dependencies:** None

**Files:**
- Modify: `internal/generator/templates/command_endpoint.go.tmpl`
- Test: `internal/generator/generator_test.go`

**Approach:**
- Remove the `MarkFlagRequired` emission blocks (lines 281-283 for params, 288-290 for body)
- Add a validation block at the start of RunE (after positional args check, before `flags.newClient()`):
  - For each required param/body field, check if the value equals zero-value AND `!flags.dryRun`
  - If so, return `fmt.Errorf("required flag \"%s\" not set", flagName)`
- Use existing template helpers `zeroValForParam` and `flagName` to generate the checks

**Patterns to follow:**
- Existing positional args validation pattern at lines 41-44 of the same template
- The `usageErr()` helper already exists in generated CLIs

**Test scenarios:**
- Happy path: Generated command with required param — source contains `fmt.Errorf("required flag"` in RunE, does NOT contain `MarkFlagRequired`
- Happy path: Generated command with no required params — no validation block emitted
- Edge case: Command with both required params and required body fields — both get RunE validation
- Integration: Generated output compiles (`go vet`) with the new validation pattern

**Verification:**
- `go test ./internal/generator/ -run TestGenerate` passes
- Generated test fixture has no `MarkFlagRequired` calls
- Generated test fixture has RunE validation guarded by `!flags.dryRun`

---

- [ ] **Unit 2: Replace MarkFlagRequired with RunE validation in promoted template**

**Goal:** Promoted commands with required flags accept `--dry-run` without error

**Requirements:** R1

**Dependencies:** Unit 1 (same pattern)

**Files:**
- Modify: `internal/generator/templates/command_promoted.go.tmpl`
- Test: `internal/generator/generator_test.go`

**Approach:**
- Same pattern as Unit 1, applied to the promoted template
- Remove `MarkFlagRequired` at lines 143-145
- Add RunE validation after the promoted command's own positional check

**Patterns to follow:**
- Unit 1's approach — identical pattern, different template file

**Test scenarios:**
- Happy path: Generated promoted command with required params has RunE validation, not `MarkFlagRequired`
- Integration: `TestGeneratedOutput_PromotedCommandCompiles` still passes

**Verification:**
- Existing promoted command tests pass
- Generated promoted command source has no `MarkFlagRequired`

---

- [ ] **Unit 3: Remove import guards from promoted template**

**Goal:** Promoted commands don't emit `var _` import guard lines

**Requirements:** R2

**Dependencies:** None (can be done in parallel with Units 1-2)

**Files:**
- Modify: `internal/generator/templates/command_promoted.go.tmpl`
- Test: `internal/generator/generator_test.go`

**Approach:**
- Remove lines 16-20 (`var _ = strings.ReplaceAll`, `var _ = fmt.Sprintf`, etc.)
- Remove unused imports from the template's import block: `io`, `strings` (promoted commands only use GET endpoints so these are provably unused)
- Keep `encoding/json`, `fmt`, `os`, `github.com/spf13/cobra` — all used by promoted GET commands

**Patterns to follow:**
- `command_endpoint.go.tmpl` — imports only what it uses with conditional blocks

**Test scenarios:**
- Happy path: Generated promoted command source does NOT contain `var _ =` lines
- Happy path: Generated promoted command source does NOT import `"io"` or `"strings"`
- Integration: `TestGeneratedOutput_PromotedCommandCompiles` still passes — generated code compiles without the guards

**Verification:**
- No `var _` in generated promoted command output
- `go vet` passes on generated output

---

- [ ] **Unit 4: Map object/array types to json.RawMessage in goType()**

**Goal:** Generated type structs use `json.RawMessage` for nested objects and arrays

**Requirements:** R3

**Dependencies:** None (can be done in parallel with Units 1-3)

**Files:**
- Modify: `internal/generator/generator.go` (`goType()` function)
- Modify: `internal/generator/templates/types.go.tmpl` (add conditional `encoding/json` import)
- Test: `internal/generator/generator_test.go`

**Approach:**
- Add cases for `"object"` and `"array"` in `goType()` that return `"json.RawMessage"`
- In `types.go.tmpl`, add `import "encoding/json"` — either unconditionally (since most specs have at least one object field) or behind a conditional that checks if any field uses `json.RawMessage`
- The types template currently has no import block at all (line 4 is `package types`); add one

**Patterns to follow:**
- `goStoreType()` at line 798 already maps JSON columns to `json.RawMessage`
- The store types template includes the `encoding/json` import

**Test scenarios:**
- Happy path: `goType("object")` returns `"json.RawMessage"`
- Happy path: `goType("array")` returns `"json.RawMessage"`
- Happy path: `goType("string")` still returns `"string"` (regression check)
- Happy path: Generated types file includes `import "encoding/json"` when object fields exist
- Edge case: Spec with only primitive fields — types file still compiles (import may be unused; verify whether this is possible or if every spec has at least one object field)

**Verification:**
- `go test ./internal/generator/ -run TestGenerate` passes
- Generated types fixture uses `json.RawMessage` for object/array fields

---

- [ ] **Unit 5: Add targeted tests for all three fixes**

**Goal:** Regression tests that prevent these issues from returning

**Requirements:** R4

**Dependencies:** Units 1-4

**Files:**
- Modify: `internal/generator/generator_test.go`

**Approach:**
- Add `TestGeneratedOutput_NoMarkFlagRequired` — generates from a spec with required params, asserts no `MarkFlagRequired` in output, asserts RunE validation present
- Add `TestGeneratedOutput_PromotedNoImportGuards` — generates promoted command, asserts no `var _ =` in output
- Add `TestGeneratedOutput_ObjectFieldsUseRawMessage` — generates types from a spec with object/array fields, asserts `json.RawMessage` in output, asserts no `string` for known object fields

**Patterns to follow:**
- Existing `TestGeneratedOutput_HasSelectFlag` pattern — generate, read output, assert string presence/absence

**Test scenarios:**
- Each test should both assert the fix IS present and assert the old pattern IS NOT present (belt and suspenders)

**Verification:**
- `go test ./internal/generator/ -count=1` passes with new tests
- `go test ./... -count=1` all green

## System-Wide Impact

- **Interaction graph:** All three templates feed into the generator pipeline. Changes affect every future `printing-press generate` run.
- **Error propagation:** RunE validation errors propagate the same way as `MarkFlagRequired` errors — through cobra's error handler. User-visible error messages will be identical.
- **State lifecycle risks:** None — templates are stateless text generation.
- **API surface parity:** The `import.go.tmpl` also has a `MarkFlagRequired` for `--input` (line 96). This is fine — import is not a verify-tested command and the required input file flag should always error without the flag.
- **Unchanged invariants:** Non-dry-run behavior is identical. Required flags still error when missing. Only the dry-run path changes.

## Risks & Dependencies

| Risk | Mitigation |
|------|------------|
| Types import block breaks specs with zero object fields | Unlikely — every real API spec has object response types. Test with a minimal all-primitives fixture to verify. |
| RunE validation error message differs from cobra's built-in | Use the same `"required flag \"%s\" not set"` format cobra uses internally. |
| Promoted template removes an import that IS conditionally used | Verified: promoted commands only generate GET endpoints (generator.go line 1189 filters `Method != "GET"`). `io` and `strings` are provably unused. |

## Sources & References

- Steam Run 2 retro: `docs/retros/2026-03-31-steam-run2-retro.md` (items #5 and #4)
- Cal.com CLI polish session (2026-04-04) — discovered all three issues during manual fix-up
- Cobra required flags docs: enforcement happens in PreRunE, before RunE
