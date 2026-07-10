# PLAN — Kind unification (phase B): PostgreSQL DDL reads `Field.Type.Storage()`

> This plan is dispatched via the CodeJob workflow. See skill: agents-workflow.
> Phase B of `tinywasm/docs/KIND_UNIFICATION_MASTER_PLAN.md` (Kind unification wave). Requires
> the published phase-A `tinywasm/model`. Runs parallel to orm/form/sqlt/mcp.

## Context (zero-context summary)

Phase A changed `tinywasm/model`: `Field.Type` is no longer the `FieldType`
enum but the interface

```go
type Kind interface {
    Storage() FieldType   // the enum survives here — same values, same meaning
    Name() string
    Validate(value string) error
}
```

This repo's `translate.go` declares `postgresType(t model.FieldType) string`
and switches over the enum. That signature is **correct and stays** — the
enum remains the storage vocabulary. What changes is every call site that
feeds it (and any other direct `f.Type` comparison): they now pass
`f.Type.Storage()`.

## Stage 1 — mechanical migration

- Bump `tinywasm/model` to the phase-A version.
- `postgresType` keeps its `model.FieldType` parameter. Call sites pass
  `f.Type.Storage()`. Grep the module for `\.Type` and migrate every direct
  enum comparison; let the compiler flag the rest.
- Test fixtures building `model.Field` literals by hand switch to the
  phase-A base kind constructors (`Type: model.Text()`, `model.Int()`, …).

## Stage 2 — tests

- `gotest ./...` green with no weakened assertions: generated DDL for
  existing fixtures must be byte-identical to before the migration.

## Harness checklist (mandatory)

- No behavior change: call-site migration only. If the `Kind` contract is
  insufficient here, **STOP and report** to the master plan.
- No unrelated refactors; `gotest` only.
- Breaking dependency bump: next minor version.

## Acceptance criteria

1. Module compiles against phase-A model; all enum access goes through
   `.Storage()`; `postgresType` signature unchanged.
2. DDL output for existing fixtures unchanged; `gotest ./...` green.

## Stages

| Stage | File(s) | Action |
|---|---|---|
| 1 | `translate.go` call sites, `introspect.go` if flagged, test fixtures | `.Storage()` migration, base-kind literals |
| 2 | `tests/*_test.go` | DDL byte-identical regression |
