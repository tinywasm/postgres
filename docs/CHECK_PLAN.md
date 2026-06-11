# PLAN — Schema-sync support (Postgres adapter)

> `tinywasm/orm`'s dev schema-sync (`db.Sync` / `db.SyncSchema`) drives the engine through agnostic
> `Action`s and an optional `TableIntrospector`. This module is the **full Postgres adapter**
> (executor **and** compiler, [adapter.go](../adapter.go) + [translate.go](../translate.go)). It must:
> 1. **register** itself so `orm.Open("postgres://…")` resolves it,
> 2. map its driver's no-rows error to `orm.ErrNoRows`,
> 3. translate the new DDL actions (`ActionAddColumn`/`RenameColumn`/`DropColumn`),
> 4. handle `IS NULL`/`IS NOT NULL` conditions (the safe-drop check has no bind value), and
> 5. (for full reconcile) implement `TableIntrospector`.
>
> **Self-contained, single-module plan** (`tinywasm/postgres`). Prerequisite: `orm` published with the
> registry (`Open`/`Register`/`Factory`), `db.SyncSchema`, `orm.ErrNoRows`, and the three column
> actions (orm core plan). Bump the dep first.

---

## 1. Development Rules (constraints copied for execution context)

- **Pure translator.** `translate(q, m)` maps an `orm.Query` → `(sql, args, error)`. It does **not**
  execute or read the DB. Introspection lives in the **executor** (`PostgresAdapter`), not in
  `translate`.
- **Postgres dialect.** This module owns Postgres-specific SQL. Reuse `postgresType()`.
- **Idempotency is native.** `ADD COLUMN IF NOT EXISTS` / `DROP COLUMN IF EXISTS` → re-running
  `db.Sync` is a no-op at the SQL level.
- **Additive by default; rename/drop only with introspection.** `db.Sync` only emits
  `RenameColumn`/`DropColumn` when the executor satisfies `TableIntrospector`; otherwise it runs
  additive-only. So Tier 1 (no introspector) is safe and complete on its own; Tier 2 adds reconcile.
- **No `database/sql` leakage into `orm`.** The executor must map `sql.ErrNoRows` →
  `orm.ErrNoRows` (orm core rule); `orm.QB` compares against `orm.ErrNoRows`.
- **`gotest` (not `go test`).** `translate` is pure — assert the SQL string. The executor/introspector
  needs a real or mocked `*sql.DB`.
- **Documentation first.**

---

## 2. Problem

1. `orm.Open("postgres://…")` has nothing to resolve — the adapter never calls `orm.Register`.
2. `translate` ([translate.go:189](../translate.go#L189)) `default:`-errors on `ActionAddColumn`,
   `ActionRenameColumn`, `ActionDropColumn` — `db.Sync`'s per-column steps fail.
3. `buildConditions` ([translate.go:235](../translate.go#L235)) always emits `field op $N` with a bind
   arg. For the safe-drop check's `IS NOT NULL` condition (no value), this yields the broken
   `col IS NOT NULL $1`.
4. `PostgresAdapter.QueryRow` ([adapter.go:65](../adapter.go#L65)) returns `*sql.Row`, whose `Scan`
   returns `sql.ErrNoRows` — not the `orm.ErrNoRows` that `orm.QB` checks.
5. No `TableColumns` → `db.Sync` can never do reconcile (rename/drop), only additive.

---

## 3. Decision

### 3.1 Register (Tier 1)

```go
// adapter.go
func init() { orm.Register("postgres", New) }
```
`New` is already `func(dsn string) (*orm.DB, error)` — it matches `orm.Factory` exactly. Zero new
construction code.

### 3.2 Map no-rows error (Tier 1) — **on both the base and tx executors**

`PostgresAdapter.QueryRow` ([adapter.go:65](../adapter.go#L65)) **and** `PostgresTx.QueryRow`
([tx.go:35](../tx.go#L35)) both return the raw `*sql.Row`. Wrap **both** so `Scan` translates the
driver error (reads inside a sync/CRUD transaction leak `sql.ErrNoRows` otherwise):

```go
type errScanner struct{ s orm.Scanner }
func (e errScanner) Scan(dest ...any) error {
    if err := e.s.Scan(dest...); err != nil {
        if err == sql.ErrNoRows {
            return orm.ErrNoRows
        }
        return err
    }
    return nil
}

func (p *PostgresAdapter) QueryRow(query string, args ...any) orm.Scanner {
    return errScanner{p.db.QueryRow(query, args...)}
}
func (p *PostgresTx) QueryRow(query string, args ...any) orm.Scanner {
    return errScanner{p.tx.QueryRow(query, args...)}
}
```

### 3.3 Translate the column actions (Tier 1: Add · Tier 2: Rename/Drop)

Add cases to `translate`'s `switch` (additive, **nullable**, no PK/UNIQUE/SERIAL on alter):

```go
case orm.ActionAddColumn:
    if q.Column == nil || q.Table == "" {
        return "", nil, fmt.Err("table and column required for add column")
    }
    sb.Write("ALTER TABLE "); sb.Write(q.Table)
    sb.Write(" ADD COLUMN IF NOT EXISTS "); sb.Write(q.Column.Name)
    sb.Write(" "); sb.Write(postgresType(q.Column.Type))

case orm.ActionRenameColumn:
    if q.Column == nil || q.OldName == "" || q.Table == "" {
        return "", nil, fmt.Err("table, old name and column required for rename")
    }
    sb.Write("ALTER TABLE "); sb.Write(q.Table)
    sb.Write(" RENAME COLUMN "); sb.Write(q.OldName)
    sb.Write(" TO "); sb.Write(q.Column.Name)

case orm.ActionDropColumn:
    if q.Table == "" || len(q.Columns) == 0 {
        return "", nil, fmt.Err("table and column required for drop column")
    }
    sb.Write("ALTER TABLE "); sb.Write(q.Table)
    sb.Write(" DROP COLUMN IF EXISTS "); sb.Write(q.Columns[0])
```

### 3.4 Null-operator conditions (Tier 2 prerequisite)

In `buildConditions`, before the bind-arg branch, special-case operators that take **no value**:

```go
op := c.Operator()
if op == "IS NULL" || op == "IS NOT NULL" {
    sb.Write(c.Field()); sb.Write(" "); sb.Write(op) // no placeholder, no arg
    continue
}
```
(Needed so the safe-drop `SELECT 1 FROM t WHERE col IS NOT NULL LIMIT 1` compiles correctly.)

### 3.5 `TableIntrospector` (Tier 2 — enables rename/drop) — **on both base and tx executors**

> **Critical — implement it on `PostgresTx` too.** Per the orm contract, `db.Sync` runs its work in
> a transaction whenever the executor implements `orm.TxExecutor` (this adapter does, via
> [tx.go](../tx.go) `BeginTx`), and it performs the `db.exec.(orm.TableIntrospector)` cast **inside**
> that transaction. So the executor it inspects is `*PostgresTx`, **not** `*PostgresAdapter`. If
> `TableColumns` lives only on the base adapter, the cast **fails inside the tx** and reconcile
> silently degrades to additive-only. (Querying via the tx is also correct — it sees the table
> `CreateTable` just made in the same tx.)

Share one helper, expose it on both executor types (both already have
`Query(string, ...any) (orm.Rows, error)`):

```go
func tableColumns(q interface{ Query(string, ...any) (orm.Rows, error) }, table string) ([]string, error) {
    rows, err := q.Query(
        `SELECT column_name FROM information_schema.columns WHERE table_name = $1`, table)
    if err != nil { return nil, err }
    defer rows.Close()
    var cols []string
    for rows.Next() {
        var c string
        if err := rows.Scan(&c); err != nil { return nil, err }
        cols = append(cols, c)
    }
    return cols, rows.Err()
}

func (p *PostgresAdapter) TableColumns(table string) ([]string, error) { return tableColumns(p, table) }
func (p *PostgresTx)      TableColumns(table string) ([]string, error) { return tableColumns(p, table) }
```
With this present, `db.Sync` reconciles (rename via `old_name`, safe-drop). Without it, `db.Sync`
stays additive — both are correct.

---

## 4. Implementation Steps

### Step 1 — Bump orm
`go get github.com/tinywasm/orm@vX` (registry + `SyncSchema` + `ErrNoRows` + column actions).

### Step 2 — Register + error mapping (Tier 1)
[adapter.go](../adapter.go): add `init()` (§3.1) and the `errScanner` wrapper on `QueryRow` (§3.2).

### Step 3 — Translate the actions (§3.3)
[translate.go](../translate.go): add the three `case`s before `default`.

### Step 4 — Null operators (§3.4)
[translate.go](../translate.go): special-case `IS NULL`/`IS NOT NULL` in `buildConditions`.

### Step 5 — Introspector (§3.5)
[adapter.go](../adapter.go): add `TableColumns`.

### Step 6 — Documentation
README/architecture: note the adapter registers as `"postgres"`, maps `ErrNoRows`, and supports the
full reconcile via `TableIntrospector`.

---

## 5. Edge Cases

- **`q.Column == nil` / empty table / empty `Columns`** → explicit error per action.
- **Column already exists / missing on drop** → `IF NOT EXISTS` / `IF EXISTS` make it a no-op.
- **Column marked NOT NULL/PK/unique in the model** → emitted **nullable** on `ADD COLUMN` (additive).
- **`IS NOT NULL` condition** → no placeholder/arg (§3.4).
- **No `TableIntrospector`** → `db.Sync` degrades to additive-only (no rename/drop). Still correct.

---

## 6. Test Strategy

`gotest` in `tinywasm/postgres/tests/`. `translate` cases via the exported `Translate(q, m)` hook.

| # | Case | Assert |
|---|------|--------|
| P1 | `ActionAddColumn`, `FieldText` | `ALTER TABLE x ADD COLUMN IF NOT EXISTS c TEXT` |
| P2 | `ActionAddColumn`, `FieldInt` | type via `postgresType` (`BIGINT`) |
| P3 | `ActionAddColumn`, col marked NOT NULL/PK | nullable, no `PRIMARY KEY`/`NOT NULL` |
| P4 | `ActionRenameColumn` (`OldName`→`Column.Name`) | `ALTER TABLE x RENAME COLUMN old TO new` |
| P5 | `ActionDropColumn` (`Columns[0]`) | `ALTER TABLE x DROP COLUMN IF EXISTS c` |
| P6 | guards: nil column / empty old name / empty table | error returned |
| P7 | `buildConditions` with `IsNotNull(col)` | `... WHERE col IS NOT NULL` (no `$N`, no arg) |
| P8 | `init()` registered | `orm.Open("postgres://…")` resolves (mock factory or build check) |
| P9 | `QueryRow` no rows | `Scan` returns `orm.ErrNoRows`, not `sql.ErrNoRows` |
| P10 | `TableColumns` against a temp table | returns its column names |

---

## 7. Out of Scope

- `db.Sync` / `db.SyncSchema` algorithm and the registry contract — orm core plan.
- The compiler-only sibling for SQLite (`tinywasm/sqlt`) and the SQLite executor
  (`tinywasm/sqlite`) — their own plans.
- Destructive type-change / table rebuild — deferred (additive dev sync only).
