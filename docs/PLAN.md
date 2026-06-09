# PLAN — Implement Introspection, Renames, and Safe Deletes (Postgres adapter)

> The `tinywasm/orm` dev schema-sync (`db.Sync`) now supports introspective checks, renames, and safe deletes.
> This adapter must comply with the extended contract by implementing the `TableIntrospector` interface
> and translating `ActionRenameColumn` and `ActionDropColumn` actions.

---

## 1. Development Rules (constraints copied for execution context)

- **Pure translator + Executor extension.** 
  - `translate(q, m)` remains a pure query-to-SQL translator. It does **not** query the DB.
  - The `TableIntrospector` method `TableColumns` is implemented on the Postgres `Executor` (or connection adapter) to read the database catalog.
- **Postgres dialect.** This module owns Postgres-specific SQL. Reuse the existing `postgresType()` type mapping.
- **Idempotency.** 
  - `ActionAddColumn` emits `ALTER TABLE … ADD COLUMN IF NOT EXISTS …`.
  - `ActionRenameColumn` emits `ALTER TABLE … RENAME COLUMN old TO new`. (Postgres doesn't support IF EXISTS on RENAME COLUMN directly, but the introspector ensures this is only called if the old column exists and the new one doesn't).
  - `ActionDropColumn` emits `ALTER TABLE … DROP COLUMN IF EXISTS …`.
- **gotest.** Assert the generated SQL strings and verify the catalog query returns the correct columns.

---

## 2. Problem

The current postgres adapter does not implement `TableIntrospector`, so `db.Sync()` cannot query the database's current column list. Also, `translate.go` has no cases to compile `ActionRenameColumn` and `ActionDropColumn`, falling back to an unsupported action error.

---

## 3. Decision

1. **Implement `TableIntrospector` on the Postgres Executor:**
   ```go
   func (e *PostgresExecutor) TableColumns(table string) ([]string, error) {
       // Query information_schema to retrieve column names
       query := `SELECT column_name FROM information_schema.columns WHERE table_name = $1 AND table_schema = current_schema()`
       // Execute and scan column names into a slice
   }
   ```
2. **Translate `ActionRenameColumn`:**
   ```sql
   ALTER TABLE <q.Table> RENAME COLUMN <q.OldName> TO <q.Column.Name>
   ```
3. **Translate `ActionDropColumn`:**
   ```sql
   ALTER TABLE <q.Table> DROP COLUMN IF EXISTS <q.Columns[0]>
   ```

---

## 4. Implementation Steps

### Step 1 — Bump the orm dependency
`go get github.com/tinywasm/orm@vX` (the version containing the new Action constants and `TableIntrospector` / `RenameProvider` definitions).

### Step 2 — Implement `TableIntrospector`
**File:** [postgres.go](../postgres.go) (or where the Executor type is defined)

Add the method `TableColumns(table string) ([]string, error)` to satisfy the interface. Query `information_schema.columns`.

### Step 3 — Add the new translation cases
**File:** [translate.go](../translate.go)

Under `switch q.Action`:
```go
case orm.ActionAddColumn:
    if q.Column == nil {
        return "", nil, fmt.Err("column is required for add column")
    }
    return fmt.Sprintf("ALTER TABLE %s ADD COLUMN IF NOT EXISTS %s %s", q.Table, q.Column.Name, postgresType(q.Column.Type)), nil, nil

case orm.ActionRenameColumn:
    if q.OldName == "" || q.Column == nil {
        return "", nil, fmt.Err("old column name and target column metadata are required for rename")
    }
    return fmt.Sprintf("ALTER TABLE %s RENAME COLUMN %s TO %s", q.Table, q.OldName, q.Column.Name), nil, nil

case orm.ActionDropColumn:
    if len(q.Columns) == 0 {
        return "", nil, fmt.Err("columns are required for drop column")
    }
    return fmt.Sprintf("ALTER TABLE %s DROP COLUMN IF EXISTS %s", q.Table, q.Columns[0]), nil, nil
```

---

## 5. Edge Cases

- **Empty/nil structures** → Explicit errors.
- **Table/Column case sensitivity** → Match Postgres lowercase conventions or double-quote if schema demands.

---

## 6. Test Strategy

`gotest` in `tinywasm/postgres/tests/`.

| # | Case | Assert |
|---|------|--------|
| P1 | `ActionAddColumn` | `ALTER TABLE x ADD COLUMN IF NOT EXISTS c TEXT` |
| P2 | `ActionRenameColumn` | `ALTER TABLE x RENAME COLUMN old TO new` |
| P3 | `ActionDropColumn` | `ALTER TABLE x DROP COLUMN IF EXISTS c` |
| P4 | Introspector check | `TableColumns("users")` returns columns via information_schema |
