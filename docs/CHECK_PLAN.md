> This plan is dispatched via the CodeJob workflow. See skill: agents-workflow.
> Master plan: tinywasm/docs/MASTER_PLAN_SCHEMA_SQL_EXPORT.md — Fase C (parallel with sqlt)

# PLAN: postgres — Implement ddl.Exporter

## Precondition

`tinywasm/orm` Fase A must be published. The `github.com/tinywasm/orm/ddl` sub-package
must be available. Update `go.mod` to the published `orm` version before starting.

No changes needed in `tinywasm/fmt`: `fmt.Field` already embeds `Permitted.Maximum`.

## Context

`postgres` already generates full `CREATE TABLE IF NOT EXISTS` SQL in `translate.go`
(`buildCreateTable`, lines ~109-178) including BIGSERIAL, composite PKs, NOT NULL, UNIQUE,
and FK constraints via `SchemaExt()`.

This plan adds `ExportDDL` — calling `c.Compile(ActionCreateTable)` in a loop. Zero new SQL.

### Current compiler structure in postgres

The `postgres` package exposes `Translate(q orm.Query, m fmt.Model) (orm.Plan, error)` as a
standalone exported function (no receiver struct visible externally). Before adding `ExportDDL`,
confirm whether a `compiler` struct already exists internally. If not, create one.

### Interface to implement

```go
// From github.com/tinywasm/orm/ddl
type Exporter interface {
    ExportDDL(models []fmt.Model) (string, error)
}
```

---

## S0a — `VARCHAR(n)` support in `translate` (`postgres/translate.go`)

`fmt.Field` embeds `Permitted` which carries `Maximum int`. When `Maximum > 0` on a `FieldText`
field, emit `VARCHAR(n)` instead of `TEXT`. No new field on `fmt.Field` needed.

```go
func postgresColumnType(f fmt.Field) string {
    if f.Type == fmt.FieldText && f.Maximum > 0 {
        return fmt.Sprintf("VARCHAR(%d)", f.Maximum)
    }
    // AutoInc handling is done inline in the CREATE TABLE block — this helper
    // is only for non-autoinc non-PK type resolution.
    return postgresType(f.Type)
}
```

In the `ActionCreateTable` block of `translate`, replace the `postgresType(f.Type)` call
(the `else` branch at the end of the isPK/isAuto chain) with `postgresColumnType(f)`.

The existing `postgresType(t fmt.FieldType) string` function stays unchanged — still used
by `ActionAddColumn`.

### `TestVarchar_Postgres`
```go
// Field: username TEXT, Maximum=100
// Assert: translate output contains "username VARCHAR(100)"
// Assert: field without maximum: "email TEXT"
// Assert: int64 PK autoinc stays "BIGSERIAL" (maximum ignored for non-text types)
```

---

## S0b — `ON DELETE` on FK constraints (`postgres/translate.go`)

Same `onDeleteSQL` helper as sqlt (identical logic, identical default CASCADE). In the FK constraint block:

```go
fkSQL := fmt.Sprintf(", CONSTRAINT fk_%s_%s FOREIGN KEY (%s) REFERENCES %s(%s)",
    q.Table, f.Name, f.Name, f.Ref, refCol)
fkSQL += " ON DELETE " + onDeleteSQL(f.OnDelete)
sb.Write(fkSQL)
```

### `TestOnDelete_Postgres`
```go
// FK with OnDelete="" (default) → "ON DELETE CASCADE"
// FK with OnDelete="restrict"   → "ON DELETE RESTRICT"
// FK with OnDelete="set_null"   → "ON DELETE SET NULL"
```

---

## S0c — Auto-index on FK columns (in `ExportDDL`)

Postgres uses `CREATE INDEX IF NOT EXISTS` (supported since Postgres 9.3, 2013).

```go
// In ExportDDL, after emitting CREATE TABLE for a model:
if ext, ok := m.(interface{ SchemaExt() []orm.FieldExt }); ok {
    for _, f := range ext.SchemaExt() {
        if f.Ref != "" {
            buf.Write(fmt.Sprintf(
                "CREATE INDEX IF NOT EXISTS idx_%s_%s ON %s(%s);\n\n",
                m.ModelName(), f.Name, m.ModelName(), f.Name))
        }
    }
}
```

### `TestAutoIndex_Postgres`
```go
// sessions has user_id FK → users
// Assert: output contains "CREATE INDEX IF NOT EXISTS idx_sessions_user_id ON sessions(user_id)"
// Assert: index appears AFTER the CREATE TABLE for sessions
// tables without FK (users, roles) must NOT generate any CREATE INDEX
```

---

## S1 — Confirm or create `compiler` struct (`postgres/compiler.go`)

If `postgres` uses only a free function `Translate`, introduce a minimal struct:

```go
package postgres

import (
    "github.com/tinywasm/fmt"
    "github.com/tinywasm/orm"
)

type Compiler struct{}

func NewCompiler() *Compiler { return &Compiler{} }

func (c *Compiler) Compile(q orm.Query, m fmt.Model) (orm.Plan, error) {
    return Translate(q, m)   // delegates to existing exported Translate function
}
```

The existing `Translate` function remains unchanged — backward compatibility preserved.

---

## S2 — `ExportDDL` on `*Compiler` (`postgres/compiler.go`)

```go
import (
    "github.com/tinywasm/fmt"
    "github.com/tinywasm/orm"
    "github.com/tinywasm/orm/ddl"
)

func (c *Compiler) ExportDDL(models []fmt.Model) (string, error) {
    sorted, err := ddl.TopologicalSort(models)
    if err != nil {
        return "", err
    }
    var buf fmt.Builder
    buf.Write("-- dialect: postgres\n\n")
    for _, m := range sorted {
        plan, err := c.Compile(orm.Query{Action: orm.ActionCreateTable, Table: m.ModelName()}, m)
        if err != nil {
            return "", err
        }
        buf.Write(plan.Query)
        buf.Write(";\n\n")
        if ext, ok := m.(interface{ SchemaExt() []orm.FieldExt }); ok {
            for _, f := range ext.SchemaExt() {
                if f.Ref != "" {
                    buf.Write(fmt.Sprintf(
                        "CREATE INDEX IF NOT EXISTS idx_%s_%s ON %s(%s);\n\n",
                        m.ModelName(), f.Name, m.ModelName(), f.Name))
                }
            }
        }
    }
    return buf.String(), nil
}

var _ ddl.Exporter = (*Compiler)(nil)
```

---

## S3 — Tests (`postgres/tests/export_test.go` — new file)

### Fixture

The golden output lives at `postgres/tests/schema.sql`.
Read it with `os.ReadFile("schema.sql")` and compare with `strings.TrimSpace`.

The fixture covers all cases:
- `users`: int64 PK AUTOINC (→BIGSERIAL), string NOT NULL UNIQUE, float64 (→DOUBLE PRECISION),
           bool (→BOOLEAN), []byte (→BYTEA)
- `roles`: int64 PK AUTOINC, string NOT NULL UNIQUE
- `sessions`: string PK no autoinc (→TEXT PRIMARY KEY), int64 FK→users.id (→BIGINT)
- `user_roles`: composite PK (int64, int64 → BIGINT NOT NULL each) + two FKs (→users, →roles)

Input models must be passed in order `[users, roles, sessions, user_roles]` so
`TopologicalSort` emits them in that same order (users/roles have in-degree 0).

### `TestExportDDL_ImplementsInterface`
```go
var _ ddl.Exporter = (*Compiler)(nil)
```

### `TestExportDDL_FullSchema`
```go
// Load golden file from schema.sql
// Build stubs for [users, roles, sessions, user_roles] exactly as documented in fixture header
// Call c.ExportDDL([users, roles, sessions, user_roles])
// Compare strings.TrimSpace(got) == strings.TrimSpace(golden)
```

This test exercises: BIGSERIAL (int64 autoinc PK), TEXT PRIMARY KEY (string PK no autoinc),
NOT NULL UNIQUE, DOUBLE PRECISION/BOOLEAN/BYTEA types, single FK, composite PK + dual FK.

### `TestExportDDL_EmptyInput`
```go
// Input: nil / empty
// Assert: "-- dialect: postgres\n\n", no error
```

---

## Constraints

- RULE: No new SQL. `ExportDDL` only calls `c.Compile(ActionCreateTable)`.
- RULE: Existing `Translate` function stays exported and unchanged.
- RULE: No stdlib. Use `github.com/tinywasm/fmt`.
- RULE: `ddl.TopologicalSort` imported from `orm/ddl`, not reimplemented.
- RULE: Compile-time assertion `var _ ddl.Exporter = (*Compiler)(nil)` must be present.

## Stages summary

| Stage | File | Change |
|---|---|---|
| S0a | `postgres/translate.go` | Add `postgresColumnType(f fmt.Field)` → VARCHAR(n) |
| S0b | `postgres/translate.go` | Add `onDeleteSQL(action string)` + emit ON DELETE in FK constraint |
| S0t | `postgres/tests/postgres_translate_test.go` | `TestVarchar_Postgres`, `TestOnDelete_Postgres` |
| S1 | `postgres/compiler.go` (new or existing) | Add/confirm `Compiler` struct + `NewCompiler()` + `Compile` delegation |
| S2 | `postgres/compiler.go` | Add `ExportDDL` (con auto-index) + interface assertion |
| S3t | `postgres/tests/export_test.go` (new) | `TestAutoIndex_Postgres`, `TestExportDDL_FullSchema`, `TestExportDDL_EmptyInput`, `TestExportDDL_ImplementsInterface` |
